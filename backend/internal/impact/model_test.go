package impact

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

type impactReaderStub struct {
	result Bundle
	err    error
}

func (s impactReaderStub) Get(context.Context, int64) (Bundle, error) { return s.result, s.err }

func TestServiceGet(t *testing.T) {
	want := Bundle{Run: Run{ID: 9, Mode: ModeSSA}}
	got, err := NewService(analysisGetterStub{}, impactReaderStub{result: want}).Get(context.Background(), 1)
	if err != nil || got.Run.ID != want.Run.ID {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	if _, err := NewService(analysisGetterStub{err: job.ErrNotFound}, impactReaderStub{}).
		Get(context.Background(), 1); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}
