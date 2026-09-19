package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *PostgresRepository { return &PostgresRepository{pool: pool} }

func (r *PostgresRepository) CreateSet(ctx context.Context, input CreateSetInput) (Set, error) {
	const query = `INSERT INTO document_sets (name, product_name, scope, description)
		VALUES ($1,$2,$3,$4)
		RETURNING id, name, product_name, scope, description, status, retention_days,
		ai_token_budget, ai_cost_budget_microusd, archived_at, created_at, updated_at, source_revision`
	var result Set
	if err := r.pool.QueryRow(ctx, query, input.Name, input.ProductName, input.Scope, input.Description).Scan(
		&result.ID, &result.Name, &result.ProductName, &result.Scope, &result.Description, &result.Status,
		&result.RetentionDays, &result.AITokenBudget, &result.AICostBudgetMicroUSD,
		&result.ArchivedAt, &result.CreatedAt, &result.UpdatedAt, &result.SourceRevision); err != nil {
		if uniqueViolation(err) {
			return Set{}, ErrAlreadyExists
		}
		return Set{}, fmt.Errorf("create document set: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListSets(ctx context.Context) ([]Set, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, product_name, scope, description, status, retention_days,
		ai_token_budget, ai_cost_budget_microusd, archived_at, created_at, updated_at, source_revision
		FROM document_sets ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list document sets: %w", err)
	}
	defer rows.Close()
	results := make([]Set, 0)
	for rows.Next() {
		var item Set
		if err := rows.Scan(&item.ID, &item.Name, &item.ProductName, &item.Scope, &item.Description, &item.Status,
			&item.RetentionDays, &item.AITokenBudget, &item.AICostBudgetMicroUSD,
			&item.ArchivedAt, &item.CreatedAt, &item.UpdatedAt, &item.SourceRevision); err != nil {
			return nil, fmt.Errorf("scan document set: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document sets: %w", err)
	}
	return results, nil
}

func (r *PostgresRepository) GetSet(ctx context.Context, id int64) (Set, error) {
	var result Set
	err := r.pool.QueryRow(ctx, `SELECT id, name, product_name, scope, description, status, retention_days,
		ai_token_budget, ai_cost_budget_microusd, archived_at, created_at, updated_at, source_revision
		FROM document_sets WHERE id=$1`, id).Scan(&result.ID, &result.Name,
		&result.ProductName, &result.Scope, &result.Description, &result.Status, &result.RetentionDays,
		&result.AITokenBudget, &result.AICostBudgetMicroUSD,
		&result.ArchivedAt, &result.CreatedAt, &result.UpdatedAt, &result.SourceRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return Set{}, ErrNotFound
	}
	if err != nil {
		return Set{}, fmt.Errorf("get document set: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) UpdateLifecycle(ctx context.Context, id int64, input LifecycleInput) (Set, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Set{}, err
	}
	defer tx.Rollback(ctx)
	var previousStatus string
	var previousRetention int
	var previousTokenBudget, previousCostBudget int64
	if err = tx.QueryRow(ctx, `SELECT status,retention_days,ai_token_budget,ai_cost_budget_microusd
		FROM document_sets WHERE id=$1 FOR UPDATE`, id).Scan(&previousStatus, &previousRetention,
		&previousTokenBudget, &previousCostBudget); errors.Is(err, pgx.ErrNoRows) {
		return Set{}, ErrNotFound
	} else if err != nil {
		return Set{}, err
	}
	if previousStatus == SetStatusPurging {
		return Set{}, fmt.Errorf("%w: purge is already in progress", ErrInvalidInput)
	}
	if input.AITokenBudget == 0 {
		input.AITokenBudget = previousTokenBudget
	}
	if input.AICostBudgetMicroUSD == 0 {
		input.AICostBudgetMicroUSD = previousCostBudget
	}
	action := "RETENTION_CHANGED"
	if input.Status == SetStatusArchived && previousStatus != SetStatusArchived {
		action = "ARCHIVED"
	} else if input.Status == SetStatusActive && previousStatus == SetStatusArchived {
		action = "RESTORED"
	} else if input.AITokenBudget != previousTokenBudget || input.AICostBudgetMicroUSD != previousCostBudget {
		action = "BUDGET_CHANGED"
	}
	if _, err = tx.Exec(ctx, `UPDATE document_sets SET status=$2,retention_days=$3,
		ai_token_budget=$4,ai_cost_budget_microusd=$5,
		archived_at=CASE WHEN $2='ARCHIVED' THEN COALESCE(archived_at,NOW()) ELSE NULL END,updated_at=NOW()
		WHERE id=$1`, id, input.Status, input.RetentionDays, input.AITokenBudget, input.AICostBudgetMicroUSD); err != nil {
		return Set{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO document_set_audit_log(document_set_id,actor,action,reason,before_state,after_state)
		VALUES($1,$2,$3,$4,jsonb_build_object('status',$5::text,'retention_days',$6::integer,
		'ai_token_budget',$7::bigint,'ai_cost_budget_microusd',$8::bigint),
		jsonb_build_object('status',$9::text,'retention_days',$10::integer,
		'ai_token_budget',$11::bigint,'ai_cost_budget_microusd',$12::bigint))`, id, input.Actor, action, input.Reason,
		previousStatus, previousRetention, previousTokenBudget, previousCostBudget,
		input.Status, input.RetentionDays, input.AITokenBudget, input.AICostBudgetMicroUSD); err != nil {
		return Set{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Set{}, err
	}
	return r.GetSet(ctx, id)
}

func (r *PostgresRepository) PurgePreview(ctx context.Context, id int64) (PurgePreview, error) {
	set, err := r.GetSet(ctx, id)
	if err != nil {
		return PurgePreview{}, err
	}
	result := PurgePreview{DocumentSetID: set.ID, Name: set.Name, Status: set.Status,
		RetentionDays: set.RetentionDays, ArchivedAt: set.ArchivedAt,
		Blockers: []string{}, Confirmation: fmt.Sprintf("PURGE DOCUMENT SET %d", set.ID)}
	if set.ArchivedAt != nil {
		eligibleAt := set.ArchivedAt.Add(time.Duration(set.RetentionDays) * 24 * time.Hour)
		result.PurgeEligibleAt = &eligibleAt
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*),COALESCE(sum(size_bytes),0)
		FROM document_versions WHERE document_set_id=$1`, id).Scan(
		&result.StorageObjectCount, &result.StorageBytes); err != nil {
		return PurgePreview{}, fmt.Errorf("summarize document purge: %w", err)
	}
	var projectBaselines, analysisSnapshots, testRuns int
	if err := r.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM project_document_baselines WHERE document_set_id=$1),
		(SELECT count(*) FROM analysis_baseline_snapshots WHERE document_set_id=$1),
		(SELECT count(*) FROM test_runs r JOIN test_suites s ON s.id=r.test_suite_id
		 WHERE s.document_set_id=$1)`, id).
		Scan(&projectBaselines, &analysisSnapshots, &testRuns); err != nil {
		return PurgePreview{}, fmt.Errorf("check document purge references: %w", err)
	}
	if projectBaselines > 0 {
		result.Blockers = append(result.Blockers, fmt.Sprintf("%d project baseline(s) still reference this set", projectBaselines))
	}
	if analysisSnapshots > 0 {
		result.Blockers = append(result.Blockers, fmt.Sprintf("%d immutable analysis snapshot(s) still reference this set", analysisSnapshots))
	}
	if testRuns > 0 {
		result.Blockers = append(result.Blockers, fmt.Sprintf("%d immutable test run(s) still reference this set", testRuns))
	}
	retentionElapsed := result.PurgeEligibleAt != nil && !time.Now().Before(*result.PurgeEligibleAt)
	result.Eligible = (set.Status == SetStatusArchived || set.Status == SetStatusPurging) &&
		retentionElapsed && len(result.Blockers) == 0
	return result, nil
}

func (r *PostgresRepository) BeginPurge(ctx context.Context, id int64, input PurgeInput) (PurgePlan, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PurgePlan{}, err
	}
	defer tx.Rollback(ctx)
	var set Set
	err = tx.QueryRow(ctx, `SELECT id,name,product_name,scope,description,status,retention_days,
		ai_token_budget,ai_cost_budget_microusd,archived_at,created_at,updated_at,source_revision
		FROM document_sets WHERE id=$1 FOR UPDATE`, id).Scan(&set.ID, &set.Name, &set.ProductName,
		&set.Scope, &set.Description, &set.Status, &set.RetentionDays, &set.AITokenBudget,
		&set.AICostBudgetMicroUSD, &set.ArchivedAt, &set.CreatedAt, &set.UpdatedAt, &set.SourceRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurgePlan{}, ErrNotFound
	}
	if err != nil {
		return PurgePlan{}, err
	}
	if input.Confirmation != fmt.Sprintf("PURGE DOCUMENT SET %d", id) {
		return PurgePlan{}, fmt.Errorf("%w: confirmation text does not match", ErrInvalidInput)
	}
	if set.Status != SetStatusArchived && set.Status != SetStatusPurging {
		return PurgePlan{}, fmt.Errorf("%w: archive the document set before purge", ErrInvalidInput)
	}
	if set.ArchivedAt == nil || time.Now().Before(set.ArchivedAt.Add(time.Duration(set.RetentionDays)*24*time.Hour)) {
		return PurgePlan{}, ErrRetentionNotMet
	}
	var projectBaselines, analysisSnapshots, testRuns int
	if err = tx.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM project_document_baselines WHERE document_set_id=$1),
		(SELECT count(*) FROM analysis_baseline_snapshots WHERE document_set_id=$1),
		(SELECT count(*) FROM test_runs r JOIN test_suites s ON s.id=r.test_suite_id
		 WHERE s.document_set_id=$1)`, id).
		Scan(&projectBaselines, &analysisSnapshots, &testRuns); err != nil {
		return PurgePlan{}, err
	}
	if projectBaselines > 0 || analysisSnapshots > 0 || testRuns > 0 {
		return PurgePlan{}, fmt.Errorf("%w: %d project baseline(s), %d analysis snapshot(s), %d test run(s)",
			ErrPurgeBlocked, projectBaselines, analysisSnapshots, testRuns)
	}
	rows, err := tx.Query(ctx, `SELECT storage_key FROM document_versions
		WHERE document_set_id=$1 ORDER BY id`, id)
	if err != nil {
		return PurgePlan{}, err
	}
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return PurgePlan{}, err
		}
		keys = append(keys, key)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return PurgePlan{}, err
	}
	var sizeBytes int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes),0) FROM document_versions
		WHERE document_set_id=$1`, id).Scan(&sizeBytes); err != nil {
		return PurgePlan{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE document_sets SET status='PURGING',updated_at=NOW() WHERE id=$1`, id); err != nil {
		return PurgePlan{}, err
	}
	var auditID int64
	err = tx.QueryRow(ctx, `INSERT INTO document_set_purge_audit
		(document_set_id,document_set_name,actor,reason,confirmation,storage_object_count,
		storage_bytes,database_snapshot)
		VALUES($1,$2,$3,$4,$5,$6,$7,jsonb_build_object(
		'status',$8::text,'retention_days',$9::integer,'archived_at',$10::timestamptz,
		'ai_token_budget',$11::bigint,'ai_cost_budget_microusd',$12::bigint)) RETURNING id`,
		id, set.Name, input.Actor, input.Reason, input.Confirmation, len(keys), sizeBytes,
		set.Status, set.RetentionDays, set.ArchivedAt, set.AITokenBudget, set.AICostBudgetMicroUSD).
		Scan(&auditID)
	if err != nil {
		return PurgePlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PurgePlan{}, err
	}
	return PurgePlan{AuditID: auditID, DocumentSetID: id, StorageKeys: keys}, nil
}

func (r *PostgresRepository) CompletePurge(ctx context.Context, plan PurgePlan) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `DELETE FROM document_sets WHERE id=$1 AND status='PURGING'`, plan.DocumentSetID)
	if err != nil {
		return fmt.Errorf("delete purging document set: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err = tx.Exec(ctx, `UPDATE document_set_purge_audit SET status='COMPLETED',
		completed_at=NOW(),error_message='' WHERE id=$1 AND document_set_id=$2`,
		plan.AuditID, plan.DocumentSetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) FailPurge(ctx context.Context, plan PurgePlan, cause error) error {
	message := "purge failed"
	if cause != nil {
		message = cause.Error()
	}
	_, err := r.pool.Exec(ctx, `UPDATE document_set_purge_audit SET status='FAILED',
		completed_at=NOW(),error_message=$2 WHERE id=$1`, plan.AuditID, message)
	return err
}

func (r *PostgresRepository) Metrics(ctx context.Context) (PipelineMetrics, error) {
	var result PipelineMetrics
	err := r.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM document_sets WHERE status='ACTIVE'),
		(SELECT count(*) FROM document_versions),
		(SELECT count(*) FROM document_versions WHERE parse_status='PARSED'),
		(SELECT count(*) FROM document_versions WHERE parse_status='FAILED'),
		(SELECT count(*) FROM requirements WHERE status='APPROVED'),
		(SELECT count(*) FROM requirement_extraction_jobs),
		(SELECT count(*) FROM requirement_extraction_jobs WHERE status='FAILED'),
		(SELECT count(*) FROM test_suites),
		(SELECT count(*) FROM test_cases WHERE status='APPROVED'),
		(SELECT count(*) FROM automation_artifacts),
		(SELECT count(*) FROM test_runs),
		(SELECT count(DISTINCT test_run_id) FROM test_run_items WHERE status IN
			('PRODUCT_FAILED','AUTOMATION_ERROR','INFRA_ERROR','TIMED_OUT','BLOCKED')),
		(SELECT count(*) FROM requirements WHERE status IN ('DRAFT','CONFLICT','TBD')) +
		(SELECT count(*) FROM test_cases WHERE status='DRAFT') +
		(SELECT count(*) FROM automation_artifacts WHERE status='DRAFT')`).Scan(
		&result.DocumentSets, &result.DocumentVersions, &result.ParsedVersions,
		&result.ParseFailures, &result.ApprovedRequirements, &result.ExtractionJobs,
		&result.ExtractionFailures, &result.TestSuites, &result.ApprovedTestCases,
		&result.AutomationArtifacts, &result.TestRuns, &result.RunsNeedingAttention,
		&result.PendingApprovalActions)
	if err != nil {
		return PipelineMetrics{}, fmt.Errorf("load document pipeline metrics: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) CreateVersion(ctx context.Context, setID int64, input UploadInput,
	stored StoredFile,
) (Document, Version, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Document{}, Version{}, fmt.Errorf("begin document upload: %w", err)
	}
	defer tx.Rollback(ctx)

	var setStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM document_sets WHERE id=$1 FOR UPDATE`, setID).Scan(&setStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Document{}, Version{}, ErrNotFound
		}
		return Document{}, Version{}, fmt.Errorf("lock document set: %w", err)
	}
	if setStatus != SetStatusActive {
		return Document{}, Version{}, fmt.Errorf("%w: document set is not active", ErrInvalidInput)
	}

	var item Document
	err = tx.QueryRow(ctx, `INSERT INTO documents (document_set_id, name, document_type)
		VALUES ($1,$2,$3)
		ON CONFLICT (document_set_id, name) DO UPDATE SET updated_at=NOW()
		RETURNING id, document_set_id, name, document_type, created_at, updated_at`,
		setID, input.DocumentName, input.DocumentType).Scan(&item.ID, &item.DocumentSetID,
		&item.Name, &item.DocumentType, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Document{}, Version{}, fmt.Errorf("create or load document: %w", err)
	}
	if item.DocumentType != input.DocumentType {
		return Document{}, Version{}, fmt.Errorf("%w: document name already uses type %s",
			ErrInvalidInput, item.DocumentType)
	}

	var versionNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version_number),0)+1
		FROM document_versions WHERE document_id=$1`, item.ID).Scan(&versionNumber); err != nil {
		return Document{}, Version{}, fmt.Errorf("allocate document version: %w", err)
	}
	const insertVersion = `INSERT INTO document_versions
		(document_id, document_set_id, version_number, original_filename, media_type,
		 size_bytes, sha256, storage_key, approval_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, document_id, document_set_id, version_number, original_filename,
		 media_type, size_bytes, sha256, storage_key, approval_status, parse_status,
		 parse_error, block_count, attempt_count, next_attempt_at, lease_expires_at,
		 uploaded_at, started_at, parsed_at`
	var version Version
	err = tx.QueryRow(ctx, insertVersion, item.ID, setID, versionNumber, input.Filename,
		input.MediaType, stored.SizeBytes, stored.SHA256, stored.StorageKey, input.ApprovalStatus).
		Scan(versionDestinations(&version)...)
	if err != nil {
		if uniqueViolation(err) {
			return Document{}, Version{}, ErrAlreadyExists
		}
		return Document{}, Version{}, fmt.Errorf("create document version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Document{}, Version{}, fmt.Errorf("commit document upload: %w", err)
	}
	item.LatestVersion = &version
	return item, version, nil
}

func (r *PostgresRepository) ListDocuments(ctx context.Context, setID int64) ([]Document, error) {
	if _, err := r.GetSet(ctx, setID); err != nil {
		return nil, err
	}
	const query = `SELECT d.id, d.document_set_id, d.name, d.document_type,
		d.created_at, d.updated_at,
		v.id, v.document_id, v.document_set_id, v.version_number, v.original_filename,
		v.media_type, v.size_bytes, v.sha256, v.storage_key, v.approval_status,
		v.parse_status, v.parse_error, v.block_count, v.attempt_count,
		v.next_attempt_at, v.lease_expires_at, v.uploaded_at, v.started_at, v.parsed_at
		FROM documents d
		LEFT JOIN LATERAL (
			SELECT * FROM document_versions
			WHERE document_id=d.id ORDER BY version_number DESC LIMIT 1
		) v ON TRUE
		WHERE d.document_set_id=$1 ORDER BY d.created_at, d.id`
	rows, err := r.pool.Query(ctx, query, setID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()
	results := make([]Document, 0)
	for rows.Next() {
		var item Document
		var version Version
		destinations := []any{&item.ID, &item.DocumentSetID, &item.Name, &item.DocumentType,
			&item.CreatedAt, &item.UpdatedAt}
		destinations = append(destinations, versionDestinations(&version)...)
		if err := rows.Scan(destinations...); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		item.LatestVersion = &version
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}
	return results, nil
}

func (r *PostgresRepository) GetVersion(ctx context.Context, documentID int64, versionNumber int) (Document, Version, error) {
	const query = `SELECT d.id, d.document_set_id, d.name, d.document_type,
		d.created_at, d.updated_at,
		v.id, v.document_id, v.document_set_id, v.version_number, v.original_filename,
		v.media_type, v.size_bytes, v.sha256, v.storage_key, v.approval_status,
		v.parse_status, v.parse_error, v.block_count, v.attempt_count,
		v.next_attempt_at, v.lease_expires_at, v.uploaded_at, v.started_at, v.parsed_at
		FROM documents d JOIN document_versions v ON v.document_id=d.id
		WHERE d.id=$1 AND v.version_number=$2`
	var item Document
	var version Version
	destinations := []any{&item.ID, &item.DocumentSetID, &item.Name, &item.DocumentType,
		&item.CreatedAt, &item.UpdatedAt}
	destinations = append(destinations, versionDestinations(&version)...)
	err := r.pool.QueryRow(ctx, query, documentID, versionNumber).Scan(destinations...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, Version{}, ErrNotFound
	}
	if err != nil {
		return Document{}, Version{}, fmt.Errorf("get document version: %w", err)
	}
	item.LatestVersion = &version
	return item, version, nil
}

func (r *PostgresRepository) ListBlocks(ctx context.Context, versionID int64) ([]Block, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, document_version_id, ordinal, block_type,
		heading_level, content, source_locator, metadata, created_at
		FROM document_blocks WHERE document_version_id=$1 ORDER BY ordinal`, versionID)
	if err != nil {
		return nil, fmt.Errorf("list document blocks: %w", err)
	}
	defer rows.Close()
	results := make([]Block, 0)
	for rows.Next() {
		var item Block
		if err := rows.Scan(&item.ID, &item.DocumentVersionID, &item.Ordinal,
			&item.BlockType, &item.HeadingLevel, &item.Content, &item.SourceLocator,
			&item.Metadata, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan document block: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document blocks: %w", err)
	}
	return results, nil
}

func (r *PostgresRepository) ClaimNext(ctx context.Context, leaseDuration time.Duration) (Version, error) {
	const query = `WITH next_version AS (
		SELECT id FROM document_versions
		WHERE (parse_status='UPLOADED' AND next_attempt_at<=NOW() AND lease_expires_at IS NULL)
		   OR (parse_status='PARSING' AND lease_expires_at<=NOW())
		ORDER BY uploaded_at, id FOR UPDATE SKIP LOCKED LIMIT 1
	)
	UPDATE document_versions v SET parse_status='PARSING', parse_error='',
		attempt_count=v.attempt_count+1, started_at=COALESCE(v.started_at,NOW()),
		lease_expires_at=NOW()+$1::interval
	FROM next_version WHERE v.id=next_version.id
	RETURNING v.id, v.document_id, v.document_set_id, v.version_number,
		v.original_filename, v.media_type, v.size_bytes, v.sha256, v.storage_key,
		v.approval_status, v.parse_status, v.parse_error, v.block_count,
		v.attempt_count, v.next_attempt_at, v.lease_expires_at, v.uploaded_at,
		v.started_at, v.parsed_at`
	var result Version
	if err := r.pool.QueryRow(ctx, query, leaseDuration.String()).Scan(versionDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Version{}, ErrNotFound
		}
		return Version{}, fmt.Errorf("claim document parse: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) RenewLease(ctx context.Context, claimed Version, leaseDuration time.Duration) error {
	result, err := r.pool.Exec(ctx, `UPDATE document_versions
		SET lease_expires_at=NOW()+$3::interval
		WHERE id=$1 AND parse_status='PARSING' AND attempt_count=$2`,
		claimed.ID, claimed.AttemptCount, leaseDuration.String())
	if err != nil {
		return fmt.Errorf("renew document parse lease: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (r *PostgresRepository) RetryOrFail(ctx context.Context, claimed Version, processErr error,
	maxAttempts int, retryDelay time.Duration,
) error {
	result, err := r.pool.Exec(ctx, `UPDATE document_versions SET
		parse_status=CASE WHEN attempt_count<$3 THEN 'UPLOADED' ELSE 'FAILED' END,
		parse_error=$4,
		next_attempt_at=CASE WHEN attempt_count<$3 THEN NOW()+$5::interval ELSE next_attempt_at END,
		lease_expires_at=NULL,
		parsed_at=CASE WHEN attempt_count<$3 THEN NULL ELSE NOW() END
		WHERE id=$1 AND parse_status='PARSING' AND attempt_count=$2`,
		claimed.ID, claimed.AttemptCount, maxAttempts, processErr.Error(), retryDelay.String())
	if err != nil {
		return fmt.Errorf("retry or fail document parse: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (r *PostgresRepository) SaveParsed(ctx context.Context, claimed Version, blocks []ParsedBlock) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save parsed document: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM document_blocks WHERE document_version_id=$1`, claimed.ID); err != nil {
		return fmt.Errorf("replace document blocks: %w", err)
	}
	const insert = `INSERT INTO document_blocks
		(document_version_id, ordinal, block_type, heading_level, content, source_locator, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`
	for index, block := range blocks {
		metadata, err := json.Marshal(block.Metadata)
		if err != nil {
			return fmt.Errorf("encode block %d metadata: %w", index+1, err)
		}
		if _, err := tx.Exec(ctx, insert, claimed.ID, index+1, block.BlockType,
			block.HeadingLevel, block.Content, block.SourceLocator, metadata); err != nil {
			return fmt.Errorf("insert document block %d: %w", index+1, err)
		}
	}
	result, err := tx.Exec(ctx, `UPDATE document_versions SET parse_status='PARSED',
		parse_error='', block_count=$3, lease_expires_at=NULL, parsed_at=NOW()
		WHERE id=$1 AND parse_status='PARSING' AND attempt_count=$2`,
		claimed.ID, claimed.AttemptCount, len(blocks))
	if err != nil {
		return fmt.Errorf("complete document parse: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit parsed document: %w", err)
	}
	return nil
}

func versionDestinations(item *Version) []any {
	return []any{&item.ID, &item.DocumentID, &item.DocumentSetID, &item.VersionNumber,
		&item.OriginalFilename, &item.MediaType, &item.SizeBytes, &item.SHA256,
		&item.StorageKey, &item.ApprovalStatus, &item.ParseStatus, &item.ParseError,
		&item.BlockCount, &item.AttemptCount, &item.NextAttemptAt, &item.LeaseExpiresAt,
		&item.UploadedAt, &item.StartedAt, &item.ParsedAt}
}

func uniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
