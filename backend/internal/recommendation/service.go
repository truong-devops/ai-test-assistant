package recommendation

import (
	"context"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type AnalysisGetter interface {
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
}

type Service struct {
	analyses   AnalysisGetter
	repository *Repository
}

func NewService(analyses AnalysisGetter, repository *Repository) *Service {
	return &Service{analyses: analyses, repository: repository}
}

func (s *Service) List(ctx context.Context, analysisID int64) ([]Recommendation, error) {
	if analysisID <= 0 {
		return nil, job.ErrNotFound
	}
	if _, _, err := s.analyses.Get(ctx, analysisID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, analysisID)
}
