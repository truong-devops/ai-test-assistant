package provenance

import (
	"context"
	"errors"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type analysisGetterStub struct {
	item job.AnalysisJob
	err  error
}

func (s analysisGetterStub) Get(context.Context, int64) (job.AnalysisJob, []job.ChangedFile, error) {
	return s.item, nil, s.err
}

type callListerStub struct {
	calls     []Call
	summaries []CallSummary
	err       error
}

func (s callListerStub) List(context.Context, int64) ([]Call, error) {
	return s.calls, s.err
}

func (s callListerStub) ListSummary(context.Context, int64) ([]CallSummary, error) {
	return s.summaries, s.err
}

func TestServiceReturnsSummaryAndFullBundle(t *testing.T) {
	analysis := job.AnalysisJob{ID: 3, ProjectID: 4}
	service := NewService(analysisGetterStub{item: analysis}, callListerStub{
		calls: []Call{{ID: 1}}, summaries: []CallSummary{{ID: 1}},
	})
	summary, err := service.GetSummary(context.Background(), analysis.ID)
	if err != nil || summary.SchemaVersion != SchemaVersion || len(summary.Calls) != 1 {
		t.Fatalf("summary=%#v error=%v", summary, err)
	}
	bundle, err := service.GetBundle(context.Background(), analysis.ID)
	if err != nil || bundle.SchemaVersion != SchemaVersion || len(bundle.Calls) != 1 {
		t.Fatalf("bundle=%#v error=%v", bundle, err)
	}
}

func TestServiceRejectsUnknownAnalysis(t *testing.T) {
	service := NewService(analysisGetterStub{err: job.ErrNotFound}, callListerStub{})
	if _, err := service.GetSummary(context.Background(), 9); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("GetSummary() error=%v", err)
	}
	if _, err := service.GetBundle(context.Background(), 0); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("GetBundle() error=%v", err)
	}
}
