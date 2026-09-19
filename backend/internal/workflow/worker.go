package workflow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/aibudget"
)

type Queue interface {
	ClaimNext(context.Context, time.Duration) (Job, error)
	Heartbeat(context.Context, Job, time.Duration) error
	Complete(context.Context, Job, any) error
	Fail(context.Context, Job, error, string, bool, time.Duration) error
}

type Processor interface {
	Process(context.Context, Job) (any, error)
}

type WorkerOptions struct {
	PollInterval   time.Duration
	RetryDelay     time.Duration
	LeaseDuration  time.Duration
	ProcessTimeout time.Duration
}

type Worker struct {
	logger    *slog.Logger
	queue     Queue
	processor Processor
	options   WorkerOptions
}

func NewWorker(logger *slog.Logger, queue Queue, processor Processor,
	options WorkerOptions,
) *Worker {
	return &Worker{logger: logger, queue: queue, processor: processor, options: options}
}

func (w *Worker) Run(ctx context.Context) error {
	if w.options.PollInterval <= 0 || w.options.RetryDelay <= 0 ||
		w.options.LeaseDuration <= 0 || w.options.ProcessTimeout <= 0 {
		return fmt.Errorf("invalid document workflow worker options")
	}
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := w.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("document workflow worker iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) error {
	claimed, err := w.queue.ClaimNext(ctx, w.options.LeaseDuration)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(claimed.Units) != 1 {
		return w.queue.Fail(context.WithoutCancel(ctx), claimed, ErrInvalidInput,
			"INVALID_JOB_UNITS", false, w.options.RetryDelay)
	}
	w.logger.Info("processing document workflow job", "job_id", claimed.ID,
		"document_set_id", claimed.DocumentSetID, "operation", claimed.Operation,
		"attempt", claimed.AttemptCount)
	processingCtx, cancel := context.WithTimeout(ctx, w.options.ProcessTimeout)
	processingCtx = aibudget.WithWorkflowAttempt(processingCtx, claimed.ID,
		claimed.Units[0].ID, claimed.Units[0].AttemptCount)
	renewalDone := make(chan error, 1)
	go func() { renewalDone <- w.renewLease(processingCtx, claimed, cancel) }()
	output, processErr := w.processor.Process(processingCtx, claimed)
	cancel()
	renewalErr := <-renewalDone
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer persistCancel()
	if processErr == nil && renewalErr == nil {
		if completeErr := w.queue.Complete(persistCtx, claimed, output); completeErr == nil {
			return nil
		} else {
			// A concurrent cancel can win after processing and before publication.
			// Route the failed publish through Fail so the durable job reaches
			// CANCELED instead of remaining RUNNING forever.
			processErr = fmt.Errorf("complete document workflow job: %w", completeErr)
		}
	}
	processErr = errors.Join(processErr, renewalErr)
	code, retryable := classifyError(processErr)
	if err := w.queue.Fail(persistCtx, claimed, processErr, code, retryable,
		w.options.RetryDelay); err != nil {
		return errors.Join(processErr, fmt.Errorf("persist document workflow failure: %w", err))
	}
	return processErr
}

func (w *Worker) renewLease(ctx context.Context, claimed Job,
	cancel context.CancelFunc,
) error {
	interval := w.options.LeaseDuration / 3
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			renewCtx, renewCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			err := w.queue.Heartbeat(renewCtx, claimed, w.options.LeaseDuration)
			renewCancel()
			if err != nil {
				cancel()
				return err
			}
		}
	}
}
