//go:build integration

package testcase

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
	"os"
	"testing"
	"time"
)

func TestUV06RequirementRevisionMarksCaseButPreservesReleaseAndRun(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	f := createUV02Fixture(t, ctx, pool)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM document_source_snapshot_items WHERE document_set_id=$1`, f.setID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, f.setID)
	}()
	repo := NewRepository(pool)
	suite := Suite{ID: f.suiteID, DocumentSetID: f.setID}
	first, _, err := repo.SaveGenerated(ctx, suite, uv02Proposal(f, "Reject old order", "order=old"))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := repo.SaveGenerated(ctx, suite, uv02Proposal(f, "Reject invalid order", "order=invalid"))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []TestCase{first, second} {
		if _, err = repo.ReviewExact(ctx, item.ID, ReviewInput{ReviewerName: "QA", Decision: StatusApproved, ExpectedContentHash: item.ContentHash}, "qa"); err != nil {
			t.Fatal(err)
		}
	}
	release, _, err := repo.PublishRelease(ctx, f.setID, PublishReleaseInput{TestSuiteID: f.suiteID, RevisionIDs: []int64{first.ID}, PublishedBy: "QA"}, "before-source-edit", "qa")
	if err != nil {
		t.Fatal(err)
	}
	runID := insertUV03ID(t, pool, ctx, `INSERT INTO test_runs(test_suite_id,suite_release_id,source_sha,status) VALUES($1,$2,'uv06','COMPLETED') RETURNING id`, f.suiteID, release.ID)
	if _, err = pool.Exec(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,status,expected_result_snapshot,expected_result_hash,actual_result) VALUES($1,$2,'PASSED',$3,$4,'Rejected old order')`, runID, first.ID, first.ExpectedResult, first.ExpectedResultHash); err != nil {
		t.Fatal(err)
	}
	reqs := requirement.NewService(requirement.NewRepository(pool), nil, nil)
	before, err := reqs.Get(ctx, f.requirementID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := reqs.Review(ctx, f.requirementID, requirement.ReviewInput{ReviewerName: "BA", Decision: requirement.DecisionApproved, ExpectedHash: before.Requirement.ReviewHash, Title: "Updated rule"})
	if err != nil || after.Requirement.ID == before.Requirement.ID {
		t.Fatalf("edit=%+v %v", after.Requirement, err)
	}
	detail, err := repo.Get(ctx, first.ID)
	if err != nil || !detail.TestCase.NeedsSourceReview || detail.TestCase.ContentHash != first.ContentHash {
		t.Fatalf("case proof=%+v %v", detail.TestCase, err)
	}
	if _, _, err = repo.PublishRelease(ctx, f.setID, PublishReleaseInput{TestSuiteID: f.suiteID, RevisionIDs: []int64{second.ID}, PublishedBy: "QA", ScopeDecision: "Subset acknowledged"}, "stale-source-release", "qa"); !errors.Is(err, ErrReviewBlocked) {
		t.Fatalf("stale release accepted: %v", err)
	}
	historical, err := repo.GetRelease(ctx, release.ID)
	if err != nil || historical.ManifestHash != release.ManifestHash {
		t.Fatalf("historical release=%+v %v", historical, err)
	}
	var expected, actual string
	if err = pool.QueryRow(ctx, `SELECT expected_result_snapshot,actual_result FROM test_run_items WHERE test_run_id=$1`, runID).Scan(&expected, &actual); err != nil || expected != first.ExpectedResult || actual != "Rejected old order" {
		t.Fatalf("run history changed: %s %s %v", expected, actual, err)
	}
}
