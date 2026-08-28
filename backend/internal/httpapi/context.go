package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

type AnalysisContextService interface {
	List(ctx context.Context, analysisID int64) ([]knowledge.KnowledgeChunk, error)
}

type analysisContextHandler struct{ service AnalysisContextService }

func (h analysisContextHandler) list(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusInternalServerError, "could not retrieve analysis context")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"context": results})
}
