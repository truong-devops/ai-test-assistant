package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) ClaimNext(ctx context.Context, leaseDuration time.Duration) (job.AnalysisJob, error) {
	const query = `
		WITH next_job AS (
			SELECT id FROM analysis_jobs
			WHERE status=$1 AND (
				(next_attempt_at<=NOW() AND lease_expires_at IS NULL)
				OR lease_expires_at<=NOW()
			)
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE analysis_jobs AS jobs
		SET started_at=COALESCE(started_at, NOW()), error_message=NULL,
			attempt_count=CASE
				WHEN jobs.lease_expires_at IS NULL AND jobs.error_message IS NULL THEN 1
				ELSE jobs.attempt_count + 1
			END,
			lease_expires_at=NOW()+$2::interval
		FROM next_job WHERE jobs.id=next_job.id
		RETURNING jobs.id, jobs.project_id, jobs.merge_request_iid, jobs.source_sha,
			jobs.target_sha, jobs.source_branch, jobs.target_branch, jobs.title, jobs.web_url,
			jobs.status, COALESCE(jobs.error_message, ''), jobs.webhook_uuid, jobs.attempt_count,
			jobs.raw_event, jobs.started_at, jobs.finished_at, jobs.created_at`
	var result job.AnalysisJob
	if err := r.pool.QueryRow(ctx, query, job.StatusGeneratingTests, leaseDuration.String()).
		Scan(analysisDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return job.AnalysisJob{}, job.ErrNotFound
		}
		return job.AnalysisJob{}, fmt.Errorf("claim generation job: %w", err)
	}
	return result, nil
}

func (r *Repository) RetryOrFail(ctx context.Context, id int64, expectedAttempt int, processErr error,
	maxAttempts int, retryDelay time.Duration,
) error {
	const query = `UPDATE analysis_jobs SET
		status=CASE WHEN attempt_count<$3 THEN $4 ELSE $5 END,
		error_message=$2,
		next_attempt_at=CASE WHEN attempt_count<$3 THEN NOW()+$6::interval ELSE next_attempt_at END,
		lease_expires_at=NULL,
		finished_at=CASE WHEN attempt_count<$3 THEN NULL ELSE NOW() END
		WHERE id=$1 AND status=$7 AND attempt_count=$8`
	result, err := r.pool.Exec(ctx, query, id, processErr.Error(), maxAttempts,
		job.StatusGeneratingTests, job.StatusFailed, retryDelay.String(),
		job.StatusGeneratingTests, expectedAttempt)
	if err != nil {
		return fmt.Errorf("retry or fail generation job: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	return nil
}

func (r *Repository) RenewLease(ctx context.Context, claimed job.AnalysisJob, leaseDuration time.Duration) error {
	result, err := r.pool.Exec(ctx, `UPDATE analysis_jobs SET lease_expires_at=NOW()+$3::interval
		WHERE id=$1 AND status=$4 AND attempt_count=$2`, claimed.ID, claimed.AttemptCount,
		leaseDuration.String(), job.StatusGeneratingTests)
	if err != nil {
		return fmt.Errorf("renew generation lease: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	return nil
}

func (r *Repository) Save(ctx context.Context, claimed job.AnalysisJob, generated []GeneratedTest) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save generated tests: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM generated_tests WHERE analysis_job_id=$1`, claimed.ID); err != nil {
		return fmt.Errorf("clear stale generated tests: %w", err)
	}
	const insert = `INSERT INTO generated_tests
		(analysis_job_id, recommendation_id, file_path, test_names, code, code_hash,
		 model_name, prompt_version, provider_response_id, generation_attempt)
		SELECT $1, recommendations.id, $3,$4,$5,$6,$7,$8,$9,$10
		FROM test_recommendations AS recommendations
		WHERE recommendations.id=$2 AND recommendations.analysis_job_id=$1`
	for _, item := range generated {
		testNames, err := json.Marshal(item.TestNames)
		if err != nil {
			return fmt.Errorf("encode generated test names: %w", err)
		}
		result, err := tx.Exec(ctx, insert, claimed.ID, item.RecommendationID, item.FilePath,
			testNames, item.Code, item.CodeHash, item.ModelName, item.PromptVersion,
			item.ProviderResponseID, item.GenerationAttempt)
		if err != nil {
			return fmt.Errorf("insert generated test for recommendation %d: %w", item.RecommendationID, err)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("recommendation %d does not belong to analysis %d", item.RecommendationID, claimed.ID)
		}
	}
	result, err := tx.Exec(ctx, `UPDATE analysis_jobs SET status=$2, error_message=NULL,
		lease_expires_at=NULL, next_attempt_at=NOW(), attempt_count=0
		WHERE id=$1 AND status=$3 AND attempt_count=$4`, claimed.ID, job.StatusValidating,
		job.StatusGeneratingTests, claimed.AttemptCount)
	if err != nil {
		return fmt.Errorf("advance generated analysis: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit generated tests: %w", err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, analysisID int64) ([]GeneratedTest, error) {
	const query = `SELECT id, analysis_job_id, recommendation_id, file_path, test_names,
		code, code_hash, model_name, prompt_version, provider_response_id,
		generation_attempt, created_at, updated_at
		FROM generated_tests WHERE analysis_job_id=$1 ORDER BY created_at, id`
	return r.list(ctx, query, analysisID)
}

func (r *Repository) ListLatest(ctx context.Context, analysisID int64) ([]GeneratedTest, error) {
	const query = `SELECT id, analysis_job_id, recommendation_id, file_path, test_names,
		code, code_hash, model_name, prompt_version, provider_response_id,
		generation_attempt, created_at, updated_at
		FROM (
			SELECT generated.*, ROW_NUMBER() OVER (
				PARTITION BY recommendation_id
				ORDER BY generation_attempt DESC, id DESC
			) AS version_rank
			FROM generated_tests AS generated WHERE analysis_job_id=$1
		) AS versions
		WHERE version_rank=1 ORDER BY recommendation_id, id`
	return r.list(ctx, query, analysisID)
}

func (r *Repository) list(ctx context.Context, query string, analysisID int64) ([]GeneratedTest, error) {
	rows, err := r.pool.Query(ctx, query, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list generated tests: %w", err)
	}
	defer rows.Close()
	results := make([]GeneratedTest, 0)
	for rows.Next() {
		var item GeneratedTest
		var testNames []byte
		if err := rows.Scan(&item.ID, &item.AnalysisJobID, &item.RecommendationID,
			&item.FilePath, &testNames, &item.Code, &item.CodeHash, &item.ModelName,
			&item.PromptVersion, &item.ProviderResponseID, &item.GenerationAttempt,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan generated test: %w", err)
		}
		if err := json.Unmarshal(testNames, &item.TestNames); err != nil {
			return nil, fmt.Errorf("decode generated test names: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate generated tests: %w", err)
	}
	return results, nil
}

func analysisDestinations(item *job.AnalysisJob) []any {
	return []any{&item.ID, &item.ProjectID, &item.MergeRequestIID, &item.SourceSHA,
		&item.TargetSHA, &item.SourceBranch, &item.TargetBranch, &item.Title, &item.WebURL,
		&item.Status, &item.ErrorMessage, &item.WebhookUUID, &item.AttemptCount, &item.RawEvent,
		&item.StartedAt, &item.FinishedAt, &item.CreatedAt}
}
