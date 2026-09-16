package scope

import (
	"context"
	"errors"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type jobStub struct{ created bool }

func (s jobStub) Enqueue(context.Context, job.EnqueueInput) (job.AnalysisJob, bool, error) {
	return job.AnalysisJob{ID: 7, ProjectID: 3}, s.created, nil
}

type snapshotStub struct {
	calls int
	err   error
}

func (s *snapshotStub) SnapshotForAnalysis(context.Context, job.AnalysisJob) (bool, error) {
	s.calls++
	return s.err == nil, s.err
}

func TestEnqueuerRetriesSnapshotForDuplicateWebhook(t *testing.T) {
	snapshot := &snapshotStub{}
	enqueuer := NewEnqueuer(jobStub{created: false}, snapshot)
	analysis, created, err := enqueuer.Enqueue(context.Background(), job.EnqueueInput{})
	if err != nil || created || analysis.ID != 7 || snapshot.calls != 1 {
		t.Fatalf("analysis=%+v created=%v calls=%d err=%v", analysis, created, snapshot.calls, err)
	}
}
func TestEnqueuerReturnsPersistedJobWhenSnapshotFails(t *testing.T) {
	snapshot := &snapshotStub{err: errors.New("database unavailable")}
	enqueuer := NewEnqueuer(jobStub{created: true}, snapshot)
	analysis, created, err := enqueuer.Enqueue(context.Background(), job.EnqueueInput{})
	if err == nil || !created || analysis.ID != 7 {
		t.Fatalf("analysis=%+v created=%v err=%v", analysis, created, err)
	}
}
