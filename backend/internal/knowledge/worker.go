package knowledge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Queue interface {
	ClaimNext(ctx context.Context, leaseDuration time.Duration) (IndexJob, error)
	RetryOrFail(ctx context.Context, claimed IndexJob, processErr error, maxAttempts int, retryDelay time.Duration) error
	RenewLease(ctx context.Context, claimed IndexJob, leaseDuration time.Duration) error
}

type Processor interface {
	Process(ctx context.Context, claimed IndexJob) error
}

type WorkerOptions struct {
	PollInterval  time.Duration
	RetryDelay    time.Duration
	LeaseDuration time.Duration
	MaxAttempts   int
}

type Worker struct {
	logger    *slog.Logger
	queue     Queue
	processor Processor
	options   WorkerOptions
}

func NewWorker(logger *slog.Logger, queue Queue, processor Processor, options WorkerOptions) *Worker {
	return &Worker{logger: logger, queue: queue, processor: processor, options: options}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := w.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("index worker iteration failed", "error", err)
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
	if errors.Is(err, ErrIndexNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	w.logger.Info("indexing project", "project_id", claimed.ProjectID,
		"ref", claimed.Ref, "generation", claimed.Generation)
	processingCtx, cancelProcessing := context.WithCancel(ctx)
	renewalDone := make(chan error, 1)
	go func() {
		renewalDone <- w.renewLease(processingCtx, claimed, cancelProcessing)
	}()
	processErr := w.processor.Process(processingCtx, claimed)
	cancelProcessing()
	renewalErr := <-renewalDone
	if processErr != nil && renewalErr != nil {
		processErr = renewalErr
	}
	if processErr != nil {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if failErr := w.queue.RetryOrFail(persistCtx, claimed, processErr,
			w.options.MaxAttempts, w.options.RetryDelay); failErr != nil {
			w.logger.Error("could not persist index failure", "project_id", claimed.ProjectID, "error", failErr)
		}
		return processErr
	}
	return nil
}

func (w *Worker) renewLease(ctx context.Context, claimed IndexJob, cancelProcessing context.CancelFunc) error {
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
			renewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			err := w.queue.RenewLease(renewCtx, claimed, w.options.LeaseDuration)
			cancel()
			if err != nil {
				cancelProcessing()
				return fmt.Errorf("renew index lease: %w", err)
			}
		}
	}
}
