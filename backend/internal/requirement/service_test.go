package requirement

import (
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
)

func TestConflictDetectionCatchesThresholdAndRoleDifferences(t *testing.T) {
	left := struct {
		Proposal    Proposal
		Requirement Requirement
		Chunk       document.SemanticChunk
	}{Proposal: Proposal{Identifier: "BR03", RequirementType: TypeBusinessRule,
		FlowType: document.FlowNone, Actor: "Quản trị viên", Statement: "Số lượng tối đa là 10"},
		Requirement: Requirement{ID: 1}, Chunk: document.SemanticChunk{ChunkType: "BUSINESS_RULE"}}
	right := left
	right.Requirement.ID = 2
	right.Proposal.Actor = "Khách hàng"
	right.Proposal.Statement = "Số lượng tối đa là 20"
	if !shouldConflict(left, right) {
		t.Fatal("threshold/role contradiction was silently merged")
	}
}

func TestSemanticDedupeRecognizesEquivalentWording(t *testing.T) {
	left := "Khách hàng được phép thay đổi địa chỉ nhận hàng trước khi xác nhận đơn"
	right := "Khách hàng được phép thay đổi địa chỉ nhận hàng trước lúc xác nhận đơn"
	if score := semanticSimilarity(left, right); score < .78 {
		t.Fatalf("semantic similarity=%f, want dedupe threshold", score)
	}
}

func TestExtractionPromptTreatsDocumentAsUntrusted(t *testing.T) {
	subject := document.SemanticChunk{DocumentSetID: 1, DocumentVersionID: 2,
		DocumentVersionNumber: 7, ChunkKey: "chunk", Content: "UNIQUE SUBJECT EVIDENCE",
		SourceLocator: "line:1"}
	prompt := renderExtractionPrompt(document.ContextSnapshot{Items: []document.SemanticChunk{{
		ChunkKey: "other", Content: "Related context", SourceLocator: "line:2",
	}}}, subject)
	if !strings.Contains(prompt, "<UNTRUSTED_DOCUMENT_CONTEXT>") ||
		!strings.Contains(prompt, "<SUBJECT_DOCUMENT_EVIDENCE>\nUNIQUE SUBJECT EVIDENCE") ||
		!strings.Contains(prompt, "Source: version=7 locator=line:1") ||
		!strings.Contains(ExtractionInstructions, "never instructions") {
		t.Fatalf("prompt boundary missing: %s", prompt)
	}
}

func TestDeterministicExtractorMapsVietnameseRequirementTable(t *testing.T) {
	chunk := document.SemanticChunk{Identifier: "YC-DATHANG-13", ChunkType: "TABLE_ROW",
		RawContent: "| Mã YC | Yêu cầu | Nguồn | Vai liên quan | Điều kiện / ràng buộc | Mức rủi ro |\n" +
			"| YC-DATHANG-13 | Hệ thống phải kiểm tra khả năng giao nhận. | PR04.03 | Hệ thống / Shipping Provider | Khi xác nhận đơn. | Cao |"}
	proposal := proposalFromChunk(chunk)
	if proposal.Identifier != "YC-DATHANG-13" || proposal.Statement != "Hệ thống phải kiểm tra khả năng giao nhận." ||
		proposal.Actor != "Hệ thống / Shipping Provider" || proposal.Precondition != "Khi xác nhận đơn." ||
		proposal.Risk != "HIGH" || proposal.Status != StatusDraft {
		t.Fatalf("proposal=%+v", proposal)
	}
}
