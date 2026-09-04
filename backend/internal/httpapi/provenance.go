package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/provenance"
)

type ProvenanceService interface {
	GetSummary(ctx context.Context, analysisID int64) (provenance.SummaryBundle, error)
	GetBundle(ctx context.Context, analysisID int64) (provenance.Bundle, error)
}

type provenanceHandler struct{ service ProvenanceService }

func (h provenanceHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := analysisID(w, r)
	if !ok {
		return
	}
	bundle, err := h.service.GetSummary(r.Context(), id)
	if err != nil {
		writeProvenanceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"evidence": bundle})
}

func (h provenanceHandler) export(w http.ResponseWriter, r *http.Request) {
	id, ok := analysisID(w, r)
	if !ok {
		return
	}
	bundle, err := h.service.GetBundle(r.Context(), id)
	if err != nil {
		writeProvenanceError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`attachment; filename="analysis-%d-evidence.json"`, id))
	writeJSON(w, http.StatusOK, bundle)
}

func writeProvenanceError(w http.ResponseWriter, err error) {
	if errors.Is(err, job.ErrNotFound) {
		writeError(w, http.StatusNotFound, "analysis not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "could not load AI evidence")
}
