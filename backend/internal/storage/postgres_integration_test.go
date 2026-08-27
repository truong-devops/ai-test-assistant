//go:build integration

package storage

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestOpenAndPing(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	database, err := Open(ctx, databaseURL, 2)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()
	if err := database.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}
