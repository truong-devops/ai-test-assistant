package project

import (
	"context"
	"errors"
	"testing"
)

type fakeRepository struct {
	created CreateInput
	items   []Project
	err     error
}

func (f *fakeRepository) Create(_ context.Context, input CreateInput) (Project, error) {
	f.created = input
	if f.err != nil {
		return Project{}, f.err
	}
	return Project{ID: 1, Name: input.Name, Language: input.Language}, nil
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
	for _, item := range f.items {
		if item.GitLabProjectID == id {
			return item, nil
		}
	}
	return Project{}, ErrNotFound
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
