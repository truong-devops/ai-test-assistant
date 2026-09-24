package testcase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

const proposalColumns = `id,document_set_id,test_suite_id,workflow_job_id,proposal_key,
	classification,reason,content,content_hash,generation,candidates,source_revision,status,
	result_revision_id,decision,decision_reason,decided_by,decided_at,created_at,workflow_unit_id`

func scanProposal(row interface{ Scan(...any) error }) (GenerationProposal, error) {
	var p GenerationProposal
	err := row.Scan(&p.ID, &p.DocumentSetID, &p.TestSuiteID, &p.WorkflowJobID, &p.ProposalKey,
		&p.Classification, &p.Reason, &p.Content, &p.ContentHash, &p.Generation, &p.Candidates,
		&p.SourceRevision, &p.Status, &p.ResultRevisionID, &p.Decision, &p.DecisionReason,
		&p.DecidedBy, &p.DecidedAt, &p.CreatedAt, &p.WorkflowUnitID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return p, err
}

// Raw response formatting and attempt bookkeeping do not redefine a scenario.
// Unknown legacy provenance is never accepted as proof of UNCHANGED.
func generationContextHash(g GenerationProvenance) string {
	if g.Provider == "" || g.Model == "" || g.PromptVersion == "" || g.PromptHash == "" || g.RequirementReviewHash == "" {
		return ""
	}
	g.WorkflowJobID, g.WorkflowInputHash, g.SourceRevision, g.ResponseHash = 0, "", 0, ""
	data, _ := json.Marshal(g)
	return hash(string(data))
}

func (r *Repository) proposalCount(ctx context.Context, setID, jobID int64) (int, error) {
	if jobID <= 0 {
		return 0, ErrInvalidInput
	}
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM test_case_generation_proposals WHERE document_set_id=$1 AND workflow_job_id=$2`, setID, jobID).Scan(&count)
	return count, err
}

type proposalHead struct {
	Target         GenerationTarget
	Revision       TestCase
	Content        RevisionContent
	GenerationHash string
}

func proposalContentFromOutput(p Proposal) RevisionContent {
	c := RevisionContent{Title: p.Title, TestType: p.TestType, Risk: defaultString(p.Risk, "MEDIUM"), Actor: p.Actor,
		Precondition: p.Precondition, TestData: p.TestData, ExpectedResult: p.ExpectedResult, Postcondition: p.Postcondition,
		Assumptions: append([]string{}, p.Assumptions...), RequirementRevisionIDs: append([]int64{}, p.RequirementIDs...)}
	for _, step := range p.Steps {
		c.Steps = append(c.Steps, StepInput{Action: step.Action, ExpectedResult: step.ExpectedResult})
	}
	return c
}

func classifyProposal(provenanceHash string, exact, related []proposalHead) (string, string, []GenerationTarget) {
	candidates := []GenerationTarget{}
	if len(exact) == 1 {
		head := exact[0]
		candidates = append(candidates, head.Target)
		if provenanceHash != "" && provenanceHash == head.GenerationHash {
			return "UNCHANGED", "Nội dung, citations và generation context trùng exact revision đã pin.", candidates
		}
		return "NEW_REVISION", "Nội dung exact như bản đã pin nhưng generation context khác hoặc chưa được ghi nhận; cần review riêng.", candidates
	}
	if len(exact) > 1 {
		related = exact
	}
	for _, head := range related {
		candidates = append(candidates, head.Target)
	}
	if len(candidates) > 0 {
		return "AMBIGUOUS_MATCH", "Có identity liên kết requirement/chuỗi revision/đối chiếu nguồn; reviewer phải chọn sửa identity hay tạo case độc lập.", candidates
	}
	return "NEW_CASE", "Không có identity đã pin liên kết nguồn này; không suy đoán lineage từ title hoặc test type.", candidates
}

func (r *Repository) saveProposals(ctx context.Context, suite Suite, input GenerationBaseline, outputs []Proposal) (int, error) {
	if input.WorkflowJobID <= 0 || input.WorkflowAttempt <= 0 || (len(outputs) == 0 && !input.IncludeRetire) {
		return 0, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var jobID int64
	err = tx.QueryRow(ctx, `SELECT id FROM document_workflow_jobs WHERE id=$1 AND document_set_id=$2
		AND operation='GENERATE_TESTCASES' AND status='RUNNING' AND attempt_count=$3 AND input_hash=$4
		AND cancel_requested_at IS NULL AND lease_expires_at>NOW() AND ($5=0 OR revision=$5) FOR UPDATE`, input.WorkflowJobID, suite.DocumentSetID, input.WorkflowAttempt, input.InputHash, input.WorkflowClaimRevision).Scan(&jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrGenerationLease
	}
	if err != nil {
		return 0, err
	}
	var count int
	if input.WorkflowUnitID > 0 {
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document_workflow_job_units WHERE id=$1 AND workflow_job_id=$2 AND status='RUNNING')`, input.WorkflowUnitID, jobID).Scan(&valid); err != nil {
			return 0, err
		}
		if !valid {
			return 0, ErrGenerationLease
		}
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM test_case_generation_proposals WHERE workflow_job_id=$1 AND COALESCE(workflow_unit_id,0)=$2`, jobID, input.WorkflowUnitID).Scan(&count); err != nil {
		return 0, err
	}
	if count > 0 {
		return count, nil
	}
	if err := checkProposalSource(ctx, tx, suite.DocumentSetID, input.SourceRevision); err != nil {
		return 0, err
	}
	heads := []proposalHead{}
	for _, target := range input.Targets {
		item, content, err := r.revisionContent(ctx, tx, target.RevisionID)
		if err != nil {
			return 0, err
		}
		if item.DocumentSetID != suite.DocumentSetID || item.FamilyID != target.FamilyID || hash(fmt.Sprintf("%d:%d:%s", item.FamilyID, item.ID, item.ContentHash)) != target.HeadToken {
			return 0, ErrInvalidInput
		}
		// The generated suite must not revise an identity from another suite.
		if item.TestSuiteID != suite.ID {
			continue
		}
		var provenance struct {
			Generation GenerationProvenance `json:"generation"`
		}
		if err := json.Unmarshal(item.Provenance, &provenance); err != nil {
			return 0, err
		}
		heads = append(heads, proposalHead{target, item, content, generationContextHash(provenance.Generation)})
	}
	for _, output := range outputs {
		content := proposalContentFromOutput(output)
		if err := validateRevisionSize(content); err != nil {
			return 0, err
		}
		content, err = r.validateRevisionContent(ctx, tx, suite.DocumentSetID, content, true)
		if err != nil {
			return 0, err
		}
		if err := validateProposalRequirements(ctx, tx, content); err != nil {
			return 0, err
		}
		contentHash, err := revisionContentHash(content)
		if err != nil {
			return 0, err
		}
		generation := GenerationProvenance{}
		if output.Generation != nil {
			generation = *output.Generation
		}
		exact, related := []proposalHead{}, []proposalHead{}
		for _, head := range heads {
			if head.Revision.ContentHash == contentHash {
				exact = append(exact, head)
				continue
			}
			linked, err := proposalSourcesRelated(ctx, tx, suite.DocumentSetID, content.RequirementRevisionIDs, head.Content.RequirementRevisionIDs)
			if err != nil {
				return 0, err
			}
			if linked {
				related = append(related, head)
			}
		}
		kind, reason, candidates := classifyProposal(generationContextHash(generation), exact, related)
		p := GenerationProposal{DocumentSetID: suite.DocumentSetID, TestSuiteID: suite.ID, WorkflowJobID: jobID,
			ProposalKey: hash(contentHash + ":" + generationContextHash(generation)), Classification: kind, Reason: reason,
			Content: content, ContentHash: contentHash, Generation: generation, Candidates: candidates, SourceRevision: input.SourceRevision}
		if input.WorkflowUnitID > 0 {
			p.WorkflowUnitID = &input.WorkflowUnitID
		}
		if err := insertProposal(ctx, tx, p); err != nil {
			return 0, err
		}
	}
	// Only ALL/AFFECTED scopes run retire; selection omissions are not removals.
	if input.IncludeRetire {
		for _, head := range heads {
			removed, err := proposalSourcesRemoved(ctx, tx, head.Content.RequirementRevisionIDs)
			if err != nil {
				return 0, err
			}
			if !removed {
				continue
			}
			p := GenerationProposal{DocumentSetID: suite.DocumentSetID, TestSuiteID: suite.ID, WorkflowJobID: jobID,
				ProposalKey: fmt.Sprintf("retire:%d", head.Target.RevisionID), Classification: "RETIRE_CANDIDATE",
				Reason:  "Tất cả requirement liên kết đều được đánh dấu REMOVED; đề xuất archive, không xóa revision/run/release.",
				Content: head.Content, ContentHash: head.Revision.ContentHash, Candidates: []GenerationTarget{head.Target}, SourceRevision: input.SourceRevision}
			if input.WorkflowUnitID > 0 {
				p.WorkflowUnitID = &input.WorkflowUnitID
			}
			if err := insertProposal(ctx, tx, p); err != nil {
				return 0, err
			}
		}
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM test_case_generation_proposals WHERE workflow_job_id=$1 AND COALESCE(workflow_unit_id,0)=$2`, jobID, input.WorkflowUnitID).Scan(&count); err != nil {
		return 0, err
	}
	if input.WorkflowUnitID > 0 {
		if _, err := tx.Exec(ctx, `UPDATE document_workflow_job_units SET status='SUCCEEDED',output_ref=jsonb_build_object('proposal_count',$2::integer),finished_at=NOW(),updated_at=NOW() WHERE id=$1`, input.WorkflowUnitID, count); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE document_workflow_jobs SET completed_units=(SELECT count(*) FROM document_workflow_job_units WHERE workflow_job_id=$1 AND unit_key<>'operation' AND status='SUCCEEDED') WHERE id=$1`, jobID); err != nil {
			return 0, err
		}
	}
	return count, tx.Commit(ctx)
}

func insertProposal(ctx context.Context, tx pgx.Tx, p GenerationProposal) error {
	_, err := tx.Exec(ctx, `INSERT INTO test_case_generation_proposals(document_set_id,test_suite_id,workflow_job_id,proposal_key,
		classification,reason,content,content_hash,generation,candidates,source_revision,workflow_unit_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(workflow_job_id,proposal_key) DO NOTHING`,
		p.DocumentSetID, p.TestSuiteID, p.WorkflowJobID, p.ProposalKey, p.Classification, p.Reason, p.Content, p.ContentHash, p.Generation, p.Candidates, p.SourceRevision, p.WorkflowUnitID)
	return err
}

func checkProposalSource(ctx context.Context, tx pgx.Tx, setID, revision int64) error {
	var current int64
	var status string
	if err := tx.QueryRow(ctx, `SELECT source_revision,status FROM document_sets WHERE id=$1 FOR SHARE`, setID).Scan(&current, &status); err != nil {
		return err
	}
	if current != revision || status != "ACTIVE" {
		return ErrProposalConflict
	}
	return nil
}

func validateProposalRequirements(ctx context.Context, tx pgx.Tx, content RevisionContent) error {
	var valid int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM requirements WHERE id=ANY($1) AND status='APPROVED' AND cardinality(requirement_review_blockers(id))=0`, content.RequirementRevisionIDs).Scan(&valid); err != nil {
		return err
	}
	if valid != len(content.RequirementRevisionIDs) {
		return ErrNoApprovedSource
	}
	var grounded bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM requirements r WHERE r.id=ANY($1) AND (btrim(r.statement)=$2 OR EXISTS(SELECT 1 FROM requirement_flow_steps s WHERE s.requirement_id=r.id AND btrim(s.expected_result)=$2)))`, content.RequirementRevisionIDs, content.ExpectedResult).Scan(&grounded); err != nil {
		return err
	}
	if !grounded {
		return &ValidationError{Field: "expected_result", Message: "Kết quả mong đợi phải có trong requirement đã duyệt."}
	}
	return nil
}

func proposalSourcesRelated(ctx context.Context, tx pgx.Tx, setID int64, current, previous []int64) (bool, error) {
	var linked bool
	err := tx.QueryRow(ctx, `WITH RECURSIVE lineage AS (
		SELECT id,supersedes_requirement_id FROM requirements WHERE document_set_id=$1 AND id=ANY($2)
		UNION SELECT r.id,r.supersedes_requirement_id FROM requirements r JOIN lineage l ON l.supersedes_requirement_id=r.id WHERE r.document_set_id=$1)
		SELECT EXISTS(SELECT 1 FROM lineage WHERE id=ANY($3)) OR EXISTS(
		SELECT 1 FROM requirement_source_comparisons c CROSS JOIN LATERAL jsonb_array_elements(c.items) item
		WHERE c.document_set_id=$1 AND EXISTS(SELECT 1 FROM jsonb_array_elements_text(item->'after_ids') v WHERE v::bigint=ANY($2))
		AND EXISTS(SELECT 1 FROM jsonb_array_elements_text(item->'before_ids') v WHERE v::bigint=ANY($3)))`, setID, current, previous).Scan(&linked)
	return linked, err
}

func proposalSourcesRemoved(ctx context.Context, tx pgx.Tx, ids []int64) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	var count int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM requirements WHERE id=ANY($1) AND source_state='REMOVED'`, ids).Scan(&count)
	return count == len(ids), err
}

func (s *Service) ListProposals(ctx context.Context, setID, jobID, before int64, limit int) (ProposalPage, error) {
	out := ProposalPage{Proposals: []GenerationProposal{}}
	if setID <= 0 || jobID < 0 || before < 0 || limit < 1 || limit > 50 {
		return out, ErrInvalidInput
	}
	rows, err := s.repository.pool.Query(ctx, `SELECT `+proposalColumns+` FROM test_case_generation_proposals WHERE document_set_id=$1 AND ($2=0 OR workflow_job_id=$2) AND ($3=0 OR id<$3) ORDER BY id DESC LIMIT $4`, setID, jobID, before, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return out, err
		}
		out.Proposals = append(out.Proposals, p)
	}
	if len(out.Proposals) > limit {
		out.Proposals = out.Proposals[:limit]
		out.NextBefore = out.Proposals[limit-1].ID
	}
	return out, rows.Err()
}

func (s *Service) DecideProposal(ctx context.Context, id int64, input ProposalDecision, key, actor string) (GenerationProposal, error) {
	return s.repository.decideProposal(ctx, id, input, key, actor)
}

// Compare the proposal with its immutable pinned revision, never a live head or
// a reconstruction from current requirement evidence.
func (s *Service) CompareProposal(ctx context.Context, id, familyID int64) (ProposalComparison, error) {
	if id <= 0 || familyID <= 0 {
		return ProposalComparison{}, ErrInvalidInput
	}
	tx, err := s.repository.pool.Begin(ctx)
	if err != nil {
		return ProposalComparison{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanProposal(tx.QueryRow(ctx, `SELECT `+proposalColumns+` FROM test_case_generation_proposals WHERE id=$1`, id))
	if err != nil {
		return ProposalComparison{}, err
	}
	for _, target := range p.Candidates {
		if target.FamilyID != familyID {
			continue
		}
		revision, content, err := s.repository.revisionContent(ctx, tx, target.RevisionID)
		if err != nil {
			return ProposalComparison{}, err
		}
		if revision.DocumentSetID != p.DocumentSetID || revision.FamilyID != familyID {
			return ProposalComparison{}, ErrInvalidInput
		}
		return ProposalComparison{Target: target, Before: content, After: p.Content}, nil
	}
	return ProposalComparison{}, ErrInvalidInput
}

func (r *Repository) decideProposal(ctx context.Context, id int64, input ProposalDecision, key, actor string) (GenerationProposal, error) {
	input.Decision = strings.ToUpper(strings.TrimSpace(input.Decision))
	input.Reason = strings.TrimSpace(input.Reason)
	key, actor = strings.TrimSpace(key), strings.TrimSpace(actor)
	if id <= 0 || key == "" || len(key) > 200 || actor == "" || input.Reason == "" || len(input.Reason) > 4000 || !slices.Contains([]string{"CREATE_NEW", "REVISE", "KEEP", "ARCHIVE", "DISMISS"}, input.Decision) {
		return GenerationProposal{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return GenerationProposal{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanProposal(tx.QueryRow(ctx, `SELECT `+proposalColumns+` FROM test_case_generation_proposals WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return p, err
	}
	payload, _ := json.Marshal(struct {
		ID    int64
		Input ProposalDecision
	}{id, input})
	requestHash := hash(string(payload))
	// Serialize the key across proposals as well as retries of the same proposal.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("proposal:%d:%s", p.DocumentSetID, key)); err != nil {
		return p, err
	}
	var storedHash string
	var stored GenerationProposal
	err = tx.QueryRow(ctx, `SELECT request_hash,result FROM test_case_proposal_commands WHERE document_set_id=$1 AND idempotency_key=$2`, p.DocumentSetID, key).Scan(&storedHash, &stored)
	if err == nil {
		if storedHash != requestHash {
			return p, ErrIdempotencyConflict
		}
		return stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	if p.Status != "PENDING" {
		return p, ErrProposalConflict
	}
	if input.Decision == "DISMISS" {
		p.Status = "DISMISSED"
	} else {
		// Successful units within partial jobs are reviewable. Canceled/failed
		// jobs retain evidence but cannot mutate a working baseline.
		var jobStatus string
		if err := tx.QueryRow(ctx, `SELECT status FROM document_workflow_jobs WHERE id=$1 FOR SHARE`, p.WorkflowJobID).Scan(&jobStatus); err != nil {
			return p, err
		}
		if jobStatus != "SUCCEEDED" && !(jobStatus == "PARTIAL_FAILED" && p.WorkflowUnitID != nil) {
			return p, ErrProposalConflict
		}
		if p.WorkflowUnitID != nil {
			var unitStatus string
			if err := tx.QueryRow(ctx, `SELECT status FROM document_workflow_job_units WHERE id=$1 AND workflow_job_id=$2`, *p.WorkflowUnitID, p.WorkflowJobID).Scan(&unitStatus); err != nil {
				return p, err
			}
			if unitStatus != "SUCCEEDED" {
				return p, ErrProposalConflict
			}
		}
		if err := checkProposalSource(ctx, tx, p.DocumentSetID, p.SourceRevision); err != nil {
			return p, err
		}
		var family Family
		var parent *int64
		if input.Decision != "CREATE_NEW" {
			var target *GenerationTarget
			for i := range p.Candidates {
				if p.Candidates[i].FamilyID == input.TargetFamilyID {
					target = &p.Candidates[i]
					break
				}
			}
			if target == nil || input.ExpectedHeadRevisionID != target.RevisionID {
				return p, ErrInvalidInput
			}
			family, err = r.lockFamily(ctx, tx, target.FamilyID)
			if err != nil {
				return p, err
			}
			if family.TestSuiteID != p.TestSuiteID || family.DocumentSetID != p.DocumentSetID {
				return p, ErrInvalidInput
			}
			if family.Archived {
				return p, ErrFamilyArchived
			}
			if family.HeadRevisionID == nil || *family.HeadRevisionID != target.RevisionID || family.HeadToken != target.HeadToken {
				current := int64(0)
				if family.HeadRevisionID != nil {
					current = *family.HeadRevisionID
				}
				return p, &RevisionConflictError{CurrentRevisionID: current, CurrentHeadToken: family.HeadToken}
			}
			parent = &target.RevisionID
		} else if input.TargetFamilyID != 0 || input.ExpectedHeadRevisionID != 0 {
			return p, ErrInvalidInput
		}
		switch input.Decision {
		case "ARCHIVE":
			if p.Classification != "RETIRE_CANDIDATE" {
				return p, ErrInvalidInput
			}
			removed, err := proposalSourcesRemoved(ctx, tx, p.Content.RequirementRevisionIDs)
			if err != nil {
				return p, err
			}
			if !removed {
				return p, ErrProposalConflict
			}
			if _, err := tx.Exec(ctx, `UPDATE test_case_families SET archived=TRUE,updated_at=NOW() WHERE id=$1`, family.ID); err != nil {
				return p, err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO test_case_revision_audit(family_id,test_case_id,event_type,actor,display_name,content_hash,reason,metadata) VALUES($1,$2,'ARCHIVED',$3,$3,$4,$5,$6)`, family.ID, *parent, actor, family.HeadToken, input.Reason, map[string]any{"proposal_id": p.ID}); err != nil {
				return p, err
			}
			p.ResultRevisionID = parent
		case "KEEP":
			if p.Classification != "UNCHANGED" {
				return p, ErrInvalidInput
			}
			if err := validateProposalRequirements(ctx, tx, p.Content); err != nil {
				return p, err
			}
			p.ResultRevisionID = parent
		case "CREATE_NEW", "REVISE":
			if p.Classification == "RETIRE_CANDIDATE" || p.Classification == "UNCHANGED" {
				return p, ErrInvalidInput
			}
			if input.Decision == "REVISE" && p.Classification != "NEW_REVISION" && p.Classification != "AMBIGUOUS_MATCH" {
				return p, ErrInvalidInput
			}
			if input.Decision == "CREATE_NEW" && p.Classification != "NEW_CASE" && p.Classification != "AMBIGUOUS_MATCH" {
				return p, ErrInvalidInput
			}
			content, err := r.validateRevisionContent(ctx, tx, p.DocumentSetID, p.Content, true)
			if err != nil {
				return p, err
			}
			if err := validateProposalRequirements(ctx, tx, content); err != nil {
				return p, err
			}
			checkedHash, err := revisionContentHash(content)
			if err != nil {
				return p, err
			}
			if checkedHash != p.ContentHash {
				return p, ErrProposalConflict
			}
			// Shared fingerprint lock rejects duplicate CREATE_NEW applications of
			// the same output, without modifying any existing revision.
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("testcase-generation:%d:%s", p.TestSuiteID, p.ContentHash)); err != nil {
				return p, err
			}
			if parent == nil {
				var existing int64
				err = tx.QueryRow(ctx, `SELECT t.id FROM test_cases t JOIN test_case_families f ON f.head_revision_id=t.id WHERE t.test_suite_id=$1 AND t.content_hash=$2 AND NOT f.archived ORDER BY t.id LIMIT 1`, p.TestSuiteID, p.ContentHash).Scan(&existing)
				if err == nil {
					return p, ErrProposalConflict
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					return p, err
				}
				family, err = r.createFamily(ctx, tx, Suite{ID: p.TestSuiteID, DocumentSetID: p.DocumentSetID})
				if err != nil {
					return p, err
				}
			}
			template := TestCase{AutomationStatus: "MANUAL", GeneratedBy: "GENERATION_PROPOSAL"}
			item, err := r.insertRevision(ctx, tx, family, content, template, family.RevisionCounter+1, parent, nil, actor, input.Reason, p.ContentHash,
				map[string]any{"operation": "APPLY_PROPOSAL", "proposal_id": p.ID, "generation": p.Generation, "model": p.Generation.Model})
			if err != nil {
				return p, err
			}
			p.ResultRevisionID = &item.ID
		}
		p.Status = "APPLIED"
	}
	p.Decision, p.DecisionReason, p.DecidedBy = input.Decision, input.Reason, actor
	p, err = scanProposal(tx.QueryRow(ctx, `UPDATE test_case_generation_proposals SET status=$2,result_revision_id=$3,decision=$4,decision_reason=$5,decided_by=$6,decided_at=NOW() WHERE id=$1 RETURNING `+proposalColumns, p.ID, p.Status, p.ResultRevisionID, p.Decision, p.DecisionReason, p.DecidedBy))
	if err != nil {
		return p, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO test_case_proposal_commands(document_set_id,idempotency_key,proposal_id,request_hash,result,actor) VALUES($1,$2,$3,$4,$5,$6)`, p.DocumentSetID, key, p.ID, requestHash, p, actor); err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}
