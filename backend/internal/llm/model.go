package llm

import (
	"context"
	"errors"
)

var (
	ErrDisabled          = errors.New("LLM provider is disabled")
	ErrMalformedResponse = errors.New("malformed LLM provider response")
)

type Request struct {
	Instructions    string
	Input           string
	SchemaName      string
	Schema          map[string]any
	MaxOutputTokens int
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type Response struct {
	ID     string
	Model  string
	Output string
	Usage  Usage
}

type Provider interface {
	Generate(ctx context.Context, request Request) (Response, error)
}

type DisabledProvider struct{}

func (DisabledProvider) Generate(context.Context, Request) (Response, error) {
	return Response{}, ErrDisabled
}
