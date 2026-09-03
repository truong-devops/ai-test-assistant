package project

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

var ErrInvalidInput = errors.New("invalid project input")

type Service struct {
	repository Repository
	resolver   scm.MetadataResolver
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func NewServiceWithResolver(repository Repository, resolver scm.MetadataResolver) *Service {
	return &Service{repository: repository, resolver: resolver}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Project, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.RepositoryURL = strings.TrimSpace(input.RepositoryURL)
	input.DefaultBranch = strings.TrimSpace(input.DefaultBranch)
	input.Language = strings.ToLower(strings.TrimSpace(input.Language))
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	if input.Provider == "" {
		if parsedURL, err := url.ParseRequestURI(input.RepositoryURL); err == nil &&
			strings.EqualFold(parsedURL.Hostname(), "github.com") {
			input.Provider = scm.ProviderGitHub
		} else {
			input.Provider = scm.ProviderGitLab
		}
	}
	if input.ProviderProjectID <= 0 && input.Provider == scm.ProviderGitLab {
		input.ProviderProjectID = input.GitLabProjectID
	}

	if input.Provider != scm.ProviderGitLab && input.Provider != scm.ProviderGitHub {
		return Project{}, fmt.Errorf("%w: provider must be gitlab or github", ErrInvalidInput)
	}
	parsedURL, err := url.ParseRequestURI(input.RepositoryURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return Project{}, fmt.Errorf("%w: repository_url must be an HTTP(S) URL", ErrInvalidInput)
	}
	if input.Provider == scm.ProviderGitHub {
		if _, _, err := scm.ParseGitHubRepository(input.RepositoryURL); err != nil {
			return Project{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}
	if input.ProviderProjectID <= 0 && s.resolver != nil {
		metadata, err := s.resolver.ResolveRepository(ctx, scm.Repository{
			Provider: input.Provider, RepositoryURL: input.RepositoryURL,
		})
		if err != nil {
			return Project{}, fmt.Errorf("%w: could not resolve repository: %v", ErrInvalidInput, err)
		}
		input.ProviderProjectID = metadata.ProviderProjectID
		if input.Name == "" {
			input.Name = strings.TrimSpace(metadata.Name)
		}
		if input.DefaultBranch == "" {
			input.DefaultBranch = strings.TrimSpace(metadata.DefaultBranch)
		}
	}
	if input.Name == "" {
		return Project{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if input.ProviderProjectID <= 0 {
		return Project{}, fmt.Errorf("%w: provider_project_id must be positive", ErrInvalidInput)
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
