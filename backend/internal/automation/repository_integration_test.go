package automation

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepairRepositoryEligibilityImmutabilityAndReview(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	suffix := time.Now().UnixNano()
	var projectID, analysisID, setID, suiteID, caseID, artifactID, runID, repairItemID, productItemID, passedItemID int64
	if err = pool.QueryRow(ctx, `INSERT INTO projects(name,provider,provider_project_id,repository_url,default_branch,language,status)
		VALUES($1,'github',$2,'https://example.test/repair','main','go','active') RETURNING id`,
		"automation-repair-fixture", suffix).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID)
	if err = pool.QueryRow(ctx, `INSERT INTO analysis_jobs(project_id,merge_request_iid,source_sha,target_sha,status,webhook_uuid,raw_event)
		VALUES($1,1,'source','target','PENDING',$2,'{}') RETURNING id`, projectID,
		"automation-repair-"+time.Unix(0, suffix).Format("150405.000000000")).Scan(&analysisID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO document_sets(name,product_name,scope) VALUES($1,'Product','Repair') RETURNING id`,
		"automation-repair-set-"+time.Unix(0, suffix).Format("150405.000000000")).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID)
	if err = pool.QueryRow(ctx, `INSERT INTO test_suites(document_set_id,name) VALUES($1,'Repair suite') RETURNING id`, setID).Scan(&suiteID); err != nil {
		t.Fatal(err)
	}
	expectedHash := strings.Repeat("a", 64)
	if err = pool.QueryRow(ctx, `INSERT INTO test_cases(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,expected_result,expected_result_hash,status)
		VALUES($1,$2,'TC-REPAIR',1,'Repair','HAPPY','created',$3,'APPROVED') RETURNING id`,
		suiteID, setID, expectedHash).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	source := "package cart\nimport \"testing\"\nfunc TestOrder(t *testing.T) { missing(t) }\n"
	sourceHash := hash([]byte(source))
	if err = pool.QueryRow(ctx, `INSERT INTO automation_artifacts(test_case_id,version_number,framework,file_path,source,source_hash,expected_result_hash,status,analysis_job_id,assertions,test_case_snapshot,business_context,technical_context)
		VALUES($1,1,'GO_TEST','cart/cart_test.go',$2,$3,$4,'APPROVED',$5,'["order created"]','{}','{}','[]') RETURNING id`,
		caseID, source, sourceHash, expectedHash, analysisID).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO test_runs(test_suite_id,project_id,analysis_job_id,source_sha,target_sha,status)
		VALUES($1,$2,$3,'source','target','COMPLETED') RETURNING id`, suiteID, projectID, analysisID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	insertItem := `INSERT INTO test_run_items(test_run_id,test_case_id,automation_artifact_id,automation_source_hash,
		attempt_number,status,expected_result_snapshot,expected_result_hash,actual_result)
		VALUES($1,$2,$3,$4,$5,$6,'created',$7,$8) RETURNING id`
	if err = pool.QueryRow(ctx, insertItem, runID, caseID, artifactID, sourceHash, 1,
		"AUTOMATION_ERROR", expectedHash, "undefined: missing").Scan(&repairItemID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, insertItem, runID, caseID, artifactID, sourceHash, 2,
		"PRODUCT_FAILED", expectedHash, "want created got rejected").Scan(&productItemID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, insertItem, runID, caseID, artifactID, sourceHash, 3,
		"PASSED", expectedHash, "created").Scan(&passedItemID); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(pool)
	if _, err = repository.RequestRepair(ctx, productItemID, RepairRequest{RequestedBy: "QA"}, 2, 1000, 100000); !errors.Is(err, ErrRepairNotEligible) {
		t.Fatalf("product failure repair error=%v", err)
	}
	if _, err = repository.RequestRepair(ctx, passedItemID, RepairRequest{RequestedBy: "QA"}, 2, 1000, 100000); !errors.Is(err, ErrRepairNotEligible) {
		t.Fatalf("passed candidate repair error=%v", err)
	}
	queued, err := repository.RequestRepair(ctx, repairItemID, RepairRequest{RequestedBy: "QA"}, 2, 1000, 100000)
	if err != nil || queued.Status != RepairPending || queued.ExpectedResultHash != expectedHash {
		t.Fatalf("queued=%+v error=%v", queued, err)
	}
	claimed, err := repository.ClaimRepair(ctx, time.Minute)
	if err != nil || claimed.ID != queued.ID {
		t.Fatalf("claimed=%+v error=%v", claimed, err)
	}
	subject, err := repository.LoadRepairSubject(ctx, claimed)
	if err != nil || subject.Artifact.ID != artifactID {
		t.Fatalf("subject=%+v error=%v", subject, err)
	}
	repairedCode := "package cart\nimport \"testing\"\nfunc TestOrder(t *testing.T) { if false { t.Fatal() } }\n"
	proposal := proposedArtifact{Framework: FrameworkGoTest, TargetFile: "cart/cart_test.go",
		PackageName: "cart", Setup: "fixture", Assertions: []string{"order created"},
		ExpectedResultHash: expectedHash, TestCaseIDs: []int64{caseID}, Code: repairedCode}
	completed, err := repository.CompleteRepair(ctx, claimed, subject, RepairCompletion{
		Proposal: proposal, AfterSourceHash: hash([]byte(repairedCode)), ModelName: "fixture",
		ProviderResponseID: "response", Prompt: "guarded prompt", Response: "{}"})
	if err != nil || completed.Status != RepairWaitingReview || completed.RepairedArtifactID == nil {
		t.Fatalf("completed=%+v error=%v", completed, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE automation_repair_jobs SET expected_result_hash=$2 WHERE id=$1`,
		completed.ID, strings.Repeat("b", 64)); err == nil {
		t.Fatal("database allowed repair expected hash mutation")
	}
	if _, err = repository.Review(ctx, *completed.RepairedArtifactID, ReviewInput{
		Decision: StatusApproved, ReviewerName: "Lead", Comment: "technical repair reviewed"}); err != nil {
		t.Fatal(err)
	}
	var rerunItemID int64
	var rerunStatus string
	if err = pool.QueryRow(ctx, `SELECT id,status FROM test_run_items
		WHERE test_run_id=$1 AND test_case_id=$2 ORDER BY attempt_number DESC LIMIT 1`,
		runID, caseID).Scan(&rerunItemID, &rerunStatus); err != nil || rerunStatus != "NOT_RUN" {
		t.Fatalf("repair rerun item=%d status=%q error=%v", rerunItemID, rerunStatus, err)
	}
	var runStatus string
	if err = pool.QueryRow(ctx, `SELECT status FROM test_runs WHERE id=$1`, runID).Scan(&runStatus); err != nil || runStatus != "PENDING" {
		t.Fatalf("repair rerun status=%q error=%v", runStatus, err)
	}
	history, err := repository.ListRepairs(ctx, repairItemID)
	if err != nil || len(history) != 1 || history[0].Status != RepairApproved ||
		history[0].ExpectedResultHash != expectedHash {
		t.Fatalf("history=%+v error=%v", history, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE test_run_items SET status='AUTOMATION_ERROR',actual_result='still does not compile'
		WHERE id=$1`, rerunItemID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE test_runs SET status='COMPLETED' WHERE id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	second, err := repository.RequestRepair(ctx, rerunItemID, RepairRequest{RequestedBy: "QA"}, 2, 1000, 100000)
	if err != nil || second.AttemptNumber != 2 {
		t.Fatalf("second=%+v error=%v", second, err)
	}
	second, err = repository.ClaimRepair(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.MarkRepairUnrepairable(ctx, second, "no safe change", "", "", "fixture", "", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.RequestRepair(ctx, rerunItemID, RepairRequest{RequestedBy: "QA"}, 2, 1000, 100000); !errors.Is(err, ErrRepairLimit) {
		t.Fatalf("repair above hard limit error=%v", err)
	}
}
