package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestGeminiProviderGenerateUsesStructuredInteractionsAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta/interactions" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if apiKey := r.Header.Get("x-goog-api-key"); apiKey != "test-key" {
			t.Fatalf("x-goog-api-key = %q", apiKey)
		}
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			t.Fatalf("Authorization = %q", authorization)
		}
		var request geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "test-model" || request.Store || request.Input != "input" ||
			request.SystemInstruction != "instructions" || request.GenerationConfig.MaxOutputTokens != 900 ||
			request.ResponseFormat.Type != "text" || request.ResponseFormat.MIMEType != "application/json" ||
			request.ResponseFormat.Schema["type"] != "object" {
			t.Fatalf("request = %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"interaction_1","model":"test-model-2026","status":"completed",`+
			`"steps":[{"type":"thought"},{"type":"model_output","content":[`+
			`{"type":"text","text":"{\"recommendations\":[]}"}]}],`+
			`"usage":{"total_input_tokens":12,"total_output_tokens":7,"total_tokens":25}}`)
	}))
	defer server.Close()
	provider, err := NewGeminiProvider(server.URL+"/v1beta", "test-key", "test-model", time.Second, 900)
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
	if response.ID != "interaction_1" || response.Model != "test-model-2026" ||
		response.Output != `{"recommendations":[]}` || response.Usage.InputTokens != 12 ||
		response.Usage.OutputTokens != 7 || response.Usage.TotalTokens != 25 {
		t.Fatalf("response = %#v", response)
	}
}

func TestGeminiProviderRejectsProviderFailuresAndMalformedResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{name: "http error", status: http.StatusTooManyRequests, body: `{"error":{"message":"limited"}}`},
		{name: "invalid json", status: http.StatusOK, body: `{`, want: ErrMalformedResponse},
		{name: "empty output", status: http.StatusOK, body: `{"id":"r","model":"m","status":"completed","steps":[]}`, want: ErrMalformedResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			provider, err := NewGeminiProvider(server.URL, "key", "model", time.Second, 200)
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

func TestGeminiProviderUsesRequestedModelWhenResponseOmitsModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"interaction-1","status":"completed","steps":[`+
			`{"type":"model_output","content":[{"type":"text","text":"{}"}]}]}`)
	}))
	defer server.Close()

	provider, err := NewGeminiProvider(server.URL, "key", "requested-model", time.Second, 200)
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), Request{
		Instructions: "i", Input: "x", SchemaName: "s", Schema: map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.ID != "interaction-1" || response.Model != "requested-model" {
		t.Fatalf("response=%+v", response)
	}
}

func TestGeminiProviderFallsBackAfterIncompleteResponse(t *testing.T) {
	models := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		models = append(models, request.Model)
		if request.Model == "thinking-model" {
			fmt.Fprint(w, `{"id":"first","status":"incomplete"}`)
			return
		}
		fmt.Fprint(w, `{"id":"second","status":"completed","steps":[`+
			`{"type":"model_output","content":[{"type":"text","text":"{}"}]}]}`)
	}))
	defer server.Close()

	provider, err := NewGeminiProviderWithFallback(server.URL, "key", "thinking-model",
		[]string{"fallback-model"}, time.Second, 200)
	if err != nil {
		t.Fatal(err)
	}
	provider.retryBaseDelay = time.Millisecond
	response, err := provider.Generate(context.Background(), Request{
		Instructions: "i", Input: "x", SchemaName: "s", Schema: map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "fallback-model" || len(models) != 2 ||
		models[0] != "thinking-model" || models[1] != "fallback-model" {
		t.Fatalf("response=%+v models=%v", response, models)
	}
}

func TestGeminiProviderFallsBackAfterTransientFailure(t *testing.T) {
	models := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		models = append(models, request.Model)
		if request.Model == "busy-model" {
			http.Error(w, `{"error":{"message":"high demand"}}`, http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"id":"fallback-response","model":"stable-model","status":"completed",`+
			`"steps":[{"type":"model_output","content":[{"type":"text","text":"{}"}]}]}`)
	}))
	defer server.Close()

	provider, err := NewGeminiProviderWithFallback(server.URL, "key", "busy-model",
		[]string{"stable-model"}, time.Second, 200)
	if err != nil {
		t.Fatal(err)
	}
	provider.retryBaseDelay = time.Millisecond
	response, err := provider.Generate(context.Background(), Request{
		Instructions: "i", Input: "x", SchemaName: "s", Schema: map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "stable-model" || len(models) != 2 ||
		models[0] != "busy-model" || models[1] != "stable-model" {
		t.Fatalf("response=%+v models=%v", response, models)
	}
}

func TestGeminiProviderFallsBackAfterRequestTimeout(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		calls.Add(1)
		if request.Model == "slow-model" {
			time.Sleep(50 * time.Millisecond)
			return
		}
		fmt.Fprint(w, `{"id":"fallback-response","model":"stable-model","status":"completed",`+
			`"steps":[{"type":"model_output","content":[{"type":"text","text":"{}"}]}]}`)
	}))
	defer server.Close()

	provider, err := NewGeminiProviderWithFallback(server.URL, "key", "slow-model",
		[]string{"stable-model"}, 20*time.Millisecond, 200)
	if err != nil {
		t.Fatal(err)
	}
	provider.retryBaseDelay = time.Millisecond
	response, err := provider.Generate(context.Background(), Request{
		Instructions: "i", Input: "x", SchemaName: "s", Schema: map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "stable-model" || calls.Load() != 2 {
		t.Fatalf("response=%+v calls=%d", response, calls.Load())
	}
}

func TestGeminiProviderDoesNotFallbackAfterPermanentFailure(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, `{"error":{"message":"invalid request"}}`, http.StatusBadRequest)
	}))
	defer server.Close()

	provider, err := NewGeminiProviderWithFallback(server.URL, "key", "bad-model",
		[]string{"unused-model"}, time.Second, 200)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Generate(context.Background(), Request{
		Instructions: "i", Input: "x", SchemaName: "s", Schema: map[string]any{"type": "object"},
	})
	if err == nil || calls != 1 {
		t.Fatalf("Generate() error=%v calls=%d", err, calls)
	}
}
