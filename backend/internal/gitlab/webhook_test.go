package gitlab

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type projectFinderStub struct{ item project.Project }

func (s projectFinderStub) GetByGitLabProjectID(_ context.Context, id int64) (project.Project, error) {
	if s.item.GitLabProjectID != id {
		return project.Project{}, project.ErrNotFound
	}
	return s.item, nil
}

type enqueuerStub struct {
	input   job.EnqueueInput
	created bool
}

func (s *enqueuerStub) Enqueue(_ context.Context, input job.EnqueueInput) (job.AnalysisJob, bool, error) {
	s.input = input
	return job.AnalysisJob{ID: 7, Status: job.StatusPending}, s.created, nil
}

func TestWebhookHandlerEnqueuesMergeRequest(t *testing.T) {
	enqueuer := &enqueuerStub{created: true}
	service := NewWebhookService(projectFinderStub{item: project.Project{ID: 2, GitLabProjectID: 12}}, enqueuer)
	handler := NewWebhookHandler("secret", service)
	payload := `{"object_kind":"merge_request","project":{"id":12},"object_attributes":{"iid":4,"action":"update","last_commit":{"id":"abc"}}}`
	request := webhookRequest(payload)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.Code, body)
	}
	if enqueuer.input.ProjectID != 2 || enqueuer.input.MergeRequestIID != 4 || enqueuer.input.SourceSHA != "abc" || enqueuer.input.WebhookUUID != "uuid-1" {
		t.Fatalf("enqueue input = %+v", enqueuer.input)
	}
}

func TestWebhookHandlerRejectsInvalidToken(t *testing.T) {
	service := NewWebhookService(projectFinderStub{}, &enqueuerStub{})
	handler := NewWebhookHandler("different", service)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(`{}`))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHandlerRejectsUnsupportedAction(t *testing.T) {
	service := NewWebhookService(projectFinderStub{item: project.Project{ID: 2, GitLabProjectID: 12}}, &enqueuerStub{})
	handler := NewWebhookHandler("secret", service)
	request := webhookRequest(`{"object_kind":"merge_request","project":{"id":12},"object_attributes":{"iid":4,"action":"merge"}}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestWebhookHandlerRejectsMalformedRequests(t *testing.T) {
	service := NewWebhookService(projectFinderStub{item: project.Project{ID: 2, GitLabProjectID: 12}}, &enqueuerStub{})
	handler := NewWebhookHandler("secret", service)
	tests := []struct {
		name   string
		mutate func(*http.Request)
		body   string
		status int
	}{
		{name: "wrong method", body: `{}`, status: http.StatusMethodNotAllowed, mutate: func(r *http.Request) { r.Method = http.MethodGet }},
		{name: "wrong event", body: `{}`, status: http.StatusBadRequest, mutate: func(r *http.Request) { r.Header.Set(WebhookEventHeader, "Push Hook") }},
		{name: "missing UUID", body: `{}`, status: http.StatusBadRequest, mutate: func(r *http.Request) { r.Header.Del(WebhookUUIDHeader) }},
		{name: "oversized UUID", body: `{}`, status: http.StatusBadRequest, mutate: func(r *http.Request) { r.Header.Set(WebhookUUIDHeader, strings.Repeat("a", maxWebhookUUIDLength+1)) }},
		{name: "invalid JSON", body: `{`, status: http.StatusBadRequest, mutate: func(*http.Request) {}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := webhookRequest(test.body)
			test.mutate(request)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}
}

func TestWebhookHandlerRejectsWhenSecretIsNotConfigured(t *testing.T) {
	handler := NewWebhookHandler("", NewWebhookService(projectFinderStub{}, &enqueuerStub{}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(`{}`))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func webhookRequest(payload string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/webhooks/gitlab", strings.NewReader(payload))
	request.Header.Set(WebhookTokenHeader, "secret")
	request.Header.Set(WebhookEventHeader, "Merge Request Hook")
	request.Header.Set(WebhookUUIDHeader, "uuid-1")
	return request
}

func FuzzWebhookHandlerNeverPanics(f *testing.F) {
	f.Add(`{"object_kind":"merge_request","project":{"id":12},"object_attributes":{"iid":4,"action":"open"}}`)
	f.Add(`{`)
	f.Add("")
	f.Fuzz(func(t *testing.T, payload string) {
		service := NewWebhookService(projectFinderStub{item: project.Project{ID: 2, GitLabProjectID: 12}}, &enqueuerStub{})
		handler := NewWebhookHandler("secret", service)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, webhookRequest(payload))
		if response.Code < 100 || response.Code > 599 {
			t.Fatalf("invalid HTTP status %d", response.Code)
		}
	})
}
