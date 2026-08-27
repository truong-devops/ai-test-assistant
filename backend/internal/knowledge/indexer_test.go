package knowledge

import (
	"context"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type indexProjectStub struct{ project project.Project }

func (s indexProjectStub) GetByID(context.Context, int64) (project.Project, error) {
	return s.project, nil
}

type sourceStub struct {
	entries []gitlab.RepositoryEntry
	files   map[string][]byte
	fetched []string
}

func (s *sourceStub) GetMergeRequest(context.Context, int64, int64) (gitlab.MergeRequest, error) {
	return gitlab.MergeRequest{}, nil
}
func (s *sourceStub) GetMergeRequestDiff(context.Context, int64, int64) ([]gitlab.FileDiff, error) {
	return nil, nil
}
func (s *sourceStub) ListRepositoryTree(context.Context, int64, string) ([]gitlab.RepositoryEntry, error) {
	return s.entries, nil
}
func (s *sourceStub) GetFileRaw(_ context.Context, _ int64, filePath, _ string) ([]byte, error) {
	s.fetched = append(s.fetched, filePath)
	return s.files[filePath], nil
}

type countingEmbedder struct {
	client    *HashEmbeddingClient
	textCount int
}

func (e *countingEmbedder) Model() string   { return e.client.Model() }
func (e *countingEmbedder) Dimensions() int { return e.client.Dimensions() }
func (e *countingEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.textCount += len(texts)
	return e.client.Embed(ctx, texts)
}

type indexStoreCapture struct {
	fingerprints map[string]ChunkFingerprint
	chunks       []KnowledgeChunk
	files        int
	skipped      int
}

func (s *indexStoreCapture) ContentFingerprints(context.Context, int64) (map[string]ChunkFingerprint, error) {
	return s.fingerprints, nil
}
func (s *indexStoreCapture) SaveIndex(_ context.Context, _ IndexJob, chunks []KnowledgeChunk,
	fileCount, skippedFileCount int, _ string,
) error {
	s.chunks, s.files, s.skipped = chunks, fileCount, skippedFileCount
	s.fingerprints = make(map[string]ChunkFingerprint, len(chunks))
	for _, chunk := range chunks {
		s.fingerprints[chunk.ChunkKey] = ChunkFingerprint{
			ContentHash: chunk.ContentHash, EmbeddingModel: chunk.EmbeddingModel,
		}
	}
	return nil
}

func TestIndexerFiltersChunksAndReusesContentHashes(t *testing.T) {
	source := &sourceStub{
		entries: []gitlab.RepositoryEntry{
			{Type: "blob", Path: "internal/user/service.go"},
			{Type: "blob", Path: "internal/user/service_test.go"},
			{Type: "blob", Path: "README.md"},
			{Type: "blob", Path: "docs/secret.md"},
			{Type: "blob", Path: "vendor/dependency.go"},
			{Type: "blob", Path: ".env"},
			{Type: "tree", Path: "internal"},
		},
		files: map[string][]byte{
			"internal/user/service.go":      []byte("package user\nfunc CreateUser() {}\n"),
			"internal/user/service_test.go": []byte("package user\nfunc TestCreateUser(t *testing.T) {}\n"),
			"README.md":                     []byte("# User service\nCreateUser validates users.\n"),
			"docs/secret.md":                []byte("-----BEGIN PRIVATE KEY-----\n"),
		},
	}
	embedder := &countingEmbedder{client: NewHashEmbeddingClient("hash-test", EmbeddingDimensions)}
	store := &indexStoreCapture{fingerprints: map[string]ChunkFingerprint{}}
	indexer := NewIndexer(indexProjectStub{project.Project{GitLabProjectID: 99}}, source, embedder, store)
	claimed := IndexJob{ProjectID: 7, Ref: "main", Generation: 1, AttemptCount: 1}
	if err := indexer.Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if store.files != 3 || store.skipped != 1 || len(store.chunks) != 3 || embedder.textCount != 3 {
		t.Fatalf("files=%d skipped=%d chunks=%d embedded=%d", store.files, store.skipped,
			len(store.chunks), embedder.textCount)
	}
	for _, fetched := range source.fetched {
		if fetched == ".env" || fetched == "vendor/dependency.go" {
			t.Fatalf("excluded path was fetched: %s", fetched)
		}
	}
	embedder.textCount = 0
	if err := indexer.Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if embedder.textCount != 0 {
		t.Fatalf("unchanged chunks were re-embedded: %d", embedder.textCount)
	}
	for _, chunk := range store.chunks {
		if len(chunk.Embedding) != 0 {
			t.Fatalf("unchanged chunk contains a new embedding: %s", chunk.SymbolName)
		}
	}
}
