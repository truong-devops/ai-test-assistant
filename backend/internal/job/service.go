package job

import "context"

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context) ([]AnalysisJob, error) {
	return s.repository.List(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (AnalysisJob, []ChangedFile, error) {
	if id <= 0 {
		return AnalysisJob{}, nil, ErrNotFound
	}
	return s.repository.Get(ctx, id)
}
