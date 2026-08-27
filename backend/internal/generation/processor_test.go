package generation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
)

type analysisReaderStub struct {
	analysis job.AnalysisJob
	files    []job.ChangedFile
	symbols  []job.ChangedSymbol
	err      error
}

func (s analysisReaderStub) Get(context.Context, int64) (job.AnalysisJob, []job.ChangedFile, error) {
	return s.analysis, s.files, s.err
}
func (s analysisReaderStub) ListChangedSymbols(context.Context, int64) ([]job.ChangedSymbol, error) {
	return s.symbols, s.err
}

type recommendationReaderStub struct {
	items []recommendation.Recommendation
	err   error
}

func (s recommendationReaderStub) List(context.Context, int64) ([]recommendation.Recommendation, error) {
	return s.items, s.err
}

type retrieverStub struct {
	query   knowledge.RetrievalQuery
	results []knowledge.KnowledgeChunk
	err     error
}

func (s *retrieverStub) RetrieveContext(_ context.Context, query knowledge.RetrievalQuery) ([]knowledge.KnowledgeChunk, error) {
	s.query = query
	return s.results, s.err
}

type providerStub struct {
	request llm.Request
	result  llm.Response
	err     error
	calls   int
}

func (s *providerStub) Generate(_ context.Context, request llm.Request) (llm.Response, error) {
	s.calls++
	s.request = request
	return s.result, s.err
}

type saverStub struct {
	claimed job.AnalysisJob
	items   []GeneratedTest
	calls   int
}

func (s *saverStub) Save(_ context.Context, claimed job.AnalysisJob, items []GeneratedTest) error {
	s.calls++
	s.claimed, s.items = claimed, items
	return nil
}

func generationFixture() (job.AnalysisJob, analysisReaderStub, recommendationReaderStub) {
	claimed := job.AnalysisJob{ID: 11, ProjectID: 22, MergeRequestIID: 3,
		Status: job.StatusGeneratingTests, AttemptCount: 1}
	reader := analysisReaderStub{
		analysis: claimed,
		files:    []job.ChangedFile{{ID: 31, NewPath: "internal/user/service.go", Diff: "+duplicate branch"}},
		symbols: []job.ChangedSymbol{{ID: 41, ChangedFileID: 31, SymbolName: "CreateUser",
			SymbolKind: "method", PackageName: "user", StartLine: 10, EndLine: 30,
			ChangeType: "modified", ChangeSummary: "modified method CreateUser"}},
	}
	recommendations := recommendationReaderStub{items: []recommendation.Recommendation{{
		ID: 51, AnalysisJobID: 11, ChangedSymbolID: 41, Title: "Duplicate email",
		Scenario: "lookup finds user", ExpectedBehavior: "returns ErrEmailExists",
	}}}
	return claimed, reader, recommendations
}

func TestProcessorGeneratesSyntaxChecksAndSavesCandidate(t *testing.T) {
	claimed, reader, recommendations := generationFixture()
	retriever := &retrieverStub{results: []knowledge.KnowledgeChunk{{
		ProjectID: 22, FilePath: "internal/user/service_test.go", SymbolName: "TestCreateUser",
		PackageName: "user", ChunkType: "test", Content: "func TestCreateUser(t *testing.T) {}",
	}}}
	provider := &providerStub{result: llm.Response{ID: "resp-1", Model: "fixture-model", Output: validGeneratedOutput}}
	saver := &saverStub{}
	if err := NewProcessor(reader, recommendations, retriever, provider, saver).
		Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if retriever.query.ProjectID != 22 || retriever.query.SymbolName != "CreateUser" ||
		!retriever.query.PreferTests || provider.calls != 1 ||
		provider.request.SchemaName != "generated_go_test" || provider.request.MaxOutputTokens != 6000 {
		t.Fatalf("query=%#v request=%#v", retriever.query, provider.request)
	}
	if saver.calls != 1 || len(saver.items) != 1 || saver.items[0].RecommendationID != 51 ||
		saver.items[0].FilePath != "internal/user/service_generated_test.go" ||
		saver.items[0].CodeHash == "" || saver.items[0].GenerationAttempt != InitialAttempt ||
		saver.items[0].PromptVersion != PromptVersion {
		t.Fatalf("saved=%#v", saver.items)
	}
}

func TestProcessorRejectsInvalidCodeWithoutPartialSave(t *testing.T) {
	claimed, reader, recommendations := generationFixture()
	retriever := &retrieverStub{results: []knowledge.KnowledgeChunk{{Content: "context"}}}
	provider := &providerStub{result: llm.Response{Model: "fixture", Output: strings.Replace(validGeneratedOutput,
		"package user", "package other", 1)}}
	saver := &saverStub{}
	err := NewProcessor(reader, recommendations, retriever, provider, saver).Process(context.Background(), claimed)
	if !errors.Is(err, ErrInvalidProviderOutput) || saver.calls != 0 {
		t.Fatalf("Process() error=%v saves=%d", err, saver.calls)
	}
}

func TestProcessorRequiresProjectContext(t *testing.T) {
	claimed, reader, recommendations := generationFixture()
	provider := &providerStub{}
	err := NewProcessor(reader, recommendations, &retrieverStub{}, provider, &saverStub{}).
		Process(context.Background(), claimed)
	if err == nil || !strings.Contains(err.Error(), "no project context") || provider.calls != 0 {
		t.Fatalf("Process() error=%v provider calls=%d", err, provider.calls)
	}
}

func TestProcessorCompletesAnalysisWithoutRecommendations(t *testing.T) {
	claimed, reader, _ := generationFixture()
	provider, saver := &providerStub{}, &saverStub{}
	if err := NewProcessor(reader, recommendationReaderStub{}, &retrieverStub{}, provider, saver).
		Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 0 || saver.calls != 1 || len(saver.items) != 0 {
		t.Fatalf("provider calls=%d saver=%#v", provider.calls, saver)
	}
}
