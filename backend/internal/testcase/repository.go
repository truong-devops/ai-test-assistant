package testcase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) EnsureSuite(ctx context.Context, setID int64) (Suite, error) {
	const query = `INSERT INTO test_suites(document_set_id,name,description)
		VALUES($1,'Generated business baseline','Document-grounded cases generated from approved requirements')
		ON CONFLICT(document_set_id,name) DO UPDATE SET updated_at=NOW()
		RETURNING id,document_set_id,name,description,status,created_at,updated_at`
	var result Suite
	if err := r.pool.QueryRow(ctx, query, setID).Scan(&result.ID, &result.DocumentSetID,
		&result.Name, &result.Description, &result.Status, &result.CreatedAt, &result.UpdatedAt); err != nil {
		return Suite{}, fmt.Errorf("ensure generated test suite: %w", err)
	}
	return result, nil
}

func (r *Repository) Save(ctx context.Context, suite Suite, proposal Proposal) (TestCase, bool, error) {
	return r.SaveGenerated(ctx, suite, proposal)
}

func (r *Repository) RecordDedupe(ctx context.Context, suite Suite, keptID int64,
	suppressedKey, matchType, reason string,
) (bool, error) {
	result, err := r.pool.Exec(ctx, `INSERT INTO test_case_dedupe_records
		(document_set_id,test_suite_id,kept_test_case_id,suppressed_key,match_type,reason)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(test_suite_id,suppressed_key) DO NOTHING`,
		suite.DocumentSetID, suite.ID, keptID, suppressedKey, matchType, reason)
	if err != nil {
		return false, fmt.Errorf("record test case dedupe: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *Repository) List(ctx context.Context, setID int64) ([]TestCase, error) {
	const query = `SELECT ` + testCaseColumnList + `,
		COALESCE((SELECT jsonb_build_object(
			'test_run_id',run.id,'status',item.status,'actual_result',item.actual_result,
			'run_at',COALESCE(run.finished_at,run.started_at,run.requested_at))
			FROM test_run_items item JOIN test_runs run ON run.id=item.test_run_id
			WHERE item.test_case_id=test_cases.id
			ORDER BY COALESCE(run.finished_at,run.started_at,run.requested_at) DESC,
				item.attempt_number DESC,item.id DESC LIMIT 1),'null'::jsonb),
		EXISTS(SELECT 1 FROM test_case_requirement_links l WHERE l.test_case_id=test_cases.id AND NOT requirement_is_current(l.requirement_id))
		FROM test_cases
		WHERE id IN (SELECT head_revision_id FROM test_case_families
			WHERE document_set_id=$1 AND archived=FALSE)
		ORDER BY test_case_key`
	rows, err := r.pool.Query(ctx, query, setID)
	if err != nil {
		return nil, fmt.Errorf("list test cases: %w", err)
	}
	defer rows.Close()
	results := make([]TestCase, 0)
	for rows.Next() {
		var item TestCase
		var executionJSON []byte
		if err := rows.Scan(append(testCaseDestinations(&item), &executionJSON, &item.NeedsSourceReview)...); err != nil {
			return nil, fmt.Errorf("scan test case: %w", err)
		}
		if string(executionJSON) != "null" {
			var execution ExecutionState
			if err := json.Unmarshal(executionJSON, &execution); err != nil {
				return nil, fmt.Errorf("decode latest test execution: %w", err)
			}
			item.LatestExecution = &execution
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id int64) (Detail, error) {
	detail := Detail{
		Steps:        make([]Step, 0),
		Requirements: make([]RequirementLink, 0),
		Evidence:     make([]requirement.Evidence, 0),
		Reviews:      make([]Review, 0),
	}
	if err := r.pool.QueryRow(ctx, `SELECT `+testCaseColumnList+`
		FROM test_cases WHERE id=$1`, id).Scan(testCaseDestinations(&detail.TestCase)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get test case: %w", err)
	}
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM test_case_requirement_links WHERE test_case_id=$1 AND NOT requirement_is_current(requirement_id))`, id).Scan(&detail.TestCase.NeedsSourceReview); err != nil {
		return Detail{}, err
	}
	var execution ExecutionState
	err := r.pool.QueryRow(ctx, `SELECT run.id,item.status,item.actual_result,
		COALESCE(run.finished_at,run.started_at,run.requested_at)
		FROM test_run_items item JOIN test_runs run ON run.id=item.test_run_id
		WHERE item.test_case_id=$1
		ORDER BY COALESCE(run.finished_at,run.started_at,run.requested_at) DESC,
		item.attempt_number DESC,item.id DESC LIMIT 1`, id).
		Scan(&execution.TestRunID, &execution.Status, &execution.ActualResult, &execution.RunAt)
	if err == nil {
		detail.TestCase.LatestExecution = &execution
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, fmt.Errorf("get revision execution: %w", err)
	}
	stepRows, err := r.pool.Query(ctx, `SELECT id,test_case_id,ordinal,action,expected_result,
		created_at FROM test_case_steps WHERE test_case_id=$1 ORDER BY ordinal`, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list test steps: %w", err)
	}
	for stepRows.Next() {
		var item Step
		if err := stepRows.Scan(&item.ID, &item.TestCaseID, &item.Ordinal, &item.Action,
			&item.ExpectedResult, &item.CreatedAt); err != nil {
			stepRows.Close()
			return Detail{}, fmt.Errorf("scan test step: %w", err)
		}
		detail.Steps = append(detail.Steps, item)
	}
	stepRows.Close()
	linkRows, err := r.pool.Query(ctx, `SELECT l.test_case_id,l.requirement_id,l.document_set_id,
		l.coverage_type,r.requirement_key,r.title,r.flow_type,l.created_at
		FROM test_case_requirement_links l JOIN requirements r ON r.id=l.requirement_id
		WHERE l.test_case_id=$1 ORDER BY r.requirement_key,l.coverage_type`, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list test requirement links: %w", err)
	}
	requirementIDs := make([]int64, 0)
	for linkRows.Next() {
		var item RequirementLink
		if err := linkRows.Scan(&item.TestCaseID, &item.RequirementID, &item.DocumentSetID,
			&item.CoverageType, &item.RequirementKey, &item.RequirementTitle,
			&item.FlowType, &item.CreatedAt); err != nil {
			linkRows.Close()
			return Detail{}, fmt.Errorf("scan test requirement link: %w", err)
		}
		detail.Requirements = append(detail.Requirements, item)
		requirementIDs = append(requirementIDs, item.RequirementID)
	}
	linkRows.Close()
	if len(requirementIDs) > 0 {
		evidenceRows, err := r.pool.Query(ctx, `SELECT e.id,e.requirement_id,e.document_set_id,
			e.document_version_id,e.document_block_id,d.name,v.version_number,v.approval_status,
			e.source_locator,b.content,e.excerpt_hash,e.created_at
			FROM test_case_evidence_links tel
			JOIN requirement_evidence e ON e.id=tel.requirement_evidence_id
			JOIN document_versions v ON v.id=e.document_version_id
			JOIN documents d ON d.id=v.document_id JOIN document_blocks b ON b.id=e.document_block_id
			WHERE tel.test_case_id=$1 ORDER BY e.requirement_id,e.id`, id)
		if err != nil {
			return Detail{}, fmt.Errorf("list expected-result evidence: %w", err)
		}
		for evidenceRows.Next() {
			var item requirement.Evidence
			if err := evidenceRows.Scan(&item.ID, &item.RequirementID, &item.DocumentSetID,
				&item.DocumentVersionID, &item.DocumentBlockID, &item.DocumentName,
				&item.VersionNumber, &item.ApprovalStatus, &item.SourceLocator, &item.Excerpt,
				&item.ExcerptHash, &item.CreatedAt); err != nil {
				evidenceRows.Close()
				return Detail{}, fmt.Errorf("scan test evidence: %w", err)
			}
			detail.Evidence = append(detail.Evidence, item)
		}
		evidenceRows.Close()
	}
	reviewRows, err := r.pool.Query(ctx, `SELECT id,test_case_id,reviewer_name,decision,comment,
		content_hash,actor,created_at FROM test_case_reviews WHERE test_case_id=$1 ORDER BY created_at,id`, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list test reviews: %w", err)
	}
	for reviewRows.Next() {
		var item Review
		if err := reviewRows.Scan(&item.ID, &item.TestCaseID, &item.ReviewerName,
			&item.Decision, &item.Comment, &item.ContentHash, &item.Actor, &item.CreatedAt); err != nil {
			reviewRows.Close()
			return Detail{}, fmt.Errorf("scan test review: %w", err)
		}
		detail.Reviews = append(detail.Reviews, item)
	}
	reviewRows.Close()
	return detail, nil
}

func (r *Repository) Coverage(ctx context.Context, setID int64) (CoverageReport, error) {
	report := CoverageReport{DocumentSetID: setID, Matrix: make([]CoverageCell, 0),
		Layers: make([]CoverageLayer, 0, 4)}
	rows, err := r.pool.Query(ctx, `SELECT r.id,r.requirement_key,r.title,r.status,r.flow_type,
		COALESCE(array_remove(array_agg(DISTINCT t.id),NULL),'{}'),
		COALESCE(array_remove(array_agg(DISTINCT t.test_type) FILTER (WHERE t.status<>'REJECTED'),NULL),'{}'),
		COALESCE(bool_or(t.id IS NOT NULL AND t.status<>'REJECTED'),FALSE)
		FROM requirements r
		LEFT JOIN test_case_requirement_links l ON l.requirement_id=r.id
		LEFT JOIN test_cases t ON t.id=l.test_case_id AND EXISTS (
			SELECT 1 FROM test_case_families f
			WHERE f.id=t.family_id AND f.head_revision_id=t.id AND f.archived=FALSE)
		WHERE r.document_set_id=$1
		AND requirement_is_current(r.id)
		GROUP BY r.id ORDER BY r.requirement_key,r.id`, setID)
	if err != nil {
		return report, fmt.Errorf("calculate requirement coverage: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cell CoverageCell
		var hasActiveCase bool
		if err := rows.Scan(&cell.RequirementID, &cell.RequirementKey, &cell.Title,
			&cell.Status, &cell.FlowType, &cell.TestCaseIDs, &cell.TestTypes, &hasActiveCase); err != nil {
			return report, fmt.Errorf("scan coverage cell: %w", err)
		}
		if cell.TestCaseIDs == nil {
			cell.TestCaseIDs = make([]int64, 0)
		}
		if cell.TestTypes == nil {
			cell.TestTypes = make([]string, 0)
		}
		cell.Warnings = make([]string, 0)
		cell.Covered = cell.Status == requirement.StatusApproved && hasActiveCase
		switch cell.Status {
		case requirement.StatusApproved:
			report.ApprovedDenominator++
			if cell.Covered {
				report.CoveredCount++
			} else {
				report.UncoveredCount++
				cell.Warnings = append(cell.Warnings, "Requirement đã duyệt chưa có test case")
			}
			if !containsType(cell.TestTypes, TypeHappy) && cell.FlowType != "EXCEPTION" {
				cell.Warnings = append(cell.Warnings, "Thiếu positive/happy coverage")
			}
			if cell.FlowType == "EXCEPTION" && !containsType(cell.TestTypes, TypeNegative) {
				cell.Warnings = append(cell.Warnings, "Exception flow chưa có negative coverage")
			}
		case requirement.StatusConflict:
			report.ConflictCount++
		case requirement.StatusTBD:
			report.TBDCount++
		case requirement.StatusRejected:
			report.RejectedCount++
		}
		report.Matrix = append(report.Matrix, cell)
	}
	if report.ApprovedDenominator > 0 {
		report.CoveragePercent = float64(report.CoveredCount) * 100 / float64(report.ApprovedDenominator)
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM test_case_dedupe_records
		WHERE document_set_id=$1`, setID).Scan(&report.DuplicateCount); err != nil {
		return report, fmt.Errorf("count suppressed duplicate tests: %w", err)
	}
	report.BaselineComplete = report.ApprovedDenominator > 0 && report.UncoveredCount == 0 &&
		report.ConflictCount == 0 && report.TBDCount == 0
	var workingSnapshotID *int64
	if err := r.pool.QueryRow(ctx, `SELECT CASE WHEN count(DISTINCT source_snapshot_id)
		FILTER (WHERE source_snapshot_id IS NOT NULL)=1 THEN min(source_snapshot_id) ELSE NULL END
		FROM requirements WHERE document_set_id=$1 AND status='APPROVED'
		AND requirement_is_current(requirements.id)`, setID).Scan(&workingSnapshotID); err != nil {
		return report, fmt.Errorf("load working coverage source: %w", err)
	}
	report.Layers = append(report.Layers, coverageLayer("DESIGNED", "Có thiết kế",
		report.CoveredCount, report.ApprovedDenominator, workingSnapshotID, nil))
	var releaseID int64
	var releaseNumber int
	var sourceSnapshotID *int64
	var approvedRequirements, coveredRequirements, releaseCases, automatedCases, executedCases int
	err = r.pool.QueryRow(ctx, `SELECT release.id,release.release_number,release.source_snapshot_id,
		release.approved_requirement_count,release.covered_requirement_count,
		count(item.test_case_id),
		count(item.test_case_id) FILTER (WHERE EXISTS(
			SELECT 1 FROM automation_artifacts artifact
			WHERE artifact.test_case_id=item.test_case_id AND artifact.status='APPROVED'
			AND artifact.expected_result_hash=item.expected_result_hash)),
		count(item.test_case_id) FILTER (WHERE EXISTS(
			SELECT 1 FROM test_runs run JOIN test_run_items run_item ON run_item.test_run_id=run.id
			WHERE run.suite_release_id=release.id AND run_item.test_case_id=item.test_case_id
			AND run_item.status<>'NOT_RUN'))
		FROM test_suite_releases release
		JOIN test_suite_release_items item ON item.release_id=release.id
		WHERE release.id=(SELECT latest.id FROM test_suite_releases latest
			WHERE latest.document_set_id=$1 ORDER BY latest.release_number DESC,latest.id DESC LIMIT 1)
		GROUP BY release.id`, setID).Scan(&releaseID, &releaseNumber, &sourceSnapshotID,
		&approvedRequirements, &coveredRequirements, &releaseCases, &automatedCases,
		&executedCases)
	if err == nil {
		report.Layers = append(report.Layers,
			coverageLayer("PUBLISHED", "Đã duyệt trong bộ", coveredRequirements,
				approvedRequirements, sourceSnapshotID, &releaseID, &releaseNumber),
			coverageLayer("AUTOMATED", "Có automation", automatedCases,
				releaseCases, sourceSnapshotID, &releaseID, &releaseNumber),
			coverageLayer("EXECUTED", "Đã chạy", executedCases,
				releaseCases, sourceSnapshotID, &releaseID, &releaseNumber))
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return report, fmt.Errorf("load published coverage layers: %w", err)
	}
	return report, rows.Err()
}

func coverageLayer(key, label string, numerator, denominator int,
	sourceSnapshotID, suiteReleaseID *int64, releaseNumber ...*int,
) CoverageLayer {
	result := CoverageLayer{Key: key, Label: label, Numerator: numerator,
		Denominator: denominator, SourceSnapshotID: sourceSnapshotID,
		SuiteReleaseID: suiteReleaseID}
	if len(releaseNumber) > 0 {
		result.ReleaseNumber = releaseNumber[0]
	}
	if denominator > 0 {
		result.Percent = float64(numerator) * 100 / float64(denominator)
	}
	return result
}

func testCaseDestinations(item *TestCase) []any {
	return []any{&item.ID, &item.TestSuiteID, &item.DocumentSetID, &item.TestCaseKey,
		&item.VersionNumber, &item.Title, &item.TestType, &item.Risk, &item.Actor,
		&item.Precondition, &item.TestData, &item.ExpectedResult, &item.ExpectedResultHash,
		&item.Postcondition, &item.Status, &item.AutomationStatus, &item.Confidence,
		&item.GeneratedBy, &item.Assumptions, &item.GenerationKey, &item.SupersedesID,
		&item.FamilyID, &item.ParentRevisionID, &item.RestoredFromID, &item.ContentHash,
		&item.CreatedBy, &item.ChangeReason, &item.SourceSnapshotID, &item.Provenance,
		&item.SealedAt,
		&item.CreatedAt, &item.UpdatedAt}
}

func hash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func uniqueSortedIDs(values []int64) []int64 {
	result := append([]int64(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return uniqueInt64(result)
}

func coverageTypeFor(testType string) string {
	switch testType {
	case TypeNegative, TypeBoundary, TypePermission, TypeState, TypeIntegration, TypeRegression:
		return testType
	default:
		return "DIRECT"
	}
}

func validTestType(value string) bool {
	switch value {
	case TypeHappy, TypeNegative, TypeBoundary, TypePermission, TypeState, TypeIntegration, TypeRegression, TypeNFR:
		return true
	default:
		return false
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func hasTestCaseEdits(current TestCase, input ReviewInput) bool {
	return input.Title != "" && strings.TrimSpace(input.Title) != current.Title ||
		input.Precondition != "" && strings.TrimSpace(input.Precondition) != current.Precondition ||
		input.TestData != "" && strings.TrimSpace(input.TestData) != current.TestData ||
		input.ExpectedResult != "" && strings.TrimSpace(input.ExpectedResult) != current.ExpectedResult ||
		input.Postcondition != "" && strings.TrimSpace(input.Postcondition) != current.Postcondition ||
		input.Risk != "" && strings.ToUpper(strings.TrimSpace(input.Risk)) != current.Risk
}

func containsType(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
