//go:build integration

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

func TestDocumentRetrievalIsolationAuthorityRankingAndSnapshotImmutability(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	firstSet := insertIndexFixture(t, ctx, pool, "first", ApprovalApproved)
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, firstSet) }()
	insertIndexDocument(t, ctx, pool, firstSet, "draft-copy", ApprovalDraft)
	secondSet := insertIndexFixture(t, ctx, pool, "second", ApprovalApproved)
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, secondSet) }()

	indexRepository := NewIndexRepository(pool)
	service := NewIndexService(indexRepository,
		knowledge.NewHashEmbeddingClient("hash-isolation", knowledge.EmbeddingDimensions))
	firstStatus, err := service.Index(ctx, firstSet)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Index(ctx, secondSet); err != nil {
		t.Fatal(err)
	}
	query := RetrievalQuery{DocumentSetID: firstSet, Query: "BR03 số lượng tối đa 10",
		Identifier: "BR03", VersionPolicy: VersionLatest, Limit: 10}
	results, err := service.Retrieve(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 || results[0].ApprovalStatus != ApprovalApproved {
		t.Fatalf("authority ranking=%+v", results)
	}
	for _, item := range results {
		if item.DocumentSetID != firstSet {
			t.Fatalf("retrieval leaked document set %d into %d", item.DocumentSetID, firstSet)
		}
	}
	for _, identifier := range []string{"UC-B08", "PR04.03", "BR03", "AC-03"} {
		golden, err := service.Retrieve(ctx, RetrievalQuery{DocumentSetID: firstSet,
			Query: identifier, Identifier: identifier, VersionPolicy: VersionLatest,
			Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		expected := make([]int64, 0)
		for _, item := range golden {
			if item.Identifier == identifier && item.ApprovalStatus == ApprovalApproved {
				expected = append(expected, item.ID)
				break
			}
		}
		evaluation := EvaluateRecallAtK(identifier, expected, golden, 5)
		if len(expected) != 1 || evaluation.RecallAtK != 1 {
			t.Fatalf("golden retrieval %s=%+v results=%+v", identifier, evaluation, golden)
		}
	}
	snapshot, err := service.RetrieveAndSnapshot(ctx, "GOLDEN_QUERY", query)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ID == 0 || len(snapshot.Items) == 0 || snapshot.Items[0].SourceLocator == "" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if _, err := pool.Exec(ctx, `UPDATE document_context_snapshots SET query_text='changed' WHERE id=$1`, snapshot.ID); err == nil {
		t.Fatal("immutable context snapshot update succeeded")
	}
	unchanged, err := service.Index(ctx, firstSet)
	if err != nil || unchanged.Generation != firstStatus.Generation {
		t.Fatalf("incremental unchanged index=%+v err=%v", unchanged, err)
	}
}

func insertIndexFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	prefix, approval string,
) int64 {
	t.Helper()
	var setID int64
	if err := pool.QueryRow(ctx, `INSERT INTO document_sets(name) VALUES($1) RETURNING id`,
		fmt.Sprintf("index-%s-%d", prefix, time.Now().UnixNano())).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	insertIndexDocument(t, ctx, pool, setID, prefix, approval)
	return setID
}

func insertIndexDocument(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	setID int64, name, approval string,
) {
	t.Helper()
	var documentID, versionID int64
	if err := pool.QueryRow(ctx, `INSERT INTO documents(document_set_id,name,document_type)
		VALUES($1,$2,'REQUIREMENTS') RETURNING id`, setID, name).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	checksum := hashTextForTest(fmt.Sprintf("%d-%s", setID, name))
	if err := pool.QueryRow(ctx, `INSERT INTO document_versions
		(document_id,document_set_id,version_number,original_filename,media_type,size_bytes,
		 sha256,storage_key,approval_status,parse_status,block_count)
		VALUES($1,$2,1,$3,'text/markdown',20,$4,$5,$6,'PARSED',4) RETURNING id`,
		documentID, setID, name+".md", checksum, fmt.Sprintf("index-test/%d/%s", setID, name), approval).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO document_blocks
		(document_version_id,ordinal,block_type,content,source_locator)
		VALUES($1,1,'PARAGRAPH','BR03 Số lượng tối đa là 10','line:1'),
		($1,2,'PARAGRAPH','UC-B08 Khách hàng đặt hàng','line:2'),
		($1,3,'PARAGRAPH','PR04.03 Thanh toán đơn hàng','line:3'),
		($1,4,'PARAGRAPH','AC-03 Đơn hợp lệ được tạo','line:4')`, versionID); err != nil {
		t.Fatal(err)
	}
}

func hashTextForTest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
