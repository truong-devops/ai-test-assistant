package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/impact"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type ImpactService interface {
	Get(ctx context.Context, analysisID int64) (impact.Bundle, error)
}

type impactHandler struct{ service ImpactService }

func (h impactHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := analysisID(w, r)
	if !ok {
		return
	}
	bundle, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, job.ErrNotFound) || errors.Is(err, impact.ErrNotFound) {
			writeError(w, http.StatusNotFound, "impact analysis not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load change impact")
		return
	}
	// Preserve the API contract for analyses without impacted Go symbols. A nil
	// slice is encoded as null and can break clients that correctly expect a
	// collection from this endpoint.
	if bundle.Nodes == nil {
		bundle.Nodes = []impact.Node{}
	}
	if bundle.Edges == nil {
		bundle.Edges = []impact.Edge{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"impact": bundle})
}
