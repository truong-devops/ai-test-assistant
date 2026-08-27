package review

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type AnalysisGetter interface {
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
}

type Repository interface {
	Decide(ctx context.Context, generatedTestID int64, decision, reviewerName, comment string) (Review, error)
	List(ctx context.Context, analysisID int64) ([]Review, error)
}

type Service struct {
	analyses   AnalysisGetter
	repository Repository
}

func NewService(analyses AnalysisGetter, repository Repository) *Service {
	return &Service{analyses: analyses, repository: repository}
}

func (s *Service) Decide(ctx context.Context, generatedTestID int64, decision string, input DecisionInput) (Review, error) {
	if generatedTestID <= 0 {
		return Review{}, fmt.Errorf("%w: generated test id must be positive", ErrInvalidInput)
	}
	decision = strings.ToUpper(strings.TrimSpace(decision))
	if decision != DecisionAccepted && decision != DecisionRejected {
		return Review{}, fmt.Errorf("%w: decision must be accepted or rejected", ErrInvalidInput)
	}
	input.ReviewerName = strings.TrimSpace(input.ReviewerName)
	if input.ReviewerName == "" {
		input.ReviewerName = DefaultReviewerName
	}
	input.Comment = strings.TrimSpace(input.Comment)
	if utf8.RuneCountInString(input.ReviewerName) > MaxReviewerNameBytes {
		return Review{}, fmt.Errorf("%w: reviewer_name is too long", ErrInvalidInput)
	}
	if len(input.Comment) > MaxCommentBytes {
		return Review{}, fmt.Errorf("%w: comment is too long", ErrInvalidInput)
	}
	return s.repository.Decide(ctx, generatedTestID, decision, input.ReviewerName, input.Comment)
}

func (s *Service) List(ctx context.Context, analysisID int64) ([]Review, error) {
	if analysisID <= 0 {
		return nil, job.ErrNotFound
	}
	if _, _, err := s.analyses.Get(ctx, analysisID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, analysisID)
}
