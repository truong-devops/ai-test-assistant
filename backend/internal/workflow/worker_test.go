package workflow

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type workflowQueueStub struct {
	job          Job
	claimErr     error
	heartbeatErr error
	completeErr  error
	completed    int
	failed       int
	failureCode  string
	retryable    bool
}

func (q *workflowQueueStub) ClaimNext(context.Context, time.Duration) (Job, error) {
	return q.job, q.claimErr
}
func (q *workflowQueueStub) Heartbeat(context.Context, Job, time.Duration) error {
	return q.heartbeatErr
}
func (q *workflowQueueStub) Complete(context.Context, Job, any) error {
	q.completed++
	return q.completeErr
}

func TestWorkerFinalizesConcurrentCancelWhenPublicationLosesLease(t *testing.T) {
	queue := &workflowQueueStub{job: testWorkflowJob(), completeErr: ErrLeaseLost}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue,
		workflowProcessorStub{output: map[string]any{"created_count": 2}}, WorkerOptions{
			PollInterval: time.Millisecond, RetryDelay: time.Millisecond,
			LeaseDuration: time.Second, ProcessTimeout: time.Second})
	err := worker.runOnce(context.Background())
	if !errors.Is(err, ErrLeaseLost) || queue.completed != 1 || queue.failed != 1 {
		t.Fatalf("error=%v completed=%d failed=%d", err, queue.completed, queue.failed)
	}
}
func (q *workflowQueueStub) Fail(_ context.Context, _ Job, _ error, code string,
	retryable bool, _ time.Duration,
) error {
	q.failed++
	q.failureCode, q.retryable = code, retryable
	return nil
}

type workflowProcessorStub struct {
	output any
	err    error
}

func (p workflowProcessorStub) Process(context.Context, Job) (any, error) {
	return p.output, p.err
}

func testWorkflowJob() Job {
	return Job{ID: 1, DocumentSetID: 2, Operation: OperationGenerate,
		Status: StatusRunning, AttemptCount: 1, MaxAttempts: 3,
		Units: []Unit{{ID: 3, WorkflowJobID: 1, AttemptCount: 1}}}
}

func TestWorkerCompletesOneDurableAttempt(t *testing.T) {
	queue := &workflowQueueStub{job: testWorkflowJob()}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue,
		workflowProcessorStub{output: map[string]any{"created_count": 2}}, WorkerOptions{
			PollInterval: time.Millisecond, RetryDelay: time.Millisecond,
			LeaseDuration: time.Second, ProcessTimeout: time.Second})
	if err := worker.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.completed != 1 || queue.failed != 0 {
		t.Fatalf("completed=%d failed=%d", queue.completed, queue.failed)
	}
}

func TestWorkerDoesNotRetryStaleInput(t *testing.T) {
	queue := &workflowQueueStub{job: testWorkflowJob()}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue,
		workflowProcessorStub{err: ErrInputStale}, WorkerOptions{
			PollInterval: time.Millisecond, RetryDelay: time.Millisecond,
			LeaseDuration: time.Second, ProcessTimeout: time.Second})
	err := worker.runOnce(context.Background())
	if !errors.Is(err, ErrInputStale) || queue.failed != 1 || queue.retryable ||
		queue.failureCode != "INPUT_SNAPSHOT_STALE" {
		t.Fatalf("error=%v failed=%d retryable=%v code=%s", err, queue.failed,
			queue.retryable, queue.failureCode)
	}
}
