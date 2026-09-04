package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/provenance"
)

type provenanceServiceStub struct {
	result  provenance.Bundle
	summary provenance.SummaryBundle
	err     error
}

func (s provenanceServiceStub) GetBundle(context.Context, int64) (provenance.Bundle, error) {
	return s.result, s.err
}

func (s provenanceServiceStub) GetSummary(context.Context, int64) (provenance.SummaryBundle, error) {
	return s.summary, s.err
}

func TestProvenanceHandlers(t *testing.T) {
	service := provenanceServiceStub{result: provenance.Bundle{
		SchemaVersion: provenance.SchemaVersion,
		Analysis:      job.AnalysisJob{ID: 3, ProjectID: 2, SourceSHA: "head", TargetSHA: "base"},
		Calls: []provenance.Call{{ID: 7, Phase: provenance.PhaseGeneration,
			PromptHash: strings.Repeat("a", 64), Status: provenance.StatusCompleted}},
	}, summary: provenance.SummaryBundle{SchemaVersion: provenance.SchemaVersion,
		Analysis: job.AnalysisJob{ID: 3, ProjectID: 2},
		Calls: []provenance.CallSummary{{ID: 7, Phase: provenance.PhaseGeneration,
			PromptHash: strings.Repeat("a", 64), Status: provenance.StatusCompleted}}}}
	for _, test := range []struct {
		path        string
		disposition bool
	}{
		{path: "/api/analyses/3/evidence"},
		{path: "/api/analyses/3/export", disposition: true},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.SetPathValue("id", "3")
		response := httptest.NewRecorder()
		handler := provenanceHandler{service: service}
		if test.disposition {
			handler.export(response, request)
		} else {
			handler.get(response, request)
		}
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), provenance.SchemaVersion) {
			t.Fatalf("GET %s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
		if test.disposition && !strings.Contains(response.Header().Get("Content-Disposition"), "analysis-3") {
			t.Fatalf("export disposition=%q", response.Header().Get("Content-Disposition"))
		}
	}
}

func TestProvenanceHandlerMapsErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{{job.ErrNotFound, http.StatusNotFound}, {errors.New("database"), http.StatusInternalServerError}} {
		request := httptest.NewRequest(http.MethodGet, "/api/analyses/3/evidence", nil)
		request.SetPathValue("id", "3")
		response := httptest.NewRecorder()
		provenanceHandler{service: provenanceServiceStub{err: test.err}}.get(response, request)
		if response.Code != test.status {
			t.Fatalf("error=%v status=%d body=%s", test.err, response.Code, response.Body.String())
		}
	}
}
