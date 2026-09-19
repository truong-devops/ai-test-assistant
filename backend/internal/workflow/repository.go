package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const jobColumns = `id,document_set_id,operation,status,revision,input_snapshot,input_hash,
	requested_by,idempotency_key,total_units,completed_units,failed_units,attempt_count,
	max_attempts,next_attempt_at,lease_expires_at,heartbeat_at,cancel_requested_at,
	error_code,error_message,retryable,output_refs,delegate_kind,delegate_job_id,
	created_at,started_at,finished_at,updated_at`

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

type readFacts struct {
	SetStatus             string
	SourceRevision        int64
	DocumentCount         int
	ParsedCount           int
	ApprovedSourceCount   int
	IndexStatus           string
	IndexGeneration       int64
	IndexedRevision       int64
	ExtractionReady       bool
	RequirementCount      int
	ApprovedRequirements  int
	TestCaseCount         int
	ApprovedTestCases     int
	ReleaseCount          int
	BudgetRemainingTokens int64
	BudgetRemainingCost   int64
	CostBudgetConfigured  bool
}

func (r *Repository) readFacts(ctx context.Context, setID int64) (readFacts, error) {
	var result readFacts
	err := r.pool.QueryRow(ctx, `SELECT status,source_revision FROM document_sets WHERE id=$1`,
		setID).Scan(&result.SetStatus, &result.SourceRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*),
		count(*) FILTER(WHERE latest.parse_status='PARSED'),
		count(*) FILTER(WHERE latest.approval_status='APPROVED')
		FROM documents document JOIN LATERAL (
			SELECT parse_status,approval_status FROM document_versions
			WHERE document_id=document.id ORDER BY version_number DESC,id DESC LIMIT 1
		) latest ON TRUE WHERE document.document_set_id=$1`, setID).Scan(
		&result.DocumentCount, &result.ParsedCount, &result.ApprovedSourceCount); err != nil {
		return result, err
	}
	var sourceSnapshotID *int64
	err = r.pool.QueryRow(ctx, `SELECT status,generation,indexed_source_revision,source_snapshot_id
		FROM document_index_status WHERE document_set_id=$1`, setID).Scan(&result.IndexStatus,
		&result.IndexGeneration, &result.IndexedRevision, &sourceSnapshotID)
	if errors.Is(err, pgx.ErrNoRows) {
		result.IndexStatus = "NOT_INDEXED"
	} else if err != nil {
		return result, err
	}
	result.ExtractionReady = result.IndexStatus == "READY" &&
		result.IndexedRevision == result.SourceRevision && sourceSnapshotID != nil
	if result.ExtractionReady {
		var included, approved int
		if err := r.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE approval_status='APPROVED')
			FROM document_source_snapshot_items WHERE source_snapshot_id=$1 AND included=TRUE`,
			*sourceSnapshotID).Scan(&included, &approved); err != nil {
			return result, err
		}
		result.ExtractionReady = included > 0 && included == approved
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='APPROVED')
		FROM requirements requirement WHERE document_set_id=$1
		AND NOT EXISTS(SELECT 1 FROM requirements newer
			WHERE newer.supersedes_requirement_id=requirement.id)`, setID).Scan(
		&result.RequirementCount, &result.ApprovedRequirements); err != nil {
		return result, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE test.status='APPROVED')
		FROM test_case_families family JOIN test_cases test ON test.id=family.head_revision_id
		WHERE family.document_set_id=$1 AND family.archived=FALSE`, setID).Scan(
		&result.TestCaseCount, &result.ApprovedTestCases); err != nil {
		return result, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM test_suite_releases
		WHERE document_set_id=$1`, setID).Scan(&result.ReleaseCount); err != nil {
		return result, err
	}
	var tokenBudget, costBudget, usedTokens, reservedTokens, usedCost, reservedCost int64
	if err := r.pool.QueryRow(ctx, `SELECT set.ai_token_budget,set.ai_cost_budget_microusd,
		COALESCE(sum(reservation.input_tokens+reservation.output_tokens)
			FILTER(WHERE reservation.status='COMPLETED'),0),
		COALESCE(sum(reservation.reserved_tokens)
			FILTER(WHERE reservation.status='RESERVED' AND reservation.expires_at>NOW()),0),
		COALESCE(sum(reservation.actual_cost_microusd)
			FILTER(WHERE reservation.status='COMPLETED'),0),
		COALESCE(sum(reservation.reserved_cost_microusd)
			FILTER(WHERE reservation.status='RESERVED' AND reservation.expires_at>NOW()),0)
		FROM document_sets set LEFT JOIN document_ai_budget_reservations reservation
			ON reservation.document_set_id=set.id WHERE set.id=$1 GROUP BY set.id`, setID).Scan(
		&tokenBudget, &costBudget, &usedTokens, &reservedTokens, &usedCost, &reservedCost); err != nil {
		return result, err
	}
	result.BudgetRemainingTokens = max(tokenBudget-usedTokens-reservedTokens, 0)
	result.BudgetRemainingCost = max(costBudget-usedCost-reservedCost, 0)
	result.CostBudgetConfigured = costBudget > 0
	return result, nil
}

func hashJSON(value any) (json.RawMessage, string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(payload)
	return payload, hex.EncodeToString(sum[:]), nil
}

type indexVersionSnapshot struct {
	DocumentID        int64  `json:"document_id"`
	DocumentName      string `json:"document_name"`
	DocumentVersionID int64  `json:"document_version_id"`
	VersionNumber     int    `json:"version_number"`
	SHA256            string `json:"sha256"`
	ParseStatus       string `json:"parse_status"`
	ApprovalStatus    string `json:"approval_status"`
	Included          bool   `json:"included"`
}

type indexInputSnapshot struct {
	SourceRevision     int64                  `json:"source_revision"`
	ExcludedVersionIDs []int64                `json:"excluded_version_ids"`
	Versions           []indexVersionSnapshot `json:"versions"`
}

func (r *Repository) BuildIndexSnapshot(ctx context.Context, setID int64,
	excluded []int64,
) (json.RawMessage, string, error) {
	if setID <= 0 {
		return nil, "", ErrInvalidInput
	}
	var sourceRevision int64
	var setStatus string
	if err := r.pool.QueryRow(ctx, `SELECT source_revision,status FROM document_sets WHERE id=$1`,
		setID).Scan(&sourceRevision, &setStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	if setStatus != "ACTIVE" {
		return nil, "", &BlockedError{Code: "DOCUMENT_SET_INACTIVE",
			Message: "document set must be active", NextAction: "REACTIVATE_DOCUMENT_SET"}
	}
	excludedSet := map[int64]bool{}
	for _, id := range excluded {
		if id <= 0 {
			return nil, "", ErrInvalidInput
		}
		excludedSet[id] = true
	}
	rows, err := r.pool.Query(ctx, `SELECT d.id,d.name,v.id,v.version_number,v.sha256,
		v.parse_status,v.approval_status
		FROM documents d JOIN LATERAL (
			SELECT id,version_number,sha256,parse_status,approval_status
			FROM document_versions WHERE document_id=d.id
			ORDER BY version_number DESC,id DESC LIMIT 1) v ON TRUE
		WHERE d.document_set_id=$1 ORDER BY d.id`, setID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	result := indexInputSnapshot{SourceRevision: sourceRevision,
		ExcludedVersionIDs: make([]int64, 0, len(excludedSet)), Versions: []indexVersionSnapshot{}}
	known := map[int64]bool{}
	included := 0
	blocked := []BlockingReason{}
	blockedVersions := []indexVersionSnapshot{}
	for rows.Next() {
		var item indexVersionSnapshot
		if err := rows.Scan(&item.DocumentID, &item.DocumentName, &item.DocumentVersionID, &item.VersionNumber,
			&item.SHA256, &item.ParseStatus, &item.ApprovalStatus); err != nil {
			return nil, "", err
		}
		known[item.DocumentVersionID] = true
		item.Included = !excludedSet[item.DocumentVersionID]
		if item.Included {
			included++
			if item.ParseStatus != "PARSED" {
				blockedVersions = append(blockedVersions, item)
				blocked = append(blocked, BlockingReason{Code: "SOURCE_NOT_PARSED",
					Message: fmt.Sprintf("document version %d is not parsed", item.DocumentVersionID),
					Step:    "SOURCE", NextAction: "WAIT_FOR_PARSE_OR_EXCLUDE"})
			}
		}
		result.Versions = append(result.Versions, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	for id := range excludedSet {
		if !known[id] {
			return nil, "", ErrInvalidInput
		}
		result.ExcludedVersionIDs = append(result.ExcludedVersionIDs, id)
	}
	sort.Slice(result.ExcludedVersionIDs, func(i, j int) bool {
		return result.ExcludedVersionIDs[i] < result.ExcludedVersionIDs[j]
	})
	if len(result.Versions) == 0 || included == 0 {
		blocked = append(blocked, BlockingReason{Code: "NO_INDEXABLE_SOURCE",
			Message: "at least one parsed document version must be included",
			Step:    "SOURCE", NextAction: "UPLOAD_OR_INCLUDE_SOURCE"})
	}
	if len(blocked) > 0 {
		return nil, "", &BlockedError{Code: "SOURCE_SNAPSHOT_BLOCKED",
			Message: "source snapshot is not ready for indexing", BlockedBy: blocked,
			NextAction: blocked[0].NextAction, Details: blockedVersions}
	}
	return hashJSON(result)
}

type generateRequirementSnapshot struct {
	ID                int64  `json:"id"`
	VersionNumber     int    `json:"version_number"`
	SourceSnapshotID  *int64 `json:"source_snapshot_id,omitempty"`
	SourceFingerprint string `json:"source_fingerprint"`
	UpdatedAt         string `json:"updated_at"`
}

type generateInputSnapshot struct {
	SourceRevision int64                         `json:"source_revision"`
	Requirements   []generateRequirementSnapshot `json:"requirements"`
}

func (r *Repository) BuildGenerateSnapshot(ctx context.Context, setID int64) (
	json.RawMessage, string, error,
) {
	if setID <= 0 {
		return nil, "", ErrInvalidInput
	}
	var sourceRevision int64
	var status string
	if err := r.pool.QueryRow(ctx, `SELECT source_revision,status FROM document_sets WHERE id=$1`,
		setID).Scan(&sourceRevision, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	if status != "ACTIVE" {
		return nil, "", &BlockedError{Code: "DOCUMENT_SET_INACTIVE",
			Message: "document set must be active", NextAction: "REACTIVATE_DOCUMENT_SET"}
	}
	rows, err := r.pool.Query(ctx, `SELECT requirement.id,requirement.version_number,
		requirement.source_snapshot_id,requirement.source_fingerprint,
		to_char(requirement.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
		FROM requirements requirement
		WHERE requirement.document_set_id=$1 AND requirement.status='APPROVED'
		AND NOT EXISTS(SELECT 1 FROM requirements newer
			WHERE newer.supersedes_requirement_id=requirement.id)
		ORDER BY requirement.id`, setID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	snapshot := generateInputSnapshot{SourceRevision: sourceRevision,
		Requirements: []generateRequirementSnapshot{}}
	for rows.Next() {
		var item generateRequirementSnapshot
		if err := rows.Scan(&item.ID, &item.VersionNumber, &item.SourceSnapshotID,
			&item.SourceFingerprint, &item.UpdatedAt); err != nil {
			return nil, "", err
		}
		snapshot.Requirements = append(snapshot.Requirements, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(snapshot.Requirements) == 0 {
		reason := BlockingReason{Code: "NO_APPROVED_REQUIREMENTS",
			Message: "approve at least one current requirement before generating test cases",
			Step:    "REQUIREMENTS", NextAction: "REVIEW_REQUIREMENTS"}
		return nil, "", &BlockedError{Code: reason.Code, Message: reason.Message,
			BlockedBy: []BlockingReason{reason}, NextAction: reason.NextAction}
	}
	return hashJSON(snapshot)
}

func (r *Repository) ValidateInputSnapshot(ctx context.Context, job Job) error {
	switch job.Operation {
	case OperationIndex:
		var snapshot indexInputSnapshot
		if err := json.Unmarshal(job.InputSnapshot, &snapshot); err != nil {
			return ErrInvalidInput
		}
		_, currentHash, err := r.BuildIndexSnapshot(ctx, job.DocumentSetID,
			snapshot.ExcludedVersionIDs)
		if err != nil {
			return err
		}
		if currentHash != job.InputHash {
			return ErrInputStale
		}
	case OperationGenerate:
		_, currentHash, err := r.BuildGenerateSnapshot(ctx, job.DocumentSetID)
		if err != nil {
			return err
		}
		if currentHash != job.InputHash {
			return ErrInputStale
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func (r *Repository) ExistingByKey(ctx context.Context, setID int64, key string) (Job, bool, error) {
	var result Job
	err := r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM document_workflow_jobs
		WHERE document_set_id=$1 AND idempotency_key=$2`, setID, key).Scan(jobDest(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	loaded, err := r.Get(ctx, result.ID)
	return loaded, true, err
}

func (r *Repository) EnqueueNative(ctx context.Context, setID int64, operation,
	requestedBy, idempotencyKey string, snapshot json.RawMessage, inputHash string,
	maxAttempts int,
) (Job, bool, error) {
	return r.enqueue(ctx, setID, operation, requestedBy, idempotencyKey, snapshot,
		inputHash, maxAttempts, "", nil)
}

func (r *Repository) EnqueueDelegated(ctx context.Context, setID int64, operation,
	requestedBy, idempotencyKey string, snapshot json.RawMessage, inputHash string,
	maxAttempts int, delegateKind string, delegateJobID int64,
) (Job, bool, error) {
	return r.enqueue(ctx, setID, operation, requestedBy, idempotencyKey, snapshot,
		inputHash, maxAttempts, delegateKind, &delegateJobID)
}

func (r *Repository) enqueue(ctx context.Context, setID int64, operation, requestedBy,
	idempotencyKey string, snapshot json.RawMessage, inputHash string, maxAttempts int,
	delegateKind string, delegateJobID *int64,
) (Job, bool, error) {
	if setID <= 0 || !validOperation(operation) || strings.TrimSpace(requestedBy) == "" ||
		strings.TrimSpace(idempotencyKey) == "" || len(idempotencyKey) > 200 ||
		len(snapshot) == 0 || len(inputHash) != 64 || maxAttempts < 1 || maxAttempts > 20 {
		return Job{}, false, ErrInvalidInput
	}
	if existing, found, err := r.ExistingByKey(ctx, setID, idempotencyKey); err != nil {
		return Job{}, false, err
	} else if found {
		if existing.InputHash != inputHash || existing.Operation != operation {
			return Job{}, false, ErrIdempotencyConflict
		}
		return existing, false, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback(ctx)
	var result Job
	err = tx.QueryRow(ctx, `INSERT INTO document_workflow_jobs
		(document_set_id,operation,input_snapshot,input_hash,requested_by,idempotency_key,
		 max_attempts,delegate_kind,delegate_job_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+jobColumns,
		setID, operation, snapshot, inputHash, requestedBy, idempotencyKey, maxAttempts,
		delegateKind, delegateJobID).Scan(jobDest(&result)...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				return Job{}, false, errors.Join(err, rollbackErr)
			}
			if existing, found, findErr := r.ExistingByKey(ctx, setID, idempotencyKey); findErr != nil {
				return Job{}, false, findErr
			} else if found {
				if existing.InputHash != inputHash || existing.Operation != operation {
					return Job{}, false, ErrIdempotencyConflict
				}
				return existing, false, nil
			}
			active, activeErr := r.activeOperation(ctx, setID, operation)
			if activeErr == nil {
				return active, false, nil
			}
			return Job{}, false, ErrOperationActive
		}
		return Job{}, false, err
	}
	var unitID int64
	if err := tx.QueryRow(ctx, `INSERT INTO document_workflow_job_units
		(workflow_job_id,unit_key,input_hash) VALUES($1,'operation',$2) RETURNING id`,
		result.ID, inputHash).Scan(&unitID); err != nil {
		return Job{}, false, err
	}
	if delegateJobID != nil {
		result, err := tx.Exec(ctx, `UPDATE requirement_extraction_jobs SET
			workflow_job_id=$2,workflow_unit_id=$3 WHERE id=$1 AND document_set_id=$4`,
			*delegateJobID, result.ID, unitID, setID)
		if err != nil {
			return Job{}, false, err
		}
		if result.RowsAffected() != 1 {
			return Job{}, false, ErrInvalidInput
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, false, err
	}
	loaded, err := r.Get(ctx, result.ID)
	return loaded, true, err
}

func (r *Repository) activeOperation(ctx context.Context, setID int64, operation string) (Job, error) {
	var result Job
	err := r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM document_workflow_jobs
		WHERE document_set_id=$1 AND operation=$2 AND status IN ('QUEUED','RUNNING')
		ORDER BY id DESC LIMIT 1`, setID, operation).Scan(jobDest(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	return r.Get(ctx, result.ID)
}

func (r *Repository) ClaimNext(ctx context.Context, lease time.Duration) (Job, error) {
	if lease <= 0 {
		return Job{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, `UPDATE document_workflow_jobs SET status='FAILED',revision=revision+1,
		error_code='ATTEMPTS_EXHAUSTED',error_message='worker lease expired after maximum attempts',
		retryable=TRUE,lease_expires_at=NULL,finished_at=NOW(),updated_at=NOW()
		WHERE delegate_kind='' AND status='RUNNING' AND lease_expires_at<=NOW()
		AND attempt_count>=max_attempts`)
	_, _ = tx.Exec(ctx, `UPDATE document_workflow_job_units unit SET status='FAILED',
		error_code='ATTEMPTS_EXHAUSTED',error_message='worker lease expired after maximum attempts',
		finished_at=NOW(),updated_at=NOW()
		FROM document_workflow_jobs job WHERE unit.workflow_job_id=job.id
		AND job.status='FAILED' AND job.error_code='ATTEMPTS_EXHAUSTED' AND unit.status='RUNNING'`)
	var result Job
	err = tx.QueryRow(ctx, `WITH candidate AS (
		SELECT id FROM document_workflow_jobs
		WHERE delegate_kind='' AND cancel_requested_at IS NULL AND attempt_count<max_attempts
		AND ((status='QUEUED' AND next_attempt_at<=NOW()) OR
			(status='RUNNING' AND lease_expires_at<=NOW()))
		ORDER BY next_attempt_at,created_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
		UPDATE document_workflow_jobs job SET status='RUNNING',revision=job.revision+1,
		attempt_count=job.attempt_count+1,started_at=COALESCE(job.started_at,NOW()),
		heartbeat_at=NOW(),lease_expires_at=NOW()+$1::interval,error_code='',
		error_message='',retryable=FALSE,updated_at=NOW()
		FROM candidate WHERE job.id=candidate.id RETURNING `+prefixedJobColumns("job."),
		lease.String()).Scan(jobDest(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	var unit Unit
	err = tx.QueryRow(ctx, `UPDATE document_workflow_job_units SET status='RUNNING',
		attempt_count=attempt_count+1,started_at=COALESCE(started_at,NOW()),finished_at=NULL,
		error_code='',error_message='',updated_at=NOW()
		WHERE workflow_job_id=$1 AND unit_key='operation' AND status IN ('QUEUED','RUNNING','FAILED')
		RETURNING id,workflow_job_id,unit_key,input_hash,status,attempt_count,output_ref,
		error_code,error_message,started_at,finished_at,updated_at`, result.ID).Scan(unitDest(&unit)...)
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	result.Units = []Unit{unit}
	return result, nil
}

func prefixedJobColumns(prefix string) string {
	parts := strings.Split(jobColumns, ",")
	for index, part := range parts {
		parts[index] = prefix + strings.TrimSpace(part)
	}
	return strings.Join(parts, ",")
}

func (r *Repository) Heartbeat(ctx context.Context, claimed Job, lease time.Duration) error {
	result, err := r.pool.Exec(ctx, `UPDATE document_workflow_jobs SET heartbeat_at=NOW(),
		lease_expires_at=NOW()+$3::interval,updated_at=NOW()
		WHERE id=$1 AND status='RUNNING' AND attempt_count=$2 AND cancel_requested_at IS NULL`,
		claimed.ID, claimed.AttemptCount, lease.String())
	if err != nil {
		return err
	}
	if result.RowsAffected() == 1 {
		return nil
	}
	var canceled bool
	_ = r.pool.QueryRow(ctx, `SELECT cancel_requested_at IS NOT NULL OR status='CANCELED'
		FROM document_workflow_jobs WHERE id=$1`, claimed.ID).Scan(&canceled)
	if canceled {
		return ErrCanceled
	}
	return ErrLeaseLost
}

func (r *Repository) Complete(ctx context.Context, claimed Job, output any) error {
	payload, err := json.Marshal(output)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE document_workflow_job_units SET status='SUCCEEDED',
		output_ref=$4,error_code='',error_message='',finished_at=NOW(),updated_at=NOW()
		WHERE workflow_job_id=$1 AND unit_key='operation' AND status='RUNNING'
		AND attempt_count=$2 AND EXISTS(SELECT 1 FROM document_workflow_jobs job
			WHERE job.id=$1 AND job.status='RUNNING' AND job.attempt_count=$3
			AND job.cancel_requested_at IS NULL)`, claimed.ID, claimed.Units[0].AttemptCount,
		claimed.AttemptCount, payload)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	result, err = tx.Exec(ctx, `UPDATE document_workflow_jobs SET status='SUCCEEDED',
		revision=revision+1,completed_units=total_units,failed_units=0,output_refs=$3,
		error_code='',error_message='',retryable=FALSE,heartbeat_at=NOW(),lease_expires_at=NULL,
		finished_at=NOW(),updated_at=NOW() WHERE id=$1 AND status='RUNNING'
		AND attempt_count=$2 AND cancel_requested_at IS NULL`, claimed.ID, claimed.AttemptCount, payload)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return tx.Commit(ctx)
}

func (r *Repository) Fail(ctx context.Context, claimed Job, processErr error, code string,
	retryable bool, retryDelay time.Duration,
) error {
	message := truncateError(processErr)
	if code == "" {
		code = "WORKFLOW_PROCESSING_FAILED"
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var cancelRequested bool
	var status string
	if err := tx.QueryRow(ctx, `SELECT cancel_requested_at IS NOT NULL,status
		FROM document_workflow_jobs WHERE id=$1 AND attempt_count=$2 FOR UPDATE`,
		claimed.ID, claimed.AttemptCount).Scan(&cancelRequested, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLeaseLost
		}
		return err
	}
	if status != StatusRunning {
		return ErrLeaseLost
	}
	nextStatus := StatusFailed
	unitStatus := UnitFailed
	willRetry := retryable && claimed.AttemptCount < claimed.MaxAttempts && !cancelRequested
	if willRetry {
		nextStatus, unitStatus = StatusQueued, UnitQueued
	}
	if cancelRequested || errors.Is(processErr, context.Canceled) || errors.Is(processErr, ErrCanceled) {
		nextStatus, unitStatus, retryable = StatusCanceled, UnitCanceled, false
		code, message = "CANCELED", "workflow canceled by user"
	}
	_, err = tx.Exec(ctx, `UPDATE document_workflow_job_units SET status=$3,
		error_code=$4,error_message=$5,finished_at=CASE WHEN $3='QUEUED' THEN NULL ELSE NOW() END,
		updated_at=NOW() WHERE workflow_job_id=$1 AND unit_key='operation' AND attempt_count=$2`,
		claimed.ID, claimed.Units[0].AttemptCount, unitStatus, code, message)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE document_workflow_jobs SET status=$3,revision=revision+1,
		failed_units=CASE WHEN $3='QUEUED' THEN 0 ELSE 1 END,error_code=$4,error_message=$5,
		retryable=$6,next_attempt_at=CASE WHEN $3='QUEUED' THEN NOW()+$7::interval ELSE next_attempt_at END,
		lease_expires_at=NULL,finished_at=CASE WHEN $3='QUEUED' THEN NULL ELSE NOW() END,
		updated_at=NOW() WHERE id=$1 AND status='RUNNING' AND attempt_count=$2`,
		claimed.ID, claimed.AttemptCount, nextStatus, code, message, retryable,
		retryDelay.String())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return tx.Commit(ctx)
}

func (r *Repository) Cancel(ctx context.Context, id int64, expectedRevision int) (Job, error) {
	if id <= 0 || expectedRevision <= 0 {
		return Job{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	var status, delegateKind string
	var delegateID *int64
	err = tx.QueryRow(ctx, `SELECT status,delegate_kind,delegate_job_id
		FROM document_workflow_jobs WHERE id=$1 AND revision=$2 FOR UPDATE`, id,
		expectedRevision).Scan(&status, &delegateKind, &delegateID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrRevisionConflict
	}
	if err != nil {
		return Job{}, err
	}
	if status != StatusQueued && status != StatusRunning {
		return Job{}, ErrNotCancelable
	}
	if delegateKind == "REQUIREMENT_EXTRACTION" && delegateID != nil {
		_, err = tx.Exec(ctx, `UPDATE requirement_extraction_jobs SET status='CANCELED',
			lease_expires_at=NULL,finished_at=NOW(),updated_at=NOW(),
			error_message='canceled by workflow user' WHERE id=$1 AND status IN ('PENDING','RUNNING')`,
			*delegateID)
		if err != nil {
			return Job{}, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE document_workflow_job_units SET status='CANCELED',
		error_code='CANCELED',error_message='workflow canceled by user',finished_at=NOW(),updated_at=NOW()
		WHERE workflow_job_id=$1 AND status IN ('QUEUED','RUNNING')`, id)
	if err != nil {
		return Job{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE document_workflow_jobs SET status=CASE WHEN status='QUEUED'
		THEN 'CANCELED' ELSE status END,revision=revision+1,cancel_requested_at=NOW(),
		error_code='CANCELED',error_message='workflow canceled by user',retryable=FALSE,
		lease_expires_at=CASE WHEN status='QUEUED' THEN NULL ELSE lease_expires_at END,
		finished_at=CASE WHEN status='QUEUED' THEN NOW() ELSE finished_at END,updated_at=NOW()
		WHERE id=$1`, id)
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repository) Retry(ctx context.Context, id int64, expectedRevision int) (Job, error) {
	if id <= 0 || expectedRevision <= 0 {
		return Job{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	var status, delegateKind string
	var delegateID *int64
	err = tx.QueryRow(ctx, `SELECT status,delegate_kind,delegate_job_id
		FROM document_workflow_jobs WHERE id=$1 AND revision=$2 FOR UPDATE`, id,
		expectedRevision).Scan(&status, &delegateKind, &delegateID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrRevisionConflict
	}
	if err != nil {
		return Job{}, err
	}
	if status != StatusFailed && status != StatusPartialFailed && status != StatusCanceled {
		return Job{}, ErrNotRetryable
	}
	if delegateKind == "REQUIREMENT_EXTRACTION" && delegateID != nil {
		_, err = tx.Exec(ctx, `UPDATE requirement_extraction_jobs SET status='PENDING',
			attempt_count=0,next_attempt_at=NOW(),lease_expires_at=NULL,error_message='',
			started_at=NULL,finished_at=NULL,updated_at=NOW() WHERE id=$1`, *delegateID)
		if err != nil {
			return Job{}, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE document_workflow_job_units SET status='QUEUED',
		error_code='',error_message='',finished_at=NULL,updated_at=NOW()
		WHERE workflow_job_id=$1 AND status IN ('FAILED','CANCELED')`, id)
	if err != nil {
		return Job{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE document_workflow_jobs SET status='QUEUED',revision=revision+1,
		attempt_count=0,completed_units=0,failed_units=0,next_attempt_at=NOW(),lease_expires_at=NULL,
		heartbeat_at=NULL,cancel_requested_at=NULL,error_code='',error_message='',retryable=FALSE,
		started_at=NULL,finished_at=NULL,updated_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repository) reconcileDelegated(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `WITH state AS (
		SELECT workflow.id,external.status,external.total_chunks,external.processed_chunks,
			external.attempt_count,external.error_message,external.started_at,external.finished_at,
			CASE external.status WHEN 'PENDING' THEN 'QUEUED' WHEN 'RUNNING' THEN 'RUNNING'
				WHEN 'COMPLETED' THEN 'SUCCEEDED' WHEN 'FAILED' THEN 'FAILED'
				WHEN 'CANCELED' THEN 'CANCELED' END AS mapped_status
		FROM document_workflow_jobs workflow JOIN requirement_extraction_jobs external
			ON external.id=workflow.delegate_job_id
		WHERE workflow.id=$1 AND workflow.delegate_kind='REQUIREMENT_EXTRACTION')
	UPDATE document_workflow_jobs workflow SET status=state.mapped_status,
		total_units=state.total_chunks,completed_units=state.processed_chunks,
		failed_units=CASE WHEN state.mapped_status='FAILED' THEN
			GREATEST(state.total_chunks-state.processed_chunks,1) ELSE 0 END,
		attempt_count=state.attempt_count,error_code=CASE WHEN state.mapped_status='FAILED'
			THEN 'REQUIREMENT_EXTRACTION_FAILED' WHEN state.mapped_status='CANCELED' THEN 'CANCELED' ELSE '' END,
		error_message=state.error_message,retryable=state.mapped_status='FAILED',
		started_at=state.started_at,finished_at=state.finished_at,
		heartbeat_at=CASE WHEN state.mapped_status='RUNNING' THEN NOW() ELSE workflow.heartbeat_at END,
		updated_at=NOW(),revision=CASE WHEN workflow.status<>state.mapped_status
			THEN workflow.revision+1 ELSE workflow.revision END
	FROM state WHERE workflow.id=state.id`, id)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `UPDATE document_workflow_job_units unit SET
		status=CASE job.status WHEN 'QUEUED' THEN 'QUEUED' WHEN 'RUNNING' THEN 'RUNNING'
			WHEN 'SUCCEEDED' THEN 'SUCCEEDED' WHEN 'FAILED' THEN 'FAILED'
			WHEN 'CANCELED' THEN 'CANCELED' ELSE unit.status END,
		attempt_count=job.attempt_count,error_code=job.error_code,error_message=job.error_message,
		started_at=job.started_at,finished_at=job.finished_at,updated_at=NOW()
	FROM document_workflow_jobs job WHERE unit.workflow_job_id=job.id AND job.id=$1
	AND job.delegate_kind='REQUIREMENT_EXTRACTION'`, id)
	return err
}

func (r *Repository) Get(ctx context.Context, id int64) (Job, error) {
	if id <= 0 {
		return Job{}, ErrInvalidInput
	}
	if err := r.reconcileDelegated(ctx, id); err != nil {
		return Job{}, err
	}
	var result Job
	err := r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM document_workflow_jobs WHERE id=$1`, id).
		Scan(jobDest(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	result.Units = []Unit{}
	rows, err := r.pool.Query(ctx, `SELECT id,workflow_job_id,unit_key,input_hash,status,
		attempt_count,output_ref,error_code,error_message,started_at,finished_at,updated_at
		FROM document_workflow_job_units WHERE workflow_job_id=$1 ORDER BY id`, id)
	if err != nil {
		return Job{}, err
	}
	for rows.Next() {
		var unit Unit
		if err := rows.Scan(unitDest(&unit)...); err != nil {
			rows.Close()
			return Job{}, err
		}
		result.Units = append(result.Units, unit)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Job{}, err
	}
	rows.Close()
	if err := r.pool.QueryRow(ctx, `SELECT
		COALESCE(sum(reserved_tokens) FILTER(WHERE status='RESERVED'),0),
		COALESCE(sum(input_tokens) FILTER(WHERE status='COMPLETED'),0),
		COALESCE(sum(output_tokens) FILTER(WHERE status='COMPLETED'),0),
		COALESCE(sum(reserved_cost_microusd) FILTER(WHERE status='RESERVED'),0),
		COALESCE(sum(actual_cost_microusd) FILTER(WHERE status='COMPLETED'),0)
		FROM document_ai_budget_reservations WHERE workflow_job_id=$1`, id).Scan(
		&result.Usage.ReservedTokens, &result.Usage.InputTokens, &result.Usage.OutputTokens,
		&result.Usage.ReservedCostMicroUSD, &result.Usage.ActualCostMicroUSD); err != nil {
		return Job{}, err
	}
	return result, nil
}

func (r *Repository) List(ctx context.Context, setID int64, limit int) ([]Job, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `SELECT id FROM document_workflow_jobs
		WHERE document_set_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`, setID, limit)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	results := make([]Job, 0, len(ids))
	for _, id := range ids {
		item, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, nil
}

func validOperation(value string) bool {
	return value == OperationIndex || value == OperationExtract || value == OperationGenerate
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 8000 {
		return value[:8000]
	}
	return value
}

func jobDest(item *Job) []any {
	return []any{&item.ID, &item.DocumentSetID, &item.Operation, &item.Status,
		&item.Revision, &item.InputSnapshot, &item.InputHash, &item.RequestedBy,
		&item.IdempotencyKey, &item.TotalUnits, &item.CompletedUnits, &item.FailedUnits,
		&item.AttemptCount, &item.MaxAttempts, &item.NextAttemptAt, &item.LeaseExpiresAt,
		&item.HeartbeatAt, &item.CancelRequested, &item.ErrorCode, &item.ErrorMessage,
		&item.Retryable, &item.OutputRefs, &item.DelegateKind, &item.DelegateJobID,
		&item.CreatedAt, &item.StartedAt, &item.FinishedAt, &item.UpdatedAt}
}

func unitDest(item *Unit) []any {
	return []any{&item.ID, &item.WorkflowJobID, &item.UnitKey, &item.InputHash,
		&item.Status, &item.AttemptCount, &item.OutputRef, &item.ErrorCode,
		&item.ErrorMessage, &item.StartedAt, &item.FinishedAt, &item.UpdatedAt}
}
