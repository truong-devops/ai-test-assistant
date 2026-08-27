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
	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/httpapi"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/storage"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	database, err := storage.Open(startupCtx, cfg.DatabaseURL, cfg.DatabaseMaxConn)
	cancel()
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	projectRepository := project.NewPostgresRepository(database.Pool())
	projectService := project.NewService(projectRepository)
	jobRepository := job.NewRepository(database.Pool())
	analysisService := job.NewService(jobRepository)
	knowledgeRepository := knowledge.NewRepository(database.Pool())
	knowledgeService := knowledge.NewService(projectRepository, knowledgeRepository)
	recommendationRepository := recommendation.NewRepository(database.Pool())
	recommendationService := recommendation.NewService(jobRepository, recommendationRepository)
	generationRepository := generation.NewRepository(database.Pool())
	generationService := generation.NewService(jobRepository, generationRepository)
	validationRepository := validation.NewRepository(database.Pool())
	validationService := validation.NewService(jobRepository, validationRepository)
	webhookService := gitlab.NewWebhookService(projectRepository, jobRepository)
	webhookHandler := gitlab.NewWebhookHandler(cfg.GitLab.WebhookSecret, webhookService)
	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouterWithPhaseSevenServices(logger, database, projectService, analysisService,
			webhookHandler, knowledgeService, recommendationService, generationService, validationService),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
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
