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
	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/github"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/impact"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/logging"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/provenance"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/repair"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/storage"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

func main() {
	bootstrapLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	logger := logging.New(cfg.LogLevel)
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
	gitHubClient, err := github.NewHTTPClient(cfg.GitHub.APIBaseURL, cfg.GitHub.Token, cfg.GitHub.RequestTimeout)
	if err != nil {
		logger.Error("configure GitHub client", "error", err)
		os.Exit(1)
	}
	sourceClient, err := scm.NewRouter(map[string]scm.Client{
		scm.ProviderGitLab: gitLabClient,
		scm.ProviderGitHub: gitHubClient,
	})
	if err != nil {
		logger.Error("configure source providers", "error", err)
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
		Model: cfg.LLM.Model, FallbackModels: cfg.LLM.FallbackModels,
		RequestTimeout:  cfg.LLM.RequestTimeout,
		MaxOutputTokens: cfg.LLM.MaxOutputTokens,
	})
	if err != nil {
		logger.Error("configure LLM provider", "error", err)
		os.Exit(1)
	}
	sourceProcessor := analysis.NewProcessor(projectRepository, sourceClient, jobRepository)
	impactEngine, err := analyzer.NewImpactEngine(analyzer.DefaultImpactOptions())
	if err != nil {
		logger.Error("configure impact analyzer", "error", err)
		os.Exit(1)
	}
	impactRepository := impact.NewRepository(database.Pool())
	changeProcessor := analyzer.NewProcessorWithImpact(projectRepository, sourceClient,
		jobRepository, impactEngine, impactRepository)
	indexProcessor := knowledge.NewIndexer(projectRepository, sourceClient, embedder, knowledgeRepository)
	recommendationRepository := recommendation.NewRepository(database.Pool())
	provenanceRepository := provenance.NewRepository(database.Pool(), provenance.RuntimeConfig{
		Provider: cfg.LLM.Provider, Model: cfg.LLM.Model,
		InputCostPerMillionUSD:  cfg.LLM.InputCostPerMillionUSD,
		OutputCostPerMillionUSD: cfg.LLM.OutputCostPerMillionUSD,
	})
	recommendationProcessor := recommendation.NewProcessorWithProvenance(jobRepository,
		knowledge.NewRetriever(knowledgeRepository, embedder), llmProvider,
		recommendationRepository, provenanceRepository)
	generationRepository := generation.NewRepository(database.Pool())
	generationProcessor := generation.NewProcessorWithProvenance(jobRepository, recommendationRepository,
		knowledge.NewRetriever(knowledgeRepository, embedder), llmProvider,
		generationRepository, provenanceRepository)
	sandboxRunner, err := validation.NewDockerRunner(validation.DockerConfig{
		Image: cfg.Sandbox.Image, Timeout: cfg.Sandbox.Timeout, MemoryMB: cfg.Sandbox.MemoryMB,
		CPUs: cfg.Sandbox.CPULimit, PIDsLimit: cfg.Sandbox.PIDsLimit,
	})
	if err != nil {
		logger.Error("configure Docker sandbox", "error", err)
		os.Exit(1)
	}
	validationRepository := validation.NewRepository(database.Pool())
	validationProcessor := validation.NewProcessor(projectRepository, generationRepository,
		validation.NewWorkspaceManager(sourceClient, validation.WorkspaceOptions{}),
		sandboxRunner, validationRepository, cfg.Sandbox.Timeout)
	repairRepository := repair.NewRepository(database.Pool())
	repairProcessor := repair.NewProcessorWithProvenance(jobRepository, recommendationRepository,
		generationRepository, validationRepository,
		knowledge.NewRetriever(knowledgeRepository, embedder), llmProvider,
		repairRepository, cfg.Repair.MaxAttempts, provenanceRepository)
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
		job.NewWorker(logger.With("phase", "generation"), generationRepository,
			generationProcessor, options),
		job.NewWorker(logger.With("phase", "validation"), validationRepository,
			validationProcessor, options),
		job.NewWorker(logger.With("phase", "repair"), repairRepository,
			repairProcessor, options),
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
