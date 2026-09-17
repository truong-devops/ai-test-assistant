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
}
