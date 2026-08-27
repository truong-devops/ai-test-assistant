package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRetrievalQuery = errors.New("invalid retrieval query")

type RetrievalStore interface {
	Retrieve(ctx context.Context, query RetrievalQuery, embedding []float32) ([]KnowledgeChunk, error)
}

type Retriever struct {
	store    RetrievalStore
	embedder EmbeddingClient
}

func NewRetriever(store RetrievalStore, embedder EmbeddingClient) *Retriever {
	return &Retriever{store: store, embedder: embedder}
}

func (r *Retriever) RetrieveContext(ctx context.Context, query RetrievalQuery) ([]KnowledgeChunk, error) {
	if query.ProjectID <= 0 {
		return nil, fmt.Errorf("%w: project ID must be positive", ErrInvalidRetrievalQuery)
	}
	query.Query = strings.TrimSpace(query.Query)
	query.SymbolName = strings.TrimSpace(query.SymbolName)
	query.PackageName = strings.TrimSpace(query.PackageName)
	query.FilePath = strings.TrimSpace(query.FilePath)
	if query.Query == "" && query.SymbolName == "" && query.PackageName == "" && query.FilePath == "" {
		return nil, fmt.Errorf("%w: at least one search signal is required", ErrInvalidRetrievalQuery)
	}
	if query.Limit == 0 {
		query.Limit = 8
	}
	if query.Limit < 1 || query.Limit > 50 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 50", ErrInvalidRetrievalQuery)
	}
	searchText := strings.Join([]string{query.Query, query.SymbolName, query.PackageName, query.FilePath}, " ")
	embeddings, err := r.embedder.Embed(ctx, []string{searchText})
	if err != nil {
		return nil, fmt.Errorf("embed retrieval query: %w", err)
	}
	if len(embeddings) != 1 || len(embeddings[0]) != EmbeddingDimensions {
		return nil, fmt.Errorf("embedding client returned an invalid retrieval vector")
	}
	return r.store.Retrieve(ctx, query, embeddings[0])
}
