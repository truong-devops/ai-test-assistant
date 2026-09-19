//go:build integration

package aibudget

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
)

func TestManagerReservesFinalizesAndEnforcesDocumentSetBudget(t *testing.T) {
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
	err = pool.QueryRow(ctx, `INSERT INTO document_sets(name,ai_token_budget,ai_cost_budget_microusd)
		VALUES($1,1000,1000000) RETURNING id`, "budget-integration-"+time.Now().Format("20060102150405.000000000")).Scan(&setID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID) }()

	manager := NewManager(pool, 1, 2)
	request := llm.Request{Instructions: "extract", Input: "approved requirement",
		SchemaName: "result", Schema: map[string]any{"type": "object"}, MaxOutputTokens: 300}
	reservation, err := manager.Reserve(ctx, setID, "REQUIREMENT_EXTRACTION", "chunk:1", request)
	if err != nil || reservation.ID == 0 {
		t.Fatalf("reserve=%+v error=%v", reservation, err)
	}
	if err = manager.Finalize(ctx, reservation, llm.Usage{InputTokens: 100, OutputTokens: 200}); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status(ctx, setID)
	if err != nil || status.UsedTokens != 300 || status.ReservedTokens != 0 || status.RemainingTokens != 700 {
		t.Fatalf("status=%+v error=%v", status, err)
	}
	large := request
	large.MaxOutputTokens = 800
	if _, err = manager.Reserve(ctx, setID, "TEST_CASE_GENERATION", "requirement:1", large); !errors.Is(err, ErrExceeded) {
		t.Fatalf("over-budget reserve error=%v, want ErrExceeded", err)
	}

	var concurrentSetID int64
	err = pool.QueryRow(ctx, `INSERT INTO document_sets(name,ai_token_budget,ai_cost_budget_microusd)
		VALUES($1,1000,1000000) RETURNING id`, "budget-concurrency-"+time.Now().Format("20060102150405.000000000")).Scan(&concurrentSetID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, concurrentSetID)
	}()
	concurrentRequest := request
	concurrentRequest.MaxOutputTokens = 600
	start := make(chan struct{})
	errorsByCall := make([]error, 2)
	reservations := make([]Reservation, 2)
	var wait sync.WaitGroup
	for index := range errorsByCall {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			reservations[index], errorsByCall[index] = manager.Reserve(ctx, concurrentSetID,
				"REQUIREMENT_EXTRACTION", "chunk:concurrent", concurrentRequest)
		}(index)
	}
	close(start)
	wait.Wait()
	succeeded, rejected := 0, 0
	for index, callErr := range errorsByCall {
		if callErr == nil {
			succeeded++
			_ = manager.Release(ctx, reservations[index])
		} else if errors.Is(callErr, ErrExceeded) {
			rejected++
		} else {
			t.Fatalf("concurrent reserve %d error=%v", index, callErr)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent reservations succeeded=%d rejected=%d", succeeded, rejected)
	}
}
