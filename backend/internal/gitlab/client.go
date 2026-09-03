package gitlab

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

const maxResponseBytes = 20 << 20

type MergeRequest = scm.MergeRequest
type DiffRefs = scm.DiffRefs
type FileDiff = scm.FileDiff
type RepositoryEntry = scm.RepositoryEntry
type Client = scm.Client

type HTTPClient struct {
	apiBaseURL     string
	repositoryHost string
	token          string
	client         *http.Client
}

func NewHTTPClient(baseURL, token string, timeout time.Duration) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid GitLab base URL")
	}
	return &HTTPClient{
		apiBaseURL:     strings.TrimRight(parsed.String(), "/") + "/api/v4",
		repositoryHost: strings.ToLower(parsed.Host),
		token:          token,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *HTTPClient) ResolveRepository(ctx context.Context, repository scm.Repository) (scm.RepositoryMetadata, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(repository.RepositoryURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		strings.ToLower(parsed.Host) != c.repositoryHost || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return scm.RepositoryMetadata{}, fmt.Errorf("GitLab repository_url must belong to configured GitLab host")
	}
	pathWithNamespace := strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git")
	if !strings.Contains(pathWithNamespace, "/") || scm.ValidateRepositoryPath(pathWithNamespace) != nil {
		return scm.RepositoryMetadata{}, fmt.Errorf("invalid GitLab repository path")
	}
	var response struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		DefaultBranch string `json:"default_branch"`
		WebURL        string `json:"web_url"`
	}
	if _, err := c.getJSON(ctx, "/projects/"+url.PathEscape(pathWithNamespace), nil, &response); err != nil {
		return scm.RepositoryMetadata{}, fmt.Errorf("resolve GitLab project: %w", err)
	}
	if response.ID <= 0 || strings.TrimSpace(response.Name) == "" {
		return scm.RepositoryMetadata{}, fmt.Errorf("GitLab returned incomplete project metadata")
	}
	return scm.RepositoryMetadata{
		ProviderProjectID: response.ID, Name: response.Name,
		DefaultBranch: response.DefaultBranch, WebURL: response.WebURL,
	}, nil
}

func (c *HTTPClient) GetMergeRequest(ctx context.Context, repository scm.Repository, iid int64) (MergeRequest, error) {
	projectID := repository.ProviderProjectID
	if projectID <= 0 || iid <= 0 {
		return MergeRequest{}, fmt.Errorf("project ID and merge request IID must be positive")
	}
	var result MergeRequest
	path := fmt.Sprintf("/projects/%d/merge_requests/%d", projectID, iid)
	if _, err := c.getJSON(ctx, path, nil, &result); err != nil {
		return MergeRequest{}, fmt.Errorf("get merge request: %w", err)
	}
	return result, nil
}

func (c *HTTPClient) GetMergeRequestDiff(ctx context.Context, repository scm.Repository, iid int64) ([]FileDiff, error) {
	projectID := repository.ProviderProjectID
	if projectID <= 0 || iid <= 0 {
		return nil, fmt.Errorf("project ID and merge request IID must be positive")
	}
	results := make([]FileDiff, 0)
	for page := 1; page <= 100; {
		query := url.Values{"page": {strconv.Itoa(page)}, "per_page": {"100"}, "unidiff": {"true"}}
		var batch []FileDiff
		path := fmt.Sprintf("/projects/%d/merge_requests/%d/diffs", projectID, iid)
		header, err := c.getJSON(ctx, path, query, &batch)
		if err != nil {
			return nil, fmt.Errorf("get merge request diffs page %d: %w", page, err)
		}
		results = append(results, batch...)
		nextPage := header.Get("X-Next-Page")
		if nextPage == "" {
			return results, nil
		}
		parsedNextPage, err := strconv.Atoi(nextPage)
		if err != nil || parsedNextPage <= page || parsedNextPage > 100 {
			return nil, fmt.Errorf("get merge request diffs: invalid X-Next-Page %q", nextPage)
		}
		page = parsedNextPage
	}
	return nil, fmt.Errorf("get merge request diffs: pagination exceeded 100 pages")
}

func (c *HTTPClient) GetFileRaw(ctx context.Context, repository scm.Repository, filePath, ref string) ([]byte, error) {
	projectID := repository.ProviderProjectID
	if projectID <= 0 {
		return nil, fmt.Errorf("project ID must be positive")
	}
	if err := scm.ValidateRepositoryPath(filePath); err != nil {
		return nil, err
	}
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("repository ref is required")
	}
	path := fmt.Sprintf("/projects/%d/repository/files/%s/raw", projectID, url.PathEscape(filePath))
	query := url.Values{"ref": {ref}}
	requestURL := c.apiBaseURL + path + "?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create repository file request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	if c.token != "" {
		request.Header.Set("PRIVATE-TOKEN", c.token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("get repository file: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("GitLab returned HTTP %d for repository file: %s",
			response.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read repository file: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("GitLab repository file exceeds %d bytes", maxResponseBytes)
	}
	return body, nil
}

func (c *HTTPClient) ListRepositoryTree(ctx context.Context, repository scm.Repository, ref string) ([]RepositoryEntry, error) {
	projectID := repository.ProviderProjectID
	if projectID <= 0 {
		return nil, fmt.Errorf("project ID must be positive")
	}
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("repository ref is required")
	}
	results := make([]RepositoryEntry, 0)
	for page := 1; page <= 100; {
		query := url.Values{
			"page": {strconv.Itoa(page)}, "per_page": {"100"}, "recursive": {"true"}, "ref": {ref},
		}
		var batch []RepositoryEntry
		header, err := c.getJSON(ctx, fmt.Sprintf("/projects/%d/repository/tree", projectID), query, &batch)
		if err != nil {
			return nil, fmt.Errorf("list repository tree page %d: %w", page, err)
		}
		results = append(results, batch...)
		nextPage := header.Get("X-Next-Page")
		if nextPage == "" {
			return results, nil
		}
		parsedNextPage, err := strconv.Atoi(nextPage)
		if err != nil || parsedNextPage <= page || parsedNextPage > 100 {
			return nil, fmt.Errorf("list repository tree: invalid X-Next-Page %q", nextPage)
		}
		page = parsedNextPage
	}
	return nil, fmt.Errorf("list repository tree: pagination exceeded 100 pages")
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
	request.Header.Set("Accept", "application/json")
	if c.token != "" {
		request.Header.Set("PRIVATE-TOKEN", c.token)
	}

	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return response.Header, fmt.Errorf("GitLab returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return response.Header, fmt.Errorf("read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return response.Header, fmt.Errorf("GitLab response exceeds %d bytes", maxResponseBytes)
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return response.Header, fmt.Errorf("decode response: %w", err)
	}
	return response.Header, nil
}
