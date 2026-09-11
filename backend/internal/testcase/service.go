package testcase

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

type RequirementReader interface {
	List(context.Context, requirement.Filter) ([]requirement.Requirement, error)
	Get(context.Context, int64) (requirement.Detail, error)
}

type Service struct {
	repository   *Repository
	requirements RequirementReader
	generator    *Generator
	llmGenerator *LLMGenerator
	callRecorder AICallRecorder
}

type AICallRecorder interface {
	SaveAICall(context.Context, requirement.AICall) error
}

type generatedCase struct {
	Proposal Proposal
	Case     TestCase
}

func NewServiceWithLLM(repository *Repository, requirements RequirementReader,
	llmGenerator *LLMGenerator, callRecorder AICallRecorder,
) *Service {
	return &Service{repository: repository, requirements: requirements, generator: NewGenerator(),
		llmGenerator: llmGenerator, callRecorder: callRecorder}
}

func NewService(repository *Repository, requirements RequirementReader) *Service {
	return &Service{repository: repository, requirements: requirements, generator: NewGenerator()}
}

func (s *Service) Generate(ctx context.Context, setID int64) (GenerateSummary, error) {
	if setID <= 0 {
		return GenerateSummary{}, ErrInvalidInput
	}
	baseline, err := s.requirements.List(ctx, requirement.Filter{
		DocumentSetID: setID, Status: requirement.StatusApproved,
	})
	if err != nil {
		return GenerateSummary{}, err
	}
	if len(baseline) == 0 {
		return GenerateSummary{}, ErrNoApprovedSource
	}
	suite, err := s.repository.EnsureSuite(ctx, setID)
	if err != nil {
		return GenerateSummary{}, err
	}
	summary := GenerateSummary{DocumentSetID: setID, SuiteID: suite.ID,
		RequirementCount: len(baseline)}
	kept := make([]generatedCase, 0)
	for _, item := range baseline {
		detail, err := s.requirements.Get(ctx, item.ID)
		if err != nil {
			return summary, err
		}
		proposals := s.generator.Generate(detail)
		if s.llmGenerator != nil {
			var call requirement.AICall
			proposals, call, err = s.llmGenerator.Generate(ctx, detail)
			if s.callRecorder != nil {
				if saveErr := s.callRecorder.SaveAICall(context.WithoutCancel(ctx), call); saveErr != nil {
					return summary, saveErr
				}
			}
			if err != nil {
				return summary, fmt.Errorf("generate test cases for %s: %w", item.RequirementKey, err)
			}
		}
		for _, proposal := range proposals {
			duplicateIndex, matchType := findDuplicate(kept, proposal)
			if duplicateIndex >= 0 {
				mergeProposalSources(&kept[duplicateIndex].Proposal, proposal)
				if err := s.repository.AddRequirementLinks(ctx, kept[duplicateIndex].Case.ID,
					setID, proposal); err != nil {
					return summary, err
				}
				suppressedKey := hash(proposalIdentity(proposal) + fmt.Sprint(proposal.RequirementIDs))
				created, err := s.repository.RecordDedupe(ctx, suite, kept[duplicateIndex].Case.ID,
					suppressedKey, matchType,
					"Test case có cùng mục tiêu và expected result; requirement source được gộp vào case giữ lại")
				if err != nil {
					return summary, err
				}
				if created {
					summary.SuppressedCount++
				}
				continue
			}
			saved, created, err := s.repository.Save(ctx, suite, proposal)
			if err != nil {
				return summary, err
			}
			if created {
				summary.CreatedCount++
			} else {
				summary.ReusedCount++
			}
			kept = append(kept, generatedCase{Proposal: proposal, Case: saved})
		}
	}
	return summary, nil
}

func (s *Service) Regenerate(ctx context.Context, setID int64) (GenerateSummary, error) {
	return s.Generate(ctx, setID)
}

func (s *Service) List(ctx context.Context, setID int64) ([]TestCase, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.repository.List(ctx, setID)
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

func (s *Service) BulkReview(ctx context.Context, input BulkReviewInput) ([]Detail, error) {
	input.ReviewerName = strings.TrimSpace(input.ReviewerName)
	if len(input.TestCaseIDs) == 0 || len(input.TestCaseIDs) > 200 || input.ReviewerName == "" {
		return nil, ErrInvalidInput
	}
	seen := make(map[int64]struct{}, len(input.TestCaseIDs))
	results := make([]Detail, 0, len(input.TestCaseIDs))
	for _, id := range input.TestCaseIDs {
		if id <= 0 {
			return nil, ErrInvalidInput
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		result, err := s.Review(ctx, id, ReviewInput{ReviewerName: input.ReviewerName,
			Decision: input.Decision, Comment: input.Comment})
		if err != nil {
			return results, fmt.Errorf("review test case %d: %w", id, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) Coverage(ctx context.Context, setID int64) (CoverageReport, error) {
	if setID <= 0 {
		return CoverageReport{}, ErrInvalidInput
	}
	return s.repository.Coverage(ctx, setID)
}

func findDuplicate(items []generatedCase, candidate Proposal) (int, string) {
	identity := proposalIdentity(candidate)
	for index, item := range items {
		if proposalIdentity(item.Proposal) == identity {
			return index, "EXACT"
		}
	}
	for index, item := range items {
		if item.Proposal.TestType == candidate.TestType &&
			jaccard(item.Proposal.Title+" "+item.Proposal.ExpectedResult,
				candidate.Title+" "+candidate.ExpectedResult) >= 0.88 {
			return index, "SEMANTIC"
		}
	}
	return -1, ""
}

func jaccard(left, right string) float64 {
	leftSet, rightSet := words(left), words(right)
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}
	intersection := 0
	for word := range leftSet {
		if _, ok := rightSet[word]; ok {
			intersection++
		}
	}
	return float64(intersection) / float64(len(leftSet)+len(rightSet)-intersection)
}

func words(value string) map[string]struct{} {
	result := make(map[string]struct{})
	var current []rune
	flush := func() {
		if len(current) > 1 {
			result[strings.ToLower(string(current))] = struct{}{}
		}
		current = current[:0]
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			current = append(current, character)
		} else {
			flush()
		}
	}
	flush()
	return result
}
