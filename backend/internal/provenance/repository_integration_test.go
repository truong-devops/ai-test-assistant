//go:build integration

package provenance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
)

func TestRepositoryRecordsImmutableHistoricalEvidence(t *testing.T) {
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

	projectID, analysis, symbolID := createProvenanceFixture(t, ctx, pool, "immutable")
	defer pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID)
	repository := NewRepository(pool, RuntimeConfig{Provider: "openai", Model: "fixture-model",
		InputCostPerMillionUSD: 2, OutputCostPerMillionUSD: 8})
	input := provenanceRecordInput(analysis, symbolID, projectID)
	created, err := repository.Record(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID <= 0 || created.PromptHash == "" || created.ConfigurationHash == "" ||
		created.Context == nil || len(created.Context.Items) != 1 || created.EstimatedCostUSD != .012 {
		t.Fatalf("created=%#v", created)
	}
	if _, err := pool.Exec(ctx, `UPDATE project_indexes SET ref='new-ref', generation=2
		WHERE project_id=$1`, projectID); err != nil {
		t.Fatal(err)
	}
	calls, err := repository.List(ctx, analysis.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Context == nil || calls[0].Context.IndexRef != "main" ||
		calls[0].Context.IndexGeneration != 1 || calls[0].Context.Items[0].Content != "func CreateUser() {}" {
		t.Fatalf("historical calls=%#v", calls)
	}
	summaries, err := repository.ListSummary(ctx, analysis.ID)
	if err != nil || len(summaries) != 1 || summaries[0].Context == nil ||
		summaries[0].Context.ItemCount != 1 {
		t.Fatalf("summaries=%#v error=%v", summaries, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE llm_calls SET model_name='changed' WHERE id=$1`, created.ID); err == nil {
		t.Fatal("updating immutable LLM provenance succeeded")
	}
}

func TestRepositoryRejectsCrossProjectContextAtomically(t *testing.T) {
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

	projectID, analysis, symbolID := createProvenanceFixture(t, ctx, pool, "isolation")
	otherProjectID, _, _ := createProvenanceFixture(t, ctx, pool, "other")
	defer pool.Exec(context.Background(), `DELETE FROM projects WHERE id=ANY($1::bigint[])`,
		[]int64{projectID, otherProjectID})
	input := provenanceRecordInput(analysis, symbolID, projectID)
	input.Contexts[0].ProjectID = otherProjectID
	repository := NewRepository(pool, RuntimeConfig{Provider: "openai", Model: "fixture-model"})
	if _, err := repository.Record(ctx, input); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("Record() error=%v want ErrInvalidRecord", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM llm_calls WHERE analysis_job_id=$1`,
		analysis.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("cross-project record left %d LLM calls", count)
	}
}

func provenanceRecordInput(analysis job.AnalysisJob, symbolID, projectID int64) RecordInput {
	return RecordInput{
		Analysis: analysis, Phase: PhaseRecommendation, SubjectID: symbolID,
		AttemptNumber: 1, PromptVersion: "recommend-test-v1", Status: StatusCompleted,
		Latency: 125 * time.Millisecond,
		RetrievalQuery: knowledge.RetrievalQuery{ProjectID: projectID,
			Query: "CreateUser tests", SymbolName: "CreateUser", Limit: 10},
		Contexts: []knowledge.KnowledgeChunk{{
			ID: 91, ProjectID: projectID, ChunkKey: "service.go:CreateUser",
			FilePath: "internal/user/service.go", PackageName: "user",
			SymbolName: "CreateUser", ChunkType: "method", Content: "func CreateUser() {}",
			ContentHash: knowledge.ContentHash("func CreateUser() {}"), StartLine: 10,
			EndLine: 20, EmbeddingModel: "hash-v1", Score: 9.5,
			IndexRef: "main", IndexGeneration: 1,
		}},
		Request: llm.Request{Instructions: "Generate grounded recommendations.",
			Input: "project evidence", SchemaName: "recommendations",
			Schema: map[string]any{"type": "object"}, MaxOutputTokens: 2_000},
		Response: llm.Response{ID: "resp-1", Model: "fixture-model", Output: `{"recommendations":[]}`,
			Usage: llm.Usage{InputTokens: 2_000, OutputTokens: 1_000, TotalTokens: 3_000}},
	}
}

func createProvenanceFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	suffix string,
) (int64, job.AnalysisJob, int64) {
	t.Helper()
	unique := time.Now().UnixNano()
	var projectID int64
	if err := pool.QueryRow(ctx, `INSERT INTO projects
		(name, provider, provider_project_id, repository_url, default_branch, language, status)
		VALUES ($1,'github',$2,$3,'main','go','active') RETURNING id`, "provenance-"+suffix,
		unique, fmt.Sprintf("https://github.com/example/%s", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var analysisID int64
	if err := pool.QueryRow(ctx, `INSERT INTO analysis_jobs
		(project_id, merge_request_iid, source_sha, target_sha, status, webhook_uuid)
		VALUES ($1,1,'source-sha','target-sha',$2,$3) RETURNING id`, projectID,
		job.StatusRecommendingTests, fmt.Sprintf("provenance-%s-%d", suffix, unique)).Scan(&analysisID); err != nil {
		t.Fatal(err)
	}
	var fileID int64
	if err := pool.QueryRow(ctx, `INSERT INTO changed_files
		(analysis_job_id, old_path, new_path, change_type, diff)
		VALUES ($1,'internal/user/service.go','internal/user/service.go','modified','+change')
		RETURNING id`, analysisID).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	var symbolID int64
	if err := pool.QueryRow(ctx, `INSERT INTO changed_symbols
		(changed_file_id, symbol_name, symbol_kind, package_name, start_line, end_line,
		 change_type, change_summary)
		VALUES ($1,'CreateUser','function','user',10,20,'modified','modified function')
		RETURNING id`, fileID).Scan(&symbolID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_indexes
		(project_id, ref, status, generation, embedding_model)
		VALUES ($1,'main','READY',1,'hash-v1')`, projectID); err != nil {
		t.Fatal(err)
	}
	analysis, _, err := job.NewRepository(pool).Get(ctx, analysisID)
	if err != nil {
		t.Fatal(err)
	}
	return projectID, analysis, symbolID
}
