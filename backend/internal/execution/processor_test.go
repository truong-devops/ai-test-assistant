package execution

import (
	"context"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

type executionProjectStub struct{ item project.Project }

func (s executionProjectStub) GetByID(context.Context, int64) (project.Project, error) {
	return s.item, nil
}

type executionSourceStub struct{}

func (executionSourceStub) ListRepositoryTree(context.Context, scm.Repository, string) ([]scm.RepositoryEntry, error) {
	return []scm.RepositoryEntry{{Path: "go.mod", Type: "blob", Mode: "100644"}, {Path: "cart/cart.go", Type: "blob", Mode: "100644"}}, nil
}
func (executionSourceStub) GetFileRaw(_ context.Context, _ scm.Repository, path, _ string) ([]byte, error) {
	if path == "go.mod" {
		return []byte("module fixture\n\ngo 1.25\n"), nil
	}
	return []byte("package cart\nfunc Create() string { return \"rejected\" }\n"), nil
}

type executionSandboxStub struct {
	results []validation.SandboxResult
	calls   int
}

func (s *executionSandboxStub) Describe(context.Context, []string) (validation.SandboxEnvironment, error) {
	return validation.SandboxEnvironment{ImageReference: "sandbox:test", ImageDigest: "sha256:image", Fingerprint: "fingerprint"}, nil
}
func (s *executionSandboxStub) Run(context.Context, validation.SandboxRequest) (validation.SandboxResult, error) {
	result := s.results[s.calls]
	s.calls++
	return result, nil
}

type executionStoreStub struct {
	loaded      Run
	outcomes    []Outcome
	environment validation.SandboxEnvironment
}

func (s *executionStoreStub) LoadAttempt(context.Context, Run) (Run, error) { return s.loaded, nil }
func (s *executionStoreStub) Save(_ context.Context, _ Run, environment validation.SandboxEnvironment, outcomes []Outcome, _ int, _ time.Duration) error {
	s.environment = environment
	s.outcomes = outcomes
	return nil
}

func TestProcessorRunsBaselineThenClassifiesApprovedAssertionFailure(t *testing.T) {
	projectID, analysisID := int64(3), int64(7)
	claimed := Run{ID: 11, ProjectID: &projectID, AnalysisJobID: &analysisID, SourceSHA: "source", AttemptCount: 1}
	store := &executionStoreStub{loaded: Run{ID: claimed.ID, ProjectID: &projectID, AnalysisJobID: &analysisID, SourceSHA: "source", Items: []Item{{ID: 21, ExpectedResultHash: "expected", Artifact: &Artifact{ID: 31, FilePath: "cart/generated_test.go", Source: "package cart\nimport \"testing\"\nfunc TestCreate(t *testing.T){if Create()!=\"created\"{t.Fatalf(\"expected created got %s\",Create())}}\n", SourceHash: "artifact-hash", ExpectedResultHash: "expected", Status: "APPROVED"}}}}}
	sandbox := &executionSandboxStub{results: []validation.SandboxResult{{ExitCode: 0, Duration: time.Millisecond}, {ExitCode: 1, Duration: 2 * time.Millisecond, Stdout: "--- FAIL: TestCreate expected created got rejected"}}}
	processor := NewProcessor(executionProjectStub{project.Project{ID: projectID, Provider: "github", ProviderProjectID: 99}}, validation.NewWorkspaceManager(executionSourceStub{}, validation.WorkspaceOptions{}), sandbox, store, 20*time.Second, 2, time.Second)
	if err := processor.Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if sandbox.calls != 2 || len(store.outcomes) != 1 || store.outcomes[0].Status != StatusProductFailed {
		t.Fatalf("calls=%d outcomes=%+v", sandbox.calls, store.outcomes)
	}
	if store.outcomes[0].ArtifactID == nil || *store.outcomes[0].ArtifactID != 31 || store.environment.ImageDigest != "sha256:image" {
		t.Fatalf("outcome/environment not reproducible: %+v %+v", store.outcomes[0], store.environment)
	}
}

func TestProcessorBlocksMissingApprovedArtifactWithoutRunningSandbox(t *testing.T) {
	projectID, analysisID := int64(3), int64(7)
	claimed := Run{ID: 11, ProjectID: &projectID, AnalysisJobID: &analysisID, SourceSHA: "source", AttemptCount: 1}
	store := &executionStoreStub{loaded: Run{ID: 11, ProjectID: &projectID, AnalysisJobID: &analysisID, SourceSHA: "source", Items: []Item{{ID: 22}}}}
	sandbox := &executionSandboxStub{}
	processor := NewProcessor(executionProjectStub{project.Project{ID: projectID, Provider: "github", ProviderProjectID: 99}}, validation.NewWorkspaceManager(executionSourceStub{}, validation.WorkspaceOptions{}), sandbox, store, 20*time.Second, 2, time.Second)
	if err := processor.Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if sandbox.calls != 0 || len(store.outcomes) != 1 || store.outcomes[0].Status != StatusBlocked {
		t.Fatalf("calls=%d outcomes=%+v", sandbox.calls, store.outcomes)
	}
}
