//go:build integration

package project

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRepositoryLifecycle(t *testing.T) {
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

	repository := NewPostgresRepository(pool)
	gitLabID := time.Now().UnixNano()
	created, err := repository.Create(ctx, CreateInput{
		Name: "project-repository-integration", Provider: "gitlab", ProviderProjectID: gitLabID,
		RepositoryURL: "https://gitlab.example.com/repository-integration.git",
		DefaultBranch: "main", Language: "go",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, created.ID) }()

	byID, err := repository.GetByID(ctx, created.ID)
	if err != nil || byID.Provider != "gitlab" || byID.ProviderProjectID != gitLabID {
		t.Fatalf("GetByID() project=%+v error=%v", byID, err)
	}
	byGitLabID, err := repository.GetByGitLabProjectID(ctx, gitLabID)
	if err != nil || byGitLabID.ID != created.ID {
		t.Fatalf("GetByGitLabProjectID() project=%+v error=%v", byGitLabID, err)
	}
	projects, err := repository.List(ctx)
	if err != nil || len(projects) == 0 {
		t.Fatalf("List() projects=%v error=%v", projects, err)
	}
	_, err = repository.Create(ctx, CreateInput{
		Name: "duplicate", Provider: "gitlab", ProviderProjectID: gitLabID,
		RepositoryURL: "https://gitlab.example.com/duplicate.git", DefaultBranch: "main", Language: "go",
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate Create() error=%v, want ErrAlreadyExists", err)
	}
	if _, err := repository.GetByID(ctx, -1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID(-1) error=%v, want ErrNotFound", err)
	}
}
