package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxProviderResponseBytes = 2 << 20

type OpenAIProvider struct {
	endpoint        string
	apiKey          string
	model           string
	maxOutputTokens int
	client          *http.Client
}

func NewOpenAIProvider(baseURL, apiKey, model string, timeout time.Duration, maxOutputTokens int) (*OpenAIProvider, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid OpenAI base URL")
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = 2000
	}
	return &OpenAIProvider{
		endpoint: baseURL + "/responses", apiKey: strings.TrimSpace(apiKey),
		model: strings.TrimSpace(model), maxOutputTokens: maxOutputTokens,
		client: &http.Client{Timeout: timeout},
	}, nil
}

type openAIRequest struct {
	Model           string           `json:"model"`
	Instructions    string           `json:"instructions"`
	Input           string           `json:"input"`
	Store           bool             `json:"store"`
	MaxOutputTokens int              `json:"max_output_tokens"`
	Text            openAITextConfig `json:"text"`
}

type openAITextConfig struct {
	Format openAITextFormat `json:"format"`
}

type openAITextFormat struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type openAIResponse struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Status string `json:"status"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage Usage `json:"usage"`
}

func (p *OpenAIProvider) Generate(ctx context.Context, request Request) (Response, error) {
	if strings.TrimSpace(request.Instructions) == "" || strings.TrimSpace(request.Input) == "" ||
		strings.TrimSpace(request.SchemaName) == "" || len(request.Schema) == 0 {
		return Response{}, fmt.Errorf("invalid LLM request")
	}
	maxTokens := request.MaxOutputTokens
	if maxTokens <= 0 || maxTokens > p.maxOutputTokens {
		maxTokens = p.maxOutputTokens
	}
	body, err := json.Marshal(openAIRequest{
		Model: p.model, Instructions: request.Instructions, Input: request.Input,
		Store: false, MaxOutputTokens: maxTokens,
		Text: openAITextConfig{Format: openAITextFormat{
			Type: "json_schema", Name: request.SchemaName, Strict: true, Schema: request.Schema,
		}},
	})
	if err != nil {
		return Response{}, fmt.Errorf("encode OpenAI request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("create OpenAI request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("call OpenAI Responses API: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxProviderResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return Response{}, fmt.Errorf("read OpenAI response: %w", err)
	}
	if len(responseBody) > maxProviderResponseBytes {
		return Response{}, fmt.Errorf("%w: response exceeds size limit", ErrMalformedResponse)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Response{}, fmt.Errorf("OpenAI API returned %s: %s", response.Status,
			providerErrorSnippet(responseBody))
	}
	var decoded openAIResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	if decoded.Error != nil {
		return Response{}, fmt.Errorf("OpenAI response error %s: %s", decoded.Error.Code, decoded.Error.Message)
	}
	if decoded.Status != "completed" {
		return Response{}, fmt.Errorf("%w: response status is %q", ErrMalformedResponse, decoded.Status)
	}
	var output strings.Builder
	for _, item := range decoded.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				output.WriteString(content.Text)
			}
		}
	}
	if strings.TrimSpace(output.String()) == "" {
		return Response{}, fmt.Errorf("%w: no output text", ErrMalformedResponse)
	}
	if decoded.ID == "" || decoded.Model == "" {
		return Response{}, fmt.Errorf("%w: response metadata is missing", ErrMalformedResponse)
	}
	return Response{ID: decoded.ID, Model: decoded.Model, Output: output.String(), Usage: decoded.Usage}, nil
}

func providerErrorSnippet(body []byte) string {
	const maxErrorBytes = 4096
	if len(body) > maxErrorBytes {
		body = append(append([]byte(nil), body[:maxErrorBytes]...), []byte("...[truncated]")...)
	}
	return strings.TrimSpace(string(body))
}

var _ Provider = (*OpenAIProvider)(nil)
