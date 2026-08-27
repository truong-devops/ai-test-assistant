//go:build integration

package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	gitLabProjectID := time.Now().UnixNano()
	var projectID int64
	err = pool.QueryRow(ctx, `INSERT INTO projects
		(name, gitlab_project_id, repository_url, default_branch, language, status)
		VALUES ('integration', $1, 'https://gitlab.example.com/test.git', 'main', 'go', 'active') RETURNING id`,
		gitLabProjectID).Scan(&projectID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) }()

	repository := NewRepository(pool)
	uuid := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	input := EnqueueInput{ProjectID: projectID, MergeRequestIID: 3, SourceSHA: "initial",
		WebhookUUID: uuid, RawEvent: json.RawMessage(`{"test":true}`)}
	const callers = 12
	var waitGroup sync.WaitGroup
	var mutex sync.Mutex
	ids := make([]int64, 0, callers)
	createdCount := 0
	errorsSeen := make([]error, 0)
	for range callers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			result, wasCreated, enqueueErr := repository.Enqueue(ctx, input)
			mutex.Lock()
			defer mutex.Unlock()
			if enqueueErr != nil {
				errorsSeen = append(errorsSeen, enqueueErr)
				return
			}
			ids = append(ids, result.ID)
			if wasCreated {
				createdCount++
			}
		}()
	}
	waitGroup.Wait()
	if len(errorsSeen) != 0 || createdCount != 1 || len(ids) != callers {
		t.Fatalf("concurrent Enqueue() errors=%v created=%d ids=%v", errorsSeen, createdCount, ids)
	}
	createdID := ids[0]
	for _, id := range ids {
		if id != createdID {
			t.Fatalf("concurrent Enqueue() returned different ids: %v", ids)
		}
	}

	claimed, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || claimed.ID != createdID || claimed.Status != StatusFetchingSource {
		t.Fatalf("ClaimNext() job=%+v error=%v", claimed, err)
	}
	if claimed.AttemptCount != 1 {
		t.Fatalf("ClaimNext() attempt_count=%d, want 1", claimed.AttemptCount)
	}
	changeQueue := NewChangeQueue(repository)
	if _, err := changeQueue.ClaimNext(ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("change queue claimed a Phase 2 job: error=%v, want ErrNotFound", err)
	}
	if err := repository.RetryOrFail(ctx, claimed.ID, claimed.AttemptCount, fmt.Errorf("temporary"), 3, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNext(ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ClaimNext() before retry delay error=%v, want ErrNotFound", err)
	}
	time.Sleep(300 * time.Millisecond)
	claimed, err = repository.ClaimNext(ctx, time.Minute)
	if err != nil || claimed.ID != createdID || claimed.AttemptCount != 2 {
		t.Fatalf("retried ClaimNext() job=%+v error=%v", claimed, err)
	}
	err = repository.SaveFetched(ctx, createdID, claimed.AttemptCount, MergeRequestMetadata{
		SourceSHA: "head", TargetSHA: "target", SourceBranch: "feature", TargetBranch: "main", Title: "change",
	}, []ChangedFile{{OldPath: "main.go", NewPath: "main.go", ChangeType: "modified", Additions: 1, Diff: "+line"}})
	if err != nil {
		t.Fatal(err)
	}
	result, files, err := repository.Get(ctx, createdID)
	if err != nil || result.Status != StatusAnalyzingChange || len(files) != 1 || files[0].Additions != 1 {
		t.Fatalf("Get() job=%+v files=%+v error=%v", result, files, err)
	}
	if result.AttemptCount != 0 {
		t.Fatalf("source-to-change handoff attempt_count=%d, want 0", result.AttemptCount)
	}
	if _, err := pool.Exec(ctx, `UPDATE analysis_jobs SET attempt_count=3 WHERE id=$1`, createdID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNext(ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source queue claimed a Phase 3 job: error=%v, want ErrNotFound", err)
	}
	changeClaim, err := changeQueue.ClaimNext(ctx, time.Minute)
	if err != nil || changeClaim.ID != createdID || changeClaim.AttemptCount != 1 {
		t.Fatalf("change ClaimNext() did not reset the legacy Phase 2 attempt budget: job=%+v error=%v", changeClaim, err)
	}
	if _, err := changeQueue.ClaimNext(ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("concurrent change ClaimNext() error=%v, want ErrNotFound", err)
	}
	if err := changeQueue.RetryOrFail(ctx, changeClaim.ID, changeClaim.AttemptCount,
		fmt.Errorf("temporary analyzer error"), 3, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, err := changeQueue.ClaimNext(ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("change ClaimNext() before retry delay error=%v, want ErrNotFound", err)
	}
	time.Sleep(300 * time.Millisecond)
	retriedChange, err := changeQueue.ClaimNext(ctx, time.Minute)
	if err != nil || retriedChange.ID != createdID || retriedChange.AttemptCount != 2 {
		t.Fatalf("retried change ClaimNext() job=%+v error=%v", retriedChange, err)
	}
	if err := repository.SaveSymbols(ctx, createdID, changeClaim.AttemptCount, nil); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale SaveSymbols() error=%v, want ErrLeaseLost", err)
	}
	if err := repository.SaveSymbols(ctx, createdID, retriedChange.AttemptCount, []ChangedSymbol{{
		ChangedFileID: files[0].ID, SymbolName: "CreateUser", SymbolKind: "function",
		PackageName: "service", StartLine: 2, EndLine: 5, ChangeType: "modified",
		ChangeSummary: "modified function CreateUser",
	}}); err != nil {
		t.Fatal(err)
	}
	result, _, err = repository.Get(ctx, createdID)
	symbols, symbolErr := repository.ListChangedSymbols(ctx, createdID)
	if err != nil || symbolErr != nil || result.Status != StatusRetrievingContext ||
		result.AttemptCount != 0 || len(symbols) != 1 || symbols[0].SymbolName != "CreateUser" {
		t.Fatalf("analyzed job=%+v symbols=%+v errors=%v/%v", result, symbols, err, symbolErr)
	}

	leaseJob, _, err := repository.Enqueue(ctx, EnqueueInput{
		ProjectID: projectID, MergeRequestIID: 4, SourceSHA: "lease",
		WebhookUUID: uuid + "-lease", RawEvent: json.RawMessage(`{"lease":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	firstLease, err := repository.ClaimNext(ctx, 100*time.Millisecond)
	if err != nil || firstLease.ID != leaseJob.ID || firstLease.AttemptCount != 1 {
		t.Fatalf("first lease claim job=%+v error=%v", firstLease, err)
	}
	if _, err := repository.ClaimNext(ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claim during active lease error=%v, want ErrNotFound", err)
	}
	time.Sleep(150 * time.Millisecond)
	recovered, err := repository.ClaimNext(ctx, time.Minute)
	if err != nil || recovered.ID != leaseJob.ID || recovered.AttemptCount != 2 {
		t.Fatalf("expired lease recovery job=%+v error=%v", recovered, err)
	}
	if err := repository.RetryOrFail(ctx, firstLease.ID, firstLease.AttemptCount,
		fmt.Errorf("stale worker"), 3, time.Second); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale attempt RetryOrFail() error=%v, want ErrLeaseLost", err)
	}
	if err := repository.RetryOrFail(ctx, recovered.ID, recovered.AttemptCount, fmt.Errorf("permanent"), 2, time.Second); err != nil {
		t.Fatal(err)
	}
	failed, _, err := repository.Get(ctx, recovered.ID)
	if err != nil || failed.Status != StatusFailed || failed.FinishedAt == nil {
		t.Fatalf("failed job=%+v error=%v", failed, err)
	}
}
