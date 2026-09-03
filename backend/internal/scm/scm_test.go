package scm

import (
	"context"
	"errors"
	"testing"
)

type clientStub struct{ calls int }

func (s *clientStub) GetMergeRequest(context.Context, Repository, int64) (MergeRequest, error) {
	s.calls++
	return MergeRequest{Title: "ok"}, nil
}
func (s *clientStub) GetMergeRequestDiff(context.Context, Repository, int64) ([]FileDiff, error) {
	return nil, nil
}
func (s *clientStub) GetFileRaw(context.Context, Repository, string, string) ([]byte, error) {
	return nil, nil
}
func (s *clientStub) ListRepositoryTree(context.Context, Repository, string) ([]RepositoryEntry, error) {
	return nil, nil
}
func (s *clientStub) ResolveRepository(context.Context, Repository) (RepositoryMetadata, error) {
	return RepositoryMetadata{ProviderProjectID: 7, Name: "service", DefaultBranch: "main"}, nil
}

func TestRouterDispatchesByProvider(t *testing.T) {
	gitLab, gitHub := &clientStub{}, &clientStub{}
	router, err := NewRouter(map[string]Client{ProviderGitLab: gitLab, ProviderGitHub: gitHub})
	if err != nil {
		t.Fatal(err)
	}
	result, err := router.GetMergeRequest(context.Background(), Repository{
		Provider: ProviderGitHub, ProviderProjectID: 1, RepositoryURL: "https://github.com/acme/service",
	}, 2)
	if err != nil || result.Title != "ok" || gitHub.calls != 1 || gitLab.calls != 0 {
		t.Fatalf("result=%+v error=%v gitlab=%d github=%d", result, err, gitLab.calls, gitHub.calls)
	}
}

func TestRouterResolvesMetadataWithoutProjectID(t *testing.T) {
	router, err := NewRouter(map[string]Client{ProviderGitHub: &clientStub{}})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := router.ResolveRepository(context.Background(), Repository{
		Provider: ProviderGitHub, RepositoryURL: "https://github.com/acme/service",
	})
	if err != nil || metadata.ProviderProjectID != 7 {
		t.Fatalf("metadata=%+v error=%v", metadata, err)
	}
}

func TestRouterRejectsUnknownProvider(t *testing.T) {
	router, err := NewRouter(map[string]Client{ProviderGitLab: &clientStub{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = router.GetMergeRequest(context.Background(), Repository{Provider: "bitbucket", ProviderProjectID: 1}, 2)
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("error=%v, want ErrUnsupportedProvider", err)
	}
}

func TestParseGitHubRepository(t *testing.T) {
	owner, repository, err := ParseGitHubRepository("https://github.com/acme/service.git")
	if err != nil || owner != "acme" || repository != "service" {
		t.Fatalf("owner=%q repository=%q error=%v", owner, repository, err)
	}
	owner, repository, err = ParseGitHubRepository("https://github.com/golang/example")
	if err != nil || owner != "golang" || repository != "example" {
		t.Fatalf("owner=%q repository=%q error=%v", owner, repository, err)
	}
	for _, value := range []string{
		"http://github.com/acme/service", "https://gitlab.com/acme/service",
		"https://github.com/acme", "https://github.com/acme/service/issues", "https://github.com/acme/service?tab=readme",
	} {
		if _, _, err := ParseGitHubRepository(value); err == nil {
			t.Fatalf("ParseGitHubRepository(%q) error=nil", value)
		}
	}
}
