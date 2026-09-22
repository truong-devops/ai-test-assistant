package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

type requirementReviewStub struct {
	requirementWorkflowStub
	writes int
	actor  string
}

func (s *requirementReviewStub) BulkReview(_ context.Context, _ int64, _ requirement.BulkReviewInput, _, actor string) ([]requirement.ReviewResult, error) {
	s.writes++
	s.actor = actor
	return []requirement.ReviewResult{{ID: 1, Status: "APPLIED"}}, nil
}
func (s *requirementReviewStub) ResolveClarification(context.Context, int64, requirement.ClarificationInput, string) error {
	s.writes++
	return nil
}
func (s *requirementReviewStub) ClarificationHistory(context.Context, int64) ([]requirement.ClarificationAudit, error) {
	return []requirement.ClarificationAudit{}, nil
}
func (s *requirementReviewStub) SourceComparisons(context.Context, int64) ([]requirement.SourceComparison, error) {
	return []requirement.SourceComparison{}, nil
}
func TestRequirementScopeRoutesAreRoleGuardedAndGETNeverWrites(t *testing.T) {
	for _, tc := range []struct {
		method, action, role string
		status, writes       int
	}{
		{"POST", "bulk-review", "editor", 403, 0}, {"POST", "clarification", "viewer", 403, 0},
		{"POST", "bulk-review", "reviewer", 200, 1}, {"POST", "clarification", "reviewer", 200, 1},
		{"GET", "bulk-review", "reviewer", 404, 0}, {"GET", "clarification", "reviewer", 404, 0},
		{"GET", "source-comparisons", "viewer", 200, 0}, {"GET", "clarification-history", "viewer", 200, 0},
	} {
		t.Run(tc.method+tc.action+tc.role, func(t *testing.T) {
			service := &requirementReviewStub{}
			handler := requirementWorkflowHandler{service: service}
			mux := http.NewServeMux()
			mux.HandleFunc("/api/document-sets/{id}/requirement-review/{action}", handler.reviewScope)
			request := httptest.NewRequest(tc.method, "/api/document-sets/1/requirement-review/"+tc.action, strings.NewReader(`{}`))
			request.Header.Set("Authorization", "Bearer test-token")
			request.Header.Set("X-Authenticated-Role", tc.role)
			request.Header.Set("X-Authenticated-Actor", "trusted-actor")
			response := httptest.NewRecorder()
			authorizationMiddleware(RouterOptions{AuthToken: "test-token"}, mux).ServeHTTP(response, request)
			if response.Code != tc.status || service.writes != tc.writes {
				t.Fatalf("response=%d writes=%d body=%s", response.Code, service.writes, response.Body.String())
			}
			if tc.action == "bulk-review" && service.writes > 0 && service.actor != "trusted-actor" {
				t.Fatalf("actor=%s", service.actor)
			}
		})
	}
}
