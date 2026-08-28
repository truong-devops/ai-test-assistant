package review

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

const MaxReviewContextChunks = 24

type ContextAnalysisReader interface {
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
	ListChangedSymbols(ctx context.Context, analysisID int64) ([]job.ChangedSymbol, error)
}

type ContextRetriever interface {
	RetrieveContext(ctx context.Context, query knowledge.RetrievalQuery) ([]knowledge.KnowledgeChunk, error)
}

// ContextService recomputes the project-filtered RAG view for changed symbols
// using the same retrieval signals as generation. Chunks include their source
// paths so a reviewer can inspect the available evidence.
type ContextService struct {
	analyses  ContextAnalysisReader
	retriever ContextRetriever
}

func NewContextService(analyses ContextAnalysisReader, retriever ContextRetriever) *ContextService {
	return &ContextService{analyses: analyses, retriever: retriever}
}

func (s *ContextService) List(ctx context.Context, analysisID int64) ([]knowledge.KnowledgeChunk, error) {
	if analysisID <= 0 {
		return nil, job.ErrNotFound
	}
	analysis, files, err := s.analyses.Get(ctx, analysisID)
	if err != nil {
		return nil, err
	}
	symbols, err := s.analyses.ListChangedSymbols(ctx, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list review context symbols: %w", err)
	}
	filesByID := make(map[int64]job.ChangedFile, len(files))
	for _, file := range files {
		filesByID[file.ID] = file
	}
	results := make([]knowledge.KnowledgeChunk, 0, min(MaxReviewContextChunks, len(symbols)*8))
	seen := make(map[string]struct{})
	for _, symbol := range symbols {
		if len(results) >= MaxReviewContextChunks {
			break
		}
		file, exists := filesByID[symbol.ChangedFileID]
		if !exists {
			return nil, fmt.Errorf("changed file %d for context symbol %d is missing", symbol.ChangedFileID, symbol.ID)
		}
		filePath := file.NewPath
		if file.DeletedFile {
			filePath = file.OldPath
		}
		chunks, err := s.retriever.RetrieveContext(ctx, knowledge.RetrievalQuery{
			ProjectID: analysis.ProjectID,
			Query: strings.Join([]string{symbol.SymbolName, symbol.ReceiverName,
				symbol.PackageName, symbol.ChangeSummary, "tests mocks interfaces signatures"}, " "),
			PackageName: symbol.PackageName,
			SymbolName:  symbol.SymbolName,
			FilePath:    filePath,
			PreferTests: true,
			Limit:       min(12, MaxReviewContextChunks-len(results)),
		})
		if err != nil {
			return nil, fmt.Errorf("retrieve review context for %s: %w", symbol.SymbolName, err)
		}
		for _, chunk := range chunks {
			if chunk.ProjectID != analysis.ProjectID {
				return nil, fmt.Errorf("review context chunk %d does not belong to the analysis project", chunk.ID)
			}
			if math.IsNaN(chunk.Score) || math.IsInf(chunk.Score, 0) {
				chunk.Score = 0
			}
			key := chunk.ChunkKey
			if key == "" {
				key = fmt.Sprintf("%d:%s:%d", chunk.ID, chunk.FilePath, chunk.StartLine)
			}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			results = append(results, chunk)
			if len(results) >= MaxReviewContextChunks {
				break
			}
		}
	}
	return results, nil
}
