package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type workspaceTestService struct {
	TestCaseWorkflowService
	previews, reads int
	actor           string
}

func (s *workspaceTestService) History(context.Context, int64, int, int) (testcase.HistoryPage, error) {
	s.reads++
	return testcase.HistoryPage{Entries: []testcase.HistoryEntry{}}, nil
}
func (s *workspaceTestService) Comparison(context.Context, int64, int64, int64, int, int) (testcase.DiffPage, error) {
	s.reads++
	return testcase.DiffPage{}, nil
}
func (s *workspaceTestService) RevisionRuns(context.Context, int64, bool, int64) (testcase.RunPage, error) {
	s.reads++
	return testcase.RunPage{Runs: []testcase.RevisionRun{}}, nil
}
func (s *workspaceTestService) PreviewRelease(_ context.Context, _ int64, _ testcase.PublishReleaseInput, actor string) (testcase.SuiteRelease, error) {
	s.previews++
	s.actor = actor
	return testcase.SuiteRelease{PreviewHash: "reviewed-scope"}, nil
}
func TestUV07WorkspaceReadBoundsAndPreviewRole(t *testing.T) {
	for _, tc := range []struct {
		method, path, role      string
		status, reads, previews int
	}{
		{"GET", "/api/test-case-families/1/history?limit=1", "viewer", 200, 1, 0},
		{"GET", "/api/test-case-families/1/history?limit=51", "viewer", 400, 0, 0},
		{"GET", "/api/test-case-families/1/history?before=-1", "viewer", 400, 0, 0},
		{"GET", "/api/test-case-families/1/comparison?from=1&to=2&offset=0", "viewer", 200, 1, 0},
		{"GET", "/api/test-case-families/1/comparison?from=abc&to=2", "viewer", 400, 0, 0},
		{"GET", "/api/test-cases/1/runs?scope=family", "viewer", 200, 1, 0},
		{"GET", "/api/test-cases/1/history", "viewer", 404, 0, 0},
		{"GET", "/api/test-case-families/1/runs", "viewer", 404, 0, 0},
		{"GET", "/api/test-cases/1/runs?scope=all-users", "viewer", 400, 0, 0},
		{"POST", "/api/document-sets/1/suite-releases/preview", "viewer", 403, 0, 0},
		{"POST", "/api/document-sets/1/suite-releases/preview", "reviewer", 200, 0, 1},
		{"GET", "/api/document-sets/1/suite-releases/preview", "reviewer", 405, 0, 0},
	} {
		t.Run(tc.method+tc.path+tc.role, func(t *testing.T) {
			svc := &workspaceTestService{}
			h := testCaseWorkflowHandler{service: svc}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/test-case-families/{id}/{action}", h.workspace)
			mux.HandleFunc("GET /api/test-cases/{id}/{action}", h.workspace)
			mux.HandleFunc("POST /api/document-sets/{id}/suite-releases/preview", h.previewRelease)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("X-Authenticated-Role", tc.role)
			req.Header.Set("X-Authenticated-Actor", "trusted-qa")
			w := httptest.NewRecorder()
			authorizationMiddleware(RouterOptions{AuthToken: "test-token"}, mux).ServeHTTP(w, req)
			if w.Code != tc.status || svc.reads != tc.reads || svc.previews != tc.previews {
				t.Fatalf("status=%d reads=%d previews=%d body=%s", w.Code, svc.reads, svc.previews, w.Body.String())
			}
			if svc.previews > 0 && svc.actor != "trusted-qa" {
				t.Fatal("untrusted actor")
			}
		})
	}
}
func TestUV07StructuredValidationAndPreviewConflict(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{&testcase.ValidationError{Field: "expected_result", Message: "Review the requirement first"}, 422, "TESTCASE_FIELD_INVALID"},
		{testcase.ErrPreviewChanged, 409, "RELEASE_PREVIEW_CHANGED"},
	} {
		w := httptest.NewRecorder()
		writeWorkflowError(w, tc.err, "fallback")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}
