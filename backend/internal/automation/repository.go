package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	automationStatus := "AUTOMATABLE"
	if input.Decision == StatusApproved {
		automationStatus = "AUTOMATED"
	}
	if _, err = tx.Exec(ctx, `UPDATE test_cases SET automation_status=$2,updated_at=NOW() WHERE id=$1`, testCaseID, automationStatus); err != nil {
		return ArtifactHistory{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ArtifactHistory{}, err
	}
	return r.History(ctx, testCaseID)
}

func artifactDest(item *Artifact) []any {
	return []any{&item.ID, &item.TestCaseID, &item.VersionNumber, &item.Framework, &item.FilePath, &item.Source, &item.SourceHash, &item.ExpectedResultHash, &item.Status, &item.ModelName, &item.PromptVersion, &item.ProviderResponseID, &item.AnalysisJobID, &item.Setup, &item.Assertions, &item.TestCaseSnapshot, &item.BusinessContext, &item.TechnicalContext, &item.BusinessContextHash, &item.TechnicalContextHash, &item.CreatedAt}
}
