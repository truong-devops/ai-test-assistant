package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) BuildSnapshot(ctx context.Context, setID int64, input ExportInput, now time.Time) (Snapshot, error) {
	result := Snapshot{
		DocumentSetID: setID, TestSuiteID: input.TestSuiteID, TestRunID: input.TestRunID,
		GeneratedAt: now.UTC(), GeneratedBy: input.GeneratedBy,
		DocumentVersions: []DocumentVersionRef{}, Rows: []ExecutionRow{}, RunHistory: []RunHistoryRow{},
		TestCaseFilter: input.TestCaseIDs, SortBy: input.SortBy,
	}
	if err := r.pool.QueryRow(ctx, `SELECT d.name,d.product_name,d.scope,s.name
		FROM document_sets d JOIN test_suites s ON s.document_set_id=d.id
		WHERE d.id=$1 AND s.id=$2`, setID, input.TestSuiteID).Scan(
		&result.DocumentSetName, &result.ProductName, &result.Scope, &result.TestSuiteName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, fmt.Errorf("load export scope: %w", err)
	}
	if input.TestRunID != nil {
		var valid bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM test_runs WHERE id=$1 AND test_suite_id=$2)`,
			*input.TestRunID, input.TestSuiteID).Scan(&valid); err != nil || !valid {
			if err != nil {
				return Snapshot{}, fmt.Errorf("validate test run: %w", err)
			}
			return Snapshot{}, ErrNotFound
		}
	}
	versionRows, err := r.pool.Query(ctx, `SELECT d.name,v.version_number,v.sha256,v.approval_status
		FROM documents d JOIN document_versions v ON v.document_id=d.id
		WHERE d.document_set_id=$1 AND NOT EXISTS(
			SELECT 1 FROM document_versions newer WHERE newer.document_id=v.document_id
			AND newer.version_number>v.version_number)
		ORDER BY d.name`, setID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list export document versions: %w", err)
	}
	for versionRows.Next() {
		var item DocumentVersionRef
		if err := versionRows.Scan(&item.DocumentName, &item.VersionNumber, &item.SHA256, &item.ApprovalStatus); err != nil {
			versionRows.Close()
			return Snapshot{}, fmt.Errorf("scan export document version: %w", err)
		}
		result.DocumentVersions = append(result.DocumentVersions, item)
	}
	versionRows.Close()

	rows, err := r.pool.Query(ctx, `SELECT t.id,t.test_case_key,
		COALESCE((SELECT string_agg(r.requirement_key, ', ' ORDER BY r.requirement_key)
			FROM test_case_requirement_links l JOIN requirements r ON r.id=l.requirement_id WHERE l.test_case_id=t.id),''),
		t.title,t.precondition,
		COALESCE((SELECT string_agg(s.ordinal::text || '. ' || s.action ||
			CASE WHEN s.expected_result='' THEN '' ELSE E'\n   Expected: ' || s.expected_result END,
			E'\n' ORDER BY s.ordinal) FROM test_case_steps s WHERE s.test_case_id=t.id),''),
		t.test_data,t.actor,t.expected_result,t.risk,
		COALESCE(run.environment,''),COALESCE(item.actual_result,''),COALESCE(item.status,'NOT_RUN'),
		COALESCE((SELECT string_agg(d.name || ' v' || v.version_number || ' — ' || e.source_locator,
			E'\n' ORDER BY d.name,v.version_number,e.source_locator)
			FROM test_case_requirement_links l JOIN requirement_evidence e ON e.requirement_id=l.requirement_id
			JOIN document_versions v ON v.id=e.document_version_id JOIN documents d ON d.id=v.document_id
			WHERE l.test_case_id=t.id),''),
		COALESCE((SELECT string_agg(value,E'\n') FROM jsonb_array_elements_text(t.assumptions) value),''),t.automation_status,t.status,
		COALESCE(review.reviewer_name,'')
		FROM test_cases t
		LEFT JOIN test_run_items item ON item.test_case_id=t.id AND item.test_run_id=$2
		LEFT JOIN test_runs run ON run.id=item.test_run_id
		LEFT JOIN test_case_reviews review ON review.test_case_id=t.id
		WHERE t.test_suite_id=$1 AND (cardinality($3::bigint[])=0 OR t.id=ANY($3)) AND NOT EXISTS(
			SELECT 1 FROM test_cases newer WHERE newer.supersedes_test_case_id=t.id)
		ORDER BY t.test_case_key,t.version_number DESC`, input.TestSuiteID, input.TestRunID, input.TestCaseIDs)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list export test cases: %w", err)
	}
	reviewers := map[string]struct{}{}
	for rows.Next() {
		var item ExecutionRow
		var steps, evidence, reviewer string
		if err := rows.Scan(&item.TestCaseID, &item.TestCaseKey, &item.RequirementTrace,
			&item.Objective, &item.Preconditions, &steps, &item.TestData, &item.Role,
			&item.ExpectedResult, &item.Priority, &item.Environment, &item.ActualResult,
			&item.Status, &evidence, &item.Notes, &item.AutomationStatus,
			&item.SourceStatus, &reviewer); err != nil {
			rows.Close()
			return Snapshot{}, fmt.Errorf("scan export test case: %w", err)
		}
		item.Steps = splitLines(steps)
		item.Evidence = splitLines(evidence)
		item.Status = managementStatus(item.Status)
		result.Rows = append(result.Rows, item)
		if reviewer != "" {
			reviewers[reviewer] = struct{}{}
		}
	}
	rows.Close()
	if len(input.TestCaseIDs) > 0 && len(result.Rows) != len(input.TestCaseIDs) {
		return Snapshot{}, ErrNotFound
	}
	switch input.SortBy {
	case "TITLE":
		sort.SliceStable(result.Rows, func(i, j int) bool { return result.Rows[i].Objective < result.Rows[j].Objective })
	case "RISK":
		sort.SliceStable(result.Rows, func(i, j int) bool { return riskRank(result.Rows[i].Priority) > riskRank(result.Rows[j].Priority) })
	}
	result.Reviewer = joinKeys(reviewers)

	historyRows, err := r.pool.Query(ctx, `SELECT t.test_case_key,r.id,i.attempt_number,r.source_sha,
		r.environment,i.actual_result,i.status,
		COALESCE((SELECT string_agg(e.evidence_type || ': ' || COALESCE(NULLIF(e.content,''),e.storage_key),E'\n'
			ORDER BY e.id) FROM test_run_evidence e WHERE e.test_run_item_id=i.id),''),i.created_at
		FROM test_run_items i JOIN test_runs r ON r.id=i.test_run_id JOIN test_cases t ON t.id=i.test_case_id
		WHERE r.test_suite_id=$1 ORDER BY i.created_at,r.id,i.attempt_number`, input.TestSuiteID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list run history: %w", err)
	}
	for historyRows.Next() {
		var item RunHistoryRow
		var evidence string
		if err := historyRows.Scan(&item.TestCaseKey, &item.RunID, &item.Attempt, &item.SourceSHA,
			&item.Environment, &item.ActualResult, &item.Status, &evidence, &item.CreatedAt); err != nil {
			historyRows.Close()
			return Snapshot{}, fmt.Errorf("scan run history: %w", err)
		}
		item.Evidence = splitLines(evidence)
		result.RunHistory = append(result.RunHistory, item)
	}
	historyRows.Close()
	return result, nil
}

func riskRank(value string) int {
	switch value {
	case "HIGH":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	default:
		return 0
	}
}

func managementStatus(value string) string {
	switch value {
	case "PASSED":
		return "P"
	case "PRODUCT_FAILED":
		return "F"
	case "NOT_RUN":
		return "NY"
	case "BLOCKED", "AUTOMATION_ERROR", "INFRA_ERROR", "TIMED_OUT":
		return "BLOCKED"
	default:
		return value
	}
}

func (r *Repository) Save(ctx context.Context, artifact ExportArtifact, snapshot Snapshot) (ExportArtifact, error) {
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return ExportArtifact{}, fmt.Errorf("encode export snapshot: %w", err)
	}
	const query = `INSERT INTO test_exports(document_set_id,test_suite_id,test_run_id,format,filename,
		content_type,content,content_hash,snapshot,snapshot_hash,row_count,generated_by,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id,created_at`
	if err := r.pool.QueryRow(ctx, query, artifact.DocumentSetID, artifact.TestSuiteID,
		artifact.TestRunID, artifact.Format, artifact.Filename, artifact.ContentType,
		artifact.Content, artifact.ContentHash, snapshotJSON, artifact.SnapshotHash,
		artifact.RowCount, artifact.GeneratedBy, snapshot.GeneratedAt).Scan(&artifact.ID, &artifact.CreatedAt); err != nil {
		return ExportArtifact{}, fmt.Errorf("save export: %w", err)
	}
	artifact.DownloadURL = fmt.Sprintf("/api/test-exports/%d/download", artifact.ID)
	return artifact, nil
}

func (r *Repository) List(ctx context.Context, setID int64) ([]ExportArtifact, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,document_set_id,test_suite_id,test_run_id,format,filename,
		content_type,content_hash,snapshot_hash,row_count,generated_by,created_at
		FROM test_exports WHERE document_set_id=$1 ORDER BY created_at DESC,id DESC`, setID)
	if err != nil {
		return nil, fmt.Errorf("list exports: %w", err)
	}
	defer rows.Close()
	result := []ExportArtifact{}
	for rows.Next() {
		var item ExportArtifact
		if err := rows.Scan(&item.ID, &item.DocumentSetID, &item.TestSuiteID, &item.TestRunID,
			&item.Format, &item.Filename, &item.ContentType, &item.ContentHash,
			&item.SnapshotHash, &item.RowCount, &item.GeneratedBy, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan export: %w", err)
		}
		item.DownloadURL = fmt.Sprintf("/api/test-exports/%d/download", item.ID)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id int64) (ExportArtifact, error) {
	var item ExportArtifact
	if err := r.pool.QueryRow(ctx, `SELECT id,document_set_id,test_suite_id,test_run_id,format,filename,
		content_type,content,content_hash,snapshot_hash,row_count,generated_by,created_at
		FROM test_exports WHERE id=$1`, id).Scan(&item.ID, &item.DocumentSetID,
		&item.TestSuiteID, &item.TestRunID, &item.Format, &item.Filename, &item.ContentType,
		&item.Content, &item.ContentHash, &item.SnapshotHash, &item.RowCount,
		&item.GeneratedBy, &item.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ExportArtifact{}, ErrNotFound
		}
		return ExportArtifact{}, fmt.Errorf("get export: %w", err)
	}
	item.DownloadURL = fmt.Sprintf("/api/test-exports/%d/download", item.ID)
	return item, nil
}

func splitLines(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	return strings.Split(value, "\n")
}

func joinKeys(values map[string]struct{}) string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
