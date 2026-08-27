package validation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type ProjectReader interface {
	GetByID(ctx context.Context, id int64) (project.Project, error)
}

type GeneratedTestReader interface {
	List(ctx context.Context, analysisID int64) ([]generation.GeneratedTest, error)
}

type WorkspacePreparer interface {
	Prepare(ctx context.Context, projectID int64, ref string,
		generated generation.GeneratedTest) (*Workspace, error)
}

type ResultSaver interface {
	Save(ctx context.Context, claimed job.AnalysisJob, runs []Run) error
}

type Processor struct {
	projects   ProjectReader
	generated  GeneratedTestReader
	workspaces WorkspacePreparer
	runner     SandboxRunner
	results    ResultSaver
	timeout    time.Duration
}

func NewProcessor(projects ProjectReader, generated GeneratedTestReader,
	workspaces WorkspacePreparer, runner SandboxRunner, results ResultSaver,
	timeout time.Duration,
) *Processor {
	return &Processor{projects: projects, generated: generated, workspaces: workspaces,
		runner: runner, results: results, timeout: timeout}
}

func (p *Processor) Process(ctx context.Context, claimed job.AnalysisJob) error {
	if claimed.ID <= 0 || claimed.ProjectID <= 0 || strings.TrimSpace(claimed.SourceSHA) == "" {
		return fmt.Errorf("claimed validation analysis is incomplete")
	}
	projectItem, err := p.projects.GetByID(ctx, claimed.ProjectID)
	if err != nil {
		return fmt.Errorf("get validation project: %w", err)
	}
	if projectItem.ID != claimed.ProjectID || projectItem.GitLabProjectID <= 0 {
		return fmt.Errorf("validation project identity does not match claimed analysis")
	}
	generatedTests, err := p.generated.List(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("list generated tests for validation: %w", err)
	}
	command := validationCommand(p.timeout)
	runs := make([]Run, 0, len(generatedTests))
	for _, generatedTest := range generatedTests {
		if generatedTest.AnalysisJobID != claimed.ID {
			return fmt.Errorf("generated test %d belongs to another analysis", generatedTest.ID)
		}
		run, err := p.validateOne(ctx, claimed, projectItem.GitLabProjectID,
			generatedTest, command)
		if err != nil {
			return err
		}
		runs = append(runs, run)
	}
	return p.results.Save(ctx, claimed, runs)
}

func (p *Processor) validateOne(ctx context.Context, claimed job.AnalysisJob,
	gitLabProjectID int64, generatedTest generation.GeneratedTest, command []string,
) (Run, error) {
	base := Run{AnalysisJobID: claimed.ID, GeneratedTestID: generatedTest.ID,
		AttemptNumber: generatedTest.GenerationAttempt, Command: strings.Join(command, " ")}
	workspace, err := p.workspaces.Prepare(ctx, gitLabProjectID, claimed.SourceSHA, generatedTest)
	if err != nil {
		if errors.Is(err, ErrGeneratedTargetExists) {
			base.Status = StatusFailed
			base.ExitCode = -1
			base.Stderr = sanitizeOutput(err.Error())
			return base, nil
		}
		return Run{}, fmt.Errorf("prepare workspace for generated test %d: %w", generatedTest.ID, err)
	}
	result, runErr := p.runner.Run(ctx, SandboxRequest{Workspace: workspace.Root, Command: command})
	cleanupErr := workspace.Cleanup()
	if runErr != nil {
		return Run{}, fmt.Errorf("run sandbox for generated test %d: %w", generatedTest.ID, runErr)
	}
	if cleanupErr != nil {
		return Run{}, fmt.Errorf("remove workspace for generated test %d: %w", generatedTest.ID, cleanupErr)
	}
	base.ExitCode = result.ExitCode
	base.DurationMS = result.Duration.Milliseconds()
	base.Stdout = sanitizeOutput(result.Stdout)
	base.Stderr = sanitizeOutput(result.Stderr)
	base.OutputTruncated = result.OutputTruncated
	switch {
	case result.TimedOut:
		base.Status = StatusTimedOut
	case result.ExitCode == 0:
		base.Status = StatusPassed
	default:
		base.Status = StatusFailed
	}
	return base, nil
}

func validationCommand(timeout time.Duration) []string {
	testTimeout := timeout - 5*time.Second
	if testTimeout < time.Second {
		testTimeout = time.Second
	}
	return []string{"go", "test", "-count=1", "-timeout=" + testTimeout.String(), "./..."}
}
