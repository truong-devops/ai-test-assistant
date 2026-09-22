package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type testcaseWorkspaceService interface {
	History(context.Context, int64, int, int) (testcase.HistoryPage, error)
	Comparison(context.Context, int64, int64, int64, int, int) (testcase.DiffPage, error)
	RevisionRuns(context.Context, int64, bool, int64) (testcase.RunPage, error)
	PreviewRelease(context.Context, int64, testcase.PublishReleaseInput, string) (testcase.SuiteRelease, error)
}

func (h testCaseWorkflowHandler) workspace(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	isFamily := strings.HasPrefix(r.URL.Path, "/api/test-case-families/")
	if (isFamily && action != "history" && action != "comparison") || (!isFamily && action != "runs") {
		writeError(w, 404, "unknown workspace read")
		return
	}
	id, ok := positiveInt64Path(w, r, "id", "testcase scope")
	if !ok {
		return
	}
	svc, ok := h.service.(testcaseWorkspaceService)
	if !ok {
		writeError(w, 501, "workspace unavailable")
		return
	}
	number := func(key string, fallback int) (int, bool) {
		v := r.URL.Query().Get(key)
		if v == "" {
			return fallback, true
		}
		n, e := strconv.Atoi(v)
		return n, e == nil && n >= 0
	}
	limit, ok := number("limit", 20)
	before, ok2 := number("before", 0)
	offset, ok3 := number("offset", 0)
	if !ok || !ok2 || !ok3 || limit < 1 || limit > 50 {
		writeError(w, 400, "invalid pagination (limit 1–50)")
		return
	}
	var result any
	var err error
	switch r.PathValue("action") {
	case "history":
		result, err = svc.History(r.Context(), id, before, limit)
	case "comparison":
		from, e1 := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
		to, e2 := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
		if e1 != nil || e2 != nil {
			writeError(w, 400, "from/to required")
			return
		}
		result, err = svc.Comparison(r.Context(), id, from, to, offset, limit)
	case "runs":
		scope := r.URL.Query().Get("scope")
		if scope != "" && scope != "revision" && scope != "family" {
			writeError(w, 400, "invalid run scope")
			return
		}
		result, err = svc.RevisionRuns(r.Context(), id, scope == "family", int64(before))
	default:
		writeError(w, 404, "unknown workspace read")
		return
	}
	if err != nil {
		writeWorkflowError(w, err, "could not load testcase workspace")
		return
	}
	writeJSON(w, 200, result)
}
func (h testCaseWorkflowHandler) previewRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	svc, ok := h.service.(testcaseWorkspaceService)
	if !ok {
		writeError(w, 501, "workspace unavailable")
		return
	}
	var input testcase.PublishReleaseInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := svc.PreviewRelease(r.Context(), id, input, workflowActor(r))
	if err != nil {
		writeWorkflowError(w, err, "could not preview release")
		return
	}
	writeJSON(w, 200, result)
}
