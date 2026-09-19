//go:build integration

package requirement

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExtractionQueueLeaseProgressIdempotencyAndTerminalFailure(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var setID int64
	if err := pool.QueryRow(ctx, `INSERT INTO document_sets(name) VALUES($1) RETURNING id`,
		"extraction-queue-"+time.Now().Format("150405.000000000")).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID)
	repository := NewRepository(pool)

	queued, err := repository.EnqueueExtraction(ctx, setID, 7, nil, 0, 2, "BA")
	if err != nil || queued.Status != ExtractionPending {
		t.Fatalf("queued=%+v error=%v", queued, err)
	}
	duplicate, err := repository.EnqueueExtraction(ctx, setID, 8, nil, 0, 99, "other")
	if err != nil || duplicate.ID != queued.ID || duplicate.IndexGeneration != 7 || duplicate.TotalChunks != 2 {
		t.Fatalf("idempotent enqueue=%+v error=%v", duplicate, err)
	}
	claimed, err := repository.ClaimExtraction(ctx, time.Minute)
	if err != nil || claimed.ID != queued.ID || claimed.AttemptCount != 1 {
		t.Fatalf("claimed=%+v error=%v", claimed, err)
	}
	progress := ExtractionSummary{ProcessedChunks: 1, CreatedCount: 1}
	if err := repository.UpdateExtractionProgress(ctx, claimed, progress); err != nil {
		t.Fatal(err)
	}
	latest, err := repository.LatestExtraction(ctx, setID)
	if err != nil || latest.ProcessedChunks != 1 || latest.CreatedCount != 1 {
		t.Fatalf("progress=%+v error=%v", latest, err)
	}
	if err := repository.CompleteExtraction(ctx, claimed, ExtractionSummary{CreatedCount: 2}); err != nil {
		t.Fatal(err)
	}
	latest, err = repository.LatestExtraction(ctx, setID)
	if err != nil || latest.Status != ExtractionCompleted || latest.ProcessedChunks != 2 {
		t.Fatalf("completed=%+v error=%v", latest, err)
	}
	if err := repository.UpdateExtractionProgress(ctx, claimed, progress); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale lease progress error=%v", err)
	}

	failedJob, err := repository.EnqueueExtraction(ctx, setID, 8, nil, 0, 1, "BA")
	if err != nil || failedJob.ID == queued.ID {
		t.Fatalf("second job=%+v error=%v", failedJob, err)
	}
	failedJob, err = repository.ClaimExtraction(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	providerErr := errors.New("provider timeout")
	if err := repository.RetryOrFailExtraction(ctx, failedJob, providerErr, 1, time.Second); err != nil {
		t.Fatal(err)
	}
	latest, err = repository.LatestExtraction(ctx, setID)
	if err != nil || latest.Status != ExtractionFailed || latest.ErrorMessage != providerErr.Error() {
		t.Fatalf("failed=%+v error=%v", latest, err)
	}
}
