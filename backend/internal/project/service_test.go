package project

import (
	"context"
	"errors"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

type fakeRepository struct {
	created CreateInput
	items   []Project
	err     error
}

type resolverStub struct {
	metadata scm.RepositoryMetadata
	err      error
}

func (s resolverStub) ResolveRepository(context.Context, scm.Repository) (scm.RepositoryMetadata, error) {
	return s.metadata, s.err
}

func (f *fakeRepository) Create(_ context.Context, input CreateInput) (Project, error) {
	f.created = input
	if f.err != nil {
		return Project{}, f.err
	}
	return Project{ID: 1, Name: input.Name, Provider: input.Provider,
		ProviderProjectID: input.ProviderProjectID, RepositoryURL: input.RepositoryURL, Language: input.Language}, nil
}

func (f *fakeRepository) List(context.Context) ([]Project, error) { return f.items, f.err }
func (f *fakeRepository) GetByID(_ context.Context, id int64) (Project, error) {
	if f.err != nil {
		return Project{}, f.err
	}
	for _, item := range f.items {
		if item.ID == id {
			return item, nil
		}
	}
	return Project{}, ErrNotFound
}
func (f *fakeRepository) GetByGitLabProjectID(_ context.Context, id int64) (Project, error) {
	return f.GetByProviderProjectID(context.Background(), "gitlab", id)
}
func (f *fakeRepository) GetByProviderProjectID(_ context.Context, provider string, id int64) (Project, error) {
	for _, item := range f.items {
		if item.RepositoryRef().Provider == provider && item.ExternalProjectID() == id {
			return item, nil
		}
	}
	return Project{}, ErrNotFound
}

func TestServiceCreateAcceptsGitHubRepository(t *testing.T) {
	repository := &fakeRepository{}
	result, err := NewService(repository).Create(context.Background(), CreateInput{
		Name: "service", Provider: "GitHub", ProviderProjectID: 456,
		RepositoryURL: "https://github.com/acme/service.git",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "github" || result.ProviderProjectID != 456 || repository.created.Provider != "github" {
		t.Fatalf("result=%+v input=%+v", result, repository.created)
	}
}

func TestServiceCreateResolvesProjectFromRepositoryURL(t *testing.T) {
	repository := &fakeRepository{}
	service := NewServiceWithResolver(repository, resolverStub{metadata: scm.RepositoryMetadata{
		ProviderProjectID: 456, Name: "service", DefaultBranch: "trunk",
	}})
	result, err := service.Create(context.Background(), CreateInput{
		RepositoryURL: "https://github.com/acme/service.git",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "github" || result.ProviderProjectID != 456 ||
		repository.created.Name != "service" || repository.created.DefaultBranch != "trunk" {
		t.Fatalf("result=%+v input=%+v", result, repository.created)
	}
}

func TestServiceCreateRejectsMismatchedGitHubURL(t *testing.T) {
	_, err := NewService(&fakeRepository{}).Create(context.Background(), CreateInput{
		Name: "service", Provider: "github", ProviderProjectID: 456,
		RepositoryURL: "https://gitlab.com/acme/service",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error=%v, want ErrInvalidInput", err)
	}
}

func TestServiceCreateAppliesDefaults(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)

	_, err := service.Create(context.Background(), CreateInput{
		Name: " sample ", GitLabProjectID: 123, RepositoryURL: "https://gitlab.com/example/sample.git",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.created.Name != "sample" || repository.created.DefaultBranch != "main" || repository.created.Language != "go" {
		t.Fatalf("Create() normalized input = %+v", repository.created)
	}
}

func TestServiceCreateRejectsUnsupportedLanguage(t *testing.T) {
	service := NewService(&fakeRepository{})
	_, err := service.Create(context.Background(), CreateInput{
		Name: "sample", GitLabProjectID: 123, RepositoryURL: "https://gitlab.com/example/sample.git", Language: "java",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestServiceGetMissingProject(t *testing.T) {
	service := NewService(&fakeRepository{})
	_, err := service.GetByID(context.Background(), 99)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}
