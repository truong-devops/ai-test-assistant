package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		{name: "incomplete", status: http.StatusOK, body: `{"id":"r","model":"m","status":"incomplete"}`, want: ErrMalformedResponse},
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
