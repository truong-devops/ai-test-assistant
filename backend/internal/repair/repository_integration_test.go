//go:build integration

package repair

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

func TestRepositoryVersionsRepairsAndTerminatesAtLimit(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	projectID, analysisID, sourceID, validationID := createRepairAnalysis(t, ctx, pool)
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()
	repository := NewRepository(pool)

	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || claimed.ID != analysisID || claimed.Status != job.StatusRepairing {
		t.Fatalf("claimed=%#v error=%v", claimed, err)
	}
	if err := repository.RenewLease(ctx, claimed, time.Minute); err != nil {
		t.Fatal(err)
	}
	version2 := repairedVersion(analysisID, 2, "version two")
	if err := repository.SaveRepairs(ctx, claimed, []ProposedRepair{{
		SourceGeneratedTestID: sourceID, ValidationRunID: validationID,
		AttemptNumber: 1, Generated: version2, Reason: "compiler failure",
	}}); err != nil {
		t.Fatal(err)
	}
	attempts, err := repository.List(ctx, analysisID)
	if err != nil || len(attempts) != 1 || attempts[0].AttemptNumber != 1 ||
		attempts[0].GeneratedTestID != sourceID || attempts[0].PreviousCodeHash == attempts[0].RepairedCodeHash {
		t.Fatalf("attempts=%#v error=%v", attempts, err)
	}
	latest, err := generation.NewRepository(pool).ListLatest(ctx, analysisID)
	if err != nil || len(latest) != 1 || latest[0].GenerationAttempt != 2 || latest[0].ID == sourceID {
		t.Fatalf("latest=%#v error=%v", latest, err)
	}
	assertAnalysisStatus(t, ctx, pool, analysisID, job.StatusValidating, false)

	validation2 := addFailedValidationAndRepairState(t, ctx, pool, analysisID, latest[0].ID, 2)
	claimed, err = repository.ClaimNext(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	version3 := repairedVersion(analysisID, 3, "version three")
	if err := repository.SaveRepairs(ctx, claimed, []ProposedRepair{{
		SourceGeneratedTestID: latest[0].ID, ValidationRunID: validation2,
		AttemptNumber: 2, Generated: version3, Reason: "assertion failure",
	}}); err != nil {
		t.Fatal(err)
	}
	latest, err = generation.NewRepository(pool).ListLatest(ctx, analysisID)
	if err != nil || len(latest) != 1 || latest[0].GenerationAttempt != 3 {
		t.Fatalf("latest=%#v error=%v", latest, err)
	}

	_ = addFailedValidationAndRepairState(t, ctx, pool, analysisID, latest[0].ID, 3)
	claimed, err = repository.ClaimNext(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveRepairs(ctx, claimed, nil); err != nil {
		t.Fatal(err)
	}
	assertAnalysisStatus(t, ctx, pool, analysisID, job.StatusWaitingReview, true)
	attempts, err = repository.List(ctx, analysisID)
	if err != nil || len(attempts) != 2 || attempts[1].AttemptNumber != 2 {
		t.Fatalf("attempts=%#v error=%v", attempts, err)
	}
}

func repairedVersion(analysisID int64, generationAttempt int, marker string) generation.GeneratedTest {
	code := fmt.Sprintf("package sample\n\nimport \"testing\"\n\nfunc TestRun(t *testing.T) { t.Log(%q) }\n", marker)
	return generation.GeneratedTest{AnalysisJobID: analysisID,
		FilePath: "service_generated_test.go", TestNames: []string{"TestRun"},
		Code: code, CodeHash: generation.CodeHash(code), ModelName: "fixture-model",
		PromptVersion: PromptVersion, ProviderResponseID: "response-" + marker,
		GenerationAttempt: generationAttempt}
}

func createRepairAnalysis(t *testing.T, ctx context.Context,
	pool *pgxpool.Pool,
) (int64, int64, int64, int64) {
	t.Helper()
	var projectID int64
	if err := pool.QueryRow(ctx, `INSERT INTO projects
		(name, gitlab_project_id, repository_url, default_branch, language, status)
		VALUES ($1,$2,'https://gitlab.example.com/repair.git','main','go','active') RETURNING id`,
		"repair-integration", time.Now().UnixNano()).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var analysisID int64
	if err := pool.QueryRow(ctx, `INSERT INTO analysis_jobs
		(project_id, merge_request_iid, source_sha, target_sha, status, webhook_uuid)
		VALUES ($1,1,'head','base',$2,$3) RETURNING id`, projectID, job.StatusRepairing,
		fmt.Sprintf("repair-%d", time.Now().UnixNano())).Scan(&analysisID); err != nil {
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
		VALUES ($1,$2,'Repair Run','Cover Run','high','Changed behavior',
		 'Call Run','Returns expected value','fixture','recommend-test-v1') RETURNING id`,
		analysisID, symbolID).Scan(&recommendationID); err != nil {
		t.Fatal(err)
	}
	code := "package sample\n\nimport \"testing\"\n\nfunc TestRun(t *testing.T) { missingSymbol() }\n"
	var generatedID int64
	if err := pool.QueryRow(ctx, `INSERT INTO generated_tests
		(analysis_job_id, recommendation_id, file_path, test_names, code, code_hash,
		 model_name, prompt_version, generation_attempt)
		VALUES ($1,$2,'service_generated_test.go','["TestRun"]',$3,$4,
		 'fixture','generate-test-v1',1) RETURNING id`, analysisID, recommendationID,
		code, generation.CodeHash(code)).Scan(&generatedID); err != nil {
		t.Fatal(err)
	}
	var validationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO validation_runs
		(analysis_job_id, generated_test_id, attempt_number, command, status, exit_code,
		 duration_ms, stdout, stderr)
		VALUES ($1,$2,1,'go test ./...',$3,1,10,'','undefined: missingSymbol') RETURNING id`,
		analysisID, generatedID, validation.StatusFailed).Scan(&validationID); err != nil {
		t.Fatal(err)
	}
	return projectID, analysisID, generatedID, validationID
}

func addFailedValidationAndRepairState(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	analysisID, generatedID int64, attempt int,
) int64 {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE analysis_jobs SET status=$2, attempt_count=0,
		lease_expires_at=NULL, next_attempt_at=NOW() WHERE id=$1`, analysisID, job.StatusRepairing); err != nil {
		t.Fatal(err)
	}
	var validationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO validation_runs
		(analysis_job_id, generated_test_id, attempt_number, command, status, exit_code,
		 duration_ms, stdout, stderr)
		VALUES ($1,$2,$3,'go test ./...',$4,1,10,'','still failing') RETURNING id`,
		analysisID, generatedID, attempt, validation.StatusFailed).Scan(&validationID); err != nil {
		t.Fatal(err)
	}
	return validationID
}

func assertAnalysisStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	analysisID int64, want string, wantFinished bool,
) {
	t.Helper()
	analysisJob, _, err := job.NewRepository(pool).Get(ctx, analysisID)
	if err != nil || analysisJob.Status != want || (analysisJob.FinishedAt != nil) != wantFinished {
		t.Fatalf("analysis=%#v error=%v want status=%s finished=%v",
			analysisJob, err, want, wantFinished)
	}
}
