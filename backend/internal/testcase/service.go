package testcase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

type suppressedCase struct {
	KeptIndex     int
	SuppressedKey string
	MatchType     string
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
	ids := make([]int64, 0, len(baseline))
	for _, item := range baseline {
		ids = append(ids, item.ID)
	}
	return s.GeneratePinned(ctx, setID, GenerationBaseline{RequirementIDs: ids})
}

// GeneratePinned consumes only the exact IDs chosen when the job was enqueued.
// Load and validate every input before invoking a provider or writing testcases.
func (s *Service) GeneratePinned(ctx context.Context, setID int64, input GenerationBaseline) (GenerateSummary, error) {
	var baseline []requirement.Detail
	var err error
	if !(input.ReviewProposals && input.IncludeRetire && len(input.RequirementIDs) == 0) {
		baseline, err = s.loadGenerationBaseline(ctx, setID, input.RequirementIDs)
	}
	if err != nil {
		return GenerateSummary{}, err
	}
	suite, err := s.repository.EnsureSuite(ctx, setID)
	if err != nil {
		return GenerateSummary{}, err
	}
	summary := GenerateSummary{DocumentSetID: setID, SuiteID: suite.ID,
		RequirementCount: len(baseline)}
	if input.ReviewProposals && input.WorkflowUnitID == 0 {
		count, err := s.repository.proposalCount(ctx, setID, input.WorkflowJobID)
		if err != nil {
			return summary, err
		}
		if count > 0 {
			summary.ProposalCount = count
			return summary, nil
		}
	}
	kept := make([]generatedCase, 0)
	suppressed := make([]suppressedCase, 0)
	for _, detail := range baseline {
		proposals := s.generator.Generate(detail)
		generation := GenerationProvenance{Provider: "disabled", Model: "deterministic",
			PromptVersion: "business-test-rules-v1", PromptHash: hash(renderGenerationPrompt(detail)),
			RequirementReviewHash: detail.Requirement.ReviewHash, WorkflowJobID: input.WorkflowJobID,
			WorkflowInputHash: input.InputHash, SourceRevision: input.SourceRevision}
		if s.llmGenerator != nil {
			var call requirement.AICall
			proposals, call, err = s.llmGenerator.Generate(ctx, detail)
			if s.callRecorder != nil {
				if saveErr := s.callRecorder.SaveAICall(context.WithoutCancel(ctx), call); saveErr != nil {
					return summary, saveErr
				}
			}
			if err != nil {
				return summary, fmt.Errorf("generate test cases for %s: %w", detail.Requirement.RequirementKey, err)
			}
			generation.Provider, generation.Model = call.Provider, call.ModelName
			generation.PromptVersion = call.PromptVersion
			generation.PromptHash = hash(call.Instructions + "\x00" + call.PromptText + "\x00" + string(call.RequestSchema))
			generation.ResponseHash = hash(call.ResponseText)
		}
		for _, proposal := range proposals {
			proposal.Generation = &generation
			duplicateIndex, matchType := findDuplicate(kept, proposal)
			if duplicateIndex >= 0 {
				suppressed = append(suppressed, suppressedCase{KeptIndex: duplicateIndex,
					SuppressedKey: hash(proposalIdentity(proposal) + fmt.Sprint(proposal.RequirementIDs)),
					MatchType:     matchType})
				continue
			}
			kept = append(kept, generatedCase{Proposal: proposal})
		}
	}
	if input.ReviewProposals {
		proposals := make([]Proposal, 0, len(kept))
		for _, item := range kept {
			proposals = append(proposals, item.Proposal)
		}
		summary.ProposalCount, err = s.repository.saveProposals(ctx, suite, input, proposals)
		return summary, err
	}
	for index := range kept {
		saved, created, err := s.repository.Save(ctx, suite, kept[index].Proposal)
		if err != nil {
			return summary, err
		}
		kept[index].Case = saved
		if created {
			summary.CreatedCount++
		} else {
			summary.ReusedCount++
		}
	}
	for _, item := range suppressed {
		created, err := s.repository.RecordDedupe(ctx, suite, kept[item.KeptIndex].Case.ID,
			item.SuppressedKey, item.MatchType,
			"Trùng toàn bộ nội dung, ordered steps và exact requirement revisions; không ghép semantic hoặc chuyển lineage")
		if err != nil {
			return summary, err
		}
		if created {
			summary.SuppressedCount++
		}
	}
	return summary, nil
}

func (s *Service) loadGenerationBaseline(ctx context.Context, setID int64, ids []int64) ([]requirement.Detail, error) {
	if setID <= 0 || len(ids) == 0 {
		return nil, ErrInvalidInput
	}
	seen := make(map[int64]bool, len(ids))
	baseline := make([]requirement.Detail, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return nil, ErrInvalidInput
		}
		seen[id] = true
		detail, err := s.requirements.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if detail.Requirement.ID != id || detail.Requirement.DocumentSetID != setID {
			return nil, ErrEvidenceInvalid
		}
		if detail.Requirement.Status != requirement.StatusApproved || len(detail.Requirement.ReviewBlockers) > 0 {
			return nil, ErrNoApprovedSource
		}
		if len(detail.Evidence) == 0 {
			return nil, ErrEvidenceInvalid
		}
		baseline = append(baseline, detail)
	}
	return baseline, nil
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
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	if input.ExpectedContentHash != "" && input.ExpectedContentHash != current.TestCase.ContentHash {
		return Detail{}, &RevisionConflictError{CurrentRevisionID: current.TestCase.ID,
			CurrentHeadToken: current.TestCase.ContentHash}
	}
	target := current.TestCase
	if hasTestCaseEdits(current.TestCase, input) {
		family, err := s.repository.GetFamily(ctx, current.TestCase.FamilyID)
		if err != nil {
			return Detail{}, err
		}
		if family.HeadRevisionID == nil {
			return Detail{}, ErrNotFound
		}
		patch := RevisionPatch{}
		if value := strings.TrimSpace(input.Title); value != "" && value != current.TestCase.Title {
			patch.Title = &value
		}
		if value := strings.TrimSpace(input.Precondition); value != "" && value != current.TestCase.Precondition {
			patch.Precondition = &value
		}
		if value := strings.TrimSpace(input.TestData); value != "" && value != current.TestCase.TestData {
			patch.TestData = &value
		}
		if value := strings.TrimSpace(input.ExpectedResult); value != "" && value != current.TestCase.ExpectedResult {
			patch.ExpectedResult = &value
		}
		if value := strings.TrimSpace(input.Postcondition); value != "" && value != current.TestCase.Postcondition {
			patch.Postcondition = &value
		}
		if value := strings.ToUpper(strings.TrimSpace(input.Risk)); value != "" && value != current.TestCase.Risk {
			patch.Risk = &value
		}
		encoded, _ := json.Marshal(struct {
			ID    int64         `json:"id"`
			Patch RevisionPatch `json:"patch"`
		}{id, patch})
		result, err := s.repository.CreateRevision(ctx, current.TestCase.FamilyID,
			CreateRevisionInput{BaseRevisionID: id, ExpectedHeadRevisionID: *family.HeadRevisionID,
				ExpectedHeadToken: family.HeadToken, Patch: &patch,
				Reason: "Edited through legacy review endpoint"},
			"legacy-review-"+hash(string(encoded)), input.ReviewerName)
		if err != nil {
			return Detail{}, err
		}
		target = result.Revision
	}
	input.ExpectedContentHash = target.ContentHash
	return s.repository.ReviewExact(ctx, target.ID, input, input.ReviewerName)
}

func (s *Service) ListFamilies(ctx context.Context, setID int64) ([]Family, error) {
	return s.repository.ListFamilies(ctx, setID)
}

func (s *Service) GetFamily(ctx context.Context, familyID int64) (Family, error) {
	return s.repository.GetFamily(ctx, familyID)
}

func (s *Service) ListVersions(ctx context.Context, familyID int64) ([]TestCase, error) {
	return s.repository.ListVersions(ctx, familyID)
}

func (s *Service) CreateRevision(ctx context.Context, familyID int64, input CreateRevisionInput,
	idempotencyKey, actor string,
) (RevisionResult, error) {
	return s.repository.CreateRevision(ctx, familyID, input, idempotencyKey, actor)
}

func (s *Service) Restore(ctx context.Context, familyID int64, input RestoreInput,
	idempotencyKey, actor string,
) (RevisionResult, error) {
	return s.repository.Restore(ctx, familyID, input, idempotencyKey, actor)
}

func (s *Service) Diff(ctx context.Context, familyID, fromID, toID int64) (RevisionDiff, error) {
	return s.repository.Diff(ctx, familyID, fromID, toID)
}

func (s *Service) Archive(ctx context.Context, familyID int64, input ArchiveInput,
	actor string,
) (Family, error) {
	return s.repository.Archive(ctx, familyID, input, actor)
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

func (s *Service) PublishRelease(ctx context.Context, setID int64, input PublishReleaseInput,
	idempotencyKey, actor string,
) (SuiteRelease, bool, error) {
	return s.repository.PublishRelease(ctx, setID, input, idempotencyKey, actor)
}

func (s *Service) ListReleases(ctx context.Context, setID int64) ([]SuiteRelease, error) {
	return s.repository.ListReleases(ctx, setID)
}

func (s *Service) GetRelease(ctx context.Context, releaseID int64) (SuiteRelease, error) {
	return s.repository.GetRelease(ctx, releaseID)
}

func findDuplicate(items []generatedCase, candidate Proposal) (int, string) {
	identity := proposalIdentity(candidate)
	for index, item := range items {
		if proposalIdentity(item.Proposal) == identity {
			return index, "EXACT"
		}
	}
	return -1, ""
}
