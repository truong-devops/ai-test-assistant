//go:build provider_smoke

package llm

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// This explicit, potentially billed lane never runs in the ordinary unit suite.
func TestGeminiLiveSmoke(t *testing.T) {
	if os.Getenv("RUN_REAL_PROVIDER_SMOKE") != "yes" {
		t.Skip("set RUN_REAL_PROVIDER_SMOKE=yes to authorize a real provider request")
	}
	keyPath := strings.TrimSpace(os.Getenv("LLM_API_KEY_FILE"))
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))
	if keyPath == "" || model == "" {
		t.Fatal("LLM_API_KEY_FILE and LLM_MODEL are required; no mock fallback")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil || len(strings.TrimSpace(string(key))) == 0 {
		t.Fatal("provider key file is unavailable or empty")
	}
	// Fixed official destination: no credentials can be redirected by a base URL override.
	provider, err := NewGeminiProvider(defaultGeminiBaseURL, strings.TrimSpace(string(key)), model, 45*time.Second, 512)
	if err != nil {
		t.Fatal("invalid provider smoke configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	response, err := provider.Generate(ctx, Request{Instructions: "Return only the requested JSON object.", Input: "Return status ok.", SchemaName: "provider_smoke", MaxOutputTokens: 512,
		Schema: map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string", "enum": []string{"ok"}}}, "required": []string{"status"}, "additionalProperties": false}})
	if err != nil {
		t.Fatalf("real provider smoke failed (%T); no provider body or credential is logged", err)
	}
	var result struct {
		Status string `json:"status"`
	}
	if json.Unmarshal([]byte(response.Output), &result) != nil || result.Status != "ok" || response.Model == "" {
		t.Fatal("provider returned invalid smoke result")
	}
	t.Logf("real Gemini smoke completed; input_tokens=%d output_tokens=%d (not a full document/SCM/sandbox demonstration)", response.Usage.InputTokens, response.Usage.OutputTokens)
}
