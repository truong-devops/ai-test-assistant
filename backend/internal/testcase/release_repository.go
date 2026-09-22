package testcase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type releaseManifestItem struct {
	FamilyID           int64  `json:"family_id"`
	TestCaseID         int64  `json:"test_case_id"`
	ContentHash        string `json:"content_hash"`
	ExpectedResultHash string `json:"expected_result_hash"`
}

type releaseRequestPayload struct {
	TestSuiteID      int64                 `json:"test_suite_id"`
	DocumentSetID    int64                 `json:"document_set_id"`
	SourceSnapshotID int64                 `json:"source_snapshot_id"`
	Items            []releaseManifestItem `json:"items"`
	ScopeDecision    string                `json:"scope_decision"`
	PublishedBy      string                `json:"published_by"`
}

const releaseTestCaseColumnList = `t.id,t.test_suite_id,t.document_set_id,t.test_case_key,
	t.version_number,t.title,t.test_type,t.risk,t.actor,t.precondition,t.test_data,
	t.expected_result,t.expected_result_hash,t.postcondition,t.status,t.automation_status,
	t.confidence,t.generated_by,t.assumptions,t.generation_key,t.supersedes_test_case_id,
	t.family_id,t.parent_revision_id,t.restored_from_revision_id,t.content_hash,t.created_by,
	t.change_reason,t.source_snapshot_id,t.provenance,t.sealed_at,t.created_at,t.updated_at`

func (r *Repository) PublishRelease(ctx context.Context, setID int64, input PublishReleaseInput,
	idempotencyKey, actor string,
) (SuiteRelease, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	input.PublishedBy = strings.TrimSpace(input.PublishedBy)
	input.ScopeDecision = strings.TrimSpace(input.ScopeDecision)
	actor = strings.TrimSpace(actor)
	if input.PublishedBy == "" {
		input.PublishedBy = actor
	}
	if setID <= 0 || input.TestSuiteID <= 0 || len(input.RevisionIDs) == 0 ||
		len(input.RevisionIDs) > 1000 || input.PublishedBy == "" || idempotencyKey == "" {
		return SuiteRelease{}, false, ErrInvalidInput
	}
	revisionIDs := uniqueSortedIDs(input.RevisionIDs)
	if len(revisionIDs) != len(input.RevisionIDs) {
		return SuiteRelease{}, false, ErrInvalidInput
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return SuiteRelease{}, false, err
	}
	defer tx.Rollback(ctx)
	var suiteSetID int64
	if err := tx.QueryRow(ctx, `SELECT document_set_id FROM test_suites
		WHERE id=$1 FOR UPDATE`, input.TestSuiteID).Scan(&suiteSetID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SuiteRelease{}, false, ErrNotFound
		}
		return SuiteRelease{}, false, err
	}
	if suiteSetID != setID {
		return SuiteRelease{}, false, ErrNotFound
	}

	rows, err := tx.Query(ctx, `SELECT `+releaseTestCaseColumnList+`,f.archived
		FROM test_cases t JOIN test_case_families f ON f.id=t.family_id
		WHERE t.id=ANY($1) ORDER BY f.public_key,t.id`, revisionIDs)
	if err != nil {
		return SuiteRelease{}, false, fmt.Errorf("load suite release revisions: %w", err)
	}
	revisions := make([]TestCase, 0, len(revisionIDs))
	seenFamilies := make(map[int64]struct{}, len(revisionIDs))
	var sourceSnapshotID int64
	for rows.Next() {
		var item TestCase
		var archived bool
		if err := rows.Scan(append(testCaseDestinations(&item), &archived)...); err != nil {
			rows.Close()
			return SuiteRelease{}, false, err
		}
		if item.TestSuiteID != input.TestSuiteID || item.DocumentSetID != setID ||
			item.Status != StatusApproved || item.SealedAt == nil || archived {
			rows.Close()
			return SuiteRelease{}, false, ErrReviewBlocked
		}
		if _, duplicate := seenFamilies[item.FamilyID]; duplicate {
			rows.Close()
			return SuiteRelease{}, false, ErrReleaseScope
		}
		seenFamilies[item.FamilyID] = struct{}{}
		if item.SourceSnapshotID == nil {
			rows.Close()
			return SuiteRelease{}, false, ErrEvidenceInvalid
		}
		if sourceSnapshotID == 0 {
			sourceSnapshotID = *item.SourceSnapshotID
		} else if sourceSnapshotID != *item.SourceSnapshotID {
			rows.Close()
			return SuiteRelease{}, false, ErrReleaseScope
		}
		revisions = append(revisions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return SuiteRelease{}, false, err
	}
	rows.Close()
	if len(revisions) != len(revisionIDs) || sourceSnapshotID == 0 {
		return SuiteRelease{}, false, ErrNotFound
	}
	if input.SourceSnapshotID != nil && *input.SourceSnapshotID != sourceSnapshotID {
		return SuiteRelease{}, false, ErrReleaseScope
	}
	var snapshotSetID int64
	if err := tx.QueryRow(ctx, `SELECT document_set_id FROM document_source_snapshots
		WHERE id=$1`, sourceSnapshotID).Scan(&snapshotSetID); err != nil || snapshotSetID != setID {
		return SuiteRelease{}, false, ErrEvidenceInvalid
	}

	manifestItems := make([]releaseManifestItem, 0, len(revisions))
	for _, item := range revisions {
		manifestItems = append(manifestItems, releaseManifestItem{FamilyID: item.FamilyID,
			TestCaseID: item.ID, ContentHash: item.ContentHash,
			ExpectedResultHash: item.ExpectedResultHash})
	}
	manifestBytes, _ := json.Marshal(struct {
		SourceSnapshotID int64                 `json:"source_snapshot_id"`
		Items            []releaseManifestItem `json:"items"`
	}{sourceSnapshotID, manifestItems})
	manifestHash := hash(string(manifestBytes))
	requestBytes, _ := json.Marshal(releaseRequestPayload{TestSuiteID: input.TestSuiteID,
		DocumentSetID: setID, SourceSnapshotID: sourceSnapshotID, Items: manifestItems,
		ScopeDecision: input.ScopeDecision, PublishedBy: input.PublishedBy})
	requestHash := hash(string(requestBytes))

	var existingID int64
	var existingRequestHash string
	err = tx.QueryRow(ctx, `SELECT id,request_hash FROM test_suite_releases
		WHERE test_suite_id=$1 AND idempotency_key=$2`, input.TestSuiteID, idempotencyKey).
		Scan(&existingID, &existingRequestHash)
	if err == nil {
		if existingRequestHash != requestHash {
			return SuiteRelease{}, false, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return SuiteRelease{}, false, err
		}
		release, err := r.GetRelease(ctx, existingID)
		return release, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SuiteRelease{}, false, err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM test_suite_releases
		WHERE test_suite_id=$1 AND manifest_hash=$2`, input.TestSuiteID, manifestHash).
		Scan(&existingID); err == nil {
		if err := tx.Commit(ctx); err != nil {
			return SuiteRelease{}, false, err
		}
		release, loadErr := r.GetRelease(ctx, existingID)
		return release, false, loadErr
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return SuiteRelease{}, false, err
	}

	var approvedCount int
	var stale bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM test_case_requirement_links WHERE test_case_id=ANY($1) AND cardinality(requirement_review_blockers(requirement_id))>0)`, revisionIDs).Scan(&stale); err != nil {
		return SuiteRelease{}, false, err
	}
	if stale {
		return SuiteRelease{}, false, fmt.Errorf("%w: requirement source changed; review dependent testcase revisions", ErrReviewBlocked)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM requirements requirement
		WHERE requirement.document_set_id=$1 AND requirement.source_snapshot_id=$2
		AND requirement.status='APPROVED' AND NOT EXISTS(
			SELECT 1 FROM requirements newer WHERE newer.supersedes_requirement_id=requirement.id)`,
		setID, sourceSnapshotID).Scan(&approvedCount); err != nil {
		return SuiteRelease{}, false, err
	}
	var coveredIDs, uncoveredIDs []int64
	if err := tx.QueryRow(ctx, `SELECT
		COALESCE(array_agg(requirement.id ORDER BY requirement.id) FILTER (WHERE EXISTS(
			SELECT 1 FROM test_case_requirement_links link
			WHERE link.requirement_id=requirement.id AND link.test_case_id=ANY($3))),'{}'),
		COALESCE(array_agg(requirement.id ORDER BY requirement.id) FILTER (WHERE NOT EXISTS(
			SELECT 1 FROM test_case_requirement_links link
			WHERE link.requirement_id=requirement.id AND link.test_case_id=ANY($3))),'{}')
		FROM requirements requirement
		WHERE requirement.document_set_id=$1 AND requirement.source_snapshot_id=$2
		AND requirement.status='APPROVED' AND NOT EXISTS(
			SELECT 1 FROM requirements newer WHERE newer.supersedes_requirement_id=requirement.id)`,
		setID, sourceSnapshotID, revisionIDs).Scan(&coveredIDs, &uncoveredIDs); err != nil {
		return SuiteRelease{}, false, err
	}
	scopeStatus := ReleaseScopeComplete
	if len(uncoveredIDs) > 0 {
		scopeStatus = ReleaseScopePartial
		if input.ScopeDecision == "" {
			return SuiteRelease{}, false, ErrReleaseScope
		}
	}
	var releaseNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(release_number),0)+1
		FROM test_suite_releases WHERE test_suite_id=$1`, input.TestSuiteID).
		Scan(&releaseNumber); err != nil {
		return SuiteRelease{}, false, err
	}
	var releaseID int64
	if err := tx.QueryRow(ctx, `INSERT INTO test_suite_releases
		(test_suite_id,document_set_id,release_number,name,source_snapshot_id,manifest_hash,
		 request_hash,scope_status,approved_requirement_count,covered_requirement_count,
		 uncovered_requirement_ids,scope_decision,published_by,idempotency_key)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		input.TestSuiteID, setID, releaseNumber, fmt.Sprintf("R%d", releaseNumber),
		sourceSnapshotID, manifestHash, requestHash, scopeStatus, approvedCount,
		len(coveredIDs), uncoveredIDs, input.ScopeDecision, input.PublishedBy,
		idempotencyKey).Scan(&releaseID); err != nil {
		return SuiteRelease{}, false, fmt.Errorf("publish suite release: %w", err)
	}
	for index, item := range revisions {
		if _, err := tx.Exec(ctx, `INSERT INTO test_suite_release_items
			(release_id,test_suite_id,document_set_id,family_id,test_case_id,ordinal,
			 public_key,revision_number,content_hash,expected_result_hash)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, releaseID, input.TestSuiteID,
			setID, item.FamilyID, item.ID, index+1, item.TestCaseKey,
			item.VersionNumber, item.ContentHash, item.ExpectedResultHash); err != nil {
			return SuiteRelease{}, false, fmt.Errorf("publish suite release item: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SuiteRelease{}, false, fmt.Errorf("commit suite release: %w", err)
	}
	release, err := r.GetRelease(ctx, releaseID)
	return release, true, err
}

func (r *Repository) ListReleases(ctx context.Context, setID int64) ([]SuiteRelease, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := r.pool.Query(ctx, `SELECT id FROM test_suite_releases
		WHERE document_set_id=$1 ORDER BY published_at DESC,id DESC`, setID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	result := make([]SuiteRelease, 0, len(ids))
	for _, id := range ids {
		item, err := r.GetRelease(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *Repository) GetRelease(ctx context.Context, releaseID int64) (SuiteRelease, error) {
	if releaseID <= 0 {
		return SuiteRelease{}, ErrInvalidInput
	}
	result := SuiteRelease{Items: []ReleaseItem{}, UncoveredRequirementIDs: []int64{}}
	err := r.pool.QueryRow(ctx, `SELECT id,test_suite_id,document_set_id,release_number,name,
		source_snapshot_id,manifest_hash,scope_status,approved_requirement_count,
		covered_requirement_count,uncovered_requirement_ids,scope_decision,published_by,
		origin,published_at,created_at FROM test_suite_releases WHERE id=$1`, releaseID).
		Scan(&result.ID, &result.TestSuiteID, &result.DocumentSetID, &result.ReleaseNumber,
			&result.Name, &result.SourceSnapshotID, &result.ManifestHash, &result.ScopeStatus,
			&result.ApprovedRequirementCount, &result.CoveredRequirementCount,
			&result.UncoveredRequirementIDs, &result.ScopeDecision, &result.PublishedBy,
			&result.Origin, &result.PublishedAt, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SuiteRelease{}, ErrNotFound
	}
	if err != nil {
		return SuiteRelease{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT item.release_id,item.family_id,item.test_case_id,
		item.ordinal,item.public_key,item.revision_number,item.content_hash,
		item.expected_result_hash,`+releaseTestCaseColumnList+`
		FROM test_suite_release_items item JOIN test_cases t ON t.id=item.test_case_id
		WHERE item.release_id=$1 ORDER BY item.ordinal`, releaseID)
	if err != nil {
		return SuiteRelease{}, err
	}
	for rows.Next() {
		var item ReleaseItem
		if err := rows.Scan(append([]any{&item.ReleaseID, &item.FamilyID, &item.TestCaseID,
			&item.Ordinal, &item.PublicKey, &item.RevisionNumber, &item.ContentHash,
			&item.ExpectedResultHash}, testCaseDestinations(&item.Revision)...)...); err != nil {
			rows.Close()
			return SuiteRelease{}, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return SuiteRelease{}, err
	}
	rows.Close()
	sort.SliceStable(result.Items, func(i, j int) bool { return result.Items[i].Ordinal < result.Items[j].Ordinal })
	return result, nil
}
