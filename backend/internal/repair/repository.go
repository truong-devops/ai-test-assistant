package repair

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
	if err := r.pool.QueryRow(ctx, query, job.StatusRepairing, leaseDuration.String()).
		Scan(analysisDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return job.AnalysisJob{}, job.ErrNotFound
		}
		return job.AnalysisJob{}, fmt.Errorf("claim repair job: %w", err)
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
		job.StatusRepairing, job.StatusFailed, retryDelay.String(), job.StatusRepairing, expectedAttempt)
	if err != nil {
		return fmt.Errorf("retry or fail repair job: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	return nil
}

func (r *Repository) RenewLease(ctx context.Context, claimed job.AnalysisJob, leaseDuration time.Duration) error {
	result, err := r.pool.Exec(ctx, `UPDATE analysis_jobs SET lease_expires_at=NOW()+$3::interval
		WHERE id=$1 AND status=$4 AND attempt_count=$2`, claimed.ID, claimed.AttemptCount,
		leaseDuration.String(), job.StatusRepairing)
	if err != nil {
		return fmt.Errorf("renew repair lease: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	return nil
}

func (r *Repository) SaveRepairs(ctx context.Context, claimed job.AnalysisJob,
	repairs []ProposedRepair,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save repair attempts: %w", err)
	}
	defer tx.Rollback(ctx)

	const insertGenerated = `INSERT INTO generated_tests
		(analysis_job_id, recommendation_id, file_path, test_names, code, code_hash,
		 model_name, prompt_version, provider_response_id, generation_attempt)
		SELECT $1, source.recommendation_id, $4,$5,$6,$7,$8,$9,$10,$11
		FROM generated_tests AS source
		JOIN validation_runs AS validation
		  ON validation.id=$3 AND validation.generated_test_id=source.id
		WHERE source.id=$2 AND source.analysis_job_id=$1
		  AND validation.analysis_job_id=$1
		  AND validation.status IN ('FAILED', 'TIMED_OUT')
		  AND source.file_path=$4
		  AND source.generation_attempt=$12
		  AND source.generation_attempt+1=$11
		  AND source.code_hash<>$7
		  AND NOT EXISTS (
			SELECT 1 FROM generated_tests AS newer
			WHERE newer.recommendation_id=source.recommendation_id
			  AND newer.generation_attempt>source.generation_attempt
		  )
		RETURNING id`
	const insertAttempt = `INSERT INTO repair_attempts
		(analysis_job_id, generated_test_id, validation_run_id, repaired_generated_test_id,
		 attempt_number, previous_code, repaired_code, previous_code_hash, repaired_code_hash,
		 model_name, prompt_version, provider_response_id, reason)
		SELECT $1, source.id, validation.id, repaired.id, $5,
			source.code, repaired.code, source.code_hash, repaired.code_hash,
			repaired.model_name, repaired.prompt_version, repaired.provider_response_id, $6
		FROM generated_tests AS source
		JOIN validation_runs AS validation ON validation.id=$3 AND validation.generated_test_id=source.id
		JOIN generated_tests AS repaired ON repaired.id=$4
		WHERE source.id=$2 AND source.analysis_job_id=$1
		  AND validation.analysis_job_id=$1 AND repaired.analysis_job_id=$1
		  AND repaired.recommendation_id=source.recommendation_id
		  AND repaired.generation_attempt=source.generation_attempt+1
		  AND $5=source.generation_attempt`
	for _, proposed := range repairs {
		testNames, err := json.Marshal(proposed.Generated.TestNames)
		if err != nil {
			return fmt.Errorf("encode repaired test names: %w", err)
		}
		var repairedGeneratedID int64
		err = tx.QueryRow(ctx, insertGenerated, claimed.ID, proposed.SourceGeneratedTestID,
			proposed.ValidationRunID, proposed.Generated.FilePath, testNames,
			proposed.Generated.Code, proposed.Generated.CodeHash, proposed.Generated.ModelName,
			proposed.Generated.PromptVersion, proposed.Generated.ProviderResponseID,
			proposed.Generated.GenerationAttempt, proposed.AttemptNumber).Scan(&repairedGeneratedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("repair source %d is stale or does not belong to analysis %d",
				proposed.SourceGeneratedTestID, claimed.ID)
		}
		if err != nil {
			return fmt.Errorf("insert repaired generated test for source %d: %w",
				proposed.SourceGeneratedTestID, err)
		}
		result, err := tx.Exec(ctx, insertAttempt, claimed.ID, proposed.SourceGeneratedTestID,
			proposed.ValidationRunID, repairedGeneratedID, proposed.AttemptNumber, proposed.Reason)
		if err != nil {
			return fmt.Errorf("insert repair attempt for source %d: %w", proposed.SourceGeneratedTestID, err)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("could not link repair attempt for source %d", proposed.SourceGeneratedTestID)
		}
	}
	nextStatus := job.StatusValidating
	finishedAt := false
	if len(repairs) == 0 {
		nextStatus = job.StatusWaitingReview
		finishedAt = true
	}
	result, err := tx.Exec(ctx, `UPDATE analysis_jobs SET status=$2, error_message=NULL,
		lease_expires_at=NULL, next_attempt_at=NOW(), attempt_count=0,
		finished_at=CASE WHEN $5 THEN NOW() ELSE NULL END
		WHERE id=$1 AND status=$3 AND attempt_count=$4`, claimed.ID, nextStatus,
		job.StatusRepairing, claimed.AttemptCount, finishedAt)
	if err != nil {
		return fmt.Errorf("advance repaired analysis: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repair attempts: %w", err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, analysisID int64) ([]Attempt, error) {
	const query = `SELECT id, analysis_job_id, generated_test_id, validation_run_id,
		repaired_generated_test_id, attempt_number, previous_code, repaired_code,
		previous_code_hash, repaired_code_hash, model_name, prompt_version,
		provider_response_id, reason, created_at
		FROM repair_attempts WHERE analysis_job_id=$1 ORDER BY created_at, id`
	rows, err := r.pool.Query(ctx, query, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list repair attempts: %w", err)
	}
	defer rows.Close()
	results := make([]Attempt, 0)
	for rows.Next() {
		var item Attempt
		if err := rows.Scan(&item.ID, &item.AnalysisJobID, &item.GeneratedTestID,
			&item.ValidationRunID, &item.RepairedGeneratedTestID, &item.AttemptNumber,
			&item.PreviousCode, &item.RepairedCode, &item.PreviousCodeHash,
			&item.RepairedCodeHash, &item.ModelName, &item.PromptVersion,
			&item.ProviderResponseID, &item.Reason, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan repair attempt: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repair attempts: %w", err)
	}
	return results, nil
}

func analysisDestinations(item *job.AnalysisJob) []any {
	return []any{&item.ID, &item.ProjectID, &item.MergeRequestIID, &item.SourceSHA,
		&item.TargetSHA, &item.SourceBranch, &item.TargetBranch, &item.Title, &item.WebURL,
		&item.Status, &item.ErrorMessage, &item.WebhookUUID, &item.AttemptCount, &item.RawEvent,
		&item.StartedAt, &item.FinishedAt, &item.CreatedAt}
}
