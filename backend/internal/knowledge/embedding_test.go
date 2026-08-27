package knowledge

import (
	"context"
	"math"
	"testing"
)

func TestHashEmbeddingIsDeterministicAndNormalized(t *testing.T) {
	client := NewHashEmbeddingClient("test", 32)
	first, err := client.Embed(context.Background(), []string{"CreateUser validates email"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Embed(context.Background(), []string{"CreateUser validates email"})
	if err != nil {
		t.Fatal(err)
	}
	var norm float64
	for index, value := range first[0] {
		if value != second[0][index] {
			t.Fatalf("embedding is not deterministic at %d", index)
		}
		norm += float64(value * value)
	}
	if math.Abs(math.Sqrt(norm)-1) > 0.0001 {
		t.Fatalf("embedding norm = %f", math.Sqrt(norm))
	}
}

func TestEmbeddingTokensSplitGoIdentifiers(t *testing.T) {
	tokens := embeddingTokens("TestServiceCreateUser")
	want := map[string]bool{"test": false, "service": false, "create": false, "user": false}
	for _, token := range tokens {
		if _, ok := want[token]; ok {
			want[token] = true
		}
	}
	for token, found := range want {
		if !found {
			t.Errorf("token %q not found in %v", token, tokens)
		}
	}
}

func TestEmbeddingProviderRejectsUnknownProvider(t *testing.T) {
	if _, err := NewEmbeddingClient("remote-unknown", "model"); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
