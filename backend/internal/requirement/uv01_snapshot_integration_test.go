//go:build integration

package requirement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

func TestUV01PinnedExtractionFinishesOldSnapshotButIsNotCurrent(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	setID, documentID, versionOneID := insertUV01RequirementSource(t, ctx, pool)
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID) }()
	indexService := document.NewIndexService(document.NewIndexRepository(pool),
		knowledge.NewHashEmbeddingClient("hash-uv01-pinned", knowledge.EmbeddingDimensions))
	ready, err := indexService.Index(ctx, setID)
	if err != nil || !ready.ExtractionReady || ready.SourceSnapshotID == nil {
		t.Fatalf("ready index=%+v err=%v", ready, err)
	}
	repository := NewRepository(pool)
	service := NewService(repository, indexService, DeterministicExtractor{})
	job, err := service.RequestExtraction(ctx, setID, "UV-01 integration")
	if err != nil {
		t.Fatal(err)
	}
	if job.IndexGeneration != ready.Generation || job.SourceSnapshotID == nil ||
		*job.SourceSnapshotID != *ready.SourceSnapshotID {
		t.Fatalf("job was not pinned to current snapshot: job=%+v index=%+v", job, ready)
	}
	if _, err := pool.Exec(ctx, `UPDATE requirement_extraction_jobs SET status='RUNNING',
		attempt_count=1,started_at=NOW() WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	job.Status, job.AttemptCount = ExtractionRunning, 1

	versionTwoID := insertUV01RequirementVersionTwo(t, ctx, pool, setID, documentID)
	stale, err := indexService.Status(ctx, setID)
	if err != nil || stale.Freshness != document.IndexFreshnessStale {
		t.Fatalf("new source did not invalidate working index: %+v err=%v", stale, err)
	}
	if _, err := service.RequestExtraction(ctx, setID, "new request"); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("new extraction on stale source error=%v", err)
	}

	summary, err := service.ExtractWithProgress(ctx, setID, job.IndexGeneration, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SourceSnapshotID == nil || *summary.SourceSnapshotID != *job.SourceSnapshotID ||
		summary.CreatedCount == 0 {
		t.Fatalf("pinned extraction summary=%+v", summary)
	}
	if err := repository.CompleteExtraction(ctx, job, summary); err != nil {
		t.Fatal(err)
	}
	completed, err := repository.LatestExtraction(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != ExtractionCompleted || completed.IsCurrent {
		t.Fatalf("old snapshot output should finish but not be current: %+v", completed)
	}
	var wrongEvidence, wrongRequirements int
	if err := pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE e.document_version_id<>$2),
		count(*) FILTER (WHERE r.source_snapshot_id<>$3 OR r.source_snapshot_id IS NULL)
		FROM requirements r JOIN requirement_evidence e ON e.requirement_id=r.id
		WHERE r.document_set_id=$1`, setID, versionOneID, job.SourceSnapshotID).
		Scan(&wrongEvidence, &wrongRequirements); err != nil {
		t.Fatal(err)
	}
	if wrongEvidence != 0 || wrongRequirements != 0 {
		t.Fatalf("pinned output mixed source: evidence=%d requirements=%d v2=%d",
			wrongEvidence, wrongRequirements, versionTwoID)
	}
}

func insertUV01RequirementSource(t *testing.T, ctx context.Context,
	pool *pgxpool.Pool,
) (int64, int64, int64) {
	t.Helper()
	var setID, documentID, versionID int64
	if err := pool.QueryRow(ctx, `INSERT INTO document_sets(name)
		VALUES($1) RETURNING id`, fmt.Sprintf("uv01-pinned-%d", time.Now().UnixNano())).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO documents(document_set_id,name,document_type)
		VALUES($1,'Order requirements','REQUIREMENTS') RETURNING id`, setID).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	checksum := uv01RequirementHash("v1")
	if err := pool.QueryRow(ctx, `INSERT INTO document_versions
		(document_id,document_set_id,version_number,original_filename,media_type,size_bytes,
		 sha256,storage_key,approval_status,parse_status,block_count)
		VALUES($1,$2,1,'order-v1.md','text/markdown',10,$3,$4,'APPROVED','PARSED',1)
		RETURNING id`, documentID, setID, checksum, fmt.Sprintf("uv01/%d/v1", setID)).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO document_blocks
		(document_version_id,ordinal,block_type,content,source_locator)
		VALUES($1,1,'PARAGRAPH','BR01 Hệ thống phải tạo đơn hàng hợp lệ','line:1')`, versionID); err != nil {
		t.Fatal(err)
	}
	return setID, documentID, versionID
}

func insertUV01RequirementVersionTwo(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	setID, documentID int64,
) int64 {
	t.Helper()
	var versionID int64
	checksum := uv01RequirementHash(fmt.Sprintf("v2-%d", time.Now().UnixNano()))
	if err := pool.QueryRow(ctx, `INSERT INTO document_versions
		(document_id,document_set_id,version_number,original_filename,media_type,size_bytes,
		 sha256,storage_key,approval_status,parse_status,block_count)
		VALUES($1,$2,2,'order-v2.md','text/markdown',10,$3,$4,'APPROVED','PARSED',1)
		RETURNING id`, documentID, setID, checksum, fmt.Sprintf("uv01/%d/v2", setID)).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO document_blocks
		(document_version_id,ordinal,block_type,content,source_locator)
		VALUES($1,1,'PARAGRAPH','BR01 Hệ thống phải tạo đơn hàng với quy tắc mới','line:1')`, versionID); err != nil {
		t.Fatal(err)
	}
	return versionID
}

func uv01RequirementHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
