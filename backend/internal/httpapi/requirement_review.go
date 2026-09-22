package httpapi

import (
	"context"
	"net/http"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

type requirementReviewService interface {
	BulkReview(context.Context, int64, requirement.BulkReviewInput, string, string) ([]requirement.ReviewResult, error)
	ResolveClarification(context.Context, int64, requirement.ClarificationInput, string) error
	ClarificationHistory(context.Context, int64) ([]requirement.ClarificationAudit, error)
	SourceComparisons(context.Context, int64) ([]requirement.SourceComparison, error)
}

func (h requirementWorkflowHandler) reviewScope(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	service, ok := h.service.(requirementReviewService)
	if !ok {
		writeError(w, http.StatusNotImplemented, "requirement review unavailable")
		return
	}
	switch r.Method + ":" + r.PathValue("action") {
	case "POST:bulk-review":
		var input requirement.BulkReviewInput
		if !decodeWorkflowJSON(w, r, &input) {
			return
		}
		result, err := service.BulkReview(r.Context(), setID, input, r.Header.Get("Idempotency-Key"), workflowActor(r))
		if err != nil {
			writeWorkflowError(w, err, "could not review requirements")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": result})
	case "POST:clarification":
		var input requirement.ClarificationInput
		if !decodeWorkflowJSON(w, r, &input) {
			return
		}
		if err := service.ResolveClarification(r.Context(), setID, input, workflowActor(r)); err != nil {
			writeWorkflowError(w, err, "could not resolve clarification")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "RESOLVED"})
	case "GET:clarification-history":
		result, err := service.ClarificationHistory(r.Context(), setID)
		if err != nil {
			writeWorkflowError(w, err, "could not load clarification history")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"history": result})
	case "GET:source-comparisons":
		result, err := service.SourceComparisons(r.Context(), setID)
		if err != nil {
			writeWorkflowError(w, err, "could not load source comparison")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"comparisons": result})
	default:
		writeError(w, http.StatusNotFound, "unknown requirement action")
	}
}
