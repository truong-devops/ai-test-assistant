package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/analysis"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/config"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	database, err := storage.Open(startupCtx, cfg.DatabaseURL, cfg.DatabaseMaxConn)
	cancelStartup()
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	gitLabClient, err := gitlab.NewHTTPClient(cfg.GitLab.BaseURL, cfg.GitLab.Token, cfg.GitLab.RequestTimeout)
	if err != nil {
		logger.Error("configure GitLab client", "error", err)
		os.Exit(1)
	}
	projectRepository := project.NewPostgresRepository(database.Pool())
	jobRepository := job.NewRepository(database.Pool())
	processor := analysis.NewProcessor(projectRepository, gitLabClient, jobRepository)
	worker := job.NewWorker(logger, jobRepository, processor, job.WorkerOptions{
		PollInterval:  cfg.Worker.PollInterval,
		RetryDelay:    cfg.Worker.RetryDelay,
		LeaseDuration: cfg.Worker.LeaseDuration,
		MaxAttempts:   cfg.Worker.MaxAttempts,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("worker started", "poll_interval", cfg.Worker.PollInterval.String(),
		"max_attempts", cfg.Worker.MaxAttempts, "lease_duration", cfg.Worker.LeaseDuration.String())
	if err := worker.Run(ctx); err != nil {
		logger.Error("worker stopped with error", "error", err)
		os.Exit(1)
	}
}
