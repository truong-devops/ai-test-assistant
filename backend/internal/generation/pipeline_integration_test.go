//go:build integration

package generation

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
)

func TestGenerationPipelineWithRecommendationRAGAndMockProvider(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	projectID, analysisID, recommendationID := createGenerationAnalysis(t, ctx, pool, "pipeline")
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()
	embedder := knowledge.NewHashEmbeddingClient("hash-generation", knowledge.EmbeddingDimensions)
	chunks := []struct{ key, path, symbol, kind, content string }{
		{"impl", "internal/user/service.go", "CreateUser", "method", "func (s *Service) CreateUser(email string) error { return s.repository.Create(email) }"},
		{"interface", "internal/user/service.go", "Repository", "interface", "type Repository interface { Create(string) error }"},
		{"test", "internal/user/service_test.go", "TestCreateUser", "test", "func TestCreateUser(t *testing.T) { /* success case */ }"},
		{"mock", "internal/user/mock_repository_test.go", "mockRepository", "mock", "type mockRepository struct { createCalls int }"},
	}
	for _, chunk := range chunks {
		vectors, err := embedder.Embed(ctx, []string{chunk.content})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO knowledge_chunks
			(project_id, chunk_key, file_path, package_name, symbol_name, chunk_type,
			 content, content_hash, start_line, end_line, embedding_model, embedding, metadata)
			VALUES ($1,$2,$3,'user',$4,$5,$6,$7,1,1,'hash-generation',$8::vector,'{}')`,
			projectID, chunk.key, chunk.path, chunk.symbol, chunk.kind, chunk.content,
			knowledge.ContentHash(chunk.content), generationVector(vectors[0])); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewRepository(pool)
	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || claimed.ID != analysisID {
		t.Fatalf("claimed=%#v error=%v", claimed, err)
	}
	provider := &providerStub{result: llm.Response{
		ID: "resp-generation", Model: "fixture-model", Output: validGeneratedOutput,
	}}
	processor := NewProcessor(job.NewRepository(pool), recommendation.NewRepository(pool),
		knowledge.NewRetriever(knowledge.NewRepository(pool), embedder), provider, repository)
	if err := processor.Process(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	items, err := repository.List(ctx, analysisID)
	if err != nil || len(items) != 1 || items[0].RecommendationID != recommendationID ||
		items[0].ProviderResponseID != "resp-generation" || items[0].CodeHash == "" {
		t.Fatalf("items=%#v error=%v", items, err)
	}
	if provider.calls != 1 || !strings.Contains(provider.request.Input, "mockRepository") ||
		!strings.Contains(provider.request.Input, "TestCreateUser") ||
		!strings.Contains(provider.request.Input, "Repository") ||
		!strings.Contains(provider.request.Input, "Duplicate email") {
		t.Fatalf("provider calls=%d prompt=%s", provider.calls, provider.request.Input)
	}
	analysisJob, _, err := job.NewRepository(pool).Get(ctx, analysisID)
	if err != nil || analysisJob.Status != job.StatusValidating {
		t.Fatalf("analysis=%#v error=%v", analysisJob, err)
	}
}

func generationVector(values []float32) string {
	var builder strings.Builder
	builder.WriteByte('[')
	for index, value := range values {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte(']')
	return builder.String()
}
