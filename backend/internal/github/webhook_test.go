package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type projectFinderStub struct{ item project.Project }

func (s projectFinderStub) GetByProviderProjectID(_ context.Context, provider string, id int64) (project.Project, error) {
	if provider != "github" || s.item.ExternalProjectID() != id {
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

func TestWebhookHandlerVerifiesAndEnqueuesPullRequest(t *testing.T) {
	enqueuer := &enqueuerStub{created: true}
	handler := NewWebhookHandler("secret", NewWebhookService(projectFinderStub{item: project.Project{
		ID: 2, Provider: "github", ProviderProjectID: 12,
	}}, enqueuer))
	payload := `{"action":"synchronize","number":4,"repository":{"id":12},"pull_request":{"number":4,"head":{"sha":"abc"}}}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(payload, "secret"))
	if response.Code != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d body=%s", response.Code, body)
	}
	if enqueuer.input.ProjectID != 2 || enqueuer.input.MergeRequestIID != 4 ||
		enqueuer.input.SourceSHA != "abc" || enqueuer.input.WebhookUUID != "github:delivery-1" {
		t.Fatalf("input=%+v", enqueuer.input)
	}
}

func TestWebhookHandlerRejectsInvalidSignatureAndEvent(t *testing.T) {
	handler := NewWebhookHandler("secret", NewWebhookService(projectFinderStub{}, &enqueuerStub{}))
	request := webhookRequest(`{}`, "wrong")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("signature status=%d", response.Code)
	}
	request = webhookRequest(`{}`, "secret")
	request.Header.Set(WebhookEventHeader, "push")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("event status=%d", response.Code)
	}
}

func TestWebhookHandlerRejectsUnsupportedAction(t *testing.T) {
	handler := NewWebhookHandler("secret", NewWebhookService(projectFinderStub{item: project.Project{
		ID: 2, Provider: "github", ProviderProjectID: 12,
	}}, &enqueuerStub{}))
	payload := `{"action":"closed","number":4,"repository":{"id":12},"pull_request":{"head":{"sha":"abc"}}}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(payload, "secret"))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func webhookRequest(payload, secret string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", strings.NewReader(payload))
	request.Header.Set(WebhookEventHeader, "pull_request")
	request.Header.Set(WebhookDeliveryHeader, "delivery-1")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	request.Header.Set(WebhookSignatureHeader, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return request
}
