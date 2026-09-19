package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workflowjob "github.com/maccuatruong/ai-test-assistant/backend/internal/workflow"
)

type documentWorkflowJobServiceStub struct {
	job     workflowjob.Job
	created bool
	err     error
}

func (s documentWorkflowJobServiceStub) Read(context.Context, int64, string) (workflowjob.ReadModel, error) {
	return workflowjob.ReadModel{}, s.err
}
func (s documentWorkflowJobServiceStub) Enqueue(context.Context, int64,
	workflowjob.OperationInput, string, string,
) (workflowjob.Job, bool, error) {
	return s.job, s.created, s.err
}
func (s documentWorkflowJobServiceStub) Get(context.Context, int64) (workflowjob.Job, error) {
	return s.job, s.err
}
func (s documentWorkflowJobServiceStub) Retry(context.Context, int64, int, string) (workflowjob.Job, error) {
	return s.job, s.err
}
func (s documentWorkflowJobServiceStub) Cancel(context.Context, int64, int, string) (workflowjob.Job, error) {
	return s.job, s.err
}

func TestWorkflowOperationReturnsAcceptedStatusURL(t *testing.T) {
	handler := documentWorkflowJobHandler{service: documentWorkflowJobServiceStub{
		job: workflowjob.Job{ID: 17, Status: workflowjob.StatusQueued}, created: true}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/document-sets/{id}/workflow-operations", handler.enqueue)
	request := httptest.NewRequest(http.MethodPost, "/api/document-sets/9/workflow-operations",
		strings.NewReader(`{"operation":"GENERATE_TESTCASES"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "generate-9")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted ||
		response.Header().Get("Location") != "/api/document-workflow-jobs/17" {
		t.Fatalf("status=%d location=%q body=%s", response.Code,
			response.Header().Get("Location"), response.Body.String())
	}
}

func TestWorkflowOperationReturnsStructuredBlocker(t *testing.T) {
	blocked := &workflowjob.BlockedError{Code: "NO_APPROVED_REQUIREMENTS",
		Message: "approve requirements first", NextAction: "REVIEW_REQUIREMENTS",
		BlockedBy: []workflowjob.BlockingReason{{Code: "NO_APPROVED_REQUIREMENTS",
			Step: "REQUIREMENTS", Message: "approve requirements first"}}}
	handler := documentWorkflowJobHandler{service: documentWorkflowJobServiceStub{err: blocked}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/document-sets/{id}/workflow-operations", handler.enqueue)
	request := httptest.NewRequest(http.MethodPost, "/api/document-sets/9/workflow-operations",
		strings.NewReader(`{"operation":"GENERATE_TESTCASES"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity ||
		body["code"] != "NO_APPROVED_REQUIREMENTS" || body["retryable"] != false ||
		body["next_action"] != "REVIEW_REQUIREMENTS" || body["blocked_by"] == nil {
		t.Fatalf("status=%d body=%v", response.Code, body)
	}
}
