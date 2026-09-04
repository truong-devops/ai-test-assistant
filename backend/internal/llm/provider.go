package llm

import (
	"fmt"
	"strings"
	"time"
)

type Config struct {
	Provider        string
	BaseURL         string
	APIKey          string
	Model           string
	RequestTimeout  time.Duration
	MaxOutputTokens int
}

func NewProvider(config Config) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(config.Provider)) {
	case "", "disabled", "none":
		return DisabledProvider{}, nil
	case "openai":
		if strings.TrimSpace(config.APIKey) == "" {
			return nil, fmt.Errorf("LLM_API_KEY is required when LLM_PROVIDER=openai")
		}
		if strings.TrimSpace(config.Model) == "" {
			return nil, fmt.Errorf("LLM_MODEL is required when LLM_PROVIDER=openai")
		}
		return NewOpenAIProvider(config.BaseURL, config.APIKey, config.Model,
			config.RequestTimeout, config.MaxOutputTokens)
	case "gemini":
		if strings.TrimSpace(config.APIKey) == "" {
			return nil, fmt.Errorf("LLM_API_KEY is required when LLM_PROVIDER=gemini")
		}
		if strings.TrimSpace(config.Model) == "" {
			return nil, fmt.Errorf("LLM_MODEL is required when LLM_PROVIDER=gemini")
		}
		return NewGeminiProvider(config.BaseURL, config.APIKey, config.Model,
			config.RequestTimeout, config.MaxOutputTokens)
	default:
		return nil, fmt.Errorf("unsupported LLM provider %q", config.Provider)
	}
}
