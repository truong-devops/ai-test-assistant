package generation

import (
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
)

func TestRenderPromptIncludesRecommendationContextAndRedactsDiff(t *testing.T) {
	data := PromptData{
		AnalysisID: 1, ProjectID: 2, MergeRequestIID: 3,
		ChangedFilePath: "internal/user/service.go", Diff: `+api_key = "super-secret-value"`,
		Symbol: job.ChangedSymbol{ID: 4, SymbolName: "CreateUser", SymbolKind: "method",
			PackageName: "user", ChangeType: "modified"},
		Recommendation: recommendation.Recommendation{ID: 5, Title: "Duplicate email",
			Scenario: "lookup finds a user", ExpectedBehavior: "returns ErrEmailExists"},
		Contexts: []knowledge.KnowledgeChunk{{FilePath: "internal/user/service_test.go",
			PackageName: "user", SymbolName: "TestCreateUser", ChunkType: "test",
			Content: "func TestCreateUser(t *testing.T) {}", StartLine: 1, EndLine: 1}},
	}
	first, err := RenderPrompt(data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderPrompt(data)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || strings.Contains(first, "super-secret-value") ||
		!strings.Contains(first, "[REDACTED") || !strings.Contains(first, "Duplicate email") ||
		!strings.Contains(first, "TestCreateUser") || !strings.Contains(first, "Do not add build constraints") {
		t.Fatalf("prompt=%s", first)
	}
}

func TestRenderPromptRejectsIncompleteData(t *testing.T) {
	if _, err := RenderPrompt(PromptData{}); err == nil {
		t.Fatal("RenderPrompt() error=nil")
	}
}
