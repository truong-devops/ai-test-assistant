package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type ProjectGetter interface {
	GetByID(ctx context.Context, id int64) (project.Project, error)
}

type IndexStore interface {
	ContentFingerprints(ctx context.Context, projectID int64) (map[string]ChunkFingerprint, error)
	SaveIndex(ctx context.Context, claimed IndexJob, chunks []KnowledgeChunk,
		fileCount, skippedFileCount int, embeddingModel string) error
}

type Indexer struct {
	projects ProjectGetter
	source   gitlab.Client
	embedder EmbeddingClient
	store    IndexStore
}

func NewIndexer(projects ProjectGetter, source gitlab.Client, embedder EmbeddingClient, store IndexStore) *Indexer {
	return &Indexer{projects: projects, source: source, embedder: embedder, store: store}
}

func (i *Indexer) Process(ctx context.Context, claimed IndexJob) error {
	registeredProject, err := i.projects.GetByID(ctx, claimed.ProjectID)
	if err != nil {
		return fmt.Errorf("get project for indexing: %w", err)
	}
	entries, err := i.source.ListRepositoryTree(ctx, registeredProject.GitLabProjectID, claimed.Ref)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Path < entries[right].Path })
	drafts := make([]DraftChunk, 0)
	fileCount, skippedFileCount := 0, 0
	seenPaths := make(map[string]struct{})
	for _, entry := range entries {
		if entry.Type != "blob" || !IsIndexablePath(entry.Path) {
			continue
		}
		if _, duplicate := seenPaths[entry.Path]; duplicate {
			continue
		}
		seenPaths[entry.Path] = struct{}{}
		content, err := i.source.GetFileRaw(ctx, registeredProject.GitLabProjectID, entry.Path, claimed.Ref)
		if err != nil {
			return fmt.Errorf("fetch index source %q: %w", entry.Path, err)
		}
		fileChunks, err := ChunkFile(entry.Path, content)
		if err != nil {
			if errors.Is(err, ErrUnsupportedFile) || errors.Is(err, ErrSensitiveFile) ||
				errors.Is(err, ErrInvalidGoFile) {
				skippedFileCount++
				continue
			}
			return err
		}
		fileCount++
		drafts = append(drafts, fileChunks...)
	}
	sort.Slice(drafts, func(left, right int) bool { return drafts[left].ChunkKey < drafts[right].ChunkKey })
	fingerprints, err := i.store.ContentFingerprints(ctx, claimed.ProjectID)
	if err != nil {
		return err
	}
	chunks := make([]KnowledgeChunk, len(drafts))
	changedIndexes := make([]int, 0, len(drafts))
	texts := make([]string, 0, len(drafts))
	for index, draft := range drafts {
		metadata, err := json.Marshal(draft.Metadata)
		if err != nil {
			return fmt.Errorf("encode chunk metadata for %q: %w", draft.SymbolName, err)
		}
		hash := ContentHash(draft.Content)
		chunks[index] = KnowledgeChunk{
			ProjectID: claimed.ProjectID, ChunkKey: draft.ChunkKey, FilePath: draft.FilePath,
			PackageName: draft.PackageName, SymbolName: draft.SymbolName, ChunkType: draft.ChunkType,
			Content: draft.Content, ContentHash: hash, StartLine: draft.StartLine, EndLine: draft.EndLine,
			EmbeddingModel: i.embedder.Model(), Metadata: metadata,
		}
		fingerprint, exists := fingerprints[draft.ChunkKey]
		if exists && fingerprint.ContentHash == hash && fingerprint.EmbeddingModel == i.embedder.Model() {
			continue
		}
		changedIndexes = append(changedIndexes, index)
		texts = append(texts, embeddingText(draft))
	}
	if len(texts) > 0 {
		embeddings, err := i.embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("embed knowledge chunks: %w", err)
		}
		if len(embeddings) != len(changedIndexes) {
			return fmt.Errorf("embedding client returned %d vectors for %d chunks", len(embeddings), len(changedIndexes))
		}
		for resultIndex, chunkIndex := range changedIndexes {
			if len(embeddings[resultIndex]) != EmbeddingDimensions {
				return fmt.Errorf("embedding for chunk %q has %d dimensions, want %d",
					chunks[chunkIndex].SymbolName, len(embeddings[resultIndex]), EmbeddingDimensions)
			}
			chunks[chunkIndex].Embedding = embeddings[resultIndex]
		}
	}
	return i.store.SaveIndex(ctx, claimed, chunks, fileCount, skippedFileCount, i.embedder.Model())
}

func embeddingText(chunk DraftChunk) string {
	return strings.Join([]string{chunk.FilePath, chunk.PackageName, chunk.ChunkType,
		chunk.SymbolName, chunk.Content}, "\n")
}
