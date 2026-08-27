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
)

const maxResponseBytes = 20 << 20

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
}

type Client interface {
	GetMergeRequest(ctx context.Context, projectID, iid int64) (MergeRequest, error)
	GetMergeRequestDiff(ctx context.Context, projectID, iid int64) ([]FileDiff, error)
}

type HTTPClient struct {
	apiBaseURL string
	token      string
	client     *http.Client
}

func NewHTTPClient(baseURL, token string, timeout time.Duration) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid GitLab base URL")
	}
	return &HTTPClient{
		apiBaseURL: strings.TrimRight(parsed.String(), "/") + "/api/v4",
		token:      token,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *HTTPClient) GetMergeRequest(ctx context.Context, projectID, iid int64) (MergeRequest, error) {
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

func (c *HTTPClient) GetMergeRequestDiff(ctx context.Context, projectID, iid int64) ([]FileDiff, error) {
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
