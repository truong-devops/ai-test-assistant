//go:build integration

package testcase

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func TestUV07WorkspacePreviewHistoryAndRevisionIsolation(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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
	svc := &Service{repository: repo}
	proposal := uv02Proposal(f, "Order validation", "invalid coupon")
	proposal.AutomationStatus = "AUTOMATED"
	v1, _, err := repo.SaveGenerated(ctx, Suite{ID: f.suiteID, DocumentSetID: f.setID}, proposal)
	if err != nil {
		t.Fatal(err)
	}
	approve := func(v TestCase) {
		t.Helper()
		for i := 0; i < 2; i++ {
			if _, e := repo.ReviewExact(ctx, v.ID, ReviewInput{ReviewerName: "display", Decision: StatusApproved, ExpectedContentHash: v.ContentHash}, "trusted-qa"); e != nil {
				t.Fatal(e)
			}
		}
		var count int
		if e := pool.QueryRow(ctx, `SELECT count(*) FROM test_case_reviews WHERE test_case_id=$1`, v.ID).Scan(&count); e != nil || count != 1 {
			t.Fatalf("review retry duplicated audit: %d %v", count, e)
		}
	}
	approve(v1)
	in := PublishReleaseInput{TestSuiteID: f.suiteID, RevisionIDs: []int64{v1.ID}, PublishedBy: "QA"}
	preview, err := svc.PreviewRelease(ctx, f.setID, in, "qa")
	if err != nil || preview.PreviewHash == "" || preview.CoveredRequirementCount != 1 {
		t.Fatalf("preview=%+v %v", preview, err)
	}
	releases, _ := repo.ListReleases(ctx, f.setID)
	if len(releases) != 0 {
		t.Fatal("preview wrote release")
	}
	in.ExpectedPreviewHash = "wrong"
	if _, _, err = repo.PublishRelease(ctx, f.setID, in, "wrong-preview", "qa"); !errors.Is(err, ErrPreviewChanged) {
		t.Fatalf("wrong preview=%v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE document_sets SET source_revision=source_revision+1 WHERE id=$1`, f.setID); err != nil {
		t.Fatal(err)
	}
	in.ExpectedPreviewHash = preview.PreviewHash
	if _, _, err = repo.PublishRelease(ctx, f.setID, in, "stale-preview", "qa"); !errors.Is(err, ErrPreviewChanged) {
		t.Fatalf("stale source preview=%v", err)
	}
	preview, err = svc.PreviewRelease(ctx, f.setID, in, "qa")
	if err != nil {
		t.Fatal(err)
	}
	in.ExpectedPreviewHash = preview.PreviewHash
	r1, _, err := repo.PublishRelease(ctx, f.setID, in, "r1", "qa")
	if err != nil {
		t.Fatal(err)
	}
	badExpected := "An unsupported promise"
	_, err = repo.CreateRevision(ctx, v1.FamilyID, CreateRevisionInput{BaseRevisionID: v1.ID, ExpectedHeadRevisionID: v1.ID, Patch: &RevisionPatch{ExpectedResult: &badExpected}, Reason: "unsupported"}, "bad-expected", "qa")
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "expected_result" {
		t.Fatalf("expected validation=%v", err)
	}
	title := "Updated order validation"
	steps := []StepInput{{Action: "First action", ExpectedResult: "First result"}, {Action: "Second action", ExpectedResult: "Second result"}}
	next, err := repo.CreateRevision(ctx, v1.FamilyID, CreateRevisionInput{BaseRevisionID: v1.ID, ExpectedHeadRevisionID: v1.ID, Patch: &RevisionPatch{Title: &title, Steps: &steps}, Reason: "more explicit steps"}, "v2", "qa")
	if err != nil {
		t.Fatal(err)
	}
	v2 := next.Revision
	if v2.Status != StatusDraft || v2.AutomationStatus == "AUTOMATED" {
		t.Fatalf("new revision inherited readiness: %+v", v2)
	}
	if _, err = repo.CreateRevision(ctx, v1.FamilyID, CreateRevisionInput{BaseRevisionID: v1.ID, ExpectedHeadRevisionID: v1.ID, Patch: &RevisionPatch{Title: &title}, Reason: "stale form"}, "stale-form", "qa"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale edit=%v", err)
	}
	page, err := svc.History(ctx, v1.FamilyID, 0, 1)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Revision.ID != v2.ID || page.NextBefore != 2 {
		t.Fatalf("page=%+v %v", page, err)
	}
	page, err = svc.History(ctx, v1.FamilyID, page.NextBefore, 1)
	if err != nil || page.Entries[0].Revision.ID != v1.ID || len(page.Entries[0].Releases) != 1 {
		t.Fatalf("old page=%+v %v", page, err)
	}
	if _, err = svc.History(ctx, v1.FamilyID, 0, 51); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("unbounded history accepted")
	}
	diff, err := svc.Comparison(ctx, v1.FamilyID, v1.ID, v2.ID, 0, 1)
	if err != nil || len(diff.Changes) != 1 || diff.NextOffset == nil || diff.Total < 3 {
		t.Fatalf("diff=%+v %v", diff, err)
	}
	other, _, err := repo.SaveGenerated(ctx, Suite{ID: f.suiteID, DocumentSetID: f.setID}, uv02Proposal(f, "Other identity", "other"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Comparison(ctx, v1.FamilyID, v1.ID, other.ID, 0, 20); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("cross-family diff accepted")
	}
	approve(v2)
	in.RevisionIDs = []int64{v2.ID}
	preview, err = svc.PreviewRelease(ctx, f.setID, in, "qa")
	if err != nil {
		t.Fatal(err)
	}
	in.ExpectedPreviewHash = preview.PreviewHash
	r2, _, err := repo.PublishRelease(ctx, f.setID, in, "r2", "qa")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := repo.Restore(ctx, v1.FamilyID, RestoreInput{FromRevisionID: v1.ID, ExpectedHeadRevisionID: v2.ID, Reason: "Restore reviewed v1 design"}, "v3", "qa")
	if err != nil || restored.Revision.VersionNumber != 3 || restored.Revision.Status != StatusDraft || restored.Revision.RestoredFromID == nil || *restored.Revision.RestoredFromID != v1.ID {
		t.Fatalf("restore=%+v %v", restored, err)
	}
	pinned, _ := repo.GetRelease(ctx, r2.ID)
	if pinned.ManifestHash != r2.ManifestHash || pinned.Items[0].TestCaseID != v2.ID {
		t.Fatal("restore changed R2")
	}
	runID := insertUV03ID(t, pool, ctx, `INSERT INTO test_runs(test_suite_id,suite_release_id,status) VALUES($1,$2,'COMPLETED') RETURNING id`, f.suiteID, r1.ID)
	if _, err = pool.Exec(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,status,expected_result_snapshot,expected_result_hash,actual_result) VALUES($1,$2,'PASSED',$3,$4,'historical pass')`, runID, v1.ID, v1.ExpectedResult, v1.ExpectedResultHash); err != nil {
		t.Fatal(err)
	}
	exact, err := svc.RevisionRuns(ctx, restored.Revision.ID, false, 0)
	if err != nil || len(exact.Runs) != 0 {
		t.Fatal("restored revision inherited run")
	}
	all, err := svc.RevisionRuns(ctx, restored.Revision.ID, true, 0)
	if err != nil || len(all.Runs) != 1 || all.Runs[0].TestCaseID != v1.ID {
		t.Fatalf("family scope=%+v %v", all, err)
	}
	historical, err := repo.Get(ctx, v1.ID)
	if err != nil || historical.TestCase.LatestExecution == nil || historical.TestCase.LatestExecution.Status != "PASSED" {
		t.Fatalf("historical execution missing: %+v %v", historical.TestCase.LatestExecution, err)
	}
	family, err := repo.GetFamily(ctx, v1.FamilyID)
	if err != nil || family.LatestRevision.LatestExecution != nil {
		t.Fatalf("family head inherited old PASS: %+v %v", family, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO test_run_items(test_run_id,test_case_id,status,expected_result_snapshot,expected_result_hash,actual_result) VALUES($1,$2,'BLOCKED',$3,$4,'new revision blocked')`, runID, restored.Revision.ID, restored.Revision.ExpectedResult, restored.Revision.ExpectedResultHash); err != nil {
		t.Fatal(err)
	}
	family, err = repo.GetFamily(ctx, v1.FamilyID)
	if err != nil || family.LatestRevision.LatestExecution == nil || family.LatestRevision.LatestExecution.Status != "BLOCKED" {
		t.Fatalf("family head lost own run state: %+v %v", family, err)
	}
	if _, err = repo.Archive(ctx, v1.FamilyID, ArchiveInput{Archived: true, Reason: "retire"}, "qa"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ReviewExact(ctx, restored.Revision.ID, ReviewInput{ReviewerName: "QA", Decision: StatusApproved, ExpectedContentHash: restored.Revision.ContentHash}, "qa"); !errors.Is(err, ErrFamilyArchived) {
		t.Fatalf("archived approval=%v", err)
	}
	page, err = svc.History(ctx, v1.FamilyID, 0, 20)
	if err != nil || len(page.Entries) != 3 {
		t.Fatal("archive lost history")
	}
}
