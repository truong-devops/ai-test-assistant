package execution

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

type ProjectReader interface {
	GetByID(context.Context, int64) (project.Project, error)
}

type WorkspacePreparer interface {
	PrepareSource(context.Context, scm.Repository, string) (*validation.Workspace, error)
}

type Sandbox interface {
	Run(context.Context, validation.SandboxRequest) (validation.SandboxResult, error)
	Describe(context.Context, []string) (validation.SandboxEnvironment, error)
}

type ResultStore interface {
	LoadAttempt(context.Context, Run) (Run, error)
	Save(context.Context, Run, validation.SandboxEnvironment, []Outcome, int, time.Duration) error
}

type Processor struct {
	projects         ProjectReader
	workspaces       WorkspacePreparer
	sandbox          Sandbox
	repository       ResultStore
	timeout          time.Duration
	maxInfraAttempts int
	retryDelay       time.Duration
}

func NewProcessor(projects ProjectReader, workspaces WorkspacePreparer, sandbox Sandbox,
	repository ResultStore, timeout time.Duration, maxInfraAttempts int, retryDelay time.Duration,
) *Processor {
	return &Processor{projects: projects, workspaces: workspaces, sandbox: sandbox,
		repository: repository, timeout: timeout, maxInfraAttempts: maxInfraAttempts,
		retryDelay: retryDelay}
}

func (p *Processor) Process(ctx context.Context, claimed Run) error {
	if claimed.ProjectID == nil || *claimed.ProjectID <= 0 || claimed.AnalysisJobID == nil ||
		strings.TrimSpace(claimed.SourceSHA) == "" {
		return fmt.Errorf("claimed document test run is incomplete")
	}
	loaded, err := p.repository.LoadAttempt(ctx, claimed)
	if err != nil {
		return err
	}
	projectItem, err := p.projects.GetByID(ctx, *claimed.ProjectID)
	if err != nil {
		return fmt.Errorf("get execution project: %w", err)
	}
	command := executionCommand(p.timeout)
	environment, describeErr := p.sandbox.Describe(ctx, command)
	outcomes := make([]Outcome, 0, len(loaded.Items))
	for _, item := range loaded.Items {
		if item.Artifact == nil {
			outcomes = append(outcomes, p.executeOne(ctx, projectItem.RepositoryRef(), loaded.SourceSHA,
				item, command))
			continue
		}
		if describeErr != nil {
			outcomes = append(outcomes, infraOutcome(item, command, "Sandbox image is unavailable: "+describeErr.Error()))
			continue
		}
		outcomes = append(outcomes, p.executeOne(ctx, projectItem.RepositoryRef(), loaded.SourceSHA,
			item, command))
	}
	return p.repository.Save(context.WithoutCancel(ctx), claimed, environment, outcomes,
		p.maxInfraAttempts, p.retryDelay)
}

func (p *Processor) executeOne(ctx context.Context, repository scm.Repository, sourceSHA string,
	item Item, command []string,
) Outcome {
	commandText := strings.Join(command, " ")
	if item.Artifact == nil {
		return Outcome{ItemID: item.ID, Status: StatusBlocked, Command: commandText,
			ActualResult: "No approved automation artifact exists for this test case and analysis."}
	}
	artifactID := item.Artifact.ID
	base := Outcome{ItemID: item.ID, ArtifactID: &artifactID,
		AutomationSourceHash: item.Artifact.SourceHash, Command: commandText,
		Evidence: []Evidence{{EvidenceType: "ARTIFACT", StorageKey: "automation-artifact:" + strconv.FormatInt(artifactID, 10), ContentHash: item.Artifact.SourceHash}}}
	if item.Artifact.ExpectedResultHash != item.ExpectedResultHash || item.Artifact.Status != "APPROVED" {
		base.Status, base.ActualResult = StatusBlocked, "Automation artifact does not match the immutable approved expected result."
		return base
	}
	workspace, err := p.workspaces.PrepareSource(ctx, repository, sourceSHA)
	if err != nil {
		base.Status, base.ActualResult = StatusInfraError, "Could not prepare source-SHA workspace: "+err.Error()
		return base
	}
	defer workspace.Cleanup()

	baseline, err := p.sandbox.Run(ctx, validation.SandboxRequest{Workspace: workspace.Root, Command: command})
	if err != nil {
		base.Status, base.ActualResult = StatusInfraError, "Could not run baseline suite: "+err.Error()
		return base
	}
	base.Evidence = appendOutputEvidence(base.Evidence, "baseline", baseline)
	if baseline.TimedOut {
		base.Status, base.ActualResult, base.ExitCode = StatusTimedOut,
			"Baseline suite timed out before the generated artifact was introduced.", intPointer(baseline.ExitCode)
		base.DurationMS, base.OutputTruncated = baseline.Duration.Milliseconds(), baseline.OutputTruncated
		return base
	}
	if baseline.ExitCode != 0 {
		status, actual := Classify(baseline.ExitCode, false, baseline.Stdout, baseline.Stderr, "", true)
		if status != StatusInfraError {
			status = StatusBlocked
		}
		base.Status, base.ActualResult, base.ExitCode = status,
			"Baseline suite failed before generated automation: "+actual, intPointer(baseline.ExitCode)
		base.DurationMS, base.OutputTruncated = baseline.Duration.Milliseconds(), baseline.OutputTruncated
		return base
	}
	if err := workspace.AddGeneratedFile(item.Artifact.FilePath, []byte(item.Artifact.Source)); err != nil {
		base.Status, base.ActualResult = StatusAutomationError, "Could not install generated automation: "+err.Error()
		return base
	}
	result, err := p.sandbox.Run(ctx, validation.SandboxRequest{Workspace: workspace.Root, Command: command})
	if err != nil {
		base.Status, base.ActualResult = StatusInfraError, "Could not run generated automation: "+err.Error()
		return base
	}
	base.Status, base.ActualResult = Classify(result.ExitCode, result.TimedOut, result.Stdout,
		result.Stderr, item.Artifact.FilePath, true)
	base.ExitCode, base.DurationMS = intPointer(result.ExitCode), result.Duration.Milliseconds()
	base.OutputTruncated = result.OutputTruncated
	base.Evidence = appendOutputEvidence(base.Evidence, "generated", result)
	return base
}

func appendOutputEvidence(items []Evidence, label string, result validation.SandboxResult) []Evidence {
	if strings.TrimSpace(result.Stdout) != "" {
		items = append(items, Evidence{EvidenceType: "STDOUT", Content: label + ":\n" + result.Stdout})
	}
	if strings.TrimSpace(result.Stderr) != "" {
		items = append(items, Evidence{EvidenceType: "STDERR", Content: label + ":\n" + result.Stderr})
	}
	return items
}

func infraOutcome(item Item, command []string, actual string) Outcome {
	return Outcome{ItemID: item.ID, Status: StatusInfraError, Command: strings.Join(command, " "), ActualResult: actual}
}

func executionCommand(timeout time.Duration) []string {
	testTimeout := timeout - 5*time.Second
	if testTimeout < time.Second {
		testTimeout = time.Second
	}
	return []string{"go", "test", "-count=1", "-timeout=" + testTimeout.String(), "./..."}
}

func intPointer(value int) *int { return &value }
