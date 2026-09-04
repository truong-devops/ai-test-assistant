package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIProviderGenerateUsesStructuredResponsesAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer test-key" {
			t.Fatalf("Authorization = %q", authorization)
		}
		var request openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "test-model" || request.Store || request.MaxOutputTokens != 900 ||
			request.Text.Format.Type != "json_schema" || !request.Text.Format.Strict ||
			request.Text.Format.Name != "fixture" {
			t.Fatalf("request = %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"resp_1","model":"test-model-2026","status":"completed",`+
			`"output":[{"type":"reasoning"},{"type":"message","content":[`+
			`{"type":"output_text","text":"{\"recommendations\":[]}"}]}],`+
			`"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19}}`)
	}))
	defer server.Close()
	provider, err := NewOpenAIProvider(server.URL+"/v1", "test-key", "test-model", time.Second, 900)
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), Request{
		Instructions: "instructions", Input: "input", SchemaName: "fixture",
		Schema: map[string]any{"type": "object"}, MaxOutputTokens: 1200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.ID != "resp_1" || response.Model != "test-model-2026" ||
		response.Output != `{"recommendations":[]}` || response.Usage.TotalTokens != 19 {
		t.Fatalf("response = %#v", response)
	}
}

func TestOpenAIProviderRejectsProviderFailuresAndMalformedResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{name: "http error", status: http.StatusTooManyRequests, body: `{"error":"limited"}`},
		{name: "invalid json", status: http.StatusOK, body: `{`, want: ErrMalformedResponse},
		{name: "incomplete", status: http.StatusOK, body: `{"id":"r","model":"m","status":"incomplete"}`, want: ErrMalformedResponse},
		{name: "empty output", status: http.StatusOK, body: `{"id":"r","model":"m","status":"completed","output":[]}`, want: ErrMalformedResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			provider, err := NewOpenAIProvider(server.URL, "key", "model", time.Second, 200)
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Generate(context.Background(), Request{
				Instructions: "i", Input: "x", SchemaName: "s", Schema: map[string]any{"type": "object"},
			})
			if err == nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("Generate() error=%v, want %v", err, test.want)
			}
		})
	}
}

func TestNewProviderConfiguration(t *testing.T) {
	provider, err := NewProvider(Config{Provider: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Generate(context.Background(), Request{}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled provider error=%v", err)
	}
	for _, config := range []Config{
		{Provider: "openai", Model: "model"},
		{Provider: "openai", APIKey: "key"},
		{Provider: "gemini", Model: "model"},
		{Provider: "gemini", APIKey: "key"},
		{Provider: "unknown"},
	} {
		if _, err := NewProvider(config); err == nil || !strings.Contains(err.Error(), "LLM") && config.Provider != "unknown" {
			t.Fatalf("NewProvider(%#v) error=%v", config, err)
		}
	}
	provider, err = NewProvider(Config{Provider: "gemini", APIKey: "key", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(*GeminiProvider); !ok {
		t.Fatalf("gemini provider = %T", provider)
	}
}
