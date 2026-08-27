package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type AnalysisService interface {
	List(ctx context.Context) ([]job.AnalysisJob, error)
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
	GetSymbols(ctx context.Context, id int64) ([]job.ChangedSymbol, error)
}

type analysisHandler struct{ service AnalysisService }

func (h analysisHandler) list(w http.ResponseWriter, r *http.Request) {
	results, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list analyses")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"analyses": results})
}

func (h analysisHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := analysisID(w, r)
	if !ok {
		return
	}
	result, files, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, job.ErrNotFound) {
			writeError(w, http.StatusNotFound, "analysis not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not get analysis")
		return
	}
	symbols, err := h.service.GetSymbols(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not get changed symbols")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"analysis": result, "changed_files": files, "changed_symbols": symbols,
	})
}

func (h analysisHandler) changes(w http.ResponseWriter, r *http.Request) {
	id, ok := analysisID(w, r)
	if !ok {
		return
	}
	_, files, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, job.ErrNotFound) {
			writeError(w, http.StatusNotFound, "analysis not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not get analysis changes")
		return
	}
	symbols, err := h.service.GetSymbols(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not get changed symbols")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed_files": files, "changed_symbols": symbols})
}

func analysisID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid analysis id")
		return 0, false
	}
	return id, true
}
