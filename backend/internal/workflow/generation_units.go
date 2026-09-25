package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/aibudget"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type generationOutput struct {
	ReviewProposals bool `json:"review_proposals"`
	ProposalCount   int  `json:"proposal_count"`
	CompletedUnits  int  `json:"completed_units"`
	FailedUnits     int  `json:"failed_units"`
}

func generationKeys(pin generateInputSnapshot) []string {
	keys := []string{}
	for _, req := range pin.Requirements {
		keys = append(keys, fmt.Sprintf("requirement:%d", req.ID))
	}
	if pin.Scope != "SELECTED" {
		keys = append(keys, "retire")
	}
	return keys
}

func (r *Repository) beginGenerationUnit(ctx context.Context, job Job, key string) (Unit, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Unit{}, err
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `SELECT id FROM document_workflow_jobs WHERE id=$1 AND status='RUNNING' AND attempt_count=$2 AND revision=$3 AND cancel_requested_at IS NULL AND lease_expires_at>NOW() FOR UPDATE`, job.ID, job.AttemptCount, job.Revision).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrLeaseLost
	}
	if err != nil {
		return Unit{}, err
	}
	var unit Unit
	err = tx.QueryRow(ctx, `SELECT id,workflow_job_id,unit_key,input_hash,status,attempt_count,output_ref,error_code,error_message,started_at,finished_at,updated_at FROM document_workflow_job_units WHERE workflow_job_id=$1 AND unit_key=$2 FOR UPDATE`, job.ID, key).Scan(unitDest(&unit)...)
	if err != nil {
		return unit, err
	}
	if unit.Status == UnitSucceeded {
		return unit, nil
	}
	err = tx.QueryRow(ctx, `UPDATE document_workflow_job_units SET status='RUNNING',attempt_count=attempt_count+1,error_code='',error_message='',started_at=NOW(),finished_at=NULL,updated_at=NOW() WHERE id=$1 RETURNING id,workflow_job_id,unit_key,input_hash,status,attempt_count,output_ref,error_code,error_message,started_at,finished_at,updated_at`, unit.ID).Scan(unitDest(&unit)...)
	if err != nil {
		return unit, err
	}
	return unit, tx.Commit(ctx)
}

func (r *Repository) failGenerationUnit(ctx context.Context, job Job, unit Unit, cause error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `SELECT id FROM document_workflow_jobs WHERE id=$1 AND status='RUNNING' AND attempt_count=$2 AND revision=$3 AND cancel_requested_at IS NULL AND lease_expires_at>NOW() FOR UPDATE`, job.ID, job.AttemptCount, job.Revision).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}
	code, _ := classifyError(cause)
	_, err = tx.Exec(ctx, `UPDATE document_workflow_job_units SET status='FAILED',error_code=$2,error_message=$3,finished_at=NOW(),updated_at=NOW() WHERE id=$1 AND status='RUNNING'`, unit.ID, code, truncateError(cause))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE document_workflow_jobs SET failed_units=(SELECT count(*) FROM document_workflow_job_units WHERE workflow_job_id=$1 AND unit_key<>'operation' AND status='FAILED') WHERE id=$1`, job.ID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) processGenerationUnits(ctx context.Context, job Job, pin generateInputSnapshot) (any, error) {
	out := generationOutput{ReviewProposals: true}
	for index, key := range generationKeys(pin) {
		unit, err := s.repository.beginGenerationUnit(ctx, job, key)
		if err != nil {
			return nil, err
		}
		if unit.Status == UnitSucceeded {
			var saved struct {
				ProposalCount int `json:"proposal_count"`
			}
			_ = json.Unmarshal(unit.OutputRef, &saved)
			out.CompletedUnits++
			out.ProposalCount += saved.ProposalCount
			continue
		}
		ids := []int64{}
		if key != "retire" {
			ids = []int64{pin.Requirements[index].ID}
		}
		unitCtx := aibudget.WithWorkflowAttempt(ctx, job.ID, unit.ID, unit.AttemptCount)
		result, err := s.testcases.GeneratePinned(unitCtx, job.DocumentSetID, testcase.GenerationBaseline{WorkflowClaimRevision: job.Revision, RequirementIDs: ids, WorkflowJobID: job.ID, WorkflowAttempt: job.AttemptCount, WorkflowUnitID: unit.ID, ReviewProposals: true, IncludeRetire: key == "retire", Targets: pin.Targets, SourceRevision: pin.SourceRevision, InputHash: job.InputHash})
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, testcase.ErrGenerationLease) {
				return nil, err
			}
			if saveErr := s.repository.failGenerationUnit(ctx, job, unit, err); saveErr != nil {
				return nil, saveErr
			}
			out.FailedUnits++
			continue
		}
		out.CompletedUnits++
		out.ProposalCount += result.ProposalCount
	}
	return out, nil
}

func (r *Repository) completeGeneration(ctx context.Context, job Job, out generationOutput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `SELECT id FROM document_workflow_jobs WHERE id=$1 AND status='RUNNING' AND attempt_count=$2 AND revision=$3 AND cancel_requested_at IS NULL AND lease_expires_at>NOW() FOR UPDATE`, job.ID, job.AttemptCount, job.Revision).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}
	// Persisted unit checkpoints, not a worker's in-memory counters, are authoritative.
	var total, completed, failed int
	err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='SUCCEEDED'),count(*) FILTER(WHERE status='FAILED') FROM document_workflow_job_units WHERE workflow_job_id=$1 AND unit_key<>'operation'`, job.ID).Scan(&total, &completed, &failed)
	if err != nil {
		return err
	}
	if total == 0 || completed+failed != total {
		return ErrLeaseLost
	}
	out.CompletedUnits, out.FailedUnits = completed, failed
	status, unitStatus, code, message := StatusSucceeded, UnitSucceeded, "", ""
	if failed > 0 {
		status, unitStatus, code, message = StatusPartialFailed, UnitFailed, "GENERATION_UNITS_FAILED", "Some generation units failed; retry preserves completed proposals"
		if completed == 0 {
			status = StatusFailed
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE document_workflow_job_units SET status=$2,output_ref=$3,error_code=$4,error_message=$5,finished_at=NOW(),updated_at=NOW() WHERE workflow_job_id=$1 AND unit_key='operation'`, job.ID, unitStatus, data, code, message)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE document_workflow_jobs SET status=$2,completed_units=$3,failed_units=$4,output_refs=$5,error_code=$6,error_message=$7,retryable=$8,revision=revision+1,lease_expires_at=NULL,finished_at=NOW(),updated_at=NOW() WHERE id=$1`, job.ID, status, completed, failed, data, code, message, failed > 0)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
