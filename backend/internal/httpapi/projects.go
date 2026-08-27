package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type projectHandler struct {
	service *project.Service
}

func (h projectHandler) create(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input project.CreateInput
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return
	}

	result, err := h.service.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, project.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, project.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "project already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create project")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h projectHandler) list(w http.ResponseWriter, r *http.Request) {
	results, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list projects")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": results})
}

func (h projectHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	result, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not get project")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
