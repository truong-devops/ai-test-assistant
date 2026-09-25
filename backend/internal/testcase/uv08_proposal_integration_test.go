//go:build integration

package testcase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUV08ProposalLifecycleCASReplayAndPinnedRelease(t *testing.T) {
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
	f := createUV02Fixture(t, ctx, pool)
	// These tests retain immutable proof fixtures, but never leave active queue work.
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE document_workflow_jobs SET status='CANCELED',finished_at=NOW() WHERE document_set_id=$1 AND status IN ('QUEUED','RUNNING')`, f.setID)
	}()
	r := NewRepository(pool)
	s := &Service{repository: r}
	suite := Suite{ID: f.suiteID, DocumentSetID: f.setID}
	output := uv02Proposal(f, "Order scenario", "home")
	output.ExpectedResult = "An order is created"
	var sourceRevision int64
	var reviewHash string
	if err := pool.QueryRow(ctx, `SELECT source_revision FROM document_sets WHERE id=$1`, f.setID).Scan(&sourceRevision); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT requirement_review_hash($1)`, f.requirementID).Scan(&reviewHash); err != nil {
		t.Fatal(err)
	}
	output.Generation = &GenerationProvenance{Provider: "fixture", Model: "m1", PromptVersion: "p1", PromptHash: hash("prompt"), RequirementReviewHash: reviewHash}
	jobNumber := 0
	newJob := func(targets []GenerationTarget) GenerationBaseline {
		t.Helper()
		jobNumber++
		input := GenerationBaseline{ReviewProposals: true, WorkflowAttempt: 1, SourceRevision: sourceRevision, InputHash: hash(fmt.Sprint(jobNumber)), Targets: targets, IncludeRetire: true}
		err := pool.QueryRow(ctx, `INSERT INTO document_workflow_jobs(document_set_id,operation,status,input_snapshot,input_hash,requested_by,idempotency_key,attempt_count,lease_expires_at)
			VALUES($1,'GENERATE_TESTCASES','RUNNING','{}',$2,'qa',$3,1,NOW()+INTERVAL '5 minutes') RETURNING id`, f.setID, input.InputHash, fmt.Sprintf("uv08-proposal-%d", jobNumber)).Scan(&input.WorkflowJobID)
		if err != nil {
			t.Fatal(err)
		}
		return input
	}
	finish := func(input GenerationBaseline) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE document_workflow_jobs SET status='SUCCEEDED',finished_at=NOW() WHERE id=$1`, input.WorkflowJobID); err != nil {
			t.Fatal(err)
		}
	}
	pin := func(id int64) GenerationTarget {
		t.Helper()
		family, err := r.GetFamily(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return GenerationTarget{FamilyID: family.ID, RevisionID: *family.HeadRevisionID, HeadToken: family.HeadToken}
	}
	persist := func(input GenerationBaseline, outputs []Proposal) []GenerationProposal {
		t.Helper()
		count, err := r.saveProposals(ctx, suite, input, outputs)
		if err != nil {
			t.Fatal(err)
		}
		page, err := s.ListProposals(ctx, f.setID, input.WorkflowJobID, 0, 50)
		if err != nil || len(page.Proposals) != count {
			t.Fatalf("proposal read=%+v %v", page, err)
		}
		return page.Proposals
	}
	input := newJob(nil)
	first := persist(input, []Proposal{output})[0]
	if first.Classification != "NEW_CASE" {
		t.Fatal(first.Classification)
	}
	families, _ := r.ListFamilies(ctx, f.setID)
	if len(families) != 0 {
		t.Fatal("generation proposal mutated working cases")
	}
	decision := ProposalDecision{Decision: "CREATE_NEW", Reason: "Reviewed source and scenario"}
	if _, err := s.DecideProposal(ctx, first.ID, decision, "apply-first", "qa"); !errors.Is(err, ErrProposalConflict) {
		t.Fatalf("applied unfinished job: %v", err)
	}
	finish(input)
	// Concurrent retries with the same key must create exactly one draft.
	var wg sync.WaitGroup
	results := make(chan GenerationProposal, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := s.DecideProposal(ctx, first.ID, decision, "apply-first", "trusted-qa")
			results <- p
			failures <- e
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var v1ID int64
	for result := range results {
		if result.ResultRevisionID == nil {
			t.Fatal("missing applied revision")
		}
		if v1ID != 0 && v1ID != *result.ResultRevisionID {
			t.Fatal("retry duplicated revision")
		}
		v1ID = *result.ResultRevisionID
	}
	v1detail, err := r.Get(ctx, v1ID)
	if err != nil {
		t.Fatal(err)
	}
	v1 := v1detail.TestCase
	if v1.Status != StatusDraft || v1.VersionNumber != 1 || v1.AutomationStatus == "AUTOMATED" {
		t.Fatal("proposal inherited approval/readiness")
	}
	changed := decision
	changed.Reason = "Different request"
	if _, err := s.DecideProposal(ctx, first.ID, changed, "apply-first", "qa"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("key reuse=%v", err)
	}
	if _, err := s.DecideProposal(ctx, first.ID, decision, "another-key", "qa"); !errors.Is(err, ErrProposalConflict) {
		t.Fatalf("decided twice=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE test_case_generation_proposals SET content='{}' WHERE id=$1`, first.ID); err == nil {
		t.Fatal("proposal proof mutated")
	}
	if _, err := r.ReviewExact(ctx, v1.ID, ReviewInput{ReviewerName: "QA", Decision: StatusApproved, ExpectedContentHash: v1.ContentHash}, "qa"); err != nil {
		t.Fatal(err)
	}
	r1, _, err := r.PublishRelease(ctx, f.setID, PublishReleaseInput{TestSuiteID: f.suiteID, RevisionIDs: []int64{v1.ID}, PublishedBy: "QA"}, "r1", "qa")
	if err != nil {
		t.Fatal(err)
	}
	input = newJob([]GenerationTarget{pin(v1.FamilyID)})
	unchanged := persist(input, []Proposal{output})[0]
	if unchanged.Classification != "UNCHANGED" {
		t.Fatalf("unchanged=%+v", unchanged)
	}
	finish(input)
	keep := ProposalDecision{Decision: "KEEP", TargetFamilyID: v1.FamilyID, ExpectedHeadRevisionID: v1.ID, Reason: "No change"}
	if _, err := s.DecideProposal(ctx, unchanged.ID, keep, "keep", "qa"); err != nil {
		t.Fatal(err)
	}
	input = newJob([]GenerationTarget{pin(v1.FamilyID)})
	modified := output
	modified.Title = "Revised order scenario"
	ambiguous := persist(input, []Proposal{modified})[0]
	if ambiguous.Classification != "AMBIGUOUS_MATCH" || len(ambiguous.Candidates) != 1 {
		t.Fatalf("unsafe lineage=%+v", ambiguous)
	}
	finish(input)
	revise := ProposalDecision{Decision: "REVISE", TargetFamilyID: v1.FamilyID, ExpectedHeadRevisionID: v1.ID, Reason: "Confirmed same scenario"}
	applied, err := s.DecideProposal(ctx, ambiguous.ID, revise, "v2", "qa")
	if err != nil {
		t.Fatal(err)
	}
	v2detail, err := r.Get(ctx, *applied.ResultRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	v2 := v2detail.TestCase
	if v2.VersionNumber != 2 || v2.Status != StatusDraft || v2.FamilyID != v1.FamilyID {
		t.Fatalf("bad successor=%+v", v2)
	}
	if _, err := r.ReviewExact(ctx, v2.ID, ReviewInput{ReviewerName: "QA", Decision: StatusApproved, ExpectedContentHash: v2.ContentHash}, "qa"); err != nil {
		t.Fatal(err)
	}
	r2, _, err := r.PublishRelease(ctx, f.setID, PublishReleaseInput{TestSuiteID: f.suiteID, RevisionIDs: []int64{v2.ID}, PublishedBy: "QA"}, "r2", "qa")
	if err != nil {
		t.Fatal(err)
	}
	// A head changed after input pinning is not silently rebased at persist or apply.
	input = newJob([]GenerationTarget{pin(v1.FamilyID)})
	humanTitle := "Human edit while AI runs"
	human, err := r.CreateRevision(ctx, v1.FamilyID, CreateRevisionInput{BaseRevisionID: v2.ID, ExpectedHeadRevisionID: v2.ID, Patch: &RevisionPatch{Title: &humanTitle}, Reason: "Human edit"}, "human", "qa")
	if err != nil {
		t.Fatal(err)
	}
	stale := persist(input, []Proposal{output})[0]
	finish(input)
	comparison, err := s.CompareProposal(ctx, stale.ID, v1.FamilyID)
	if err != nil || comparison.Target.RevisionID != v2.ID || comparison.Before.Title != v2.Title || comparison.After.Title != output.Title {
		t.Fatalf("comparison rebased onto live head: %+v %v", comparison, err)
	}
	if _, err := s.CompareProposal(ctx, stale.ID, 999999); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign comparison target accepted: %v", err)
	}
	revise.ExpectedHeadRevisionID = v2.ID
	if _, err := s.DecideProposal(ctx, stale.ID, revise, "stale", "qa"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale apply=%v", err)
	}
	family, _ := r.GetFamily(ctx, v1.FamilyID)
	if *family.HeadRevisionID != human.Revision.ID {
		t.Fatal("human head overwritten")
	}
	for _, release := range []SuiteRelease{r1, r2} {
		current, err := r.GetRelease(ctx, release.ID)
		if err != nil || current.ManifestHash != release.ManifestHash {
			t.Fatal("release proof changed")
		}
	}
	// Same content but a different model is not UNCHANGED.
	input = newJob([]GenerationTarget{pin(v1.FamilyID)})
	modelChange := output
	modelChange.Title = humanTitle
	g := *output.Generation
	g.Model = "m2"
	modelChange.Generation = &g
	changedModel := persist(input, []Proposal{modelChange})[0]
	if changedModel.Classification != "NEW_REVISION" {
		t.Fatalf("generation context ignored: %s", changedModel.Classification)
	}
	finish(input)
	// Wrong scope/target cannot steal another identity; source edits invalidate apply.
	wrong := ProposalDecision{Decision: "REVISE", TargetFamilyID: 999999, ExpectedHeadRevisionID: human.Revision.ID, Reason: "wrong target"}
	if _, err := s.DecideProposal(ctx, changedModel.ID, wrong, "wrong-target", "qa"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign target=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE document_sets SET source_revision=source_revision+1 WHERE id=$1`, f.setID); err != nil {
		t.Fatal(err)
	}
	wrong.TargetFamilyID = v1.FamilyID
	if _, err := s.DecideProposal(ctx, changedModel.ID, wrong, "changed-source", "qa"); !errors.Is(err, ErrProposalConflict) {
		t.Fatalf("source change=%v", err)
	}
	if _, err := s.DecideProposal(ctx, changedModel.ID, ProposalDecision{Decision: "DISMISS", Reason: "Source superseded"}, "dismiss", "qa"); err != nil {
		t.Fatal(err)
	}
	// Keep historical run proof while all source requirements of an identity retire.
	runID := insertUV03ID(t, pool, ctx, `INSERT INTO test_runs(test_suite_id,suite_release_id,status) VALUES($1,$2,'COMPLETED') RETURNING id`, f.suiteID, r1.ID)
	if _, err := pool.Exec(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,status,expected_result_snapshot,expected_result_hash,actual_result) VALUES($1,$2,'PASSED',$3,$4,'historical pass')`, runID, v1.ID, v1.ExpectedResult, v1.ExpectedResultHash); err != nil {
		t.Fatal(err)
	}
	var newRequirement int64
	if err := pool.QueryRow(ctx, `INSERT INTO requirements(document_set_id,requirement_key,title,statement,requirement_type,flow_type,status,source_snapshot_id)
		SELECT document_set_id,'REQ-UV08-NEW',title,statement,requirement_type,flow_type,'APPROVED',source_snapshot_id FROM requirements WHERE id=$1 RETURNING id`, f.requirementID).Scan(&newRequirement); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO requirement_evidence(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
		SELECT $2,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash FROM requirement_evidence WHERE requirement_id=$1`, f.requirementID, newRequirement); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE requirements SET source_state='REMOVED' WHERE id=$1`, f.requirementID); err != nil {
		t.Fatal(err)
	}
	sourceRevision++
	newOutput := output
	newOutput.RequirementIDs = []int64{newRequirement}
	newGeneration := *output.Generation
	if err := pool.QueryRow(ctx, `SELECT requirement_review_hash($1)`, newRequirement).Scan(&newGeneration.RequirementReviewHash); err != nil {
		t.Fatal(err)
	}
	newOutput.Generation = &newGeneration
	selected := newJob([]GenerationTarget{pin(v1.FamilyID)})
	selected.IncludeRetire = false
	selectedProposals := persist(selected, []Proposal{newOutput})
	finish(selected)
	if len(selectedProposals) != 1 || selectedProposals[0].Classification != "NEW_CASE" {
		t.Fatal("selected generation inferred retirement from omitted source")
	}
	input = newJob([]GenerationTarget{pin(v1.FamilyID)})
	proposals := persist(input, []Proposal{newOutput})
	finish(input)
	var retire, newCase GenerationProposal
	for _, p := range proposals {
		if p.Classification == "RETIRE_CANDIDATE" {
			retire = p
		} else if p.Classification == "NEW_CASE" {
			newCase = p
		}
	}
	if retire.ID == 0 {
		t.Fatal("missing retire candidate")
	}
	archive := ProposalDecision{Decision: "ARCHIVE", TargetFamilyID: v1.FamilyID, ExpectedHeadRevisionID: human.Revision.ID, Reason: "Removed source reviewed"}
	if _, err := s.DecideProposal(ctx, retire.ID, archive, "archive", "qa"); err != nil {
		t.Fatal(err)
	}
	family, _ = r.GetFamily(ctx, v1.FamilyID)
	if !family.Archived || family.RevisionCounter != 3 {
		t.Fatal("archive removed history")
	}
	runs, err := s.RevisionRuns(ctx, v1.ID, false, 0)
	if err != nil || len(runs.Runs) != 1 || runs.Runs[0].Status != "PASSED" {
		t.Fatal("archive changed old run")
	}
	for _, release := range []SuiteRelease{r1, r2} {
		current, err := r.GetRelease(ctx, release.ID)
		if err != nil || current.ManifestHash != release.ManifestHash {
			t.Fatal("retire changed pinned release")
		}
	}
	// Distinct jobs proposed the same new content before either applied. Only one
	// may create an identity; the losing proposal stays pending for fresh review.
	duplicateResults := make(chan error, 2)
	for _, p := range []GenerationProposal{newCase, selectedProposals[0]} {
		wg.Add(1)
		go func(p GenerationProposal) {
			defer wg.Done()
			_, err := s.DecideProposal(ctx, p.ID, decision, fmt.Sprintf("duplicate-%d", p.ID), "qa")
			duplicateResults <- err
		}(p)
	}
	wg.Wait()
	close(duplicateResults)
	succeeded, conflicts := 0, 0
	for err := range duplicateResults {
		if err == nil {
			succeeded++
		} else if errors.Is(err, ErrProposalConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicts != 1 {
		t.Fatalf("duplicate create successes=%d conflicts=%d", succeeded, conflicts)
	}
	// An invalid second result rolls back the first proposal too.
	input = newJob(nil)
	bad := newOutput
	bad.ExpectedResult = "unsupported expected"
	if _, err := r.saveProposals(ctx, suite, input, []Proposal{newOutput, bad}); err == nil {
		t.Fatal("ungrounded output accepted")
	}
	page, err := s.ListProposals(ctx, f.setID, input.WorkflowJobID, 0, 20)
	if err != nil || len(page.Proposals) != 0 {
		t.Fatal("partial proposal batch escaped rollback")
	}
	if _, err := pool.Exec(ctx, `UPDATE document_workflow_jobs SET cancel_requested_at=NOW() WHERE id=$1`, input.WorkflowJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.saveProposals(ctx, suite, input, []Proposal{newOutput}); !errors.Is(err, ErrGenerationLease) {
		t.Fatalf("cancel raced publication: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE document_workflow_jobs SET cancel_requested_at=NULL,lease_expires_at=NOW()-INTERVAL '1 second' WHERE id=$1`, input.WorkflowJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.saveProposals(ctx, suite, input, []Proposal{newOutput}); !errors.Is(err, ErrGenerationLease) {
		t.Fatalf("expired lease published: %v", err)
	}
}
