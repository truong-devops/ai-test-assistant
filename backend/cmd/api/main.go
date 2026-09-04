package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/config"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/evaluation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/github"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/httpapi"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/impact"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/logging"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/provenance"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/repair"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/review"
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

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	database, err := storage.Open(startupCtx, cfg.DatabaseURL, cfg.DatabaseMaxConn)
	cancel()
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	projectRepository := project.NewPostgresRepository(database.Pool())
	gitLabSourceClient, err := gitlab.NewHTTPClient(cfg.GitLab.BaseURL, cfg.GitLab.Token, cfg.GitLab.RequestTimeout)
	if err != nil {
		logger.Error("configure GitLab source client", "error", err)
		os.Exit(1)
	}
	gitHubSourceClient, err := github.NewHTTPClient(cfg.GitHub.APIBaseURL, cfg.GitHub.Token, cfg.GitHub.RequestTimeout)
	if err != nil {
		logger.Error("configure GitHub source client", "error", err)
		os.Exit(1)
	}
	sourceResolver, err := scm.NewRouter(map[string]scm.Client{
		scm.ProviderGitLab: gitLabSourceClient,
		scm.ProviderGitHub: gitHubSourceClient,
	})
	if err != nil {
		logger.Error("configure source metadata resolver", "error", err)
		os.Exit(1)
	}
	projectService := project.NewServiceWithResolver(projectRepository, sourceResolver)
	jobRepository := job.NewRepository(database.Pool())
	analysisService := job.NewService(jobRepository)
	knowledgeRepository := knowledge.NewRepository(database.Pool())
	knowledgeService := knowledge.NewService(projectRepository, knowledgeRepository)
	embedder, err := knowledge.NewEmbeddingClient(cfg.Embedding.Provider, cfg.Embedding.Model)
	if err != nil {
		logger.Error("configure embedding client", "error", err)
		os.Exit(1)
	}
	recommendationRepository := recommendation.NewRepository(database.Pool())
	recommendationService := recommendation.NewService(jobRepository, recommendationRepository)
	generationRepository := generation.NewRepository(database.Pool())
	generationService := generation.NewService(jobRepository, generationRepository)
	validationRepository := validation.NewRepository(database.Pool())
	validationService := validation.NewService(jobRepository, validationRepository)
	repairRepository := repair.NewRepository(database.Pool())
	repairService := repair.NewService(jobRepository, repairRepository)
	reviewRepository := review.NewRepository(database.Pool())
	reviewService := review.NewService(jobRepository, reviewRepository)
	contextService := review.NewContextService(jobRepository, knowledge.NewRetriever(knowledgeRepository, embedder))
	evaluationService := evaluation.NewService(evaluation.NewRepository(database.Pool()))
	provenanceRepository := provenance.NewRepository(database.Pool(), provenance.RuntimeConfig{
		Provider: cfg.LLM.Provider, Model: cfg.LLM.Model,
		InputCostPerMillionUSD:  cfg.LLM.InputCostPerMillionUSD,
		OutputCostPerMillionUSD: cfg.LLM.OutputCostPerMillionUSD,
	})
	provenanceService := provenance.NewService(jobRepository, provenanceRepository)
	impactRepository := impact.NewRepository(database.Pool())
	impactService := impact.NewService(jobRepository, impactRepository)
	webhookService := gitlab.NewWebhookService(projectRepository, jobRepository)
	gitLabWebhookHandler := gitlab.NewWebhookHandler(cfg.GitLab.WebhookSecret, webhookService)
	gitHubWebhookService := github.NewWebhookService(projectRepository, jobRepository)
	gitHubWebhookHandler := github.NewWebhookHandler(cfg.GitHub.WebhookSecret, gitHubWebhookService)
	webhookHandler := http.NewServeMux()
	webhookHandler.Handle("POST /api/webhooks/gitlab", gitLabWebhookHandler)
	webhookHandler.Handle("POST /api/webhooks/github", gitHubWebhookHandler)
	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouterWithPhaseThirteenServices(logger, database, projectService, analysisService,
			webhookHandler, knowledgeService, recommendationService, generationService,
			validationService, repairService, reviewService, contextService, evaluationService,
			provenanceService, impactService,
			httpapi.RouterOptions{RateLimitPerSecond: cfg.HTTP.RateLimitPerSecond,
				RateLimitBurst: cfg.HTTP.RateLimitBurst, RateLimitMaxClients: cfg.HTTP.RateLimitMaxClients}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("api started", "address", cfg.HTTPAddr, "environment", cfg.AppEnv)
		serverErrors <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		logger.Info("shutdown requested", "signal", sig.String())
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api server failed", "error", err)
			os.Exit(1)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
