package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/automation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/execution"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/report"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scope"
)

type ReportService interface {
	Export(context.Context, int64, report.ExportInput) (report.ExportArtifact, error)
	List(context.Context, int64) ([]report.ExportArtifact, error)
	Download(context.Context, int64) (report.ExportArtifact, error)
}
type ScopeService interface {
	BaselineView(context.Context, int64) (scope.BaselineView, error)
	Select(context.Context, int64, scope.SelectInput) (scope.Baseline, error)
	Get(context.Context, int64) (scope.Bundle, error)
	Decide(context.Context, int64, scope.ManualInput) (scope.Bundle, error)
}
type AutomationService interface {
	Generate(context.Context, int64, automation.GenerateInput) (automation.GenerationResult, error)
	History(context.Context, int64) (automation.ArtifactHistory, error)
	Review(context.Context, int64, automation.ReviewInput) (automation.ArtifactHistory, error)
}
type ExecutionService interface {
	Get(context.Context, int64) (execution.Run, error)
	Request(context.Context, int64, execution.RequestInput) (execution.Run, error)
	ReviewClassification(context.Context, int64, execution.ClassificationInput) (execution.Run, error)
}

type reportHandler struct{ service ReportService }

func (h reportHandler) export(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	var input report.ExportInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Export(r.Context(), id, input)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not export test cases")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (h reportHandler) list(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	result, err := h.service.List(r.Context(), id)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not list exports")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"exports": result})
}
func (h reportHandler) download(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "export")
	if !ok {
		return
	}
	result, err := h.service.Download(r.Context(), id)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not download export")
		return
	}
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.Filename))
	w.Header().Set("X-Content-SHA256", result.ContentHash)
	w.Header().Set("Content-Length", strconv.Itoa(len(result.Content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Content)
}

type scopeHandler struct{ service ScopeService }

func (h scopeHandler) baseline(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "project")
	if !ok {
		return
	}
	result, err := h.service.BaselineView(r.Context(), id)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not get project baseline")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h scopeHandler) selectBaseline(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "project")
	if !ok {
		return
	}
	var input scope.SelectInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Select(r.Context(), id, input)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not select project baseline")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h scopeHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "analysis")
	if !ok {
		return
	}
	result, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not get analysis test scope")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h scopeHandler) decide(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "analysis")
	if !ok {
		return
	}
	var input scope.ManualInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Decide(r.Context(), id, input)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not update analysis test scope")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type automationHandler struct{ service AutomationService }

func (h automationHandler) generate(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "analysis")
	if !ok {
		return
	}
	var input automation.GenerateInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Generate(r.Context(), id, input)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not generate automation")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h automationHandler) history(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "test case")
	if !ok {
		return
	}
	result, err := h.service.History(r.Context(), id)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not get automation history")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h automationHandler) review(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "automation artifact")
	if !ok {
		return
	}
	var input automation.ReviewInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Review(r.Context(), id, input)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not review automation artifact")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type executionHandler struct{ service ExecutionService }

func (h executionHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "test run")
	if !ok {
		return
	}
	result, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not get test run")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h executionHandler) request(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "test run")
	if !ok {
		return
	}
	var input execution.RequestInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Request(r.Context(), id, input)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not request test run")
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h executionHandler) classify(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "test run item")
	if !ok {
		return
	}
	var input execution.ClassificationInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.ReviewClassification(r.Context(), id, input)
	if err != nil {
		writeDocumentExecutionError(w, err, "could not review result classification")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeDocumentExecutionError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, report.ErrNotFound), errors.Is(err, scope.ErrNotFound), errors.Is(err, automation.ErrNotFound), errors.Is(err, execution.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, report.ErrInvalidInput), errors.Is(err, scope.ErrInvalidInput), errors.Is(err, automation.ErrInvalidInput), errors.Is(err, automation.ErrInvalidOutput), errors.Is(err, execution.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, scope.ErrUnsafeChange), errors.Is(err, automation.ErrBlocked), errors.Is(err, automation.ErrAlreadyReviewed), errors.Is(err, execution.ErrNotRequestable):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}
