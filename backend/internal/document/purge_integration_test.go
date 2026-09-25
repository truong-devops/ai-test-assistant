//go:build integration

package document

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPhysicalPurgeDeletesObjectsAndRetainsAudit(t *testing.T) {
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
	files, err := NewLocalFileStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(NewRepository(pool), files)
	set, err := service.CreateSet(ctx, CreateSetInput{Name: "purge-integration-" + time.Now().Format("20060102150405.000000000")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, set.ID) }()
	_, version, err := service.Upload(ctx, set.ID, UploadInput{DocumentName: "Purge source",
		DocumentType: TypeRequirements, Filename: "purge.md"}, strings.NewReader("# Requirement"))
	if err != nil {
		t.Fatal(err)
	}
	var blockID, requirementID int64
	if err = pool.QueryRow(ctx, `INSERT INTO document_blocks
		(document_version_id,ordinal,block_type,content,source_locator)
		VALUES($1,1,'PARAGRAPH','Order is created','line:1') RETURNING id`, version.ID).
		Scan(&blockID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO requirements
		(document_set_id,requirement_key,title,statement,requirement_type)
		VALUES($1,'REQ-PURGE','Create order','Order is created','FUNCTIONAL') RETURNING id`, set.ID).
		Scan(&requirementID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO requirement_evidence
		(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
		VALUES($1,$2,$3,$4,'line:1',$5)`, requirementID, set.ID, version.ID, blockID,
		strings.Repeat("f", 64)); err != nil {
		t.Fatal(err)
	}
	// UV09: retained proposal receipts and checkpoint/result FKs must not break
	// an otherwise eligible set purge after schema 30/31.
	if _, err = pool.Exec(ctx, `WITH suite AS (
 INSERT INTO test_suites(document_set_id,name) VALUES($1,'Purge proposal suite') RETURNING id
 ), revision AS (
 INSERT INTO test_cases(test_suite_id,document_set_id,test_case_key,title,test_type,expected_result,expected_result_hash)
 SELECT id,$1,'TC-PURGE','Purge draft','HAPPY','Order is created',encode(sha256(convert_to('Order is created','UTF8')),'hex') FROM suite RETURNING id,test_suite_id
 ), job AS (
 INSERT INTO document_workflow_jobs(document_set_id,operation,input_snapshot,input_hash,requested_by,idempotency_key,status)
 VALUES($1,'GENERATE_TESTCASES','{}',repeat('a',64),'fixture','purge-fixture','SUCCEEDED') RETURNING id
 ), unit AS (
 INSERT INTO document_workflow_job_units(workflow_job_id,unit_key,input_hash,status)
 SELECT id,'requirement:fixture',repeat('a',64),'SUCCEEDED' FROM job RETURNING id,workflow_job_id
 ), proposal AS (
 INSERT INTO test_case_generation_proposals(document_set_id,test_suite_id,workflow_job_id,workflow_unit_id,proposal_key,classification,reason,content,content_hash,generation,candidates,source_revision,status,result_revision_id,decision,decision_reason,decided_by,decided_at)
 SELECT $1,revision.test_suite_id,unit.workflow_job_id,unit.id,'fixture','NEW_CASE','purge fixture','{}',repeat('b',64),'{}','[]',1,'APPLIED',revision.id,'CREATE_NEW','purge fixture','fixture',NOW() FROM unit,revision RETURNING id
 ) INSERT INTO test_case_proposal_commands(document_set_id,idempotency_key,proposal_id,request_hash,result,actor)
 SELECT $1,'purge-fixture',id,repeat('c',64),'{}','fixture' FROM proposal`, set.ID); err != nil {
		t.Fatal(err)
	}
	var releaseID int64
	if err = pool.QueryRow(ctx, `INSERT INTO test_suite_releases
 (test_suite_id,document_set_id,release_number,name,manifest_hash,request_hash,scope_status,published_by,idempotency_key,origin)
 SELECT id,$1,1,'Purge migration snapshot',repeat('d',64),repeat('e',64),'PARTIAL','MIGRATION','purge-migration','MIGRATED_CURRENT_STATE'
 FROM test_suites WHERE document_set_id=$1 RETURNING id`, set.ID).Scan(&releaseID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO test_suite_release_timestamp_audits
 (release_id,migration_version,previous_published_at,previous_created_at,previous_item_created_at,reason)
 VALUES($1,31,'2001-01-01T00:00:00Z','2001-01-01T00:00:00Z','[]','Purge correction audit fixture')`, releaseID); err != nil {
		t.Fatal(err)
	}
	set, err = service.UpdateLifecycle(ctx, set.ID, LifecycleInput{Status: SetStatusArchived,
		RetentionDays: 30, Actor: "integration", Reason: "exercise retention purge"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE document_sets SET archived_at=NOW()-INTERVAL '31 days' WHERE id=$1`, set.ID); err != nil {
		t.Fatal(err)
	}
	preview, err := service.PurgePreview(ctx, set.ID)
	if err != nil || !preview.Eligible || preview.StorageObjectCount != 1 {
		t.Fatalf("preview=%+v error=%v", preview, err)
	}
	confirmation := "PURGE DOCUMENT SET " + strings.TrimPrefix(preview.Confirmation, "PURGE DOCUMENT SET ")
	result, err := service.Purge(ctx, set.ID, PurgeInput{Actor: "integration-admin",
		Reason: "retention elapsed", Confirmation: confirmation})
	if err != nil || result.Status != "COMPLETED" || result.DeletedObjects != 1 {
		t.Fatalf("purge=%+v error=%v", result, err)
	}
	if _, err = files.Open(ctx, version.StorageKey); err != ErrNotFound {
		t.Fatalf("open purged object error=%v, want ErrNotFound", err)
	}
	var setCount, auditCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM document_sets WHERE id=$1`, set.ID).Scan(&setCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM document_set_purge_audit
		WHERE document_set_id=$1 AND status='COMPLETED'`, set.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if setCount != 0 || auditCount != 1 {
		t.Fatalf("set count=%d audit count=%d", setCount, auditCount)
	}
	var correctionCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM test_suite_release_timestamp_audits WHERE release_id=$1`, releaseID).Scan(&correctionCount); err != nil || correctionCount != 0 {
		t.Fatalf("purged release correction count=%d error=%v", correctionCount, err)
	}
}
