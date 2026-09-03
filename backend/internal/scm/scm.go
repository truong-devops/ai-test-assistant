package scm

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	ProviderGitLab = "gitlab"
	ProviderGitHub = "github"
)

var ErrUnsupportedProvider = errors.New("unsupported source provider")

type Repository struct {
	Provider          string
	ProviderProjectID int64
	RepositoryURL     string
}

type MergeRequest struct {
	IID          int64    `json:"iid"`
	ProjectID    int64    `json:"project_id"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	SHA          string   `json:"sha"`
	WebURL       string   `json:"web_url"`
	DiffRefs     DiffRefs `json:"diff_refs"`
}

type DiffRefs struct {
	BaseSHA  string `json:"base_sha"`
	HeadSHA  string `json:"head_sha"`
	StartSHA string `json:"start_sha"`
}

type FileDiff struct {
	OldPath       string `json:"old_path"`
	NewPath       string `json:"new_path"`
	Diff          string `json:"diff"`
	NewFile       bool   `json:"new_file"`
	RenamedFile   bool   `json:"renamed_file"`
	DeletedFile   bool   `json:"deleted_file"`
	Collapsed     bool   `json:"collapsed"`
	TooLarge      bool   `json:"too_large"`
	GeneratedFile bool   `json:"generated_file"`
	Additions     int    `json:"additions"`
	Deletions     int    `json:"deletions"`
}

type RepositoryEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Mode string `json:"mode"`
}

type RepositoryMetadata struct {
	ProviderProjectID int64
	Name              string
	DefaultBranch     string
	WebURL            string
}

type MetadataResolver interface {
	ResolveRepository(ctx context.Context, repository Repository) (RepositoryMetadata, error)
}

type Client interface {
	GetMergeRequest(ctx context.Context, repository Repository, number int64) (MergeRequest, error)
	GetMergeRequestDiff(ctx context.Context, repository Repository, number int64) ([]FileDiff, error)
	GetFileRaw(ctx context.Context, repository Repository, filePath, ref string) ([]byte, error)
	ListRepositoryTree(ctx context.Context, repository Repository, ref string) ([]RepositoryEntry, error)
}

type Router struct {
	providers map[string]Client
}

func NewRouter(providers map[string]Client) (*Router, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("source provider registry is empty")
	}
	normalized := make(map[string]Client, len(providers))
	for name, client := range providers {
		name = strings.ToLower(strings.TrimSpace(name))
		if (name != ProviderGitLab && name != ProviderGitHub) || client == nil {
			return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, name)
		}
		normalized[name] = client
	}
	return &Router{providers: normalized}, nil
}

func (r *Router) GetMergeRequest(ctx context.Context, repository Repository, number int64) (MergeRequest, error) {
	client, err := r.client(repository)
	if err != nil {
		return MergeRequest{}, err
	}
	return client.GetMergeRequest(ctx, repository, number)
}

func (r *Router) GetMergeRequestDiff(ctx context.Context, repository Repository, number int64) ([]FileDiff, error) {
	client, err := r.client(repository)
	if err != nil {
		return nil, err
	}
	return client.GetMergeRequestDiff(ctx, repository, number)
}

func (r *Router) GetFileRaw(ctx context.Context, repository Repository, filePath, ref string) ([]byte, error) {
	client, err := r.client(repository)
	if err != nil {
		return nil, err
	}
	return client.GetFileRaw(ctx, repository, filePath, ref)
}

func (r *Router) ListRepositoryTree(ctx context.Context, repository Repository, ref string) ([]RepositoryEntry, error) {
	client, err := r.client(repository)
	if err != nil {
		return nil, err
	}
	return client.ListRepositoryTree(ctx, repository, ref)
}

func (r *Router) ResolveRepository(ctx context.Context, repository Repository) (RepositoryMetadata, error) {
	client, err := r.provider(repository.Provider)
	if err != nil {
		return RepositoryMetadata{}, err
	}
	resolver, ok := client.(MetadataResolver)
	if !ok {
		return RepositoryMetadata{}, fmt.Errorf("source provider %q cannot resolve repository metadata", repository.Provider)
	}
	return resolver.ResolveRepository(ctx, repository)
}

func (r *Router) client(repository Repository) (Client, error) {
	client, err := r.provider(repository.Provider)
	if err != nil {
		return nil, err
	}
	if repository.ProviderProjectID <= 0 {
		return nil, fmt.Errorf("source provider project ID must be positive")
	}
	return client, nil
}

func (r *Router) provider(name string) (Client, error) {
	provider := strings.ToLower(strings.TrimSpace(name))
	client, ok := r.providers[provider]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, provider)
	}
	return client, nil
}

func ParseGitHubRepository(repositoryURL string) (owner, repository string, err error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(repositoryURL))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") || parsed.User != nil {
		return "", "", fmt.Errorf("GitHub repository_url must be an https://github.com/owner/repository URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" {
		return "", "", fmt.Errorf("GitHub repository_url must not contain a port, query, or fragment")
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) != 2 {
		return "", "", fmt.Errorf("GitHub repository_url must identify exactly one owner and repository")
	}
	owner, err = url.PathUnescape(segments[0])
	if err != nil {
		return "", "", fmt.Errorf("invalid GitHub repository owner")
	}
	repository, err = url.PathUnescape(segments[1])
	if err != nil {
		return "", "", fmt.Errorf("invalid GitHub repository name")
	}
	repository = strings.TrimSuffix(repository, ".git")
	if !validSlug(owner) || !validSlug(repository) {
		return "", "", fmt.Errorf("invalid GitHub repository owner or name")
	}
	return owner, repository, nil
}

func validSlug(value string) bool {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) ||
		strings.ContainsRune(value, '\x00') {
		return false
	}
	return true
}

func ValidateRepositoryPath(filePath string) error {
	if filePath == "" || strings.HasPrefix(filePath, "/") || strings.ContainsRune(filePath, '\x00') {
		return fmt.Errorf("invalid repository file path")
	}
	for _, segment := range strings.Split(filePath, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid repository file path")
		}
	}
	return nil
}

func EscapePath(filePath string) (string, error) {
	if err := ValidateRepositoryPath(filePath); err != nil {
		return "", err
	}
	segments := strings.Split(filePath, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return strings.Join(segments, "/"), nil
}
