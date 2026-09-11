package requirement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) SaveProposal(ctx context.Context, setID int64, chunk document.SemanticChunk,
	proposal Proposal,
) (Requirement, bool, error) {
	if chunk.DocumentBlockID <= 0 || chunk.DocumentVersionID <= 0 || chunk.DocumentSetID != setID {
		return Requirement{}, false, ErrMissingEvidence
	}
	if err := normalizeProposal(&proposal); err != nil {
		return Requirement{}, false, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	key := requirementKey(proposal, chunk)
	extractionKey := hashText(strings.ToUpper(proposal.Identifier) + "\x00" + proposal.FlowType + "\x00" + normalizeForDedupe(proposal.Statement))
	if proposal.Assumptions == nil {
		proposal.Assumptions = []string{}
	}
	assumptions, _ := json.Marshal(proposal.Assumptions)
	rawPayload, _ := json.Marshal(proposal)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Requirement{}, false, fmt.Errorf("begin save requirement: %w", err)
	}
	defer tx.Rollback(ctx)
	const insert = `INSERT INTO requirements
		(document_set_id,requirement_key,version_number,title,statement,requirement_type,
		 flow_type,actor,precondition,postcondition,priority,risk,status,confidence,
		 extraction_key,source_fingerprint,assumptions,raw_payload)
		VALUES($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT(document_set_id,extraction_key) WHERE extraction_key<>'' DO NOTHING
		RETURNING id,document_set_id,requirement_key,version_number,title,statement,
		requirement_type,flow_type,actor,precondition,postcondition,priority,risk,status,
		confidence,supersedes_requirement_id,assumptions,extraction_key,source_fingerprint,
		created_at,updated_at`
	var result Requirement
	err = tx.QueryRow(ctx, insert, setID, key, proposal.Title, proposal.Statement,
		proposal.RequirementType, proposal.FlowType, proposal.Actor, proposal.Precondition,
		proposal.Postcondition, proposal.Priority, proposal.Risk, proposal.Status,
		proposal.Confidence, extractionKey, chunk.ContentHash, assumptions, rawPayload).
		Scan(requirementDestinations(&result)...)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id,document_set_id,requirement_key,version_number,title,
			statement,requirement_type,flow_type,actor,precondition,postcondition,priority,risk,
			status,confidence,supersedes_requirement_id,assumptions,extraction_key,
			source_fingerprint,created_at,updated_at FROM requirements
			WHERE document_set_id=$1 AND extraction_key=$2`, setID, extractionKey).
			Scan(requirementDestinations(&result)...)
	}
	if err != nil {
		return Requirement{}, false, fmt.Errorf("save extracted requirement: %w", err)
	}
	excerptHash := hashText(chunk.RawContent)
	_, err = tx.Exec(ctx, `INSERT INTO requirement_evidence
		(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(requirement_id,document_block_id) DO NOTHING`,
		result.ID, setID, chunk.DocumentVersionID, chunk.DocumentBlockID,
		chunk.SourceLocator, excerptHash)
	if err != nil {
		return Requirement{}, false, fmt.Errorf("save requirement evidence: %w", err)
	}
	for index, step := range proposal.Steps {
		if _, err := tx.Exec(ctx, `INSERT INTO requirement_flow_steps
			(requirement_id,ordinal,action,expected_result) VALUES($1,$2,$3,$4)
			ON CONFLICT(requirement_id,ordinal) DO NOTHING`, result.ID, index+1,
			step.Action, step.ExpectedResult); err != nil {
			return Requirement{}, false, fmt.Errorf("save requirement flow step: %w", err)
		}
	}
	if proposal.Status == StatusTBD {
		question := "Cần làm rõ chi tiết còn thiếu cho " + result.RequirementKey + ": " + truncate(proposal.Statement, 240)
		if _, err := tx.Exec(ctx, `INSERT INTO open_questions
			(document_set_id,requirement_id,question,owner_role) VALUES($1,$2,$3,'PO/BA')
			ON CONFLICT(document_set_id,requirement_id,question) DO NOTHING`, setID, result.ID, question); err != nil {
			return Requirement{}, false, fmt.Errorf("save requirement open question: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Requirement{}, false, fmt.Errorf("commit requirement: %w", err)
	}
	return result, created, nil
}

func (r *Repository) SaveAICall(ctx context.Context, call AICall) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO document_ai_calls
		(document_set_id,phase,subject_type,subject_key,context_snapshot_id,provider,model_name,
		 prompt_version,instructions,prompt_text,request_schema,response_text,provider_response_id,
		 status,error_message,input_tokens,output_tokens,latency_ms)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		call.DocumentSetID, call.Phase, call.SubjectType, call.SubjectKey,
		call.ContextSnapshotID, call.Provider, call.ModelName, call.PromptVersion,
		call.Instructions, call.PromptText, call.RequestSchema, call.ResponseText,
		call.ProviderResponseID, call.Status, call.ErrorMessage, call.InputTokens,
		call.OutputTokens, call.LatencyMS)
	if err != nil {
		return fmt.Errorf("save document AI call: %w", err)
	}
	return nil
}

func (r *Repository) MarkConflict(ctx context.Context, setID, leftID, rightID int64,
	reason string,
) (bool, error) {
	if leftID > rightID {
		leftID, rightID = rightID, leftID
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin requirement conflict: %w", err)
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `INSERT INTO requirement_conflicts
		(document_set_id,left_requirement_id,right_requirement_id,reason)
		VALUES($1,$2,$3,$4) ON CONFLICT(document_set_id,left_requirement_id,right_requirement_id) DO NOTHING`,
		setID, leftID, rightID, reason)
	if err != nil {
		return false, fmt.Errorf("save requirement conflict: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE requirements SET status='CONFLICT',updated_at=NOW()
		WHERE id=ANY($1) AND status IN ('DRAFT','TBD')`, []int64{leftID, rightID}); err != nil {
		return false, fmt.Errorf("mark conflicting requirements: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit requirement conflict: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *Repository) List(ctx context.Context, filter Filter) ([]Requirement, error) {
	const query = `SELECT r.id,r.document_set_id,r.requirement_key,r.version_number,r.title,
		r.statement,r.requirement_type,r.flow_type,r.actor,r.precondition,r.postcondition,
		r.priority,r.risk,r.status,r.confidence,r.supersedes_requirement_id,r.assumptions,
		r.extraction_key,r.source_fingerprint,r.created_at,r.updated_at,
		COALESCE((SELECT array_agg(DISTINCT v.document_id ORDER BY v.document_id)
			FROM requirement_evidence e JOIN document_versions v ON v.id=e.document_version_id
			WHERE e.requirement_id=r.id),'{}')
		FROM requirements r
		WHERE r.document_set_id=$1
		AND ($2='' OR r.requirement_type=$2) AND ($3='' OR r.status=$3)
		AND ($4='' OR r.risk=$4) AND ($5='' OR lower(r.actor)=lower($5))
		AND ($6='' OR r.flow_type=$6)
		AND ($7=0 OR EXISTS(SELECT 1 FROM requirement_evidence e
			JOIN document_versions v ON v.id=e.document_version_id
			WHERE e.requirement_id=r.id AND v.document_id=$7))
		AND NOT EXISTS(SELECT 1 FROM requirements newer WHERE newer.supersedes_requirement_id=r.id)
		ORDER BY r.requirement_key,r.version_number DESC,r.id`
	rows, err := r.pool.Query(ctx, query, filter.DocumentSetID, strings.ToUpper(filter.RequirementType),
		strings.ToUpper(filter.Status), strings.ToUpper(filter.Risk), strings.TrimSpace(filter.Actor),
		strings.ToUpper(filter.FlowType), filter.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("list requirements: %w", err)
	}
	defer rows.Close()
	results := make([]Requirement, 0)
	for rows.Next() {
		var item Requirement
		destinations := append(requirementDestinations(&item), &item.DocumentIDs)
		if err := rows.Scan(destinations...); err != nil {
			return nil, fmt.Errorf("scan requirement: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id int64) (Detail, error) {
	detail := Detail{
		Evidence:  make([]Evidence, 0),
		FlowSteps: make([]FlowStep, 0),
		Reviews:   make([]Review, 0),
	}
	const requirementQuery = `SELECT id,document_set_id,requirement_key,version_number,title,
		statement,requirement_type,flow_type,actor,precondition,postcondition,priority,risk,
		status,confidence,supersedes_requirement_id,assumptions,extraction_key,
		source_fingerprint,created_at,updated_at FROM requirements WHERE id=$1`
	if err := r.pool.QueryRow(ctx, requirementQuery, id).Scan(requirementDestinations(&detail.Requirement)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get requirement: %w", err)
	}
	evidenceRows, err := r.pool.Query(ctx, `SELECT e.id,e.requirement_id,e.document_set_id,
		e.document_version_id,e.document_block_id,d.name,v.version_number,v.approval_status,
		e.source_locator,b.content,e.excerpt_hash,e.created_at
		FROM requirement_evidence e JOIN document_versions v ON v.id=e.document_version_id
		JOIN documents d ON d.id=v.document_id JOIN document_blocks b ON b.id=e.document_block_id
		WHERE e.requirement_id=$1 ORDER BY e.id`, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list requirement evidence: %w", err)
	}
	for evidenceRows.Next() {
		var item Evidence
		if err := evidenceRows.Scan(&item.ID, &item.RequirementID, &item.DocumentSetID,
			&item.DocumentVersionID, &item.DocumentBlockID, &item.DocumentName,
			&item.VersionNumber, &item.ApprovalStatus, &item.SourceLocator, &item.Excerpt,
			&item.ExcerptHash, &item.CreatedAt); err != nil {
			evidenceRows.Close()
			return Detail{}, fmt.Errorf("scan requirement evidence: %w", err)
		}
		detail.Evidence = append(detail.Evidence, item)
	}
	evidenceRows.Close()
	stepRows, err := r.pool.Query(ctx, `SELECT id,requirement_id,ordinal,action,expected_result,
		created_at FROM requirement_flow_steps WHERE requirement_id=$1 ORDER BY ordinal`, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list requirement flow steps: %w", err)
	}
	for stepRows.Next() {
		var item FlowStep
		if err := stepRows.Scan(&item.ID, &item.RequirementID, &item.Ordinal, &item.Action,
			&item.ExpectedResult, &item.CreatedAt); err != nil {
			stepRows.Close()
			return Detail{}, fmt.Errorf("scan requirement flow step: %w", err)
		}
		detail.FlowSteps = append(detail.FlowSteps, item)
	}
	stepRows.Close()
	reviewRows, err := r.pool.Query(ctx, `SELECT id,requirement_id,reviewer_name,decision,comment,
		created_at FROM requirement_reviews WHERE requirement_id=$1 ORDER BY created_at,id`, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list requirement reviews: %w", err)
	}
	for reviewRows.Next() {
		var item Review
		if err := reviewRows.Scan(&item.ID, &item.RequirementID, &item.ReviewerName,
			&item.Decision, &item.Comment, &item.CreatedAt); err != nil {
			reviewRows.Close()
			return Detail{}, fmt.Errorf("scan requirement review: %w", err)
		}
		detail.Reviews = append(detail.Reviews, item)
	}
	reviewRows.Close()
	return detail, nil
}

func (r *Repository) Review(ctx context.Context, id int64, input ReviewInput) (Detail, error) {
	input.ReviewerName = strings.TrimSpace(input.ReviewerName)
	input.Decision = strings.ToUpper(strings.TrimSpace(input.Decision))
	input.Comment = strings.TrimSpace(input.Comment)
	if input.ReviewerName == "" || input.Decision != DecisionApproved && input.Decision != DecisionRejected {
		return Detail{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin requirement review: %w", err)
	}
	defer tx.Rollback(ctx)
	var current Requirement
	const selectForUpdate = `SELECT id,document_set_id,requirement_key,version_number,title,
		statement,requirement_type,flow_type,actor,precondition,postcondition,priority,risk,
		status,confidence,supersedes_requirement_id,assumptions,extraction_key,
		source_fingerprint,created_at,updated_at FROM requirements WHERE id=$1 FOR UPDATE`
	if err := tx.QueryRow(ctx, selectForUpdate, id).Scan(requirementDestinations(&current)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("lock requirement: %w", err)
	}
	edited := hasRequirementEdits(current, input)
	if (current.Status == StatusConflict || current.Status == StatusTBD) &&
		input.Decision == DecisionApproved && !edited {
		return Detail{}, fmt.Errorf("%w: resolve conflict or TBD first", ErrReviewBlocked)
	}
	targetID := current.ID
	if edited {
		title := defaultValue(strings.TrimSpace(input.Title), current.Title)
		statement := defaultValue(strings.TrimSpace(input.Statement), current.Statement)
		if err := tx.QueryRow(ctx, `INSERT INTO requirements
			(document_set_id,requirement_key,version_number,title,statement,requirement_type,
			 flow_type,actor,precondition,postcondition,priority,risk,status,confidence,
			 supersedes_requirement_id,assumptions,source_fingerprint,raw_payload)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'DRAFT',$13,$14,$15,$16,'{}') RETURNING id`,
			current.DocumentSetID, current.RequirementKey, current.VersionNumber+1,
			title, statement, current.RequirementType,
			current.FlowType, strings.TrimSpace(input.Actor), strings.TrimSpace(input.Precondition),
			strings.TrimSpace(input.Postcondition), defaultValue(strings.ToUpper(input.Priority), current.Priority),
			defaultValue(strings.ToUpper(input.Risk), current.Risk), current.Confidence, current.ID,
			current.Assumptions, current.SourceFingerprint).Scan(&targetID); err != nil {
			return Detail{}, fmt.Errorf("version edited requirement: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO requirement_evidence
			(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
			SELECT $1,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash
			FROM requirement_evidence WHERE requirement_id=$2`, targetID, current.ID); err != nil {
			return Detail{}, fmt.Errorf("copy requirement evidence: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO requirement_flow_steps
			(requirement_id,ordinal,action,expected_result)
			SELECT $1,ordinal,action,expected_result FROM requirement_flow_steps WHERE requirement_id=$2`,
			targetID, current.ID); err != nil {
			return Detail{}, fmt.Errorf("copy requirement flow: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE requirements SET status=$2,updated_at=NOW() WHERE id=$1`,
		targetID, input.Decision); err != nil {
		return Detail{}, fmt.Errorf("apply requirement decision: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO requirement_reviews
		(requirement_id,reviewer_name,decision,comment) VALUES($1,$2,$3,$4)`,
		targetID, input.ReviewerName, input.Decision, input.Comment); err != nil {
		return Detail{}, fmt.Errorf("save requirement review: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit requirement review: %w", err)
	}
	return r.Get(ctx, targetID)
}

func (r *Repository) ListConflicts(ctx context.Context, setID int64) ([]Conflict, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,document_set_id,left_requirement_id,
		right_requirement_id,reason,status,resolution,created_at,resolved_at
		FROM requirement_conflicts WHERE document_set_id=$1 ORDER BY status,id`, setID)
	if err != nil {
		return nil, fmt.Errorf("list requirement conflicts: %w", err)
	}
	defer rows.Close()
	results := make([]Conflict, 0)
	for rows.Next() {
		var item Conflict
		if err := rows.Scan(&item.ID, &item.DocumentSetID, &item.LeftRequirementID,
			&item.RightRequirementID, &item.Reason, &item.Status, &item.Resolution,
			&item.CreatedAt, &item.ResolvedAt); err != nil {
			return nil, fmt.Errorf("scan requirement conflict: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *Repository) ListOpenQuestions(ctx context.Context, setID int64) ([]OpenQuestion, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,document_set_id,requirement_id,question,
		owner_role,status,answer,created_at,answered_at FROM open_questions
		WHERE document_set_id=$1 ORDER BY status,id`, setID)
	if err != nil {
		return nil, fmt.Errorf("list requirement open questions: %w", err)
	}
	defer rows.Close()
	results := make([]OpenQuestion, 0)
	for rows.Next() {
		var item OpenQuestion
		if err := rows.Scan(&item.ID, &item.DocumentSetID, &item.RequirementID,
			&item.Question, &item.OwnerRole, &item.Status, &item.Answer,
			&item.CreatedAt, &item.AnsweredAt); err != nil {
			return nil, fmt.Errorf("scan open question: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func requirementDestinations(item *Requirement) []any {
	return []any{&item.ID, &item.DocumentSetID, &item.RequirementKey, &item.VersionNumber,
		&item.Title, &item.Statement, &item.RequirementType, &item.FlowType, &item.Actor,
		&item.Precondition, &item.Postcondition, &item.Priority, &item.Risk, &item.Status,
		&item.Confidence, &item.SupersedesID, &item.Assumptions, &item.ExtractionKey,
		&item.SourceFingerprint, &item.CreatedAt, &item.UpdatedAt}
}

func requirementKey(proposal Proposal, chunk document.SemanticChunk) string {
	base := strings.ToUpper(strings.TrimSpace(proposal.Identifier))
	if base == "" {
		base = "REQ"
	}
	if proposal.FlowType != document.FlowNone {
		base += "-" + proposal.FlowType
	}
	digest := sha256.Sum256([]byte(fmt.Sprint(chunk.DocumentVersionID) + "\x00" + chunk.SourceLocator + "\x00" + proposal.Statement))
	return base + "-" + strings.ToUpper(hex.EncodeToString(digest[:3]))
}

func hashText(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func normalizeForDedupe(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func hasRequirementEdits(current Requirement, input ReviewInput) bool {
	return strings.TrimSpace(input.Title) != "" && strings.TrimSpace(input.Title) != current.Title ||
		strings.TrimSpace(input.Statement) != "" && strings.TrimSpace(input.Statement) != current.Statement ||
		strings.TrimSpace(input.Actor) != "" && strings.TrimSpace(input.Actor) != current.Actor ||
		strings.TrimSpace(input.Precondition) != "" && strings.TrimSpace(input.Precondition) != current.Precondition ||
		strings.TrimSpace(input.Postcondition) != "" && strings.TrimSpace(input.Postcondition) != current.Postcondition ||
		strings.TrimSpace(input.Priority) != "" && strings.ToUpper(strings.TrimSpace(input.Priority)) != current.Priority ||
		strings.TrimSpace(input.Risk) != "" && strings.ToUpper(strings.TrimSpace(input.Risk)) != current.Risk
}

func defaultValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
