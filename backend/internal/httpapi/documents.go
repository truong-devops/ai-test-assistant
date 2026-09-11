package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
)

type DocumentService interface {
	CreateSet(context.Context, document.CreateSetInput) (document.Set, error)
	ListSets(context.Context) ([]document.Set, error)
	GetSet(context.Context, int64) (document.Set, error)
	Upload(context.Context, int64, document.UploadInput, io.Reader) (document.Document, document.Version, error)
	ListDocuments(context.Context, int64) ([]document.Document, error)
	GetVersion(context.Context, int64, int) (document.Document, document.Version, []document.Block, error)
}

type documentHandler struct {
	service        DocumentService
	maxUploadBytes int64
}

func (h documentHandler) createSet(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input document.CreateSetInput
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return
	}
	result, err := h.service.CreateSet(r.Context(), input)
	if err != nil {
		writeDocumentError(w, err, "could not create document set")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h documentHandler) listSets(w http.ResponseWriter, r *http.Request) {
	results, err := h.service.ListSets(r.Context())
	if err != nil {
		writeDocumentError(w, err, "could not list document sets")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document_sets": results})
}

func (h documentHandler) getSet(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	result, err := h.service.GetSet(r.Context(), id)
	if err != nil {
		writeDocumentError(w, err, "could not get document set")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h documentHandler) upload(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	defer r.Body.Close()
	// The allowance covers multipart boundaries and small text fields; the file
	// store independently enforces the exact file-size limit.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "document upload is too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid multipart request")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "document file is required")
		return
	}
	defer file.Close()
	input := document.UploadInput{
		DocumentName:   r.FormValue("document_name"),
		DocumentType:   r.FormValue("document_type"),
		ApprovalStatus: r.FormValue("approval_status"),
		Filename:       header.Filename,
	}
	item, version, err := h.service.Upload(r.Context(), setID, input, file)
	if err != nil {
		writeDocumentError(w, err, "could not upload document")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"document": item, "version": version})
}

func (h documentHandler) listDocuments(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	results, err := h.service.ListDocuments(r.Context(), setID)
	if err != nil {
		writeDocumentError(w, err, "could not list documents")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": results})
}

func (h documentHandler) getVersion(w http.ResponseWriter, r *http.Request) {
	documentID, ok := positiveInt64Path(w, r, "id", "document")
	if !ok {
		return
	}
	versionNumber, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || versionNumber <= 0 {
		writeError(w, http.StatusBadRequest, "invalid document version")
		return
	}
	item, version, blocks, err := h.service.GetVersion(r.Context(), documentID, versionNumber)
	if err != nil {
		writeDocumentError(w, err, "could not get document version")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"document": item, "version": version, "blocks": blocks,
	})
}

func positiveInt64Path(w http.ResponseWriter, r *http.Request, key, resource string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+resource+" id")
		return 0, false
	}
	return id, true
}

func writeDocumentError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, document.ErrNotFound):
		writeError(w, http.StatusNotFound, "document resource not found")
	case errors.Is(err, document.ErrFileTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, document.ErrAlreadyExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, document.ErrInvalidInput), errors.Is(err, document.ErrUnsupported),
		errors.Is(err, document.ErrUnsafeDocument):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}
