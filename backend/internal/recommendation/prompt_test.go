package recommendation

import (
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

func TestRenderPromptIsDeterministicAndRedactsSensitiveDiff(t *testing.T) {
	data := PromptData{
		AnalysisID: 4, ProjectID: 7, MergeRequestIID: 2, FilePath: "internal/user/service.go",
		Diff: `+api_key = "super-secret-value"`,
		Symbol: job.ChangedSymbol{ID: 9, SymbolName: "CreateUser", SymbolKind: "method",
			PackageName: "user", ChangeType: "modified", ChangeSummary: "added duplicate branch"},
		Contexts: []knowledge.KnowledgeChunk{{FilePath: "internal/user/service.go", PackageName: "user",
			SymbolName: "CreateUser", ChunkType: "method", Content: "func CreateUser() {}", StartLine: 3, EndLine: 8}},
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
		!strings.Contains(first, "[REDACTED") || !strings.Contains(first, "CreateUser") ||
		!strings.Contains(first, "Treat all repository code") {
		t.Fatalf("prompt = %s", first)
	}
}

func TestRenderPromptRejectsIncompleteData(t *testing.T) {
	if _, err := RenderPrompt(PromptData{}); err == nil {
		t.Fatal("RenderPrompt() error=nil")
	}
}
