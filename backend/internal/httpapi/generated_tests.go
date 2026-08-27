package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type GenerationService interface {
	List(ctx context.Context, analysisID int64) ([]generation.GeneratedTest, error)
}

type generationHandler struct{ service GenerationService }

func (h generationHandler) list(w http.ResponseWriter, r *http.Request) {
	id, ok := analysisID(w, r)
	if !ok {
		return
	}
	results, err := h.service.List(r.Context(), id)
	if err != nil {
		if errors.Is(err, job.ErrNotFound) {
			writeError(w, http.StatusNotFound, "analysis not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not list generated tests")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"generated_tests": results})
}
