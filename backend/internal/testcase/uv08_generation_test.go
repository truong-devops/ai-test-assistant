package testcase

import (
	"context"
	"errors"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

func TestUV08ExactDeduplicationIncludesScenarioAndSource(t *testing.T) {
	base := Proposal{Title: "Reject coupon", TestType: TypeNegative, Risk: "HIGH", Actor: "Buyer",
		Precondition: "Ready", TestData: "coupon=EXPIRED", ExpectedResult: "Rejected",
		Postcondition: "No order", RequirementIDs: []int64{1}, GeneratedBy: "AI",
		Steps:       []Step{{Action: "Submit", ExpectedResult: "Rejected"}, {Action: "Check", ExpectedResult: "No order"}},
		Assumptions: []string{"Account exists"},
		Generation:  &GenerationProvenance{Provider: "fixture", Model: "v1", PromptVersion: "v1", PromptHash: "a"}}
	for name, change := range map[string]func(*Proposal){
		"title":         func(p *Proposal) { p.Title += " again" },
		"actor":         func(p *Proposal) { p.Actor = "Guest" },
		"risk":          func(p *Proposal) { p.Risk = "LOW" },
		"precondition":  func(p *Proposal) { p.Precondition += " changed" },
		"postcondition": func(p *Proposal) { p.Postcondition += " changed" },
		"data":          func(p *Proposal) { p.TestData = "coupon=MISSING" },
		"data case":     func(p *Proposal) { p.TestData = "coupon=expired" },
		"data spacing":  func(p *Proposal) { p.TestData += " " },
		"step action":   func(p *Proposal) { p.Steps[0].Action = "Submit another code" },
		"step expected": func(p *Proposal) { p.Steps[0].ExpectedResult = "Other rejection" },
		"ordered steps": func(p *Proposal) { p.Steps[0], p.Steps[1] = p.Steps[1], p.Steps[0] },
		"source":        func(p *Proposal) { p.RequirementIDs = []int64{2} },
		"assumptions":   func(p *Proposal) { p.Assumptions = []string{"Different assumption"} },
		"model":         func(p *Proposal) { p.Generation.Model = "v2" },
		"prompt":        func(p *Proposal) { p.Generation.PromptHash = "b" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			candidate.Steps = append([]Step(nil), base.Steps...)
			provenance := *base.Generation
			candidate.Generation = &provenance
			change(&candidate)
			if index, _ := findDuplicate([]generatedCase{{Proposal: base}}, candidate); index >= 0 {
				t.Fatal("distinct scenario/source/provenance was suppressed")
			}
			if got := dedupeProposals([]Proposal{base, candidate, base}); len(got) != 2 {
				t.Fatalf("deduped %d scenarios, want 2", len(got))
			}
		})
	}
	copy := base
	provenance := *base.Generation
	provenance.WorkflowJobID, provenance.WorkflowInputHash = 99, "new job"
	copy.Generation = &provenance
	if index, kind := findDuplicate([]generatedCase{{Proposal: base}}, copy); index != 0 || kind != "EXACT" {
		t.Fatal("new attempt bookkeeping alone must not create a distinct scenario")
	}
}

type pinnedRequirementReader struct {
	items     map[int64]requirement.Detail
	requested []int64
	listCalls int
}

func (r *pinnedRequirementReader) List(context.Context, requirement.Filter) ([]requirement.Requirement, error) {
	r.listCalls++
	return nil, errors.New("pinned generation must not list the live baseline")
}
func (r *pinnedRequirementReader) Get(_ context.Context, id int64) (requirement.Detail, error) {
	r.requested = append(r.requested, id)
	item, ok := r.items[id]
	if !ok {
		return requirement.Detail{}, requirement.ErrNotFound
	}
	return item, nil
}

func TestUV08LoadExactGenerationBaseline(t *testing.T) {
	valid := requirement.Detail{Requirement: requirement.Requirement{ID: 1, DocumentSetID: 7, Status: requirement.StatusApproved}, Evidence: []requirement.Evidence{{ID: 1}}}
	reader := &pinnedRequirementReader{items: map[int64]requirement.Detail{1: valid, 2: valid}}
	service := NewService(nil, reader)
	items, err := service.loadGenerationBaseline(context.Background(), 7, []int64{1})
	if err != nil || len(items) != 1 || len(reader.requested) != 1 || reader.requested[0] != 1 || reader.listCalls != 0 {
		t.Fatalf("pinned baseline=%+v reader=%+v err=%v", items, reader, err)
	}
	for name, mutate := range map[string]func(*requirement.Detail){
		"foreign set":      func(d *requirement.Detail) { d.Requirement.DocumentSetID++ },
		"draft":            func(d *requirement.Detail) { d.Requirement.Status = requirement.StatusDraft },
		"source blocker":   func(d *requirement.Detail) { d.Requirement.ReviewBlockers = []string{"STALE"} },
		"missing evidence": func(d *requirement.Detail) { d.Evidence = nil },
	} {
		t.Run(name, func(t *testing.T) {
			detail := valid
			mutate(&detail)
			reader.items[1] = detail
			// Nil repository proves validation fails before any testcase write.
			if _, err := service.GeneratePinned(context.Background(), 7, GenerationBaseline{RequirementIDs: []int64{1}}); err == nil {
				t.Fatal("accepted invalid baseline")
			}
		})
	}
	reader.items[1] = valid
	for _, ids := range [][]int64{nil, {0}, {1, 1}, {99}} {
		if _, err := service.GeneratePinned(context.Background(), 7, GenerationBaseline{RequirementIDs: ids}); err == nil {
			t.Fatalf("accepted invalid IDs: %v", ids)
		}
	}
}
