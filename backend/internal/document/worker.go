package document

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type ParseQueue interface {
	ClaimNext(context.Context, time.Duration) (Version, error)
	RenewLease(context.Context, Version, time.Duration) error
	RetryOrFail(context.Context, Version, error, int, time.Duration) error
}

type VersionProcessor interface {
	Process(context.Context, Version) error
}

type WorkerOptions struct {
	PollInterval  time.Duration
	RetryDelay    time.Duration
	LeaseDuration time.Duration
	ParseTimeout  time.Duration
	MaxAttempts   int
}

type Worker struct {
	logger    *slog.Logger
	queue     ParseQueue
	processor VersionProcessor
	options   WorkerOptions
}

func NewWorker(logger *slog.Logger, queue ParseQueue, processor VersionProcessor,
	options WorkerOptions,
) *Worker {
	return &Worker{logger: logger, queue: queue, processor: processor, options: options}
}

func (w *Worker) Run(ctx context.Context) error {
	if err := validateWorkerOptions(w.options); err != nil {
		return err
	}
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := w.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("document worker iteration failed", "error", err)
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
	w.logger.Info("processing document", "document_version_id", claimed.ID,
		"document_id", claimed.DocumentID, "attempt", claimed.AttemptCount)
	processingCtx, cancel := context.WithTimeout(ctx, w.options.ParseTimeout)
	renewalDone := make(chan error, 1)
	go func() {
		renewalDone <- w.renewLease(processingCtx, claimed, cancel)
	}()
	processErr := w.processor.Process(processingCtx, claimed)
	cancel()
	renewalErr := <-renewalDone
	if processErr == nil {
		// SaveParsed has already verified and consumed this exact lease. A
		// renewal racing just after that commit can observe PARSED and report a
		// harmless lease loss; the committed parse result remains authoritative.
		return nil
	}
	if renewalErr != nil {
		processErr = errors.Join(processErr, renewalErr)
	}
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer persistCancel()
	if err := w.queue.RetryOrFail(persistCtx, claimed, processErr,
		w.options.MaxAttempts, w.options.RetryDelay); err != nil {
		return errors.Join(processErr, fmt.Errorf("persist document parse failure: %w", err))
	}
	return processErr
}

func (w *Worker) renewLease(ctx context.Context, claimed Version, cancel context.CancelFunc) error {
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
			err := w.queue.RenewLease(renewCtx, claimed, w.options.LeaseDuration)
			renewCancel()
			if err != nil {
				cancel()
				return err
			}
		}
	}
}

func validateWorkerOptions(options WorkerOptions) error {
	if options.PollInterval <= 0 || options.RetryDelay <= 0 || options.LeaseDuration <= 0 ||
		options.ParseTimeout <= 0 || options.MaxAttempts < 1 || options.MaxAttempts > 20 {
		return fmt.Errorf("invalid document worker options")
	}
	return nil
}
