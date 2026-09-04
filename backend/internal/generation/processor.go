package generation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/provenance"
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
	recorder        provenance.Recorder
}

func NewProcessor(analyses AnalysisReader, recommendations RecommendationReader,
	retriever ContextRetriever, provider llm.Provider, results ResultSaver,
) *Processor {
	return &Processor{analyses: analyses, recommendations: recommendations,
		retriever: retriever, provider: provider, results: results}
}

func NewProcessorWithProvenance(analyses AnalysisReader, recommendations RecommendationReader,
	retriever ContextRetriever, provider llm.Provider, results ResultSaver,
	recorder provenance.Recorder,
) *Processor {
	processor := NewProcessor(analyses, recommendations, retriever, provider, results)
	processor.recorder = recorder
	return processor
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
		retrievalQuery := knowledge.RetrievalQuery{
			ProjectID: claimed.ProjectID,
			Query: strings.Join([]string{symbol.SymbolName, symbol.ReceiverName, symbol.PackageName,
				recommendationItem.Title, recommendationItem.Scenario,
				recommendationItem.ExpectedBehavior, "tests mocks interfaces signatures"}, " "),
			PackageName: symbol.PackageName, SymbolName: symbol.SymbolName,
			FilePath: filePath, PreferTests: true, Limit: 12,
		}
		contexts, err := p.retriever.RetrieveContext(ctx, retrievalQuery)
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
		request := llm.Request{
			Instructions: Instructions, Input: prompt, SchemaName: "generated_go_test",
			Schema: ResponseSchema(), MaxOutputTokens: 6000,
		}
		started := time.Now()
		response, err := p.provider.Generate(ctx, request)
		latency := time.Since(started)
		if err != nil {
			processErr := fmt.Errorf("generate test for recommendation %d: %w", recommendationItem.ID, err)
			if recordErr := p.recordCall(ctx, analysisJob, claimed.AttemptCount,
				recommendationItem.ID, retrievalQuery, contexts, request, response,
				provenance.StatusFailed, processErr.Error(), latency); recordErr != nil {
				return errors.Join(processErr, recordErr)
			}
			return processErr
		}
		if strings.TrimSpace(response.Model) == "" {
			processErr := fmt.Errorf("generate test for recommendation %d: provider model is missing", recommendationItem.ID)
			if recordErr := p.recordCall(ctx, analysisJob, claimed.AttemptCount,
				recommendationItem.ID, retrievalQuery, contexts, request, response,
				provenance.StatusInvalidOutput, processErr.Error(), latency); recordErr != nil {
				return errors.Join(processErr, recordErr)
			}
			return processErr
		}
		proposed, err := ParseResponse(response.Output, filePath, symbol.PackageName)
		if err != nil {
			processErr := fmt.Errorf("generate test for recommendation %d: %w", recommendationItem.ID, err)
			if recordErr := p.recordCall(ctx, analysisJob, claimed.AttemptCount,
				recommendationItem.ID, retrievalQuery, contexts, request, response,
				provenance.StatusInvalidOutput, processErr.Error(), latency); recordErr != nil {
				return errors.Join(processErr, recordErr)
			}
			return processErr
		}
		if err := p.recordCall(ctx, analysisJob, claimed.AttemptCount, recommendationItem.ID,
			retrievalQuery, contexts, request, response, provenance.StatusCompleted,
			"", latency); err != nil {
			return err
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

func (p *Processor) recordCall(ctx context.Context, analysisJob job.AnalysisJob,
	attemptNumber int, recommendationID int64, query knowledge.RetrievalQuery,
	contexts []knowledge.KnowledgeChunk, request llm.Request, response llm.Response,
	status, errorMessage string, latency time.Duration,
) error {
	if p.recorder == nil {
		return nil
	}
	if attemptNumber <= 0 {
		attemptNumber = 1
	}
	_, err := p.recorder.Record(ctx, provenance.RecordInput{
		Analysis: analysisJob, Phase: provenance.PhaseGeneration, SubjectID: recommendationID,
		AttemptNumber: attemptNumber, PromptVersion: PromptVersion,
		RetrievalQuery: query, Contexts: contexts, Request: request, Response: response,
		Status: status, ErrorMessage: errorMessage, Latency: latency,
	})
	if err != nil {
		return fmt.Errorf("record generation provenance for recommendation %d: %w",
			recommendationID, err)
	}
	return nil
}
