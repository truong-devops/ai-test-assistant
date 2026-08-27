//go:build integration

package recommendation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

func TestRepositoryRecommendationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	projectID, analysisID, symbolID := createRecommendationAnalysis(t, ctx, pool, "lifecycle")
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()
	repository := NewRepository(pool)
	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || claimed.ID != analysisID || claimed.Status != job.StatusRecommendingTests || claimed.AttemptCount != 1 {
		t.Fatalf("claimed=%#v error=%v", claimed, err)
	}
	if err := repository.RenewLease(ctx, claimed, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, claimed, []Recommendation{{
		ChangedSymbolID: symbolID, Title: "Duplicate email", Description: "Cover duplicate branch",
		Priority: PriorityHigh, Rationale: "branch is uncovered", Scenario: "lookup finds user",
		ExpectedBehavior: "returns ErrEmailExists", ModelName: "fixture-model",
		PromptVersion: PromptVersion, ProviderResponseID: "resp-1",
	}}); err != nil {
		t.Fatal(err)
	}
	items, err := repository.List(ctx, analysisID)
	if err != nil || len(items) != 1 || items[0].ChangedSymbolID != symbolID ||
		items[0].Status != StatusPending || items[0].ExpectedBehavior != "returns ErrEmailExists" {
		t.Fatalf("items=%#v error=%v", items, err)
	}
	analysisJob, _, err := job.NewRepository(pool).Get(ctx, analysisID)
	if err != nil || analysisJob.Status != job.StatusGeneratingTests || analysisJob.AttemptCount != 0 {
		t.Fatalf("analysis=%#v error=%v", analysisJob, err)
	}
	if err := repository.RetryOrFail(ctx, claimed.ID, claimed.AttemptCount,
		fmt.Errorf("stale worker"), 3, time.Second); !errors.Is(err, job.ErrLeaseLost) {
		t.Fatalf("stale RetryOrFail() error=%v", err)
	}
}

func TestRepositoryRejectsCrossAnalysisSymbolAtomically(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	projectID, analysisID, _ := createRecommendationAnalysis(t, ctx, pool, "isolation-one")
	_, _, otherSymbolID := createRecommendationAnalysisForProject(t, ctx, pool, projectID, "isolation-two")
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()
	repository := NewRepository(pool)
	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || claimed.ID != analysisID {
		t.Fatalf("claimed=%#v error=%v", claimed, err)
	}
	err = repository.Save(ctx, claimed, []Recommendation{{
		ChangedSymbolID: otherSymbolID, Title: "foreign", Description: "description",
		Priority: PriorityLow, Rationale: "rationale", Scenario: "scenario",
		ExpectedBehavior: "expected", ModelName: "model", PromptVersion: PromptVersion,
	}})
	if err == nil {
		t.Fatal("Save() error=nil, want cross-analysis rejection")
	}
	var status string
	var count int
	if err := pool.QueryRow(ctx, `SELECT status FROM analysis_jobs WHERE id=$1`, analysisID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM test_recommendations WHERE analysis_job_id=$1`, analysisID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if status != job.StatusRecommendingTests || count != 0 {
		t.Fatalf("status=%s count=%d", status, count)
	}
}

func createRecommendationAnalysis(t *testing.T, ctx context.Context, pool *pgxpool.Pool, suffix string) (int64, int64, int64) {
	t.Helper()
	var projectID int64
	if err := pool.QueryRow(ctx, `INSERT INTO projects
		(name, gitlab_project_id, repository_url, default_branch, language, status)
		VALUES ($1,$2,'https://gitlab.example.com/recommend.git','main','go','active') RETURNING id`,
		"recommend-"+suffix, time.Now().UnixNano()).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	_, analysisID, symbolID := createRecommendationAnalysisForProject(t, ctx, pool, projectID, suffix)
	return projectID, analysisID, symbolID
}

func createRecommendationAnalysisForProject(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	projectID int64, suffix string,
) (int64, int64, int64) {
	t.Helper()
	var analysisID int64
	if err := pool.QueryRow(ctx, `INSERT INTO analysis_jobs
		(project_id, merge_request_iid, source_sha, target_sha, status, webhook_uuid)
		VALUES ($1,1,'head','base',$2,$3) RETURNING id`, projectID,
		job.StatusRetrievingContext, fmt.Sprintf("recommend-%s-%d", suffix, time.Now().UnixNano())).Scan(&analysisID); err != nil {
		t.Fatal(err)
	}
	var fileID int64
	if err := pool.QueryRow(ctx, `INSERT INTO changed_files
		(analysis_job_id, old_path, new_path, change_type, diff)
		VALUES ($1,'internal/user/service.go','internal/user/service.go','modified','+branch') RETURNING id`,
		analysisID).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	var symbolID int64
	if err := pool.QueryRow(ctx, `INSERT INTO changed_symbols
		(changed_file_id, symbol_name, symbol_kind, package_name, start_line, end_line, change_type, change_summary)
		VALUES ($1,'CreateUser','method','user',10,30,'modified','modified CreateUser') RETURNING id`,
		fileID).Scan(&symbolID); err != nil {
		t.Fatal(err)
	}
	return projectID, analysisID, symbolID
}
