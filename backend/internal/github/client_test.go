package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

func githubRepository(id int64) scm.Repository {
	return scm.Repository{Provider: scm.ProviderGitHub, ProviderProjectID: id,
		RepositoryURL: "https://github.com/acme/service.git"}
}

func TestHTTPClientGetsPullRequestAndFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("X-GitHub-Api-Version") != apiVersion {
			t.Fatalf("headers=%v", r.Header)
		}
		switch r.URL.Path {
		case "/repos/acme/service/pulls/4":
			fmt.Fprint(w, `{"number":4,"title":"change","state":"open","html_url":"https://github.com/acme/service/pull/4","head":{"ref":"feature","sha":"head"},"base":{"ref":"main","sha":"base","repo":{"id":12}}}`)
		case "/repos/acme/service/pulls/4/files":
			if r.URL.Query().Get("per_page") != "100" {
				t.Fatalf("query=%v", r.URL.Query())
			}
			fmt.Fprint(w, `[{"filename":"new.go","previous_filename":"old.go","status":"renamed","patch":"@@ -1 +1 @@\n-old\n+new","additions":1,"deletions":1,"changes":2},{"filename":"large.go","status":"modified","additions":20,"deletions":2,"changes":22}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pullRequest, err := client.GetMergeRequest(context.Background(), githubRepository(12), 4)
	if err != nil {
		t.Fatal(err)
	}
	if pullRequest.DiffRefs.HeadSHA != "head" || pullRequest.DiffRefs.StartSHA != "base" || pullRequest.SourceBranch != "feature" {
		t.Fatalf("pull request=%+v", pullRequest)
	}
	files, err := client.GetMergeRequestDiff(context.Background(), githubRepository(12), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].OldPath != "old.go" || !files[0].RenamedFile ||
		!files[1].TooLarge || files[1].Additions != 20 {
		t.Fatalf("files=%+v", files)
	}
}

func TestHTTPClientResolvesRepositoryMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/service" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":12,"name":"service","default_branch":"trunk","html_url":"https://github.com/acme/service"}`)
	}))
	defer server.Close()
	client, _ := NewHTTPClient(server.URL, "", time.Second)
	metadata, err := client.ResolveRepository(context.Background(), scm.Repository{
		Provider: scm.ProviderGitHub, RepositoryURL: "https://github.com/acme/service.git",
	})
	if err != nil || metadata.ProviderProjectID != 12 || metadata.DefaultBranch != "trunk" {
		t.Fatalf("metadata=%+v error=%v", metadata, err)
	}
}

func TestHTTPClientGetsTreeAndRawFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/git/trees/"):
			if r.URL.Query().Get("recursive") != "1" {
				t.Fatalf("query=%v", r.URL.Query())
			}
			fmt.Fprint(w, `{"truncated":false,"tree":[{"path":"internal/service.go","mode":"100644","type":"blob","sha":"blob-sha"}]}`)
		case r.URL.Path == "/repos/acme/service/contents/internal/service.go":
			if r.URL.Query().Get("ref") != "head" || r.Header.Get("Accept") != "application/vnd.github.raw+json" {
				t.Fatalf("query=%v accept=%q", r.URL.Query(), r.Header.Get("Accept"))
			}
			fmt.Fprint(w, "package service\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, _ := NewHTTPClient(server.URL, "", time.Second)
	entries, err := client.ListRepositoryTree(context.Background(), githubRepository(12), "head")
	if err != nil || len(entries) != 1 || entries[0].ID != "blob-sha" || entries[0].Name != "service.go" {
		t.Fatalf("entries=%+v error=%v", entries, err)
	}
	contents, err := client.GetFileRaw(context.Background(), githubRepository(12), "internal/service.go", "head")
	if err != nil || string(contents) != "package service\n" {
		t.Fatalf("contents=%q error=%v", contents, err)
	}
}

func TestHTTPClientRejectsMismatchedRepositoryAndTruncatedTree(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			fmt.Fprint(w, `{"number":1,"head":{"sha":"head"},"base":{"sha":"base","repo":{"id":99}}}`)
			return
		}
		fmt.Fprint(w, `{"truncated":true,"tree":[]}`)
	}))
	defer server.Close()
	client, _ := NewHTTPClient(server.URL, "", time.Second)
	if _, err := client.GetMergeRequest(context.Background(), githubRepository(12), 1); err == nil {
		t.Fatal("GetMergeRequest() accepted mismatched repository ID")
	}
	if _, err := client.ListRepositoryTree(context.Background(), githubRepository(12), "main"); err == nil {
		t.Fatal("ListRepositoryTree() accepted truncated tree")
	}
}

func TestHTTPClientDoesNotForwardTokenAcrossRedirect(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalled = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()
	client, _ := NewHTTPClient(source.URL, "secret", time.Second)
	if _, err := client.GetMergeRequest(context.Background(), githubRepository(12), 1); err == nil {
		t.Fatal("GetMergeRequest() followed redirect")
	}
	if targetCalled {
		t.Fatal("redirect target was called")
	}
}
