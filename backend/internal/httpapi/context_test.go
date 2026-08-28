package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

type analysisContextServiceStub struct {
	items []knowledge.KnowledgeChunk
	err   error
}

func (s analysisContextServiceStub) List(context.Context, int64) ([]knowledge.KnowledgeChunk, error) {
	return s.items, s.err
}

func TestAnalysisContextHandlerListsSourceEvidence(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/3/context", nil)
	request.SetPathValue("id", "3")
	response := httptest.NewRecorder()
	analysisContextHandler{service: analysisContextServiceStub{items: []knowledge.KnowledgeChunk{{
		ID: 1, ProjectID: 2, FilePath: "internal/user/service.go", Content: "func CreateUser() {}",
	}}}}.list(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "service.go") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAnalysisContextHandlerMapsErrors(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/3/context", nil)
	request.SetPathValue("id", "3")
	response := httptest.NewRecorder()
	analysisContextHandler{service: analysisContextServiceStub{err: job.ErrNotFound}}.list(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/analyses/3/context", nil)
	request.SetPathValue("id", "3")
	response = httptest.NewRecorder()
	analysisContextHandler{service: analysisContextServiceStub{err: errors.New("database")}}.list(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
