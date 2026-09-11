package scope

import (
	"context"
	"fmt"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type JobEnqueuer interface {
	Enqueue(context.Context, job.EnqueueInput) (job.AnalysisJob, bool, error)
}
type Snapshotter interface {
	SnapshotForAnalysis(context.Context, job.AnalysisJob) (bool, error)
}

type Enqueuer struct {
	jobs      JobEnqueuer
	snapshots Snapshotter
}

func NewEnqueuer(jobs JobEnqueuer, snapshots Snapshotter) *Enqueuer {
	return &Enqueuer{jobs: jobs, snapshots: snapshots}
}

func (e *Enqueuer) Enqueue(ctx context.Context, input job.EnqueueInput) (job.AnalysisJob, bool, error) {
	analysis, created, err := e.jobs.Enqueue(ctx, input)
	if err != nil {
		return analysis, created, err
	}
	if _, err := e.snapshots.SnapshotForAnalysis(ctx, analysis); err != nil {
		return analysis, created, fmt.Errorf("snapshot document baseline: %w", err)
	}
	return analysis, created, nil
}
