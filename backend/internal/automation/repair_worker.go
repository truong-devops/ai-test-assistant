package automation

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type RepairQueue interface {
	ClaimRepair(context.Context, time.Duration) (RepairJob, error)
	RetryOrFailRepair(context.Context, RepairJob, error, int, time.Duration) error
	RenewRepairLease(context.Context, RepairJob, time.Duration) error
}

type RepairJobProcessor interface {
	Process(context.Context, RepairJob) error
}

type RepairWorkerOptions struct {
	PollInterval   time.Duration
	RetryDelay     time.Duration
	LeaseDuration  time.Duration
	ProcessTimeout time.Duration
	MaxAttempts    int
}

type RepairWorker struct {
	logger    *slog.Logger
	queue     RepairQueue
	processor RepairJobProcessor
	options   RepairWorkerOptions
}

func NewRepairWorker(logger *slog.Logger, queue RepairQueue, processor RepairJobProcessor,
	options RepairWorkerOptions,
) *RepairWorker {
	if options.PollInterval <= 0 {
		options.PollInterval = 2 * time.Second
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = 5 * time.Second
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = 5 * time.Minute
	}
	if options.ProcessTimeout <= 0 {
		options.ProcessTimeout = 2 * time.Minute
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 3
	}
	return &RepairWorker{logger: logger, queue: queue, processor: processor, options: options}
}

func (w *RepairWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := w.runOne(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("automation repair worker iteration", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *RepairWorker) runOne(ctx context.Context) error {
	job, err := w.queue.ClaimRepair(ctx, w.options.LeaseDuration)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	processCtx, cancel := context.WithTimeout(ctx, w.options.ProcessTimeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(w.options.LeaseDuration / 2)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-processCtx.Done():
				return
			case <-ticker.C:
				_ = w.queue.RenewRepairLease(processCtx, job, w.options.LeaseDuration)
			}
		}
	}()
	err = w.processor.Process(processCtx, job)
	close(done)
	if err == nil {
		return nil
	}
	return w.queue.RetryOrFailRepair(ctx, job, err, w.options.MaxAttempts, w.options.RetryDelay)
}
