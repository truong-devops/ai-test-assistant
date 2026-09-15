package requirement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type ExtractionQueue interface {
	ClaimExtraction(context.Context, time.Duration) (ExtractionJob, error)
	RenewExtractionLease(context.Context, ExtractionJob, time.Duration) error
	UpdateExtractionProgress(context.Context, ExtractionJob, ExtractionSummary) error
	CompleteExtraction(context.Context, ExtractionJob, ExtractionSummary) error
	RetryOrFailExtraction(context.Context, ExtractionJob, error, int, time.Duration) error
}

type ExtractionProcessor interface {
	ExtractWithProgress(context.Context, int64, int64, func(ExtractionSummary) error) (ExtractionSummary, error)
}

type WorkerOptions struct {
	PollInterval   time.Duration
	RetryDelay     time.Duration
	LeaseDuration  time.Duration
	ProcessTimeout time.Duration
	MaxAttempts    int
}

type Worker struct {
	logger    *slog.Logger
	queue     ExtractionQueue
	processor ExtractionProcessor
	options   WorkerOptions
}

func NewWorker(logger *slog.Logger, queue ExtractionQueue, processor ExtractionProcessor,
	options WorkerOptions,
) *Worker {
	return &Worker{logger: logger, queue: queue, processor: processor, options: options}
}

func (w *Worker) Run(ctx context.Context) error {
	if w.options.PollInterval <= 0 || w.options.RetryDelay <= 0 || w.options.LeaseDuration <= 0 ||
		w.options.ProcessTimeout <= 0 || w.options.MaxAttempts < 1 || w.options.MaxAttempts > 20 {
		return fmt.Errorf("invalid requirement extraction worker options")
	}
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := w.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("requirement extraction worker iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) error {
	claimed, err := w.queue.ClaimExtraction(ctx, w.options.LeaseDuration)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	w.logger.Info("processing requirement extraction", "job_id", claimed.ID,
		"document_set_id", claimed.DocumentSetID, "attempt", claimed.AttemptCount)
	processingCtx, cancel := context.WithTimeout(ctx, w.options.ProcessTimeout)
	renewalDone := make(chan error, 1)
	go func() { renewalDone <- w.renewLease(processingCtx, claimed, cancel) }()
	summary, processErr := w.processor.ExtractWithProgress(processingCtx, claimed.DocumentSetID,
		claimed.IndexGeneration,
		func(progress ExtractionSummary) error {
			persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(processingCtx), 5*time.Second)
			defer persistCancel()
			return w.queue.UpdateExtractionProgress(persistCtx, claimed, progress)
		})
	cancel()
	renewalErr := <-renewalDone
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer persistCancel()
	if processErr == nil && renewalErr == nil {
		return w.queue.CompleteExtraction(persistCtx, claimed, summary)
	}
	processErr = errors.Join(processErr, renewalErr)
	if err := w.queue.RetryOrFailExtraction(persistCtx, claimed, processErr,
		w.options.MaxAttempts, w.options.RetryDelay); err != nil {
		return errors.Join(processErr, fmt.Errorf("persist requirement extraction failure: %w", err))
	}
	return processErr
}

func (w *Worker) renewLease(ctx context.Context, claimed ExtractionJob, cancel context.CancelFunc) error {
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
			err := w.queue.RenewExtractionLease(renewCtx, claimed, w.options.LeaseDuration)
			renewCancel()
			if err != nil {
				cancel()
				return err
			}
		}
	}
}
