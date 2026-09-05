package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	geminiRetryBaseDelay = 500 * time.Millisecond
	maxGeminiModels      = 4
)

var errGeminiIncomplete = errors.New("Gemini response is incomplete")

type GeminiProvider struct {
	endpoint        string
	apiKey          string
	models          []string
	maxOutputTokens int
	client          *http.Client
	retryBaseDelay  time.Duration
}

func NewGeminiProvider(baseURL, apiKey, model string, timeout time.Duration, maxOutputTokens int) (*GeminiProvider, error) {
	return NewGeminiProviderWithFallback(baseURL, apiKey, model, nil, timeout, maxOutputTokens)
}

func NewGeminiProviderWithFallback(baseURL, apiKey, model string, fallbackModels []string,
	timeout time.Duration, maxOutputTokens int,
) (*GeminiProvider, error) {
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
	models, err := normalizeGeminiModels(model, fallbackModels)
	if err != nil {
		return nil, err
	}
	return &GeminiProvider{
		endpoint: baseURL + "/interactions", apiKey: strings.TrimSpace(apiKey),
		models: models, maxOutputTokens: maxOutputTokens,
		client: &http.Client{Timeout: timeout}, retryBaseDelay: geminiRetryBaseDelay,
	}, nil
}

func normalizeGeminiModels(primary string, fallbacks []string) ([]string, error) {
	primary = strings.TrimSpace(primary)
	if primary == "" {
		return nil, fmt.Errorf("Gemini model is required")
	}
	models := make([]string, 0, 1+len(fallbacks))
	seen := make(map[string]struct{}, 1+len(fallbacks))
	for _, model := range append([]string{primary}, fallbacks...) {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	if len(models) > maxGeminiModels {
		return nil, fmt.Errorf("Gemini supports at most %d configured models", maxGeminiModels)
	}
	return models, nil
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
	var lastErr error
	attempted := make([]string, 0, len(p.models))
	for index, model := range p.models {
		attempted = append(attempted, model)
		response, err := p.generateWithModel(ctx, request, model)
		if err == nil {
			return response, nil
		}
		lastErr = err
		if ctx.Err() != nil || !isTransientGeminiError(err) || index == len(p.models)-1 {
			break
		}
		delay := p.retryBaseDelay * time.Duration(1<<index)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Response{}, fmt.Errorf("Gemini request canceled after models %s: %w",
				strings.Join(attempted, ", "), ctx.Err())
		case <-timer.C:
		}
	}
	return Response{}, fmt.Errorf("Gemini models exhausted (%s): %w", strings.Join(attempted, ", "), lastErr)
}

func (p *GeminiProvider) generateWithModel(ctx context.Context, request Request, model string) (Response, error) {
	maxTokens := request.MaxOutputTokens
	if maxTokens <= 0 || maxTokens > p.maxOutputTokens {
		maxTokens = p.maxOutputTokens
	}
	body, err := json.Marshal(geminiRequest{
		Model: model, Input: request.Input, SystemInstruction: request.Instructions,
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
		return Response{}, &geminiAPIError{statusCode: response.StatusCode,
			status: response.Status, message: providerErrorSnippet(responseBody)}
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
		if decoded.Status == "incomplete" {
			return Response{}, fmt.Errorf("%w: output token budget was exhausted", errGeminiIncomplete)
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
	if decoded.ID == "" {
		return Response{}, fmt.Errorf("%w: response id is missing", ErrMalformedResponse)
	}
	responseModel := strings.TrimSpace(decoded.Model)
	if responseModel == "" {
		// The Interactions create response may omit the model even though the
		// request succeeded. The attempted model is authoritative for this
		// stateless request and keeps provenance records complete.
		responseModel = model
	}
	return Response{
		ID: decoded.ID, Model: responseModel, Output: output.String(),
		Usage: Usage{
			InputTokens: decoded.Usage.TotalInputTokens, OutputTokens: decoded.Usage.TotalOutputTokens,
			TotalTokens: decoded.Usage.TotalTokens,
		},
	}, nil
}

type geminiAPIError struct {
	statusCode int
	status     string
	message    string
}

func (e *geminiAPIError) Error() string {
	return fmt.Sprintf("Gemini API returned %s: %s", e.status, e.message)
}

func isTransientGeminiError(err error) bool {
	if errors.Is(err, errGeminiIncomplete) {
		return true
	}
	var apiErr *geminiAPIError
	if errors.As(err, &apiErr) {
		return apiErr.statusCode == http.StatusRequestTimeout ||
			apiErr.statusCode == http.StatusTooManyRequests || apiErr.statusCode >= 500
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) && (networkErr.Timeout() || networkErr.Temporary())
}

var _ Provider = (*GeminiProvider)(nil)
