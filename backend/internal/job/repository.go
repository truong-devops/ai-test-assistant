package job

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("analysis job not found")
	ErrLeaseLost = errors.New("analysis job lease lost")
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Enqueue(ctx context.Context, input EnqueueInput) (AnalysisJob, bool, error) {
	const insert = `
		INSERT INTO analysis_jobs
			(project_id, merge_request_iid, source_sha, target_sha, status, webhook_uuid, raw_event)
		VALUES ($1, $2, $3, '', $4, $5, $6)
		ON CONFLICT DO NOTHING
		RETURNING id, project_id, merge_request_iid, source_sha, target_sha, source_branch,
			target_branch, title, web_url, status, COALESCE(error_message, ''), webhook_uuid,
			attempt_count, raw_event, started_at, finished_at, created_at`
	var result AnalysisJob
	err := r.pool.QueryRow(ctx, insert, input.ProjectID, input.MergeRequestIID, input.SourceSHA,
		StatusPending, input.WebhookUUID, input.RawEvent).Scan(jobDestinations(&result)...)
	if err == nil {
		return result, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AnalysisJob{}, false, fmt.Errorf("enqueue analysis job: %w", err)
	}

	result, err = r.getByWebhookUUID(ctx, input.WebhookUUID)
	if err != nil {
		return AnalysisJob{}, false, fmt.Errorf("get duplicate analysis job: %w", err)
	}
	return result, false, nil
}

func (r *Repository) ClaimNext(ctx context.Context, leaseDuration time.Duration) (AnalysisJob, error) {
	return r.claimNext(ctx, StatusPending, StatusFetchingSource, leaseDuration)
}

func (r *Repository) claimNext(ctx context.Context, waitingStatus, activeStatus string, leaseDuration time.Duration) (AnalysisJob, error) {
	const query = `
		WITH next_job AS (
			SELECT id FROM analysis_jobs
			WHERE (status = $1 AND next_attempt_at <= NOW() AND lease_expires_at IS NULL)
			   OR (status = $2 AND lease_expires_at <= NOW())
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE analysis_jobs AS jobs
		SET status = $2, started_at = COALESCE(started_at, NOW()), error_message = NULL,
			attempt_count = attempt_count + 1, lease_expires_at = NOW() + $3::interval
		FROM next_job WHERE jobs.id = next_job.id
		RETURNING jobs.id, jobs.project_id, jobs.merge_request_iid, jobs.source_sha,
			jobs.target_sha, jobs.source_branch, jobs.target_branch, jobs.title, jobs.web_url,
			jobs.status, COALESCE(jobs.error_message, ''), jobs.webhook_uuid, jobs.attempt_count, jobs.raw_event,
			jobs.started_at, jobs.finished_at, jobs.created_at`
	var result AnalysisJob
	if err := r.pool.QueryRow(ctx, query, waitingStatus, activeStatus, leaseDuration.String()).Scan(jobDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AnalysisJob{}, ErrNotFound
		}
		return AnalysisJob{}, fmt.Errorf("claim analysis job: %w", err)
	}
	return result, nil
}

func (r *Repository) SaveFetched(ctx context.Context, id int64, expectedAttempt int, metadata MergeRequestMetadata, files []ChangedFile) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save fetched analysis: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM changed_files WHERE analysis_job_id = $1`, id); err != nil {
		return fmt.Errorf("clear changed files: %w", err)
	}
	const insertFile = `
		INSERT INTO changed_files
			(analysis_job_id, old_path, new_path, change_type, additions, deletions, diff,
			 new_file, renamed_file, deleted_file, collapsed, too_large)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`
	for _, file := range files {
		if _, err := tx.Exec(ctx, insertFile, id, file.OldPath, file.NewPath, file.ChangeType,
			file.Additions, file.Deletions, file.Diff, file.NewFile, file.RenamedFile,
			file.DeletedFile, file.Collapsed, file.TooLarge); err != nil {
			return fmt.Errorf("insert changed file %q: %w", file.NewPath, err)
		}
	}

	const updateJob = `
		UPDATE analysis_jobs SET source_sha=$2, target_sha=$3, source_branch=$4,
			target_branch=$5, title=$6, web_url=$7, status=$8, error_message=NULL,
			lease_expires_at=NULL, next_attempt_at=NOW(), attempt_count=0
		WHERE id=$1 AND status=$9 AND attempt_count=$10`
	result, err := tx.Exec(ctx, updateJob, id, metadata.SourceSHA, metadata.TargetSHA,
		metadata.SourceBranch, metadata.TargetBranch, metadata.Title, metadata.WebURL, StatusAnalyzingChange,
		StatusFetchingSource, expectedAttempt)
	if err != nil {
		return fmt.Errorf("update fetched analysis job: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit fetched analysis: %w", err)
	}
	return nil
}

func (r *Repository) RetryOrFail(ctx context.Context, id int64, expectedAttempt int, processErr error, maxAttempts int, retryDelay time.Duration) error {
	return r.retryOrFail(ctx, id, expectedAttempt, processErr, maxAttempts, retryDelay,
		StatusFetchingSource, StatusPending)
}

func (r *Repository) retryOrFail(ctx context.Context, id int64, expectedAttempt int, processErr error,
	maxAttempts int, retryDelay time.Duration, activeStatus, retryStatus string,
) error {
	const query = `
		UPDATE analysis_jobs SET
			status = CASE WHEN attempt_count < $3 THEN $4 ELSE $5 END,
			error_message = $2,
			next_attempt_at = CASE WHEN attempt_count < $3 THEN NOW() + $6::interval ELSE next_attempt_at END,
			lease_expires_at = NULL,
			finished_at = CASE WHEN attempt_count < $3 THEN NULL ELSE NOW() END
		WHERE id = $1 AND status = $7 AND attempt_count = $8`
	result, err := r.pool.Exec(ctx, query, id, processErr.Error(), maxAttempts, retryStatus,
		StatusFailed, retryDelay.String(), activeStatus, expectedAttempt)
	if err != nil {
		return fmt.Errorf("retry or fail analysis job: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

type changeQueue struct{ repository *Repository }

func NewChangeQueue(repository *Repository) Queue {
	return &changeQueue{repository: repository}
}

func (q *changeQueue) ClaimNext(ctx context.Context, leaseDuration time.Duration) (AnalysisJob, error) {
	return q.repository.claimNext(ctx, StatusAnalyzingChange, StatusAnalyzingChange, leaseDuration)
}

func (q *changeQueue) RetryOrFail(ctx context.Context, id int64, expectedAttempt int, processErr error,
	maxAttempts int, retryDelay time.Duration,
) error {
	return q.repository.retryOrFail(ctx, id, expectedAttempt, processErr, maxAttempts, retryDelay,
		StatusAnalyzingChange, StatusAnalyzingChange)
}

func (r *Repository) SaveSymbols(ctx context.Context, analysisID int64, expectedAttempt int, symbols []ChangedSymbol) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save changed symbols: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM changed_symbols WHERE changed_file_id IN
		(SELECT id FROM changed_files WHERE analysis_job_id=$1)`, analysisID); err != nil {
		return fmt.Errorf("clear changed symbols: %w", err)
	}
	const insert = `
		INSERT INTO changed_symbols
			(changed_file_id, symbol_name, symbol_kind, receiver_name, package_name,
			 start_line, end_line, change_type, change_summary)
		SELECT id, $3, $4, $5, $6, $7, $8, $9, $10
		FROM changed_files WHERE id=$1 AND analysis_job_id=$2`
	for _, symbol := range symbols {
		result, err := tx.Exec(ctx, insert, symbol.ChangedFileID, analysisID, symbol.SymbolName,
			symbol.SymbolKind, symbol.ReceiverName, symbol.PackageName, symbol.StartLine,
			symbol.EndLine, symbol.ChangeType, symbol.ChangeSummary)
		if err != nil {
			return fmt.Errorf("insert changed symbol %q: %w", symbol.SymbolName, err)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("changed file %d does not belong to analysis %d", symbol.ChangedFileID, analysisID)
		}
	}
	result, err := tx.Exec(ctx, `UPDATE analysis_jobs SET status=$2, error_message=NULL,
		lease_expires_at=NULL, next_attempt_at=NOW(), attempt_count=0
		WHERE id=$1 AND status=$3 AND attempt_count=$4`, analysisID, StatusRetrievingContext,
		StatusAnalyzingChange, expectedAttempt)
	if err != nil {
		return fmt.Errorf("update analyzed job: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit changed symbols: %w", err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context) ([]AnalysisJob, error) {
	const query = `
		SELECT id, project_id, merge_request_iid, source_sha, target_sha, source_branch,
			target_branch, title, web_url, status, COALESCE(error_message, ''), webhook_uuid,
			attempt_count, raw_event, started_at, finished_at, created_at
		FROM analysis_jobs ORDER BY created_at DESC, id DESC`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list analysis jobs: %w", err)
	}
	defer rows.Close()
	results := make([]AnalysisJob, 0)
	for rows.Next() {
		var item AnalysisJob
		if err := rows.Scan(jobDestinations(&item)...); err != nil {
			return nil, fmt.Errorf("scan analysis job: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analysis jobs: %w", err)
	}
	return results, nil
}

func (r *Repository) Get(ctx context.Context, id int64) (AnalysisJob, []ChangedFile, error) {
	const query = `
		SELECT id, project_id, merge_request_iid, source_sha, target_sha, source_branch,
			target_branch, title, web_url, status, COALESCE(error_message, ''), webhook_uuid,
			attempt_count, raw_event, started_at, finished_at, created_at
		FROM analysis_jobs WHERE id=$1`
	var result AnalysisJob
	if err := r.pool.QueryRow(ctx, query, id).Scan(jobDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AnalysisJob{}, nil, ErrNotFound
		}
		return AnalysisJob{}, nil, fmt.Errorf("get analysis job: %w", err)
	}
	files, err := r.listChangedFiles(ctx, id)
	if err != nil {
		return AnalysisJob{}, nil, err
	}
	return result, files, nil
}

func (r *Repository) getByWebhookUUID(ctx context.Context, uuid string) (AnalysisJob, error) {
	const query = `
		SELECT id, project_id, merge_request_iid, source_sha, target_sha, source_branch,
			target_branch, title, web_url, status, COALESCE(error_message, ''), webhook_uuid,
			attempt_count, raw_event, started_at, finished_at, created_at
		FROM analysis_jobs WHERE webhook_uuid=$1`
	var result AnalysisJob
	if err := r.pool.QueryRow(ctx, query, uuid).Scan(jobDestinations(&result)...); err != nil {
		return AnalysisJob{}, err
	}
	return result, nil
}

func (r *Repository) listChangedFiles(ctx context.Context, id int64) ([]ChangedFile, error) {
	const query = `
		SELECT id, analysis_job_id, old_path, new_path, change_type, additions, deletions,
			diff, new_file, renamed_file, deleted_file, collapsed, too_large
		FROM changed_files WHERE analysis_job_id=$1 ORDER BY id`
	rows, err := r.pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("list changed files: %w", err)
	}
	defer rows.Close()
	results := make([]ChangedFile, 0)
	for rows.Next() {
		var item ChangedFile
		if err := rows.Scan(&item.ID, &item.AnalysisJobID, &item.OldPath, &item.NewPath,
			&item.ChangeType, &item.Additions, &item.Deletions, &item.Diff, &item.NewFile,
			&item.RenamedFile, &item.DeletedFile, &item.Collapsed, &item.TooLarge); err != nil {
			return nil, fmt.Errorf("scan changed file: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *Repository) ListChangedSymbols(ctx context.Context, analysisID int64) ([]ChangedSymbol, error) {
	const query = `
		SELECT symbols.id, symbols.changed_file_id, symbols.symbol_name, symbols.symbol_kind,
			symbols.receiver_name, symbols.package_name, symbols.start_line, symbols.end_line,
			symbols.change_type, symbols.change_summary
		FROM changed_symbols AS symbols
		JOIN changed_files AS files ON files.id=symbols.changed_file_id
		WHERE files.analysis_job_id=$1
		ORDER BY files.id, symbols.start_line, symbols.symbol_name`
	rows, err := r.pool.Query(ctx, query, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list changed symbols: %w", err)
	}
	defer rows.Close()
	results := make([]ChangedSymbol, 0)
	for rows.Next() {
		var item ChangedSymbol
		if err := rows.Scan(&item.ID, &item.ChangedFileID, &item.SymbolName, &item.SymbolKind,
			&item.ReceiverName, &item.PackageName, &item.StartLine, &item.EndLine,
			&item.ChangeType, &item.ChangeSummary); err != nil {
			return nil, fmt.Errorf("scan changed symbol: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate changed symbols: %w", err)
	}
	return results, nil
}

func jobDestinations(item *AnalysisJob) []any {
	return []any{&item.ID, &item.ProjectID, &item.MergeRequestIID, &item.SourceSHA,
		&item.TargetSHA, &item.SourceBranch, &item.TargetBranch, &item.Title, &item.WebURL,
		&item.Status, &item.ErrorMessage, &item.WebhookUUID, &item.AttemptCount, &item.RawEvent, &item.StartedAt,
		&item.FinishedAt, &item.CreatedAt}
}
