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
	"github.com/maccuatruong/ai-test-assistant/backend/internal/repair"
)

type repairServiceStub struct {
	attempts []repair.Attempt
	err      error
}

func (s repairServiceStub) List(context.Context, int64) ([]repair.Attempt, error) {
	return s.attempts, s.err
}

func TestRepairHandlerListsPersistedAttempts(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/7/repairs", nil)
	request.SetPathValue("id", "7")
	recorder := httptest.NewRecorder()
	repairHandler{service: repairServiceStub{attempts: []repair.Attempt{{
		ID: 1, AnalysisJobID: 7, GeneratedTestID: 8, RepairedGeneratedTestID: 9,
	}}}}.list(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Attempts []repair.Attempt `json:"repair_attempts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil ||
		len(body.Attempts) != 1 || body.Attempts[0].RepairedGeneratedTestID != 9 {
		t.Fatalf("body=%s error=%v", recorder.Body.String(), err)
	}
}

func TestRepairHandlerErrors(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/404/repairs", nil)
	request.SetPathValue("id", "404")
	for _, test := range []struct {
		err    error
		status int
	}{{job.ErrNotFound, http.StatusNotFound}, {errors.New("database"), http.StatusInternalServerError}} {
		recorder := httptest.NewRecorder()
		repairHandler{service: repairServiceStub{err: test.err}}.list(recorder, request)
		if recorder.Code != test.status {
			t.Fatalf("error=%v status=%d body=%s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestPhaseEightRouterRegistersRepairEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithPhaseEightServices(logger, nil, nil, nil, nil, nil, nil, nil, nil,
		repairServiceStub{attempts: []repair.Attempt{{ID: 1, AnalysisJobID: 7}}})
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/7/repairs", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "repair_attempts") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
