//go:build integration

package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/report"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type failGenerationOnce struct {
	service *testcase.Service
	failID  int64
	calls   map[int64]int
}

func (f *failGenerationOnce) GeneratePinned(ctx context.Context, id int64, input testcase.GenerationBaseline) (testcase.GenerateSummary, error) {
	if len(input.RequirementIDs) > 0 {
		key := input.RequirementIDs[0]
		f.calls[key]++
		if key == f.failID && f.calls[key] == 1 {
			return testcase.GenerateSummary{}, errors.New("controlled provider failure")
		}
	}
	return f.service.GeneratePinned(ctx, id, input)
}

func TestUV08AffectedPartialRetryRemovedAndHistoricalProof(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	setID := workflowFixture(t, ctx, pool)
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE document_workflow_jobs SET status='CANCELED',finished_at=NOW() WHERE document_set_id=$1 AND status IN ('QUEUED','RUNNING')`, setID)
	}()
	var firstReq int64
	if err = pool.QueryRow(ctx, `SELECT id FROM requirements WHERE document_set_id=$1`, setID).Scan(&firstReq); err != nil {
		t.Fatal(err)
	}
	cloneReq := func(key string, parent *int64) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO requirements(document_set_id,requirement_key,title,statement,requirement_type,flow_type,status,source_snapshot_id,supersedes_requirement_id) SELECT document_set_id,$2,title,statement,requirement_type,flow_type,'APPROVED',source_snapshot_id,$3 FROM requirements WHERE id=$1 RETURNING id`, firstReq, key, parent).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO requirement_evidence(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash) SELECT $2,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash FROM requirement_evidence WHERE requirement_id=$1`, firstReq, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	secondReq := cloneReq("REQ-OTHER", nil)
	cases := testcase.NewRepository(pool)
	caseService := testcase.NewService(cases, requirement.NewRepository(pool))
	if _, err = caseService.Generate(ctx, setID); err != nil {
		t.Fatal(err)
	}
	initial, err := cases.List(ctx, setID)
	if err != nil || len(initial) != 2 {
		t.Fatalf("initial=%+v %v", initial, err)
	}
	var original testcase.TestCase
	ids := []int64{}
	for _, c := range initial {
		d, err := cases.Get(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if d.Requirements[0].RequirementID == firstReq {
			original = c
		}
		ids = append(ids, c.ID)
		if _, err := cases.ReviewExact(ctx, c.ID, testcase.ReviewInput{ReviewerName: "QA", Decision: testcase.StatusApproved, ExpectedContentHash: c.ContentHash}, "qa"); err != nil {
			t.Fatal(err)
		}
	}
	r1, _, err := cases.PublishRelease(ctx, setID, testcase.PublishReleaseInput{TestSuiteID: original.TestSuiteID, RevisionIDs: ids, PublishedBy: "QA"}, "r1", "qa")
	if err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err = pool.QueryRow(ctx, `INSERT INTO test_runs(test_suite_id,suite_release_id,status) VALUES($1,$2,'COMPLETED') RETURNING id`, original.TestSuiteID, r1.ID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,status,expected_result_snapshot,expected_result_hash,actual_result) VALUES($1,$2,'PASSED',$3,$4,'immutable historical pass')`, runID, original.ID, original.ExpectedResult, original.ExpectedResultHash); err != nil {
		t.Fatal(err)
	}
	reports := report.NewService(report.NewRepository(pool))
	oldExport, err := reports.Export(ctx, setID, report.ExportInput{TestSuiteID: original.TestSuiteID, SuiteReleaseID: &r1.ID, TestRunID: &runID, Format: "XLSX", GeneratedBy: "QA"})
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	controlled := &failGenerationOnce{caseService, secondReq, map[int64]int{}}
	svc := NewService(repo, nil, nil, controlled, 3)
	for i, input := range []OperationInput{
		{Operation: OperationGenerate, ReviewProposals: true, GenerationScope: "SELECTED"},
		{Operation: OperationGenerate, ReviewProposals: true, GenerationScope: "ALL", RequirementIDs: []int64{firstReq}},
		{Operation: OperationGenerate, ReviewProposals: true, GenerationScope: "AFFECTED", RequirementIDs: []int64{firstReq}},
		{Operation: OperationGenerate, GenerationScope: "ALL"},
	} {
		if _, _, e := svc.Enqueue(ctx, setID, input, fmt.Sprint("invalid-scope-", i), "editor"); !errors.Is(e, ErrInvalidInput) {
			t.Fatalf("invalid scope accepted: %+v %v", input, e)
		}
	}
	if _, _, e := svc.Enqueue(ctx, setID, OperationInput{Operation: OperationGenerate, ReviewProposals: true}, "no-changes", "editor"); e == nil {
		t.Fatal("default affected scope regenerated fully covered current sources")
	} else {
		var blocked *BlockedError
		if !errors.As(e, &blocked) || blocked.Code != "NO_GENERATION_CHANGES" {
			t.Fatalf("unexpected no-change error: %v", e)
		}
	}
	var previousClaim Job
	run := func(job Job) Job {
		t.Helper()
		claim, e := repo.ClaimNext(ctx, time.Minute)
		if e != nil || claim.ID != job.ID {
			t.Fatalf("claim=%+v %v", claim, e)
		}
		if previousClaim.ID == claim.ID {
			if e := repo.Heartbeat(ctx, previousClaim, time.Minute); !errors.Is(e, ErrLeaseLost) {
				t.Fatalf("old heartbeat revived: %v", e)
			}
			if e := repo.Fail(ctx, previousClaim, errors.New("old worker"), "OLD", false, time.Second); !errors.Is(e, ErrLeaseLost) {
				t.Fatalf("old worker failed retry: %v", e)
			}
			if _, e := svc.Process(ctx, previousClaim); !errors.Is(e, ErrLeaseLost) {
				t.Fatalf("old worker processed retry: %v", e)
			}
		}
		previousClaim = claim
		out, e := svc.Process(ctx, claim)
		if e != nil {
			t.Fatal(e)
		}
		if e = repo.Complete(ctx, claim, out); e != nil {
			t.Fatal(e)
		}
		done, e := repo.Get(ctx, job.ID)
		if e != nil {
			t.Fatal(e)
		}
		return done
	}
	job, _, err := svc.Enqueue(ctx, setID, OperationInput{Operation: OperationGenerate, ReviewProposals: true, GenerationScope: "ALL"}, "partial", "editor")
	if err != nil {
		t.Fatal(err)
	}
	partial := run(job)
	if partial.Status != StatusPartialFailed || partial.FailedUnits != 1 || partial.CompletedUnits != 2 {
		t.Fatalf("partial=%+v", partial)
	}
	page, err := caseService.ListProposals(ctx, setID, job.ID, 0, 50)
	if err != nil || len(page.Proposals) != 1 {
		t.Fatalf("proposals=%+v %v", page, err)
	}
	p := page.Proposals[0]
	if _, err = caseService.DecideProposal(ctx, p.ID, testcase.ProposalDecision{Decision: "KEEP", TargetFamilyID: original.FamilyID, ExpectedHeadRevisionID: original.ID, Reason: "Reviewed during partial failure"}, "partial-keep", "qa"); err != nil {
		t.Fatal(err)
	}
	title := "Human edit survives partial retry"
	human, err := cases.CreateRevision(ctx, original.FamilyID, testcase.CreateRevisionInput{BaseRevisionID: original.ID, ExpectedHeadRevisionID: original.ID, Patch: &testcase.RevisionPatch{Title: &title}, Reason: "Human"}, "human", "qa")
	if err != nil {
		t.Fatal(err)
	}
	retried, err := svc.Retry(ctx, job.ID, partial.Revision, "editor")
	if err != nil {
		t.Fatal(err)
	}
	completed := run(retried)
	if completed.Status != StatusSucceeded || controlled.calls[firstReq] != 1 || controlled.calls[secondReq] != 2 {
		t.Fatalf("retry=%+v calls=%v", completed, controlled.calls)
	}
	family, err := cases.GetFamily(ctx, original.FamilyID)
	if err != nil || *family.HeadRevisionID != human.Revision.ID {
		t.Fatal("retry replaced human head")
	}
	page, _ = caseService.ListProposals(ctx, setID, job.ID, 0, 50)
	if len(page.Proposals) != 2 {
		t.Fatal("retry duplicated successful unit")
	}
	for _, saved := range page.Proposals {
		if saved.ID == p.ID && saved.Status != "APPLIED" {
			t.Fatal("retry reset reviewer decision")
		}
	}
	// Supersede one source only; the unrelated approved case is outside default scope.
	replacement := cloneReq("REQ-REPLACEMENT", &firstReq)
	if _, err = pool.Exec(ctx, `UPDATE requirements SET source_state='HISTORICAL' WHERE id=$1`, firstReq); err != nil {
		t.Fatal(err)
	}
	preview, err := repo.GenerationScope(ctx, setID)
	if err != nil || len(preview.AffectedRequirementIDs) != 1 || preview.AffectedRequirementIDs[0] != replacement || len(preview.AffectedFamilyIDs) != 1 || preview.AffectedFamilyIDs[0] != original.FamilyID {
		t.Fatalf("scope=%+v %v", preview, err)
	}
	changed, _, err := svc.Enqueue(ctx, setID, OperationInput{Operation: OperationGenerate, ReviewProposals: true}, "affected", "editor")
	if err != nil {
		t.Fatal(err)
	}
	done := run(changed)
	if done.Status != StatusSucceeded {
		t.Fatalf("affected=%+v", done)
	}
	page, err = caseService.ListProposals(ctx, setID, changed.ID, 0, 50)
	if err != nil || len(page.Proposals) != 1 {
		t.Fatalf("affected proposals=%+v %v", page, err)
	}
	p = page.Proposals[0]
	if p.Classification != "AMBIGUOUS_MATCH" || len(p.Candidates) != 1 || p.Candidates[0].FamilyID != original.FamilyID {
		t.Fatalf("unrelated lineage candidate=%+v", p)
	}
	applied, err := caseService.DecideProposal(ctx, p.ID, testcase.ProposalDecision{Decision: "REVISE", TargetFamilyID: original.FamilyID, ExpectedHeadRevisionID: human.Revision.ID, Reason: "Confirmed new source revision"}, "affected-apply", "qa")
	if err != nil {
		t.Fatal(err)
	}
	next, err := cases.Get(ctx, *applied.ResultRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if next.TestCase.Status != testcase.StatusDraft {
		t.Fatal("auto-approved replacement")
	}
	if _, err = cases.ReviewExact(ctx, next.TestCase.ID, testcase.ReviewInput{ReviewerName: "QA", Decision: testcase.StatusApproved, ExpectedContentHash: next.TestCase.ContentHash}, "qa"); err != nil {
		t.Fatal(err)
	}
	ids = []int64{next.TestCase.ID}
	for _, c := range initial {
		if c.FamilyID != original.FamilyID {
			ids = append(ids, c.ID)
		}
	}
	if _, _, err = cases.PublishRelease(ctx, setID, testcase.PublishReleaseInput{TestSuiteID: original.TestSuiteID, RevisionIDs: ids, PublishedBy: "QA"}, "r2", "qa"); err != nil {
		t.Fatal(err)
	}
	// All sources removed: generate only retire, no provider/requirement unit calls.
	if _, err = pool.Exec(ctx, `UPDATE requirements SET source_state='REMOVED' WHERE document_set_id=$1`, setID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO document_ai_budget_reservations(document_set_id,phase,subject_key,status,reserved_tokens,reserved_cost_microusd,input_tokens,expires_at,completed_at) SELECT id,'TEST','exhausted-budget','COMPLETED',ai_token_budget,0,ai_token_budget,NOW(),NOW() FROM document_sets WHERE id=$1`, setID); err != nil {
		t.Fatal(err)
	}
	read, err := svc.Read(ctx, setID, "editor")
	if err != nil || !read.Capabilities.CanGenerate || len(read.GenerationScope.RemovedFamilyIDs) != 2 {
		t.Fatalf("retire without AI budget must remain available: %+v %v", read, err)
	}
	retired, _, err := svc.Enqueue(ctx, setID, OperationInput{Operation: OperationGenerate, ReviewProposals: true}, "retire-all", "editor")
	if err != nil {
		t.Fatal(err)
	}
	if result := run(retired); result.Status != StatusSucceeded || result.TotalUnits != 1 {
		t.Fatalf("retire=%+v", result)
	}
	page, err = caseService.ListProposals(ctx, setID, retired.ID, 0, 50)
	if err != nil || len(page.Proposals) != 2 {
		t.Fatalf("retire proposals=%+v %v", page, err)
	}
	for _, p := range page.Proposals {
		if p.Classification != "RETIRE_CANDIDATE" {
			t.Fatal(p.Classification)
		}
		target := p.Candidates[0]
		if _, err := caseService.DecideProposal(ctx, p.ID, testcase.ProposalDecision{Decision: "ARCHIVE", TargetFamilyID: target.FamilyID, ExpectedHeadRevisionID: target.RevisionID, Reason: "Confirmed removed source"}, fmt.Sprint("archive-", p.ID), "qa"); err != nil {
			t.Fatal(err)
		}
	}
	if controlled.calls[firstReq] != 1 || controlled.calls[secondReq] != 2 || controlled.calls[replacement] != 1 {
		t.Fatalf("unnecessary generation=%v", controlled.calls)
	}
	oldRelease, err := cases.GetRelease(ctx, r1.ID)
	if err != nil || oldRelease.ManifestHash != r1.ManifestHash {
		t.Fatal("R1 mutated")
	}
	downloaded, err := reports.Download(ctx, oldExport.ID)
	if err != nil || downloaded.ContentHash != oldExport.ContentHash || !bytes.Equal(downloaded.Content, oldExport.Content) {
		t.Fatal("old XLSX changed")
	}
	runs, err := caseService.RevisionRuns(ctx, original.ID, false, 0)
	if err != nil || len(runs.Runs) != 1 || runs.Runs[0].Status != "PASSED" {
		t.Fatalf("old run changed=%+v %v", runs, err)
	}
	newerRuns, err := caseService.RevisionRuns(ctx, next.TestCase.ID, false, 0)
	if err != nil || len(newerRuns.Runs) != 0 {
		t.Fatal("new revision inherited PASS")
	}
}
