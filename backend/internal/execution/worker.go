package execution

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type WorkerOptions struct {
	PollInterval  time.Duration
	RetryDelay    time.Duration
	LeaseDuration time.Duration
	MaxAttempts   int
}

type Worker struct {
	logger     *slog.Logger
	repository *Repository
	processor  *Processor
	options    WorkerOptions
}

func NewWorker(logger *slog.Logger, repository *Repository, processor *Processor, options WorkerOptions) *Worker {
	return &Worker{logger: logger, repository: repository, processor: processor, options: options}
}

func (w *Worker) Run(ctx context.Context) error {
	if w.options.PollInterval <= 0 || w.options.RetryDelay <= 0 || w.options.LeaseDuration <= 0 || w.options.MaxAttempts < 1 {
		return ErrInvalidInput
	}
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := w.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("document execution worker iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) error {
	claimed, err := w.repository.ClaimNext(ctx, w.options.LeaseDuration)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	processingCtx, cancel := context.WithCancel(ctx)
	renewed := make(chan error, 1)
	go func() { renewed <- w.renew(processingCtx, claimed, cancel) }()
	processErr := w.processor.Process(processingCtx, claimed)
	cancel()
	renewalErr := <-renewed
	if processErr == nil {
		return nil
	}
	if renewalErr != nil {
		processErr = errors.Join(processErr, renewalErr)
	}
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer persistCancel()
	if err := w.repository.RetryOrFail(persistCtx, claimed, processErr, w.options.MaxAttempts, w.options.RetryDelay); err != nil {
		return errors.Join(processErr, err)
	}
	return processErr
}

func (w *Worker) renew(ctx context.Context, claimed Run, cancel context.CancelFunc) error {
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
			err := w.repository.RenewLease(renewCtx, claimed, w.options.LeaseDuration)
			renewCancel()
			if err != nil {
				cancel()
				return err
			}
		}
	}
}
