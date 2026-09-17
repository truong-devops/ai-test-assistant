//go:build integration

package testcase

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This test records VER-04 at the UV-00 baseline. UV-02/03 must make latest,
// latest-approved and pinned-release selection explicit instead of using leaf rows.
func TestUV00CharacterizationDraftSuccessorHidesApprovedRevisionFromList(t *testing.T) {
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

	var setID, suiteID, approvedID, draftID int64
	if err := pool.QueryRow(ctx, `INSERT INTO document_sets(name) VALUES($1) RETURNING id`,
		"uv00-version-list-"+time.Now().Format("20060102150405.000000000")).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID) }()
	if err := pool.QueryRow(ctx, `INSERT INTO test_suites(document_set_id,name)
		VALUES($1,'UV-00 suite') RETURNING id`, setID).Scan(&suiteID); err != nil {
		t.Fatal(err)
	}
	expectedHash := strings.Repeat("a", 64)
	if err := pool.QueryRow(ctx, `INSERT INTO test_cases
		(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,
		expected_result,expected_result_hash,status)
		VALUES($1,$2,'TC-UV00',1,'Đặt hàng hợp lệ','HAPPY','Đơn được tạo',$3,'APPROVED') RETURNING id`,
		suiteID, setID, expectedHash).Scan(&approvedID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_cases
		(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,
		expected_result,expected_result_hash,status,supersedes_test_case_id)
		VALUES($1,$2,'TC-UV00',2,'Đặt hàng hợp lệ với COD','HAPPY','Đơn được tạo',$3,'DRAFT',$4) RETURNING id`,
		suiteID, setID, expectedHash, approvedID).Scan(&draftID); err != nil {
		t.Fatal(err)
	}

	items, err := NewRepository(pool).List(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != draftID || items[0].Status != StatusDraft {
		t.Fatalf("baseline changed: leaf list should expose only draft successor; items=%+v", items)
	}
	for _, item := range items {
		if item.ID == approvedID {
			t.Fatalf("characterization invalid: approved revision unexpectedly remained visible: %+v", items)
		}
	}
}
