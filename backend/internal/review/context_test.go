package review

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

type contextAnalysisReaderStub struct {
	analysis job.AnalysisJob
	files    []job.ChangedFile
	symbols  []job.ChangedSymbol
	err      error
}

func (s contextAnalysisReaderStub) Get(context.Context, int64) (job.AnalysisJob, []job.ChangedFile, error) {
	return s.analysis, s.files, s.err
}

func (s contextAnalysisReaderStub) ListChangedSymbols(context.Context, int64) ([]job.ChangedSymbol, error) {
	return s.symbols, s.err
}

type contextRetrieverStub struct {
	queries []knowledge.RetrievalQuery
	chunks  []knowledge.KnowledgeChunk
	err     error
}

func (s *contextRetrieverStub) RetrieveContext(_ context.Context, query knowledge.RetrievalQuery) ([]knowledge.KnowledgeChunk, error) {
	s.queries = append(s.queries, query)
	return s.chunks, s.err
}

func TestContextServiceReturnsDeduplicatedProjectEvidence(t *testing.T) {
	retriever := &contextRetrieverStub{chunks: []knowledge.KnowledgeChunk{{ID: 5, ProjectID: 2,
		ChunkKey: "service", FilePath: "internal/user/service.go", Content: "func CreateUser() {}"}}}
	service := NewContextService(contextAnalysisReaderStub{
		analysis: job.AnalysisJob{ID: 1, ProjectID: 2},
		files:    []job.ChangedFile{{ID: 3, NewPath: "internal/user/service.go"}},
		symbols: []job.ChangedSymbol{
			{ID: 4, ChangedFileID: 3, SymbolName: "CreateUser", PackageName: "user"},
			{ID: 5, ChangedFileID: 3, SymbolName: "CreateUser", PackageName: "user"},
		},
	}, retriever)
	chunks, err := service.List(context.Background(), 1)
	if err != nil || len(chunks) != 1 || len(retriever.queries) != 2 ||
		retriever.queries[0].FilePath != "internal/user/service.go" || !retriever.queries[0].PreferTests {
		t.Fatalf("chunks=%#v queries=%#v error=%v", chunks, retriever.queries, err)
	}
}

func TestContextServiceRejectsInvalidAndCrossProjectResults(t *testing.T) {
	service := NewContextService(contextAnalysisReaderStub{}, &contextRetrieverStub{})
	if _, err := service.List(context.Background(), 0); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("List(0) error=%v", err)
	}
	service = NewContextService(contextAnalysisReaderStub{
		analysis: job.AnalysisJob{ID: 1, ProjectID: 2},
		files:    []job.ChangedFile{{ID: 3, NewPath: "service.go"}},
		symbols:  []job.ChangedSymbol{{ID: 4, ChangedFileID: 3, SymbolName: "Run"}},
	}, &contextRetrieverStub{chunks: []knowledge.KnowledgeChunk{{ID: 5, ProjectID: 9}}})
	if _, err := service.List(context.Background(), 1); err == nil {
		t.Fatal("List() error=nil for a cross-project context chunk")
	}
}

func TestContextServiceNormalizesNonFiniteScores(t *testing.T) {
	service := NewContextService(contextAnalysisReaderStub{
		analysis: job.AnalysisJob{ID: 1, ProjectID: 2},
		files:    []job.ChangedFile{{ID: 3, NewPath: "service.go"}},
		symbols:  []job.ChangedSymbol{{ID: 4, ChangedFileID: 3, SymbolName: "Run"}},
	}, &contextRetrieverStub{chunks: []knowledge.KnowledgeChunk{{
		ID: 5, ProjectID: 2, ChunkKey: "run", Score: math.NaN(),
	}}})
	chunks, err := service.List(context.Background(), 1)
	if err != nil || len(chunks) != 1 || chunks[0].Score != 0 {
		t.Fatalf("chunks=%#v error=%v", chunks, err)
	}
}
