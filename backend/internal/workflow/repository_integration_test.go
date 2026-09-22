//go:build integration

package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/aibudget"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type workflowTestCaseGeneratorStub struct{}

func (workflowTestCaseGeneratorStub) GeneratePinned(_ context.Context, setID int64, input testcase.GenerationBaseline) (
	testcase.GenerateSummary, error,
) {
	if len(input.RequirementIDs) != 1 || input.RequirementIDs[0] <= 0 || input.WorkflowJobID <= 0 || len(input.InputHash) != 64 {
		return testcase.GenerateSummary{}, errors.New("workflow did not forward its pinned generation baseline")
	}
	return testcase.GenerateSummary{DocumentSetID: setID, SuiteID: 77,
		RequirementCount: 1, CreatedCount: 1}, nil
}

func TestUV04WorkflowQueueLifecycleDelegateUsageAndPinnedInput(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	setID := workflowFixture(t, ctx, pool)
	defer cleanupWorkflowFixtures(t, pool, setID)
	repository := NewRepository(pool)

	snapshot, inputHash, err := repository.BuildIndexSnapshot(ctx, setID, nil)
	if err != nil {
		t.Fatal(err)
	}
	queued, created, err := repository.EnqueueNative(ctx, setID, OperationIndex, "QA",
		"uv04-index", snapshot, inputHash, 3)
	if err != nil || !created || queued.Status != StatusQueued || queued.Revision != 1 ||
		len(queued.Units) != 1 {
		t.Fatalf("enqueue=%+v created=%v error=%v", queued, created, err)
	}
	replayed, created, err := repository.EnqueueNative(ctx, setID, OperationIndex, "QA",
		"uv04-index", snapshot, inputHash, 3)
	if err != nil || created || replayed.ID != queued.ID {
		t.Fatalf("idempotent replay=%+v created=%v error=%v", replayed, created, err)
	}
	if _, _, err := repository.EnqueueNative(ctx, setID, OperationGenerate, "QA",
		"uv04-index", snapshot, strings.Repeat("f", 64), 3); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency collision error=%v", err)
	}

	claimed, err := repository.ClaimNext(ctx, time.Second)
	if err != nil || claimed.ID != queued.ID || claimed.AttemptCount != 1 ||
		claimed.Units[0].AttemptCount != 1 {
		t.Fatalf("claim=%+v error=%v", claimed, err)
	}
	if err := repository.Heartbeat(ctx, claimed, time.Second); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	budget := aibudget.NewManager(pool, 0.10, 0.20)
	reservation, err := budget.Reserve(aibudget.WithWorkflowAttempt(ctx, claimed.ID,
		claimed.Units[0].ID, claimed.Units[0].AttemptCount), setID, "TESTCASE_GENERATION",
		"requirement:1", llm.Request{Instructions: "generate", Input: "approved requirement",
			MaxOutputTokens: 100})
	if err != nil {
		t.Fatalf("reserve workflow budget: %v", err)
	}
	if err := budget.Finalize(ctx, reservation, llm.Usage{InputTokens: 10, OutputTokens: 20}); err != nil {
		t.Fatalf("finalize workflow budget: %v", err)
	}
	if err := repository.Complete(ctx, claimed, map[string]any{"index_generation": 1}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	completed, err := repository.Get(ctx, queued.ID)
	if err != nil || completed.Status != StatusSucceeded || completed.CompletedUnits != 1 ||
		completed.Usage.InputTokens != 10 || completed.Usage.OutputTokens != 20 {
		t.Fatalf("completed=%+v error=%v", completed, err)
	}

	staleSnapshot, staleHash, err := repository.BuildIndexSnapshot(ctx, setID, nil)
	if err != nil {
		t.Fatal(err)
	}
	stale, _, err := repository.EnqueueNative(ctx, setID, OperationIndex, "QA",
		"uv04-stale", staleSnapshot, staleHash, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE document_sets SET source_revision=source_revision+1 WHERE id=$1`, setID); err != nil {
		t.Fatal(err)
	}
	service := NewService(repository, nil, nil, nil, 3)
	replayedAfterChange, created, err := service.Enqueue(ctx, setID,
		OperationInput{Operation: OperationIndex}, "uv04-index", "editor")
	if err != nil || created || replayedAfterChange.ID != completed.ID {
		t.Fatalf("idempotent replay after source change=%+v created=%v error=%v",
			replayedAfterChange, created, err)
	}
	if _, _, err := service.Enqueue(ctx, setID,
		OperationInput{Operation: OperationExtract}, "uv04-index", "editor"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("service idempotency collision error=%v", err)
	}
	if err := repository.ValidateInputSnapshot(ctx, stale); !errors.Is(err, ErrInputStale) {
		t.Fatalf("changed source accepted by pinned job: %v", err)
	}
	stale, err = repository.Cancel(ctx, stale.ID, stale.Revision)
	if err != nil || stale.Status != StatusCanceled {
		t.Fatalf("cancel queued=%+v error=%v", stale, err)
	}
	stale, err = repository.Retry(ctx, stale.ID, stale.Revision)
	if err != nil || stale.Status != StatusQueued {
		t.Fatalf("retry canceled=%+v error=%v", stale, err)
	}
	claimedStale, err := repository.ClaimNext(ctx, time.Second)
	if err != nil || claimedStale.ID != stale.ID {
		t.Fatalf("claim stale retry=%+v error=%v", claimedStale, err)
	}
	canceling, err := repository.Cancel(ctx, claimedStale.ID, claimedStale.Revision)
	if err != nil || canceling.CancelRequested == nil {
		t.Fatalf("request running cancellation=%+v error=%v", canceling, err)
	}
	if err := repository.Complete(ctx, claimedStale, map[string]any{"must": "not publish"}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("canceled attempt published output: %v", err)
	}
	if err := repository.Fail(ctx, claimedStale, ErrCanceled, "CANCELED", false,
		time.Millisecond); err != nil {
		t.Fatalf("finalize cancellation: %v", err)
	}
	canceled, _ := repository.Get(ctx, stale.ID)
	if canceled.Status != StatusCanceled {
		t.Fatalf("running cancellation status=%+v", canceled)
	}

	crashSnapshot, crashHash, err := repository.BuildIndexSnapshot(ctx, setID, nil)
	if err != nil {
		t.Fatal(err)
	}
	crashJob, _, err := repository.EnqueueNative(ctx, setID, OperationIndex, "QA",
		"uv04-crash-recovery", crashSnapshot, crashHash, 3)
	if err != nil {
		t.Fatal(err)
	}
	firstAttempt, err := repository.ClaimNext(ctx, time.Second)
	if err != nil || firstAttempt.ID != crashJob.ID || firstAttempt.AttemptCount != 1 {
		t.Fatalf("first crash attempt=%+v error=%v", firstAttempt, err)
	}
	held, err := budget.Reserve(aibudget.WithWorkflowAttempt(ctx, firstAttempt.ID,
		firstAttempt.Units[0].ID, firstAttempt.Units[0].AttemptCount), setID,
		"TESTCASE_GENERATION", "provider-result-unknown",
		llm.Request{Instructions: "generate", Input: "request accepted by provider",
			MaxOutputTokens: 50})
	if err != nil || held.ID == 0 {
		t.Fatalf("reserve uncertain provider attempt=%+v error=%v", held, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE document_workflow_jobs
		SET lease_expires_at=NOW()-INTERVAL '1 second' WHERE id=$1`, crashJob.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := repository.ClaimNext(ctx, time.Second)
	if err != nil || recovered.ID != crashJob.ID || recovered.AttemptCount != 2 ||
		recovered.Units[0].AttemptCount != 2 {
		t.Fatalf("recovered attempt=%+v error=%v", recovered, err)
	}
	if err := repository.Complete(ctx, recovered, map[string]any{"index_generation": 2}); err != nil {
		t.Fatal(err)
	}
	recovered, err = repository.Get(ctx, crashJob.ID)
	if err != nil || recovered.Status != StatusSucceeded || recovered.Usage.ReservedTokens == 0 {
		t.Fatalf("recovered job lost uncertain usage=%+v error=%v", recovered, err)
	}

	var extractionID int64
	err = pool.QueryRow(ctx, `INSERT INTO requirement_extraction_jobs
		(document_set_id,index_generation,source_revision,total_chunks,requested_by)
		VALUES($1,1,1,4,'QA') RETURNING id`, setID).Scan(&extractionID)
	if err != nil {
		t.Fatal(err)
	}
	extractSnapshot, extractHash, err := hashJSON(map[string]any{
		"index_generation": 1, "source_revision": 1, "total_chunks": 4})
	if err != nil {
		t.Fatal(err)
	}
	delegated, created, err := repository.EnqueueDelegated(ctx, setID, OperationExtract,
		"QA", "uv04-extract", extractSnapshot, extractHash, 3,
		"REQUIREMENT_EXTRACTION", extractionID)
	if err != nil || !created || delegated.Status != StatusQueued {
		t.Fatalf("delegated enqueue=%+v created=%v error=%v", delegated, created, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE requirement_extraction_jobs SET status='RUNNING',
		attempt_count=1,processed_chunks=2,started_at=NOW() WHERE id=$1`, extractionID); err != nil {
		t.Fatal(err)
	}
	delegated, err = repository.Get(ctx, delegated.ID)
	if err != nil || delegated.Status != StatusRunning || delegated.CompletedUnits != 2 ||
		delegated.TotalUnits != 4 {
		t.Fatalf("delegated progress=%+v error=%v", delegated, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE requirement_extraction_jobs SET status='COMPLETED',
		processed_chunks=4,finished_at=NOW() WHERE id=$1`, extractionID); err != nil {
		t.Fatal(err)
	}
	delegated, err = repository.Get(ctx, delegated.ID)
	if err != nil || delegated.Status != StatusSucceeded || delegated.CompletedUnits != 4 {
		t.Fatalf("delegated completion=%+v error=%v", delegated, err)
	}

	generateSnapshot, generateHash, err := repository.BuildGenerateSnapshot(ctx, setID)
	if err != nil {
		t.Fatalf("build generation input: %v", err)
	}
	generateJob, created, err := repository.EnqueueNative(ctx, setID, OperationGenerate,
		"QA", "uv04-generate", generateSnapshot, generateHash, 3)
	if err != nil || !created {
		t.Fatalf("enqueue generation=%+v created=%v error=%v", generateJob, created, err)
	}
	claimedGenerate, err := repository.ClaimNext(ctx, time.Second)
	if err != nil || claimedGenerate.ID != generateJob.ID {
		t.Fatalf("claim generation=%+v error=%v", claimedGenerate, err)
	}
	processor := NewService(repository, nil, nil, workflowTestCaseGeneratorStub{}, 3)
	generateOutput, err := processor.Process(ctx, claimedGenerate)
	if err != nil {
		t.Fatalf("process generation: %v", err)
	}
	if err := repository.Complete(ctx, claimedGenerate, generateOutput); err != nil {
		t.Fatalf("complete generation: %v", err)
	}
	generateJob, err = repository.Get(ctx, generateJob.ID)
	var generateRefs map[string]any
	decodeErr := json.Unmarshal(generateJob.OutputRefs, &generateRefs)
	if err != nil || decodeErr != nil || generateJob.Status != StatusSucceeded ||
		generateRefs["created_count"] != float64(1) {
		t.Fatalf("completed generation=%+v error=%v", generateJob, err)
	}

	read, err := NewService(repository, nil, nil, nil, 3).Read(ctx, setID, "reviewer")
	if err != nil || !read.Capabilities.CanIndex || len(read.Steps) != 5 ||
		len(read.RecentJobs) < 3 {
		t.Fatalf("workflow read model=%+v error=%v", read, err)
	}
}

func workflowFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()
	var setID, documentID, versionID, blockID, snapshotID, requirementID int64
	err := pool.QueryRow(ctx, `INSERT INTO document_sets
		(name,product_name,scope,ai_token_budget,ai_cost_budget_microusd)
		VALUES($1,'Workflow','UV-04',1000000,1000000) RETURNING id`,
		"workflow-"+time.Now().Format("150405.000000000")).Scan(&setID)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO documents(document_set_id,name,document_type)
		VALUES($1,'URD','REQUIREMENTS') RETURNING id`, setID).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO document_versions
		(document_id,document_set_id,version_number,original_filename,media_type,size_bytes,
		 sha256,storage_key,approval_status,parse_status)
		VALUES($1,$2,1,'urd.md','text/markdown',10,$3,$4,'APPROVED','PARSED') RETURNING id`,
		documentID, setID, strings.Repeat("a", 64),
		"workflow/"+time.Now().Format("150405.000000000")).Scan(&versionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO document_blocks
		(document_version_id,ordinal,block_type,content,source_locator)
		VALUES($1,1,'PARAGRAPH','Checkout creates an order','line:1') RETURNING id`,
		versionID).Scan(&blockID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO document_source_snapshots
		(document_set_id,source_revision,fingerprint,included_count,excluded_count)
		SELECT id,source_revision,$2,1,0 FROM document_sets WHERE id=$1 RETURNING id`,
		setID, strings.Repeat("c", 64)).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO document_source_snapshot_items
		(source_snapshot_id,document_set_id,document_id,document_name,document_version_id,
		 version_number,sha256,parse_status,approval_status,included)
		VALUES($1,$2,$3,'URD',$4,1,$5,'PARSED','APPROVED',TRUE)`, snapshotID,
		setID, documentID, versionID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO requirements
		(document_set_id,requirement_key,title,statement,requirement_type,status,
		 confidence,source_snapshot_id,source_fingerprint)
		VALUES($1,'REQ-CHECKOUT','Checkout','Checkout creates an order','FUNCTIONAL',
		 'DRAFT',1,$2,$3) RETURNING id`, setID, snapshotID,
		strings.Repeat("d", 64)).Scan(&requirementID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO requirement_evidence
		(requirement_id,document_set_id,document_version_id,document_block_id,
		 source_locator,excerpt_hash) VALUES($1,$2,$3,$4,'line:1',$5)`,
		requirementID, setID, versionID, blockID, strings.Repeat("e", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE requirements SET status='APPROVED' WHERE id=$1`,
		requirementID); err != nil {
		t.Fatal(err)
	}
	return setID
}

func cleanupWorkflowFixtures(t *testing.T, pool *pgxpool.Pool, setIDs ...int64) {
	t.Helper()
	// Run before pool.Close. RESTRICT snapshot links require dependency ordering;
	// leaving a queued fixture behind makes a later ClaimNext take the wrong job.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Error(err)
		return
	}
	defer tx.Rollback(ctx)
	for _, query := range []string{
		`DELETE FROM document_workflow_jobs WHERE document_set_id=ANY($1)`,
		`DELETE FROM requirements WHERE document_set_id=ANY($1)`,
		`DELETE FROM document_source_snapshots WHERE document_set_id=ANY($1)`,
		`DELETE FROM document_sets WHERE id=ANY($1)`,
	} {
		if _, err := tx.Exec(ctx, query, setIDs); err != nil {
			t.Errorf("workflow fixture cleanup: %v", err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Error(err)
	}
}

func TestUV04WorkflowJobJSONKeepsMachineReadableProgress(t *testing.T) {
	payload, err := json.Marshal(Job{Status: StatusRunning, TotalUnits: 3,
		CompletedUnits: 1, ErrorCode: "PROVIDER_TIMEOUT"})
	if err != nil || !strings.Contains(string(payload), `"completed_units":1`) ||
		!strings.Contains(string(payload), `"error_code":"PROVIDER_TIMEOUT"`) {
		t.Fatalf("payload=%s error=%v", payload, err)
	}
}
