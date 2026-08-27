//go:build sandbox

package repair

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

func TestControlledFailRepairPassExperiment(t *testing.T) {
	if os.Getenv("RUN_SANDBOX_TESTS") != "1" {
		t.Skip("RUN_SANDBOX_TESTS=1 is required")
	}
	claimed, analyses, recommendations, generatedTests, validations := repairFixture()
	initial := runGeneratedCodeInSandbox(t, generatedTests.items[0].Code)
	if initial.TimedOut || initial.ExitCode == 0 {
		t.Fatalf("initial generated test unexpectedly passed: %#v", initial)
	}
	provider := &providerStub{result: llm.Response{ID: "controlled-repair",
		Model: "fixture-model", Output: repairedOutput}}
	saver := &saverStub{}
	processor := NewProcessor(analyses, recommendations, generatedTests, validations,
		&retrieverStub{results: []knowledge.KnowledgeChunk{{Content: "exact fixture context"}}},
		provider, saver, 2)
	if err := processor.Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if len(saver.repairs) != 1 ||
		saver.repairs[0].Generated.CodeHash == generatedTests.items[0].CodeHash {
		t.Fatalf("repair did not create a changed version: %#v", saver.repairs)
	}
	final := runGeneratedCodeInSandbox(t, saver.repairs[0].Generated.Code)
	if final.TimedOut || final.ExitCode != 0 {
		t.Fatalf("repaired generated test did not pass: %#v", final)
	}
}

func runGeneratedCodeInSandbox(t *testing.T, code string) validation.SandboxResult {
	t.Helper()
	image := os.Getenv("SANDBOX_TEST_IMAGE")
	if image == "" {
		image = "ai-test-assistant-sandbox:phase7"
	}
	workspace := t.TempDir()
	packageDirectory := filepath.Join(workspace, "internal", "user")
	if err := os.MkdirAll(packageDirectory, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"),
		[]byte("module example.com/repaircase\n\ngo 1.25.0\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDirectory, "service_generated_test.go"),
		[]byte(code), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(workspace, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(workspace, "internal"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(packageDirectory, 0o777); err != nil {
		t.Fatal(err)
	}
	runner, err := validation.NewDockerRunner(validation.DockerConfig{Image: image,
		Timeout: 60 * time.Second, MemoryMB: 512, CPUs: 1, PIDsLimit: 64,
		MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), validation.SandboxRequest{
		Workspace: workspace,
		Command:   []string{"go", "test", "-count=1", "-timeout=55s", "./..."},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
