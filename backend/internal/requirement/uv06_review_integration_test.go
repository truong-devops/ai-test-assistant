//go:build integration

package requirement

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
	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
)

func TestUV06ExactBatchClarificationAndSourceProof(t *testing.T) {
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
	var setID, docID int64
	if err = pool.QueryRow(ctx, `INSERT INTO document_sets(name) VALUES($1) RETURNING id`, fmt.Sprintf("uv06-%d", time.Now().UnixNano())).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM document_source_snapshot_items WHERE document_set_id=$1`, setID)
		if _, err := pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	if err = pool.QueryRow(ctx, `INSERT INTO documents(document_set_id,name,document_type) VALUES($1,'Order rules','REQUIREMENTS') RETURNING id`, setID).Scan(&docID); err != nil {
		t.Fatal(err)
	}
	source := func(number int) (int64, int64) {
		t.Helper()
		var version, block int64
		if err := pool.QueryRow(ctx, `INSERT INTO document_versions(document_id,document_set_id,version_number,original_filename,media_type,size_bytes,sha256,storage_key,approval_status,parse_status,block_count)
     VALUES($1,$2,$3,'source.md','text/markdown',100,$4,$5,'APPROVED','PARSED',1) RETURNING id`, docID, setID, number, hashText(fmt.Sprint(number)), fmt.Sprintf("uv06/%d/%d", setID, number)).Scan(&version); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO document_blocks(document_version_id,ordinal,block_type,content,source_locator) VALUES($1,1,'PARAGRAPH','The system must validate and save an order.','line:1') RETURNING id`, version).Scan(&block); err != nil {
			t.Fatal(err)
		}
		return version, block
	}
	v1, b1 := source(1)
	index := document.NewIndexService(document.NewIndexRepository(pool), knowledge.NewHashEmbeddingClient("uv06", knowledge.EmbeddingDimensions))
	indexed, err := index.Index(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	service := NewService(repo, index, DeterministicExtractor{})
	proposal := func(identifier, statement string) Proposal {
		return Proposal{Identifier: identifier, Title: identifier, Statement: statement, RequirementType: TypeFunctional, FlowType: document.FlowNone, Priority: "MEDIUM", Risk: "MEDIUM", Status: StatusDraft, Confidence: .9, Assumptions: []string{}}
	}
	save := func(p Proposal, v, b int64, snapshot *int64) Requirement {
		t.Helper()
		r, _, err := repo.SaveProposal(ctx, setID, document.SemanticChunk{DocumentSetID: setID, DocumentVersionID: v, DocumentBlockID: b, SourceLocator: "line:1", RawContent: "The system must validate and save an order.", ContentHash: hashText(p.Statement)}, snapshot, p)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	saved := []Requirement{}
	for i := 0; i < 20; i++ {
		saved = append(saved, save(proposal(fmt.Sprintf("REQ-%02d", i), fmt.Sprintf("The system must validate order rule %d.", i)), v1, b1, indexed.SourceSnapshotID))
	}
	tbdProposal := proposal("REQ-TBD", "The system must validate the order limit, to be clarified.")
	tbdProposal.Status = StatusTBD
	tbd := save(tbdProposal, v1, b1, indexed.SourceSnapshotID)
	stale := save(proposal("REQ-STALE", "The system must record the order."), v1, b1, indexed.SourceSnapshotID)
	ambiguous := save(proposal("REQ-AMB", "The system must apply the old discount."), v1, b1, indexed.SourceSnapshotID)
	if err = repo.ReconcileSources(ctx, setID, *indexed.SourceSnapshotID, indexed.Generation); err != nil {
		t.Fatal(err)
	}
	selection := func(id int64) ReviewSelection {
		t.Helper()
		d, err := service.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return ReviewSelection{ID: id, ExpectedHash: d.Requirement.ReviewHash}
	}
	input := BulkReviewInput{Decision: DecisionApproved, ReviewerName: "untrusted-display", Comment: "Evidence checked"}
	for _, r := range saved {
		input.Items = append(input.Items, selection(r.ID))
	}
	input.Items = append(input.Items, selection(tbd.ID), ReviewSelection{ID: stale.ID, ExpectedHash: strings.Repeat("0", 64)})
	var otherSet, foreign int64
	if err = pool.QueryRow(ctx, `INSERT INTO document_sets(name) VALUES('uv06-foreign') RETURNING id`).Scan(&otherSet); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, otherSet)
	if err = pool.QueryRow(ctx, `INSERT INTO requirements(document_set_id,requirement_key,title,statement,requirement_type) VALUES($1,'FOREIGN','foreign','foreign','FUNCTIONAL') RETURNING id`, otherSet).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	input.Items = append(input.Items, selection(foreign))
	results, err := service.BulkReview(ctx, setID, input, "batch-20", "trusted-qa")
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range results {
		if (i < 20 && row.Status != "APPLIED") || (i >= 20 && row.Status != "BLOCKED") {
			t.Fatalf("item %d: %+v", i, row)
		}
	}
	replay, err := service.BulkReview(ctx, setID, input, "batch-20", "trusted-qa")
	if err != nil || len(replay) != 23 {
		t.Fatalf("replay=%+v %v", replay, err)
	}
	var count int
	var actor string
	if err = pool.QueryRow(ctx, `SELECT count(*),min(reviewer_name) FROM requirement_reviews WHERE requirement_id=ANY($1)`, func() []int64 {
		out := []int64{}
		for _, r := range saved {
			out = append(out, r.ID)
		}
		return out
	}()).Scan(&count, &actor); err != nil || count != 20 || actor != "trusted-qa" {
		t.Fatalf("audit count=%d actor=%s err=%v", count, actor, err)
	}
	changed := input
	changed.Decision = DecisionRejected
	if _, err = service.BulkReview(ctx, setID, changed, "batch-20", "trusted-qa"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed command=%v", err)
	}
	if _, err = service.Review(ctx, tbd.ID, ReviewInput{ReviewerName: "qa", Decision: DecisionApproved, Title: "Cosmetic edit"}); !errors.Is(err, ErrReviewBlocked) {
		t.Fatalf("TBD edit bypass=%v", err)
	}
	if _, err = service.Review(ctx, tbd.ID, ReviewInput{ReviewerName: "qa", Decision: DecisionRejected, Title: "Reject/edit bypass"}); !errors.Is(err, ErrReviewBlocked) {
		t.Fatalf("TBD reject/edit bypass=%v", err)
	}
	questions, err := service.ListOpenQuestions(ctx, setID)
	if err != nil || len(questions) != 1 {
		t.Fatalf("questions=%+v %v", questions, err)
	}
	resolution := ClarificationInput{Kind: "QUESTION", ID: questions[0].ID, Resolution: "The order limit is 20 units and excess orders must be rejected."}
	if err = service.ResolveClarification(ctx, otherSet, resolution, "qa"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign resolution=%v", err)
	}
	if err = service.ResolveClarification(ctx, setID, resolution, "trusted-qa"); err != nil {
		t.Fatal(err)
	}
	if err = service.ResolveClarification(ctx, setID, resolution, "trusted-qa"); err != nil {
		t.Fatal(err)
	}
	history, err := service.ClarificationHistory(ctx, setID)
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%+v %v", history, err)
	}
	detail, err := service.Get(ctx, tbd.ID)
	if err != nil || detail.Requirement.Status != StatusDraft || len(detail.Requirement.ReviewBlockers) != 0 {
		t.Fatalf("resolution state=%+v %v", detail.Requirement, err)
	}
	if _, err = service.Review(ctx, tbd.ID, ReviewInput{ReviewerName: "qa", Decision: DecisionApproved, ExpectedHash: detail.Requirement.ReviewHash}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.MarkConflict(ctx, setID, stale.ID, ambiguous.ID, "Conflicting discount rule"); err != nil {
		t.Fatal(err)
	}
	conflicts, err := service.ListConflicts(ctx, setID)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts=%+v %v", conflicts, err)
	}
	if _, err = service.Review(ctx, stale.ID, ReviewInput{ReviewerName: "qa", Decision: DecisionApproved, Risk: "HIGH"}); !errors.Is(err, ErrReviewBlocked) {
		t.Fatalf("conflict risk bypass=%v", err)
	}
	if err = service.ResolveClarification(ctx, setID, ClarificationInput{Kind: "CONFLICT", ID: conflicts[0].ID, Resolution: "The recording rule and discount rule apply to separate order stages."}, "trusted-qa"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.MarkConflict(ctx, setID, stale.ID, ambiguous.ID, "Repeated extraction"); err != nil {
		t.Fatal(err)
	}
	detail, _ = service.Get(ctx, stale.ID)
	if detail.Requirement.Status != StatusDraft {
		t.Fatalf("resolved conflict reopened: %+v", detail.Requirement)
	}
	// Concurrent decisions with the same original hash: exactly one can apply.
	exact := selection(stale.ID)
	var wg sync.WaitGroup
	out := make(chan []ReviewResult, 2)
	fail := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rows, err := service.BulkReview(ctx, setID, BulkReviewInput{Items: []ReviewSelection{exact}, Decision: DecisionApproved}, fmt.Sprintf("race-%d", i), "qa")
			out <- rows
			fail <- err
		}(i)
	}
	wg.Wait()
	close(out)
	close(fail)
	successes := 0
	for err := range fail {
		if err != nil {
			t.Fatal(err)
		}
	}
	for rows := range out {
		if rows[0].Status == "APPLIED" {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successes=%d", successes)
	}
	// Changing only a prior decision also creates a revision; it must not drop
	// a newly discovered clarification and allow approval via the rejected copy.
	if _, err = repo.MarkConflict(ctx, setID, saved[10].ID, saved[11].ID, "A later source reveals conflicting limits"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Review(ctx, saved[10].ID, ReviewInput{ReviewerName: "qa", Decision: DecisionRejected}); !errors.Is(err, ErrReviewBlocked) {
		t.Fatalf("decision-only revision bypassed clarification: %v", err)
	}
	// Proof already reviewed cannot receive citations or changed business fields.
	if _, err = pool.Exec(ctx, `UPDATE requirements SET risk='HIGH' WHERE id=$1`, saved[0].ID); err == nil {
		t.Fatal("reviewed risk changed in place")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO requirement_flow_steps(requirement_id,ordinal,action,expected_result) VALUES($1,1,'new step','new expected')`, saved[0].ID); err == nil {
		t.Fatal("reviewed steps changed")
	}
	var bExtra int64
	if err = pool.QueryRow(ctx, `INSERT INTO document_blocks(document_version_id,ordinal,block_type,content,source_locator) VALUES($1,2,'PARAGRAPH','Extra evidence','line:2') RETURNING id`, v1).Scan(&bExtra); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO requirement_evidence(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash) VALUES($1,$2,$3,$4,'line:2',$5)`, saved[0].ID, setID, v1, bExtra, hashText("Extra evidence")); err == nil {
		t.Fatal("reviewed citations appended")
	}
	old, _ := service.Get(ctx, saved[0].ID)
	edited, err := service.Review(ctx, saved[0].ID, ReviewInput{ReviewerName: "qa", Decision: DecisionApproved, ExpectedHash: old.Requirement.ReviewHash, Title: "Revised order rule"})
	if err != nil || edited.Requirement.ID == saved[0].ID || edited.Requirement.VersionNumber != 2 {
		t.Fatalf("edited revision=%+v %v", edited.Requirement, err)
	}
	oldAgain, _ := service.Get(ctx, saved[0].ID)
	if oldAgain.Requirement.Title != old.Requirement.Title || len(oldAgain.Evidence) != 1 {
		t.Fatal("old proof changed")
	}
	v2, b2 := source(2)
	indexed2, err := index.Index(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	fresh := save(proposal("REQ-03", "The system must validate order rule 3."), v2, b2, indexed2.SourceSnapshotID)
	save(proposal("REQ-01", "The system must validate the changed order rule."), v2, b2, indexed2.SourceSnapshotID)
	save(proposal("REQ-NEW", "The system must send an order receipt."), v2, b2, indexed2.SourceSnapshotID)
	save(proposal("REQ-AMB", "The system must apply discount A."), v2, b2, indexed2.SourceSnapshotID)
	save(proposal("REQ-AMB", "The system must apply discount B."), v2, b2, indexed2.SourceSnapshotID)
	if fresh.ID == saved[3].ID {
		t.Fatal("new source mutated approved revision")
	}
	if err = repo.ReconcileSources(ctx, setID, *indexed2.SourceSnapshotID, indexed2.Generation); err != nil {
		t.Fatal(err)
	}
	comparisons, err := service.SourceComparisons(ctx, setID)
	if err != nil || len(comparisons) != 2 {
		t.Fatalf("comparisons=%+v %v", comparisons, err)
	}
	classifications := map[string]int{}
	for _, item := range comparisons[0].Items {
		classifications[item.Classification]++
	}
	for _, kind := range []string{"ADDED", "CHANGED", "REMOVED", "UNCHANGED", "AMBIGUOUS"} {
		if classifications[kind] == 0 {
			t.Fatalf("missing %s: %+v", kind, comparisons[0])
		}
	}
	inventory, err := service.List(ctx, Filter{DocumentSetID: setID})
	if err != nil || len(inventory) != 5 {
		t.Fatalf("new inventory=%+v %v", inventory, err)
	}
	for _, item := range inventory {
		if item.Status != StatusDraft {
			t.Fatal("new source inherited approval")
		}
	}
	proof, _ := service.Get(ctx, saved[3].ID)
	if proof.Requirement.Status != StatusApproved || len(proof.Evidence) != 1 || proof.Evidence[0].DocumentVersionID != v1 {
		t.Fatal("old source proof rewritten")
	}
	if _, err = service.Review(ctx, proof.Requirement.ID, ReviewInput{ReviewerName: "qa", Decision: DecisionApproved}); !errors.Is(err, ErrReviewBlocked) {
		t.Fatalf("historical review allowed: %v", err)
	}
}
