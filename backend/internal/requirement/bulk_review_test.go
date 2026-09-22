package requirement

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBulkReviewRejectsInvalidEnvelopeBeforeStorage(t *testing.T) {
	valid := ReviewSelection{ID: 1, ExpectedHash: strings.Repeat("a", 64)}
	for _, tc := range []struct {
		name  string
		items []ReviewSelection
		key   string
	}{
		{"empty", nil, "key"},
		{"duplicate", []ReviewSelection{valid, valid}, "key"},
		{"oversized", make([]ReviewSelection, 101), "key"},
		{"missing hash", []ReviewSelection{{ID: 1}}, "key"},
		{"negative ID", []ReviewSelection{{ID: -1, ExpectedHash: valid.ExpectedHash}}, "key"},
		{"missing key", []ReviewSelection{valid}, ""},
		{"oversized key", []ReviewSelection{valid}, strings.Repeat("k", 161)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&Service{}).BulkReview(context.Background(), 1, BulkReviewInput{Items: tc.items, Decision: DecisionApproved}, tc.key, "trusted-actor")
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}
