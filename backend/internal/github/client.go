package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

const (
	maxResponseBytes = 20 << 20
	apiVersion       = "2026-03-10"
)

type HTTPClient struct {
	apiBaseURL string
	token      string
	client     *http.Client
}

func NewHTTPClient(baseURL, token string, timeout time.Duration) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid GitHub API base URL")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("GitHub request timeout must be positive")
	}
	return &HTTPClient{
		apiBaseURL: strings.TrimRight(parsed.String(), "/"),
		token:      strings.TrimSpace(token),
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

type pullRequestResponse struct {
	Number  int64  `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	Head    struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			ID int64 `json:"id"`
		} `json:"repo"`
	} `json:"base"`
}

func (c *HTTPClient) ResolveRepository(ctx context.Context, repository scm.Repository) (scm.RepositoryMetadata, error) {
	owner, name, err := scm.ParseGitHubRepository(repository.RepositoryURL)
	if err != nil {
		return scm.RepositoryMetadata{}, err
	}
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	var response struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		DefaultBranch string `json:"default_branch"`
		HTMLURL       string `json:"html_url"`
	}
	if _, err := c.getJSON(ctx, path, nil, &response); err != nil {
		return scm.RepositoryMetadata{}, fmt.Errorf("resolve GitHub repository: %w", err)
	}
	if response.ID <= 0 || strings.TrimSpace(response.Name) == "" {
		return scm.RepositoryMetadata{}, fmt.Errorf("GitHub returned incomplete repository metadata")
	}
	return scm.RepositoryMetadata{
		ProviderProjectID: response.ID, Name: response.Name,
		DefaultBranch: response.DefaultBranch, WebURL: response.HTMLURL,
	}, nil
}

func (c *HTTPClient) GetMergeRequest(ctx context.Context, repository scm.Repository, number int64) (scm.MergeRequest, error) {
	if number <= 0 {
		return scm.MergeRequest{}, fmt.Errorf("pull request number must be positive")
	}
	path, err := repositoryPath(repository)
	if err != nil {
		return scm.MergeRequest{}, err
	}
	var response pullRequestResponse
	if _, err := c.getJSON(ctx, fmt.Sprintf("%s/pulls/%d", path, number), nil, &response); err != nil {
		return scm.MergeRequest{}, fmt.Errorf("get pull request: %w", err)
	}
	if response.Base.Repo.ID > 0 && response.Base.Repo.ID != repository.ProviderProjectID {
		return scm.MergeRequest{}, fmt.Errorf("GitHub repository ID does not match registered provider_project_id")
	}
	return scm.MergeRequest{
		IID: response.Number, ProjectID: response.Base.Repo.ID, Title: response.Title, State: response.State,
		SourceBranch: response.Head.Ref, TargetBranch: response.Base.Ref, SHA: response.Head.SHA,
		WebURL:   response.HTMLURL,
		DiffRefs: scm.DiffRefs{BaseSHA: response.Base.SHA, StartSHA: response.Base.SHA, HeadSHA: response.Head.SHA},
	}, nil
}

type pullRequestFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Patch            string `json:"patch"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
}

func (c *HTTPClient) GetMergeRequestDiff(ctx context.Context, repository scm.Repository, number int64) ([]scm.FileDiff, error) {
	if number <= 0 {
		return nil, fmt.Errorf("pull request number must be positive")
	}
	path, err := repositoryPath(repository)
	if err != nil {
		return nil, err
	}
	results := make([]scm.FileDiff, 0)
	for page := 1; page <= 30; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "per_page": {"100"}}
		var batch []pullRequestFile
		header, err := c.getJSON(ctx, fmt.Sprintf("%s/pulls/%d/files", path, number), query, &batch)
		if err != nil {
			return nil, fmt.Errorf("get pull request files page %d: %w", page, err)
		}
		for _, file := range batch {
			oldPath := file.Filename
			if file.Status == "renamed" && file.PreviousFilename != "" {
				oldPath = file.PreviousFilename
			}
			results = append(results, scm.FileDiff{
				OldPath: oldPath, NewPath: file.Filename, Diff: file.Patch,
				NewFile: file.Status == "added", RenamedFile: file.Status == "renamed",
				DeletedFile: file.Status == "removed", TooLarge: file.Changes > 0 && strings.TrimSpace(file.Patch) == "",
				Additions: file.Additions, Deletions: file.Deletions,
			})
		}
		if !hasNextPage(header) {
			return results, nil
		}
	}
	return nil, fmt.Errorf("get pull request files: pagination exceeded GitHub's 3000-file limit")
}

func (c *HTTPClient) GetFileRaw(ctx context.Context, repository scm.Repository, filePath, ref string) ([]byte, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("repository ref is required")
	}
	path, err := repositoryPath(repository)
	if err != nil {
		return nil, err
	}
	escapedPath, err := scm.EscapePath(filePath)
	if err != nil {
		return nil, err
	}
	requestURL := c.apiBaseURL + path + "/contents/" + escapedPath + "?" +
		url.Values{"ref": {ref}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create repository file request: %w", err)
	}
	c.setHeaders(request, "application/vnd.github.raw+json")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("get repository file: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, responseError(response, "repository file")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read repository file: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("GitHub repository file exceeds %d bytes", maxResponseBytes)
	}
	return body, nil
}

type treeResponse struct {
	Truncated bool `json:"truncated"`
	Tree      []struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"tree"`
}

func (c *HTTPClient) ListRepositoryTree(ctx context.Context, repository scm.Repository, ref string) ([]scm.RepositoryEntry, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("repository ref is required")
	}
	path, err := repositoryPath(repository)
	if err != nil {
		return nil, err
	}
	var response treeResponse
	query := url.Values{"recursive": {"1"}}
	if _, err := c.getJSON(ctx, path+"/git/trees/"+url.PathEscape(ref), query, &response); err != nil {
		return nil, fmt.Errorf("list repository tree: %w", err)
	}
	if response.Truncated {
		return nil, fmt.Errorf("GitHub repository tree is truncated")
	}
	entries := make([]scm.RepositoryEntry, 0, len(response.Tree))
	for _, entry := range response.Tree {
		name := entry.Path
		if slash := strings.LastIndex(name, "/"); slash >= 0 {
			name = name[slash+1:]
		}
		entries = append(entries, scm.RepositoryEntry{
			ID: entry.SHA, Name: name, Type: entry.Type, Path: entry.Path, Mode: entry.Mode,
		})
	}
	return entries, nil
}

func repositoryPath(repository scm.Repository) (string, error) {
	if repository.ProviderProjectID <= 0 {
		return "", fmt.Errorf("GitHub repository ID must be positive")
	}
	owner, name, err := scm.ParseGitHubRepository(repository.RepositoryURL)
	if err != nil {
		return "", err
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name), nil
}

func (c *HTTPClient) getJSON(ctx context.Context, path string, query url.Values, destination any) (http.Header, error) {
	requestURL := c.apiBaseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(request, "application/vnd.github+json")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.Header, responseError(response, "API request")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return response.Header, fmt.Errorf("read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return response.Header, fmt.Errorf("GitHub response exceeds %d bytes", maxResponseBytes)
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return response.Header, fmt.Errorf("decode response: %w", err)
	}
	return response.Header, nil
}

func (c *HTTPClient) setHeaders(request *http.Request, accept string) {
	request.Header.Set("Accept", accept)
	request.Header.Set("X-GitHub-Api-Version", apiVersion)
	request.Header.Set("User-Agent", "ai-test-assistant")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func responseError(response *http.Response, operation string) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	return fmt.Errorf("GitHub returned HTTP %d for %s: %s",
		response.StatusCode, operation, strings.TrimSpace(string(body)))
}

func hasNextPage(header http.Header) bool {
	for _, link := range strings.Split(header.Get("Link"), ",") {
		if strings.Contains(link, `rel="next"`) {
			return true
		}
	}
	return false
}
