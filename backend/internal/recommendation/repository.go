package recommendation

import (
	"context"
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
			WHERE (status=$1 AND next_attempt_at<=NOW() AND lease_expires_at IS NULL)
			   OR (status=$2 AND lease_expires_at<=NOW())
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE analysis_jobs AS jobs
		SET status=$2, started_at=COALESCE(started_at, NOW()), error_message=NULL,
			attempt_count=CASE
				WHEN jobs.lease_expires_at IS NULL AND jobs.error_message IS NULL THEN 1
				ELSE jobs.attempt_count + 1
			END,
			lease_expires_at=NOW()+$3::interval
		FROM next_job WHERE jobs.id=next_job.id
		RETURNING jobs.id, jobs.project_id, jobs.merge_request_iid, jobs.source_sha,
			jobs.target_sha, jobs.source_branch, jobs.target_branch, jobs.title, jobs.web_url,
			jobs.status, COALESCE(jobs.error_message, ''), jobs.webhook_uuid, jobs.attempt_count,
			jobs.raw_event, jobs.started_at, jobs.finished_at, jobs.created_at`
	var result job.AnalysisJob
	if err := r.pool.QueryRow(ctx, query, job.StatusRetrievingContext, job.StatusRecommendingTests,
		leaseDuration.String()).Scan(analysisDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return job.AnalysisJob{}, job.ErrNotFound
		}
		return job.AnalysisJob{}, fmt.Errorf("claim recommendation job: %w", err)
	}
	return result, nil
}

func (r *Repository) RetryOrFail(ctx context.Context, id int64, expectedAttempt int, processErr error,
	maxAttempts int, retryDelay time.Duration,
) error {
	const query = `
		UPDATE analysis_jobs SET
			status=CASE WHEN attempt_count<$3 THEN $4 ELSE $5 END,
			error_message=$2,
			next_attempt_at=CASE WHEN attempt_count<$3 THEN NOW()+$6::interval ELSE next_attempt_at END,
			lease_expires_at=NULL,
			finished_at=CASE WHEN attempt_count<$3 THEN NULL ELSE NOW() END
		WHERE id=$1 AND status=$7 AND attempt_count=$8`
	result, err := r.pool.Exec(ctx, query, id, processErr.Error(), maxAttempts,
		job.StatusRetrievingContext, job.StatusFailed, retryDelay.String(),
		job.StatusRecommendingTests, expectedAttempt)
	if err != nil {
		return fmt.Errorf("retry or fail recommendation job: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	return nil
}

func (r *Repository) RenewLease(ctx context.Context, claimed job.AnalysisJob, leaseDuration time.Duration) error {
	result, err := r.pool.Exec(ctx, `UPDATE analysis_jobs SET lease_expires_at=NOW()+$3::interval
		WHERE id=$1 AND status=$4 AND attempt_count=$2`, claimed.ID, claimed.AttemptCount,
		leaseDuration.String(), job.StatusRecommendingTests)
	if err != nil {
		return fmt.Errorf("renew recommendation lease: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	return nil
}

func (r *Repository) Save(ctx context.Context, claimed job.AnalysisJob, recommendations []Recommendation) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save recommendations: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM test_recommendations WHERE analysis_job_id=$1`, claimed.ID); err != nil {
		return fmt.Errorf("clear stale recommendations: %w", err)
	}
	const insert = `
		INSERT INTO test_recommendations
			(analysis_job_id, changed_symbol_id, title, description, priority, rationale,
			 scenario, expected_behavior, status, model_name, prompt_version, provider_response_id)
		SELECT $1, symbols.id, $3,$4,$5,$6,$7,$8,$9,$10,$11,$12
		FROM changed_symbols AS symbols
		JOIN changed_files AS files ON files.id=symbols.changed_file_id
		WHERE symbols.id=$2 AND files.analysis_job_id=$1`
	for _, item := range recommendations {
		result, err := tx.Exec(ctx, insert, claimed.ID, item.ChangedSymbolID, item.Title,
			item.Description, item.Priority, item.Rationale, item.Scenario, item.ExpectedBehavior,
			StatusPending, item.ModelName, item.PromptVersion, item.ProviderResponseID)
		if err != nil {
			return fmt.Errorf("insert recommendation %q: %w", item.Title, err)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("changed symbol %d does not belong to analysis %d", item.ChangedSymbolID, claimed.ID)
		}
	}
	result, err := tx.Exec(ctx, `UPDATE analysis_jobs SET status=$2, error_message=NULL,
		lease_expires_at=NULL, next_attempt_at=NOW(), attempt_count=0
		WHERE id=$1 AND status=$3 AND attempt_count=$4`, claimed.ID, job.StatusGeneratingTests,
		job.StatusRecommendingTests, claimed.AttemptCount)
	if err != nil {
		return fmt.Errorf("advance recommended analysis: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit recommendations: %w", err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, analysisID int64) ([]Recommendation, error) {
	const query = `SELECT id, analysis_job_id, changed_symbol_id, title, description, priority,
		rationale, scenario, expected_behavior, status, model_name, prompt_version,
		provider_response_id, created_at, updated_at
		FROM test_recommendations WHERE analysis_job_id=$1 ORDER BY created_at, id`
	rows, err := r.pool.Query(ctx, query, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list recommendations: %w", err)
	}
	defer rows.Close()
	results := make([]Recommendation, 0)
	for rows.Next() {
		var item Recommendation
		if err := rows.Scan(&item.ID, &item.AnalysisJobID, &item.ChangedSymbolID, &item.Title,
			&item.Description, &item.Priority, &item.Rationale, &item.Scenario,
			&item.ExpectedBehavior, &item.Status, &item.ModelName, &item.PromptVersion,
			&item.ProviderResponseID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan recommendation: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recommendations: %w", err)
	}
	return results, nil
}

func analysisDestinations(item *job.AnalysisJob) []any {
	return []any{&item.ID, &item.ProjectID, &item.MergeRequestIID, &item.SourceSHA,
		&item.TargetSHA, &item.SourceBranch, &item.TargetBranch, &item.Title, &item.WebURL,
		&item.Status, &item.ErrorMessage, &item.WebhookUUID, &item.AttemptCount, &item.RawEvent,
		&item.StartedAt, &item.FinishedAt, &item.CreatedAt}
}
