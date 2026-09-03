//go:build integration

package knowledge

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

func TestRequestedIndexToRetrievalPipeline(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO projects
		(name, provider, provider_project_id, repository_url, default_branch, language, status)
		VALUES ('index-pipeline','gitlab',$1,'https://gitlab.example.com/index.git','main','go','active') RETURNING id`,
		gitLabProjectID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()

	gitLabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := fmt.Sprintf("/api/v4/projects/%d", gitLabProjectID)
		switch r.URL.Path {
		case prefix + "/repository/tree":
			fmt.Fprint(w, `[{"type":"blob","path":"internal/user/service.go"},`+
				`{"type":"blob","path":"internal/user/service_test.go"},`+
				`{"type":"blob","path":"internal/user/mock_repository_test.go"},`+
				`{"type":"blob","path":"README.md"}]`)
		case prefix + "/repository/files/internal/user/service.go/raw":
			fmt.Fprint(w, "package user\ntype Repository interface { Create() error }\nfunc CreateUser() {}\n")
		case prefix + "/repository/files/internal/user/service_test.go/raw":
			fmt.Fprint(w, "package user\nfunc TestCreateUser(t *testing.T) {}\n")
		case prefix + "/repository/files/internal/user/mock_repository_test.go/raw":
			fmt.Fprint(w, "package user\ntype mockRepository struct{}\n")
		case prefix + "/repository/files/README.md/raw":
			fmt.Fprint(w, "# User service\nCreateUser business rules.\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer gitLabServer.Close()
	client, err := gitlab.NewHTTPClient(gitLabServer.URL, "", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewPostgresRepository(pool)
	repository := NewRepository(pool)
	embedder := NewHashEmbeddingClient("hash-pipeline", EmbeddingDimensions)
	service := NewService(projects, repository)
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), repository,
		NewIndexer(projects, client, embedder, repository), WorkerOptions{
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
	requested, err := service.RequestIndex(ctx, projectID)
	if err != nil || requested.Status != IndexStatusPending {
		t.Fatalf("requested=%#v error=%v", requested, err)
	}
	var status IndexJob
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, err = service.GetIndexStatus(ctx, projectID)
		if err == nil && status.Status == IndexStatusReady {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status.Status != IndexStatusReady || status.FileCount != 4 || status.ChunkCount != 5 {
		t.Fatalf("status=%#v error=%v", status, err)
	}
	results, err := NewRetriever(repository, embedder).RetrieveContext(ctx, RetrievalQuery{
		ProjectID: projectID, Query: "CreateUser tests repository mock", PackageName: "user",
		SymbolName: "CreateUser", PreferTests: true, Limit: 8,
	})
	if err != nil || len(results) < 4 || results[0].SymbolName != "CreateUser" {
		t.Fatalf("results=%#v error=%v", results, err)
	}
}
