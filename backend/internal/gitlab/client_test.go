package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

func testRepository(id int64) scm.Repository {
	return scm.Repository{Provider: scm.ProviderGitLab, ProviderProjectID: id,
		RepositoryURL: "https://gitlab.com/acme/service"}
}

func TestHTTPClientGetsMergeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/12/merge_requests/4" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "token" {
			t.Fatal("PRIVATE-TOKEN was not sent")
		}
		fmt.Fprint(w, `{"iid":4,"project_id":12,"title":"change","diff_refs":{"head_sha":"head","start_sha":"target"}}`)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GetMergeRequest(context.Background(), testRepository(12), 4)
	if err != nil {
		t.Fatal(err)
	}
	if result.IID != 4 || result.DiffRefs.HeadSHA != "head" {
		t.Fatalf("result = %+v", result)
	}
}

func TestHTTPClientResolvesProjectMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/api/v4/projects/acme%2Fservice" {
			t.Fatalf("request URI=%q", r.RequestURI)
		}
		fmt.Fprint(w, `{"id":12,"name":"service","default_branch":"trunk","web_url":"https://gitlab.example/acme/service"}`)
	}))
	defer server.Close()
	client, _ := NewHTTPClient(server.URL, "", time.Second)
	metadata, err := client.ResolveRepository(context.Background(), scm.Repository{
		Provider: scm.ProviderGitLab, RepositoryURL: server.URL + "/acme/service.git",
	})
	if err != nil || metadata.ProviderProjectID != 12 || metadata.DefaultBranch != "trunk" {
		t.Fatalf("metadata=%+v error=%v", metadata, err)
	}
}

func TestHTTPClientPaginatesDiffs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if r.URL.Query().Get("per_page") != "100" || r.URL.Query().Get("unidiff") != "true" {
			t.Fatalf("query = %v", r.URL.Query())
		}
		if page == "1" {
			w.Header().Set("X-Next-Page", "2")
			fmt.Fprint(w, `[{"new_path":"one.go"}]`)
			return
		}
		fmt.Fprint(w, `[{"new_path":"two.go"}]`)
	}))
	defer server.Close()
	client, _ := NewHTTPClient(server.URL, "", time.Second)
	results, err := client.GetMergeRequestDiff(context.Background(), testRepository(1), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[1].NewPath != "two.go" {
		t.Fatalf("results = %+v", results)
	}
}

func TestHTTPClientGetsRawRepositoryFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/api/v4/projects/12/repository/files/internal%2Fservice%20file.go/raw?ref=feature%2Fbranch" {
			t.Fatalf("request URI = %q", r.RequestURI)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "token" {
			t.Fatal("PRIVATE-TOKEN was not sent")
		}
		fmt.Fprint(w, "package service\n")
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GetFileRaw(context.Background(), testRepository(12), "internal/service file.go", "feature/branch")
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "package service\n" {
		t.Fatalf("result = %q", result)
	}
}

func TestHTTPClientListsRecursiveRepositoryTree(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/12/repository/tree" ||
			r.URL.Query().Get("recursive") != "true" || r.URL.Query().Get("ref") != "main" {
			t.Fatalf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.URL.Query().Get("page") == "1" {
			w.Header().Set("X-Next-Page", "2")
			fmt.Fprint(w, `[{"type":"blob","path":"main.go"}]`)
			return
		}
		fmt.Fprint(w, `[{"type":"tree","path":"internal"},{"type":"blob","path":"README.md"}]`)
	}))
	defer server.Close()
	client, _ := NewHTTPClient(server.URL, "", time.Second)
	entries, err := client.ListRepositoryTree(context.Background(), testRepository(12), "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[2].Path != "README.md" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestHTTPClientReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer server.Close()
	client, _ := NewHTTPClient(server.URL, "", time.Second)
	if _, err := client.GetMergeRequest(context.Background(), testRepository(1), 2); err == nil {
		t.Fatal("GetMergeRequest() error = nil")
	}
}

func TestHTTPClientRejectsInvalidIdentifiers(t *testing.T) {
	client, err := NewHTTPClient("https://gitlab.example.com", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetMergeRequest(context.Background(), testRepository(0), 1); err == nil {
		t.Fatal("GetMergeRequest() accepted zero project ID")
	}
	if _, err := client.GetMergeRequestDiff(context.Background(), testRepository(1), -1); err == nil {
		t.Fatal("GetMergeRequestDiff() accepted negative IID")
	}
	for _, filePath := range []string{"", "/main.go", "../main.go", "a//main.go"} {
		if _, err := client.GetFileRaw(context.Background(), testRepository(1), filePath, "main"); err == nil {
			t.Fatalf("GetFileRaw() accepted path %q", filePath)
		}
	}
	if _, err := client.GetFileRaw(context.Background(), testRepository(1), "main.go", ""); err == nil {
		t.Fatal("GetFileRaw() accepted an empty ref")
	}
	if _, err := client.ListRepositoryTree(context.Background(), testRepository(1), ""); err == nil {
		t.Fatal("ListRepositoryTree() accepted an empty ref")
	}
}

func TestHTTPClientRejectsInvalidPaginationHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Next-Page", "same")
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	client, _ := NewHTTPClient(server.URL, "", time.Second)
	if _, err := client.GetMergeRequestDiff(context.Background(), testRepository(1), 1); err == nil {
		t.Fatal("GetMergeRequestDiff() accepted invalid pagination")
	}
}

func TestHTTPClientDoesNotFollowRedirectsWithToken(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalled = true
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()
	client, _ := NewHTTPClient(source.URL, "sensitive", time.Second)
	if _, err := client.GetMergeRequest(context.Background(), testRepository(1), 1); err == nil {
		t.Fatal("GetMergeRequest() followed redirect")
	}
	if targetCalled {
		t.Fatal("redirect target was called")
	}
}
