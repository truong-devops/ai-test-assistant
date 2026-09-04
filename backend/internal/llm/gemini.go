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

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

type GeminiProvider struct {
	endpoint        string
	apiKey          string
	model           string
	maxOutputTokens int
	client          *http.Client
}

func NewGeminiProvider(baseURL, apiKey, model string, timeout time.Duration, maxOutputTokens int) (*GeminiProvider, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Gemini base URL")
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = 2000
	}
	return &GeminiProvider{
		endpoint: baseURL + "/interactions", apiKey: strings.TrimSpace(apiKey),
		model: strings.TrimSpace(model), maxOutputTokens: maxOutputTokens,
		client: &http.Client{Timeout: timeout},
	}, nil
}

type geminiRequest struct {
	Model             string                   `json:"model"`
	Input             string                   `json:"input"`
	SystemInstruction string                   `json:"system_instruction"`
	Store             bool                     `json:"store"`
	GenerationConfig  geminiGenerationConfig   `json:"generation_config"`
	ResponseFormat    geminiTextResponseFormat `json:"response_format"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int `json:"max_output_tokens"`
}

type geminiTextResponseFormat struct {
	Type     string         `json:"type"`
	MIMEType string         `json:"mime_type"`
	Schema   map[string]any `json:"schema"`
}

type geminiResponse struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Status string `json:"status"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Steps []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"steps"`
	Usage struct {
		TotalInputTokens  int `json:"total_input_tokens"`
		TotalOutputTokens int `json:"total_output_tokens"`
		TotalTokens       int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *GeminiProvider) Generate(ctx context.Context, request Request) (Response, error) {
	if strings.TrimSpace(request.Instructions) == "" || strings.TrimSpace(request.Input) == "" ||
		strings.TrimSpace(request.SchemaName) == "" || len(request.Schema) == 0 {
		return Response{}, fmt.Errorf("invalid LLM request")
	}
	maxTokens := request.MaxOutputTokens
	if maxTokens <= 0 || maxTokens > p.maxOutputTokens {
		maxTokens = p.maxOutputTokens
	}
	body, err := json.Marshal(geminiRequest{
		Model: p.model, Input: request.Input, SystemInstruction: request.Instructions,
		Store: false, GenerationConfig: geminiGenerationConfig{MaxOutputTokens: maxTokens},
		ResponseFormat: geminiTextResponseFormat{
			Type: "text", MIMEType: "application/json", Schema: request.Schema,
		},
	})
	if err != nil {
		return Response{}, fmt.Errorf("encode Gemini request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("create Gemini request: %w", err)
	}
	httpRequest.Header.Set("x-goog-api-key", p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("call Gemini Interactions API: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxProviderResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return Response{}, fmt.Errorf("read Gemini response: %w", err)
	}
	if len(responseBody) > maxProviderResponseBytes {
		return Response{}, fmt.Errorf("%w: response exceeds size limit", ErrMalformedResponse)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Response{}, fmt.Errorf("Gemini API returned %s: %s", response.Status,
			providerErrorSnippet(responseBody))
	}
	var decoded geminiResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	if decoded.Status != "completed" {
		if len(decoded.Errors) > 0 {
			return Response{}, fmt.Errorf("Gemini response error %s: %s",
				decoded.Errors[0].Code, decoded.Errors[0].Message)
		}
		return Response{}, fmt.Errorf("%w: response status is %q", ErrMalformedResponse, decoded.Status)
	}
	var output strings.Builder
	for _, step := range decoded.Steps {
		if step.Type != "model_output" {
			continue
		}
		for _, content := range step.Content {
			if content.Type == "text" {
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
	return Response{
		ID: decoded.ID, Model: decoded.Model, Output: output.String(),
		Usage: Usage{
			InputTokens: decoded.Usage.TotalInputTokens, OutputTokens: decoded.Usage.TotalOutputTokens,
			TotalTokens: decoded.Usage.TotalTokens,
		},
	}, nil
}

var _ Provider = (*GeminiProvider)(nil)
