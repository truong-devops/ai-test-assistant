package provenance

import (
	"math"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
)

func TestHashTextIsStable(t *testing.T) {
	first := HashText("prompt")
	second := HashText("prompt")
	if first != second || len(first) != 64 || first == HashText("other") {
		t.Fatalf("unexpected hashes: %q %q", first, second)
	}
}

func TestEstimateCost(t *testing.T) {
	got := estimateCost(1_000_000, 500_000, 2.5, 10)
	if got != 7.5 {
		t.Fatalf("estimateCost()=%v want 7.5", got)
	}
	if got := estimateCost(1, 1, math.NaN(), 1); got != 0 {
		t.Fatalf("estimateCost()=%v want 0 for a non-finite rate", got)
	}
}

func TestTruncateBytesPreservesUTF8(t *testing.T) {
	if got := truncateBytes("abcễ", 4); got != "abc" {
		t.Fatalf("truncateBytes()=%q want %q", got, "abc")
	}
}

func TestValidateRecordInput(t *testing.T) {
	valid := RecordInput{
		Analysis: job.AnalysisJob{ID: 1, ProjectID: 2, SourceSHA: "head", TargetSHA: "base"},
		Phase:    PhaseRecommendation, SubjectID: 3, AttemptNumber: 1,
		PromptVersion: "v1", Status: StatusCompleted, Latency: time.Millisecond,
		Request: llm.Request{Instructions: "safe", Input: "prompt", SchemaName: "result",
			Schema: map[string]any{"type": "object"}},
	}
	if err := validateRecordInput(valid); err != nil {
		t.Fatal(err)
	}
	valid.Phase = "unknown"
	if err := validateRecordInput(valid); err == nil {
		t.Fatal("validateRecordInput() error=nil, want unsupported phase")
	}
}
