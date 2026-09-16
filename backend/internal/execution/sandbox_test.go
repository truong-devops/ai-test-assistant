//go:build sandbox

package execution

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

func TestDocumentArtifactRunsAfterCleanBaselineInDocker(t *testing.T) {
	if os.Getenv("RUN_SANDBOX_TESTS") != "1" {
		t.Skip("RUN_SANDBOX_TESTS=1 is required")
	}
	image := os.Getenv("SANDBOX_TEST_IMAGE")
	if image == "" {
		image = "ai-test-assistant-sandbox:phase7"
	}
	runner, err := validation.NewDockerRunner(validation.DockerConfig{Image: image, Timeout: 60 * time.Second, MemoryMB: 512, CPUs: 1, PIDsLimit: 64, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	projectID, analysisID := int64(3), int64(7)
	claimed := Run{ID: 11, ProjectID: &projectID, AnalysisJobID: &analysisID, SourceSHA: "source", AttemptCount: 1}
	store := &executionStoreStub{loaded: Run{ID: 11, ProjectID: &projectID, AnalysisJobID: &analysisID, SourceSHA: "source", Items: []Item{{ID: 21, ExpectedResultHash: "expected", Artifact: &Artifact{ID: 31, FilePath: "cart/generated_test.go", Source: "package cart\nimport \"testing\"\nfunc TestCreate(t *testing.T){if Create()!=\"rejected\"{t.Fatal(\"unexpected\")}}\n", SourceHash: "artifact", ExpectedResultHash: "expected", Status: "APPROVED"}}}}}
	processor := NewProcessor(executionProjectStub{project.Project{ID: projectID, Provider: "github", ProviderProjectID: 99}}, validation.NewWorkspaceManager(executionSourceStub{}, validation.WorkspaceOptions{}), runner, store, 60*time.Second, 2, time.Second)
	if err := processor.Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if len(store.outcomes) != 1 || store.outcomes[0].Status != StatusPassed || store.environment.ImageDigest == "" || store.environment.Fingerprint == "" {
		t.Fatalf("outcome=%+v environment=%+v", store.outcomes, store.environment)
	}
}
