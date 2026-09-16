package execution

import "context"

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }
func (s *Service) Get(ctx context.Context, id int64) (Run, error) {
	if id <= 0 {
		return Run{}, ErrInvalidInput
	}
	return s.repository.Get(ctx, id)
}
func (s *Service) Request(ctx context.Context, id int64, input RequestInput) (Run, error) {
	return s.repository.Request(ctx, id, input)
}
func (s *Service) ReviewClassification(ctx context.Context, id int64, input ClassificationInput) (Run, error) {
	return s.repository.ReviewClassification(ctx, id, input)
}
