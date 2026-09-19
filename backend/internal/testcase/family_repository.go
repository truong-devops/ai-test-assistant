package testcase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
)

const testCaseColumnList = `id,test_suite_id,document_set_id,test_case_key,version_number,
	title,test_type,risk,actor,precondition,test_data,expected_result,expected_result_hash,
	postcondition,status,automation_status,confidence,generated_by,assumptions,generation_key,
	supersedes_test_case_id,family_id,parent_revision_id,restored_from_revision_id,content_hash,
	created_by,change_reason,source_snapshot_id,provenance,sealed_at,created_at,updated_at`

func (r *Repository) SaveGenerated(ctx context.Context, suite Suite, proposal Proposal) (TestCase, bool, error) {
	if suite.ID <= 0 || suite.DocumentSetID <= 0 || len(proposal.RequirementIDs) == 0 ||
		strings.TrimSpace(proposal.Title) == "" || strings.TrimSpace(proposal.ExpectedResult) == "" ||
		!validTestType(proposal.TestType) {
		return TestCase{}, false, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TestCase{}, false, fmt.Errorf("begin save generated testcase: %w", err)
	}
	defer tx.Rollback(ctx)
	content := RevisionContent{Title: proposal.Title, TestType: proposal.TestType,
		Risk: defaultString(proposal.Risk, "MEDIUM"), Actor: proposal.Actor,
		Precondition: proposal.Precondition, TestData: proposal.TestData,
		ExpectedResult: proposal.ExpectedResult, Postcondition: proposal.Postcondition,
		Assumptions:            append([]string(nil), proposal.Assumptions...),
		RequirementRevisionIDs: append([]int64(nil), proposal.RequirementIDs...)}
	for _, step := range proposal.Steps {
		content.Steps = append(content.Steps, StepInput{Action: step.Action,
			ExpectedResult: step.ExpectedResult})
	}
	content, err = r.validateRevisionContent(ctx, tx, suite.DocumentSetID, content, true)
	if err != nil {
		return TestCase{}, false, err
	}
	contentHash, err := revisionContentHash(content)
	if err != nil {
		return TestCase{}, false, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("testcase-generation:%d:%s", suite.ID, contentHash)); err != nil {
		return TestCase{}, false, fmt.Errorf("lock generated testcase identity: %w", err)
	}
	var existing TestCase
	err = tx.QueryRow(ctx, `SELECT `+testCaseColumnList+` FROM test_cases
		WHERE test_suite_id=$1 AND content_hash=$2 AND sealed_at IS NOT NULL
		ORDER BY id LIMIT 1`, suite.ID, contentHash).Scan(testCaseDestinations(&existing)...)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return TestCase{}, false, fmt.Errorf("commit reused generated testcase: %w", err)
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return TestCase{}, false, fmt.Errorf("find generated testcase replay: %w", err)
	}
	family, err := r.createFamily(ctx, tx, suite)
	if err != nil {
		return TestCase{}, false, err
	}
	template := TestCase{TestSuiteID: suite.ID, DocumentSetID: suite.DocumentSetID,
		TestType: content.TestType, Risk: content.Risk,
		AutomationStatus: defaultString(proposal.AutomationStatus, "MANUAL"),
		Confidence:       proposal.Confidence, GeneratedBy: defaultString(proposal.GeneratedBy, "RULE_ENGINE")}
	result, err := r.insertRevision(ctx, tx, family, content, template, 1, nil, nil,
		template.GeneratedBy, "Generated from approved requirement revisions", contentHash,
		map[string]any{"operation": "GENERATE", "requirement_revision_ids": content.RequirementRevisionIDs})
	if err != nil {
		return TestCase{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TestCase{}, false, fmt.Errorf("commit generated testcase: %w", err)
	}
	return result, true, nil
}

func (r *Repository) createFamily(ctx context.Context, tx pgx.Tx, suite Suite) (Family, error) {
	for attempt := 0; attempt < 20; attempt++ {
		var sequence int64
		if err := tx.QueryRow(ctx, `SELECT nextval('test_case_public_key_seq')`).Scan(&sequence); err != nil {
			return Family{}, fmt.Errorf("allocate testcase public key: %w", err)
		}
		publicKey := fmt.Sprintf("TC-SYS-%06d", sequence)
		var result Family
		err := tx.QueryRow(ctx, `INSERT INTO test_case_families
			(test_suite_id,document_set_id,public_key,head_token)
			VALUES($1,$2,$3,$4) ON CONFLICT(test_suite_id,public_key) DO NOTHING
			RETURNING id,test_suite_id,document_set_id,public_key,legacy_key,archived,
			revision_counter,head_revision_id,head_token,needs_identity_review,created_at,updated_at`,
			suite.ID, suite.DocumentSetID, publicKey, hash("empty-family:"+publicKey)).
			Scan(familyDestinations(&result)...)
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Family{}, fmt.Errorf("create testcase family: %w", err)
		}
	}
	return Family{}, fmt.Errorf("allocate unique testcase public key: %w", ErrRevisionConflict)
}

func (r *Repository) CreateRevision(ctx context.Context, familyID int64, input CreateRevisionInput,
	idempotencyKey, actor string,
) (RevisionResult, error) {
	return r.createRevision(ctx, familyID, input, RestoreInput{}, "CREATE_REVISION",
		idempotencyKey, actor)
}

func (r *Repository) Restore(ctx context.Context, familyID int64, input RestoreInput,
	idempotencyKey, actor string,
) (RevisionResult, error) {
	return r.createRevision(ctx, familyID, CreateRevisionInput{}, input, "RESTORE",
		idempotencyKey, actor)
}

func (r *Repository) GetFamily(ctx context.Context, familyID int64) (Family, error) {
	if familyID <= 0 {
		return Family{}, ErrInvalidInput
	}
	var result Family
	err := r.pool.QueryRow(ctx, `SELECT id,test_suite_id,document_set_id,public_key,legacy_key,
		archived,revision_counter,head_revision_id,head_token,needs_identity_review,
		created_at,updated_at FROM test_case_families WHERE id=$1`, familyID).
		Scan(familyDestinations(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Family{}, ErrNotFound
	}
	if err != nil {
		return Family{}, fmt.Errorf("get testcase family: %w", err)
	}
	if result.HeadRevisionID != nil {
		item, err := r.Get(ctx, *result.HeadRevisionID)
		if err != nil {
			return Family{}, err
		}
		result.LatestRevision = &item.TestCase
	}
	var approved TestCase
	err = r.pool.QueryRow(ctx, `SELECT `+testCaseColumnList+` FROM test_cases
		WHERE family_id=$1 AND status='APPROVED' ORDER BY version_number DESC LIMIT 1`, familyID).
		Scan(testCaseDestinations(&approved)...)
	if err == nil {
		result.LatestApproved = &approved
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Family{}, fmt.Errorf("get latest approved testcase revision: %w", err)
	}
	return result, nil
}

func (r *Repository) ListFamilies(ctx context.Context, setID int64) ([]Family, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := r.pool.Query(ctx, `SELECT id,test_suite_id,document_set_id,public_key,legacy_key,
		archived,revision_counter,head_revision_id,head_token,needs_identity_review,
		created_at,updated_at FROM test_case_families WHERE document_set_id=$1
		ORDER BY public_key,id`, setID)
	if err != nil {
		return nil, fmt.Errorf("list testcase families: %w", err)
	}
	results := make([]Family, 0)
	for rows.Next() {
		var item Family
		if err := rows.Scan(familyDestinations(&item)...); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan testcase family: %w", err)
		}
		results = append(results, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for index := range results {
		loaded, err := r.GetFamily(ctx, results[index].ID)
		if err != nil {
			return nil, err
		}
		results[index] = loaded
	}
	return results, nil
}

func (r *Repository) ListVersions(ctx context.Context, familyID int64) ([]TestCase, error) {
	if familyID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := r.pool.Query(ctx, `SELECT `+testCaseColumnList+` FROM test_cases
		WHERE family_id=$1 ORDER BY version_number DESC,id DESC`, familyID)
	if err != nil {
		return nil, fmt.Errorf("list testcase revisions: %w", err)
	}
	defer rows.Close()
	results := make([]TestCase, 0)
	for rows.Next() {
		var item TestCase
		if err := rows.Scan(testCaseDestinations(&item)...); err != nil {
			return nil, fmt.Errorf("scan testcase revision: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		if _, err := r.GetFamily(ctx, familyID); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (r *Repository) Diff(ctx context.Context, familyID, fromID, toID int64) (RevisionDiff, error) {
	if familyID <= 0 || fromID <= 0 || toID <= 0 {
		return RevisionDiff{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RevisionDiff{}, err
	}
	defer tx.Rollback(ctx)
	from, before, err := r.revisionContent(ctx, tx, fromID)
	if err != nil {
		return RevisionDiff{}, err
	}
	to, after, err := r.revisionContent(ctx, tx, toID)
	if err != nil {
		return RevisionDiff{}, err
	}
	if from.FamilyID != familyID || to.FamilyID != familyID {
		return RevisionDiff{}, ErrInvalidInput
	}
	changes := make([]FieldDiff, 0)
	appendChange := func(field string, left, right any) {
		if !reflect.DeepEqual(left, right) {
			changes = append(changes, FieldDiff{Field: field, Before: left, After: right})
		}
	}
	appendChange("title", before.Title, after.Title)
	appendChange("test_type", before.TestType, after.TestType)
	appendChange("risk", before.Risk, after.Risk)
	appendChange("actor", before.Actor, after.Actor)
	appendChange("precondition", before.Precondition, after.Precondition)
	appendChange("test_data", before.TestData, after.TestData)
	appendChange("steps", before.Steps, after.Steps)
	appendChange("expected_result", before.ExpectedResult, after.ExpectedResult)
	appendChange("postcondition", before.Postcondition, after.Postcondition)
	appendChange("assumptions", before.Assumptions, after.Assumptions)
	appendChange("requirement_revision_ids", before.RequirementRevisionIDs, after.RequirementRevisionIDs)
	appendChange("evidence_refs", before.EvidenceRefs, after.EvidenceRefs)
	appendChange("source_snapshot_id", before.SourceSnapshotID, after.SourceSnapshotID)
	if err := tx.Commit(ctx); err != nil {
		return RevisionDiff{}, err
	}
	return RevisionDiff{FamilyID: familyID, From: from, To: to, Changes: changes}, nil
}

func (r *Repository) Archive(ctx context.Context, familyID int64, input ArchiveInput,
	actor string,
) (Family, error) {
	if familyID <= 0 || strings.TrimSpace(input.Reason) == "" {
		return Family{}, ErrInvalidInput
	}
	actor = defaultString(strings.TrimSpace(actor), "USER")
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Family{}, err
	}
	defer tx.Rollback(ctx)
	current, err := r.lockFamily(ctx, tx, familyID)
	if err != nil {
		return Family{}, err
	}
	if current.Archived != input.Archived {
		if _, err := tx.Exec(ctx, `UPDATE test_case_families SET archived=$2,updated_at=NOW()
			WHERE id=$1`, familyID, input.Archived); err != nil {
			return Family{}, fmt.Errorf("archive testcase family: %w", err)
		}
		event := "ARCHIVED"
		if !input.Archived {
			event = "UNARCHIVED"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO test_case_revision_audit
			(family_id,test_case_id,event_type,actor,display_name,content_hash,reason)
			VALUES($1,$2,$3,$4,$4,$5,$6)`, familyID, current.HeadRevisionID,
			event, actor, current.HeadToken, strings.TrimSpace(input.Reason)); err != nil {
			return Family{}, fmt.Errorf("audit testcase archive: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Family{}, err
	}
	return r.GetFamily(ctx, familyID)
}

func (r *Repository) ReviewExact(ctx context.Context, id int64, input ReviewInput,
	actor string,
) (Detail, error) {
	input.ReviewerName = strings.TrimSpace(input.ReviewerName)
	input.Decision = strings.ToUpper(strings.TrimSpace(input.Decision))
	actor = defaultString(strings.TrimSpace(actor), input.ReviewerName)
	if id <= 0 || input.ReviewerName == "" || input.ExpectedContentHash == "" ||
		input.Decision != StatusApproved && input.Decision != StatusRejected {
		return Detail{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	current, err := loadTestCaseTx(ctx, tx, id)
	if err != nil {
		return Detail{}, err
	}
	if current.ContentHash != input.ExpectedContentHash {
		return Detail{}, &RevisionConflictError{CurrentRevisionID: current.ID,
			CurrentHeadToken: current.ContentHash}
	}
	if _, err := tx.Exec(ctx, `UPDATE test_cases SET status=$2,updated_at=NOW() WHERE id=$1`,
		id, input.Decision); err != nil {
		if input.Decision == StatusApproved {
			return Detail{}, fmt.Errorf("%w: %v", ErrReviewBlocked, err)
		}
		return Detail{}, fmt.Errorf("apply testcase review: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO test_case_reviews
		(test_case_id,reviewer_name,decision,comment,content_hash,actor)
		VALUES($1,$2,$3,$4,$5,$6)`,
		id, input.ReviewerName, input.Decision,
		strings.TrimSpace(input.Comment), current.ContentHash, actor); err != nil {
		return Detail{}, fmt.Errorf("save testcase review: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO test_case_revision_audit
		(family_id,test_case_id,event_type,actor,display_name,content_hash,reason,metadata)
		VALUES($1,$2,'REVIEWED',$3,$4,$5,$6,jsonb_build_object('decision',$7::text))`,
		current.FamilyID, id, actor, input.ReviewerName, current.ContentHash,
		strings.TrimSpace(input.Comment), input.Decision); err != nil {
		return Detail{}, fmt.Errorf("audit testcase review: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repository) createRevision(ctx context.Context, familyID int64, input CreateRevisionInput,
	restore RestoreInput, operation, idempotencyKey, actor string,
) (RevisionResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	actor = defaultString(strings.TrimSpace(actor), "USER")
	if familyID <= 0 || idempotencyKey == "" || len(idempotencyKey) > 200 {
		return RevisionResult{}, ErrInvalidInput
	}
	requestPayload, _ := json.Marshal(struct {
		Operation string              `json:"operation"`
		Input     CreateRevisionInput `json:"input"`
		Restore   RestoreInput        `json:"restore"`
	}{operation, input, restore})
	requestHash := hash(string(requestPayload))
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RevisionResult{}, err
	}
	defer tx.Rollback(ctx)
	family, err := r.lockFamily(ctx, tx, familyID)
	if err != nil {
		return RevisionResult{}, err
	}
	var commandHash string
	var replayID int64
	err = tx.QueryRow(ctx, `SELECT request_hash,result_revision_id
		FROM test_case_revision_commands WHERE family_id=$1 AND idempotency_key=$2`,
		familyID, idempotencyKey).Scan(&commandHash, &replayID)
	if err == nil {
		if commandHash != requestHash {
			return RevisionResult{}, ErrIdempotencyConflict
		}
		replay, loadErr := loadTestCaseTx(ctx, tx, replayID)
		if loadErr != nil {
			return RevisionResult{}, loadErr
		}
		if err := tx.Commit(ctx); err != nil {
			return RevisionResult{}, err
		}
		return revisionResult(replay, false), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RevisionResult{}, fmt.Errorf("read testcase idempotency command: %w", err)
	}
	if family.Archived {
		return RevisionResult{}, ErrFamilyArchived
	}
	if family.HeadRevisionID == nil {
		return RevisionResult{}, ErrNotFound
	}
	expectedHead := input.ExpectedHeadRevisionID
	expectedToken := input.ExpectedHeadToken
	if operation == "RESTORE" {
		expectedHead, expectedToken = restore.ExpectedHeadRevisionID, restore.ExpectedHeadToken
	}
	if expectedHead <= 0 || expectedHead != *family.HeadRevisionID ||
		expectedToken != "" && expectedToken != family.HeadToken {
		return RevisionResult{}, &RevisionConflictError{CurrentRevisionID: *family.HeadRevisionID,
			CurrentHeadToken: family.HeadToken}
	}
	if operation == "CREATE_REVISION" && input.BaseRevisionID != *family.HeadRevisionID {
		return RevisionResult{}, &RevisionConflictError{CurrentRevisionID: *family.HeadRevisionID,
			CurrentHeadToken: family.HeadToken}
	}
	head, err := loadTestCaseTx(ctx, tx, *family.HeadRevisionID)
	if err != nil {
		return RevisionResult{}, err
	}
	var source TestCase
	var content RevisionContent
	var restoredFrom *int64
	reason := strings.TrimSpace(input.Reason)
	if operation == "RESTORE" {
		if restore.FromRevisionID <= 0 || strings.TrimSpace(restore.Reason) == "" {
			return RevisionResult{}, ErrInvalidInput
		}
		source, content, err = r.revisionContent(ctx, tx, restore.FromRevisionID)
		reason = strings.TrimSpace(restore.Reason)
		if err != nil || source.FamilyID != familyID {
			if err == nil {
				err = ErrInvalidInput
			}
			return RevisionResult{}, err
		}
		content, err = r.validateRevisionContent(ctx, tx, family.DocumentSetID, content, false)
		restoredFrom = &source.ID
	} else {
		if input.BaseRevisionID <= 0 || reason == "" || (input.Content == nil) == (input.Patch == nil) {
			return RevisionResult{}, ErrInvalidInput
		}
		var baseContent RevisionContent
		source, baseContent, err = r.revisionContent(ctx, tx, input.BaseRevisionID)
		if err != nil || source.FamilyID != familyID {
			if err == nil {
				err = ErrInvalidInput
			}
			return RevisionResult{}, err
		}
		if input.Content != nil {
			content = *input.Content
		} else {
			content = applyRevisionPatch(baseContent, *input.Patch)
		}
		content, err = r.validateRevisionContent(ctx, tx, family.DocumentSetID, content, true)
		if err == nil && source.ID == head.ID && revisionContentsEqual(baseContent, content) {
			if _, err = tx.Exec(ctx, `INSERT INTO test_case_revision_commands
				(family_id,operation,idempotency_key,request_hash,result_revision_id)
				VALUES($1,$2,$3,$4,$5)`, familyID, operation, idempotencyKey,
				requestHash, source.ID); err != nil {
				return RevisionResult{}, fmt.Errorf("save no-op testcase command: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return RevisionResult{}, err
			}
			return revisionResult(source, false), nil
		}
	}
	if err != nil {
		return RevisionResult{}, err
	}
	contentHash, err := revisionContentHash(content)
	if err != nil {
		return RevisionResult{}, err
	}
	parentID := head.ID
	provenance := map[string]any{"operation": operation, "base_revision_id": source.ID,
		"parent_revision_id": parentID}
	result, err := r.insertRevision(ctx, tx, family, content, source,
		family.RevisionCounter+1, &parentID, restoredFrom, actor, reason, contentHash, provenance)
	if err != nil {
		return RevisionResult{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO test_case_revision_commands
		(family_id,operation,idempotency_key,request_hash,result_revision_id)
		VALUES($1,$2,$3,$4,$5)`, familyID, operation, idempotencyKey, requestHash,
		result.ID); err != nil {
		return RevisionResult{}, fmt.Errorf("save testcase idempotency command: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RevisionResult{}, fmt.Errorf("commit testcase revision: %w", err)
	}
	return revisionResult(result, true), nil
}

func (r *Repository) insertRevision(ctx context.Context, tx pgx.Tx, family Family,
	content RevisionContent, template TestCase, version int, parentID, restoredFromID *int64,
	actor, reason, contentHash string, provenance map[string]any,
) (TestCase, error) {
	assumptions, _ := json.Marshal(content.Assumptions)
	provenanceJSON, _ := json.Marshal(provenance)
	expectedHash := hash(content.ExpectedResult)
	var result TestCase
	err := tx.QueryRow(ctx, `INSERT INTO test_cases
		(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,risk,
		 actor,precondition,test_data,expected_result,expected_result_hash,postcondition,status,
		 automation_status,confidence,generated_by,assumptions,generation_key,
		 supersedes_test_case_id,family_id,parent_revision_id,restored_from_revision_id,
		 content_hash,created_by,change_reason,source_snapshot_id,provenance,sealed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'DRAFT',$14,$15,$16,
			 $17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,NULL)
		RETURNING `+testCaseColumnList, family.TestSuiteID, family.DocumentSetID,
		family.PublicKey, version, content.Title, content.TestType, content.Risk, content.Actor,
		content.Precondition, content.TestData, content.ExpectedResult, expectedHash,
		content.Postcondition, defaultString(template.AutomationStatus, "MANUAL"),
		template.Confidence, defaultString(template.GeneratedBy, "USER"),
		assumptions, contentHash, parentID, family.ID, parentID, restoredFromID, contentHash,
		actor, reason, content.SourceSnapshotID, provenanceJSON).
		Scan(testCaseDestinations(&result)...)
	if err != nil {
		return TestCase{}, fmt.Errorf("insert testcase revision: %w", err)
	}
	for index, step := range content.Steps {
		if _, err := tx.Exec(ctx, `INSERT INTO test_case_steps
			(test_case_id,ordinal,action,expected_result) VALUES($1,$2,$3,$4)`,
			result.ID, index+1, step.Action, step.ExpectedResult); err != nil {
			return TestCase{}, fmt.Errorf("insert testcase revision step: %w", err)
		}
	}
	coverageType := coverageTypeFor(content.TestType)
	for _, requirementID := range content.RequirementRevisionIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO test_case_requirement_links
			(test_case_id,requirement_id,document_set_id,coverage_type)
			VALUES($1,$2,$3,$4)`, result.ID, requirementID, family.DocumentSetID,
			coverageType); err != nil {
			return TestCase{}, fmt.Errorf("insert testcase revision requirement: %w", err)
		}
	}
	for _, evidence := range content.EvidenceRefs {
		if _, err := tx.Exec(ctx, `INSERT INTO test_case_evidence_links
			(test_case_id,family_id,document_set_id,requirement_id,requirement_evidence_id,
			 document_version_id,document_block_id,source_locator,excerpt_hash)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, result.ID, family.ID,
			family.DocumentSetID, evidence.RequirementRevisionID,
			evidence.RequirementEvidenceID, evidence.DocumentVersionID,
			evidence.DocumentBlockID, evidence.SourceLocator, evidence.ExcerptHash); err != nil {
			return TestCase{}, fmt.Errorf("insert testcase revision evidence: %w", err)
		}
	}
	if err := tx.QueryRow(ctx, `UPDATE test_cases SET sealed_at=NOW(),updated_at=NOW()
		WHERE id=$1 RETURNING sealed_at,updated_at`, result.ID).
		Scan(&result.SealedAt, &result.UpdatedAt); err != nil {
		return TestCase{}, fmt.Errorf("seal testcase revision: %w", err)
	}
	event := "CREATED"
	if restoredFromID != nil {
		event = "RESTORED"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO test_case_revision_audit
		(family_id,test_case_id,event_type,actor,display_name,content_hash,reason,metadata)
		VALUES($1,$2,$3,$4,$4,$5,$6,$7)`, family.ID, result.ID, event, actor,
		contentHash, reason, provenanceJSON); err != nil {
		return TestCase{}, fmt.Errorf("audit testcase revision: %w", err)
	}
	return result, nil
}

func (r *Repository) validateRevisionContent(ctx context.Context, tx pgx.Tx, setID int64,
	content RevisionContent, requireApproved bool,
) (RevisionContent, error) {
	content, err := normalizeRevisionContent(content)
	if err != nil {
		return RevisionContent{}, err
	}
	requirements := make(map[int64]struct{}, len(content.RequirementRevisionIDs))
	var commonSnapshot *int64
	mixedSnapshots := false
	for _, id := range content.RequirementRevisionIDs {
		var owner int64
		var status string
		var snapshot *int64
		if err := tx.QueryRow(ctx, `SELECT document_set_id,status,source_snapshot_id
			FROM requirements WHERE id=$1`, id).Scan(&owner, &status, &snapshot); err != nil {
			return RevisionContent{}, ErrEvidenceInvalid
		}
		if owner != setID || requireApproved && status != "APPROVED" {
			return RevisionContent{}, ErrNoApprovedSource
		}
		requirements[id] = struct{}{}
		if snapshot != nil {
			if commonSnapshot == nil && !mixedSnapshots {
				value := *snapshot
				commonSnapshot = &value
			} else if commonSnapshot != nil && *commonSnapshot != *snapshot {
				commonSnapshot = nil
				mixedSnapshots = true
			}
		} else {
			commonSnapshot = nil
			mixedSnapshots = true
		}
	}
	if content.SourceSnapshotID == nil && commonSnapshot != nil {
		content.SourceSnapshotID = commonSnapshot
	} else if content.SourceSnapshotID != nil {
		var owner int64
		if err := tx.QueryRow(ctx, `SELECT document_set_id FROM document_source_snapshots
			WHERE id=$1`, *content.SourceSnapshotID).Scan(&owner); err != nil || owner != setID ||
			commonSnapshot == nil || *commonSnapshot != *content.SourceSnapshotID {
			return RevisionContent{}, ErrEvidenceInvalid
		}
	}
	type evidenceRow struct {
		ref      EvidenceRef
		approved bool
	}
	rows, err := tx.Query(ctx, `SELECT e.id,e.requirement_id,e.document_version_id,
		e.document_block_id,e.source_locator,e.excerpt_hash,v.approval_status
		FROM requirement_evidence e JOIN document_versions v ON v.id=e.document_version_id
		WHERE e.document_set_id=$1 AND e.requirement_id=ANY($2)
		ORDER BY e.requirement_id,e.document_version_id,e.document_block_id,e.id`,
		setID, content.RequirementRevisionIDs)
	if err != nil {
		return RevisionContent{}, fmt.Errorf("load testcase evidence candidates: %w", err)
	}
	available := make([]evidenceRow, 0)
	for rows.Next() {
		var item evidenceRow
		var approval string
		if err := rows.Scan(&item.ref.RequirementEvidenceID, &item.ref.RequirementRevisionID,
			&item.ref.DocumentVersionID, &item.ref.DocumentBlockID, &item.ref.SourceLocator,
			&item.ref.ExcerptHash, &approval); err != nil {
			rows.Close()
			return RevisionContent{}, err
		}
		item.approved = approval == "APPROVED"
		available = append(available, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return RevisionContent{}, err
	}
	rows.Close()
	resolved := make([]EvidenceRef, 0)
	if len(content.EvidenceRefs) == 0 {
		for _, item := range available {
			if !requireApproved || item.approved {
				resolved = append(resolved, item.ref)
			}
		}
	} else {
		for _, requested := range content.EvidenceRefs {
			matched := false
			for _, item := range available {
				idMatch := requested.RequirementEvidenceID > 0 &&
					requested.RequirementEvidenceID == item.ref.RequirementEvidenceID
				sourceMatch := requested.RequirementEvidenceID == 0 &&
					requested.RequirementRevisionID == item.ref.RequirementRevisionID &&
					requested.DocumentVersionID == item.ref.DocumentVersionID &&
					requested.DocumentBlockID == item.ref.DocumentBlockID
				if !idMatch && !sourceMatch {
					continue
				}
				if requireApproved && !item.approved ||
					requested.RequirementRevisionID != 0 && requested.RequirementRevisionID != item.ref.RequirementRevisionID ||
					requested.SourceLocator != "" && canonicalText(requested.SourceLocator) != item.ref.SourceLocator ||
					requested.ExcerptHash != "" && !strings.EqualFold(requested.ExcerptHash, item.ref.ExcerptHash) {
					return RevisionContent{}, ErrEvidenceInvalid
				}
				resolved = append(resolved, item.ref)
				matched = true
				break
			}
			if !matched {
				return RevisionContent{}, ErrEvidenceInvalid
			}
		}
	}
	for requirementID := range requirements {
		found := false
		for _, evidence := range resolved {
			if evidence.RequirementRevisionID == requirementID {
				found = true
				break
			}
		}
		if !found {
			return RevisionContent{}, ErrEvidenceInvalid
		}
	}
	content.EvidenceRefs = resolved
	return normalizeRevisionContent(content)
}

func (r *Repository) revisionContent(ctx context.Context, tx pgx.Tx,
	revisionID int64,
) (TestCase, RevisionContent, error) {
	item, err := loadTestCaseTx(ctx, tx, revisionID)
	if err != nil {
		return TestCase{}, RevisionContent{}, err
	}
	var assumptions []string
	if err := json.Unmarshal(item.Assumptions, &assumptions); err != nil {
		return TestCase{}, RevisionContent{}, fmt.Errorf("decode testcase assumptions: %w", err)
	}
	content := RevisionContent{Title: item.Title, TestType: item.TestType, Risk: item.Risk,
		Actor: item.Actor, Precondition: item.Precondition, TestData: item.TestData,
		ExpectedResult: item.ExpectedResult, Postcondition: item.Postcondition,
		Assumptions: assumptions, SourceSnapshotID: item.SourceSnapshotID}
	stepRows, err := tx.Query(ctx, `SELECT action,expected_result FROM test_case_steps
		WHERE test_case_id=$1 ORDER BY ordinal`, revisionID)
	if err != nil {
		return TestCase{}, RevisionContent{}, err
	}
	for stepRows.Next() {
		var step StepInput
		if err := stepRows.Scan(&step.Action, &step.ExpectedResult); err != nil {
			stepRows.Close()
			return TestCase{}, RevisionContent{}, err
		}
		content.Steps = append(content.Steps, step)
	}
	stepRows.Close()
	linkRows, err := tx.Query(ctx, `SELECT requirement_id FROM test_case_requirement_links
		WHERE test_case_id=$1 ORDER BY requirement_id`, revisionID)
	if err != nil {
		return TestCase{}, RevisionContent{}, err
	}
	for linkRows.Next() {
		var id int64
		if err := linkRows.Scan(&id); err != nil {
			linkRows.Close()
			return TestCase{}, RevisionContent{}, err
		}
		content.RequirementRevisionIDs = append(content.RequirementRevisionIDs, id)
	}
	linkRows.Close()
	evidenceRows, err := tx.Query(ctx, `SELECT requirement_id,requirement_evidence_id,
		document_version_id,document_block_id,source_locator,excerpt_hash
		FROM test_case_evidence_links WHERE test_case_id=$1
		ORDER BY requirement_id,document_version_id,document_block_id,requirement_evidence_id`, revisionID)
	if err != nil {
		return TestCase{}, RevisionContent{}, err
	}
	for evidenceRows.Next() {
		var evidence EvidenceRef
		if err := evidenceRows.Scan(&evidence.RequirementRevisionID,
			&evidence.RequirementEvidenceID, &evidence.DocumentVersionID,
			&evidence.DocumentBlockID, &evidence.SourceLocator, &evidence.ExcerptHash); err != nil {
			evidenceRows.Close()
			return TestCase{}, RevisionContent{}, err
		}
		content.EvidenceRefs = append(content.EvidenceRefs, evidence)
	}
	evidenceRows.Close()
	content, err = normalizeRevisionContent(content)
	return item, content, err
}

func loadTestCaseTx(ctx context.Context, tx pgx.Tx, id int64) (TestCase, error) {
	var result TestCase
	err := tx.QueryRow(ctx, `SELECT `+testCaseColumnList+` FROM test_cases WHERE id=$1`, id).
		Scan(testCaseDestinations(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return TestCase{}, ErrNotFound
	}
	if err != nil {
		return TestCase{}, fmt.Errorf("load testcase revision: %w", err)
	}
	return result, nil
}

func (r *Repository) lockFamily(ctx context.Context, tx pgx.Tx, familyID int64) (Family, error) {
	var result Family
	err := tx.QueryRow(ctx, `SELECT id,test_suite_id,document_set_id,public_key,legacy_key,
		archived,revision_counter,head_revision_id,head_token,needs_identity_review,
		created_at,updated_at FROM test_case_families WHERE id=$1 FOR UPDATE`, familyID).
		Scan(familyDestinations(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Family{}, ErrNotFound
	}
	if err != nil {
		return Family{}, fmt.Errorf("lock testcase family: %w", err)
	}
	return result, nil
}

func familyDestinations(item *Family) []any {
	return []any{&item.ID, &item.TestSuiteID, &item.DocumentSetID, &item.PublicKey,
		&item.LegacyKey, &item.Archived, &item.RevisionCounter, &item.HeadRevisionID,
		&item.HeadToken, &item.NeedsIdentityReview, &item.CreatedAt, &item.UpdatedAt}
}

func revisionResult(item TestCase, created bool) RevisionResult {
	return RevisionResult{Revision: item, Created: created,
		Location: fmt.Sprintf("/api/test-cases/%d", item.ID)}
}
