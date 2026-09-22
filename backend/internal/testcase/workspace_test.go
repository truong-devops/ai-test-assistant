package testcase

import (
	"errors"
	"strings"
	"testing"
)

func TestEditorSizeLimitDoesNotInvalidateHistoricalContent(t *testing.T) {
	content := RevisionContent{Title: "Historical scenario", TestType: "HAPPY", Risk: "LOW",
		ExpectedResult: strings.Repeat("x", 16001), RequirementRevisionIDs: []int64{1}}
	var field *ValidationError
	if err := validateRevisionSize(content); !errors.As(err, &field) || field.Field != "expected_result" {
		t.Fatalf("expected editor field error, got %v", err)
	}
	if _, err := normalizeRevisionContent(content); err != nil {
		t.Fatalf("historical normalization must remain compatible: %v", err)
	}
	content.ExpectedResult = "Grounded result"
	if err := validateRevisionSize(content); err != nil {
		t.Fatal(err)
	}
	content.Steps = make([]StepInput, 201)
	if err := validateRevisionSize(content); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected bounded new steps, got %v", err)
	}
}
