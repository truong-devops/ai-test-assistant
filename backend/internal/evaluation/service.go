package evaluation

import (
	"context"
	"fmt"
)

type Store interface {
	Save(ctx context.Context, dataset Dataset, report Report) (Run, error)
	List(ctx context.Context) ([]Run, error)
	Get(ctx context.Context, id int64) (Run, Dataset, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Import(ctx context.Context, dataset Dataset) (StoredReport, error) {
	report, err := BuildReport(dataset)
	if err != nil {
		return StoredReport{}, err
	}
	run, err := s.store.Save(ctx, dataset, report)
	if err != nil {
		return StoredReport{}, fmt.Errorf("save evaluation report: %w", err)
	}
	return StoredReport{Run: run, Report: report}, nil
}

func (s *Service) List(ctx context.Context) ([]Run, error) {
	return s.store.List(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (StoredReport, error) {
	if id <= 0 {
		return StoredReport{}, ErrNotFound
	}
	run, dataset, err := s.store.Get(ctx, id)
	if err != nil {
		return StoredReport{}, err
	}
	report, err := BuildReport(dataset)
	if err != nil {
		return StoredReport{}, fmt.Errorf("rebuild stored evaluation report: %w", err)
	}
	if report.DatasetHash != run.DatasetHash {
		return StoredReport{}, fmt.Errorf("stored evaluation dataset hash does not match run %d", run.ID)
	}
	return StoredReport{Run: run, Report: report}, nil
}
