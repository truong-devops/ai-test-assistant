package requirement

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
)

type Service struct {
	repository *Repository
	index      *document.IndexService
	extractor  Extractor
}

func NewService(repository *Repository, index *document.IndexService, extractor Extractor) *Service {
	return &Service{repository: repository, index: index, extractor: extractor}
}

func (s *Service) RequestExtraction(ctx context.Context, setID int64, requestedBy string) (ExtractionJob, error) {
	if setID <= 0 {
		return ExtractionJob{}, ErrInvalidInput
	}
	status, err := s.index.Status(ctx, setID)
	if err != nil {
		return ExtractionJob{}, err
	}
	if status.Status != document.IndexReady {
		return ExtractionJob{}, ErrNoIndex
	}
	return s.repository.EnqueueExtraction(ctx, setID, status.Generation, status.ChunkCount, requestedBy)
}

func (s *Service) ExtractionStatus(ctx context.Context, setID int64) (ExtractionJob, error) {
	if setID <= 0 {
		return ExtractionJob{}, ErrInvalidInput
	}
	return s.repository.LatestExtraction(ctx, setID)
}

func (s *Service) Extract(ctx context.Context, setID int64) (ExtractionSummary, error) {
	return s.extract(ctx, setID, 0, nil)
}

func (s *Service) ExtractWithProgress(ctx context.Context, setID, expectedGeneration int64,
	progress func(ExtractionSummary) error,
) (ExtractionSummary, error) {
	return s.extract(ctx, setID, expectedGeneration, progress)
}

func (s *Service) extract(ctx context.Context, setID, expectedGeneration int64,
	progress func(ExtractionSummary) error,
) (ExtractionSummary, error) {
	if setID <= 0 {
		return ExtractionSummary{}, ErrInvalidInput
	}
	status, err := s.index.Status(ctx, setID)
	if err != nil {
		return ExtractionSummary{}, err
	}
	if status.Status != document.IndexReady {
		return ExtractionSummary{}, ErrNoIndex
	}
	if expectedGeneration > 0 && status.Generation != expectedGeneration {
		return ExtractionSummary{}, ErrStaleIndex
	}
	chunks, err := s.index.AllChunks(ctx, setID)
	if err != nil {
		return ExtractionSummary{}, err
	}
	summary := ExtractionSummary{DocumentSetID: setID, ChunkCount: len(chunks)}
	type seenProposal struct {
		Proposal    Proposal
		Requirement Requirement
		Chunk       document.SemanticChunk
	}
	seen := make([]seenProposal, 0, len(chunks))
	for _, chunk := range chunks {
		queryText := chunk.Content
		if len([]rune(queryText)) > 1000 {
			queryText = string([]rune(queryText)[:1000])
		}
		snapshot, err := s.index.RetrieveAndSnapshot(ctx, "REQUIREMENT_EXTRACTION",
			document.RetrievalQuery{DocumentSetID: setID, Query: queryText,
				Identifier: chunk.Identifier, VersionPolicy: document.VersionLatest, Limit: 8})
		if err != nil {
			return summary, fmt.Errorf("build extraction context for chunk %s: %w", chunk.ChunkKey, err)
		}
		proposals, call, extractErr := s.extractor.Extract(ctx, snapshot, chunk)
		if saveErr := s.repository.SaveAICall(context.WithoutCancel(ctx), call); saveErr != nil {
			return summary, saveErr
		}
		if extractErr != nil {
			return summary, fmt.Errorf("extract requirement from chunk %s: %w", chunk.ChunkKey, extractErr)
		}
		for _, proposal := range proposals {
			candidate := proposal
			for _, previous := range seen {
				if sameSemanticIdentity(previous.Proposal, proposal) &&
					semanticSimilarity(previous.Proposal.Statement, proposal.Statement) >= 0.78 {
					candidate = previous.Proposal
					break
				}
			}
			saved, created, err := s.repository.SaveProposal(ctx, setID, chunk, candidate)
			if err != nil {
				return summary, err
			}
			if created {
				summary.CreatedCount++
			} else {
				summary.ReusedCount++
			}
			if candidate.Status == StatusTBD {
				summary.OpenQuestionCount++
			}
			for _, previous := range seen {
				if shouldConflict(previous, seenProposal{Proposal: candidate, Requirement: saved, Chunk: chunk}) {
					createdConflict, err := s.repository.MarkConflict(ctx, setID,
						previous.Requirement.ID, saved.ID,
						"Cùng định danh nghiệp vụ nhưng nguồn mô tả trạng thái, vai trò, ngưỡng hoặc kết quả khác nhau")
					if err != nil {
						return summary, err
					}
					if createdConflict {
						summary.ConflictCount++
					}
				}
			}
			seen = append(seen, seenProposal{Proposal: candidate, Requirement: saved, Chunk: chunk})
		}
		summary.ProcessedChunks++
		if progress != nil {
			if err := progress(summary); err != nil {
				return summary, err
			}
		}
	}
	return summary, nil
}

func (s *Service) List(ctx context.Context, filter Filter) ([]Requirement, error) {
	if filter.DocumentSetID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.repository.List(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id int64) (Detail, error) {
	if id <= 0 {
		return Detail{}, ErrInvalidInput
	}
	return s.repository.Get(ctx, id)
}

func (s *Service) Review(ctx context.Context, id int64, input ReviewInput) (Detail, error) {
	if id <= 0 {
		return Detail{}, ErrInvalidInput
	}
	return s.repository.Review(ctx, id, input)
}

func (s *Service) ListConflicts(ctx context.Context, setID int64) ([]Conflict, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.repository.ListConflicts(ctx, setID)
}

func (s *Service) ListOpenQuestions(ctx context.Context, setID int64) ([]OpenQuestion, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.repository.ListOpenQuestions(ctx, setID)
}

func sameSemanticIdentity(left, right Proposal) bool {
	return strings.EqualFold(strings.TrimSpace(left.Identifier), strings.TrimSpace(right.Identifier)) &&
		left.FlowType == right.FlowType && left.RequirementType == right.RequirementType
}

func shouldConflict(left, right struct {
	Proposal    Proposal
	Requirement Requirement
	Chunk       document.SemanticChunk
}) bool {
	if left.Requirement.ID == right.Requirement.ID || !sameSemanticIdentity(left.Proposal, right.Proposal) ||
		left.Chunk.ChunkType == "FLOW_STEP" || right.Chunk.ChunkType == "FLOW_STEP" {
		return false
	}
	similarity := semanticSimilarity(left.Proposal.Statement, right.Proposal.Statement)
	if similarity >= 0.78 {
		return false
	}
	return contradictory(left.Proposal.Statement, right.Proposal.Statement) ||
		left.Proposal.Actor != "" && right.Proposal.Actor != "" && !strings.EqualFold(left.Proposal.Actor, right.Proposal.Actor) ||
		differentThresholds(left.Proposal.Statement, right.Proposal.Statement)
}

func semanticSimilarity(left, right string) float64 {
	leftTokens, rightTokens := tokenSet(left), tokenSet(right)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}
	intersection := 0
	for token := range leftTokens {
		if _, ok := rightTokens[token]; ok {
			intersection++
		}
	}
	union := len(leftTokens) + len(rightTokens) - intersection
	return float64(intersection) / float64(union)
}

func tokenSet(value string) map[string]struct{} {
	result := make(map[string]struct{})
	var token []rune
	flush := func() {
		if len(token) > 1 {
			result[strings.ToLower(string(token))] = struct{}{}
		}
		token = token[:0]
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			token = append(token, character)
		} else {
			flush()
		}
	}
	flush()
	return result
}

func contradictory(left, right string) bool {
	negativeWords := []string{"không", "khong", "not", "never", "cấm", "cam", "reject", "từ chối"}
	hasNegative := func(value string) bool {
		value = strings.ToLower(value)
		for _, word := range negativeWords {
			if strings.Contains(value, word) {
				return true
			}
		}
		return false
	}
	return hasNegative(left) != hasNegative(right) && semanticSimilarity(left, right) >= 0.35
}

var numberPattern = regexp.MustCompile(`\b\d+(?:[.,]\d+)?\b`)

func differentThresholds(left, right string) bool {
	leftNumbers, rightNumbers := numberPattern.FindAllString(left, -1), numberPattern.FindAllString(right, -1)
	if len(leftNumbers) == 0 || len(rightNumbers) == 0 {
		return false
	}
	return strings.Join(leftNumbers, ",") != strings.Join(rightNumbers, ",") &&
		semanticSimilarity(left, right) >= 0.35
}
