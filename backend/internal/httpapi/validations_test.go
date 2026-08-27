package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

type validationServiceStub struct {
	runs []validation.Run
	err  error
}

func (s validationServiceStub) List(context.Context, int64) ([]validation.Run, error) {
	return s.runs, s.err
}

func TestValidationHandlerListsPersistedRuns(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/7/validations", nil)
	request.SetPathValue("id", "7")
	recorder := httptest.NewRecorder()
	validationHandler{service: validationServiceStub{runs: []validation.Run{{
		ID: 1, AnalysisJobID: 7, GeneratedTestID: 8, Status: validation.StatusPassed,
	}}}}.list(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Runs []validation.Run `json:"validation_runs"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || len(body.Runs) != 1 ||
		body.Runs[0].GeneratedTestID != 8 {
		t.Fatalf("body=%s error=%v", recorder.Body.String(), err)
	}
}

func TestValidationHandlerReturnsNotFound(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/404/validations", nil)
	request.SetPathValue("id", "404")
	recorder := httptest.NewRecorder()
	validationHandler{service: validationServiceStub{err: job.ErrNotFound}}.list(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	validationHandler{service: validationServiceStub{err: errors.New("database")}}.list(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPhaseSevenRouterRegistersValidationEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithPhaseSevenServices(logger, nil, nil, nil, nil, nil, nil, nil,
		validationServiceStub{runs: []validation.Run{{ID: 1, AnalysisJobID: 7}}})
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/7/validations", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "validation_runs") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
