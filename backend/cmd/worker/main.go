package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/analysis"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/analyzer"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/config"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
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
	knowledgeRepository := knowledge.NewRepository(database.Pool())
	embedder, err := knowledge.NewEmbeddingClient(cfg.Embedding.Provider, cfg.Embedding.Model)
	if err != nil {
		logger.Error("configure embedding client", "error", err)
		os.Exit(1)
	}
	llmProvider, err := llm.NewProvider(llm.Config{
		Provider: cfg.LLM.Provider, BaseURL: cfg.LLM.BaseURL, APIKey: cfg.LLM.APIKey,
		Model: cfg.LLM.Model, RequestTimeout: cfg.LLM.RequestTimeout,
		MaxOutputTokens: cfg.LLM.MaxOutputTokens,
	})
	if err != nil {
		logger.Error("configure LLM provider", "error", err)
		os.Exit(1)
	}
	sourceProcessor := analysis.NewProcessor(projectRepository, gitLabClient, jobRepository)
	changeProcessor := analyzer.NewProcessor(projectRepository, gitLabClient, jobRepository, jobRepository)
	indexProcessor := knowledge.NewIndexer(projectRepository, gitLabClient, embedder, knowledgeRepository)
	recommendationRepository := recommendation.NewRepository(database.Pool())
	recommendationProcessor := recommendation.NewProcessor(jobRepository,
		knowledge.NewRetriever(knowledgeRepository, embedder), llmProvider, recommendationRepository)
	options := job.WorkerOptions{
		PollInterval:  cfg.Worker.PollInterval,
		RetryDelay:    cfg.Worker.RetryDelay,
		LeaseDuration: cfg.Worker.LeaseDuration,
		MaxAttempts:   cfg.Worker.MaxAttempts,
	}
	type runner interface{ Run(context.Context) error }
	workers := []runner{
		job.NewWorker(logger.With("phase", "source"), jobRepository, sourceProcessor, options),
		job.NewWorker(logger.With("phase", "change"), job.NewChangeQueue(jobRepository), changeProcessor, options),
		knowledge.NewWorker(logger.With("phase", "index"), knowledgeRepository, indexProcessor,
			knowledge.WorkerOptions{
				PollInterval: options.PollInterval, RetryDelay: options.RetryDelay,
				LeaseDuration: options.LeaseDuration, MaxAttempts: options.MaxAttempts,
			}),
		job.NewWorker(logger.With("phase", "recommendation"), recommendationRepository,
			recommendationProcessor, options),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("workers started", "poll_interval", cfg.Worker.PollInterval.String(),
		"max_attempts", cfg.Worker.MaxAttempts, "lease_duration", cfg.Worker.LeaseDuration.String())
	var waitGroup sync.WaitGroup
	for _, worker := range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			if err := worker.Run(ctx); err != nil {
				logger.Error("worker stopped with error", "error", err)
				stop()
			}
		}()
	}
	waitGroup.Wait()
}
