package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
	workflowjob "github.com/maccuatruong/ai-test-assistant/backend/internal/workflow"
)

type DocumentWorkflowJobService interface {
	Read(context.Context, int64, string) (workflowjob.ReadModel, error)
	Enqueue(context.Context, int64, workflowjob.OperationInput, string, string) (workflowjob.Job, bool, error)
	Get(context.Context, int64) (workflowjob.Job, error)
	Retry(context.Context, int64, int, string) (workflowjob.Job, error)
	Cancel(context.Context, int64, int, string) (workflowjob.Job, error)
}

type documentWorkflowJobHandler struct{ service DocumentWorkflowJobService }

func (h documentWorkflowJobHandler) sourceCommand(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	service, ok := h.service.(interface {
		SourceCommand(context.Context, int64, workflowjob.SourceCommand, string, string, string) (workflowjob.SourceIntent, error)
	})
	if !ok {
		writeError(w, http.StatusNotImplemented, "source workflow unavailable")
		return
	}
	var input workflowjob.SourceCommand
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := service.SourceCommand(r.Context(), id, input, r.Header.Get("Idempotency-Key"), authenticatedRole(r), workflowActor(r))
	if err != nil {
		writeWorkflowJobError(w, r, err, "could not review source scope")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"intent": result})
}

func (h documentWorkflowJobHandler) read(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	result, err := h.service.Read(r.Context(), setID, authenticatedRole(r))
	if err != nil {
		writeWorkflowJobError(w, r, err, "could not load document workflow")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h documentWorkflowJobHandler) enqueue(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	var input workflowjob.OperationInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	input.RequestedBy = workflowActor(r)
	result, created, err := h.service.Enqueue(r.Context(), setID, input,
		r.Header.Get("Idempotency-Key"), authenticatedRole(r))
	if err != nil {
		writeWorkflowJobError(w, r, err, "could not enqueue document workflow operation")
		return
	}
	statusURL := fmt.Sprintf("/api/document-workflow-jobs/%d", result.ID)
	w.Header().Set("Location", statusURL)
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, result.Revision))
	w.Header().Set("Retry-After", "1")
	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"job": result, "status_url": statusURL,
		"created": created})
}

func (h documentWorkflowJobHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document workflow job")
	if !ok {
		return
	}
	result, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeWorkflowJobError(w, r, err, "could not load document workflow job")
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, result.Revision))
	if !workflowjob.Terminal(result.Status) {
		w.Header().Set("Retry-After", "1")
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": result})
}

func (h documentWorkflowJobHandler) retry(w http.ResponseWriter, r *http.Request) {
	h.mutate(w, r, true)
}

func (h documentWorkflowJobHandler) cancel(w http.ResponseWriter, r *http.Request) {
	h.mutate(w, r, false)
}

func (h documentWorkflowJobHandler) mutate(w http.ResponseWriter, r *http.Request, retry bool) {
	id, ok := positiveInt64Path(w, r, "id", "document workflow job")
	if !ok {
		return
	}
	var input workflowjob.MutationInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	var result workflowjob.Job
	var err error
	if retry {
		result, err = h.service.Retry(r.Context(), id, input.ExpectedRevision,
			authenticatedRole(r))
	} else {
		result, err = h.service.Cancel(r.Context(), id, input.ExpectedRevision,
			authenticatedRole(r))
	}
	if err != nil {
		writeWorkflowJobError(w, r, err, "could not update document workflow job")
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, result.Revision))
	w.Header().Set("Retry-After", "1")
	writeJSON(w, http.StatusAccepted, map[string]any{"job": result,
		"status_url": fmt.Sprintf("/api/document-workflow-jobs/%d", result.ID)})
}

func authenticatedRole(r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(r.Header.Get("X-Authenticated-Role")))
}

func writeWorkflowJobError(w http.ResponseWriter, r *http.Request, err error,
	fallback string,
) {
	envelope := map[string]any{"error": fallback, "message": fallback,
		"code": "INTERNAL_ERROR", "retryable": false,
		"request_id": w.Header().Get("X-Request-ID")}
	status := http.StatusInternalServerError
	var blocked *workflowjob.BlockedError
	switch {
	case errors.As(err, &blocked):
		status = http.StatusUnprocessableEntity
		envelope["error"], envelope["message"], envelope["code"] = blocked.Message,
			blocked.Message, blocked.Code
		envelope["blocked_by"], envelope["next_action"] = blocked.BlockedBy,
			blocked.NextAction
		if blocked.Details != nil {
			envelope["details"] = blocked.Details
		}
	case errors.Is(err, workflowjob.ErrNotFound):
		status = http.StatusNotFound
		envelope["error"], envelope["message"], envelope["code"] = err.Error(), err.Error(),
			"WORKFLOW_JOB_NOT_FOUND"
	case errors.Is(err, workflowjob.ErrInvalidInput):
		status = http.StatusBadRequest
		envelope["error"], envelope["message"], envelope["code"] = err.Error(), err.Error(),
			"INVALID_WORKFLOW_INPUT"
	case errors.Is(err, workflowjob.ErrForbidden):
		status = http.StatusForbidden
		envelope["error"], envelope["message"], envelope["code"] = err.Error(), err.Error(),
			"WORKFLOW_FORBIDDEN"
	case errors.Is(err, workflowjob.ErrRevisionConflict):
		status = http.StatusConflict
		envelope["error"], envelope["message"], envelope["code"] = err.Error(), err.Error(),
			"WORKFLOW_REVISION_CONFLICT"
	case errors.Is(err, workflowjob.ErrIdempotencyConflict):
		status = http.StatusConflict
		envelope["error"], envelope["message"], envelope["code"] = err.Error(), err.Error(),
			"IDEMPOTENCY_KEY_REUSED"
	case errors.Is(err, workflowjob.ErrOperationActive):
		status = http.StatusConflict
		envelope["error"], envelope["message"], envelope["code"] = err.Error(), err.Error(),
			"WORKFLOW_OPERATION_ACTIVE"
	case errors.Is(err, workflowjob.ErrNotRetryable), errors.Is(err, workflowjob.ErrNotCancelable):
		status = http.StatusConflict
		envelope["error"], envelope["message"] = err.Error(), err.Error()
		envelope["code"] = "WORKFLOW_STATE_CONFLICT"
	case errors.Is(err, workflowjob.ErrInputStale):
		status = http.StatusConflict
		envelope["error"], envelope["message"], envelope["code"] = err.Error(), err.Error(),
			"INPUT_SNAPSHOT_STALE"
	case errors.Is(err, requirement.ErrNoIndex), errors.Is(err, requirement.ErrStaleIndex),
		errors.Is(err, requirement.ErrSourceNotApproved), errors.Is(err, testcase.ErrNoApprovedSource),
		errors.Is(err, document.ErrNoParsedDocuments), errors.Is(err, document.ErrSourceSnapshotBlocked):
		status = http.StatusUnprocessableEntity
		envelope["error"], envelope["message"], envelope["code"] = err.Error(), err.Error(),
			"WORKFLOW_PRECONDITION_FAILED"
	default:
		_ = r
	}
	writeJSON(w, status, envelope)
}
