package repair

import (
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

func TestRenderPromptIncludesFailureAndEnforcesImmutableProductionCode(t *testing.T) {
	data := PromptData{
		AnalysisID: 1, ProjectID: 2, MergeRequestIID: 3, RepairAttempt: 1,
		ChangedFilePath: "internal/user/service.go",
		Symbol: job.ChangedSymbol{ID: 4, SymbolName: "CreateUser", SymbolKind: "method",
			PackageName: "user", ChangeType: "modified"},
		Recommendation: recommendation.Recommendation{ID: 5, Title: "Duplicate email",
			Scenario: "lookup finds user", ExpectedBehavior: "returns ErrEmailExists"},
		Previous: generation.GeneratedTest{ID: 6, FilePath: "internal/user/service_generated_test.go",
			Code: "package user\n", GenerationAttempt: 1},
		Validation: validation.Run{ID: 7, Status: validation.StatusFailed, ExitCode: 1,
			Command: "go test ./...", Stderr: "undefined: missingRepository"},
		Contexts: []knowledge.KnowledgeChunk{{FilePath: "internal/user/service_test.go",
			PackageName: "user", ChunkType: "test", Content: "func TestCreateUser(t *testing.T) {}"}},
	}
	first, err := RenderPrompt(data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderPrompt(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"undefined: missingRepository", "Duplicate email",
		"service_generated_test.go", "Modify only the generated test file",
		"Do not delete or weaken assertions", "TestCreateUser"} {
		if !strings.Contains(first, required) {
			t.Errorf("prompt missing %q", required)
		}
	}
	if first != second {
		t.Fatal("RenderPrompt() is not deterministic")
	}
}

func TestRenderPromptRedactsSensitiveValidationOutput(t *testing.T) {
	data := PromptData{
		AnalysisID: 1, ProjectID: 2, RepairAttempt: 1, ChangedFilePath: "service.go",
		Symbol:         job.ChangedSymbol{ID: 3, SymbolName: "Run", PackageName: "sample"},
		Recommendation: recommendation.Recommendation{ID: 4},
		Previous:       generation.GeneratedTest{ID: 5, FilePath: "service_test.go", Code: "package sample\n"},
		Validation: validation.Run{ID: 6, Status: validation.StatusFailed,
			Stderr: `api_key = "super-secret-value"`},
	}
	prompt, err := RenderPrompt(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "super-secret-value") || !strings.Contains(prompt, "[REDACTED") {
		t.Fatalf("prompt did not redact validation output: %s", prompt)
	}
}

func TestRenderPromptRejectsIncompleteData(t *testing.T) {
	if _, err := RenderPrompt(PromptData{}); err == nil {
		t.Fatal("RenderPrompt() error=nil")
	}
}
