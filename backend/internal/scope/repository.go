package scope

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) BaselineView(ctx context.Context, projectID int64) (BaselineView, error) {
	if projectID <= 0 {
		return BaselineView{}, ErrInvalidInput
	}
	result := BaselineView{Candidates: []Candidate{}}
	rows, err := r.pool.Query(ctx, `SELECT d.id,d.name,s.id,s.name,release.id,
		release.release_number,release.name,release.manifest_hash,release.published_at,
		count(item.test_case_id)
		FROM document_sets d JOIN test_suites s ON s.document_set_id=d.id
		JOIN test_suite_releases release ON release.test_suite_id=s.id
		LEFT JOIN test_suite_release_items item ON item.release_id=release.id
		WHERE d.status='ACTIVE'
		GROUP BY d.id,d.name,s.id,s.name,release.id
		ORDER BY d.name,s.name,release.release_number DESC`)
	if err != nil {
		return result, fmt.Errorf("list baseline candidates: %w", err)
	}
	for rows.Next() {
		var item Candidate
		if err := rows.Scan(&item.DocumentSetID, &item.DocumentSetName, &item.TestSuiteID,
			&item.TestSuiteName, &item.SuiteReleaseID, &item.ReleaseNumber,
			&item.ReleaseName, &item.ManifestHash, &item.ReleasePublishedAt,
			&item.ApprovedTestCases); err != nil {
			rows.Close()
			return result, err
		}
		result.Candidates = append(result.Candidates, item)
	}
	rows.Close()
	var baseline Baseline
	err = r.pool.QueryRow(ctx, `SELECT b.project_id,b.document_set_id,d.name,b.test_suite_id,s.name,
		release.id,release.release_number,release.name,release.manifest_hash,release.published_at,
		b.selection_mode,b.selected_by,b.created_at,b.updated_at
		FROM project_document_baselines b JOIN document_sets d ON d.id=b.document_set_id
		JOIN test_suites s ON s.id=b.test_suite_id
		JOIN test_suite_releases release ON release.id=b.suite_release_id
		WHERE b.project_id=$1`, projectID).Scan(
		&baseline.ProjectID, &baseline.DocumentSetID, &baseline.DocumentSetName, &baseline.TestSuiteID,
		&baseline.TestSuiteName, &baseline.SuiteReleaseID, &baseline.ReleaseNumber,
		&baseline.ReleaseName, &baseline.ManifestHash, &baseline.ReleasePublishedAt, &baseline.SelectionMode,
		&baseline.SelectedBy, &baseline.CreatedAt, &baseline.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("get project baseline: %w", err)
	}
	result.Bound = true
	result.Baseline = &baseline
	return result, nil
}

func (r *Repository) Select(ctx context.Context, projectID int64, input SelectInput) (Baseline, error) {
	input.SelectionMode = strings.ToUpper(strings.TrimSpace(input.SelectionMode))
	input.SelectedBy = strings.TrimSpace(input.SelectedBy)
	if projectID <= 0 || input.SelectedBy == "" ||
		input.SelectionMode != ModeFullApproved && input.SelectionMode != ModeMappedFallback {
		return Baseline{}, ErrInvalidInput
	}
	if input.SuiteReleaseID <= 0 && (input.DocumentSetID <= 0 || input.TestSuiteID <= 0) {
		return Baseline{}, ErrInvalidInput
	}
	var setID, suiteID, releaseID int64
	query := `SELECT release.document_set_id,release.test_suite_id,release.id
		FROM test_suite_releases release JOIN document_sets d ON d.id=release.document_set_id
		WHERE d.status='ACTIVE' AND EXISTS(SELECT 1 FROM projects WHERE id=$1)`
	args := []any{projectID}
	if input.SuiteReleaseID > 0 {
		query += ` AND release.id=$2`
		args = append(args, input.SuiteReleaseID)
	} else {
		query += ` AND release.document_set_id=$2 AND release.test_suite_id=$3
			ORDER BY release.release_number DESC LIMIT 1`
		args = append(args, input.DocumentSetID, input.TestSuiteID)
	}
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&setID, &suiteID, &releaseID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Baseline{}, ErrNotFound
		}
		return Baseline{}, err
	}
	if input.DocumentSetID > 0 && input.DocumentSetID != setID ||
		input.TestSuiteID > 0 && input.TestSuiteID != suiteID {
		return Baseline{}, ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO project_document_baselines
		(project_id,document_set_id,test_suite_id,suite_release_id,selection_mode,selected_by)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(project_id) DO UPDATE SET
		document_set_id=EXCLUDED.document_set_id,test_suite_id=EXCLUDED.test_suite_id,
		suite_release_id=EXCLUDED.suite_release_id,selection_mode=EXCLUDED.selection_mode,
		selected_by=EXCLUDED.selected_by,updated_at=NOW()`, projectID, setID, suiteID,
		releaseID, input.SelectionMode, input.SelectedBy)
	if err != nil {
		return Baseline{}, fmt.Errorf("select project baseline: %w", err)
	}
	view, err := r.BaselineView(ctx, projectID)
	if err != nil {
		return Baseline{}, err
	}
	return *view.Baseline, nil
}

type requirementSnapshot struct {
	ID        int64    `json:"id"`
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Statement string   `json:"statement"`
	Evidence  []string `json:"evidence"`
}
type caseSnapshot struct {
	ID                 int64   `json:"id"`
	Key                string  `json:"key"`
	Title              string  `json:"title"`
	Type               string  `json:"type"`
	Risk               string  `json:"risk"`
	Actor              string  `json:"actor"`
	Preconditions      string  `json:"preconditions"`
	TestData           string  `json:"test_data"`
	ExpectedResult     string  `json:"expected_result"`
	ExpectedResultHash string  `json:"expected_result_hash"`
	RequirementIDs     []int64 `json:"requirement_ids"`
}

var explicitIDPattern = regexp.MustCompile(`(?i)\b(?:(?:UC|FR|BR|AC|NFR|REQ|US|STORY)(?:[-_][A-Z0-9]+|[0-9][A-Z0-9]*)|[A-Z][A-Z0-9]{1,9}-[0-9]+)(?:[.-][A-Z0-9]+)*\b`)

func (r *Repository) SnapshotForAnalysis(ctx context.Context, analysis job.AnalysisJob) (bool, error) {
	var pipelineMode string
	if err := r.pool.QueryRow(ctx, `SELECT pipeline_mode FROM projects WHERE id=$1`, analysis.ProjectID).Scan(&pipelineMode); err != nil {
		return false, err
	}
	if pipelineMode == "LEGACY" {
		return false, nil
	}
	var setID, suiteID, releaseID int64
	var configuredMode string
	err := r.pool.QueryRow(ctx, `SELECT document_set_id,test_suite_id,suite_release_id,
		selection_mode FROM project_document_baselines WHERE project_id=$1`, analysis.ProjectID).
		Scan(&setID, &suiteID, &releaseID, &configuredMode)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var active bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document_sets
		WHERE id=$1 AND status='ACTIVE')`, setID).Scan(&active); err != nil {
		return false, err
	}
	if !active {
		return false, nil
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM analysis_baseline_snapshots WHERE analysis_job_id=$1)`, analysis.ID).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	requirements := []requirementSnapshot{}
	requirementByID := map[int64]requirementSnapshot{}
	rows, err := r.pool.Query(ctx, `SELECT r.id,r.requirement_key,r.title,r.statement,
		COALESCE(array_agg(DISTINCT d.name || ' v' || v.version_number || ' — ' || e.source_locator) FILTER(WHERE e.id IS NOT NULL),'{}')
		FROM requirements r JOIN (
			SELECT DISTINCT link.requirement_id FROM test_suite_release_items item
			JOIN test_case_requirement_links link ON link.test_case_id=item.test_case_id
			WHERE item.release_id=$1) selected ON selected.requirement_id=r.id
		LEFT JOIN requirement_evidence e ON e.requirement_id=r.id
		LEFT JOIN document_versions v ON v.id=e.document_version_id LEFT JOIN documents d ON d.id=v.document_id
		WHERE r.document_set_id=$2
		GROUP BY r.id ORDER BY r.requirement_key,r.id`, releaseID, setID)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var item requirementSnapshot
		if err := rows.Scan(&item.ID, &item.Key, &item.Title, &item.Statement, &item.Evidence); err != nil {
			rows.Close()
			return false, err
		}
		requirements = append(requirements, item)
		requirementByID[item.ID] = item
	}
	rows.Close()
	cases := []caseSnapshot{}
	rows, err = r.pool.Query(ctx, `SELECT t.id,t.test_case_key,t.title,t.test_type,t.risk,t.actor,t.precondition,t.test_data,t.expected_result,t.expected_result_hash,
		COALESCE(array_agg(l.requirement_id ORDER BY l.requirement_id) FILTER(WHERE l.requirement_id IS NOT NULL),'{}')
		FROM test_suite_release_items release_item JOIN test_cases t ON t.id=release_item.test_case_id
		LEFT JOIN test_case_requirement_links l ON l.test_case_id=t.id
		WHERE release_item.release_id=$1
		GROUP BY t.id,release_item.ordinal ORDER BY release_item.ordinal`, releaseID)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var item caseSnapshot
		if err := rows.Scan(&item.ID, &item.Key, &item.Title, &item.Type, &item.Risk, &item.Actor, &item.Preconditions, &item.TestData, &item.ExpectedResult, &item.ExpectedResultHash, &item.RequirementIDs); err != nil {
			rows.Close()
			return false, err
		}
		cases = append(cases, item)
	}
	rows.Close()
	if len(cases) == 0 {
		return false, ErrNotFound
	}
	versions := []map[string]any{}
	rows, err = r.pool.Query(ctx, `SELECT item.document_version_id,item.document_name,
		item.version_number,item.sha256,item.approval_status
		FROM test_suite_releases release
		JOIN document_source_snapshot_items item ON item.source_snapshot_id=release.source_snapshot_id
		WHERE release.id=$1 AND item.included=TRUE ORDER BY item.document_name,item.document_id`, releaseID)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var id int64
		var name, hash, approval string
		var version int
		if err := rows.Scan(&id, &name, &version, &hash, &approval); err != nil {
			rows.Close()
			return false, err
		}
		versions = append(versions, map[string]any{"id": id, "name": name,
			"version": version, "sha256": hash, "approval_status": approval})
	}
	rows.Close()

	identifiers := extractIdentifiers(string(analysis.RawEvent))
	matched := map[int64]bool{}
	for _, id := range identifiers {
		for requirementID, requirement := range requirementByID {
			if normalizedID(requirement.Key) == normalizedID(id) || strings.HasPrefix(normalizedID(requirement.Key), normalizedID(id)+"-") {
				matched[requirementID] = true
			}
		}
	}
	mode, confidence, warning := ModeFullApproved, 1.0, ""
	selected := map[int64]bool{}
	if configuredMode == ModeMappedFallback && len(matched) > 0 {
		mode = ModeExplicitTrace
		for _, item := range cases {
			for _, id := range item.RequirementIDs {
				if matched[id] {
					selected[item.ID] = true
				}
			}
		}
	}
	if configuredMode == ModeMappedFallback && len(selected) == 0 {
		mode = ModeFullFallback
		confidence = .35
		warning = "PR/MR không có liên kết requirement rõ ràng; hệ thống giữ toàn bộ approved suite để tránh bỏ sót."
	}
	if mode != ModeExplicitTrace {
		for _, item := range cases {
			selected[item.ID] = true
		}
	}
	versionJSON, _ := json.Marshal(versions)
	requirementJSON, _ := json.Marshal(requirements)
	caseJSON, _ := json.Marshal(cases)
	baselineHash := hashJSON(map[string]any{"suite_release_id": releaseID,
		"document_versions": versions, "requirements": requirements, "test_cases": cases})
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var snapshotID int64
	err = tx.QueryRow(ctx, `INSERT INTO analysis_baseline_snapshots
		(analysis_job_id,project_id,document_set_id,test_suite_id,suite_release_id,
		 baseline_hash,document_versions,requirements,test_cases,explicit_identifiers,
		 selection_mode,mapping_confidence,warning)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		analysis.ID, analysis.ProjectID, setID, suiteID, releaseID, baselineHash,
		versionJSON, requirementJSON, caseJSON, identifiers, mode, confidence, warning).
		Scan(&snapshotID)
	if err != nil {
		return false, fmt.Errorf("save analysis baseline snapshot: %w", err)
	}
	var runID int64
	err = tx.QueryRow(ctx, `INSERT INTO test_runs(test_suite_id,suite_release_id,project_id,
		analysis_job_id,source_sha,target_sha,environment,status)
		VALUES($1,$2,$3,$4,$5,'','pending','PENDING') RETURNING id`, suiteID,
		releaseID, analysis.ProjectID, analysis.ID, analysis.SourceSHA).Scan(&runID)
	if err != nil {
		return false, fmt.Errorf("create scoped test run: %w", err)
	}
	for _, item := range cases {
		included := selected[item.ID]
		reason := mode
		explanation := "Approved baseline policy selected this test case."
		if mode == ModeExplicitTrace {
			explanation = "Requirement identifier from PR/MR explicitly traces to this test case."
		}
		if mode == ModeFullFallback {
			explanation = "Included by safe full-suite fallback because explicit mapping was absent or uncertain."
		}
		_, err = tx.Exec(ctx, `INSERT INTO analysis_test_scope_items(baseline_snapshot_id,analysis_job_id,test_case_id,included,selection_reason,confidence,explanation) VALUES($1,$2,$3,$4,$5,$6,$7)`, snapshotID, analysis.ID, item.ID, included, reason, confidence, explanation)
		if err != nil {
			return false, err
		}
		if included {
			_, err = tx.Exec(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,expected_result_snapshot,expected_result_hash) VALUES($1,$2,$3,$4)`, runID, item.ID, item.ExpectedResult, item.ExpectedResultHash)
			if err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) Get(ctx context.Context, analysisID int64) (Bundle, error) {
	result := Bundle{AnalysisJobID: analysisID, ExplicitIdentifiers: []string{}, Items: []Item{}, Decisions: []Decision{}, Signals: []Signal{}}
	err := r.pool.QueryRow(ctx, `SELECT s.project_id,s.document_set_id,s.test_suite_id,
		s.suite_release_id,r.id,s.baseline_hash,s.explicit_identifiers,s.selection_mode,
		s.mapping_confidence,s.warning,s.created_at
		FROM analysis_baseline_snapshots s JOIN test_runs r ON r.analysis_job_id=s.analysis_job_id
		WHERE s.analysis_job_id=$1`, analysisID).Scan(&result.ProjectID, &result.DocumentSetID,
		&result.TestSuiteID, &result.SuiteReleaseID, &result.TestRunID, &result.BaselineHash,
		&result.ExplicitIdentifiers, &result.SelectionMode, &result.MappingConfidence,
		&result.Warning, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Bundle{}, ErrNotFound
	}
	if err != nil {
		return Bundle{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT i.id,t.id,t.test_case_key,t.title,t.test_type,t.risk,t.expected_result,t.expected_result_hash,t.automation_status,i.included,i.selection_reason,i.confidence,i.explanation,i.updated_at
		FROM analysis_test_scope_items i JOIN test_cases t ON t.id=i.test_case_id WHERE i.analysis_job_id=$1 ORDER BY t.test_case_key`, analysisID)
	if err != nil {
		return Bundle{}, err
	}
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.TestCaseID, &item.TestCaseKey, &item.Title, &item.TestType, &item.Risk, &item.ExpectedResult, &item.ExpectedResultHash, &item.AutomationStatus, &item.Included, &item.SelectionReason, &item.Confidence, &item.Explanation, &item.UpdatedAt); err != nil {
			rows.Close()
			return Bundle{}, err
		}
		result.Items = append(result.Items, item)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT id,signal_type,signal_value,confidence,explanation,created_at FROM analysis_scope_signals WHERE analysis_job_id=$1 ORDER BY signal_type,signal_value`, analysisID)
	if err != nil {
		return Bundle{}, err
	}
	for rows.Next() {
		var item Signal
		if err := rows.Scan(&item.ID, &item.SignalType, &item.SignalValue, &item.Confidence, &item.Explanation, &item.CreatedAt); err != nil {
			rows.Close()
			return Bundle{}, err
		}
		result.Signals = append(result.Signals, item)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT d.id,d.scope_item_id,d.reviewer_name,d.included,d.comment,d.created_at FROM analysis_scope_decisions d WHERE d.analysis_job_id=$1 ORDER BY d.created_at,d.id`, analysisID)
	if err != nil {
		return Bundle{}, err
	}
	for rows.Next() {
		var item Decision
		if err := rows.Scan(&item.ID, &item.ScopeItemID, &item.ReviewerName, &item.Included, &item.Comment, &item.CreatedAt); err != nil {
			rows.Close()
			return Bundle{}, err
		}
		result.Decisions = append(result.Decisions, item)
	}
	rows.Close()
	return result, nil
}

func (r *Repository) Decide(ctx context.Context, analysisID int64, input ManualInput) (Bundle, error) {
	input.ReviewerName = strings.TrimSpace(input.ReviewerName)
	input.Comment = strings.TrimSpace(input.Comment)
	if analysisID <= 0 || input.TestCaseID <= 0 || input.ReviewerName == "" || input.Comment == "" {
		return Bundle{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Bundle{}, err
	}
	defer tx.Rollback(ctx)
	var itemID, runID int64
	var current bool
	var expected, expectedHash string
	err = tx.QueryRow(ctx, `SELECT i.id,i.included,r.id,t.expected_result,t.expected_result_hash FROM analysis_test_scope_items i
		JOIN test_runs r ON r.analysis_job_id=i.analysis_job_id JOIN test_cases t ON t.id=i.test_case_id
		WHERE i.analysis_job_id=$1 AND i.test_case_id=$2 AND t.status='APPROVED' FOR UPDATE`, analysisID, input.TestCaseID).Scan(&itemID, &current, &runID, &expected, &expectedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Bundle{}, ErrNotFound
	}
	if err != nil {
		return Bundle{}, err
	}
	if current != input.Included {
		if input.Included {
			_, err = tx.Exec(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,expected_result_snapshot,expected_result_hash) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, runID, input.TestCaseID, expected, expectedHash)
		} else {
			var changed int64
			tag, deleteErr := tx.Exec(ctx, `DELETE FROM test_run_items WHERE test_run_id=$1 AND test_case_id=$2 AND status='NOT_RUN'`, runID, input.TestCaseID)
			if deleteErr != nil {
				return Bundle{}, deleteErr
			}
			changed = tag.RowsAffected()
			if changed == 0 {
				return Bundle{}, ErrUnsafeChange
			}
		}
	}
	_, err = tx.Exec(ctx, `UPDATE analysis_test_scope_items SET included=$3,selection_reason='MANUAL',confidence=1,explanation=$4,updated_at=NOW() WHERE id=$1 AND analysis_job_id=$2`, itemID, analysisID, input.Included, "Reviewer override: "+input.Comment)
	if err != nil {
		return Bundle{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO analysis_scope_decisions(scope_item_id,analysis_job_id,reviewer_name,included,comment) VALUES($1,$2,$3,$4,$5)`, itemID, analysisID, input.ReviewerName, input.Included, input.Comment)
	if err != nil {
		return Bundle{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Bundle{}, err
	}
	return r.Get(ctx, analysisID)
}

func extractIdentifiers(raw string) []string {
	values := explicitIDPattern.FindAllString(raw, -1)
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", "-"))
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
func normalizedID(value string) string {
	replacer := strings.NewReplacer("_", "-", " ", "-")
	return strings.ToUpper(replacer.Replace(strings.TrimSpace(value)))
}
func hashJSON(value any) string {
	content, _ := json.Marshal(value)
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
