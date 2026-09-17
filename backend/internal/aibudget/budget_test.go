package aibudget

import (
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
)

func TestEstimateCostUsesMicroUSDPerTokenEquivalent(t *testing.T) {
	if got := estimateCost(1_000_000, 500_000, 2.5, 10); got != 7_500_000 {
		t.Fatalf("estimateCost() = %d, want 7500000", got)
	}
}

func TestEstimateInputTokensIsConservative(t *testing.T) {
	request := llm.Request{Instructions: "abc", Input: "đặt hàng",
		SchemaName: "result", Schema: map[string]any{"type": "object"}}
	wantMinimum := int64(len(request.Instructions) + len(request.Input) + len(request.SchemaName))
	if got := estimateInputTokens(request); got < wantMinimum {
		t.Fatalf("estimateInputTokens() = %d, want at least %d", got, wantMinimum)
	}
}
