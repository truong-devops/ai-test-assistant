//go:build integration

package impact

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

func TestRepositorySavesImpactAtomically(t *testing.T) {
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
	projectID, analysisID, fileID := createImpactFixture(t, ctx, pool)
	defer pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID)
	repository := NewRepository(pool)
	graph := Result{SourceSHA: "source", Mode: ModeSSA, Algorithm: "cha-v1+x-tools-v0.49.0",
		MaxDepth: 3, MaxNodes: 250, PackageCount: 2,
		Nodes: []Node{
			{Key: "direct", PackagePath: "example/service", PackageName: "service", SymbolName: "Load",
				SymbolKind: "function", FilePath: "service.go", StartLine: 3, EndLine: 5,
				DirectChange: true, Score: 1, ReasonCodes: []string{ReasonDirectChange}},
			{Key: "test", PackagePath: "example/service", PackageName: "service", SymbolName: "TestLoad",
				SymbolKind: "test", FilePath: "service_test.go", StartLine: 3, EndLine: 6,
				ExistingTest: true, Depth: 1, Score: .92, ReasonCodes: []string{ReasonExistingTest}},
		}, Edges: []Edge{{FromKey: "test", ToKey: "direct", Relation: RelationCalls,
			ReasonCode: ReasonExistingTest, Depth: 1, Score: .92}}}
	symbols := []job.ChangedSymbol{{ChangedFileID: fileID, SymbolName: "Load",
		SymbolKind: "function", PackageName: "service", StartLine: 3, EndLine: 5,
		ChangeType: "modified", ChangeSummary: "direct change"}}
	if err := repository.SaveAnalysis(ctx, analysisID, projectID, 1, symbols, graph); err != nil {
		t.Fatal(err)
	}
	bundle, err := repository.Get(ctx, analysisID)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Run.Mode != ModeSSA || len(bundle.Nodes) != 2 || len(bundle.Edges) != 1 ||
		bundle.Edges[0].FromNodeID == 0 || bundle.Edges[0].ToNodeID == 0 {
		t.Fatalf("bundle=%#v", bundle)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM analysis_jobs WHERE id=$1`, analysisID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != job.StatusRetrievingContext {
		t.Fatalf("status=%s", status)
	}
}

func TestRepositoryRejectsWrongProjectWithoutPartialWrites(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	projectID, analysisID, _ := createImpactFixture(t, ctx, pool)
	defer pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID)
	err = NewRepository(pool).SaveAnalysis(ctx, analysisID, projectID+999, 1, nil,
		Result{SourceSHA: "source", Mode: ModeSSA, Algorithm: "cha-v1", MaxDepth: 3, MaxNodes: 10})
	if !errors.Is(err, job.ErrLeaseLost) {
		t.Fatalf("err=%v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM impact_analysis_runs WHERE analysis_job_id=$1`, analysisID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("partial impact rows=%d", count)
	}
}

func createImpactFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (int64, int64, int64) {
	t.Helper()
	unique := time.Now().UnixNano()
	var projectID int64
	if err := pool.QueryRow(ctx, `INSERT INTO projects
		(name, provider, provider_project_id, repository_url, default_branch, language, status)
		VALUES ($1,'github',$2,$3,'main','go','active') RETURNING id`,
		fmt.Sprintf("impact-%d", unique), unique, fmt.Sprintf("https://github.com/example/impact-%d", unique)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var analysisID int64
	if err := pool.QueryRow(ctx, `INSERT INTO analysis_jobs
		(project_id, merge_request_iid, source_sha, target_sha, status, webhook_uuid, attempt_count)
		VALUES ($1,1,'source','target',$2,$3,1) RETURNING id`, projectID,
		job.StatusAnalyzingChange, fmt.Sprintf("impact-%d", unique)).Scan(&analysisID); err != nil {
		t.Fatal(err)
	}
	var fileID int64
	if err := pool.QueryRow(ctx, `INSERT INTO changed_files
		(analysis_job_id, old_path, new_path, change_type, diff)
		VALUES ($1,'service.go','service.go','modified','+change') RETURNING id`, analysisID).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	return projectID, analysisID, fileID
}
