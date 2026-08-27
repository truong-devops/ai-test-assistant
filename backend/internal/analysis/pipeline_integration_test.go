//go:build integration

package analysis_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/analysis"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

func TestWebhookToFetchedChangesPipeline(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	gitLabProjectID := time.Now().UnixNano()
	var projectID int64
	err = pool.QueryRow(ctx, `INSERT INTO projects
		(name, gitlab_project_id, repository_url, default_branch, language, status)
		VALUES ('pipeline', $1, 'https://gitlab.example.com/pipeline.git', 'main', 'go', 'active') RETURNING id`,
		gitLabProjectID).Scan(&projectID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()

	projects := project.NewPostgresRepository(pool)
	jobs := job.NewRepository(pool)
	webhook := gitlab.NewWebhookHandler("secret", gitlab.NewWebhookService(projects, jobs))
	payload := fmt.Sprintf(`{"object_kind":"merge_request","project":{"id":%d},"object_attributes":{"iid":5,"action":"open","last_commit":{"id":"payload-sha"}}}`, gitLabProjectID)
	request := httptest.NewRequest(http.MethodPost, "/api/webhooks/gitlab", strings.NewReader(payload))
	request.Header.Set(gitlab.WebhookTokenHeader, "secret")
	request.Header.Set(gitlab.WebhookEventHeader, "Merge Request Hook")
	request.Header.Set(gitlab.WebhookUUIDHeader, fmt.Sprintf("pipeline-%d", time.Now().UnixNano()))
	response := httptest.NewRecorder()
	webhook.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("webhook status=%d body=%s", response.Code, response.Body.String())
	}

	gitLabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case fmt.Sprintf("/api/v4/projects/%d/merge_requests/5", gitLabProjectID):
			fmt.Fprint(w, `{"iid":5,"title":"business rule","source_branch":"feature","target_branch":"main","web_url":"https://gitlab.example.com/mr/5","diff_refs":{"head_sha":"authoritative-head","start_sha":"authoritative-target"}}`)
		case fmt.Sprintf("/api/v4/projects/%d/merge_requests/5/diffs", gitLabProjectID):
			fmt.Fprint(w, `[{"old_path":"service.go","new_path":"service.go","diff":"@@ -1 +1 @@\n-old\n+new","new_file":false,"renamed_file":false,"deleted_file":false}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gitLabServer.Close()
	client, err := gitlab.NewHTTPClient(gitLabServer.URL, "token", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	processor := analysis.NewProcessor(projects, client, jobs)
	worker := job.NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), jobs, processor, job.WorkerOptions{
		PollInterval: 10 * time.Millisecond, RetryDelay: 10 * time.Millisecond,
		LeaseDuration: time.Minute, MaxAttempts: 3,
	})
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() { workerDone <- worker.Run(workerCtx) }()
	defer func() {
		stopWorker()
		<-workerDone
	}()

	var webhookResponse struct {
		AnalysisJobID int64 `json:"analysis_job_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &webhookResponse); err != nil {
		t.Fatal(err)
	}
	var result job.AnalysisJob
	var files []job.ChangedFile
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		result, files, err = jobs.Get(ctx, webhookResponse.AnalysisJobID)
		if err == nil && result.Status == job.StatusAnalyzingChange {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if result.Status != job.StatusAnalyzingChange || result.SourceSHA != "authoritative-head" || len(files) != 1 {
		t.Fatalf("result=%+v files=%+v", result, files)
	}
}
