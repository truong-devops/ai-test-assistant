package job

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type queueStub struct {
	item              AnalysisJob
	claimErr          error
	failedID          int64
	failure           error
	persistContextErr error
}

func (q *queueStub) ClaimNext(context.Context, time.Duration) (AnalysisJob, error) {
	return q.item, q.claimErr
}
func (q *queueStub) RetryOrFail(ctx context.Context, id int64, _ int, err error, _ int, _ time.Duration) error {
	q.failedID, q.failure = id, err
	q.persistContextErr = ctx.Err()
	return nil
}

type processorStub struct{ err error }

func (p processorStub) Process(context.Context, AnalysisJob) error { return p.err }

func TestWorkerMarksProcessingFailure(t *testing.T) {
	processErr := errors.New("GitLab unavailable")
	queue := &queueStub{item: AnalysisJob{ID: 9}}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue, processorStub{err: processErr}, WorkerOptions{
		PollInterval: time.Second, RetryDelay: time.Second, LeaseDuration: time.Minute, MaxAttempts: 3,
	})
	if err := worker.runOnce(context.Background()); !errors.Is(err, processErr) {
		t.Fatalf("runOnce() error = %v", err)
	}
	if queue.failedID != 9 || !errors.Is(queue.failure, processErr) {
		t.Fatalf("failure id=%d error=%v", queue.failedID, queue.failure)
	}
}

func TestWorkerIgnoresEmptyQueue(t *testing.T) {
	queue := &queueStub{claimErr: ErrNotFound}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue, processorStub{}, WorkerOptions{
		PollInterval: time.Second, RetryDelay: time.Second, LeaseDuration: time.Minute, MaxAttempts: 3,
	})
	if err := worker.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce() error = %v", err)
	}
}

func TestWorkerPersistsFailureAfterCancellation(t *testing.T) {
	queue := &queueStub{item: AnalysisJob{ID: 11}}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue,
		processorStub{err: context.Canceled}, WorkerOptions{
			PollInterval: time.Second, RetryDelay: time.Second, LeaseDuration: time.Minute, MaxAttempts: 3,
		})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := worker.runOnce(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("runOnce() error=%v, want context.Canceled", err)
	}
	if queue.failedID != 11 || queue.persistContextErr != nil {
		t.Fatalf("failure was not persisted with a live context: id=%d contextErr=%v", queue.failedID, queue.persistContextErr)
	}
}
