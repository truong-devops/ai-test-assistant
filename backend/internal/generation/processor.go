package generation

import (
	"context"
	"fmt"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
)

type AnalysisReader interface {
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
	ListChangedSymbols(ctx context.Context, analysisID int64) ([]job.ChangedSymbol, error)
}

type RecommendationReader interface {
	List(ctx context.Context, analysisID int64) ([]recommendation.Recommendation, error)
}

type ContextRetriever interface {
	RetrieveContext(ctx context.Context, query knowledge.RetrievalQuery) ([]knowledge.KnowledgeChunk, error)
}

type ResultSaver interface {
	Save(ctx context.Context, claimed job.AnalysisJob, generated []GeneratedTest) error
}

type Processor struct {
	analyses        AnalysisReader
	recommendations RecommendationReader
	retriever       ContextRetriever
	provider        llm.Provider
	results         ResultSaver
}

func NewProcessor(analyses AnalysisReader, recommendations RecommendationReader,
	retriever ContextRetriever, provider llm.Provider, results ResultSaver,
) *Processor {
	return &Processor{analyses: analyses, recommendations: recommendations,
		retriever: retriever, provider: provider, results: results}
}

func (p *Processor) Process(ctx context.Context, claimed job.AnalysisJob) error {
	analysisJob, files, err := p.analyses.Get(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("get generation analysis: %w", err)
	}
	if analysisJob.ProjectID != claimed.ProjectID {
		return fmt.Errorf("claimed project does not match persisted analysis")
	}
	symbols, err := p.analyses.ListChangedSymbols(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("list generation symbols: %w", err)
	}
	recommendations, err := p.recommendations.List(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("list generation recommendations: %w", err)
	}
	if len(recommendations) == 0 {
		return p.results.Save(ctx, claimed, nil)
	}
	filesByID := make(map[int64]job.ChangedFile, len(files))
	for _, file := range files {
		filesByID[file.ID] = file
	}
	symbolsByID := make(map[int64]job.ChangedSymbol, len(symbols))
	for _, symbol := range symbols {
		symbolsByID[symbol.ID] = symbol
	}
	generated := make([]GeneratedTest, 0, len(recommendations))
	for _, recommendationItem := range recommendations {
		symbol, exists := symbolsByID[recommendationItem.ChangedSymbolID]
		if !exists {
			return fmt.Errorf("changed symbol %d for recommendation %d is missing",
				recommendationItem.ChangedSymbolID, recommendationItem.ID)
		}
		file, exists := filesByID[symbol.ChangedFileID]
		if !exists {
			return fmt.Errorf("changed file %d for symbol %d is missing", symbol.ChangedFileID, symbol.ID)
		}
		filePath := file.NewPath
		if file.DeletedFile {
			filePath = file.OldPath
		}
		contexts, err := p.retriever.RetrieveContext(ctx, knowledge.RetrievalQuery{
			ProjectID: claimed.ProjectID,
			Query: strings.Join([]string{symbol.SymbolName, symbol.ReceiverName, symbol.PackageName,
				recommendationItem.Title, recommendationItem.Scenario,
				recommendationItem.ExpectedBehavior, "tests mocks interfaces signatures"}, " "),
			PackageName: symbol.PackageName, SymbolName: symbol.SymbolName,
			FilePath: filePath, PreferTests: true, Limit: 12,
		})
		if err != nil {
			return fmt.Errorf("retrieve generation context for %s: %w", symbol.SymbolName, err)
		}
		if len(contexts) == 0 {
			return fmt.Errorf("retrieve generation context for %s: no project context found", symbol.SymbolName)
		}
		prompt, err := RenderPrompt(PromptData{
			AnalysisID: claimed.ID, ProjectID: claimed.ProjectID,
			MergeRequestIID: analysisJob.MergeRequestIID, ChangedFilePath: filePath,
			Diff: file.Diff, Symbol: symbol, Recommendation: recommendationItem, Contexts: contexts,
		})
		if err != nil {
			return err
		}
		response, err := p.provider.Generate(ctx, llm.Request{
			Instructions: Instructions, Input: prompt, SchemaName: "generated_go_test",
			Schema: ResponseSchema(), MaxOutputTokens: 6000,
		})
		if err != nil {
			return fmt.Errorf("generate test for recommendation %d: %w", recommendationItem.ID, err)
		}
		if strings.TrimSpace(response.Model) == "" {
			return fmt.Errorf("generate test for recommendation %d: provider model is missing", recommendationItem.ID)
		}
		proposed, err := ParseResponse(response.Output, filePath, symbol.PackageName)
		if err != nil {
			return fmt.Errorf("generate test for recommendation %d: %w", recommendationItem.ID, err)
		}
		generated = append(generated, GeneratedTest{
			AnalysisJobID: claimed.ID, RecommendationID: recommendationItem.ID,
			FilePath: proposed.TargetFile, TestNames: proposed.TestNames, Code: proposed.Code,
			CodeHash: CodeHash(proposed.Code), ModelName: response.Model,
			PromptVersion: PromptVersion, ProviderResponseID: response.ID,
			GenerationAttempt: InitialAttempt,
		})
	}
	return p.results.Save(ctx, claimed, generated)
}
