//go:build integration

package testcase

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUV03PublishedReleasePinsExactApprovedRevisions(t *testing.T) {
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
	f := createUV02Fixture(t, ctx, pool)
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, f.setID) }()
	repository := NewRepository(pool)
	suite := Suite{ID: f.suiteID, DocumentSetID: f.setID}

	first, created, err := repository.SaveGenerated(ctx, suite,
		uv02Proposal(f, "Reject expired coupon", "coupon=expired"))
	if err != nil || !created {
		t.Fatalf("create first revision: created=%v err=%v", created, err)
	}
	second, created, err := repository.SaveGenerated(ctx, suite,
		uv02Proposal(f, "Reject missing address", "address=missing"))
	if err != nil || !created {
		t.Fatalf("create second revision: created=%v err=%v", created, err)
	}
	for _, item := range []TestCase{first, second} {
		if _, err := repository.ReviewExact(ctx, item.ID, ReviewInput{ReviewerName: "QA",
			Decision: StatusApproved, ExpectedContentHash: item.ContentHash}, "qa"); err != nil {
			t.Fatalf("approve revision %d: %v", item.ID, err)
		}
	}
	release1, created, err := repository.PublishRelease(ctx, f.setID, PublishReleaseInput{
		TestSuiteID: f.suiteID, RevisionIDs: []int64{first.ID, second.ID}, PublishedBy: "QA",
	}, "uv03-release-1", "qa")
	if err != nil || !created || release1.ReleaseNumber != 1 || len(release1.Items) != 2 ||
		release1.ScopeStatus != ReleaseScopeComplete {
		t.Fatalf("publish R1: release=%+v created=%v err=%v", release1, created, err)
	}
	replayed, created, err := repository.PublishRelease(ctx, f.setID, PublishReleaseInput{
		TestSuiteID: f.suiteID, RevisionIDs: []int64{first.ID, second.ID}, PublishedBy: "QA",
	}, "uv03-release-1", "qa")
	if err != nil || created || replayed.ID != release1.ID {
		t.Fatalf("replay R1: release=%+v created=%v err=%v", replayed, created, err)
	}
	runID := insertUV03ID(t, pool, ctx, `INSERT INTO test_runs
		(test_suite_id,suite_release_id,source_sha,status,started_at,finished_at)
		VALUES($1,$2,'release-r1','COMPLETED',NOW(),NOW()) RETURNING id`, f.suiteID, release1.ID)
	if _, err := pool.Exec(ctx, `INSERT INTO test_run_items
		(test_run_id,test_case_id,status,expected_result_snapshot,expected_result_hash,actual_result)
		VALUES($1,$2,'PASSED',$3,$4,'Expired coupon was rejected')`, runID, first.ID,
		first.ExpectedResult, first.ExpectedResultHash); err != nil {
		t.Fatalf("save exact R1 execution: %v", err)
	}
	working, err := repository.List(ctx, f.setID)
	if err != nil {
		t.Fatalf("list working revisions with execution: %v", err)
	}
	firstWorking := findUV03Revision(working, first.ID)
	if firstWorking == nil || firstWorking.LatestExecution == nil ||
		firstWorking.LatestExecution.Status != "PASSED" ||
		firstWorking.LatestExecution.ActualResult != "Expired coupon was rejected" {
		t.Fatalf("exact revision execution missing: %+v", firstWorking)
	}

	family, err := repository.GetFamily(ctx, first.FamilyID)
	if err != nil || family.HeadRevisionID == nil {
		t.Fatal(err)
	}
	updatedData := "coupon=already-used"
	v2, err := repository.CreateRevision(ctx, first.FamilyID, CreateRevisionInput{
		BaseRevisionID: first.ID, ExpectedHeadRevisionID: first.ID,
		ExpectedHeadToken: family.HeadToken, Patch: &RevisionPatch{TestData: &updatedData},
		Reason: "Update coupon fixture",
	}, "uv03-first-v2", "qa")
	if err != nil || !v2.Created {
		t.Fatalf("create v2: result=%+v err=%v", v2, err)
	}
	working, err = repository.List(ctx, f.setID)
	if err != nil {
		t.Fatalf("list working revisions after successor: %v", err)
	}
	v2Working := findUV03Revision(working, v2.Revision.ID)
	if v2Working == nil || v2Working.LatestExecution != nil {
		t.Fatalf("draft successor inherited R1 execution: %+v", v2Working)
	}
	if _, err := repository.ReviewExact(ctx, v2.Revision.ID, ReviewInput{ReviewerName: "QA",
		Decision: StatusApproved, ExpectedContentHash: v2.Revision.ContentHash}, "qa"); err != nil {
		t.Fatalf("approve v2: %v", err)
	}

	stillR1, err := repository.GetRelease(ctx, release1.ID)
	if err != nil || stillR1.Items[0].TestCaseID != first.ID {
		t.Fatalf("R1 changed after v2: %+v err=%v", stillR1, err)
	}
	release2, created, err := repository.PublishRelease(ctx, f.setID, PublishReleaseInput{
		TestSuiteID: f.suiteID, RevisionIDs: []int64{v2.Revision.ID, second.ID}, PublishedBy: "QA",
	}, "uv03-release-2", "qa")
	if err != nil || !created || release2.ReleaseNumber != 2 || release2.ID == release1.ID {
		t.Fatalf("publish R2: release=%+v created=%v err=%v", release2, created, err)
	}
	if _, _, err := repository.PublishRelease(ctx, f.setID, PublishReleaseInput{
		TestSuiteID: f.suiteID, RevisionIDs: []int64{first.ID, v2.Revision.ID},
		PublishedBy: "QA",
	}, "uv03-duplicate-family", "qa"); !errors.Is(err, ErrReleaseScope) {
		t.Fatalf("two revisions from one family error=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE test_suite_release_items SET ordinal=9
		WHERE release_id=$1 AND test_case_id=$2`, release1.ID, first.ID); err == nil {
		t.Fatal("published release item accepted mutation")
	}
}

func insertUV03ID(t *testing.T, pool *pgxpool.Pool, ctx context.Context,
	query string, args ...any,
) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func findUV03Revision(items []TestCase, id int64) *TestCase {
	for index := range items {
		if items[index].ID == id {
			return &items[index]
		}
	}
	return nil
}
