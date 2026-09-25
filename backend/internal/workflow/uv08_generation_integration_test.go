//go:build integration

package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

func TestUV08ProposalWorkflowPinsTargetsAndReusesCheckpoint(t *testing.T) {
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
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE document_workflow_jobs SET status='CANCELED',finished_at=NOW() WHERE document_set_id=$1 AND status IN ('QUEUED','RUNNING')`, setID)
	}()
	cases := testcase.NewRepository(pool)
	caseService := testcase.NewService(cases, requirement.NewRepository(pool))
	if _, err := caseService.Generate(ctx, setID); err != nil {
		t.Fatal(err)
	}
	before, err := cases.List(ctx, setID)
	if err != nil || len(before) == 0 {
		t.Fatalf("initial cases=%v %v", before, err)
	}
	repo := NewRepository(pool)
	service := NewService(repo, nil, nil, caseService, 3)
	input := OperationInput{Operation: OperationGenerate, ReviewProposals: true, GenerationScope: "ALL"}
	job, created, err := service.Enqueue(ctx, setID, input, "proposal-workflow", "editor")
	if err != nil || !created {
		t.Fatalf("enqueue=%+v %v", job, err)
	}
	var pinned generateInputSnapshot
	if err := json.Unmarshal(job.InputSnapshot, &pinned); err != nil || !pinned.ReviewProposals || len(pinned.Targets) != len(before) {
		t.Fatalf("pinned targets=%s %v", job.InputSnapshot, err)
	}
	if _, _, err := service.Enqueue(ctx, setID, OperationInput{Operation: OperationGenerate}, "proposal-workflow", "editor"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("mode collision=%v", err)
	}
	target := pinned.Targets[0]
	title := "Human edit after generation enqueue"
	human, err := cases.CreateRevision(ctx, target.FamilyID, testcase.CreateRevisionInput{
		BaseRevisionID: target.RevisionID, ExpectedHeadRevisionID: target.RevisionID,
		Patch: &testcase.RevisionPatch{Title: &title}, Reason: "Human edit",
	}, "human-edit", "qa")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateInputSnapshot(ctx, job); err != nil {
		t.Fatalf("head change must not repin/invalidate generation source: %v", err)
	}
	claimed, err := repo.ClaimNext(ctx, time.Minute)
	if err != nil || claimed.ID != job.ID {
		t.Fatalf("claim=%+v %v", claimed, err)
	}
	output, err := service.Process(ctx, claimed)
	if err != nil {
		t.Fatal(err)
	}
	page, err := caseService.ListProposals(ctx, setID, job.ID, 0, 50)
	if err != nil || len(page.Proposals) != len(before) {
		t.Fatalf("proposals=%+v %v", page, err)
	}
	// A crash after atomic proposal persistence can retry without new revisions
	// or duplicate proposal rows. Completion still fences the worker attempt.
	replayOutput, err := service.Process(ctx, claimed)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(output)
	replayJSON, _ := json.Marshal(replayOutput)
	if string(firstJSON) != string(replayJSON) {
		t.Fatalf("checkpoint changed output: %s / %s", firstJSON, replayJSON)
	}
	if err := repo.Complete(ctx, claimed, output); err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, p := range page.Proposals {
		if len(p.Candidates) != 1 || p.Candidates[0].FamilyID != target.FamilyID {
			continue
		}
		matched = true
		if p.Candidates[0] != target || p.Classification != "UNCHANGED" {
			t.Fatalf("comparison used live head: %+v", p)
		}
		_, err := caseService.DecideProposal(ctx, p.ID, testcase.ProposalDecision{Decision: "KEEP", TargetFamilyID: target.FamilyID, ExpectedHeadRevisionID: target.RevisionID, Reason: "Review exact target"}, "stale-keep", "qa")
		if !errors.Is(err, testcase.ErrRevisionConflict) {
			t.Fatalf("stale apply=%v", err)
		}
	}
	if !matched {
		t.Fatal("missing pinned target proposal")
	}
	family, err := cases.GetFamily(ctx, target.FamilyID)
	if err != nil || family.HeadRevisionID == nil || *family.HeadRevisionID != human.Revision.ID {
		t.Fatalf("human head changed=%+v %v", family, err)
	}
	var revisionCount, proposalCount int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM test_cases WHERE document_set_id=$1),(SELECT count(*) FROM test_case_generation_proposals WHERE workflow_job_id=$2)`, setID, job.ID).Scan(&revisionCount, &proposalCount); err != nil {
		t.Fatal(err)
	}
	if revisionCount != len(before)+1 || proposalCount != len(before) {
		t.Fatalf("generation/checkpoint mutated cases: revisions=%d proposals=%d", revisionCount, proposalCount)
	}
}

func TestUV08SelectedGenerationSnapshotOwnershipReplayAndFreshness(t *testing.T) {
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
	foreignSet := workflowFixture(t, ctx, pool)
	defer cleanupWorkflowFixtures(t, pool, setID, foreignSet)
	var reqID, foreignID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM requirements WHERE document_set_id=$1`, setID).Scan(&reqID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM requirements WHERE document_set_id=$1`, foreignSet).Scan(&foreignID); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	service := NewService(repo, nil, nil, nil, 3)
	input := OperationInput{Operation: OperationGenerate, RequirementIDs: []int64{reqID}, RequestedBy: "QA"}
	job, created, err := service.Enqueue(ctx, setID, input, "uv08-selected", "editor")
	if err != nil || !created {
		t.Fatalf("enqueue=%+v %v", job, err)
	}
	var pinned generateInputSnapshot
	if json.Unmarshal(job.InputSnapshot, &pinned) != nil || len(pinned.Requirements) != 1 || pinned.Requirements[0].ID != reqID || len(pinned.SelectedRequirementIDs) != 1 {
		t.Fatalf("snapshot=%s", job.InputSnapshot)
	}
	allSnapshot, allHash, err := repo.BuildGenerateSnapshot(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	var lateID int64
	if err := pool.QueryRow(ctx, `INSERT INTO requirements(document_set_id,requirement_key,title,statement,requirement_type,flow_type,status,source_snapshot_id)
		SELECT document_set_id,'REQ-UV08-LATE',title,statement,requirement_type,flow_type,'APPROVED',source_snapshot_id FROM requirements WHERE id=$1 RETURNING id`, reqID).Scan(&lateID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO requirement_evidence(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
		SELECT $2,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash FROM requirement_evidence WHERE requirement_id=$1`, reqID, lateID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateInputSnapshot(ctx, job); err != nil {
		t.Fatalf("unselected addition invalidated selected input: %v", err)
	}
	if err := repo.ValidateInputSnapshot(ctx, Job{Operation: OperationGenerate, DocumentSetID: setID, InputSnapshot: allSnapshot, InputHash: allHash}); !errors.Is(err, ErrInputStale) {
		t.Fatalf("legacy all-input freshness=%v", err)
	}
	replay, created, err := service.Enqueue(ctx, setID, input, "uv08-selected", "editor")
	if err != nil || created || replay.ID != job.ID {
		t.Fatalf("replay=%+v %v", replay, err)
	}
	if _, _, err := service.Enqueue(ctx, setID, OperationInput{Operation: OperationGenerate}, "uv08-selected", "editor"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("scope key collision=%v", err)
	}
	for _, ids := range [][]int64{{foreignID}, {reqID, foreignID}, {reqID, 9223372036854775807}} {
		_, _, err := repo.BuildGenerateSnapshotForRequirements(ctx, setID, ids)
		var blocked *BlockedError
		if !errors.As(err, &blocked) || blocked.Code != "GENERATION_SELECTION_BLOCKED" {
			t.Fatalf("selection=%v error=%v", ids, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE requirements SET status='DRAFT' WHERE id=$1`, reqID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateInputSnapshot(ctx, job); err == nil {
		t.Fatal("revoked selected approval was accepted")
	}
}
