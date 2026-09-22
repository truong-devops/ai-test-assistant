//go:build integration

package workflow

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

func TestUV05SourceUploadReviewAndDurableContinuation(t *testing.T) {
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
	repo := document.NewRepository(pool)
	files, err := document.NewLocalFileStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	docs := document.NewService(repo, files)
	set, err := docs.CreateSet(ctx, document.CreateSetInput{Name: "uv05-" + time.Now().Format("150405.000000000")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM document_source_snapshot_items WHERE document_set_id=$1`, set.ID)
		if _, err := pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, set.ID); err != nil {
			t.Errorf("cleanup fixture: %v", err)
		}
	}()
	source := "# REQ-ORDER\n\nThe system must create an order when the customer confirms checkout."
	doc, v1, err := docs.Upload(ctx, set.ID, document.UploadInput{DocumentName: "Order", Filename: "order.md", NewDocument: true}, strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	embedder, err := knowledge.NewEmbeddingClient("local", "hash-v1")
	if err != nil {
		t.Fatal(err)
	}
	index := document.NewIndexService(document.NewIndexRepository(pool), embedder)
	reqRepo := requirement.NewRepository(pool)
	reqs := requirement.NewService(reqRepo, index, requirement.DeterministicExtractor{})
	queue := NewRepository(pool)
	service := NewService(queue, index, reqs, nil, 3)
	// Advance only this fixture's intents. Other packages share TEST_DATABASE_URL;
	// a test must never consume or execute their jobs through the global worker.
	advance := func() error {
		intents, err := queue.SourceIntents(ctx, set.ID)
		if err != nil {
			return err
		}
		for i := len(intents) - 1; i >= 0; i-- {
			if err := service.advanceSource(ctx, intents[i].ID); err != nil {
				return err
			}
		}
		return nil
	}
	claimVersion := func(v document.Version) document.Version {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE document_versions SET parse_status='PARSING',attempt_count=attempt_count+1,
			lease_expires_at=NOW()+INTERVAL '1 minute' WHERE id=$1 AND parse_status='UPLOADED'`, v.ID); err != nil {
			t.Fatal(err)
		}
		_, claimed, _, err := docs.GetVersion(ctx, v.DocumentID, v.VersionNumber)
		if err != nil {
			t.Fatal(err)
		}
		return claimed
	}
	// Upload and its continuation are durable together; reads do not create jobs.
	read, err := service.Read(ctx, set.ID, "reviewer")
	if err != nil || len(read.SourceIntents) != 1 || len(read.RecentJobs) != 0 {
		t.Fatalf("upload intent=%+v err=%v", read, err)
	}
	if err = advance(); err != nil {
		t.Fatal(err)
	}
	read, err = service.Read(ctx, set.ID, "reviewer")
	if err != nil || read.SourceIntents[0].Status != "WAITING_PARSE" {
		t.Fatalf("before parse=%+v err=%v", read, err)
	}
	// A different set cannot receive a version of this logical document.
	other, err := docs.CreateSet(ctx, document.CreateSetInput{Name: "uv05-other-" + time.Now().Format("150405.000000000")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, other.ID) }()
	if _, _, err = docs.Upload(ctx, other.ID, document.UploadInput{DocumentID: doc.ID, Filename: "foreign.md"}, strings.NewReader("foreign")); !errors.Is(err, document.ErrNotFound) {
		t.Fatalf("cross-set upload err=%v", err)
	}
	if _, err = docs.ListVersions(ctx, other.ID, doc.ID); !errors.Is(err, document.ErrNotFound) {
		t.Fatalf("cross-set history err=%v", err)
	}
	if _, _, err = docs.Upload(ctx, set.ID, document.UploadInput{DocumentName: "Order", Filename: "new.md", NewDocument: true}, strings.NewReader("different")); !errors.Is(err, document.ErrAlreadyExists) {
		t.Fatalf("new document silently became version: %v", err)
	}
	parse := document.NewProcessor(files, document.NewStructuredParser(), repo)
	claimed := claimVersion(v1)
	if err = parse.Process(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	// Exact hash, revision, and reviewer role guard the all-or-nothing approval.
	set, err = docs.GetSet(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := SourceCommand{Command: "APPROVE_AND_EXTRACT", ExpectedSourceRevision: set.SourceRevision,
		Selected: []SourceSelection{{VersionID: v1.ID, SHA256: v1.SHA256, ApprovalStatus: "DRAFT"}}, ReviewerName: "Display QA"}
	if _, err = service.SourceCommand(ctx, set.ID, input, "denied", "editor", "trusted-editor"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("editor approved: %v", err)
	}
	invalid := input
	invalid.Selected = []SourceSelection{{VersionID: v1.ID, SHA256: strings.Repeat("f", 64), ApprovalStatus: "DRAFT"}}
	if _, err = service.SourceCommand(ctx, set.ID, invalid, "bad-hash", "reviewer", "trusted-qa"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("hash guard: %v", err)
	}
	intent, err := service.SourceCommand(ctx, set.ID, input, "review-and-extract", "reviewer", "trusted-qa")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.SourceCommand(ctx, set.ID, input, "review-and-extract", "reviewer", "trusted-qa")
	if err != nil || replay.ID != intent.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	var reviewCount int
	var actor string
	if err = pool.QueryRow(ctx, `SELECT count(*),max(reviewer_name) FROM document_version_reviews WHERE document_version_id=$1`, v1.ID).Scan(&reviewCount, &actor); err != nil || reviewCount != 1 || actor != "trusted-qa" {
		t.Fatalf("review audit count=%d actor=%s err=%v", reviewCount, actor, err)
	}
	// Simulate API/browser exit and a fresh worker using only persisted state.
	service = NewService(NewRepository(pool), index, reqs, nil, 3)
	if err = advance(); err != nil {
		t.Fatal(err)
	}
	intent, err = queue.sourceIntent(ctx, intent.ID)
	if err != nil || intent.IndexJobID == nil {
		t.Fatalf("index not enqueued: %+v %v", intent, err)
	}
	// A manual retry must reopen the persisted continuation, not merely the
	// index job; otherwise an approved extraction would remain failed forever.
	if _, err = pool.Exec(ctx, `UPDATE document_workflow_jobs SET status='FAILED',retryable=TRUE WHERE id=$1`, *intent.IndexJobID); err != nil {
		t.Fatal(err)
	}
	if err = advance(); err != nil {
		t.Fatal(err)
	}
	failedIntent, err := queue.sourceIntent(ctx, intent.ID)
	if err != nil || failedIntent.Status != "FAILED" {
		t.Fatalf("failure not persisted: %+v %v", failedIntent, err)
	}
	failedJob, err := queue.Get(ctx, *intent.IndexJobID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Retry(ctx, failedJob.ID, failedJob.Revision, "editor"); err != nil {
		t.Fatal(err)
	}
	retriedIntent, err := queue.sourceIntent(ctx, intent.ID)
	if err != nil || retriedIntent.Status != "INDEXING" || retriedIntent.ErrorMessage != "" {
		t.Fatalf("retry lost continuation: %+v %v", retriedIntent, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE document_workflow_jobs SET status='RUNNING',attempt_count=attempt_count+1,lease_expires_at=NOW()+INTERVAL '1 minute' WHERE id=$1`, *intent.IndexJobID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE document_workflow_job_units SET status='RUNNING',attempt_count=attempt_count+1 WHERE workflow_job_id=$1`, *intent.IndexJobID); err != nil {
		t.Fatal(err)
	}
	indexJob, err := queue.Get(ctx, *intent.IndexJobID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Process(ctx, indexJob)
	if err != nil {
		t.Fatal(err)
	}
	if err = queue.Complete(ctx, indexJob, result); err != nil {
		t.Fatal(err)
	}
	if err = advance(); err != nil {
		t.Fatal(err)
	}
	intent, err = queue.sourceIntent(ctx, intent.ID)
	if err != nil || intent.Status != "EXTRACTING" || intent.ExtractionJobID == nil {
		t.Fatalf("continuation=%+v err=%v", intent, err)
	}
	extractionJob, err := queue.Get(ctx, *intent.ExtractionJobID)
	if err != nil || extractionJob.DelegateJobID == nil {
		t.Fatalf("delegate missing: %+v %v", extractionJob, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE requirement_extraction_jobs SET status='RUNNING',attempt_count=attempt_count+1,lease_expires_at=NOW()+INTERVAL '1 minute' WHERE id=$1`, *extractionJob.DelegateJobID); err != nil {
		t.Fatal(err)
	}
	extraction, err := reqRepo.LatestExtraction(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := reqs.ExtractWithProgress(ctx, set.ID, extraction.IndexGeneration, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = reqRepo.CompleteExtraction(ctx, extraction, summary); err != nil {
		t.Fatal(err)
	}
	if err = advance(); err != nil {
		t.Fatal(err)
	}
	intent, err = queue.sourceIntent(ctx, intent.ID)
	if err != nil || intent.Status != "SUCCEEDED" {
		t.Fatalf("completed=%+v err=%v", intent, err)
	}
	// v2 uses the explicit document identity even when its filename is different.
	doc2, v2, err := docs.Upload(ctx, set.ID, document.UploadInput{DocumentID: doc.ID, Filename: "renamed.md"}, strings.NewReader(source+"\nA confirmation number must be returned."))
	if err != nil || doc2.ID != doc.ID || v2.VersionNumber != 2 {
		t.Fatalf("version2=%+v err=%v", v2, err)
	}
	versions, err := docs.ListVersions(ctx, set.ID, doc.ID)
	if err != nil || len(versions) != 2 || versions[0].ID != v2.ID || versions[1].ID != v1.ID {
		t.Fatalf("history=%+v err=%v", versions, err)
	}
	if _, err = service.SourceCommand(ctx, set.ID, input, "stale-revision", "reviewer", "trusted-qa"); !errors.Is(err, ErrInputStale) {
		t.Fatalf("stale input accepted: %v", err)
	}
	replay, err = service.SourceCommand(ctx, set.ID, input, "review-and-extract", "reviewer", "trusted-qa")
	if err != nil || replay.ID != intent.ID {
		t.Fatalf("historical replay=%+v err=%v", replay, err)
	}
	claimed = claimVersion(v2)
	if err = parse.Process(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	_, bad, err := docs.Upload(ctx, set.ID, document.UploadInput{Filename: "broken.docx", NewDocument: true}, strings.NewReader("not a zip archive"))
	if err != nil {
		t.Fatal(err)
	}
	claimed = claimVersion(bad)
	parseErr := parse.Process(ctx, claimed)
	if parseErr == nil {
		t.Fatal("malformed docx unexpectedly parsed")
	}
	if err = repo.RetryOrFail(ctx, claimed, parseErr, 1, time.Second); err != nil {
		t.Fatal(err)
	}
	set, err = docs.GetSet(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	v2Input := SourceCommand{Command: "APPROVE_AND_EXTRACT", ExpectedSourceRevision: set.SourceRevision, ReviewerName: "QA",
		Selected: []SourceSelection{{VersionID: v2.ID, SHA256: v2.SHA256, ApprovalStatus: "DRAFT"}}}
	_, err = service.SourceCommand(ctx, set.ID, v2Input, "blocked-batch", "reviewer", "trusted-qa")
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("failed source accepted without exclusion: %v", err)
	}
	_, storedV2, _, err := docs.GetVersion(ctx, doc.ID, 2)
	if err != nil || storedV2.ApprovalStatus != "DRAFT" {
		t.Fatalf("failed batch left partial approval: %+v %v", storedV2, err)
	}
	v2Input.ExcludedVersionIDs = []int64{bad.ID}
	v2Intent, err := service.SourceCommand(ctx, set.ID, v2Input, "explicit-exclusion", "reviewer", "trusted-qa")
	if err != nil {
		t.Fatal(err)
	}
	// An upload before continuation invalidates this intent instead of silently
	// sending a new version (which has not been reviewed) to the AI.
	if _, _, err = docs.Upload(ctx, set.ID, document.UploadInput{DocumentID: doc.ID, Filename: "v3.md"}, strings.NewReader(source+"\nVersion three changes.")); err != nil {
		t.Fatal(err)
	}
	if err = advance(); err != nil {
		t.Fatal(err)
	}
	v2Intent, err = queue.sourceIntent(ctx, v2Intent.ID)
	if err != nil || v2Intent.Status != "SUPERSEDED" || v2Intent.ExtractionJobID != nil {
		t.Fatalf("changed source intent=%+v err=%v", v2Intent, err)
	}
}
