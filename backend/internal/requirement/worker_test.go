package requirement

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type extractionQueueStub struct {
	job        ExtractionJob
	claimErr   error
	progress   []ExtractionSummary
	completed  bool
	retryCalls int
}

func (q *extractionQueueStub) ClaimExtraction(context.Context, time.Duration) (ExtractionJob, error) {
	return q.job, q.claimErr
}
func (q *extractionQueueStub) RenewExtractionLease(context.Context, ExtractionJob, time.Duration) error {
	return nil
}
func (q *extractionQueueStub) UpdateExtractionProgress(_ context.Context, _ ExtractionJob, summary ExtractionSummary) error {
	q.progress = append(q.progress, summary)
	return nil
}
func (q *extractionQueueStub) CompleteExtraction(context.Context, ExtractionJob, ExtractionSummary) error {
	q.completed = true
	return nil
}
func (q *extractionQueueStub) RetryOrFailExtraction(context.Context, ExtractionJob, error, int, time.Duration) error {
	q.retryCalls++
	return nil
}

type extractionProcessorStub struct {
	err error
}

func (p extractionProcessorStub) ExtractWithProgress(_ context.Context, setID, _ int64,
	progress func(ExtractionSummary) error,
) (ExtractionSummary, error) {
	summary := ExtractionSummary{DocumentSetID: setID, ChunkCount: 2, ProcessedChunks: 1}
	if err := progress(summary); err != nil {
		return summary, err
	}
	summary.ProcessedChunks = 2
	return summary, p.err
}

func extractionWorkerForTest(queue ExtractionQueue, processor ExtractionProcessor) *Worker {
	return NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue, processor, WorkerOptions{
		PollInterval: time.Second, RetryDelay: time.Second, LeaseDuration: time.Minute,
		ProcessTimeout: time.Second, MaxAttempts: 3,
	})
}

func TestExtractionWorkerCompletesOutsideRequestContext(t *testing.T) {
	queue := &extractionQueueStub{job: ExtractionJob{ID: 1, DocumentSetID: 9, AttemptCount: 1}}
	if err := extractionWorkerForTest(queue, extractionProcessorStub{}).runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !queue.completed || queue.retryCalls != 0 || len(queue.progress) != 1 ||
		queue.progress[0].ProcessedChunks != 1 {
		t.Fatalf("completed=%v retry=%d progress=%+v", queue.completed, queue.retryCalls, queue.progress)
	}
}

func TestExtractionWorkerRetriesTransientProcessingFailure(t *testing.T) {
	queue := &extractionQueueStub{job: ExtractionJob{ID: 1, DocumentSetID: 9, AttemptCount: 1}}
	err := extractionWorkerForTest(queue, extractionProcessorStub{err: errors.New("Gemini timeout")}).runOnce(context.Background())
	if err == nil || queue.completed || queue.retryCalls != 1 {
		t.Fatalf("error=%v completed=%v retry=%d", err, queue.completed, queue.retryCalls)
	}
}

func TestExtractionWorkerTreatsEmptyQueueAsSuccess(t *testing.T) {
	queue := &extractionQueueStub{claimErr: ErrNotFound}
	if err := extractionWorkerForTest(queue, extractionProcessorStub{}).runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}
