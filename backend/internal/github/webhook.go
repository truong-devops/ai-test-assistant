package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

const (
	WebhookEventHeader     = "X-GitHub-Event"
	WebhookDeliveryHeader  = "X-GitHub-Delivery"
	WebhookSignatureHeader = "X-Hub-Signature-256"
	maxDeliveryIDLength    = 128
)

var (
	ErrUnsupportedEvent = errors.New("unsupported GitHub event")
	ErrInvalidWebhook   = errors.New("invalid GitHub webhook")
)

type WebhookPayload struct {
	Action     string `json:"action"`
	Number     int64  `json:"number"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	PullRequest struct {
		Number int64 `json:"number"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
}

type ProjectFinder interface {
	GetByProviderProjectID(ctx context.Context, provider string, providerProjectID int64) (project.Project, error)
}

type JobEnqueuer interface {
	Enqueue(ctx context.Context, input job.EnqueueInput) (job.AnalysisJob, bool, error)
}

type WebhookService struct {
	projects ProjectFinder
	jobs     JobEnqueuer
}

func NewWebhookService(projects ProjectFinder, jobs JobEnqueuer) *WebhookService {
	return &WebhookService{projects: projects, jobs: jobs}
}

func (s *WebhookService) Accept(ctx context.Context, deliveryID string, rawEvent json.RawMessage) (job.AnalysisJob, bool, error) {
	var payload WebhookPayload
	if err := json.Unmarshal(rawEvent, &payload); err != nil {
		return job.AnalysisJob{}, false, ErrInvalidWebhook
	}
	number := payload.Number
	if number <= 0 {
		number = payload.PullRequest.Number
	}
	if payload.Repository.ID <= 0 || number <= 0 || strings.TrimSpace(payload.PullRequest.Head.SHA) == "" {
		return job.AnalysisJob{}, false, ErrInvalidWebhook
	}
	if !supportedPullRequestAction(payload.Action) {
		return job.AnalysisJob{}, false, ErrUnsupportedEvent
	}
	registeredProject, err := s.projects.GetByProviderProjectID(ctx, scm.ProviderGitHub, payload.Repository.ID)
	if err != nil {
		return job.AnalysisJob{}, false, err
	}
	return s.jobs.Enqueue(ctx, job.EnqueueInput{
		ProjectID: registeredProject.ID, MergeRequestIID: number,
		SourceSHA: payload.PullRequest.Head.SHA, WebhookUUID: "github:" + deliveryID, RawEvent: rawEvent,
	})
}

func supportedPullRequestAction(action string) bool {
	return action == "opened" || action == "reopened" || action == "synchronize"
}

type WebhookHandler struct {
	secret  string
	service *WebhookService
}

func NewWebhookHandler(secret string, service *WebhookService) *WebhookHandler {
	return &WebhookHandler{secret: secret, service: service}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWebhookError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.secret == "" {
		writeWebhookError(w, http.StatusServiceUnavailable, "GitHub webhook is not configured")
		return
	}
	if r.Header.Get(WebhookEventHeader) != "pull_request" {
		writeWebhookError(w, http.StatusBadRequest, "unsupported webhook event")
		return
	}
	deliveryID := strings.TrimSpace(r.Header.Get(WebhookDeliveryHeader))
	if deliveryID == "" || len(deliveryID) > maxDeliveryIDLength {
		writeWebhookError(w, http.StatusBadRequest, "invalid webhook delivery ID")
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		writeWebhookError(w, http.StatusBadRequest, "invalid webhook body")
		return
	}
	if !validSignature(h.secret, body, r.Header.Get(WebhookSignatureHeader)) {
		writeWebhookError(w, http.StatusUnauthorized, "invalid webhook signature")
		return
	}
	analysis, created, err := h.service.Accept(r.Context(), deliveryID, body)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnsupportedEvent), errors.Is(err, ErrInvalidWebhook):
			writeWebhookError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, project.ErrNotFound):
			writeWebhookError(w, http.StatusNotFound, "GitHub repository is not registered")
		default:
			writeWebhookError(w, http.StatusInternalServerError, "could not enqueue analysis")
		}
		return
	}
	writeWebhookJSON(w, http.StatusAccepted, map[string]any{
		"analysis_job_id": analysis.ID, "status": analysis.Status, "created": created,
	})
}

func validSignature(secret string, body []byte, provided string) bool {
	if !strings.HasPrefix(provided, "sha256=") {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(provided, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(decoded, mac.Sum(nil))
}

func writeWebhookError(w http.ResponseWriter, status int, message string) {
	writeWebhookJSON(w, status, map[string]string{"error": message})
}

func writeWebhookJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
