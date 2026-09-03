package github

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

func TestLivePublicGitHubSource(t *testing.T) {
	if os.Getenv("RUN_LIVE_GITHUB_TESTS") != "1" {
		t.Skip("RUN_LIVE_GITHUB_TESTS is not set")
	}
	client, err := NewHTTPClient("https://api.github.com", "", 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	repository := scm.Repository{
		Provider: scm.ProviderGitHub, ProviderProjectID: 1296269,
		RepositoryURL: "https://github.com/octocat/Hello-World",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	entries, err := client.ListRepositoryTree(ctx, repository, "master")
	if err != nil {
		t.Fatal(err)
	}
	readmePath := ""
	for _, entry := range entries {
		if entry.Type == "blob" && strings.EqualFold(entry.Path, "README") {
			readmePath = entry.Path
			break
		}
	}
	if readmePath == "" {
		t.Fatalf("README not found in %d entries", len(entries))
	}
	contents, err := client.GetFileRaw(ctx, repository, readmePath, "master")
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) == 0 {
		t.Fatal("GitHub returned an empty README")
	}
	pullRequest, err := client.GetMergeRequest(ctx, repository, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pullRequest.DiffRefs.HeadSHA == "" || pullRequest.DiffRefs.StartSHA == "" {
		t.Fatalf("incomplete pull request metadata: %+v", pullRequest)
	}
	files, err := client.GetMergeRequestDiff(ctx, repository, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("GitHub returned no files for public pull request #1")
	}
}
