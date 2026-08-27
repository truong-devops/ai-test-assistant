package gitlab

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

const (
	WebhookEventHeader   = "X-Gitlab-Event"
	WebhookTokenHeader   = "X-Gitlab-Token"
	WebhookUUIDHeader    = "X-Gitlab-Webhook-UUID"
	maxWebhookUUIDLength = 128
)

var (
	ErrUnsupportedEvent = errors.New("unsupported GitLab event")
	ErrInvalidWebhook   = errors.New("invalid GitLab webhook")
)

type WebhookPayload struct {
	ObjectKind string `json:"object_kind"`
	Project    struct {
		ID int64 `json:"id"`
	} `json:"project"`
	ObjectAttributes struct {
		IID         int64  `json:"iid"`
		Action      string `json:"action"`
		OldRevision string `json:"oldrev"`
		LastCommit  struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
}

type ProjectFinder interface {
	GetByGitLabProjectID(ctx context.Context, gitLabProjectID int64) (project.Project, error)
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

func (s *WebhookService) Accept(ctx context.Context, webhookUUID string, rawEvent json.RawMessage) (job.AnalysisJob, bool, error) {
	var payload WebhookPayload
	if err := json.Unmarshal(rawEvent, &payload); err != nil {
		return job.AnalysisJob{}, false, ErrInvalidWebhook
	}
	if payload.ObjectKind != "merge_request" || payload.Project.ID <= 0 || payload.ObjectAttributes.IID <= 0 {
		return job.AnalysisJob{}, false, ErrInvalidWebhook
	}
	if !supportedMergeRequestAction(payload.ObjectAttributes.Action) {
		return job.AnalysisJob{}, false, ErrUnsupportedEvent
	}
	registeredProject, err := s.projects.GetByGitLabProjectID(ctx, payload.Project.ID)
	if err != nil {
		return job.AnalysisJob{}, false, err
	}
	return s.jobs.Enqueue(ctx, job.EnqueueInput{
		ProjectID:       registeredProject.ID,
		MergeRequestIID: payload.ObjectAttributes.IID,
		SourceSHA:       payload.ObjectAttributes.LastCommit.ID,
		WebhookUUID:     webhookUUID,
		RawEvent:        rawEvent,
	})
}

func supportedMergeRequestAction(action string) bool {
	return action == "open" || action == "reopen" || action == "update"
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
		writeWebhookError(w, http.StatusServiceUnavailable, "GitLab webhook is not configured")
		return
	}
	providedSecret := r.Header.Get(WebhookTokenHeader)
	if subtle.ConstantTimeCompare([]byte(providedSecret), []byte(h.secret)) != 1 {
		writeWebhookError(w, http.StatusUnauthorized, "invalid webhook token")
		return
	}
	if r.Header.Get(WebhookEventHeader) != "Merge Request Hook" {
		writeWebhookError(w, http.StatusBadRequest, "unsupported webhook event")
		return
	}
	webhookUUID := strings.TrimSpace(r.Header.Get(WebhookUUIDHeader))
	if webhookUUID == "" || len(webhookUUID) > maxWebhookUUIDLength {
		writeWebhookError(w, http.StatusBadRequest, "invalid webhook UUID")
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		writeWebhookError(w, http.StatusBadRequest, "invalid webhook body")
		return
	}
	analysis, created, err := h.service.Accept(r.Context(), webhookUUID, body)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnsupportedEvent):
			writeWebhookError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrInvalidWebhook):
			writeWebhookError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, project.ErrNotFound):
			writeWebhookError(w, http.StatusNotFound, "GitLab project is not registered")
		default:
			writeWebhookError(w, http.StatusInternalServerError, "could not enqueue analysis")
		}
		return
	}
	writeWebhookJSON(w, http.StatusAccepted, map[string]any{
		"analysis_job_id": analysis.ID,
		"status":          analysis.Status,
		"created":         created,
	})
}

func writeWebhookError(w http.ResponseWriter, status int, message string) {
	writeWebhookJSON(w, status, map[string]string{"error": message})
}

func writeWebhookJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
