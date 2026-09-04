package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/impact"
)

type impactServiceStub struct {
	result impact.Bundle
	err    error
}

func (s impactServiceStub) Get(context.Context, int64) (impact.Bundle, error) { return s.result, s.err }

func TestImpactHandler(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/4/impact", nil)
	request.SetPathValue("id", "4")
	response := httptest.NewRecorder()
	impactHandler{service: impactServiceStub{result: impact.Bundle{Run: impact.Run{
		ID: 2, AnalysisJobID: 4, Mode: impact.ModeSSA, Algorithm: "cha-v1"},
		Nodes: []impact.Node{{ID: 3, SymbolName: "Load", ReasonCodes: []string{impact.ReasonCaller}}},
	}}}.get(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"algorithm":"cha-v1"`) ||
		!strings.Contains(response.Body.String(), `"symbol_name":"Load"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestImpactHandlerNotFound(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/4/impact", nil)
	request.SetPathValue("id", "4")
	response := httptest.NewRecorder()
	impactHandler{service: impactServiceStub{err: impact.ErrNotFound}}.get(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d", response.Code)
	}
}
