package workflow

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestUV08GenerationSelectionValidationAndCommandReplay(t *testing.T) {
	for _, ids := range [][]int64{{0}, {-1}, {1, 1}, make([]int64, 101)} {
		if _, err := generationSelection(ids); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("accepted %v", ids)
		}
	}
	ids := []int64{3, 1}
	got, err := generationSelection(ids)
	if err != nil || !reflect.DeepEqual(got, []int64{1, 3}) || ids[0] != 3 {
		t.Fatalf("selection=%v input=%v error=%v", got, ids, err)
	}
	snapshot, _ := json.Marshal(generateInputSnapshot{SelectedRequirementIDs: []int64{1, 3}})
	job := Job{Operation: OperationGenerate, InputSnapshot: snapshot}
	if !sameIdempotentCommand(job, OperationInput{Operation: OperationGenerate, RequirementIDs: []int64{3, 1}}) {
		t.Fatal("reordered selection should replay")
	}
	for _, ids := range [][]int64{nil, {1}, {1, 3, 4}, {1, 1}} {
		if sameIdempotentCommand(job, OperationInput{Operation: OperationGenerate, RequirementIDs: ids}) {
			t.Fatalf("different selection reused key: %v", ids)
		}
	}
	// Existing jobs do not contain selected_requirement_ids and retain all-baseline semantics.
	job.InputSnapshot = json.RawMessage(`{"source_revision":1,"requirements":[{"id":1}]}`)
	if !sameIdempotentCommand(job, OperationInput{Operation: OperationGenerate}) {
		t.Fatal("legacy job replay changed")
	}
}

func TestUV08GenerationScopeReplayAndUnitKeys(t *testing.T) {
	for _, scope := range []string{"ALL", "SELECTED", "AFFECTED"} {
		pin := generateInputSnapshot{Scope: scope, RequestedScope: scope, ReviewProposals: true, PerRequirementUnits: true, Requirements: []generateRequirementSnapshot{{ID: 11}, {ID: 22}}}
		keys := generationKeys(pin)
		want := []string{"requirement:11", "requirement:22"}
		if scope != "SELECTED" {
			want = append(want, "retire")
		}
		if !reflect.DeepEqual(keys, want) {
			t.Fatalf("%s keys=%v", scope, keys)
		}
		payload, _ := json.Marshal(pin)
		job := Job{Operation: OperationGenerate, InputSnapshot: payload}
		for _, requested := range []string{"", "ALL", "SELECTED", "AFFECTED"} {
			matches := sameIdempotentCommand(job, OperationInput{Operation: OperationGenerate, ReviewProposals: true, GenerationScope: requested})
			if matches != (scope == requested) {
				t.Fatalf("scope=%s requested=%s match=%v", scope, requested, matches)
			}
		}
	}
	if keys := generationKeys(generateInputSnapshot{Scope: "AFFECTED"}); !reflect.DeepEqual(keys, []string{"retire"}) {
		t.Fatalf("empty baseline must retain retire checkpoint: %v", keys)
	}
}
