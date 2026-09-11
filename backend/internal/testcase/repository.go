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
	if suite.ID <= 0 || suite.DocumentSetID <= 0 || len(proposal.RequirementIDs) == 0 ||
		strings.TrimSpace(proposal.Title) == "" || strings.TrimSpace(proposal.ExpectedResult) == "" ||
		!validTestType(proposal.TestType) {
		return TestCase{}, false, ErrInvalidInput
	}
	proposal.RequirementIDs = uniqueSortedIDs(proposal.RequirementIDs)
	var approvedCount int
	if err := r.pool.QueryRow(ctx, `SELECT count(DISTINCT r.id) FROM requirements r
		WHERE r.document_set_id=$1 AND r.id=ANY($2) AND r.status='APPROVED'
		AND EXISTS(SELECT 1 FROM requirement_evidence e WHERE e.requirement_id=r.id)`,
		suite.DocumentSetID, proposal.RequirementIDs).Scan(&approvedCount); err != nil {
		return TestCase{}, false, fmt.Errorf("validate test case sources: %w", err)
	}
	if approvedCount != len(proposal.RequirementIDs) {
		return TestCase{}, false, ErrNoApprovedSource
	}
	logicalKey := logicalCaseKey(proposal)
	generationKey := generationKey(proposal)
	expectedHash := hash(proposal.ExpectedResult)
	if proposal.Assumptions == nil {
		proposal.Assumptions = []string{}
	}
	assumptions, _ := json.Marshal(proposal.Assumptions)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TestCase{}, false, fmt.Errorf("begin save test case: %w", err)
	}
	defer tx.Rollback(ctx)
	var previous TestCase
	err = tx.QueryRow(ctx, `SELECT id,test_suite_id,document_set_id,test_case_key,version_number,
		title,test_type,risk,actor,precondition,test_data,expected_result,expected_result_hash,
		postcondition,status,automation_status,confidence,generated_by,assumptions,generation_key,
		supersedes_test_case_id,created_at,updated_at FROM test_cases
		WHERE test_suite_id=$1 AND test_case_key=$2 ORDER BY version_number DESC LIMIT 1 FOR UPDATE`,
		suite.ID, logicalKey).Scan(testCaseDestinations(&previous)...)
	if err == nil && previous.GenerationKey == generationKey {
		if err := saveRequirementLinks(ctx, tx, previous.ID, suite.DocumentSetID, proposal); err != nil {
			return TestCase{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return TestCase{}, false, fmt.Errorf("commit reused test case: %w", err)
		}
		return previous, false, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return TestCase{}, false, fmt.Errorf("find previous test case: %w", err)
	}
	version := 1
	var supersedes *int64
	if err == nil {
		version = previous.VersionNumber + 1
		supersedes = &previous.ID
	}
	const insert = `INSERT INTO test_cases
		(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,risk,
		 actor,precondition,test_data,expected_result,expected_result_hash,postcondition,status,
		 automation_status,confidence,generated_by,assumptions,generation_key,supersedes_test_case_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'DRAFT',$14,$15,$16,$17,$18,$19)
		RETURNING id,test_suite_id,document_set_id,test_case_key,version_number,title,test_type,
		risk,actor,precondition,test_data,expected_result,expected_result_hash,postcondition,
		status,automation_status,confidence,generated_by,assumptions,generation_key,
		supersedes_test_case_id,created_at,updated_at`
	var result TestCase
	if err := tx.QueryRow(ctx, insert, suite.ID, suite.DocumentSetID, logicalKey, version,
		strings.TrimSpace(proposal.Title), proposal.TestType, defaultString(proposal.Risk, "MEDIUM"),
		strings.TrimSpace(proposal.Actor), strings.TrimSpace(proposal.Precondition),
		strings.TrimSpace(proposal.TestData), strings.TrimSpace(proposal.ExpectedResult), expectedHash,
		strings.TrimSpace(proposal.Postcondition), defaultString(proposal.AutomationStatus, "MANUAL"),
		proposal.Confidence, defaultString(proposal.GeneratedBy, "RULE_ENGINE"), assumptions,
		generationKey, supersedes).Scan(testCaseDestinations(&result)...); err != nil {
		return TestCase{}, false, fmt.Errorf("insert test case: %w", err)
	}
	for index, step := range proposal.Steps {
		if strings.TrimSpace(step.Action) == "" {
			return TestCase{}, false, ErrInvalidInput
		}
		if _, err := tx.Exec(ctx, `INSERT INTO test_case_steps
			(test_case_id,ordinal,action,expected_result) VALUES($1,$2,$3,$4)`,
			result.ID, index+1, strings.TrimSpace(step.Action), strings.TrimSpace(step.ExpectedResult)); err != nil {
			return TestCase{}, false, fmt.Errorf("insert test case step: %w", err)
		}
	}
	if err := saveRequirementLinks(ctx, tx, result.ID, suite.DocumentSetID, proposal); err != nil {
		return TestCase{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TestCase{}, false, fmt.Errorf("commit test case: %w", err)
	}
	return result, true, nil
}

func saveRequirementLinks(ctx context.Context, tx pgx.Tx, caseID, setID int64, proposal Proposal) error {
	coverageType := coverageTypeFor(proposal.TestType)
	for _, requirementID := range proposal.RequirementIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO test_case_requirement_links
			(test_case_id,requirement_id,document_set_id,coverage_type)
			VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, caseID, requirementID, setID,
			coverageType); err != nil {
			return fmt.Errorf("link test case requirement: %w", err)
		}
	}
	return nil
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

func (r *Repository) AddRequirementLinks(ctx context.Context, caseID, setID int64,
	proposal Proposal,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin merge test case sources: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := saveRequirementLinks(ctx, tx, caseID, setID, proposal); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) List(ctx context.Context, setID int64) ([]TestCase, error) {
	const query = `SELECT t.id,t.test_suite_id,t.document_set_id,t.test_case_key,t.version_number,
		t.title,t.test_type,t.risk,t.actor,t.precondition,t.test_data,t.expected_result,
		t.expected_result_hash,t.postcondition,t.status,t.automation_status,t.confidence,
		t.generated_by,t.assumptions,t.generation_key,t.supersedes_test_case_id,t.created_at,t.updated_at
		FROM test_cases t WHERE t.document_set_id=$1
		AND NOT EXISTS(SELECT 1 FROM test_cases newer WHERE newer.supersedes_test_case_id=t.id)
		ORDER BY t.test_case_key,t.version_number DESC`
	rows, err := r.pool.Query(ctx, query, setID)
	if err != nil {
		return nil, fmt.Errorf("list test cases: %w", err)
	}
	defer rows.Close()
	results := make([]TestCase, 0)
	for rows.Next() {
		var item TestCase
		if err := rows.Scan(testCaseDestinations(&item)...); err != nil {
			return nil, fmt.Errorf("scan test case: %w", err)
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
	if err := r.pool.QueryRow(ctx, `SELECT id,test_suite_id,document_set_id,test_case_key,
		version_number,title,test_type,risk,actor,precondition,test_data,expected_result,
		expected_result_hash,postcondition,status,automation_status,confidence,generated_by,
		assumptions,generation_key,supersedes_test_case_id,created_at,updated_at
		FROM test_cases WHERE id=$1`, id).Scan(testCaseDestinations(&detail.TestCase)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get test case: %w", err)
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
			FROM requirement_evidence e JOIN document_versions v ON v.id=e.document_version_id
			JOIN documents d ON d.id=v.document_id JOIN document_blocks b ON b.id=e.document_block_id
			WHERE e.requirement_id=ANY($1) ORDER BY e.requirement_id,e.id`, requirementIDs)
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
		created_at FROM test_case_reviews WHERE test_case_id=$1 ORDER BY created_at,id`, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list test reviews: %w", err)
	}
	for reviewRows.Next() {
		var item Review
		if err := reviewRows.Scan(&item.ID, &item.TestCaseID, &item.ReviewerName,
			&item.Decision, &item.Comment, &item.CreatedAt); err != nil {
			reviewRows.Close()
			return Detail{}, fmt.Errorf("scan test review: %w", err)
		}
		detail.Reviews = append(detail.Reviews, item)
	}
	reviewRows.Close()
	return detail, nil
}

func (r *Repository) Review(ctx context.Context, id int64, input ReviewInput) (Detail, error) {
	input.ReviewerName = strings.TrimSpace(input.ReviewerName)
	input.Decision = strings.ToUpper(strings.TrimSpace(input.Decision))
	if input.ReviewerName == "" || input.Decision != StatusApproved && input.Decision != StatusRejected {
		return Detail{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin test case review: %w", err)
	}
	defer tx.Rollback(ctx)
	var current TestCase
	if err := tx.QueryRow(ctx, `SELECT id,test_suite_id,document_set_id,test_case_key,
		version_number,title,test_type,risk,actor,precondition,test_data,expected_result,
		expected_result_hash,postcondition,status,automation_status,confidence,generated_by,
		assumptions,generation_key,supersedes_test_case_id,created_at,updated_at
		FROM test_cases WHERE id=$1 FOR UPDATE`, id).Scan(testCaseDestinations(&current)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("lock test case: %w", err)
	}
	targetID := current.ID
	if hasTestCaseEdits(current, input) {
		title := defaultString(strings.TrimSpace(input.Title), current.Title)
		expected := defaultString(strings.TrimSpace(input.ExpectedResult), current.ExpectedResult)
		if title == "" || expected == "" {
			return Detail{}, ErrInvalidInput
		}
		if err := tx.QueryRow(ctx, `INSERT INTO test_cases
			(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,risk,actor,
			 precondition,test_data,expected_result,expected_result_hash,postcondition,status,
			 automation_status,confidence,generated_by,assumptions,generation_key,supersedes_test_case_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'DRAFT',$14,$15,$16,$17,$18,$19)
			RETURNING id`, current.TestSuiteID, current.DocumentSetID, current.TestCaseKey,
			current.VersionNumber+1, title, current.TestType,
			defaultString(strings.ToUpper(strings.TrimSpace(input.Risk)), current.Risk), current.Actor,
			defaultString(strings.TrimSpace(input.Precondition), current.Precondition),
			defaultString(strings.TrimSpace(input.TestData), current.TestData), expected, hash(expected),
			defaultString(strings.TrimSpace(input.Postcondition), current.Postcondition),
			current.AutomationStatus, current.Confidence, current.GeneratedBy, current.Assumptions,
			hash(current.GenerationKey+"\x00"+expected), current.ID).Scan(&targetID); err != nil {
			return Detail{}, fmt.Errorf("version edited test case: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO test_case_steps(test_case_id,ordinal,action,expected_result)
			SELECT $1,ordinal,action,expected_result FROM test_case_steps WHERE test_case_id=$2`,
			targetID, current.ID); err != nil {
			return Detail{}, fmt.Errorf("copy test case steps: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO test_case_requirement_links
			(test_case_id,requirement_id,document_set_id,coverage_type)
			SELECT $1,requirement_id,document_set_id,coverage_type
			FROM test_case_requirement_links WHERE test_case_id=$2`, targetID, current.ID); err != nil {
			return Detail{}, fmt.Errorf("copy test case requirement links: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE test_cases SET status=$2,updated_at=NOW() WHERE id=$1`,
		targetID, input.Decision); err != nil {
		return Detail{}, fmt.Errorf("apply test case decision: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO test_case_reviews
		(test_case_id,reviewer_name,decision,comment) VALUES($1,$2,$3,$4)`,
		targetID, input.ReviewerName, input.Decision, strings.TrimSpace(input.Comment)); err != nil {
		return Detail{}, fmt.Errorf("save test case review: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit test case review: %w", err)
	}
	return r.Get(ctx, targetID)
}

func (r *Repository) Coverage(ctx context.Context, setID int64) (CoverageReport, error) {
	report := CoverageReport{DocumentSetID: setID, Matrix: make([]CoverageCell, 0)}
	rows, err := r.pool.Query(ctx, `SELECT r.id,r.requirement_key,r.title,r.status,r.flow_type,
		COALESCE(array_remove(array_agg(DISTINCT t.id),NULL),'{}'),
		COALESCE(array_remove(array_agg(DISTINCT t.test_type) FILTER (WHERE t.status<>'REJECTED'),NULL),'{}'),
		COALESCE(bool_or(t.id IS NOT NULL AND t.status<>'REJECTED'),FALSE)
		FROM requirements r
		LEFT JOIN test_case_requirement_links l ON l.requirement_id=r.id
		LEFT JOIN test_cases t ON t.id=l.test_case_id
			AND NOT EXISTS(SELECT 1 FROM test_cases newer WHERE newer.supersedes_test_case_id=t.id)
		WHERE r.document_set_id=$1
		AND NOT EXISTS(SELECT 1 FROM requirements newer WHERE newer.supersedes_requirement_id=r.id)
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
	return report, rows.Err()
}

func testCaseDestinations(item *TestCase) []any {
	return []any{&item.ID, &item.TestSuiteID, &item.DocumentSetID, &item.TestCaseKey,
		&item.VersionNumber, &item.Title, &item.TestType, &item.Risk, &item.Actor,
		&item.Precondition, &item.TestData, &item.ExpectedResult, &item.ExpectedResultHash,
		&item.Postcondition, &item.Status, &item.AutomationStatus, &item.Confidence,
		&item.GeneratedBy, &item.Assumptions, &item.GenerationKey, &item.SupersedesID,
		&item.CreatedAt, &item.UpdatedAt}
}

func logicalCaseKey(proposal Proposal) string {
	keys := append([]string(nil), proposal.RequirementKeys...)
	sort.Strings(keys)
	digest := sha256.Sum256([]byte(proposal.TestType + "\x00" + strings.Join(keys, "\x00")))
	return "TC-" + strings.ToUpper(hex.EncodeToString(digest[:5]))
}

func generationKey(proposal Proposal) string {
	ids := uniqueSortedIDs(proposal.RequirementIDs)
	return hash(fmt.Sprint(ids) + "\x00" + proposalIdentity(proposal))
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
