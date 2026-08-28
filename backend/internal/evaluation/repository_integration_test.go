//go:build integration

package evaluation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryRoundTripPreservesDatasetHashAndIsIdempotent(t *testing.T) {
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
	service := NewService(NewRepository(pool))
	dataset := validDataset()
	dataset.Name = "integration-round-trip"
	dataset.Description = "verifies immutable Phase 10 persistence"
	first, err := service.Import(ctx, dataset)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM evaluation_runs WHERE id=$1`, first.Run.ID)
	second, err := service.Import(ctx, dataset)
	if err != nil {
		t.Fatal(err)
	}
	if first.Run.ID != second.Run.ID {
		t.Fatalf("idempotent IDs differ: %d and %d", first.Run.ID, second.Run.ID)
	}
	stored, err := service.Get(ctx, first.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Report.DatasetHash != first.Report.DatasetHash || stored.Run.ObservationCount != len(dataset.Observations) {
		t.Fatalf("stored=%#v first=%#v", stored, first)
	}
	runs, err := service.List(ctx)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%#v error=%v", runs, err)
	}
}
