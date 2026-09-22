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
