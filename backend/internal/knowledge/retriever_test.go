package knowledge

import (
	"context"
	"errors"
	"testing"
)

type retrievalStoreStub struct {
	query  RetrievalQuery
	result []KnowledgeChunk
}

func (s *retrievalStoreStub) Retrieve(_ context.Context, query RetrievalQuery, embedding []float32) ([]KnowledgeChunk, error) {
	s.query = query
	if len(embedding) != EmbeddingDimensions {
		return nil, errors.New("invalid embedding")
	}
	return s.result, nil
}

func TestRetrieverValidatesAndScopesQuery(t *testing.T) {
	store := &retrievalStoreStub{result: []KnowledgeChunk{{ProjectID: 12, SymbolName: "CreateUser"}}}
	retriever := NewRetriever(store, NewHashEmbeddingClient("test", EmbeddingDimensions))
	results, err := retriever.RetrieveContext(context.Background(), RetrievalQuery{
		ProjectID: 12, SymbolName: "CreateUser", PackageName: "user", PreferTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || store.query.ProjectID != 12 || store.query.Limit != 8 {
		t.Fatalf("results=%#v query=%#v", results, store.query)
	}
	if _, err := retriever.RetrieveContext(context.Background(), RetrievalQuery{ProjectID: 0, Query: "user"}); !errors.Is(err, ErrInvalidRetrievalQuery) {
		t.Fatalf("invalid project error=%v", err)
	}
	if _, err := retriever.RetrieveContext(context.Background(), RetrievalQuery{ProjectID: 1}); !errors.Is(err, ErrInvalidRetrievalQuery) {
		t.Fatalf("empty query error=%v", err)
	}
}
