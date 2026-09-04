package provenance

import (
	"context"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type AnalysisGetter interface {
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
}

type CallLister interface {
	List(ctx context.Context, analysisID int64) ([]Call, error)
	ListSummary(ctx context.Context, analysisID int64) ([]CallSummary, error)
}

func (s *Service) GetSummary(ctx context.Context, analysisID int64) (SummaryBundle, error) {
	if analysisID <= 0 {
		return SummaryBundle{}, job.ErrNotFound
	}
	analysis, _, err := s.analyses.Get(ctx, analysisID)
	if err != nil {
		return SummaryBundle{}, err
	}
	calls, err := s.calls.ListSummary(ctx, analysisID)
	if err != nil {
		return SummaryBundle{}, err
	}
	return SummaryBundle{SchemaVersion: SchemaVersion, Analysis: analysis, Calls: calls}, nil
}

type Service struct {
	analyses AnalysisGetter
	calls    CallLister
}

func NewService(analyses AnalysisGetter, calls CallLister) *Service {
	return &Service{analyses: analyses, calls: calls}
}

func (s *Service) GetBundle(ctx context.Context, analysisID int64) (Bundle, error) {
	if analysisID <= 0 {
		return Bundle{}, job.ErrNotFound
	}
	analysis, _, err := s.analyses.Get(ctx, analysisID)
	if err != nil {
		return Bundle{}, err
	}
	calls, err := s.calls.List(ctx, analysisID)
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{SchemaVersion: SchemaVersion, Analysis: analysis, Calls: calls}, nil
}
