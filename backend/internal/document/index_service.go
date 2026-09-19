package document

import (
	"context"
	"fmt"
	"strings"
)

type DocumentEmbeddingClient interface {
	Model() string
	Dimensions() int
	Embed(context.Context, []string) ([][]float32, error)
}

type IndexService struct {
	repository *IndexRepository
	chunker    *SemanticChunker
	embedder   DocumentEmbeddingClient
}

func NewIndexService(repository *IndexRepository, embedder DocumentEmbeddingClient) *IndexService {
	return &IndexService{repository: repository, chunker: NewSemanticChunker(), embedder: embedder}
}

func (s *IndexService) Index(ctx context.Context, setID int64) (IndexStatus, error) {
	return s.IndexWithOptions(ctx, setID, IndexOptions{})
}

func (s *IndexService) IndexWithOptions(ctx context.Context, setID int64,
	options IndexOptions,
) (IndexStatus, error) {
	if setID <= 0 {
		return IndexStatus{}, ErrInvalidInput
	}
	snapshot, sources, err := s.repository.CreateSourceSnapshot(ctx, setID, options)
	if err != nil {
		return IndexStatus{}, err
	}
	if len(sources) == 0 {
		return IndexStatus{}, ErrNoParsedDocuments
	}
	if s.embedder.Dimensions() != 384 {
		return IndexStatus{}, fmt.Errorf("document index requires 384-dimensional embeddings")
	}
	fingerprint := indexFingerprint(sources, s.embedder.Model())
	started, unchanged, err := s.repository.Begin(ctx, snapshot, fingerprint, s.embedder.Model())
	if err != nil || unchanged {
		return started, err
	}
	chunks := make([]SemanticChunk, 0)
	warnings := 0
	for _, source := range sources {
		generated := s.chunker.Chunk(source)
		for index := range generated {
			generated[index].EmbeddingModel = s.embedder.Model()
			if ContainsPromptInjection(generated[index].RawContent) {
				warnings++
			}
		}
		chunks = append(chunks, generated...)
	}
	if len(chunks) == 0 {
		err = ErrNoParsedDocuments
		_ = s.repository.Fail(context.WithoutCancel(ctx), started, err)
		return IndexStatus{}, err
	}
	for offset := 0; offset < len(chunks); offset += 32 {
		end := min(offset+32, len(chunks))
		texts := make([]string, end-offset)
		for index := offset; index < end; index++ {
			texts[index-offset] = chunks[index].Content
		}
		vectors, embedErr := s.embedder.Embed(ctx, texts)
		if embedErr != nil || len(vectors) != len(texts) {
			if embedErr == nil {
				embedErr = fmt.Errorf("embedding client returned %d vectors for %d chunks", len(vectors), len(texts))
			}
			_ = s.repository.Fail(context.WithoutCancel(ctx), started, embedErr)
			return IndexStatus{}, fmt.Errorf("embed document chunks: %w", embedErr)
		}
		for index := range vectors {
			chunks[offset+index].Embedding = vectors[index]
		}
	}
	result, err := s.repository.Complete(ctx, started, chunks, len(sources),
		snapshot.ExcludedCount, warnings)
	if err != nil {
		_ = s.repository.Fail(context.WithoutCancel(ctx), started, err)
	}
	return result, err
}

func (s *IndexService) Status(ctx context.Context, setID int64) (IndexStatus, error) {
	if setID <= 0 {
		return IndexStatus{}, ErrInvalidInput
	}
	return s.repository.GetStatus(ctx, setID)
}

func (s *IndexService) Generation(ctx context.Context, setID, generation int64) (IndexGeneration, error) {
	if setID <= 0 || generation <= 0 {
		return IndexGeneration{}, ErrInvalidInput
	}
	return s.repository.GetGeneration(ctx, setID, generation)
}

func (s *IndexService) ListChunks(ctx context.Context, setID int64, chunkType, flowType string,
	limit int,
) ([]SemanticChunk, error) {
	return s.ListGenerationChunks(ctx, setID, 0, chunkType, flowType, limit)
}

func (s *IndexService) ListGenerationChunks(ctx context.Context, setID, generation int64,
	chunkType, flowType string, limit int,
) ([]SemanticChunk, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	if generation < 0 {
		return nil, ErrInvalidInput
	}
	if generation == 0 {
		status, err := s.Status(ctx, setID)
		if err != nil {
			return nil, err
		}
		if status.Status != IndexReady {
			return nil, ErrIndexNotReady
		}
		if status.Freshness != IndexFreshnessCurrent {
			return nil, ErrIndexStale
		}
		generation = status.Generation
	}
	return s.repository.ListChunks(ctx, setID, generation,
		strings.ToUpper(strings.TrimSpace(chunkType)),
		strings.ToUpper(strings.TrimSpace(flowType)), limit)
}

// AllChunks is for internal document workflows. Public inspector requests stay
// bounded by ListChunks, while extraction must not silently truncate a document set.
func (s *IndexService) AllChunks(ctx context.Context, setID int64) ([]SemanticChunk, error) {
	status, err := s.Status(ctx, setID)
	if err != nil {
		return nil, err
	}
	if status.Status != IndexReady || status.Freshness != IndexFreshnessCurrent {
		return nil, ErrIndexStale
	}
	return s.AllChunksForGeneration(ctx, setID, status.Generation)
}

func (s *IndexService) AllChunksForGeneration(ctx context.Context, setID, generation int64) ([]SemanticChunk, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	if generation <= 0 {
		return nil, ErrInvalidInput
	}
	return s.repository.ListChunks(ctx, setID, generation, "", "", -1)
}

func (s *IndexService) Retrieve(ctx context.Context, query RetrievalQuery) ([]SemanticChunk, error) {
	query.Query = strings.TrimSpace(query.Query)
	query.Identifier = strings.ToUpper(strings.TrimSpace(query.Identifier))
	query.ChunkType = strings.ToUpper(strings.TrimSpace(query.ChunkType))
	query.FlowType = strings.ToUpper(strings.TrimSpace(query.FlowType))
	query.VersionPolicy = strings.ToUpper(strings.TrimSpace(query.VersionPolicy))
	if query.VersionPolicy == "" {
		query.VersionPolicy = VersionLatest
	}
	if query.Identifier == "" {
		query.Identifier = extractSemanticIdentifier(query.Query)
	}
	if query.DocumentSetID <= 0 || query.Query == "" && query.Identifier == "" ||
		query.VersionPolicy != VersionLatest && query.VersionPolicy != VersionLatestApproved &&
			query.VersionPolicy != VersionAllIndexed {
		return nil, ErrInvalidSearch
	}
	if query.Limit == 0 {
		query.Limit = 10
	}
	if query.Limit < 1 || query.Limit > 50 {
		return nil, ErrInvalidSearch
	}
	if query.IndexGeneration == 0 {
		status, err := s.repository.GetStatus(ctx, query.DocumentSetID)
		if err != nil {
			return nil, err
		}
		if status.Status != IndexReady {
			return nil, ErrIndexNotReady
		}
		if status.Freshness != IndexFreshnessCurrent {
			return nil, ErrIndexStale
		}
		query.IndexGeneration = status.Generation
	} else {
		generation, err := s.repository.GetGeneration(ctx, query.DocumentSetID,
			query.IndexGeneration)
		if err != nil {
			return nil, err
		}
		if generation.Status != IndexReady {
			return nil, ErrIndexNotReady
		}
	}
	searchText := strings.TrimSpace(query.Query + " " + query.Identifier)
	vectors, err := s.embedder.Embed(ctx, []string{searchText})
	if err != nil {
		return nil, fmt.Errorf("embed document query: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embedding client returned an invalid query vector")
	}
	return s.repository.Retrieve(ctx, query, vectors[0])
}

func (s *IndexService) RetrieveAndSnapshot(ctx context.Context, purpose string,
	query RetrievalQuery,
) (ContextSnapshot, error) {
	if query.IndexGeneration == 0 {
		status, err := s.Status(ctx, query.DocumentSetID)
		if err != nil {
			return ContextSnapshot{}, err
		}
		if status.Status != IndexReady {
			return ContextSnapshot{}, ErrIndexNotReady
		}
		if status.Freshness != IndexFreshnessCurrent {
			return ContextSnapshot{}, ErrIndexStale
		}
		query.IndexGeneration = status.Generation
	}
	items, err := s.Retrieve(ctx, query)
	if err != nil {
		return ContextSnapshot{}, err
	}
	generation, err := s.repository.GetGeneration(ctx, query.DocumentSetID,
		query.IndexGeneration)
	if err != nil {
		return ContextSnapshot{}, err
	}
	status := IndexStatus{DocumentSetID: generation.DocumentSetID,
		Generation: generation.Generation, EmbeddingModel: generation.EmbeddingModel,
		SourceSnapshotID: generation.SourceSnapshotID}
	return s.repository.SaveSnapshot(ctx, strings.TrimSpace(purpose), query, status, items)
}

func (s *IndexService) RetrieveAndSnapshotForSubject(ctx context.Context, purpose string,
	query RetrievalQuery, subject SemanticChunk,
) (ContextSnapshot, error) {
	if query.Limit == 0 {
		query.Limit = 10
	}
	if query.IndexGeneration == 0 {
		status, err := s.Status(ctx, query.DocumentSetID)
		if err != nil {
			return ContextSnapshot{}, err
		}
		if status.Status != IndexReady || status.Freshness != IndexFreshnessCurrent {
			return ContextSnapshot{}, ErrIndexStale
		}
		query.IndexGeneration = status.Generation
	}
	items, err := s.Retrieve(ctx, query)
	if err != nil {
		return ContextSnapshot{}, err
	}
	found := false
	for _, item := range items {
		if item.ID == subject.ID {
			found = true
			break
		}
	}
	if !found {
		items = append([]SemanticChunk{subject}, items...)
		if len(items) > query.Limit {
			items = items[:query.Limit]
		}
	}
	generation, err := s.repository.GetGeneration(ctx, query.DocumentSetID,
		query.IndexGeneration)
	if err != nil {
		return ContextSnapshot{}, err
	}
	status := IndexStatus{DocumentSetID: generation.DocumentSetID,
		Generation: generation.Generation, EmbeddingModel: generation.EmbeddingModel,
		SourceSnapshotID: generation.SourceSnapshotID}
	return s.repository.SaveSnapshot(ctx, strings.TrimSpace(purpose), query, status, items)
}

func (s *IndexService) ReviewVersion(ctx context.Context, versionID int64,
	input VersionReviewInput,
) (VersionReview, error) {
	if versionID <= 0 {
		return VersionReview{}, ErrInvalidInput
	}
	return s.repository.ReviewVersion(ctx, versionID, input)
}
