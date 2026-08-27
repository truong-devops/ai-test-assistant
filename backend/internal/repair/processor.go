package repair

import (
	"context"
	"fmt"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

type AnalysisReader interface {
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
	ListChangedSymbols(ctx context.Context, analysisID int64) ([]job.ChangedSymbol, error)
}

type RecommendationReader interface {
	List(ctx context.Context, analysisID int64) ([]recommendation.Recommendation, error)
}

type GeneratedTestReader interface {
	ListLatest(ctx context.Context, analysisID int64) ([]generation.GeneratedTest, error)
}

type ValidationReader interface {
	List(ctx context.Context, analysisID int64) ([]validation.Run, error)
}

type ContextRetriever interface {
	RetrieveContext(ctx context.Context, query knowledge.RetrievalQuery) ([]knowledge.KnowledgeChunk, error)
}

type ResultSaver interface {
	SaveRepairs(ctx context.Context, claimed job.AnalysisJob, repairs []ProposedRepair) error
}

type Processor struct {
	analyses          AnalysisReader
	recommendations   RecommendationReader
	generated         GeneratedTestReader
	validations       ValidationReader
	retriever         ContextRetriever
	provider          llm.Provider
	results           ResultSaver
	maxRepairAttempts int
}

func NewProcessor(analyses AnalysisReader, recommendations RecommendationReader,
	generated GeneratedTestReader, validations ValidationReader, retriever ContextRetriever,
	provider llm.Provider, results ResultSaver, maxRepairAttempts int,
) *Processor {
	return &Processor{analyses: analyses, recommendations: recommendations,
		generated: generated, validations: validations, retriever: retriever,
		provider: provider, results: results, maxRepairAttempts: maxRepairAttempts}
}

func (p *Processor) Process(ctx context.Context, claimed job.AnalysisJob) error {
	if claimed.ID <= 0 || claimed.ProjectID <= 0 || p.maxRepairAttempts < 0 || p.maxRepairAttempts > 3 {
		return fmt.Errorf("claimed repair analysis or max attempts is invalid")
	}
	analysisJob, files, err := p.analyses.Get(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("get repair analysis: %w", err)
	}
	if analysisJob.ProjectID != claimed.ProjectID {
		return fmt.Errorf("claimed repair project does not match persisted analysis")
	}
	symbols, err := p.analyses.ListChangedSymbols(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("list repair symbols: %w", err)
	}
	recommendations, err := p.recommendations.List(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("list repair recommendations: %w", err)
	}
	generatedTests, err := p.generated.ListLatest(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("list latest generated tests for repair: %w", err)
	}
	validationRuns, err := p.validations.List(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("list validation feedback for repair: %w", err)
	}
	filesByID := make(map[int64]job.ChangedFile, len(files))
	for _, file := range files {
		filesByID[file.ID] = file
	}
	symbolsByID := make(map[int64]job.ChangedSymbol, len(symbols))
	for _, symbol := range symbols {
		symbolsByID[symbol.ID] = symbol
	}
	recommendationsByID := make(map[int64]recommendation.Recommendation, len(recommendations))
	for _, item := range recommendations {
		recommendationsByID[item.ID] = item
	}
	validationsByGeneratedID := make(map[int64]validation.Run, len(validationRuns))
	for _, run := range validationRuns {
		validationsByGeneratedID[run.GeneratedTestID] = run
	}

	repairs := make([]ProposedRepair, 0)
	for _, previous := range generatedTests {
		if previous.AnalysisJobID != claimed.ID || previous.GenerationAttempt <= 0 {
			return fmt.Errorf("generated test %d has invalid repair ownership or version", previous.ID)
		}
		failedValidation, exists := validationsByGeneratedID[previous.ID]
		if !exists {
			return fmt.Errorf("latest generated test %d has no validation feedback", previous.ID)
		}
		if failedValidation.Status == validation.StatusPassed {
			continue
		}
		if failedValidation.Status != validation.StatusFailed && failedValidation.Status != validation.StatusTimedOut {
			return fmt.Errorf("generated test %d has invalid validation status %q", previous.ID, failedValidation.Status)
		}
		repairAttempt := previous.GenerationAttempt
		if repairAttempt > p.maxRepairAttempts {
			continue
		}
		recommendationItem, exists := recommendationsByID[previous.RecommendationID]
		if !exists || recommendationItem.AnalysisJobID != claimed.ID {
			return fmt.Errorf("recommendation %d for generated test %d is missing", previous.RecommendationID, previous.ID)
		}
		symbol, exists := symbolsByID[recommendationItem.ChangedSymbolID]
		if !exists {
			return fmt.Errorf("changed symbol %d for repair is missing", recommendationItem.ChangedSymbolID)
		}
		file, exists := filesByID[symbol.ChangedFileID]
		if !exists {
			return fmt.Errorf("changed file %d for repair is missing", symbol.ChangedFileID)
		}
		changedFilePath := file.NewPath
		if file.DeletedFile {
			changedFilePath = file.OldPath
		}
		contexts, err := p.retriever.RetrieveContext(ctx, knowledge.RetrievalQuery{
			ProjectID: claimed.ProjectID,
			Query: strings.Join([]string{symbol.SymbolName, symbol.ReceiverName, symbol.PackageName,
				recommendationItem.Title, recommendationItem.Scenario,
				"repair compiler assertion timeout interfaces signatures tests mocks"}, " "),
			PackageName: symbol.PackageName, SymbolName: symbol.SymbolName,
			FilePath: changedFilePath, PreferTests: true, Limit: 12,
		})
		if err != nil {
			return fmt.Errorf("retrieve repair context for generated test %d: %w", previous.ID, err)
		}
		if len(contexts) == 0 {
			return fmt.Errorf("retrieve repair context for generated test %d: no project context found", previous.ID)
		}
		prompt, err := RenderPrompt(PromptData{
			AnalysisID: claimed.ID, ProjectID: claimed.ProjectID,
			MergeRequestIID: analysisJob.MergeRequestIID, RepairAttempt: repairAttempt,
			ChangedFilePath: changedFilePath, Symbol: symbol, Recommendation: recommendationItem,
			Previous: previous, Validation: failedValidation, Contexts: contexts,
		})
		if err != nil {
			return err
		}
		response, err := p.provider.Generate(ctx, llm.Request{
			Instructions: Instructions, Input: prompt, SchemaName: "repaired_go_test",
			Schema: generation.ResponseSchema(), MaxOutputTokens: 6000,
		})
		if err != nil {
			return fmt.Errorf("repair generated test %d: %w", previous.ID, err)
		}
		if strings.TrimSpace(response.Model) == "" {
			return fmt.Errorf("repair generated test %d: provider model is missing", previous.ID)
		}
		repaired, err := generation.ParseResponse(response.Output, changedFilePath, symbol.PackageName)
		if err != nil {
			return fmt.Errorf("repair generated test %d: %w", previous.ID, err)
		}
		if repaired.TargetFile != previous.FilePath {
			return fmt.Errorf("repair generated test %d: target file must remain unchanged", previous.ID)
		}
		repairedHash := generation.CodeHash(repaired.Code)
		if repairedHash == previous.CodeHash {
			return fmt.Errorf("repair generated test %d: provider returned unchanged code", previous.ID)
		}
		repairs = append(repairs, ProposedRepair{
			SourceGeneratedTestID: previous.ID, ValidationRunID: failedValidation.ID,
			AttemptNumber: repairAttempt, Reason: repairReason(failedValidation),
			Generated: generation.GeneratedTest{
				AnalysisJobID: claimed.ID, RecommendationID: previous.RecommendationID,
				FilePath: repaired.TargetFile, TestNames: repaired.TestNames, Code: repaired.Code,
				CodeHash: repairedHash, ModelName: response.Model, PromptVersion: PromptVersion,
				ProviderResponseID: response.ID, GenerationAttempt: previous.GenerationAttempt + 1,
			},
		})
	}
	return p.results.SaveRepairs(ctx, claimed, repairs)
}

func repairReason(run validation.Run) string {
	reason := fmt.Sprintf("validation %s (exit %d)", run.Status, run.ExitCode)
	feedback := strings.TrimSpace(run.Stderr)
	if feedback == "" {
		feedback = strings.TrimSpace(run.Stdout)
	}
	if feedback != "" {
		reason += ": " + feedback
	}
	return truncateRunes(reason, MaxReasonRunes)
}
