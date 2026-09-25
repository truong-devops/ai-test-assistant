// Package evidence verifies persisted release/run/export relationships read-only.
// It does not certify that a provider or sandbox was actually contacted.
package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/report"
)

type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Count  int64  `json:"count"`
	Detail string `json:"detail"`
}

type manifestItem struct {
	FamilyID           int64  `json:"family_id"`
	TestCaseID         int64  `json:"test_case_id"`
	ContentHash        string `json:"content_hash"`
	ExpectedResultHash string `json:"expected_result_hash"`
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func VerifyPinned(ctx context.Context, pool *pgxpool.Pool, setID, projectID, analysisID, runID int64) ([]Check, error) {
	checks := []Check{}
	add := func(name string, ok bool, count int64, detail string) {
		checks = append(checks, Check{name, ok, count, detail})
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return checks, err
	}
	defer tx.Rollback(ctx)
	var releaseID, suiteID, snapshotID int64
	var manifest, origin string
	err = tx.QueryRow(ctx, `SELECT rel.id,rel.test_suite_id,COALESCE(rel.source_snapshot_id,0),rel.manifest_hash,rel.origin
 FROM analysis_baseline_snapshots s JOIN test_runs r ON r.analysis_job_id=s.analysis_job_id
 JOIN test_suite_releases rel ON rel.id=s.suite_release_id
 WHERE s.document_set_id=$1 AND s.project_id=$2 AND s.analysis_job_id=$3 AND r.id=$4
 AND r.project_id=s.project_id AND r.test_suite_id=s.test_suite_id AND r.suite_release_id=s.suite_release_id
 AND rel.document_set_id=s.document_set_id AND rel.test_suite_id=s.test_suite_id`, setID, projectID, analysisID, runID).Scan(&releaseID, &suiteID, &snapshotID, &manifest, &origin)
	if errors.Is(err, pgx.ErrNoRows) {
		add("pinned release chain", false, 0, "Analysis/run must reference the same release, suite, set and project; legacy unpinned proof is not sufficient.")
		return checks, nil
	}
	if err != nil {
		return checks, err
	}
	add("pinned release chain", true, 1, fmt.Sprintf("Analysis and run pin release #%d independently of the project's current binding.", releaseID))
	rows, err := tx.Query(ctx, `SELECT i.family_id,i.test_case_id,i.content_hash,i.expected_result_hash,
 i.content_hash=t.content_hash AND i.expected_result_hash=t.expected_result_hash
 AND i.revision_number=t.version_number AND i.public_key=t.test_case_key AND t.sealed_at IS NOT NULL
 FROM test_suite_release_items i JOIN test_cases t ON t.id=i.test_case_id AND t.family_id=i.family_id
 AND t.document_set_id=i.document_set_id AND t.test_suite_id=i.test_suite_id WHERE i.release_id=$1 ORDER BY i.ordinal`, releaseID)
	if err != nil {
		return checks, err
	}
	items := []manifestItem{}
	valid := true
	for rows.Next() {
		var item manifestItem
		var ok bool
		if err = rows.Scan(&item.FamilyID, &item.TestCaseID, &item.ContentHash, &item.ExpectedResultHash, &ok); err != nil {
			rows.Close()
			return checks, err
		}
		items = append(items, item)
		valid = valid && ok
	}
	rows.Close()
	if rows.Err() != nil {
		return checks, rows.Err()
	}
	add("release revision hashes", valid && len(items) > 0, int64(len(items)), "Every pinned item matches its sealed exact revision/hash; the live family head is irrelevant.")
	payload, _ := json.Marshal(struct {
		SourceSnapshotID int64          `json:"source_snapshot_id"`
		Items            []manifestItem `json:"items"`
	}{snapshotID, items})
	computed := digest(payload)
	if origin == "MIGRATED_CURRENT_STATE" {
		// Migration 25 used PostgreSQL JSONB text rather than the publisher's Go encoding.
		err = tx.QueryRow(ctx, `SELECT encode(digest(convert_to(jsonb_agg(jsonb_build_object('family_id',family_id,'test_case_id',test_case_id,'content_hash',content_hash,'expected_result_hash',expected_result_hash) ORDER BY ordinal)::text,'UTF8'),'sha256'),'hex') FROM test_suite_release_items WHERE release_id=$1`, releaseID).Scan(&computed)
		if err != nil {
			return checks, err
		}
	}
	add("release manifest checksum", computed == manifest, 1, "Recomputed ordered manifest using the release origin's encoding.")
	var scopeOK, runOK bool
	err = tx.QueryRow(ctx, `SELECT jsonb_array_length(s.test_cases)=(SELECT count(*) FROM test_suite_release_items WHERE release_id=$2)
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(s.test_cases) c LEFT JOIN test_suite_release_items i ON i.release_id=$2 AND i.test_case_id=(c->>'id')::bigint
 WHERE i.test_case_id IS NULL OR i.expected_result_hash IS DISTINCT FROM c->>'expected_result_hash')
 AND (SELECT count(DISTINCT (c->>'id')::bigint) FROM jsonb_array_elements(s.test_cases) c)=jsonb_array_length(s.test_cases)
 AND NOT EXISTS(SELECT 1 FROM analysis_test_scope_items x LEFT JOIN test_suite_release_items i ON i.release_id=$2 AND i.test_case_id=x.test_case_id WHERE x.analysis_job_id=$1 AND i.test_case_id IS NULL)
 FROM analysis_baseline_snapshots s WHERE s.analysis_job_id=$1`, analysisID, releaseID).Scan(&scopeOK)
	if err != nil {
		return checks, err
	}
	add("analysis revision manifest", scopeOK, 1, "Snapshot revision IDs/hashes and scope members belong to this exact release.")
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM test_run_items WHERE test_run_id=$1)
 AND NOT EXISTS(SELECT 1 FROM test_run_items r LEFT JOIN test_suite_release_items i ON i.release_id=$2 AND i.test_case_id=r.test_case_id
 LEFT JOIN test_cases t ON t.id=r.test_case_id LEFT JOIN automation_artifacts a ON a.id=r.automation_artifact_id
 WHERE r.test_run_id=$1 AND (i.test_case_id IS NULL OR r.expected_result_hash<>i.expected_result_hash OR r.expected_result_snapshot<>t.expected_result
 OR encode(digest(convert_to(trim(r.expected_result_snapshot),'UTF8'),'sha256'),'hex')<>r.expected_result_hash
 OR (r.automation_artifact_id IS NOT NULL AND (a.test_case_id<>r.test_case_id OR a.expected_result_hash<>r.expected_result_hash))))
 AND NOT EXISTS(SELECT 1 FROM analysis_test_scope_items x WHERE x.analysis_job_id=$3 AND x.included AND NOT EXISTS(SELECT 1 FROM test_run_items r WHERE r.test_run_id=$1 AND r.test_case_id=x.test_case_id))`, runID, releaseID, analysisID).Scan(&runOK)
	if err != nil {
		return checks, err
	}
	add("run revision proof", runOK, 1, "Run items preserve exact expected content/hash and artifact revision; all included scope items have a run item.")
	rows, err = tx.Query(ctx, `SELECT id,content,content_hash,snapshot,snapshot_hash,row_count,
 row_count=(SELECT count(DISTINCT test_case_id) FROM test_run_items WHERE test_run_id=$2)
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(snapshot->'rows') row WHERE NOT EXISTS(SELECT 1 FROM test_run_items i WHERE i.test_run_id=$2 AND i.test_case_id=(row->>'test_case_id')::bigint))
 FROM test_exports WHERE document_set_id=$1 AND test_run_id=$2 AND suite_release_id=$3 AND format='XLSX'
 AND jsonb_array_length(COALESCE(snapshot->'test_case_filter','[]'::jsonb))=0 ORDER BY id`, setID, runID, releaseID)
	if err != nil {
		return checks, err
	}
	count := int64(0)
	exportsOK := true
	for rows.Next() {
		var id int64
		var content, raw []byte
		var contentHash, snapshotHash string
		var rowCount int
		var complete bool
		if err = rows.Scan(&id, &content, &contentHash, &raw, &snapshotHash, &rowCount, &complete); err != nil {
			rows.Close()
			return checks, err
		}
		count++
		exportsOK = validateExport(content, raw, contentHash, snapshotHash, rowCount, setID, suiteID, runID, releaseID, manifest, items) && exportsOK && complete
	}
	rows.Close()
	if rows.Err() != nil {
		return checks, rows.Err()
	}
	add("XLSX pinned snapshot and bytes", count > 0 && exportsOK, count, "Stored hashes, workbook relationships, data/metadata cells and summary formulas must agree with the pinned snapshot. Requires the generated XLSX format (32 parts, 16 MiB/part, 64 MiB total); not a sandbox execution certificate.")
	return checks, tx.Commit(ctx)
}

func validateExport(content, raw []byte, contentHash, snapshotHash string, rowCount int, setID, suiteID, runID, releaseID int64, manifest string, items []manifestItem) bool {
	var snapshot report.Snapshot
	if json.Unmarshal(raw, &snapshot) != nil {
		return false
	}
	canonical, err := json.Marshal(snapshot)
	if err != nil || digest(content) != contentHash || digest(canonical) != snapshotHash || len(content) < 4 || string(content[:2]) != "PK" || snapshot.DocumentSetID != setID || snapshot.TestSuiteID != suiteID || snapshot.TestRunID == nil || *snapshot.TestRunID != runID || snapshot.SuiteReleaseID == nil || *snapshot.SuiteReleaseID != releaseID || snapshot.ReleaseManifestHash != manifest || rowCount == 0 || rowCount != len(snapshot.Rows) || len(snapshot.RevisionManifest) != rowCount {
		return false
	}
	allowed := map[int64]manifestItem{}
	for _, item := range items {
		allowed[item.TestCaseID] = item
	}
	seen := map[int64]bool{}
	for _, ref := range snapshot.RevisionManifest {
		item, ok := allowed[ref.TestCaseID]
		if !ok || seen[ref.TestCaseID] || ref.FamilyID != item.FamilyID || ref.ContentHash != item.ContentHash {
			return false
		}
		seen[ref.TestCaseID] = true
	}
	for _, row := range snapshot.Rows {
		item, ok := allowed[row.TestCaseID]
		if !ok || !seen[row.TestCaseID] || digest([]byte(row.ExpectedResult)) != item.ExpectedResultHash {
			return false
		}
		delete(seen, row.TestCaseID)
	}
	return len(seen) == 0 && validateWorkbook(content, snapshot)
}
