package job

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type Processor interface {
	Process(ctx context.Context, analysis AnalysisJob) error
}

type Queue interface {
	ClaimNext(ctx context.Context, leaseDuration time.Duration) (AnalysisJob, error)
	RetryOrFail(ctx context.Context, id int64, expectedAttempt int, processErr error, maxAttempts int, retryDelay time.Duration) error
}

type WorkerOptions struct {
	PollInterval  time.Duration
	RetryDelay    time.Duration
	LeaseDuration time.Duration
	MaxAttempts   int
}

type Worker struct {
	logger     *slog.Logger
	repository Queue
	processor  Processor
	options    WorkerOptions
}

func NewWorker(logger *slog.Logger, repository Queue, processor Processor, options WorkerOptions) *Worker {
	return &Worker{logger: logger, repository: repository, processor: processor, options: options}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := w.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("worker iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) error {
	analysis, err := w.repository.ClaimNext(ctx, w.options.LeaseDuration)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	w.logger.Info("processing analysis job", "analysis_job_id", analysis.ID,
		"project_id", analysis.ProjectID, "merge_request_iid", analysis.MergeRequestIID)
	if err := w.processor.Process(ctx, analysis); err != nil {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if failErr := w.repository.RetryOrFail(persistCtx, analysis.ID, analysis.AttemptCount, err, w.options.MaxAttempts, w.options.RetryDelay); failErr != nil {
			w.logger.Error("could not persist job failure", "analysis_job_id", analysis.ID, "error", failErr)
		}
		return err
	}
	return nil
}
