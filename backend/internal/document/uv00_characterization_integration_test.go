//go:build integration

package document

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

// This test records DATA-01 and DATA-02 at the UV-00 baseline. UV-01 must
// replace these expectations with source snapshots and generation membership.
func TestUV00CharacterizationUploadDoesNotInvalidateReadyIndexAndAllChunksIncludesHistory(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	setID := insertIndexFixture(t, ctx, pool, "uv00-order", ApprovalApproved)
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID) }()
	var documentID, versionOneID int64
	if err := pool.QueryRow(ctx, `SELECT d.id,v.id FROM documents d
		JOIN document_versions v ON v.document_id=d.id
		WHERE d.document_set_id=$1 AND v.version_number=1`, setID).Scan(&documentID, &versionOneID); err != nil {
		t.Fatal(err)
	}

	indexRepository := NewIndexRepository(pool)
	indexService := NewIndexService(indexRepository,
		knowledge.NewHashEmbeddingClient("hash-uv00", knowledge.EmbeddingDimensions))
	ready, err := indexService.Index(ctx, setID)
	if err != nil || ready.Status != IndexReady {
		t.Fatalf("initial index=%+v err=%v", ready, err)
	}

	fixture, err := os.Open("testdata/uv00/order-v2.md")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	files, err := NewLocalFileStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	documentService := NewService(NewRepository(pool), files)
	_, versionTwo, err := documentService.Upload(ctx, setID, UploadInput{
		DocumentName: "uv00-order", DocumentType: TypeRequirements,
		Filename: "order-v2.md",
	}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if versionTwo.DocumentID != documentID || versionTwo.VersionNumber != 2 {
		t.Fatalf("uploaded version=%+v document_id=%d", versionTwo, documentID)
	}

	// Current behavior: upload leaves the old READY status untouched.
	stale, err := indexService.Status(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != IndexReady || stale.Generation != ready.Generation ||
		stale.InputFingerprint != ready.InputFingerprint {
		t.Fatalf("baseline changed: upload now invalidates the index; replace DATA-01 characterization: before=%+v after=%+v", ready, stale)
	}

	if _, err := pool.Exec(ctx, `UPDATE document_versions SET parse_status='PARSING',
		attempt_count=1,lease_expires_at=NOW()+INTERVAL '1 minute' WHERE id=$1`, versionTwo.ID); err != nil {
		t.Fatal(err)
	}
	versionTwo.ParseStatus, versionTwo.AttemptCount = ParseParsing, 1
	processor := NewProcessor(files, NewStructuredParser(), NewRepository(pool))
	if err := processor.Process(ctx, versionTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := indexService.Index(ctx, setID); err != nil {
		t.Fatal(err)
	}
	chunks, err := indexService.AllChunks(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, chunk := range chunks {
		seen[chunk.DocumentVersionID] = true
	}
	if !seen[versionOneID] || !seen[versionTwo.ID] {
		t.Fatalf("baseline changed: AllChunks no longer exposes both historical versions; seen=%v", seen)
	}
	if len(seen) < 2 || strings.TrimSpace(stale.InputFingerprint) == "" {
		t.Fatalf("invalid characterization evidence: versions=%v stale=%+v", seen, stale)
	}
}
