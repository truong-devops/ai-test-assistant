package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultHTTPAddr        = ":8080"
	defaultDatabaseMaxConn = int32(10)
	defaultShutdownTimeout = 10 * time.Second
)

type Config struct {
	AppEnv          string
	HTTPAddr        string
	DatabaseURL     string
	DatabaseMaxConn int32
	ShutdownTimeout time.Duration
	GitLab          GitLabConfig
	Embedding       EmbeddingConfig
	LLM             LLMConfig
	Worker          WorkerConfig
}

type GitLabConfig struct {
	BaseURL        string
	Token          string
	WebhookSecret  string
	RequestTimeout time.Duration
}

type WorkerConfig struct {
	PollInterval  time.Duration
	RetryDelay    time.Duration
	LeaseDuration time.Duration
	MaxAttempts   int
}

type EmbeddingConfig struct {
	Provider string
	Model    string
}

type LLMConfig struct {
	Provider        string
	BaseURL         string
	APIKey          string
	Model           string
	RequestTimeout  time.Duration
	MaxOutputTokens int
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:          envOrDefault("APP_ENV", "development"),
		HTTPAddr:        envOrDefault("HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		DatabaseMaxConn: defaultDatabaseMaxConn,
		ShutdownTimeout: defaultShutdownTimeout,
		GitLab: GitLabConfig{
			BaseURL:        envOrDefault("GITLAB_BASE_URL", "https://gitlab.com"),
			Token:          os.Getenv("GITLAB_TOKEN"),
			WebhookSecret:  os.Getenv("GITLAB_WEBHOOK_SECRET"),
			RequestTimeout: 15 * time.Second,
		},
		Embedding: EmbeddingConfig{
			Provider: envOrDefault("EMBEDDING_PROVIDER", "local"),
			Model:    envOrDefault("EMBEDDING_MODEL", "hash-v1"),
		},
		LLM: LLMConfig{
			Provider: envOrDefault("LLM_PROVIDER", "disabled"),
			BaseURL:  envOrDefault("LLM_BASE_URL", "https://api.openai.com/v1"),
			APIKey:   os.Getenv("LLM_API_KEY"), Model: os.Getenv("LLM_MODEL"),
			RequestTimeout: 60 * time.Second, MaxOutputTokens: 2000,
		},
		Worker: WorkerConfig{
			PollInterval:  2 * time.Second,
			RetryDelay:    5 * time.Second,
			LeaseDuration: 5 * time.Minute,
			MaxAttempts:   3,
		},
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	if value := os.Getenv("DATABASE_MAX_CONNS"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("DATABASE_MAX_CONNS must be a positive integer")
		}
		cfg.DatabaseMaxConn = int32(parsed)
	}

	if value := os.Getenv("SHUTDOWN_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT must be a positive duration")
		}
		cfg.ShutdownTimeout = parsed
	}

	if value := os.Getenv("GITLAB_REQUEST_TIMEOUT"); value != "" {
		parsed, err := positiveDuration("GITLAB_REQUEST_TIMEOUT", value)
		if err != nil {
			return Config{}, err
		}
		cfg.GitLab.RequestTimeout = parsed
	}
	if value := os.Getenv("WORKER_POLL_INTERVAL"); value != "" {
		parsed, err := positiveDuration("WORKER_POLL_INTERVAL", value)
		if err != nil {
			return Config{}, err
		}
		cfg.Worker.PollInterval = parsed
	}
	if value := os.Getenv("WORKER_RETRY_DELAY"); value != "" {
		parsed, err := positiveDuration("WORKER_RETRY_DELAY", value)
		if err != nil {
			return Config{}, err
		}
		cfg.Worker.RetryDelay = parsed
	}
	if value := os.Getenv("WORKER_LEASE_DURATION"); value != "" {
		parsed, err := positiveDuration("WORKER_LEASE_DURATION", value)
		if err != nil {
			return Config{}, err
		}
		cfg.Worker.LeaseDuration = parsed
	}
	if value := os.Getenv("WORKER_MAX_ATTEMPTS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 20 {
			return Config{}, fmt.Errorf("WORKER_MAX_ATTEMPTS must be between 1 and 20")
		}
		cfg.Worker.MaxAttempts = parsed
	}
	if value := os.Getenv("LLM_REQUEST_TIMEOUT"); value != "" {
		parsed, err := positiveDuration("LLM_REQUEST_TIMEOUT", value)
		if err != nil {
			return Config{}, err
		}
		cfg.LLM.RequestTimeout = parsed
	}
	if value := os.Getenv("LLM_MAX_OUTPUT_TOKENS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 100 || parsed > 10000 {
			return Config{}, fmt.Errorf("LLM_MAX_OUTPUT_TOKENS must be between 100 and 10000")
		}
		cfg.LLM.MaxOutputTokens = parsed
	}

	return cfg, nil
}

func positiveDuration(name, value string) (time.Duration, error) {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
