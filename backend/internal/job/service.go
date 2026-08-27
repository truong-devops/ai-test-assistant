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

func (s *Service) GetSymbols(ctx context.Context, id int64) ([]ChangedSymbol, error) {
	if id <= 0 {
		return nil, ErrNotFound
	}
	if _, _, err := s.repository.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repository.ListChangedSymbols(ctx, id)
}
