package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Subject struct {
	AnalysisID         int64
	ProjectID          int64
	TestCaseID         int64
	TestCaseKey        string
	Title              string
	ExpectedResult     string
	ExpectedResultHash string
	Snapshot           json.RawMessage
	BusinessContext    json.RawMessage
}

type Call struct {
	AnalysisID       int64
	TestCaseID       int64
	ArtifactID       *int64
	Provider         string
	ModelName        string
	Instructions     string
	Prompt           string
	Schema           map[string]any
	Response         string
	ResponseID       string
	BusinessContext  json.RawMessage
	TechnicalContext json.RawMessage
	ExpectedHash     string
	Status           string
	ErrorMessage     string
	InputTokens      int
	OutputTokens     int
	LatencyMS        int64
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Subject(ctx context.Context, analysisID, testCaseID int64) (Subject, error) {
	var result Subject
	err := r.pool.QueryRow(ctx, `SELECT s.analysis_job_id,s.project_id,t.id,t.test_case_key,t.title,t.expected_result,t.expected_result_hash,
		jsonb_build_object('id',t.id,'key',t.test_case_key,'version',t.version_number,'title',t.title,
		'type',t.test_type,'risk',t.risk,'actor',t.actor,'precondition',t.precondition,'test_data',t.test_data,
		'expected_result',t.expected_result,'expected_result_hash',t.expected_result_hash),
		jsonb_build_object('test_case_snapshot',jsonb_build_object('id',t.id,'key',t.test_case_key,
		'version',t.version_number,'title',t.title,'type',t.test_type,'risk',t.risk,'actor',t.actor,
		'precondition',t.precondition,'test_data',t.test_data,'expected_result',t.expected_result,
		'expected_result_hash',t.expected_result_hash),'approved_requirements',COALESCE((
			SELECT jsonb_agg(requirement_snapshot) FROM jsonb_array_elements(s.requirements) requirement_snapshot
			WHERE (requirement_snapshot->>'id')::bigint IN (
				SELECT link.requirement_id FROM test_case_requirement_links link WHERE link.test_case_id=t.id)
		),'[]'::jsonb),
		'approved_document_versions',s.document_versions,'baseline_hash',s.baseline_hash)
		FROM analysis_baseline_snapshots s JOIN analysis_test_scope_items i ON i.analysis_job_id=s.analysis_job_id
		JOIN test_cases t ON t.id=i.test_case_id WHERE s.analysis_job_id=$1 AND t.id=$2 AND i.included AND t.status='APPROVED'`, analysisID, testCaseID).Scan(
		&result.AnalysisID, &result.ProjectID, &result.TestCaseID, &result.TestCaseKey,
		&result.Title, &result.ExpectedResult, &result.ExpectedResultHash, &result.Snapshot,
		&result.BusinessContext)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subject{}, ErrNotFound
	}
	if err != nil {
		return Subject{}, err
	}
	return result, nil
}

func (r *Repository) SaveCall(ctx context.Context, call Call) (int64, error) {
	schema, _ := json.Marshal(call.Schema)
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO automation_generation_calls(analysis_job_id,test_case_id,artifact_id,provider,model_name,prompt_version,instructions,prompt_text,request_schema,response_text,provider_response_id,business_context,technical_context,expected_result_hash,status,error_message,input_tokens,output_tokens,latency_ms)
	VALUES($1,$2,$3,$4,$5,'document-automation-v1',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING id`, call.AnalysisID, call.TestCaseID, call.ArtifactID, call.Provider, call.ModelName, call.Instructions, call.Prompt, schema, call.Response, call.ResponseID, call.BusinessContext, call.TechnicalContext, call.ExpectedHash, call.Status, call.ErrorMessage, call.InputTokens, call.OutputTokens, call.LatencyMS).Scan(&id)
	return id, err
}

func (r *Repository) SaveArtifact(ctx context.Context, subject Subject, proposal proposedArtifact, business, technical json.RawMessage, businessHash, technicalHash, model, responseID string, call Call) (Artifact, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Artifact{}, err
	}
	defer tx.Rollback(ctx)
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version_number),0)+1 FROM automation_artifacts WHERE test_case_id=$1`, subject.TestCaseID).Scan(&version); err != nil {
		return Artifact{}, err
	}
	sourceHash := hash([]byte(proposal.Code))
	assertions, _ := json.Marshal(proposal.Assertions)
	var result Artifact
	err = tx.QueryRow(ctx, `INSERT INTO automation_artifacts(test_case_id,version_number,framework,file_path,source,source_hash,expected_result_hash,status,model_name,prompt_version,provider_response_id,analysis_job_id,setup_text,assertions,test_case_snapshot,business_context,technical_context,business_context_hash,technical_context_hash)
		VALUES($1,$2,$3,$4,$5,$6,$7,'DRAFT',$8,'document-automation-v1',$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id,test_case_id,version_number,framework,file_path,source,source_hash,expected_result_hash,status,model_name,prompt_version,provider_response_id,analysis_job_id,setup_text,assertions,test_case_snapshot,business_context,technical_context,business_context_hash,technical_context_hash,created_at`, subject.TestCaseID, version, proposal.Framework, proposal.TargetFile, proposal.Code, sourceHash, subject.ExpectedResultHash, model, responseID, subject.AnalysisID, proposal.Setup, assertions, subject.Snapshot, business, technical, businessHash, technicalHash).Scan(artifactDest(&result)...)
	if err != nil {
		return Artifact{}, fmt.Errorf("save automation artifact: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE test_cases SET automation_status='AUTOMATABLE',updated_at=NOW() WHERE id=$1`, subject.TestCaseID); err != nil {
		return Artifact{}, err
	}
	schema, _ := json.Marshal(call.Schema)
	if _, err = tx.Exec(ctx, `INSERT INTO automation_generation_calls(analysis_job_id,test_case_id,artifact_id,provider,model_name,prompt_version,instructions,prompt_text,request_schema,response_text,provider_response_id,business_context,technical_context,expected_result_hash,status,error_message,input_tokens,output_tokens,latency_ms)
		VALUES($1,$2,$3,$4,$5,'document-automation-v1',$6,$7,$8,$9,$10,$11,$12,$13,'COMPLETED','',$14,$15,$16)`,
		call.AnalysisID, call.TestCaseID, result.ID, call.Provider, call.ModelName,
		call.Instructions, call.Prompt, schema, call.Response, call.ResponseID,
		call.BusinessContext, call.TechnicalContext, call.ExpectedHash, call.InputTokens,
		call.OutputTokens, call.LatencyMS); err != nil {
		return Artifact{}, fmt.Errorf("save completed automation provenance: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Artifact{}, err
	}
	return result, nil
}

func (r *Repository) History(ctx context.Context, testCaseID int64) (ArtifactHistory, error) {
	result := ArtifactHistory{Artifacts: []Artifact{}, Reviews: []Review{}}
	rows, err := r.pool.Query(ctx, `SELECT id,test_case_id,version_number,framework,file_path,source,source_hash,expected_result_hash,status,model_name,prompt_version,provider_response_id,analysis_job_id,setup_text,assertions,test_case_snapshot,business_context,technical_context,business_context_hash,technical_context_hash,created_at FROM automation_artifacts WHERE test_case_id=$1 ORDER BY version_number DESC`, testCaseID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item Artifact
		if err := rows.Scan(artifactDest(&item)...); err != nil {
			rows.Close()
			return result, err
		}
		result.Artifacts = append(result.Artifacts, item)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT r.id,r.automation_artifact_id,r.reviewer_name,r.decision,r.comment,r.created_at FROM automation_artifact_reviews r JOIN automation_artifacts a ON a.id=r.automation_artifact_id WHERE a.test_case_id=$1 ORDER BY r.created_at DESC,r.id DESC`, testCaseID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item Review
		if err := rows.Scan(&item.ID, &item.AutomationArtifactID, &item.ReviewerName, &item.Decision, &item.Comment, &item.CreatedAt); err != nil {
			rows.Close()
			return result, err
		}
		result.Reviews = append(result.Reviews, item)
	}
	rows.Close()
	return result, nil
}

func (r *Repository) MarkBlocked(ctx context.Context, testCaseID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE test_cases SET automation_status='BLOCKED',updated_at=NOW() WHERE id=$1`, testCaseID)
	return err
}

func (r *Repository) Review(ctx context.Context, id int64, input ReviewInput) (ArtifactHistory, error) {
	input.Decision = strings.ToUpper(strings.TrimSpace(input.Decision))
	input.ReviewerName = strings.TrimSpace(input.ReviewerName)
	input.Comment = strings.TrimSpace(input.Comment)
	if id <= 0 || input.ReviewerName == "" || input.Decision != StatusApproved && input.Decision != StatusRejected {
		return ArtifactHistory{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ArtifactHistory{}, err
	}
	defer tx.Rollback(ctx)
	var testCaseID int64
	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT test_case_id,status FROM automation_artifacts WHERE id=$1 FOR UPDATE`, id).Scan(&testCaseID, &currentStatus); errors.Is(err, pgx.ErrNoRows) {
		return ArtifactHistory{}, ErrNotFound
	} else if err != nil {
		return ArtifactHistory{}, err
	}
	if currentStatus != StatusDraft {
		return ArtifactHistory{}, ErrAlreadyReviewed
	}
	if _, err = tx.Exec(ctx, `UPDATE automation_artifacts SET status=$2 WHERE id=$1`, id, input.Decision); err != nil {
		return ArtifactHistory{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO automation_artifact_reviews(automation_artifact_id,reviewer_name,decision,comment) VALUES($1,$2,$3,$4)`, id, input.ReviewerName, input.Decision, input.Comment); err != nil {
		return ArtifactHistory{}, err
	}
	var repairSourceItemID int64
	repairErr := tx.QueryRow(ctx, `SELECT test_run_item_id FROM automation_repair_jobs
		WHERE repaired_artifact_id=$1 AND status='WAITING_REVIEW' FOR UPDATE`, id).Scan(&repairSourceItemID)
	if repairErr != nil && !errors.Is(repairErr, pgx.ErrNoRows) {
		return ArtifactHistory{}, repairErr
	}
	hasRepair := repairErr == nil
	if _, err = tx.Exec(ctx, `UPDATE automation_repair_jobs SET status=$2,finished_at=NOW()
		WHERE repaired_artifact_id=$1 AND status='WAITING_REVIEW'`, id, input.Decision); err != nil {
		return ArtifactHistory{}, err
	}
	automationStatus := "AUTOMATABLE"
	if input.Decision == StatusApproved {
		automationStatus = "AUTOMATED"
	}
	if _, err = tx.Exec(ctx, `UPDATE test_cases SET automation_status=$2,updated_at=NOW() WHERE id=$1`, testCaseID, automationStatus); err != nil {
		return ArtifactHistory{}, err
	}
	if input.Decision == StatusApproved && hasRepair {
		var runID int64
		var nextAttempt int
		if err = tx.QueryRow(ctx, `SELECT run.id FROM test_run_items source
			JOIN test_runs run ON run.id=source.test_run_id
			WHERE source.id=$1 AND source.status='AUTOMATION_ERROR'
			AND run.status IN ('COMPLETED','FAILED') FOR UPDATE OF run`, repairSourceItemID).Scan(&runID); errors.Is(err, pgx.ErrNoRows) {
			return ArtifactHistory{}, ErrRepairNotEligible
		} else if err != nil {
			return ArtifactHistory{}, err
		}
		if err = tx.QueryRow(ctx, `SELECT COALESCE(max(attempt_number),0)+1 FROM test_run_items
			WHERE test_run_id=$1`, runID).Scan(&nextAttempt); err != nil {
			return ArtifactHistory{}, err
		}
		result, insertErr := tx.Exec(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,
			automation_artifact_id,automation_source_hash,attempt_number,status,
			expected_result_snapshot,expected_result_hash)
			SELECT source.test_run_id,source.test_case_id,$2,artifact.source_hash,$3,'NOT_RUN',
				source.expected_result_snapshot,source.expected_result_hash
			FROM test_run_items source JOIN automation_artifacts artifact ON artifact.id=$2
			WHERE source.id=$1 AND artifact.status='APPROVED'
			AND artifact.test_case_id=source.test_case_id
			AND artifact.expected_result_hash=source.expected_result_hash`, repairSourceItemID, id, nextAttempt)
		if insertErr != nil {
			return ArtifactHistory{}, insertErr
		}
		if result.RowsAffected() != 1 {
			return ArtifactHistory{}, ErrRepairNotEligible
		}
		result, err = tx.Exec(ctx, `UPDATE test_runs SET status='PENDING',attempt_count=$2-1,
			execution_requested_at=NOW(),execution_requested_by=$3,next_attempt_at=NOW(),
			lease_expires_at=NULL,finished_at=NULL,error_message='' WHERE id=$1
			AND status IN ('COMPLETED','FAILED')`, runID, nextAttempt, input.ReviewerName)
		if err != nil {
			return ArtifactHistory{}, err
		}
		if result.RowsAffected() != 1 {
			return ArtifactHistory{}, ErrRepairNotEligible
		}
	}
	if input.Decision == StatusApproved {
		// Automatically queue the document-driven run only when every scoped case
		// has an approved artifact generated for this exact analysis. Runs with
		// manual/blocked cases remain reviewable and can be explicitly requested.
		if _, err = tx.Exec(ctx, `UPDATE test_runs r SET execution_requested_at=COALESCE(r.execution_requested_at,NOW()),
			execution_requested_by=CASE WHEN r.execution_requested_by='' THEN 'SYSTEM_ARTIFACT_APPROVAL' ELSE r.execution_requested_by END,
			next_attempt_at=NOW()
			WHERE r.status='PENDING' AND r.analysis_job_id=(SELECT analysis_job_id FROM automation_artifacts WHERE id=$1)
			AND NOT EXISTS (SELECT 1 FROM test_run_items i WHERE i.test_run_id=r.id AND i.attempt_number=1
				AND NOT EXISTS (SELECT 1 FROM automation_artifacts candidate
					WHERE candidate.test_case_id=i.test_case_id AND candidate.analysis_job_id=r.analysis_job_id
					AND candidate.status='APPROVED'))`, id); err != nil {
			return ArtifactHistory{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return ArtifactHistory{}, err
	}
	return r.History(ctx, testCaseID)
}

var repairPolicy = json.RawMessage(`{"mutable":["automation_source","setup_text"],"immutable":["requirements","test_case_snapshot","expected_result","expected_result_hash","semantic_assertions","production_source"]}`)

func (r *Repository) RequestRepair(ctx context.Context, itemID int64, input RepairRequest,
	maxAttempts, maxOutputTokens int, maxCostMicroUSD int64,
) (RepairJob, error) {
	input.RequestedBy = strings.TrimSpace(input.RequestedBy)
	if itemID <= 0 || input.RequestedBy == "" || maxAttempts < 1 || maxAttempts > 3 ||
		maxOutputTokens < 100 || maxOutputTokens > 10000 || maxCostMicroUSD < 0 {
		return RepairJob{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RepairJob{}, err
	}
	defer tx.Rollback(ctx)
	var artifactID, runID, testCaseID int64
	var status, actual, expectedHash, artifactExpectedHash, sourceHash string
	var assertions json.RawMessage
	err = tx.QueryRow(ctx, `SELECT i.test_run_id,i.test_case_id,i.status,i.actual_result,i.expected_result_hash,a.id,a.expected_result_hash,
		a.source_hash,a.assertions FROM test_run_items i
		JOIN test_runs run ON run.id=i.test_run_id
		JOIN automation_artifacts a ON a.id=i.automation_artifact_id AND a.test_case_id=i.test_case_id
		WHERE i.id=$1 AND a.status='APPROVED' FOR UPDATE OF i,run`, itemID).Scan(
		&runID, &testCaseID, &status, &actual, &expectedHash, &artifactID, &artifactExpectedHash, &sourceHash, &assertions)
	if errors.Is(err, pgx.ErrNoRows) {
		return RepairJob{}, ErrNotFound
	}
	if err != nil {
		return RepairJob{}, err
	}
	if status != "AUTOMATION_ERROR" {
		return RepairJob{}, ErrRepairNotEligible
	}
	if artifactExpectedHash != expectedHash {
		return RepairJob{}, ErrRepairNotEligible
	}
	var existing RepairJob
	err = tx.QueryRow(ctx, repairSelect+` JOIN test_run_items repair_item ON repair_item.id=j.test_run_item_id
		WHERE repair_item.test_run_id=$1 AND repair_item.test_case_id=$2
		AND j.status IN ('PENDING','RUNNING','WAITING_REVIEW') ORDER BY j.id DESC LIMIT 1`, runID, testCaseID).Scan(repairDest(&existing)...)
	if err == nil {
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RepairJob{}, err
	}
	var attempt int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(job.attempt_number),0)+1
		FROM automation_repair_jobs job JOIN test_run_items repair_item ON repair_item.id=job.test_run_item_id
		WHERE repair_item.test_run_id=$1 AND repair_item.test_case_id=$2`, runID, testCaseID).Scan(&attempt); err != nil {
		return RepairJob{}, err
	}
	if attempt > maxAttempts {
		return RepairJob{}, ErrRepairLimit
	}
	result := RepairJob{}
	err = tx.QueryRow(ctx, `INSERT INTO automation_repair_jobs AS j(test_run_item_id,source_artifact_id,
		attempt_number,status,error_type,reason,requested_by,allowed_change_policy,expected_result_hash,
		before_source_hash,before_assertions,max_output_tokens,max_cost_microusd)
		VALUES($1,$2,$3,'PENDING','AUTOMATION_ERROR',$4,$5,$6,$7,$8,$9,$10,$11) RETURNING `+
		repairColumns, itemID, artifactID, attempt, boundedText(actual, 16000), input.RequestedBy,
		repairPolicy, expectedHash, sourceHash, assertions, maxOutputTokens, maxCostMicroUSD).Scan(repairDest(&result)...)
	if err != nil {
		return RepairJob{}, fmt.Errorf("enqueue automation repair: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return RepairJob{}, err
	}
	return result, nil
}

func (r *Repository) ListRepairs(ctx context.Context, itemID int64) ([]RepairJob, error) {
	if itemID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := r.pool.Query(ctx, repairSelect+` JOIN test_run_items repair_item ON repair_item.id=j.test_run_item_id
		JOIN test_run_items target_item ON target_item.id=$1
		WHERE repair_item.test_run_id=target_item.test_run_id
		AND repair_item.test_case_id=target_item.test_case_id
		ORDER BY j.attempt_number DESC,j.id DESC`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RepairJob{}
	for rows.Next() {
		var item RepairJob
		if err := rows.Scan(repairDest(&item)...); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ClaimRepair(ctx context.Context, lease time.Duration) (RepairJob, error) {
	var result RepairJob
	err := r.pool.QueryRow(ctx, `WITH candidate AS (SELECT id FROM automation_repair_jobs
		WHERE status='PENDING' AND next_attempt_at<=NOW() AND (lease_expires_at IS NULL OR lease_expires_at<=NOW())
		ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
		UPDATE automation_repair_jobs j SET status='RUNNING',queue_attempt_count=queue_attempt_count+1,
		started_at=COALESCE(started_at,NOW()),lease_expires_at=NOW()+$1::interval,error_message=''
		FROM candidate WHERE j.id=candidate.id RETURNING `+repairColumns, lease.String()).Scan(repairDest(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return RepairJob{}, ErrNotFound
	}
	return result, err
}

func (r *Repository) LoadRepairSubject(ctx context.Context, job RepairJob) (RepairSubject, error) {
	var result RepairSubject
	result.Job = job
	var evidence json.RawMessage
	err := r.pool.QueryRow(ctx, `SELECT a.id,a.test_case_id,a.version_number,a.framework,a.file_path,a.source,
		a.source_hash,a.expected_result_hash,a.status,a.model_name,a.prompt_version,a.provider_response_id,
		a.analysis_job_id,a.setup_text,a.assertions,a.test_case_snapshot,a.business_context,a.technical_context,
		a.business_context_hash,a.technical_context_hash,a.created_at,i.actual_result,
		COALESCE((SELECT jsonb_agg(jsonb_build_object('type',e.evidence_type,'content',e.content,
			'storage_key',e.storage_key,'content_hash',e.content_hash) ORDER BY e.id)
			FROM test_run_evidence e WHERE e.test_run_item_id=i.id),'[]'::jsonb)
		FROM automation_repair_jobs j JOIN test_run_items i ON i.id=j.test_run_item_id
		JOIN automation_artifacts a ON a.id=j.source_artifact_id
		WHERE j.id=$1 AND j.status='RUNNING' AND j.queue_attempt_count=$2
		AND i.status='AUTOMATION_ERROR' AND i.expected_result_hash=j.expected_result_hash
		AND a.source_hash=j.before_source_hash AND a.expected_result_hash=j.expected_result_hash`,
		job.ID, job.QueueAttemptCount).Scan(append(artifactDest(&result.Artifact), &result.ActualResult, &evidence)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return RepairSubject{}, ErrRepairLeaseLost
	}
	if err != nil {
		return RepairSubject{}, err
	}
	result.Evidence = evidence
	result.TestCaseID = result.Artifact.TestCaseID
	if result.Artifact.AnalysisJobID != nil {
		result.AnalysisID = *result.Artifact.AnalysisJobID
	}
	return result, nil
}

type RepairCompletion struct {
	Proposal              proposedArtifact
	AfterSourceHash       string
	ModelName             string
	ProviderResponseID    string
	Prompt                string
	Response              string
	InputTokens           int
	OutputTokens          int
	EstimatedCostMicroUSD int64
}

func (r *Repository) CompleteRepair(ctx context.Context, job RepairJob, subject RepairSubject,
	completion RepairCompletion,
) (RepairJob, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RepairJob{}, err
	}
	defer tx.Rollback(ctx)
	var version int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(version_number),0)+1 FROM automation_artifacts
		WHERE test_case_id=$1`, subject.Artifact.TestCaseID).Scan(&version); err != nil {
		return RepairJob{}, err
	}
	assertions, _ := json.Marshal(completion.Proposal.Assertions)
	var artifactID int64
	err = tx.QueryRow(ctx, `INSERT INTO automation_artifacts(test_case_id,version_number,framework,file_path,
		source,source_hash,expected_result_hash,status,model_name,prompt_version,provider_response_id,
		analysis_job_id,setup_text,assertions,test_case_snapshot,business_context,technical_context,
		business_context_hash,technical_context_hash)
		VALUES($1,$2,$3,$4,$5,$6,$7,'DRAFT',$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING id`,
		subject.Artifact.TestCaseID, version, completion.Proposal.Framework, completion.Proposal.TargetFile,
		completion.Proposal.Code, completion.AfterSourceHash, subject.Artifact.ExpectedResultHash,
		completion.ModelName, RepairPromptVersion, completion.ProviderResponseID,
		subject.Artifact.AnalysisJobID, completion.Proposal.Setup, assertions,
		subject.Artifact.TestCaseSnapshot, subject.Artifact.BusinessContext, subject.Artifact.TechnicalContext,
		subject.Artifact.BusinessContextHash, subject.Artifact.TechnicalContextHash).Scan(&artifactID)
	if err != nil {
		return RepairJob{}, err
	}
	var result RepairJob
	err = tx.QueryRow(ctx, `UPDATE automation_repair_jobs AS j SET status='WAITING_REVIEW',repaired_artifact_id=$3,
		after_source_hash=$4,after_assertions=$5,model_name=$6,provider_response_id=$7,prompt_text=$8,
		response_text=$9,input_tokens=$10,output_tokens=$11,estimated_cost_microusd=$12,
		lease_expires_at=NULL,finished_at=NOW() WHERE id=$1 AND status='RUNNING' AND queue_attempt_count=$2
		RETURNING `+repairColumns, job.ID, job.QueueAttemptCount, artifactID, completion.AfterSourceHash,
		assertions, completion.ModelName, completion.ProviderResponseID, completion.Prompt,
		completion.Response, completion.InputTokens, completion.OutputTokens,
		completion.EstimatedCostMicroUSD).Scan(repairDest(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return RepairJob{}, ErrRepairLeaseLost
	}
	if err != nil {
		return RepairJob{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RepairJob{}, err
	}
	return result, nil
}

func (r *Repository) MarkRepairUnrepairable(ctx context.Context, job RepairJob, message, prompt,
	response, model, responseID string, inputTokens, outputTokens int, cost int64,
) error {
	result, err := r.pool.Exec(ctx, `UPDATE automation_repair_jobs SET status='UNREPAIRABLE',
		error_message=$3,prompt_text=$4,response_text=$5,model_name=$6,provider_response_id=$7,
		input_tokens=$8,output_tokens=$9,estimated_cost_microusd=$10,lease_expires_at=NULL,finished_at=NOW()
		WHERE id=$1 AND status='RUNNING' AND queue_attempt_count=$2`, job.ID, job.QueueAttemptCount,
		boundedText(message, 16000), prompt, response, model, responseID, inputTokens, outputTokens, cost)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrRepairLeaseLost
	}
	return nil
}

func (r *Repository) RetryOrFailRepair(ctx context.Context, job RepairJob, processErr error,
	maxQueueAttempts int, delay time.Duration,
) error {
	result, err := r.pool.Exec(ctx, `UPDATE automation_repair_jobs SET
		status=CASE WHEN queue_attempt_count<$3 THEN 'PENDING' ELSE 'FAILED' END,
		error_message=$4,next_attempt_at=CASE WHEN queue_attempt_count<$3 THEN NOW()+$5::interval ELSE next_attempt_at END,
		lease_expires_at=NULL,finished_at=CASE WHEN queue_attempt_count<$3 THEN NULL ELSE NOW() END
		WHERE id=$1 AND status='RUNNING' AND queue_attempt_count=$2`, job.ID, job.QueueAttemptCount,
		maxQueueAttempts, boundedText(processErr.Error(), 16000), delay.String())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrRepairLeaseLost
	}
	return nil
}

func (r *Repository) RenewRepairLease(ctx context.Context, job RepairJob, lease time.Duration) error {
	result, err := r.pool.Exec(ctx, `UPDATE automation_repair_jobs SET lease_expires_at=NOW()+$3::interval
		WHERE id=$1 AND status='RUNNING' AND queue_attempt_count=$2`, job.ID, job.QueueAttemptCount, lease.String())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrRepairLeaseLost
	}
	return nil
}

const repairColumns = `j.id,j.test_run_item_id,j.source_artifact_id,j.repaired_artifact_id,
	j.attempt_number,j.status,j.error_type,j.reason,j.requested_by,j.allowed_change_policy,
	j.expected_result_hash,j.before_source_hash,COALESCE(j.after_source_hash,''),j.before_assertions,
	COALESCE(j.after_assertions,'[]'::jsonb),j.model_name,j.prompt_version,j.provider_response_id,
	j.input_tokens,j.output_tokens,j.max_output_tokens,j.max_cost_microusd,j.estimated_cost_microusd,
	j.queue_attempt_count,j.error_message,j.created_at,j.started_at,j.finished_at`
const repairSelect = `SELECT ` + repairColumns + ` FROM automation_repair_jobs j`

func repairDest(item *RepairJob) []any {
	return []any{&item.ID, &item.TestRunItemID, &item.SourceArtifactID, &item.RepairedArtifactID,
		&item.AttemptNumber, &item.Status, &item.ErrorType, &item.Reason, &item.RequestedBy,
		&item.AllowedChangePolicy, &item.ExpectedResultHash, &item.BeforeSourceHash,
		&item.AfterSourceHash, &item.BeforeAssertions, &item.AfterAssertions, &item.ModelName,
		&item.PromptVersion, &item.ProviderResponseID, &item.InputTokens, &item.OutputTokens,
		&item.MaxOutputTokens, &item.MaxCostMicroUSD, &item.EstimatedCostMicroUSD,
		&item.QueueAttemptCount, &item.ErrorMessage, &item.CreatedAt, &item.StartedAt, &item.FinishedAt}
}

func boundedText(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func artifactDest(item *Artifact) []any {
	return []any{&item.ID, &item.TestCaseID, &item.VersionNumber, &item.Framework, &item.FilePath, &item.Source, &item.SourceHash, &item.ExpectedResultHash, &item.Status, &item.ModelName, &item.PromptVersion, &item.ProviderResponseID, &item.AnalysisJobID, &item.Setup, &item.Assertions, &item.TestCaseSnapshot, &item.BusinessContext, &item.TechnicalContext, &item.BusinessContextHash, &item.TechnicalContextHash, &item.CreatedAt}
}
