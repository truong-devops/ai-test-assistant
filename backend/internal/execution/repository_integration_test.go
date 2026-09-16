package execution

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/report"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/validation"
)

func TestRepositoryQueuesInfraRetryPersistsEvidenceAndAuditsClassification(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	// A one-connection pool catches accidental nested cursor queries that would
	// otherwise hang only in constrained deployments.
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	suffix := time.Now().UnixNano()
	var projectID int64
	if err := pool.QueryRow(ctx, `INSERT INTO projects(name,provider,provider_project_id,repository_url,default_branch,language,status) VALUES($1,'github',$2,'https://example.test/repo','main','go','active') RETURNING id`, "execution-fixture", suffix).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID)
	var analysisID int64
	if err := pool.QueryRow(ctx, `INSERT INTO analysis_jobs(project_id,merge_request_iid,source_sha,target_sha,status,webhook_uuid,raw_event) VALUES($1,1,'source-sha','target-sha','PENDING',$2,'{}') RETURNING id`, projectID, "execution-"+time.Unix(0, suffix).Format("150405.000000000")).Scan(&analysisID); err != nil {
		t.Fatal(err)
	}
	var setID, suiteID, caseID, runID, itemID, artifactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO document_sets(name,product_name,scope) VALUES($1,'Product','Execution') RETURNING id`, "execution-set-"+time.Unix(0, suffix).Format("150405.000000000")).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM test_exports WHERE document_set_id=$1`, setID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM test_runs WHERE test_suite_id IN (SELECT id FROM test_suites WHERE document_set_id=$1)`, setID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID)
	}()
	if err := pool.QueryRow(ctx, `INSERT INTO test_suites(document_set_id,name) VALUES($1,'Execution suite') RETURNING id`, setID).Scan(&suiteID); err != nil {
		t.Fatal(err)
	}
	expectedHash := strings.Repeat("a", 64)
	sourceHash := strings.Repeat("b", 64)
	if err := pool.QueryRow(ctx, `INSERT INTO test_cases(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,expected_result,expected_result_hash,status) VALUES($1,$2,'TC-EXEC',1,'Execute','HAPPY','created',$3,'APPROVED') RETURNING id`, suiteID, setID, expectedHash).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO automation_artifacts(test_case_id,version_number,framework,file_path,source,source_hash,expected_result_hash,status,analysis_job_id) VALUES($1,1,'GO_TEST','cart/generated_test.go','package cart',$2,$3,'APPROVED',$4) RETURNING id`, caseID, sourceHash, expectedHash, analysisID).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_runs(test_suite_id,project_id,analysis_job_id,source_sha,target_sha) VALUES($1,$2,$3,'source-sha','target-sha') RETURNING id`, suiteID, projectID, analysisID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,expected_result_snapshot,expected_result_hash) VALUES($1,$2,'created',$3) RETURNING id`, runID, caseID, expectedHash).Scan(&itemID); err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(pool)
	if _, err := repository.Request(ctx, runID, RequestInput{RequestedBy: "QA"}); err != nil {
		t.Fatal(err)
	}
	first, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || first.AttemptCount != 1 {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	loaded, err := repository.LoadAttempt(ctx, first)
	if err != nil || len(loaded.Items) != 1 || loaded.Items[0].Artifact == nil || loaded.Items[0].Artifact.ID != artifactID {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	exit := 1
	if err := repository.Save(ctx, first, validation.SandboxEnvironment{ImageReference: "sandbox:test", ImageDigest: "sha256:image", Fingerprint: strings.Repeat("c", 64)}, []Outcome{{ItemID: itemID, ArtifactID: &artifactID, AutomationSourceHash: sourceHash, Status: StatusInfraError, ActualResult: "daemon unavailable", Command: "go test ./...", ExitCode: &exit, Evidence: []Evidence{{EvidenceType: "STDERR", Content: "temporary failure"}}}}, 2, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	afterRetry, err := repository.Get(ctx, runID)
	if err != nil || afterRetry.Status != RunPending || len(afterRetry.Items) != 2 || afterRetry.Items[0].Evidence[0].Content != "temporary failure" {
		t.Fatalf("after retry=%+v err=%v", afterRetry, err)
	}
	second, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || second.AttemptCount != 2 {
		t.Fatalf("second claim=%+v err=%v", second, err)
	}
	loaded, err = repository.LoadAttempt(ctx, second)
	if err != nil || loaded.Items[0].AutomationArtifactID == nil {
		t.Fatalf("retry item=%+v err=%v", loaded, err)
	}
	retryItem := loaded.Items[0]
	if err := repository.Save(ctx, second, validation.SandboxEnvironment{ImageReference: "sandbox:test", ImageDigest: "sha256:image", Fingerprint: strings.Repeat("c", 64)}, []Outcome{{ItemID: retryItem.ID, ArtifactID: &artifactID, AutomationSourceHash: sourceHash, Status: StatusPassed, ActualResult: "created", Command: "go test ./...", ExitCode: func() *int { v := 0; return &v }()}}, 2, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	completed, err := repository.Get(ctx, runID)
	if err != nil || completed.Status != RunCompleted || completed.ImageDigest != "sha256:image" {
		t.Fatalf("completed=%+v err=%v", completed, err)
	}
	reviewed, err := repository.ReviewClassification(ctx, retryItem.ID, ClassificationInput{Status: StatusProductFailed, ReviewerName: "Lead", Reason: "Observed approved assertion mismatch"})
	if err != nil || len(reviewed.Reviews) != 1 || reviewed.Reviews[0].PreviousStatus != StatusPassed {
		t.Fatalf("reviewed=%+v err=%v", reviewed, err)
	}
	reportService := report.NewService(report.NewRepository(pool))
	exported, err := reportService.Export(ctx, setID, report.ExportInput{
		TestSuiteID: suiteID,
		TestRunID:   &runID,
		Format:      report.FormatMarkdown,
		GeneratedBy: "QA",
	})
	if err != nil {
		t.Fatal(err)
	}
	content := string(exported.Content)
	if exported.RowCount != 1 || !strings.Contains(content, "PRODUCT_FAILED") ||
		!strings.Contains(content, "temporary failure") {
		t.Fatalf("unexpected execution report row_count=%d content=%s", exported.RowCount, content)
	}
}
