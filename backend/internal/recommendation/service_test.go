package recommendation

import (
	"context"
	"errors"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type analysisGetterStub struct{ err error }

func (s analysisGetterStub) Get(context.Context, int64) (job.AnalysisJob, []job.ChangedFile, error) {
	return job.AnalysisJob{ID: 1}, nil, s.err
}

func TestServiceRejectsInvalidOrMissingAnalysisBeforeRepositoryAccess(t *testing.T) {
	service := NewService(analysisGetterStub{}, nil)
	if _, err := service.List(context.Background(), 0); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("List(0) error=%v", err)
	}
	service = NewService(analysisGetterStub{err: job.ErrNotFound}, nil)
	if _, err := service.List(context.Background(), 99); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("List(missing) error=%v", err)
	}
}
