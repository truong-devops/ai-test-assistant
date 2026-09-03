package gitlab

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

func TestLivePublicGitLabSource(t *testing.T) {
	if os.Getenv("RUN_LIVE_GITLAB_TESTS") != "1" {
		t.Skip("RUN_LIVE_GITLAB_TESTS is not set")
	}
	client, err := NewHTTPClient("https://gitlab.com", "", 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	repository := scm.Repository{
		Provider: scm.ProviderGitLab, RepositoryURL: "https://gitlab.com/gitlab-org/gitlab-test.git",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	metadata, err := client.ResolveRepository(ctx, repository)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ProviderProjectID != 72724 || metadata.DefaultBranch == "" {
		t.Fatalf("metadata=%+v", metadata)
	}
	repository.ProviderProjectID = metadata.ProviderProjectID
	entries, err := client.ListRepositoryTree(ctx, repository, metadata.DefaultBranch)
	if err != nil || len(entries) == 0 {
		t.Fatalf("entries=%d error=%v", len(entries), err)
	}
	contents, err := client.GetFileRaw(ctx, repository, "README.md", metadata.DefaultBranch)
	if err != nil || len(contents) == 0 {
		t.Fatalf("README bytes=%d error=%v", len(contents), err)
	}
}
