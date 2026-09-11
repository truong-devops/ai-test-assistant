//go:build integration

package document

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRepositoryDocumentLifecycleAndImmutability(t *testing.T) {
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

	repository := NewRepository(pool)
	files, err := NewLocalFileStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repository, files)
	set, err := service.CreateSet(ctx, CreateSetInput{
		Name:        "document-integration-" + time.Now().Format("20060102150405.000000000"),
		Description: "Document-driven test source",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, set.ID) }()

	item, version, err := service.Upload(ctx, set.ID, UploadInput{
		DocumentName: "Order requirements", DocumentType: TypeRequirements,
		Filename: "order.md",
	}, strings.NewReader("# Đặt hàng\n\nĐơn hợp lệ phải được tạo."))
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == 0 || version.VersionNumber != 1 || version.ApprovalStatus != ApprovalDraft ||
		version.ParseStatus != ParseUploaded {
		t.Fatalf("document=%+v version=%+v", item, version)
	}
	if _, err := pool.Exec(ctx, `UPDATE document_versions SET parse_status='PARSING',
		attempt_count=1, lease_expires_at=NOW()+INTERVAL '1 minute' WHERE id=$1`, version.ID); err != nil {
		t.Fatal(err)
	}
	version.ParseStatus, version.AttemptCount = ParseParsing, 1
	processor := NewProcessor(files, NewStructuredParser(), repository)
	if err := processor.Process(ctx, version); err != nil {
		t.Fatal(err)
	}
	loadedDocument, loadedVersion, blocks, err := service.GetVersion(ctx, item.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if loadedDocument.ID != item.ID || loadedVersion.ParseStatus != ParseParsed ||
		loadedVersion.BlockCount != 2 || len(blocks) != 2 || blocks[0].SourceLocator != "line:1" {
		t.Fatalf("document=%+v version=%+v blocks=%+v", loadedDocument, loadedVersion, blocks)
	}
	var requirementID, evidenceID int64
	if err := pool.QueryRow(ctx, `INSERT INTO requirements
		(document_set_id,requirement_key,title,statement,requirement_type)
		VALUES($1,'REQ-001','Create order','A valid order is created','FUNCTIONAL') RETURNING id`,
		set.ID).Scan(&requirementID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO requirement_evidence
		(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, requirementID, set.ID, version.ID,
		blocks[1].ID, blocks[1].SourceLocator, strings.Repeat("b", 64)).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE requirement_evidence SET source_locator='changed' WHERE id=$1`, evidenceID); err == nil {
		t.Fatal("requirement evidence update succeeded, want immutable trigger error")
	}

	otherSet, err := service.CreateSet(ctx, CreateSetInput{Name: "other-" + time.Now().Format("20060102150405.000000000")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, otherSet.ID) }()
	_, otherVersion, err := service.Upload(ctx, otherSet.ID, UploadInput{
		DocumentName: "Foreign", DocumentType: TypeRequirements, Filename: "foreign.md",
	}, strings.NewReader("# Foreign\n\nNot part of the first set."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE document_versions SET parse_status='PARSING',
		attempt_count=1, lease_expires_at=NOW()+INTERVAL '1 minute' WHERE id=$1`, otherVersion.ID); err != nil {
		t.Fatal(err)
	}
	otherVersion.ParseStatus, otherVersion.AttemptCount = ParseParsing, 1
	if err := processor.Process(ctx, otherVersion); err != nil {
		t.Fatal(err)
	}
	_, _, otherBlocks, err := service.GetVersion(ctx, otherVersion.DocumentID, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO requirement_evidence
		(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
		VALUES($1,$2,$3,$4,'foreign',$5)`, requirementID, set.ID, otherVersion.ID,
		otherBlocks[0].ID, strings.Repeat("c", 64))
	if err == nil {
		t.Fatal("cross-document-set evidence insert succeeded, want ownership violation")
	}

	_, secondVersion, err := service.Upload(ctx, set.ID, UploadInput{
		DocumentName: "Order requirements", DocumentType: TypeRequirements,
		Filename: "order-v2.md",
	}, strings.NewReader("# Đặt hàng\n\nBổ sung trường hợp hết hàng."))
	if err != nil {
		t.Fatal(err)
	}
	if secondVersion.VersionNumber != 2 || secondVersion.DocumentID != item.ID {
		t.Fatalf("second version=%+v", secondVersion)
	}
	if _, err := pool.Exec(ctx, `UPDATE document_versions SET sha256=$2 WHERE id=$1`,
		version.ID, strings.Repeat("f", 64)); err == nil {
		t.Fatal("document version identity update succeeded, want immutable trigger error")
	}
}

func TestDocumentSchemaRejectsExpectedResultMutationAfterRun(t *testing.T) {
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

	var setID, suiteID, caseID, artifactID, runID, itemID int64
	name := "snapshot-integration-" + time.Now().Format("20060102150405.000000000")
	if err := pool.QueryRow(ctx, `INSERT INTO document_sets(name) VALUES($1) RETURNING id`, name).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM test_runs WHERE id=$1`, runID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID)
	}()
	if err := pool.QueryRow(ctx, `INSERT INTO test_suites(document_set_id,name) VALUES($1,'Order') RETURNING id`, setID).Scan(&suiteID); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	if err := pool.QueryRow(ctx, `INSERT INTO test_cases
		(test_suite_id,document_set_id,test_case_key,title,test_type,expected_result,expected_result_hash)
		VALUES($1,$2,'TC-001','Successful order','HAPPY','Order is created',$3) RETURNING id`,
		suiteID, setID, hash).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	artifactHash := strings.Repeat("d", 64)
	_, err = pool.Exec(ctx, `INSERT INTO automation_artifacts
		(test_case_id,version_number,framework,file_path,source,source_hash,expected_result_hash)
		VALUES($1,1,'go-test','order_test.go','package order',$2,$3)`, caseID, artifactHash, hash)
	if err == nil {
		t.Fatal("artifact for a draft test case succeeded, want approval policy error")
	}
	if _, err := pool.Exec(ctx, `UPDATE test_cases SET status='APPROVED' WHERE id=$1`, caseID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO automation_artifacts
		(test_case_id,version_number,framework,file_path,source,source_hash,expected_result_hash)
		VALUES($1,1,'go-test','order_test.go','package order',$2,$3)`, caseID, artifactHash, strings.Repeat("c", 64))
	if err == nil {
		t.Fatal("artifact with a changed expected-result hash succeeded")
	}
	if err := pool.QueryRow(ctx, `INSERT INTO automation_artifacts
		(test_case_id,version_number,framework,file_path,source,source_hash,expected_result_hash,status)
		VALUES($1,1,'go-test','order_test.go','package order',$2,$3,'APPROVED') RETURNING id`,
		caseID, artifactHash, hash).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_runs(test_suite_id) VALUES($1) RETURNING id`, suiteID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_run_items
		(test_run_id,test_case_id,automation_artifact_id,expected_result_snapshot,
		 expected_result_hash,automation_source_hash)
		VALUES($1,$2,$3,'Order is created',$4,$5) RETURNING id`, runID, caseID,
		artifactID, hash, artifactHash).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE test_run_items SET expected_result_snapshot='changed' WHERE id=$1`, itemID)
	if err == nil {
		t.Fatal("expected result snapshot update succeeded, want immutable trigger error")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected timeout: %v", err)
	}
}
