//go:build integration

package validation

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

func TestRepositoryValidationLifecycle(t *testing.T) {
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
	projectID, analysisID, generatedID := createValidationAnalysis(t, ctx, pool, "passing")
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()
	repository := NewRepository(pool)
	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || claimed.ID != analysisID || claimed.Status != job.StatusValidating || claimed.AttemptCount != 1 {
		t.Fatalf("claimed=%#v error=%v", claimed, err)
	}
	if err := repository.RenewLease(ctx, claimed, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, claimed, []Run{{
		GeneratedTestID: generatedID, AttemptNumber: 1,
		Command: "go test -count=1 ./...", Status: StatusPassed,
		ExitCode: 0, DurationMS: 12, Stdout: "ok",
	}}); err != nil {
		t.Fatal(err)
	}
	runs, err := repository.List(ctx, analysisID)
	if err != nil || len(runs) != 1 || runs[0].GeneratedTestID != generatedID ||
		runs[0].Status != StatusPassed || runs[0].DurationMS != 12 {
		t.Fatalf("runs=%#v error=%v", runs, err)
	}
	analysisJob, _, err := job.NewRepository(pool).Get(ctx, analysisID)
	if err != nil || analysisJob.Status != job.StatusWaitingReview || analysisJob.AttemptCount != 0 ||
		analysisJob.FinishedAt == nil {
		t.Fatalf("analysis=%#v error=%v", analysisJob, err)
	}
	if err := repository.RetryOrFail(ctx, claimed.ID, claimed.AttemptCount,
		fmt.Errorf("stale worker"), 3, time.Second); !errors.Is(err, job.ErrLeaseLost) {
		t.Fatalf("stale RetryOrFail() error=%v", err)
	}
}

func TestRepositoryAdvancesFailedValidationToRepairing(t *testing.T) {
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
	projectID, analysisID, generatedID := createValidationAnalysis(t, ctx, pool, "failing")
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()
	repository := NewRepository(pool)
	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, claimed, []Run{{GeneratedTestID: generatedID,
		AttemptNumber: 1, Command: "go test ./...", Status: StatusFailed,
		ExitCode: 1, Stderr: "compile failed"}}); err != nil {
		t.Fatal(err)
	}
	analysisJob, _, err := job.NewRepository(pool).Get(ctx, analysisID)
	if err != nil || analysisJob.Status != job.StatusRepairing || analysisJob.FinishedAt != nil {
		t.Fatalf("analysis=%#v error=%v", analysisJob, err)
	}
}

func createValidationAnalysis(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	suffix string,
) (int64, int64, int64) {
	t.Helper()
	var projectID int64
	if err := pool.QueryRow(ctx, `INSERT INTO projects
		(name, gitlab_project_id, repository_url, default_branch, language, status)
		VALUES ($1,$2,'https://gitlab.example.com/validate.git','main','go','active') RETURNING id`,
		"validate-"+suffix, time.Now().UnixNano()).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var analysisID int64
	if err := pool.QueryRow(ctx, `INSERT INTO analysis_jobs
		(project_id, merge_request_iid, source_sha, target_sha, status, webhook_uuid)
		VALUES ($1,1,'head','base',$2,$3) RETURNING id`, projectID,
		job.StatusValidating, fmt.Sprintf("validate-%s-%d", suffix, time.Now().UnixNano())).Scan(&analysisID); err != nil {
		t.Fatal(err)
	}
	var fileID int64
	if err := pool.QueryRow(ctx, `INSERT INTO changed_files
		(analysis_job_id, old_path, new_path, change_type, diff)
		VALUES ($1,'service.go','service.go','modified','+change') RETURNING id`, analysisID).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	var symbolID int64
	if err := pool.QueryRow(ctx, `INSERT INTO changed_symbols
		(changed_file_id, symbol_name, symbol_kind, package_name, start_line, end_line, change_type, change_summary)
		VALUES ($1,'Run','function','sample',1,2,'modified','modified Run') RETURNING id`, fileID).Scan(&symbolID); err != nil {
		t.Fatal(err)
	}
	var recommendationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO test_recommendations
		(analysis_job_id, changed_symbol_id, title, description, priority, rationale,
		 scenario, expected_behavior, model_name, prompt_version)
		VALUES ($1,$2,'Validate Run','Cover Run','high','Changed behavior',
		 'Call Run','Returns expected value','fixture','recommend-test-v1') RETURNING id`,
		analysisID, symbolID).Scan(&recommendationID); err != nil {
		t.Fatal(err)
	}
	var generatedID int64
	if err := pool.QueryRow(ctx, `INSERT INTO generated_tests
		(analysis_job_id, recommendation_id, file_path, test_names, code, code_hash,
		 model_name, prompt_version, generation_attempt)
		VALUES ($1,$2,'service_generated_test.go','["TestRun"]','package sample',
		 $3,'fixture','generate-test-v1',1) RETURNING id`, analysisID, recommendationID,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Scan(&generatedID); err != nil {
		t.Fatal(err)
	}
	return projectID, analysisID, generatedID
}
