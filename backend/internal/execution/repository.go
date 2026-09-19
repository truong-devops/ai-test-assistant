package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Request(ctx context.Context, id int64, input RequestInput) (Run, error) {
	input.RequestedBy = strings.TrimSpace(input.RequestedBy)
	if id <= 0 || input.RequestedBy == "" {
		return Run{}, ErrInvalidInput
	}
	result, err := r.pool.Exec(ctx, `UPDATE test_runs SET execution_requested_at=COALESCE(execution_requested_at,NOW()),
		execution_requested_by=CASE WHEN execution_requested_by='' THEN $2 ELSE execution_requested_by END,
		next_attempt_at=NOW() WHERE id=$1 AND status='PENDING'`, id, input.RequestedBy)
	if err != nil {
		return Run{}, fmt.Errorf("request test run: %w", err)
	}
	if result.RowsAffected() != 1 {
		var exists bool
		if checkErr := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM test_runs WHERE id=$1)`, id).Scan(&exists); checkErr != nil {
			return Run{}, checkErr
		}
		if !exists {
			return Run{}, ErrNotFound
		}
		return Run{}, ErrNotRequestable
	}
	return r.Get(ctx, id)
}

func (r *Repository) ClaimNext(ctx context.Context, lease time.Duration) (Run, error) {
	const query = `WITH candidate AS (
		SELECT id FROM test_runs WHERE status='PENDING' AND execution_requested_at IS NOT NULL
		AND next_attempt_at<=NOW() AND (lease_expires_at IS NULL OR lease_expires_at<=NOW())
		ORDER BY execution_requested_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
		UPDATE test_runs r SET status='RUNNING',started_at=COALESCE(started_at,NOW()),finished_at=NULL,
		attempt_count=attempt_count+1,lease_expires_at=NOW()+$1::interval,error_message=''
		FROM candidate WHERE r.id=candidate.id
		RETURNING r.id,r.test_suite_id,r.suite_release_id,r.project_id,r.analysis_job_id,r.source_sha,r.target_sha,
		r.environment,r.environment_fingerprint,r.image_reference,r.image_digest,r.status,
		r.execution_requested_at,r.execution_requested_by,r.attempt_count,r.error_message,
		r.requested_at,r.started_at,r.finished_at`
	var result Run
	if err := r.pool.QueryRow(ctx, query, lease.String()).Scan(runDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, ErrNotFound
		}
		return Run{}, fmt.Errorf("claim test run: %w", err)
	}
	return result, nil
}

func (r *Repository) LoadAttempt(ctx context.Context, claimed Run) (Run, error) {
	result := claimed
	result.Items = []Item{}
	rows, err := r.pool.Query(ctx, `SELECT i.id,i.test_run_id,i.test_case_id,t.test_case_key,t.title,
		i.expected_result_snapshot,i.expected_result_hash,i.automation_artifact_id,i.automation_source_hash,
		i.attempt_number,i.status,i.actual_result,i.command,i.exit_code,i.duration_ms,i.output_truncated,i.created_at,
		a.id,a.file_path,a.source,a.source_hash,a.expected_result_hash,a.status
		FROM test_run_items i JOIN test_cases t ON t.id=i.test_case_id
		LEFT JOIN LATERAL (SELECT candidate.id,candidate.file_path,candidate.source,candidate.source_hash,
			candidate.expected_result_hash,candidate.status FROM automation_artifacts candidate
			WHERE candidate.test_case_id=i.test_case_id AND candidate.status='APPROVED'
			AND ((i.automation_artifact_id IS NOT NULL AND candidate.id=i.automation_artifact_id)
				OR (i.automation_artifact_id IS NULL AND candidate.analysis_job_id=$1))
			ORDER BY candidate.version_number DESC LIMIT 1) a ON TRUE
		WHERE i.test_run_id=$2 AND i.attempt_number=$3 ORDER BY i.id`, claimed.AnalysisJobID,
		claimed.ID, claimed.AttemptCount)
	if err != nil {
		return Run{}, fmt.Errorf("load test run attempt: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item Item
		var artifactID *int64
		var artifactPath, artifactSource, artifactHash, artifactExpected, artifactStatus *string
		destinations := append(itemDestinations(&item), &artifactID, &artifactPath, &artifactSource,
			&artifactHash, &artifactExpected, &artifactStatus)
		if err := rows.Scan(destinations...); err != nil {
			return Run{}, err
		}
		if artifactID != nil {
			item.Artifact = &Artifact{ID: *artifactID, FilePath: value(artifactPath), Source: value(artifactSource),
				SourceHash: value(artifactHash), ExpectedResultHash: value(artifactExpected), Status: value(artifactStatus)}
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return Run{}, err
	}
	if len(result.Items) == 0 {
		return Run{}, fmt.Errorf("%w: run attempt has no items", ErrInvalidInput)
	}
	return result, nil
}

func (r *Repository) Save(ctx context.Context, claimed Run, environment validation.SandboxEnvironment,
	outcomes []Outcome, maxInfraAttempts int, retryDelay time.Duration,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if len(outcomes) == 0 {
		return ErrInvalidInput
	}
	retry := false
	for _, outcome := range outcomes {
		result, err := tx.Exec(ctx, `UPDATE test_run_items SET automation_artifact_id=$4,
			automation_source_hash=$5,status=$6,actual_result=$7,command=$8,exit_code=$9,
			duration_ms=$10,output_truncated=$11 WHERE id=$1 AND test_run_id=$2
			AND attempt_number=$3 AND status='NOT_RUN'`, outcome.ItemID, claimed.ID,
			claimed.AttemptCount, outcome.ArtifactID, outcome.AutomationSourceHash, outcome.Status,
			outcome.ActualResult, outcome.Command, outcome.ExitCode, outcome.DurationMS, outcome.OutputTruncated)
		if err != nil {
			return fmt.Errorf("save test run item %d: %w", outcome.ItemID, err)
		}
		if result.RowsAffected() != 1 {
			return ErrLeaseLost
		}
		for _, evidence := range outcome.Evidence {
			contentHash := evidence.ContentHash
			if contentHash == "" {
				contentHash = hashText(evidence.Content + "\x00" + evidence.StorageKey)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO test_run_evidence(test_run_item_id,evidence_type,content,storage_key,content_hash)
				VALUES($1,$2,$3,$4,$5)`, outcome.ItemID, evidence.EvidenceType,
				evidence.Content, evidence.StorageKey, contentHash); err != nil {
				return err
			}
		}
		if outcome.Status == StatusInfraError && claimed.AttemptCount < maxInfraAttempts {
			retry = true
			_, err = tx.Exec(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,automation_artifact_id,
				attempt_number,status,expected_result_snapshot,expected_result_hash,automation_source_hash)
				SELECT test_run_id,test_case_id,automation_artifact_id,attempt_number+1,'NOT_RUN',
				expected_result_snapshot,expected_result_hash,automation_source_hash FROM test_run_items WHERE id=$1`, outcome.ItemID)
			if err != nil {
				return fmt.Errorf("queue infra retry: %w", err)
			}
		}
	}
	nextStatus := RunCompleted
	finished := true
	if retry {
		nextStatus, finished = RunPending, false
	}
	result, err := tx.Exec(ctx, `UPDATE test_runs SET status=$2,environment='docker',
		environment_fingerprint=$3,image_reference=$4,image_digest=$5,lease_expires_at=NULL,
		next_attempt_at=CASE WHEN $6 THEN NOW()+$7::interval ELSE next_attempt_at END,
		finished_at=CASE WHEN $8 THEN NOW() ELSE NULL END
		WHERE id=$1 AND status='RUNNING' AND attempt_count=$9`, claimed.ID, nextStatus,
		environment.Fingerprint, environment.ImageReference, environment.ImageDigest, retry,
		retryDelay.String(), finished, claimed.AttemptCount)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return tx.Commit(ctx)
}

func (r *Repository) RetryOrFail(ctx context.Context, claimed Run, processErr error,
	maxAttempts int, retryDelay time.Duration,
) error {
	result, err := r.pool.Exec(ctx, `UPDATE test_runs SET status=CASE WHEN attempt_count<$3 THEN 'PENDING' ELSE 'FAILED' END,
		error_message=$2,attempt_count=CASE WHEN attempt_count<$3 THEN attempt_count-1 ELSE attempt_count END,
		next_attempt_at=CASE WHEN attempt_count<$3 THEN NOW()+$4::interval ELSE next_attempt_at END,
		lease_expires_at=NULL,finished_at=CASE WHEN attempt_count<$3 THEN NULL ELSE NOW() END
		WHERE id=$1 AND status='RUNNING' AND attempt_count=$5`, claimed.ID, processErr.Error(),
		maxAttempts, retryDelay.String(), claimed.AttemptCount)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (r *Repository) RenewLease(ctx context.Context, claimed Run, lease time.Duration) error {
	result, err := r.pool.Exec(ctx, `UPDATE test_runs SET lease_expires_at=NOW()+$3::interval
		WHERE id=$1 AND status='RUNNING' AND attempt_count=$2`, claimed.ID, claimed.AttemptCount, lease.String())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id int64) (Run, error) {
	var result Run
	if err := r.pool.QueryRow(ctx, `SELECT id,test_suite_id,suite_release_id,project_id,analysis_job_id,source_sha,target_sha,
		environment,environment_fingerprint,image_reference,image_digest,status,execution_requested_at,
		execution_requested_by,attempt_count,error_message,requested_at,started_at,finished_at
		FROM test_runs WHERE id=$1`, id).Scan(runDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, ErrNotFound
		}
		return Run{}, err
	}
	result.Items, result.Reviews = []Item{}, []ClassificationReview{}
	rows, err := r.pool.Query(ctx, `SELECT i.id,i.test_run_id,i.test_case_id,t.test_case_key,t.title,
		i.expected_result_snapshot,i.expected_result_hash,i.automation_artifact_id,i.automation_source_hash,
		i.attempt_number,i.status,i.actual_result,i.command,i.exit_code,i.duration_ms,i.output_truncated,i.created_at
		FROM test_run_items i JOIN test_cases t ON t.id=i.test_case_id WHERE i.test_run_id=$1
		ORDER BY i.test_case_id,i.attempt_number`, id)
	if err != nil {
		return Run{}, err
	}
	for rows.Next() {
		var item Item
		if err := rows.Scan(itemDestinations(&item)...); err != nil {
			rows.Close()
			return Run{}, err
		}
		item.Evidence = []Evidence{}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Run{}, err
	}
	rows.Close()
	// Close the item cursor before loading evidence so Get also works with a
	// single-connection pool instead of waiting for a second connection.
	for index := range result.Items {
		item := &result.Items[index]
		evidenceRows, evidenceErr := r.pool.Query(ctx, `SELECT id,evidence_type,content,storage_key,content_hash,created_at
			FROM test_run_evidence WHERE test_run_item_id=$1 ORDER BY id`, item.ID)
		if evidenceErr != nil {
			return Run{}, evidenceErr
		}
		for evidenceRows.Next() {
			var evidence Evidence
			if scanErr := evidenceRows.Scan(&evidence.ID, &evidence.EvidenceType, &evidence.Content, &evidence.StorageKey, &evidence.ContentHash, &evidence.CreatedAt); scanErr != nil {
				evidenceRows.Close()
				return Run{}, scanErr
			}
			item.Evidence = append(item.Evidence, evidence)
		}
		if err := evidenceRows.Err(); err != nil {
			evidenceRows.Close()
			return Run{}, err
		}
		evidenceRows.Close()
	}
	reviewRows, err := r.pool.Query(ctx, `SELECT c.id,c.test_run_item_id,c.reviewer_name,c.previous_status,c.new_status,c.reason,c.created_at
		FROM test_run_classification_reviews c JOIN test_run_items i ON i.id=c.test_run_item_id
		WHERE i.test_run_id=$1 ORDER BY c.created_at,c.id`, id)
	if err != nil {
		return Run{}, err
	}
	defer reviewRows.Close()
	for reviewRows.Next() {
		var review ClassificationReview
		if err := reviewRows.Scan(&review.ID, &review.TestRunItemID, &review.ReviewerName, &review.PreviousStatus, &review.NewStatus, &review.Reason, &review.CreatedAt); err != nil {
			return Run{}, err
		}
		result.Reviews = append(result.Reviews, review)
	}
	return result, reviewRows.Err()
}

func (r *Repository) ReviewClassification(ctx context.Context, itemID int64, input ClassificationInput) (Run, error) {
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	input.ReviewerName = strings.TrimSpace(input.ReviewerName)
	input.Reason = strings.TrimSpace(input.Reason)
	if itemID <= 0 || input.ReviewerName == "" || input.Reason == "" || !terminalStatus(input.Status) {
		return Run{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	var runID int64
	var previous string
	err = tx.QueryRow(ctx, `SELECT i.test_run_id,i.status FROM test_run_items i JOIN test_runs r ON r.id=i.test_run_id
		WHERE i.id=$1 AND i.status<>'NOT_RUN' AND r.status<>'RUNNING' FOR UPDATE`, itemID).Scan(&runID, &previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE test_run_items SET status=$2 WHERE id=$1`, itemID, input.Status); err != nil {
		return Run{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO test_run_classification_reviews(test_run_item_id,reviewer_name,previous_status,new_status,reason) VALUES($1,$2,$3,$4,$5)`, itemID, input.ReviewerName, previous, input.Status, input.Reason); err != nil {
		return Run{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	return r.Get(ctx, runID)
}

func runDestinations(run *Run) []any {
	return []any{&run.ID, &run.TestSuiteID, &run.SuiteReleaseID, &run.ProjectID, &run.AnalysisJobID, &run.SourceSHA, &run.TargetSHA, &run.Environment, &run.EnvironmentFingerprint, &run.ImageReference, &run.ImageDigest, &run.Status, &run.ExecutionRequestedAt, &run.ExecutionRequestedBy, &run.AttemptCount, &run.ErrorMessage, &run.RequestedAt, &run.StartedAt, &run.FinishedAt}
}
func itemDestinations(item *Item) []any {
	return []any{&item.ID, &item.TestRunID, &item.TestCaseID, &item.TestCaseKey, &item.Title, &item.ExpectedResult, &item.ExpectedResultHash, &item.AutomationArtifactID, &item.AutomationSourceHash, &item.AttemptNumber, &item.Status, &item.ActualResult, &item.Command, &item.ExitCode, &item.DurationMS, &item.OutputTruncated, &item.CreatedAt}
}
func value(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func terminalStatus(value string) bool {
	switch value {
	case StatusPassed, StatusProductFailed, StatusAutomationError, StatusInfraError, StatusTimedOut, StatusBlocked:
		return true
	}
	return false
}
