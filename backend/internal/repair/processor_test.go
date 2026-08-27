package repair

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

const repairedOutput = `{"target_file":"internal/user/service_generated_test.go","test_names":["TestService_CreateUser_DuplicateEmail"],"code":"package user\n\nimport \"testing\"\n\nfunc TestService_CreateUser_DuplicateEmail(t *testing.T) { if got := 1; got != 1 { t.Fatalf(\"got %d\", got) } }\n"}`

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

type generatedReaderStub struct {
	items []generation.GeneratedTest
	err   error
}

func (s generatedReaderStub) ListLatest(context.Context, int64) ([]generation.GeneratedTest, error) {
	return s.items, s.err
}

type validationReaderStub struct {
	items []validation.Run
	err   error
}

func (s validationReaderStub) List(context.Context, int64) ([]validation.Run, error) {
	return s.items, s.err
}

type retrieverStub struct {
	query   knowledge.RetrievalQuery
	results []knowledge.KnowledgeChunk
	err     error
}

func (s *retrieverStub) RetrieveContext(_ context.Context,
	query knowledge.RetrievalQuery,
) ([]knowledge.KnowledgeChunk, error) {
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
	repairs []ProposedRepair
	calls   int
}

func (s *saverStub) SaveRepairs(_ context.Context, claimed job.AnalysisJob,
	repairs []ProposedRepair,
) error {
	s.claimed = claimed
	s.repairs = append([]ProposedRepair(nil), repairs...)
	s.calls++
	return nil
}

func repairFixture() (job.AnalysisJob, analysisReaderStub, recommendationReaderStub,
	generatedReaderStub, validationReaderStub,
) {
	claimed := job.AnalysisJob{ID: 11, ProjectID: 22, MergeRequestIID: 3,
		Status: job.StatusRepairing, AttemptCount: 1}
	analysisReader := analysisReaderStub{
		analysis: claimed,
		files:    []job.ChangedFile{{ID: 31, NewPath: "internal/user/service.go"}},
		symbols: []job.ChangedSymbol{{ID: 41, ChangedFileID: 31, SymbolName: "CreateUser",
			SymbolKind: "method", PackageName: "user", ChangeType: "modified"}},
	}
	recommendations := recommendationReaderStub{items: []recommendation.Recommendation{{
		ID: 51, AnalysisJobID: 11, ChangedSymbolID: 41, Title: "Duplicate email",
		Scenario: "lookup finds user", ExpectedBehavior: "returns ErrEmailExists",
	}}}
	previousCode := "package user\n\nimport \"testing\"\n\nfunc TestService_CreateUser_DuplicateEmail(t *testing.T) { missingRepository() }\n"
	generatedTests := generatedReaderStub{items: []generation.GeneratedTest{{
		ID: 61, AnalysisJobID: 11, RecommendationID: 51,
		FilePath:  "internal/user/service_generated_test.go",
		TestNames: []string{"TestService_CreateUser_DuplicateEmail"}, Code: previousCode,
		CodeHash: generation.CodeHash(previousCode), GenerationAttempt: 1,
	}}}
	validations := validationReaderStub{items: []validation.Run{{
		ID: 71, AnalysisJobID: 11, GeneratedTestID: 61, AttemptNumber: 1,
		Status: validation.StatusFailed, ExitCode: 1, Stderr: "undefined: missingRepository",
	}}}
	return claimed, analysisReader, recommendations, generatedTests, validations
}

func TestProcessorRepairsFailedLatestVersionAndSavesAuditData(t *testing.T) {
	claimed, analyses, recommendations, generatedTests, validations := repairFixture()
	retriever := &retrieverStub{results: []knowledge.KnowledgeChunk{{
		ProjectID: 22, FilePath: "internal/user/service_test.go", PackageName: "user",
		ChunkType: "test", Content: "func TestCreateUser(t *testing.T) {}",
	}}}
	provider := &providerStub{result: llm.Response{ID: "resp-repair-1",
		Model: "fixture-model", Output: repairedOutput}}
	saver := &saverStub{}
	processor := NewProcessor(analyses, recommendations, generatedTests, validations,
		retriever, provider, saver, 2)
	if err := processor.Process(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || provider.request.SchemaName != "repaired_go_test" ||
		provider.request.MaxOutputTokens != 6000 ||
		!strings.Contains(provider.request.Input, "undefined: missingRepository") {
		t.Fatalf("provider calls=%d request=%#v", provider.calls, provider.request)
	}
	if saver.calls != 1 || len(saver.repairs) != 1 {
		t.Fatalf("saved=%#v", saver)
	}
	repair := saver.repairs[0]
	if repair.SourceGeneratedTestID != 61 || repair.ValidationRunID != 71 ||
		repair.AttemptNumber != 1 || repair.Generated.GenerationAttempt != 2 ||
		repair.Generated.FilePath != "internal/user/service_generated_test.go" ||
		repair.Generated.PromptVersion != PromptVersion || repair.Generated.CodeHash == "" ||
		repair.Generated.CodeHash == generatedTests.items[0].CodeHash ||
		!strings.Contains(repair.Reason, "undefined: missingRepository") {
		t.Fatalf("repair=%#v", repair)
	}
}

func TestProcessorTerminatesWithoutCallingProviderAtRepairLimit(t *testing.T) {
	claimed, analyses, recommendations, generatedTests, validations := repairFixture()
	generatedTests.items[0].GenerationAttempt = 3
	validations.items[0].AttemptNumber = 3
	provider, saver := &providerStub{}, &saverStub{}
	err := NewProcessor(analyses, recommendations, generatedTests, validations,
		&retrieverStub{}, provider, saver, 2).Process(context.Background(), claimed)
	if err != nil || provider.calls != 0 || saver.calls != 1 || len(saver.repairs) != 0 {
		t.Fatalf("Process() error=%v provider=%d saver=%#v", err, provider.calls, saver)
	}
}

func TestProcessorSkipsPassingLatestVersion(t *testing.T) {
	claimed, analyses, recommendations, generatedTests, validations := repairFixture()
	validations.items[0].Status = validation.StatusPassed
	provider, saver := &providerStub{}, &saverStub{}
	err := NewProcessor(analyses, recommendations, generatedTests, validations,
		&retrieverStub{}, provider, saver, 2).Process(context.Background(), claimed)
	if err != nil || provider.calls != 0 || saver.calls != 1 || len(saver.repairs) != 0 {
		t.Fatalf("Process() error=%v provider=%d saver=%#v", err, provider.calls, saver)
	}
}

func TestProcessorRejectsChangedTargetAndUnchangedCode(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "changed target", output: strings.Replace(repairedOutput,
			"service_generated_test.go", "other_test.go", 1)},
		{name: "unchanged code", output: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claimed, analyses, recommendations, generatedTests, validations := repairFixture()
			output := test.output
			if output == "" {
				output = `{"target_file":"internal/user/service_generated_test.go","test_names":["TestService_CreateUser_DuplicateEmail"],"code":"package user\n\nimport \"testing\"\n\nfunc TestService_CreateUser_DuplicateEmail(t *testing.T) { missingRepository() }\n"}`
			}
			provider := &providerStub{result: llm.Response{Model: "fixture", Output: output}}
			saver := &saverStub{}
			err := NewProcessor(analyses, recommendations, generatedTests, validations,
				&retrieverStub{results: []knowledge.KnowledgeChunk{{Content: "context"}}},
				provider, saver, 2).Process(context.Background(), claimed)
			if err == nil || saver.calls != 0 {
				t.Fatalf("Process() error=%v saver calls=%d", err, saver.calls)
			}
		})
	}
}

func TestProcessorRequiresValidationFeedbackAndContext(t *testing.T) {
	claimed, analyses, recommendations, generatedTests, _ := repairFixture()
	provider, saver := &providerStub{}, &saverStub{}
	err := NewProcessor(analyses, recommendations, generatedTests, validationReaderStub{},
		&retrieverStub{}, provider, saver, 2).Process(context.Background(), claimed)
	if err == nil || !strings.Contains(err.Error(), "no validation feedback") || provider.calls != 0 {
		t.Fatalf("missing validation error=%v calls=%d", err, provider.calls)
	}
	_, _, _, _, validations := repairFixture()
	err = NewProcessor(analyses, recommendations, generatedTests, validations,
		&retrieverStub{}, provider, saver, 2).Process(context.Background(), claimed)
	if err == nil || !strings.Contains(err.Error(), "no project context") || provider.calls != 0 {
		t.Fatalf("missing context error=%v calls=%d", err, provider.calls)
	}
}

func TestProcessorDoesNotPersistProviderFailure(t *testing.T) {
	claimed, analyses, recommendations, generatedTests, validations := repairFixture()
	provider := &providerStub{err: errors.New("provider unavailable")}
	saver := &saverStub{}
	err := NewProcessor(analyses, recommendations, generatedTests, validations,
		&retrieverStub{results: []knowledge.KnowledgeChunk{{Content: "context"}}},
		provider, saver, 2).Process(context.Background(), claimed)
	if err == nil || saver.calls != 0 {
		t.Fatalf("Process() error=%v saver calls=%d", err, saver.calls)
	}
}
