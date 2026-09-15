package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

type requirementWorkflowStub struct {
	job         requirement.ExtractionJob
	requestedBy string
}

func (s *requirementWorkflowStub) RequestExtraction(_ context.Context, _ int64, requestedBy string) (requirement.ExtractionJob, error) {
	s.requestedBy = requestedBy
	return s.job, nil
}
func (s *requirementWorkflowStub) ExtractionStatus(context.Context, int64) (requirement.ExtractionJob, error) {
	return s.job, nil
}
func (s *requirementWorkflowStub) List(context.Context, requirement.Filter) ([]requirement.Requirement, error) {
	return nil, nil
}
func (s *requirementWorkflowStub) Get(context.Context, int64) (requirement.Detail, error) {
	return requirement.Detail{}, nil
}
func (s *requirementWorkflowStub) Review(context.Context, int64, requirement.ReviewInput) (requirement.Detail, error) {
	return requirement.Detail{}, nil
}
func (s *requirementWorkflowStub) ListConflicts(context.Context, int64) ([]requirement.Conflict, error) {
	return nil, nil
}
func (s *requirementWorkflowStub) ListOpenQuestions(context.Context, int64) ([]requirement.OpenQuestion, error) {
	return nil, nil
}

func TestRequirementExtractionReturnsAcceptedJob(t *testing.T) {
	service := &requirementWorkflowStub{job: requirement.ExtractionJob{ID: 7, Status: requirement.ExtractionPending}}
	request := httptest.NewRequest(http.MethodPost, "/api/document-sets/1/requirements/extract",
		strings.NewReader(`{"requested_by":"BA"}`))
	request.SetPathValue("id", "1")
	response := httptest.NewRecorder()
	requirementWorkflowHandler{service: service}.extract(response, request)
	if response.Code != http.StatusAccepted || service.requestedBy != "BA" ||
		!strings.Contains(response.Body.String(), `"status_url"`) {
		t.Fatalf("status=%d requestedBy=%q body=%s", response.Code, service.requestedBy, response.Body.String())
	}
}
