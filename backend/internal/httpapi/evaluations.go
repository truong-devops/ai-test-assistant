package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/evaluation"
)

type EvaluationService interface {
	List(ctx context.Context) ([]evaluation.Run, error)
	Get(ctx context.Context, id int64) (evaluation.StoredReport, error)
}

type evaluationHandler struct{ service EvaluationService }

func (h evaluationHandler) list(w http.ResponseWriter, r *http.Request) {
	results, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list evaluation runs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"evaluation_runs": results})
}

func (h evaluationHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid evaluation run id")
		return
	}
	result, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, evaluation.ErrNotFound) {
			writeError(w, http.StatusNotFound, "evaluation run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not get evaluation report")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
