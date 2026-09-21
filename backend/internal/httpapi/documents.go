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

type DocumentMetricsService interface {
	Metrics(context.Context) (document.PipelineMetrics, error)
}
type DocumentLifecycleService interface {
	UpdateLifecycle(context.Context, int64, document.LifecycleInput) (document.Set, error)
}
type DocumentPurgeService interface {
	PurgePreview(context.Context, int64) (document.PurgePreview, error)
	Purge(context.Context, int64, document.PurgeInput) (document.PurgeResult, error)
}
type DocumentAIBudgetService interface {
	AIBudgetStatus(context.Context, int64) (document.AIBudgetStatus, error)
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

func (h documentHandler) metrics(w http.ResponseWriter, r *http.Request) {
	service, ok := h.service.(DocumentMetricsService)
	if !ok {
		writeError(w, http.StatusNotImplemented, "document metrics are unavailable")
		return
	}
	result, err := service.Metrics(r.Context())
	if err != nil {
		writeDocumentError(w, err, "could not load document pipeline metrics")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h documentHandler) lifecycle(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	service, ok := h.service.(DocumentLifecycleService)
	if !ok {
		writeError(w, http.StatusNotImplemented, "document lifecycle policy is unavailable")
		return
	}
	var input document.LifecycleInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := service.UpdateLifecycle(r.Context(), id, input)
	if err != nil {
		writeDocumentError(w, err, "could not update document lifecycle")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h documentHandler) purgePreview(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	service, ok := h.service.(DocumentPurgeService)
	if !ok {
		writeError(w, http.StatusNotImplemented, "document purge policy is unavailable")
		return
	}
	result, err := service.PurgePreview(r.Context(), id)
	if err != nil {
		writeDocumentError(w, err, "could not inspect document purge")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h documentHandler) purge(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	service, ok := h.service.(DocumentPurgeService)
	if !ok {
		writeError(w, http.StatusNotImplemented, "document purge policy is unavailable")
		return
	}
	var input document.PurgeInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := service.Purge(r.Context(), id, input)
	if err != nil {
		writeDocumentError(w, err, "could not purge document set")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h documentHandler) aiBudget(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	service, ok := h.service.(DocumentAIBudgetService)
	if !ok {
		writeError(w, http.StatusNotImplemented, "document AI budget is unavailable")
		return
	}
	result, err := service.AIBudgetStatus(r.Context(), id)
	if err != nil {
		writeDocumentError(w, err, "could not load document AI budget")
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	input.NewDocument = r.FormValue("new_document") == "true"
	if r.PathValue("documentID") != "" {
		var valid bool
		input.DocumentID, valid = positiveInt64Path(w, r, "documentID", "document")
		if !valid {
			return
		}
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

func (h documentHandler) listVersions(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	documentID, ok := positiveInt64Path(w, r, "documentID", "document")
	if !ok {
		return
	}
	service, ok := h.service.(interface {
		ListVersions(context.Context, int64, int64) ([]document.Version, error)
	})
	if !ok {
		writeError(w, http.StatusNotImplemented, "version history unavailable")
		return
	}
	versions, err := service.ListVersions(r.Context(), setID, documentID)
	if err != nil {
		writeDocumentError(w, err, "could not list versions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
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
	case errors.Is(err, document.ErrRetentionNotMet), errors.Is(err, document.ErrPurgeBlocked):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, document.ErrInvalidInput), errors.Is(err, document.ErrUnsupported),
		errors.Is(err, document.ErrUnsafeDocument):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}
