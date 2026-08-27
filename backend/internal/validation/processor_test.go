package validation

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type projectReaderStub struct {
	item project.Project
	err  error
}

func (s projectReaderStub) GetByID(context.Context, int64) (project.Project, error) {
	return s.item, s.err
}

type generatedReaderStub struct {
	items []generation.GeneratedTest
	err   error
}

func (s generatedReaderStub) ListLatest(context.Context, int64) ([]generation.GeneratedTest, error) {
	return s.items, s.err
}

type workspacePreparerStub struct {
	err   error
	paths []string
}

func (s *workspacePreparerStub) Prepare(_ context.Context, _ int64, _ string,
	_ generation.GeneratedTest,
) (*Workspace, error) {
	if s.err != nil {
		return nil, s.err
	}
	base, err := os.MkdirTemp("", "validation-processor-test-")
	if err != nil {
		return nil, err
	}
	s.paths = append(s.paths, base)
	return &Workspace{Root: base, base: base}, nil
}

type sandboxRunnerStub struct {
	results []SandboxResult
	err     error
	calls   int
	request SandboxRequest
}

func (s *sandboxRunnerStub) Run(_ context.Context, request SandboxRequest) (SandboxResult, error) {
	s.request = request
	index := s.calls
	s.calls++
	if s.err != nil {
		return SandboxResult{}, s.err
	}
	return s.results[index], nil
}

type validationSaverStub struct {
	claimed job.AnalysisJob
	runs    []Run
	calls   int
}

func (s *validationSaverStub) Save(_ context.Context, claimed job.AnalysisJob, runs []Run) error {
	s.claimed, s.runs = claimed, append([]Run(nil), runs...)
	s.calls++
	return nil
}

func TestProcessorMapsPassingFailureAndTimeoutResults(t *testing.T) {
	claimed := job.AnalysisJob{ID: 11, ProjectID: 22, SourceSHA: "head",
		Status: job.StatusValidating, AttemptCount: 1}
	generated := []generation.GeneratedTest{
		{ID: 31, AnalysisJobID: 11, GenerationAttempt: 1, FilePath: "pass_test.go", Code: "package test"},
		{ID: 32, AnalysisJobID: 11, GenerationAttempt: 1, FilePath: "fail_test.go", Code: "package test"},
		{ID: 33, AnalysisJobID: 11, GenerationAttempt: 2, FilePath: "timeout_test.go", Code: "package test"},
	}
	workspaces := &workspacePreparerStub{}
	runner := &sandboxRunnerStub{results: []SandboxResult{
		{ExitCode: 0, Duration: 12 * time.Millisecond, Stdout: "ok"},
		{ExitCode: 1, Duration: 5 * time.Millisecond, Stderr: "password=unsafe"},
		{ExitCode: -1, Duration: time.Second, TimedOut: true, OutputTruncated: true},
	}}
	saver := &validationSaverStub{}
	processor := NewProcessor(projectReaderStub{item: project.Project{ID: 22, GitLabProjectID: 99}},
		generatedReaderStub{items: generated}, workspaces, runner, saver, 10*time.Second)
	if err := processor.Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if saver.calls != 1 || len(saver.runs) != 3 || saver.runs[0].Status != StatusPassed ||
		saver.runs[1].Status != StatusFailed || saver.runs[2].Status != StatusTimedOut ||
		saver.runs[2].AttemptNumber != 2 || saver.runs[2].Command != "go test -count=1 -timeout=5s ./..." {
		t.Fatalf("saved runs=%#v", saver.runs)
	}
	if saver.runs[1].Stderr == "password=unsafe" {
		t.Fatalf("secret was not redacted: %#v", saver.runs[1])
	}
	for _, path := range workspaces.paths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("temporary workspace %s remains: %v", path, err)
		}
	}
}

func TestProcessorPersistsTargetCollisionAsRepairableFailure(t *testing.T) {
	claimed := job.AnalysisJob{ID: 1, ProjectID: 2, SourceSHA: "head",
		Status: job.StatusValidating, AttemptCount: 1}
	workspaces := &workspacePreparerStub{err: ErrGeneratedTargetExists}
	runner, saver := &sandboxRunnerStub{}, &validationSaverStub{}
	err := NewProcessor(projectReaderStub{item: project.Project{ID: 2, GitLabProjectID: 3}},
		generatedReaderStub{items: []generation.GeneratedTest{{
			ID: 4, AnalysisJobID: 1, GenerationAttempt: 1,
		}}}, workspaces, runner, saver, time.Minute).Process(context.Background(), claimed)
	if err != nil || runner.calls != 0 || len(saver.runs) != 1 || saver.runs[0].Status != StatusFailed {
		t.Fatalf("Process() error=%v runner calls=%d saved=%#v", err, runner.calls, saver.runs)
	}
}

func TestProcessorDoesNotPersistSandboxInfrastructureFailure(t *testing.T) {
	claimed := job.AnalysisJob{ID: 1, ProjectID: 2, SourceSHA: "head",
		Status: job.StatusValidating, AttemptCount: 1}
	runner := &sandboxRunnerStub{err: errors.New("Docker daemon unavailable")}
	saver := &validationSaverStub{}
	err := NewProcessor(projectReaderStub{item: project.Project{ID: 2, GitLabProjectID: 3}},
		generatedReaderStub{items: []generation.GeneratedTest{{
			ID: 4, AnalysisJobID: 1, GenerationAttempt: 1,
		}}}, &workspacePreparerStub{}, runner, saver, time.Minute).Process(context.Background(), claimed)
	if err == nil || saver.calls != 0 {
		t.Fatalf("Process() error=%v saver calls=%d", err, saver.calls)
	}
}

func TestProcessorCompletesEmptyGenerationWithoutSandbox(t *testing.T) {
	claimed := job.AnalysisJob{ID: 1, ProjectID: 2, SourceSHA: "head",
		Status: job.StatusValidating, AttemptCount: 1}
	runner, saver := &sandboxRunnerStub{}, &validationSaverStub{}
	err := NewProcessor(projectReaderStub{item: project.Project{ID: 2, GitLabProjectID: 3}},
		generatedReaderStub{}, &workspacePreparerStub{}, runner, saver, time.Minute).
		Process(context.Background(), claimed)
	if err != nil || runner.calls != 0 || saver.calls != 1 || len(saver.runs) != 0 {
		t.Fatalf("Process() error=%v runner=%d saver=%#v", err, runner.calls, saver)
	}
}
