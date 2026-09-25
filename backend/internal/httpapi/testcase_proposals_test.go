package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type proposalServiceStub struct {
	TestCaseWorkflowService
	reads, writes int
	actor, key    string
}

func (s *proposalServiceStub) ListProposals(context.Context, int64, int64, int64, int) (testcase.ProposalPage, error) {
	s.reads++
	return testcase.ProposalPage{Proposals: []testcase.GenerationProposal{}}, nil
}
func (s *proposalServiceStub) CompareProposal(context.Context, int64, int64) (testcase.ProposalComparison, error) {
	s.reads++
	return testcase.ProposalComparison{}, nil
}
func (s *proposalServiceStub) DecideProposal(_ context.Context, _ int64, _ testcase.ProposalDecision, key, actor string) (testcase.GenerationProposal, error) {
	s.writes++
	s.actor, s.key = actor, key
	return testcase.GenerationProposal{ID: 1}, nil
}

func TestProposalReadsBoundsAndReviewerOnlyDecision(t *testing.T) {
	for _, tc := range []struct {
		method, path, role    string
		status, reads, writes int
	}{
		{"GET", "/api/document-sets/1/test-case-proposals?job_id=2&limit=1", "viewer", 200, 1, 0},
		{"GET", "/api/document-sets/1/test-case-proposals?limit=51", "viewer", 400, 0, 0},
		{"GET", "/api/document-sets/1/test-case-proposals?before=-1", "viewer", 400, 0, 0},
		{"GET", "/api/test-case-proposals/1/comparison?target_family_id=2", "viewer", 200, 1, 0},
		{"GET", "/api/test-case-proposals/1/comparison?target_family_id=-2", "viewer", 400, 0, 0},
		{"GET", "/api/test-case-proposals/1/comparison", "viewer", 400, 0, 0},
		{"POST", "/api/test-case-proposals/1/review", "viewer", 403, 0, 0},
		{"POST", "/api/test-case-proposals/1/review", "editor", 403, 0, 0},
		{"POST", "/api/test-case-proposals/1/review", "reviewer", 200, 0, 1},
		{"GET", "/api/test-case-proposals/1/review", "reviewer", 405, 0, 0},
	} {
		t.Run(tc.method+tc.path+tc.role, func(t *testing.T) {
			svc := &proposalServiceStub{}
			h := testCaseWorkflowHandler{service: svc}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/document-sets/{id}/test-case-proposals", h.proposals)
			mux.HandleFunc("POST /api/test-case-proposals/{id}/review", h.decideProposal)
			mux.HandleFunc("GET /api/test-case-proposals/{id}/comparison", h.compareProposal)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"decision":"CREATE_NEW","reason":"Reviewed evidence"}`))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("X-Authenticated-Role", tc.role)
			req.Header.Set("X-Authenticated-Actor", "trusted-qa")
			req.Header.Set("Idempotency-Key", "proposal-review")
			w := httptest.NewRecorder()
			authorizationMiddleware(RouterOptions{AuthToken: "test-token"}, mux).ServeHTTP(w, req)
			if w.Code != tc.status || svc.reads != tc.reads || svc.writes != tc.writes {
				t.Fatalf("%d %s reads=%d writes=%d", w.Code, w.Body.String(), svc.reads, svc.writes)
			}
			if svc.writes > 0 && (svc.actor != "trusted-qa" || svc.key != "proposal-review") {
				t.Fatal("lost trusted actor/idempotency")
			}
		})
	}
}
