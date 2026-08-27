package project

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var ErrInvalidInput = errors.New("invalid project input")

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Project, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.RepositoryURL = strings.TrimSpace(input.RepositoryURL)
	input.DefaultBranch = strings.TrimSpace(input.DefaultBranch)
	input.Language = strings.ToLower(strings.TrimSpace(input.Language))

	if input.Name == "" {
		return Project{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if input.GitLabProjectID <= 0 {
		return Project{}, fmt.Errorf("%w: gitlab_project_id must be positive", ErrInvalidInput)
	}
	parsedURL, err := url.ParseRequestURI(input.RepositoryURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return Project{}, fmt.Errorf("%w: repository_url must be an HTTP(S) URL", ErrInvalidInput)
	}
	if input.DefaultBranch == "" {
		input.DefaultBranch = "main"
	}
	if input.Language == "" {
		input.Language = "go"
	}
	if input.Language != "go" {
		return Project{}, fmt.Errorf("%w: only Go is supported in the MVP", ErrInvalidInput)
	}
	return s.repository.Create(ctx, input)
}

func (s *Service) List(ctx context.Context) ([]Project, error) {
	return s.repository.List(ctx)
}

func (s *Service) GetByID(ctx context.Context, id int64) (Project, error) {
	if id <= 0 {
		return Project{}, ErrNotFound
	}
	return s.repository.GetByID(ctx, id)
}
