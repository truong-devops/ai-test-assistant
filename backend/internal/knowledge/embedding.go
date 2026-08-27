package knowledge

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

var ErrUnsupportedEmbeddingProvider = errors.New("unsupported embedding provider")

type EmbeddingClient interface {
	Model() string
	Dimensions() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type HashEmbeddingClient struct {
	model      string
	dimensions int
}

func NewEmbeddingClient(provider, model string) (EmbeddingClient, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || provider == "local" || provider == "hash" {
		if strings.TrimSpace(model) == "" {
			model = "hash-v1"
		}
		return NewHashEmbeddingClient(model, EmbeddingDimensions), nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupportedEmbeddingProvider, provider)
}

func NewHashEmbeddingClient(model string, dimensions int) *HashEmbeddingClient {
	return &HashEmbeddingClient{model: model, dimensions: dimensions}
}

func (c *HashEmbeddingClient) Model() string { return c.model }

func (c *HashEmbeddingClient) Dimensions() int { return c.dimensions }

func (c *HashEmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if c.dimensions <= 0 {
		return nil, fmt.Errorf("embedding dimensions must be positive")
	}
	results := make([][]float32, len(texts))
	for index, input := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vector := make([]float32, c.dimensions)
		for _, token := range embeddingTokens(input) {
			hasher := fnv.New64a()
			_, _ = hasher.Write([]byte(token))
			vector[int(hasher.Sum64()%uint64(c.dimensions))]++
		}
		var norm float64
		for _, value := range vector {
			norm += float64(value * value)
		}
		if norm == 0 {
			return nil, fmt.Errorf("embedding text %d has no searchable tokens", index)
		}
		norm = math.Sqrt(norm)
		for dimension := range vector {
			vector[dimension] = float32(float64(vector[dimension]) / norm)
		}
		results[index] = vector
	}
	return results, nil
}

func embeddingTokens(input string) []string {
	result := make([]string, 0)
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		word := string(current)
		result = append(result, strings.ToLower(word))
		for _, part := range splitIdentifier(word) {
			part = strings.ToLower(part)
			if part != "" && part != strings.ToLower(word) {
				result = append(result, part)
			}
		}
		current = current[:0]
	}
	for _, character := range input {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			current = append(current, character)
		} else {
			flush()
		}
	}
	flush()
	return result
}

func splitIdentifier(input string) []string {
	runes := []rune(input)
	if len(runes) == 0 {
		return nil
	}
	result := make([]string, 0)
	start := 0
	for index := 1; index < len(runes); index++ {
		boundary := unicode.IsUpper(runes[index]) && (unicode.IsLower(runes[index-1]) ||
			(index+1 < len(runes) && unicode.IsLower(runes[index+1])))
		if boundary {
			result = append(result, string(runes[start:index]))
			start = index
		}
	}
	result = append(result, string(runes[start:]))
	return result
}
