package validation

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
	if err := r.pool.QueryRow(ctx, query, job.StatusValidating, leaseDuration.String()).
		Scan(analysisDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return job.AnalysisJob{}, job.ErrNotFound
		}
		return job.AnalysisJob{}, fmt.Errorf("claim validation job: %w", err)
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
		job.StatusValidating, job.StatusFailed, retryDelay.String(), job.StatusValidating, expectedAttempt)
	if err != nil {
		return fmt.Errorf("retry or fail validation job: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	return nil
}

func (r *Repository) RenewLease(ctx context.Context, claimed job.AnalysisJob, leaseDuration time.Duration) error {
	result, err := r.pool.Exec(ctx, `UPDATE analysis_jobs SET lease_expires_at=NOW()+$3::interval
		WHERE id=$1 AND status=$4 AND attempt_count=$2`, claimed.ID, claimed.AttemptCount,
		leaseDuration.String(), job.StatusValidating)
	if err != nil {
		return fmt.Errorf("renew validation lease: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	return nil
}

func (r *Repository) Save(ctx context.Context, claimed job.AnalysisJob, runs []Run) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save validation runs: %w", err)
	}
	defer tx.Rollback(ctx)

	const insert = `INSERT INTO validation_runs
		(analysis_job_id, generated_test_id, attempt_number, command, status, exit_code,
		 duration_ms, stdout, stderr, output_truncated)
		SELECT $1, generated.id, $3,$4,$5,$6,$7,$8,$9,$10
		FROM generated_tests AS generated
		WHERE generated.id=$2 AND generated.analysis_job_id=$1
		ON CONFLICT (generated_test_id, attempt_number) DO UPDATE SET
			command=EXCLUDED.command, status=EXCLUDED.status, exit_code=EXCLUDED.exit_code,
			duration_ms=EXCLUDED.duration_ms, stdout=EXCLUDED.stdout, stderr=EXCLUDED.stderr,
			output_truncated=EXCLUDED.output_truncated`
	allPassed := true
	for _, run := range runs {
		result, err := tx.Exec(ctx, insert, claimed.ID, run.GeneratedTestID, run.AttemptNumber,
			run.Command, run.Status, run.ExitCode, run.DurationMS, run.Stdout, run.Stderr,
			run.OutputTruncated)
		if err != nil {
			return fmt.Errorf("insert validation for generated test %d: %w", run.GeneratedTestID, err)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("generated test %d does not belong to analysis %d", run.GeneratedTestID, claimed.ID)
		}
		if run.Status != StatusPassed {
			allPassed = false
		}
	}
	nextStatus := job.StatusRepairing
	if allPassed {
		nextStatus = job.StatusWaitingReview
	}
	result, err := tx.Exec(ctx, `UPDATE analysis_jobs SET status=$2, error_message=NULL,
		lease_expires_at=NULL, next_attempt_at=NOW(), attempt_count=0,
		finished_at=CASE WHEN $2=$5 THEN NOW() ELSE NULL END
		WHERE id=$1 AND status=$3 AND attempt_count=$4`, claimed.ID, nextStatus,
		job.StatusValidating, claimed.AttemptCount, job.StatusWaitingReview)
	if err != nil {
		return fmt.Errorf("advance validated analysis: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit validation runs: %w", err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, analysisID int64) ([]Run, error) {
	const query = `SELECT id, analysis_job_id, generated_test_id, attempt_number, command,
		status, exit_code, duration_ms, stdout, stderr, output_truncated, created_at
		FROM validation_runs WHERE analysis_job_id=$1 ORDER BY created_at, id`
	rows, err := r.pool.Query(ctx, query, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list validation runs: %w", err)
	}
	defer rows.Close()
	results := make([]Run, 0)
	for rows.Next() {
		var item Run
		if err := rows.Scan(&item.ID, &item.AnalysisJobID, &item.GeneratedTestID,
			&item.AttemptNumber, &item.Command, &item.Status, &item.ExitCode,
			&item.DurationMS, &item.Stdout, &item.Stderr, &item.OutputTruncated,
			&item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan validation run: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate validation runs: %w", err)
	}
	return results, nil
}

func analysisDestinations(item *job.AnalysisJob) []any {
	return []any{&item.ID, &item.ProjectID, &item.MergeRequestIID, &item.SourceSHA,
		&item.TargetSHA, &item.SourceBranch, &item.TargetBranch, &item.Title, &item.WebURL,
		&item.Status, &item.ErrorMessage, &item.WebhookUUID, &item.AttemptCount, &item.RawEvent,
		&item.StartedAt, &item.FinishedAt, &item.CreatedAt}
}
