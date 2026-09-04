package config

import "testing"

func TestLoadLeavesLLMBaseURLEmptyForProviderDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("LLM_BASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.BaseURL != "" {
		t.Fatalf("LLM base URL = %q, want provider default", cfg.LLM.BaseURL)
	}
}
