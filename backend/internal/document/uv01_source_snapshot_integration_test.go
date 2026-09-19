//go:build integration

package document

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

func TestUV01BlocksIncompleteLatestVersionsUntilExplicitExclusion(t *testing.T) {
	ctx, pool := uv01Pool(t)
	setID := insertIndexFixture(t, ctx, pool, "blocked-sources", ApprovalApproved)
	defer deleteDocumentSet(pool, setID)
	failedID := insertUV01Version(t, ctx, pool, setID, "failed-source", ParseFailed, 0)
	emptyID := insertUV01Version(t, ctx, pool, setID, "empty-source", ParseParsed, 0)
	service := NewIndexService(NewIndexRepository(pool),
		knowledge.NewHashEmbeddingClient("hash-uv01-exclusion", knowledge.EmbeddingDimensions))

	_, err := service.Index(ctx, setID)
	var blocked *SourceSnapshotBlockedError
	if !errors.As(err, &blocked) || len(blocked.Issues) != 2 {
		t.Fatalf("index error=%v blocked=%+v, want both failed and empty sources", err, blocked)
	}
	status, err := service.IndexWithOptions(ctx, setID, IndexOptions{
		ExcludedVersionIDs: []int64{failedID, emptyID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != IndexReady || status.Freshness != IndexFreshnessCurrent ||
		status.VersionCount != 1 || status.SkippedVersionCount != 2 || len(status.Sources) != 3 {
		t.Fatalf("explicit exclusion status=%+v", status)
	}
	excluded := 0
	for _, source := range status.Sources {
		if !source.Included {
			excluded++
			if source.ExclusionReason != "USER_EXCLUDED" {
				t.Fatalf("exclusion did not preserve reason: %+v", source)
			}
		}
	}
	if excluded != 2 {
		t.Fatalf("excluded sources=%d, want 2", excluded)
	}
}

func TestUV01ApprovalRefreshesWarningsWithoutEmbedding(t *testing.T) {
	ctx, pool := uv01Pool(t)
	setID := insertIndexFixture(t, ctx, pool, "approval", ApprovalDraft)
	defer deleteDocumentSet(pool, setID)
	var versionID int64
	if err := pool.QueryRow(ctx, `SELECT v.id FROM document_versions v
		WHERE v.document_set_id=$1 ORDER BY v.version_number DESC LIMIT 1`, setID).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	embedder := &countingDocumentEmbedder{inner: knowledge.NewHashEmbeddingClient(
		"hash-uv01-approval", knowledge.EmbeddingDimensions)}
	repository := NewIndexRepository(pool)
	service := NewIndexService(repository, embedder)
	indexed, err := service.Index(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	if indexed.WarningCount != 1 || indexed.ExtractionReady {
		t.Fatalf("draft source authority status=%+v", indexed)
	}
	calls := embedder.Calls()
	if _, err := repository.ReviewVersion(ctx, versionID, VersionReviewInput{
		ReviewerName: "PO", Decision: ApprovalApproved, Comment: "Approved after indexing",
	}); err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.Status(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.WarningCount != 0 || !refreshed.ExtractionReady ||
		refreshed.Generation != indexed.Generation || embedder.Calls() != calls {
		t.Fatalf("approval should refresh authority metadata without embedding: before=%+v after=%+v calls=%d/%d",
			indexed, refreshed, calls, embedder.Calls())
	}
}

func TestUV01FailurePersistsAndRetryBuildsNewGeneration(t *testing.T) {
	ctx, pool := uv01Pool(t)
	setID := insertIndexFixture(t, ctx, pool, "retry", ApprovalApproved)
	defer deleteDocumentSet(pool, setID)
	repository := NewIndexRepository(pool)
	failing := NewIndexService(repository, failingDocumentEmbedder{})
	if _, err := failing.Index(ctx, setID); err == nil {
		t.Fatal("failing embedder unexpectedly completed")
	}
	failed, err := failing.Status(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != IndexFailed || failed.ErrorMessage == "" ||
		len(failed.Generations) != 1 || failed.Generations[0].Status != IndexFailed {
		t.Fatalf("failure metadata was not reloadable: %+v", failed)
	}
	working := NewIndexService(repository,
		knowledge.NewHashEmbeddingClient("hash-uv01-retry", knowledge.EmbeddingDimensions))
	ready, err := working.Index(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != IndexReady || ready.Generation <= failed.Generation || ready.ErrorMessage != "" {
		t.Fatalf("retry status=%+v failed=%+v", ready, failed)
	}
}

func TestUV01ConcurrentBuildsReuseSameSnapshotAndIsolateSets(t *testing.T) {
	ctx, pool := uv01Pool(t)
	setID := insertIndexFixture(t, ctx, pool, "concurrent-same", ApprovalApproved)
	defer deleteDocumentSet(pool, setID)
	blocking := newBlockingDocumentEmbedder()
	service := NewIndexService(NewIndexRepository(pool), blocking)
	firstResult := make(chan IndexStatus, 1)
	firstError := make(chan error, 1)
	go func() {
		result, err := service.Index(ctx, setID)
		firstResult <- result
		firstError <- err
	}()
	select {
	case <-blocking.entered:
	case <-ctx.Done():
		t.Fatal("first build did not reach embedding")
	}
	second, err := service.Index(ctx, setID)
	if err != nil || second.Status != IndexIndexing {
		t.Fatalf("same-snapshot build=%+v err=%v", second, err)
	}
	close(blocking.release)
	first := <-firstResult
	if err := <-firstError; err != nil {
		t.Fatal(err)
	}
	if first.Status != IndexReady || first.Generation != second.Generation || blocking.Calls() != 1 {
		t.Fatalf("same snapshot was built twice: first=%+v second=%+v calls=%d",
			first, second, blocking.Calls())
	}

	leftSet := insertIndexFixture(t, ctx, pool, "concurrent-left", ApprovalApproved)
	rightSet := insertIndexFixture(t, ctx, pool, "concurrent-right", ApprovalApproved)
	defer deleteDocumentSet(pool, leftSet)
	defer deleteDocumentSet(pool, rightSet)
	shared := NewIndexService(NewIndexRepository(pool),
		knowledge.NewHashEmbeddingClient("hash-uv01-sets", knowledge.EmbeddingDimensions))
	type outcome struct {
		setID  int64
		status IndexStatus
		err    error
	}
	outcomes := make(chan outcome, 2)
	for _, candidate := range []int64{leftSet, rightSet} {
		go func(candidate int64) {
			status, err := shared.Index(ctx, candidate)
			outcomes <- outcome{setID: candidate, status: status, err: err}
		}(candidate)
	}
	for range 2 {
		result := <-outcomes
		if result.err != nil || result.status.Status != IndexReady ||
			result.status.DocumentSetID != result.setID {
			t.Fatalf("concurrent set outcome=%+v", result)
		}
		chunks, err := shared.AllChunks(ctx, result.setID)
		if err != nil || len(chunks) == 0 {
			t.Fatalf("set %d chunks=%d err=%v", result.setID, len(chunks), err)
		}
		for _, chunk := range chunks {
			if chunk.DocumentSetID != result.setID {
				t.Fatalf("set %d received chunk from set %d", result.setID, chunk.DocumentSetID)
			}
		}
	}
}

type countingDocumentEmbedder struct {
	inner DocumentEmbeddingClient
	mu    sync.Mutex
	calls int
}

func (e *countingDocumentEmbedder) Model() string   { return e.inner.Model() }
func (e *countingDocumentEmbedder) Dimensions() int { return e.inner.Dimensions() }
func (e *countingDocumentEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	return e.inner.Embed(ctx, texts)
}
func (e *countingDocumentEmbedder) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

type failingDocumentEmbedder struct{}

func (failingDocumentEmbedder) Model() string   { return "failing-uv01" }
func (failingDocumentEmbedder) Dimensions() int { return knowledge.EmbeddingDimensions }
func (failingDocumentEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("deliberate UV-01 embedding failure")
}

type blockingDocumentEmbedder struct {
	inner   DocumentEmbeddingClient
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	calls   int
}

func newBlockingDocumentEmbedder() *blockingDocumentEmbedder {
	return &blockingDocumentEmbedder{
		inner:   knowledge.NewHashEmbeddingClient("hash-uv01-blocking", knowledge.EmbeddingDimensions),
		entered: make(chan struct{}), release: make(chan struct{}),
	}
}
func (e *blockingDocumentEmbedder) Model() string   { return e.inner.Model() }
func (e *blockingDocumentEmbedder) Dimensions() int { return e.inner.Dimensions() }
func (e *blockingDocumentEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	e.once.Do(func() { close(e.entered) })
	select {
	case <-e.release:
		return e.inner.Embed(ctx, texts)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (e *blockingDocumentEmbedder) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func uv01Pool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}

func insertUV01Version(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	setID int64, name, parseStatus string, blockCount int,
) int64 {
	t.Helper()
	var documentID, versionID int64
	if err := pool.QueryRow(ctx, `INSERT INTO documents(document_set_id,name,document_type)
		VALUES($1,$2,'REQUIREMENTS') RETURNING id`, setID, name).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	checksum := hashTextForTest(fmt.Sprintf("%d-%s-%d", setID, name, time.Now().UnixNano()))
	if err := pool.QueryRow(ctx, `INSERT INTO document_versions
		(document_id,document_set_id,version_number,original_filename,media_type,size_bytes,
		 sha256,storage_key,approval_status,parse_status,parse_error,block_count)
		VALUES($1,$2,1,$3,'text/markdown',1,$4,$5,'APPROVED',$6,
		 CASE WHEN $6='FAILED' THEN 'fixture parse error' ELSE '' END,$7) RETURNING id`,
		documentID, setID, name+".md", checksum, fmt.Sprintf("uv01/%d/%s", setID, name),
		parseStatus, blockCount).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	return versionID
}

func deleteDocumentSet(pool *pgxpool.Pool, setID int64) {
	_, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID)
}
