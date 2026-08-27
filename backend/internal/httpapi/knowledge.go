package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type KnowledgeService interface {
	RequestIndex(ctx context.Context, projectID int64) (knowledge.IndexJob, error)
	GetIndexStatus(ctx context.Context, projectID int64) (knowledge.IndexJob, error)
}

type knowledgeHandler struct{ service KnowledgeService }

func (h knowledgeHandler) requestIndex(w http.ResponseWriter, r *http.Request) {
	projectID, ok := indexProjectID(w, r)
	if !ok {
		return
	}
	result, err := h.service.RequestIndex(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not request project index")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"index": result})
}

func (h knowledgeHandler) status(w http.ResponseWriter, r *http.Request) {
	projectID, ok := indexProjectID(w, r)
	if !ok {
		return
	}
	result, err := h.service.GetIndexStatus(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not get project index status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"index": result})
}

func indexProjectID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	projectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || projectID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return 0, false
	}
	return projectID, true
}
