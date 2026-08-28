package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("DATABASE_MAX_CONNS", "7")
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("HTTP_READ_TIMEOUT", "4s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "8s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "12s")
	t.Setenv("HTTP_MAX_HEADER_BYTES", "65536")
	t.Setenv("HTTP_RATE_LIMIT_RPS", "5.5")
	t.Setenv("HTTP_RATE_LIMIT_BURST", "12")
	t.Setenv("HTTP_RATE_LIMIT_MAX_CLIENTS", "500")
	t.Setenv("GITLAB_BASE_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_WEBHOOK_SECRET", "test-secret")
	t.Setenv("WORKER_POLL_INTERVAL", "250ms")
	t.Setenv("WORKER_RETRY_DELAY", "500ms")
	t.Setenv("WORKER_LEASE_DURATION", "1m")
	t.Setenv("WORKER_MAX_ATTEMPTS", "4")
	t.Setenv("EMBEDDING_PROVIDER", "hash")
	t.Setenv("EMBEDDING_MODEL", "fixture-v1")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", "https://llm.example.com/v1")
	t.Setenv("LLM_API_KEY", "llm-key")
	t.Setenv("LLM_MODEL", "fixture-model")
	t.Setenv("LLM_REQUEST_TIMEOUT", "45s")
	t.Setenv("LLM_MAX_OUTPUT_TOKENS", "1500")
	t.Setenv("SANDBOX_IMAGE", "sandbox:test")
	t.Setenv("SANDBOX_TIMEOUT_SECONDS", "45")
	t.Setenv("SANDBOX_MEMORY_MB", "768")
	t.Setenv("SANDBOX_CPU_LIMIT", "1.5")
	t.Setenv("SANDBOX_PIDS_LIMIT", "96")
	t.Setenv("MAX_REPAIR_ATTEMPTS", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.DatabaseMaxConn != 7 || cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("Load() = %+v", cfg)
	}
	if cfg.LogLevel != "warn" || cfg.HTTP.ReadTimeout != 4*time.Second || cfg.HTTP.WriteTimeout != 8*time.Second ||
		cfg.HTTP.IdleTimeout != 12*time.Second || cfg.HTTP.MaxHeaderBytes != 65536 ||
		cfg.HTTP.RateLimitPerSecond != 5.5 || cfg.HTTP.RateLimitBurst != 12 || cfg.HTTP.RateLimitMaxClients != 500 {
		t.Fatalf("Load() HTTP/log config = %+v %+v", cfg.HTTP, cfg.LogLevel)
	}
	if cfg.GitLab.BaseURL != "https://gitlab.example.com" || cfg.GitLab.Token != "test-token" ||
		cfg.Worker.PollInterval != 250*time.Millisecond || cfg.Worker.RetryDelay != 500*time.Millisecond ||
		cfg.Worker.LeaseDuration != time.Minute || cfg.Worker.MaxAttempts != 4 {
		t.Fatalf("Load() GitLab/worker config = %+v", cfg)
	}
	if cfg.Embedding.Provider != "hash" || cfg.Embedding.Model != "fixture-v1" {
		t.Fatalf("Load() embedding config = %+v", cfg.Embedding)
	}
	if cfg.LLM.Provider != "openai" || cfg.LLM.BaseURL != "https://llm.example.com/v1" ||
		cfg.LLM.APIKey != "llm-key" || cfg.LLM.Model != "fixture-model" ||
		cfg.LLM.RequestTimeout != 45*time.Second || cfg.LLM.MaxOutputTokens != 1500 {
		t.Fatalf("Load() LLM config = %+v", cfg.LLM)
	}
	if cfg.Sandbox.Image != "sandbox:test" || cfg.Sandbox.Timeout != 45*time.Second ||
		cfg.Sandbox.MemoryMB != 768 || cfg.Sandbox.CPULimit != 1.5 || cfg.Sandbox.PIDsLimit != 96 {
		t.Fatalf("Load() sandbox config = %+v", cfg.Sandbox)
	}
	if cfg.Repair.MaxAttempts != 3 {
		t.Fatalf("Load() repair config = %+v", cfg.Repair)
	}
}

func TestLoadReadsSecretsFromFiles(t *testing.T) {
	directory := t.TempDir()
	writeSecret := func(name, value string) string {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, key := range []string{"DATABASE_URL", "GITLAB_TOKEN", "GITLAB_WEBHOOK_SECRET", "LLM_API_KEY"} {
		t.Setenv(key, "")
	}
	t.Setenv("DATABASE_URL_FILE", writeSecret("database-url", "postgres://from-file"))
	t.Setenv("GITLAB_TOKEN_FILE", writeSecret("gitlab-token", "gitlab-from-file"))
	t.Setenv("GITLAB_WEBHOOK_SECRET_FILE", writeSecret("webhook-secret", "webhook-from-file"))
	t.Setenv("LLM_API_KEY_FILE", writeSecret("llm-key", "llm-from-file"))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://from-file" || cfg.GitLab.Token != "gitlab-from-file" ||
		cfg.GitLab.WebhookSecret != "webhook-from-file" || cfg.LLM.APIKey != "llm-from-file" {
		t.Fatalf("file secrets were not loaded: %+v", cfg)
	}
}

func TestLoadRejectsAmbiguousSecretAndInvalidHTTPConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://inline")
	t.Setenv("DATABASE_URL_FILE", filepath.Join(t.TempDir(), "database-url"))
	if _, err := Load(); err == nil {
		t.Fatal("Load() error=nil, want ambiguous secret error")
	}
	t.Setenv("DATABASE_URL_FILE", "")
	t.Setenv("HTTP_RATE_LIMIT_BURST", "0")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error=nil, want HTTP rate limit error")
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

func TestLoadRejectsInvalidLLMConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("LLM_MAX_OUTPUT_TOKENS", "99")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want LLM validation error")
	}
}

func TestLoadUsesPhaseSixLLMOutputLimitByDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("LLM_MAX_OUTPUT_TOKENS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.MaxOutputTokens != 6000 {
		t.Fatalf("LLM max output tokens=%d, want 6000", cfg.LLM.MaxOutputTokens)
	}
}

func TestLoadRejectsInvalidSandboxConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("SANDBOX_MEMORY_MB", "32")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want sandbox validation error")
	}
}

func TestLoadRejectsUnboundedRepairConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("MAX_REPAIR_ATTEMPTS", "4")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want repair bound error")
	}
}

func TestLoadUsesBoundedRepairDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("MAX_REPAIR_ATTEMPTS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repair.MaxAttempts != 2 {
		t.Fatalf("repair max attempts=%d, want 2", cfg.Repair.MaxAttempts)
	}
}
