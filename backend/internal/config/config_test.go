package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("DATABASE_MAX_CONNS", "7")
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("GITLAB_BASE_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_WEBHOOK_SECRET", "test-secret")
	t.Setenv("WORKER_POLL_INTERVAL", "250ms")
	t.Setenv("WORKER_RETRY_DELAY", "500ms")
	t.Setenv("WORKER_LEASE_DURATION", "1m")
	t.Setenv("WORKER_MAX_ATTEMPTS", "4")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.DatabaseMaxConn != 7 || cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("Load() = %+v", cfg)
	}
	if cfg.GitLab.BaseURL != "https://gitlab.example.com" || cfg.GitLab.Token != "test-token" ||
		cfg.Worker.PollInterval != 250*time.Millisecond || cfg.Worker.RetryDelay != 500*time.Millisecond ||
		cfg.Worker.LeaseDuration != time.Minute || cfg.Worker.MaxAttempts != 4 {
		t.Fatalf("Load() GitLab/worker config = %+v", cfg)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want required database error")
	}
}

func TestLoadRejectsInvalidMaxConnections(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("DATABASE_MAX_CONNS", "zero")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
}

func TestLoadRejectsInvalidWorkerConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("WORKER_MAX_ATTEMPTS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want worker validation error")
	}
}
