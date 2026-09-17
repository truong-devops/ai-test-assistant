//go:build integration

package report

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This test records VER-06 at the UV-00 baseline. UV-03 must build a run export
// from the revision pinned in the run item, not the latest leaf in the suite.
func TestUV00CharacterizationRunExportSelectsDraftSuccessorInsteadOfExecutedRevision(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var setID, suiteID, executedID, successorID, runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO document_sets(name) VALUES($1) RETURNING id`,
		"uv00-report-"+time.Now().Format("20060102150405.000000000")).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM test_runs WHERE id=$1`, runID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID)
	}()
	if err := pool.QueryRow(ctx, `INSERT INTO test_suites(document_set_id,name)
		VALUES($1,'UV-00 export suite') RETURNING id`, setID).Scan(&suiteID); err != nil {
		t.Fatal(err)
	}
	expectedHash := strings.Repeat("b", 64)
	if err := pool.QueryRow(ctx, `INSERT INTO test_cases
		(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,
		expected_result,expected_result_hash,status)
		VALUES($1,$2,'TC-UV00',1,'Đặt hàng v1','HAPPY','Đơn v1 được tạo',$3,'APPROVED') RETURNING id`,
		suiteID, setID, expectedHash).Scan(&executedID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_runs(test_suite_id,status,environment)
		VALUES($1,'COMPLETED','uv00') RETURNING id`, suiteID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO test_run_items
		(test_run_id,test_case_id,status,expected_result_snapshot,expected_result_hash,actual_result)
		VALUES($1,$2,'PASSED','Đơn v1 được tạo',$3,'Đơn v1 được tạo')`, runID, executedID, expectedHash); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_cases
		(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,
		expected_result,expected_result_hash,status,supersedes_test_case_id)
		VALUES($1,$2,'TC-UV00',2,'Đặt hàng v2 draft','HAPPY','Đơn v2 được tạo',$3,'DRAFT',$4) RETURNING id`,
		suiteID, setID, strings.Repeat("c", 64), executedID).Scan(&successorID); err != nil {
		t.Fatal(err)
	}

	snapshot, err := NewRepository(pool).BuildSnapshot(ctx, setID, ExportInput{
		TestSuiteID: suiteID, TestRunID: &runID, TestCaseIDs: []int64{}, GeneratedBy: "uv00",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Rows) != 1 || snapshot.Rows[0].TestCaseID != successorID ||
		snapshot.Rows[0].Status != "NY" || snapshot.Rows[0].ActualResult != "" {
		t.Fatalf("baseline changed: run export no longer substitutes the draft leaf; rows=%+v", snapshot.Rows)
	}
	if snapshot.Rows[0].TestCaseID == executedID {
		t.Fatalf("characterization invalid: export already selected the executed revision: %+v", snapshot.Rows[0])
	}
}
