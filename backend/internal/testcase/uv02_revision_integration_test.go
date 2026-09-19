//go:build integration

package testcase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type uv02Fixture struct {
	setID, suiteID, requirementID  int64
	evidenceID, versionID, blockID int64
}

func createUV02Fixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uv02Fixture {
	t.Helper()
	var result uv02Fixture
	name := "uv02-revisions-" + time.Now().Format("20060102150405.000000000")
	if err := pool.QueryRow(ctx, `INSERT INTO document_sets(name) VALUES($1) RETURNING id`, name).
		Scan(&result.setID); err != nil {
		t.Fatal(err)
	}
	var documentID, snapshotID int64
	if err := pool.QueryRow(ctx, `INSERT INTO documents(document_set_id,name,document_type)
		VALUES($1,'UV-02 source','REQUIREMENTS') RETURNING id`, result.setID).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO document_versions
		(document_id,document_set_id,version_number,original_filename,media_type,size_bytes,
		 sha256,storage_key,approval_status,parse_status)
		VALUES($1,$2,1,'uv02.md','text/markdown',1,$3,$4,'APPROVED','PARSED') RETURNING id`,
		documentID, result.setID, strings.Repeat("1", 64), fmt.Sprintf("uv02/%d/v1.md", result.setID)).
		Scan(&result.versionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO document_blocks
		(document_version_id,ordinal,block_type,content,source_locator)
		VALUES($1,1,'PARAGRAPH','An order is created','line:1') RETURNING id`, result.versionID).
		Scan(&result.blockID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO document_source_snapshots
		(document_set_id,source_revision,fingerprint,included_count,excluded_count)
		SELECT id,source_revision,$2,1,0 FROM document_sets WHERE id=$1 RETURNING id`,
		result.setID, strings.Repeat("2", 64)).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO document_source_snapshot_items
		(source_snapshot_id,document_set_id,document_id,document_name,document_version_id,
		 version_number,sha256,parse_status,approval_status,included)
		VALUES($1,$2,$3,'UV-02 source',$4,1,$5,'PARSED','APPROVED',TRUE)`,
		snapshotID, result.setID, documentID, result.versionID, strings.Repeat("1", 64)); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO requirements
		(document_set_id,requirement_key,title,statement,requirement_type,flow_type,status,
		 source_snapshot_id)
		VALUES($1,'REQ-UV02','Create order','An order is created','FUNCTIONAL','MAIN',
		 'APPROVED',$2) RETURNING id`, result.setID, snapshotID).Scan(&result.requirementID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO requirement_evidence
		(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
		VALUES($1,$2,$3,$4,'line:1',$5) RETURNING id`, result.requirementID, result.setID,
		result.versionID, result.blockID, strings.Repeat("3", 64)).Scan(&result.evidenceID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_suites(document_set_id,name)
		VALUES($1,'UV-02 suite') RETURNING id`, result.setID).Scan(&result.suiteID); err != nil {
		t.Fatal(err)
	}
	return result
}

func uv02Proposal(f uv02Fixture, title, data string) Proposal {
	return Proposal{Title: title, TestType: TypeNegative, Risk: "HIGH", Actor: "Buyer",
		Precondition: "Cart is ready", TestData: data, ExpectedResult: "Order is rejected",
		Postcondition: "No order exists", AutomationStatus: "AUTOMATABLE", Confidence: .9,
		GeneratedBy: "UV02_TEST", Assumptions: []string{"Inventory is available"},
		Steps:          []Step{{Action: "Submit invalid order", ExpectedResult: "Validation error is shown"}},
		RequirementIDs: []int64{f.requirementID}, RequirementKeys: []string{"REQ-UV02"}}
}

func TestUV02RevisionLifecycle(t *testing.T) {
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

	first, created, err := repository.SaveGenerated(ctx, suite, uv02Proposal(f, "Reject invalid coupon", "coupon=expired"))
	if err != nil || !created {
		t.Fatalf("save first scenario: created=%v err=%v", created, err)
	}
	second, created, err := repository.SaveGenerated(ctx, suite, uv02Proposal(f, "Reject invalid address", "address=missing"))
	if err != nil || !created {
		t.Fatalf("save second scenario: created=%v err=%v", created, err)
	}
	if first.FamilyID == second.FamilyID || first.TestCaseKey == second.TestCaseKey {
		t.Fatalf("distinct negative scenarios were merged: first=%+v second=%+v", first, second)
	}
	families, err := repository.ListFamilies(ctx, f.setID)
	if err != nil || len(families) != 2 {
		t.Fatalf("list distinct families: count=%d err=%v", len(families), err)
	}

	family, err := repository.GetFamily(ctx, first.FamilyID)
	if err != nil || family.HeadRevisionID == nil {
		t.Fatalf("get family: %+v err=%v", family, err)
	}
	newData := "coupon=used"
	newSteps := []StepInput{{Action: "Submit used coupon", ExpectedResult: "Coupon error is shown"}}
	v2Result, err := repository.CreateRevision(ctx, first.FamilyID, CreateRevisionInput{
		BaseRevisionID: first.ID, ExpectedHeadRevisionID: first.ID, ExpectedHeadToken: family.HeadToken,
		Patch: &RevisionPatch{TestData: &newData, Steps: &newSteps}, Reason: "Exercise content revision",
	}, "uv02-v2", "qa-a")
	if err != nil || !v2Result.Created || v2Result.Revision.VersionNumber != 2 {
		t.Fatalf("create v2: result=%+v err=%v", v2Result, err)
	}
	noOp, err := repository.CreateRevision(ctx, first.FamilyID, CreateRevisionInput{
		BaseRevisionID: v2Result.Revision.ID, ExpectedHeadRevisionID: v2Result.Revision.ID,
		Patch: &RevisionPatch{TestData: &newData, Steps: &newSteps}, Reason: "No semantic change",
	}, "uv02-noop", "qa-a")
	if err != nil || noOp.Created || noOp.Revision.ID != v2Result.Revision.ID {
		t.Fatalf("no-op revision: result=%+v err=%v", noOp, err)
	}

	var version2ID, block2ID, evidence2ID int64
	if err := pool.QueryRow(ctx, `INSERT INTO document_versions
		(document_id,document_set_id,version_number,original_filename,media_type,size_bytes,
		 sha256,storage_key,approval_status,parse_status)
		SELECT document_id,document_set_id,2,'uv02-v2.md','text/markdown',1,$2,$3,
		 'APPROVED','PARSED' FROM document_versions WHERE id=$1 RETURNING id`, f.versionID,
		strings.Repeat("4", 64), fmt.Sprintf("uv02/%d/v2.md", f.setID)).Scan(&version2ID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO document_blocks
		(document_version_id,ordinal,block_type,content,source_locator)
		VALUES($1,1,'PARAGRAPH','An updated order rule','line:2') RETURNING id`, version2ID).
		Scan(&block2ID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO requirement_evidence
		(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
		VALUES($1,$2,$3,$4,'line:2',$5) RETURNING id`, f.requirementID, f.setID,
		version2ID, block2ID, strings.Repeat("5", 64)).Scan(&evidence2ID); err != nil {
		t.Fatal(err)
	}
	family, _ = repository.GetFamily(ctx, first.FamilyID)
	evidence := []EvidenceRef{{RequirementRevisionID: f.requirementID,
		RequirementEvidenceID: evidence2ID, DocumentVersionID: version2ID,
		DocumentBlockID: block2ID, SourceLocator: "line:2", ExcerptHash: strings.Repeat("5", 64)}}
	v3Result, err := repository.CreateRevision(ctx, first.FamilyID, CreateRevisionInput{
		BaseRevisionID: v2Result.Revision.ID, ExpectedHeadRevisionID: v2Result.Revision.ID,
		ExpectedHeadToken: family.HeadToken, Patch: &RevisionPatch{EvidenceRefs: &evidence},
		Reason: "Update exact evidence citation",
	}, "uv02-v3-evidence", "qa-a")
	if err != nil || !v3Result.Created || v3Result.Revision.ContentHash == v2Result.Revision.ContentHash {
		t.Fatalf("citation revision: result=%+v err=%v", v3Result, err)
	}
	diff, err := repository.Diff(ctx, first.FamilyID, v2Result.Revision.ID, v3Result.Revision.ID)
	if err != nil || len(diff.Changes) != 1 || diff.Changes[0].Field != "evidence_refs" {
		t.Fatalf("citation diff: %+v err=%v", diff, err)
	}

	family, _ = repository.GetFamily(ctx, first.FamilyID)
	v4Result, err := repository.Restore(ctx, first.FamilyID, RestoreInput{FromRevisionID: first.ID,
		ExpectedHeadRevisionID: v3Result.Revision.ID, ExpectedHeadToken: family.HeadToken,
		Reason: "Restore original design"}, "uv02-restore", "qa-a")
	if err != nil || v4Result.Revision.VersionNumber != 4 ||
		v4Result.Revision.ParentRevisionID == nil || *v4Result.Revision.ParentRevisionID != v3Result.Revision.ID ||
		v4Result.Revision.RestoredFromID == nil || *v4Result.Revision.RestoredFromID != first.ID {
		t.Fatalf("restore v1 as v4: result=%+v err=%v", v4Result, err)
	}

	family, _ = repository.GetFamily(ctx, first.FamilyID)
	low := "LOW"
	input := CreateRevisionInput{BaseRevisionID: v4Result.Revision.ID,
		ExpectedHeadRevisionID: v4Result.Revision.ID, ExpectedHeadToken: family.HeadToken,
		Patch: &RevisionPatch{Risk: &low}, Reason: "Lower reviewed risk"}
	v5, err := repository.CreateRevision(ctx, first.FamilyID, input, "uv02-replay", "qa-a")
	if err != nil || !v5.Created {
		t.Fatalf("create idempotent revision: %+v err=%v", v5, err)
	}
	replay, err := repository.CreateRevision(ctx, first.FamilyID, input, "uv02-replay", "qa-a")
	if err != nil || replay.Created || replay.Revision.ID != v5.Revision.ID {
		t.Fatalf("idempotent replay: %+v err=%v", replay, err)
	}
	changedReason := input
	changedReason.Reason = "Different request"
	if _, err := repository.CreateRevision(ctx, first.FamilyID, changedReason, "uv02-replay", "qa-a"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency collision error=%v", err)
	}

	secondFamily, _ := repository.GetFamily(ctx, second.FamilyID)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for index, title := range []string{"Concurrent A", "Concurrent B"} {
		wg.Add(1)
		go func(index int, title string) {
			defer wg.Done()
			_, err := repository.CreateRevision(context.Background(), second.FamilyID,
				CreateRevisionInput{BaseRevisionID: second.ID, ExpectedHeadRevisionID: second.ID,
					ExpectedHeadToken: secondFamily.HeadToken, Patch: &RevisionPatch{Title: &title},
					Reason: "Concurrent update"}, fmt.Sprintf("uv02-concurrent-%d", index), "qa-b")
			results <- err
		}(index, title)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrRevisionConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrency successes=%d conflicts=%d", successes, conflicts)
	}

	foreign := createUV02Fixture(t, ctx, pool)
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, foreign.setID) }()
	firstFamily, _ := repository.GetFamily(ctx, first.FamilyID)
	foreignIDs := []int64{foreign.requirementID}
	if _, err := repository.CreateRevision(ctx, first.FamilyID, CreateRevisionInput{
		BaseRevisionID: v5.Revision.ID, ExpectedHeadRevisionID: v5.Revision.ID,
		ExpectedHeadToken: firstFamily.HeadToken,
		Patch:             &RevisionPatch{RequirementRevisionIDs: &foreignIDs}, Reason: "Invalid foreign source",
	}, "uv02-foreign-source", "qa-a"); !errors.Is(err, ErrNoApprovedSource) {
		t.Fatalf("foreign-set requirement error=%v", err)
	}
	secondFamily, _ = repository.GetFamily(ctx, second.FamilyID)
	if _, err := repository.CreateRevision(ctx, second.FamilyID, CreateRevisionInput{
		BaseRevisionID: first.ID, ExpectedHeadRevisionID: *secondFamily.HeadRevisionID,
		ExpectedHeadToken: secondFamily.HeadToken, Patch: &RevisionPatch{Risk: &low},
		Reason: "Invalid foreign family base",
	}, "uv02-foreign-family", "qa-b"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("foreign-family base error=%v", err)
	}

	var stepID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM test_case_steps WHERE test_case_id=$1 LIMIT 1`, first.ID).
		Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE test_case_steps SET action='mutated' WHERE id=$1`, stepID); err == nil {
		t.Fatal("direct mutation of a sealed revision step succeeded")
	}
	if _, err := pool.Exec(ctx, `UPDATE test_case_evidence_links SET source_locator='mutated'
		WHERE test_case_id=$1`, first.ID); err == nil {
		t.Fatal("direct mutation of sealed revision evidence succeeded")
	}
	archived, err := repository.Archive(ctx, second.FamilyID,
		ArchiveInput{Archived: true, Reason: "Retire duplicate"}, "qa-b")
	if err != nil || !archived.Archived {
		t.Fatalf("archive family: %+v err=%v", archived, err)
	}
	secondFamily, _ = repository.GetFamily(ctx, second.FamilyID)
	archiveTitle := "Blocked while archived"
	if _, err := repository.CreateRevision(ctx, second.FamilyID, CreateRevisionInput{
		BaseRevisionID:         *secondFamily.HeadRevisionID,
		ExpectedHeadRevisionID: *secondFamily.HeadRevisionID,
		ExpectedHeadToken:      secondFamily.HeadToken, Patch: &RevisionPatch{Title: &archiveTitle},
		Reason: "Must not write archived family"}, "uv02-archived", "qa-b"); !errors.Is(err, ErrFamilyArchived) {
		t.Fatalf("archived family write error=%v", err)
	}
	if _, err := repository.Archive(ctx, second.FamilyID,
		ArchiveInput{Archived: false, Reason: "Reactivate after validation"}, "qa-b"); err != nil {
		t.Fatal(err)
	}

	blankStep := uv02Proposal(f, "Strict review evidence", "coupon=other")
	blankStep.Steps[0].ExpectedResult = ""
	strict, _, err := repository.SaveGenerated(ctx, suite, blankStep)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.ReviewExact(ctx, strict.ID, ReviewInput{ReviewerName: "QA",
		Decision: StatusApproved, ExpectedContentHash: strict.ContentHash}, "qa")
	if !errors.Is(err, ErrReviewBlocked) {
		t.Fatalf("approval without step expected evidence error=%v", err)
	}
}
