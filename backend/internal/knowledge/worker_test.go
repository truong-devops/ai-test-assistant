package knowledge

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type indexQueueStub struct {
	item              IndexJob
	claimErr          error
	failure           error
	persistContextErr error
	renewCount        int
	renewErr          error
}

func (q *indexQueueStub) RenewLease(context.Context, IndexJob, time.Duration) error {
	q.renewCount++
	return q.renewErr
}

func (q *indexQueueStub) ClaimNext(context.Context, time.Duration) (IndexJob, error) {
	return q.item, q.claimErr
}
func (q *indexQueueStub) RetryOrFail(ctx context.Context, _ IndexJob, processErr error, _ int, _ time.Duration) error {
	q.failure, q.persistContextErr = processErr, ctx.Err()
	return nil
}

type indexProcessorStub struct{ err error }

func (p indexProcessorStub) Process(context.Context, IndexJob) error { return p.err }

type waitingIndexProcessor struct{ duration time.Duration }

func (p waitingIndexProcessor) Process(ctx context.Context, _ IndexJob) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(p.duration):
		return nil
	}
}

func TestIndexWorkerPersistsFailureAfterCancellation(t *testing.T) {
	queue := &indexQueueStub{item: IndexJob{ProjectID: 3}}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue,
		indexProcessorStub{err: context.Canceled}, WorkerOptions{
			PollInterval: time.Second, RetryDelay: time.Second, LeaseDuration: time.Minute, MaxAttempts: 3,
		})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := worker.runOnce(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("runOnce() error=%v", err)
	}
	if !errors.Is(queue.failure, context.Canceled) || queue.persistContextErr != nil {
		t.Fatalf("failure=%v persistContextErr=%v", queue.failure, queue.persistContextErr)
	}
}

func TestIndexWorkerIgnoresEmptyQueue(t *testing.T) {
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)),
		&indexQueueStub{claimErr: ErrIndexNotFound}, indexProcessorStub{}, WorkerOptions{
			PollInterval: time.Second, RetryDelay: time.Second, LeaseDuration: time.Minute, MaxAttempts: 3,
		})
	if err := worker.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestIndexWorkerRenewsLeaseDuringLongIndex(t *testing.T) {
	queue := &indexQueueStub{item: IndexJob{ProjectID: 4, Generation: 1, AttemptCount: 1}}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue,
		waitingIndexProcessor{duration: 250 * time.Millisecond}, WorkerOptions{
			PollInterval: time.Second, RetryDelay: time.Second,
			LeaseDuration: 300 * time.Millisecond, MaxAttempts: 3,
		})
	if err := worker.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.renewCount < 2 {
		t.Fatalf("lease renew count=%d, want at least 2", queue.renewCount)
	}
}
