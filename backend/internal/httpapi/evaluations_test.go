package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/evaluation"
)

type evaluationServiceStub struct {
	runs   []evaluation.Run
	report evaluation.StoredReport
	err    error
}

func (s evaluationServiceStub) List(context.Context) ([]evaluation.Run, error) { return s.runs, s.err }
func (s evaluationServiceStub) Get(context.Context, int64) (evaluation.StoredReport, error) {
	return s.report, s.err
}

func TestEvaluationHandlerListsAndGetsRuns(t *testing.T) {
	service := evaluationServiceStub{runs: []evaluation.Run{{ID: 4, Name: "trial"}},
		report: evaluation.StoredReport{Run: evaluation.Run{ID: 4}, Report: evaluation.Report{DatasetName: "trial"}}}
	listResponse := httptest.NewRecorder()
	evaluationHandler{service: service}.list(listResponse, httptest.NewRequest(http.MethodGet, "/api/evaluations", nil))
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"evaluation_runs"`) {
		t.Fatalf("status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	getRequest := httptest.NewRequest(http.MethodGet, "/api/evaluations/4", nil)
	getRequest.SetPathValue("id", "4")
	getResponse := httptest.NewRecorder()
	evaluationHandler{service: service}.get(getResponse, getRequest)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"dataset_name":"trial"`) {
		t.Fatalf("status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
}

func TestEvaluationHandlerMapsErrors(t *testing.T) {
	tests := []struct {
		id     string
		err    error
		status int
	}{
		{id: "bad", status: http.StatusBadRequest},
		{id: "2", err: evaluation.ErrNotFound, status: http.StatusNotFound},
		{id: "2", err: errors.New("database"), status: http.StatusInternalServerError},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, "/api/evaluations/"+test.id, nil)
		request.SetPathValue("id", test.id)
		response := httptest.NewRecorder()
		evaluationHandler{service: evaluationServiceStub{err: test.err}}.get(response, request)
		if response.Code != test.status {
			t.Fatalf("id=%s status=%d want=%d body=%s", test.id, response.Code, test.status, response.Body.String())
		}
	}
}
