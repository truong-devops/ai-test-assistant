package recommendation

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
)

type AnalysisReader interface {
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
	ListChangedSymbols(ctx context.Context, analysisID int64) ([]job.ChangedSymbol, error)
}

type ContextRetriever interface {
	RetrieveContext(ctx context.Context, query knowledge.RetrievalQuery) ([]knowledge.KnowledgeChunk, error)
}

type ResultSaver interface {
	Save(ctx context.Context, claimed job.AnalysisJob, recommendations []Recommendation) error
}

type Processor struct {
	analyses  AnalysisReader
	retriever ContextRetriever
	provider  llm.Provider
	results   ResultSaver
	recorder  provenance.Recorder
}

func NewProcessor(analyses AnalysisReader, retriever ContextRetriever, provider llm.Provider, results ResultSaver) *Processor {
	return &Processor{analyses: analyses, retriever: retriever, provider: provider, results: results}
}

func NewProcessorWithProvenance(analyses AnalysisReader, retriever ContextRetriever,
	provider llm.Provider, results ResultSaver, recorder provenance.Recorder,
) *Processor {
	processor := NewProcessor(analyses, retriever, provider, results)
	processor.recorder = recorder
	return processor
}

func (p *Processor) Process(ctx context.Context, claimed job.AnalysisJob) error {
	analysisJob, files, err := p.analyses.Get(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("get recommendation analysis: %w", err)
	}
	if analysisJob.ProjectID != claimed.ProjectID {
		return fmt.Errorf("claimed project does not match persisted analysis")
	}
	symbols, err := p.analyses.ListChangedSymbols(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("list recommendation symbols: %w", err)
	}
	if len(symbols) == 0 {
		return p.results.Save(ctx, claimed, nil)
	}
	filesByID := make(map[int64]job.ChangedFile, len(files))
	for _, file := range files {
		filesByID[file.ID] = file
	}
	results := make([]Recommendation, 0, len(symbols))
	for _, symbol := range symbols {
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
			Query: strings.Join([]string{symbol.SymbolName, symbol.ReceiverName,
				symbol.PackageName, symbol.ChangeSummary, "tests mocks interfaces"}, " "),
			PackageName: symbol.PackageName, SymbolName: symbol.SymbolName,
			FilePath: filePath, PreferTests: true, Limit: 10,
		}
		contexts, err := p.retriever.RetrieveContext(ctx, retrievalQuery)
		if err != nil {
			return fmt.Errorf("retrieve context for %s: %w", symbol.SymbolName, err)
		}
		if len(contexts) == 0 {
			return fmt.Errorf("retrieve context for %s: no project context found", symbol.SymbolName)
		}
		prompt, err := RenderPrompt(PromptData{
			AnalysisID: claimed.ID, ProjectID: claimed.ProjectID,
			MergeRequestIID: analysisJob.MergeRequestIID, FilePath: filePath,
			Diff: file.Diff, Symbol: symbol, Contexts: contexts,
		})
		if err != nil {
			return err
		}
		request := llm.Request{
			Instructions: Instructions, Input: prompt, SchemaName: "test_recommendations",
			Schema: ResponseSchema(), MaxOutputTokens: 2000,
		}
		started := time.Now()
		response, err := p.provider.Generate(ctx, request)
		latency := time.Since(started)
		if err != nil {
			processErr := fmt.Errorf("recommend tests for %s: %w", symbol.SymbolName, err)
			if recordErr := p.recordCall(ctx, analysisJob, claimed.AttemptCount, symbol.ID,
				retrievalQuery, contexts, request, response, provenance.StatusFailed,
				processErr.Error(), latency); recordErr != nil {
				return errors.Join(processErr, recordErr)
			}
			return processErr
		}
		if strings.TrimSpace(response.Model) == "" {
			processErr := fmt.Errorf("recommend tests for %s: provider model is missing", symbol.SymbolName)
			if recordErr := p.recordCall(ctx, analysisJob, claimed.AttemptCount, symbol.ID,
				retrievalQuery, contexts, request, response, provenance.StatusInvalidOutput,
				processErr.Error(), latency); recordErr != nil {
				return errors.Join(processErr, recordErr)
			}
			return processErr
		}
		proposed, err := ParseResponse(response.Output)
		if err != nil {
			processErr := fmt.Errorf("recommend tests for %s: %w", symbol.SymbolName, err)
			if recordErr := p.recordCall(ctx, analysisJob, claimed.AttemptCount, symbol.ID,
				retrievalQuery, contexts, request, response, provenance.StatusInvalidOutput,
				processErr.Error(), latency); recordErr != nil {
				return errors.Join(processErr, recordErr)
			}
			return processErr
		}
		if err := p.recordCall(ctx, analysisJob, claimed.AttemptCount, symbol.ID,
			retrievalQuery, contexts, request, response, provenance.StatusCompleted,
			"", latency); err != nil {
			return err
		}
		for _, item := range proposed.Recommendations {
			results = append(results, Recommendation{
				AnalysisJobID: claimed.ID, ChangedSymbolID: symbol.ID,
				Title: item.Title, Description: item.Description, Priority: item.Priority,
				Rationale: item.Rationale, Scenario: item.Scenario,
				ExpectedBehavior: item.ExpectedBehavior, Status: StatusPending,
				ModelName: response.Model, PromptVersion: PromptVersion,
				ProviderResponseID: response.ID,
			})
		}
	}
	return p.results.Save(ctx, claimed, results)
}

func (p *Processor) recordCall(ctx context.Context, analysisJob job.AnalysisJob,
	attemptNumber int, symbolID int64, query knowledge.RetrievalQuery,
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
		Analysis: analysisJob, Phase: provenance.PhaseRecommendation, SubjectID: symbolID,
		AttemptNumber: attemptNumber, PromptVersion: PromptVersion,
		RetrievalQuery: query, Contexts: contexts, Request: request, Response: response,
		Status: status, ErrorMessage: errorMessage, Latency: latency,
	})
	if err != nil {
		return fmt.Errorf("record recommendation provenance for symbol %d: %w", symbolID, err)
	}
	return nil
}
