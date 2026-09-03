//go:build integration

package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRetrievalRankingFixtureCreateUser(t *testing.T) {
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
	projectID := createKnowledgeProject(t, ctx, pool, "ranking")
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()
	repository := NewRepository(pool)
	embedder := NewHashEmbeddingClient("hash-integration", EmbeddingDimensions)
	if _, err := repository.RequestIndex(ctx, projectID, "fixture"); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	fixtureFiles := []string{
		"user-service/internal/user/service.go",
		"user-service/internal/user/service_test.go",
		"user-service/internal/user/mock_repository_test.go",
	}
	chunks := make([]KnowledgeChunk, 0)
	for _, filePath := range fixtureFiles {
		content, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "go-microservices", filePath))
		if err != nil {
			t.Fatal(err)
		}
		drafts, err := ChunkFile(filePath, content)
		if err != nil {
			t.Fatal(err)
		}
		for _, draft := range drafts {
			vectors, err := embedder.Embed(ctx, []string{embeddingText(draft)})
			if err != nil {
				t.Fatal(err)
			}
			metadata, _ := json.Marshal(draft.Metadata)
			chunks = append(chunks, KnowledgeChunk{
				ProjectID: projectID, ChunkKey: draft.ChunkKey, FilePath: draft.FilePath,
				PackageName: draft.PackageName, SymbolName: draft.SymbolName, ChunkType: draft.ChunkType,
				Content: draft.Content, ContentHash: ContentHash(draft.Content), StartLine: draft.StartLine,
				EndLine: draft.EndLine, EmbeddingModel: embedder.Model(), Metadata: metadata,
				Embedding: vectors[0],
			})
		}
	}
	if err := repository.SaveIndex(ctx, claimed, chunks, len(fixtureFiles), 0, embedder.Model()); err != nil {
		t.Fatal(err)
	}
	retriever := NewRetriever(repository, embedder)
	results, err := retriever.RetrieveContext(ctx, RetrievalQuery{
		ProjectID: projectID, Query: "CreateUser repository validation tests mock",
		PackageName: "user", SymbolName: "CreateUser",
		FilePath: "user-service/internal/user/service.go", PreferTests: true, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"CreateUser": false, "TestServiceCreateUser": false, "Repository": false, "mockRepository": false}
	for _, result := range results {
		if _, ok := want[result.SymbolName]; ok {
			want[result.SymbolName] = true
		}
	}
	for symbol, found := range want {
		if !found {
			t.Errorf("expected %q in top results: %#v", symbol, results)
		}
	}
	if len(results) == 0 || results[0].SymbolName != "CreateUser" {
		t.Fatalf("top result=%#v, want CreateUser", results)
	}
}

func TestRepositoryIndexLifecycleAndProjectIsolation(t *testing.T) {
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

	projectOne := createKnowledgeProject(t, ctx, pool, "one")
	projectTwo := createKnowledgeProject(t, ctx, pool, "two")
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=ANY($1::bigint[])`,
			[]int64{projectOne, projectTwo})
	}()
	repository := NewRepository(pool)
	embedder := NewHashEmbeddingClient("hash-integration", EmbeddingDimensions)

	requested, err := repository.RequestIndex(ctx, projectOne, "main")
	if err != nil || requested.Status != IndexStatusPending || requested.Generation != 1 {
		t.Fatalf("requested=%#v error=%v", requested, err)
	}
	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || claimed.ProjectID != projectOne || claimed.AttemptCount != 1 {
		t.Fatalf("claimed=%#v error=%v", claimed, err)
	}
	if _, err := repository.ClaimNext(ctx, time.Minute); !errors.Is(err, ErrIndexNotFound) {
		t.Fatalf("concurrent claim error=%v", err)
	}
	chunkOne := integrationChunk(t, ctx, embedder, projectOne, "service", "CreateUser",
		"internal/user/service.go", "method", "func (s *Service) CreateUser() {}")
	if err := repository.SaveIndex(ctx, claimed, []KnowledgeChunk{chunkOne}, 1, 0, embedder.Model()); err != nil {
		t.Fatal(err)
	}
	status, err := repository.GetIndex(ctx, projectOne)
	if err != nil || status.Status != IndexStatusReady || status.ChunkCount != 1 || status.AttemptCount != 0 {
		t.Fatalf("status=%#v error=%v", status, err)
	}

	retriever := NewRetriever(repository, embedder)
	results, err := retriever.RetrieveContext(ctx, RetrievalQuery{
		ProjectID: projectOne, Query: "CreateUser", PackageName: "user", SymbolName: "CreateUser", Limit: 5,
	})
	if err != nil || len(results) != 1 || results[0].ProjectID != projectOne || results[0].SymbolName != "CreateUser" {
		t.Fatalf("results=%#v error=%v", results, err)
	}
	originalID := results[0].ID

	if _, err := repository.RequestIndex(ctx, projectOne, "main"); err != nil {
		t.Fatal(err)
	}
	reindexClaim, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	fingerprints, err := repository.ContentFingerprints(ctx, projectOne)
	if err != nil || fingerprints[chunkOne.ChunkKey].ContentHash != chunkOne.ContentHash {
		t.Fatalf("fingerprints=%#v error=%v", fingerprints, err)
	}
	unchanged := chunkOne
	unchanged.Embedding = nil
	unchanged.StartLine, unchanged.EndLine = 10, 10
	if err := repository.SaveIndex(ctx, reindexClaim, []KnowledgeChunk{unchanged}, 1, 0, embedder.Model()); err != nil {
		t.Fatal(err)
	}
	results, err = retriever.RetrieveContext(ctx, RetrievalQuery{ProjectID: projectOne, Query: "CreateUser", Limit: 5})
	if err != nil || len(results) != 1 || results[0].ID != originalID || results[0].StartLine != 10 {
		t.Fatalf("content-hash update results=%#v error=%v", results, err)
	}

	requestedTwo, err := repository.RequestIndex(ctx, projectTwo, "main")
	if err != nil {
		t.Fatal(err)
	}
	claimedTwo, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || claimedTwo.Generation != requestedTwo.Generation {
		t.Fatal(err)
	}
	chunkTwo := integrationChunk(t, ctx, embedder, projectTwo, "foreign", "CreateUser",
		"internal/foreign/service.go", "function", "func CreateUser() { panic(\"foreign\") }")
	if err := repository.SaveIndex(ctx, claimedTwo, []KnowledgeChunk{chunkTwo}, 1, 0, embedder.Model()); err != nil {
		t.Fatal(err)
	}
	results, err = retriever.RetrieveContext(ctx, RetrievalQuery{ProjectID: projectOne, Query: "CreateUser", Limit: 10})
	if err != nil || len(results) != 1 || results[0].ProjectID != projectOne {
		t.Fatalf("project isolation results=%#v error=%v", results, err)
	}
}

func TestRepositoryIndexGenerationAndRetry(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	projectID := createKnowledgeProject(t, ctx, pool, "generation")
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()
	repository := NewRepository(pool)

	if _, err := repository.RequestIndex(ctx, projectID, "main"); err != nil {
		t.Fatal(err)
	}
	first, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RenewLease(ctx, first, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := repository.RetryOrFail(ctx, first, fmt.Errorf("temporary"), 3, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNext(ctx, time.Minute); !errors.Is(err, ErrIndexNotFound) {
		t.Fatalf("claim before retry delay error=%v", err)
	}
	time.Sleep(300 * time.Millisecond)
	retried, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || retried.AttemptCount != 2 {
		t.Fatalf("retried=%#v error=%v", retried, err)
	}
	newRequest, err := repository.RequestIndex(ctx, projectID, "develop")
	if err != nil || newRequest.Generation != first.Generation+1 {
		t.Fatalf("new request=%#v error=%v", newRequest, err)
	}
	if err := repository.SaveIndex(ctx, retried, nil, 0, 0, "hash-integration"); !errors.Is(err, ErrIndexLeaseLost) {
		t.Fatalf("stale SaveIndex error=%v", err)
	}
	latest, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || latest.Generation != newRequest.Generation || latest.Ref != "develop" || latest.AttemptCount != 1 {
		t.Fatalf("latest=%#v error=%v", latest, err)
	}
}

func TestRepositoryRetrievalKeepsZeroVectorScoresFinite(t *testing.T) {
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

	projectID := createKnowledgeProject(t, ctx, pool, "zero-vector")
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()
	repository := NewRepository(pool)
	embedder := NewHashEmbeddingClient("hash-integration", EmbeddingDimensions)
	if _, err := repository.RequestIndex(ctx, projectID, "main"); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	chunk := KnowledgeChunk{
		ProjectID: projectID, ChunkKey: "zero-vector", FilePath: "internal/user/service.go",
		PackageName: "user", SymbolName: "CreateUser", ChunkType: "function",
		Content:     "func CreateUser() error { return nil }",
		ContentHash: ContentHash("func CreateUser() error { return nil }"),
		StartLine:   1, EndLine: 1, EmbeddingModel: embedder.Model(),
		Metadata:  json.RawMessage(`{"fixture":"zero-vector"}`),
		Embedding: make([]float32, EmbeddingDimensions),
	}
	if err := repository.SaveIndex(ctx, claimed, []KnowledgeChunk{chunk}, 1, 0, embedder.Model()); err != nil {
		t.Fatal(err)
	}
	results, err := NewRetriever(repository, embedder).RetrieveContext(ctx, RetrievalQuery{
		ProjectID: projectID, Query: "CreateUser", SymbolName: "CreateUser", Limit: 1,
	})
	if err != nil || len(results) != 1 || math.IsNaN(results[0].Score) || math.IsInf(results[0].Score, 0) {
		t.Fatalf("results=%#v error=%v", results, err)
	}
}

func createKnowledgeProject(t *testing.T, ctx context.Context, pool *pgxpool.Pool, suffix string) int64 {
	t.Helper()
	var projectID int64
	gitLabID := time.Now().UnixNano()
	if err := pool.QueryRow(ctx, `INSERT INTO projects
		(name, provider, provider_project_id, repository_url, default_branch, language, status)
		VALUES ($1,'gitlab',$2,$3,'main','go','active') RETURNING id`, "knowledge-"+suffix,
		gitLabID, fmt.Sprintf("https://gitlab.example.com/%s.git", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	return projectID
}

func integrationChunk(t *testing.T, ctx context.Context, embedder EmbeddingClient, projectID int64,
	packageName, symbolName, filePath, chunkType, content string,
) KnowledgeChunk {
	t.Helper()
	vectors, err := embedder.Embed(ctx, []string{content + " " + symbolName + " " + packageName})
	if err != nil {
		t.Fatal(err)
	}
	return KnowledgeChunk{
		ProjectID: projectID, ChunkKey: stableChunkKey(filePath, chunkType, symbolName),
		FilePath: filePath, PackageName: packageName, SymbolName: symbolName, ChunkType: chunkType,
		Content: content, ContentHash: ContentHash(content), StartLine: 1, EndLine: 1,
		EmbeddingModel: embedder.Model(), Metadata: json.RawMessage(`{"fixture":true}`), Embedding: vectors[0],
	}
}
