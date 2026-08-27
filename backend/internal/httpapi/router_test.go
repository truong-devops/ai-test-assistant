package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/recommendation"
)

type checkerStub struct{ err error }

func (c checkerStub) Ping(context.Context) error { return c.err }

type repositoryStub struct {
	items     []project.Project
	createErr error
	readErr   error
}

func (r *repositoryStub) Create(_ context.Context, input project.CreateInput) (project.Project, error) {
	if r.createErr != nil {
		return project.Project{}, r.createErr
	}
	item := project.Project{ID: 1, Name: input.Name, GitLabProjectID: input.GitLabProjectID, RepositoryURL: input.RepositoryURL,
		DefaultBranch: input.DefaultBranch, Language: input.Language, Status: project.StatusActive}
	r.items = append(r.items, item)
	return item, nil
}
func (r *repositoryStub) List(context.Context) ([]project.Project, error) { return r.items, r.readErr }
func (r *repositoryStub) GetByID(_ context.Context, id int64) (project.Project, error) {
	if r.readErr != nil {
		return project.Project{}, r.readErr
	}
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return project.Project{}, project.ErrNotFound
}
func (r *repositoryStub) GetByGitLabProjectID(_ context.Context, id int64) (project.Project, error) {
	for _, item := range r.items {
		if item.GitLabProjectID == id {
			return item, nil
		}
	}
	return project.Project{}, project.ErrNotFound
}

func testRouter(checker ReadinessChecker) http.Handler {
	return testRouterWithDependencies(checker, &repositoryStub{}, nil)
}

func testRouterWithDependencies(checker ReadinessChecker, repository *repositoryStub, analyses AnalysisService) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(logger, checker, project.NewService(repository), analyses, nil)
}

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	testRouter(checkerStub{}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestRequestLoggerRecordsStatusAndRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := NewRouter(logger, checkerStub{}, project.NewService(&repositoryStub{}), nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("X-Request-ID", "test-request")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") != "test-request" {
		t.Fatalf("X-Request-ID = %q", response.Header().Get("X-Request-ID"))
	}
	if !strings.Contains(logs.String(), `"request_id":"test-request"`) || !strings.Contains(logs.String(), `"status":200`) {
		t.Fatalf("log = %s", logs.String())
	}
}

func TestReadyWhenDatabaseUnavailable(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	response := httptest.NewRecorder()
	testRouter(checkerStub{err: errors.New("offline")}).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestCreateProject(t *testing.T) {
	body := `{"name":"sample","gitlab_project_id":123,"repository_url":"https://gitlab.com/example/sample.git"}`
	request := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(body))
	response := httptest.NewRecorder()
	testRouter(checkerStub{}).ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
}

func TestProjectEndpoints(t *testing.T) {
	repository := &repositoryStub{items: []project.Project{{ID: 1, Name: "sample"}}}
	router := testRouterWithDependencies(checkerStub{}, repository, nil)
	tests := []struct {
		path   string
		status int
	}{
		{path: "/api/projects", status: http.StatusOK},
		{path: "/api/projects/1", status: http.StatusOK},
		{path: "/api/projects/99", status: http.StatusNotFound},
		{path: "/api/projects/not-a-number", status: http.StatusBadRequest},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.status {
			t.Fatalf("GET %s status=%d, want %d", test.path, response.Code, test.status)
		}
	}
}

func TestCreateProjectRejectsInvalidBodies(t *testing.T) {
	tests := []string{
		`{`,
		`{"name":"sample","gitlab_project_id":1,"repository_url":"https://gitlab.com/a/b","unknown":true}`,
		`{"name":"sample","gitlab_project_id":1,"repository_url":"https://gitlab.com/a/b"}{}`,
	}
	for _, body := range tests {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(body))
		testRouter(checkerStub{}).ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body %q status=%d, want %d", body, response.Code, http.StatusBadRequest)
		}
	}
}

func TestCreateProjectReturnsConflict(t *testing.T) {
	repository := &repositoryStub{createErr: project.ErrAlreadyExists}
	response := httptest.NewRecorder()
	body := `{"name":"sample","gitlab_project_id":123,"repository_url":"https://gitlab.com/example/sample.git"}`
	testRouterWithDependencies(checkerStub{}, repository, nil).ServeHTTP(response,
		httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(body)))
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusConflict)
	}
}

type analysisServiceStub struct {
	items   []job.AnalysisJob
	files   []job.ChangedFile
	symbols []job.ChangedSymbol
	err     error
}

func (s analysisServiceStub) GetSymbols(context.Context, int64) ([]job.ChangedSymbol, error) {
	return s.symbols, s.err
}

func (s analysisServiceStub) List(context.Context) ([]job.AnalysisJob, error) { return s.items, s.err }
func (s analysisServiceStub) Get(_ context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error) {
	if s.err != nil {
		return job.AnalysisJob{}, nil, s.err
	}
	for _, item := range s.items {
		if item.ID == id {
			return item, s.files, nil
		}
	}
	return job.AnalysisJob{}, nil, job.ErrNotFound
}

func TestAnalysisEndpoints(t *testing.T) {
	analyses := analysisServiceStub{items: []job.AnalysisJob{{ID: 3, Status: job.StatusAnalyzingChange}},
		files:   []job.ChangedFile{{ID: 4, NewPath: "service.go"}},
		symbols: []job.ChangedSymbol{{ID: 5, ChangedFileID: 4, SymbolName: "CreateUser"}}}
	router := testRouterWithDependencies(checkerStub{}, &repositoryStub{}, analyses)
	tests := []struct {
		path   string
		status int
	}{
		{path: "/api/analyses", status: http.StatusOK},
		{path: "/api/analyses/3", status: http.StatusOK},
		{path: "/api/analyses/3/changes", status: http.StatusOK},
		{path: "/api/analyses/99", status: http.StatusNotFound},
		{path: "/api/analyses/bad", status: http.StatusBadRequest},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.status {
			t.Fatalf("GET %s status=%d, want %d", test.path, response.Code, test.status)
		}
		if test.path == "/api/analyses/3/changes" &&
			(!strings.Contains(response.Body.String(), `"changed_symbols"`) ||
				!strings.Contains(response.Body.String(), `"CreateUser"`)) {
			t.Fatalf("GET %s body=%s", test.path, response.Body.String())
		}
	}
}

type knowledgeServiceStub struct {
	result knowledge.IndexJob
	err    error
}

func (s knowledgeServiceStub) RequestIndex(context.Context, int64) (knowledge.IndexJob, error) {
	return s.result, s.err
}
func (s knowledgeServiceStub) GetIndexStatus(context.Context, int64) (knowledge.IndexJob, error) {
	return s.result, s.err
}

func TestKnowledgeIndexEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repository := &repositoryStub{items: []project.Project{{ID: 3, Name: "sample"}}}
	service := knowledgeServiceStub{result: knowledge.IndexJob{ProjectID: 3, Ref: "main", Status: knowledge.IndexStatusPending}}
	router := NewRouterWithKnowledge(logger, checkerStub{}, project.NewService(repository), nil, nil, service)
	tests := []struct {
		method string
		path   string
		status int
	}{
		{method: http.MethodPost, path: "/api/projects/3/index", status: http.StatusAccepted},
		{method: http.MethodGet, path: "/api/projects/3/index/status", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/projects/bad/index", status: http.StatusBadRequest},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.status ||
			(test.status < 300 && !strings.Contains(response.Body.String(), `"PENDING"`)) {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

type recommendationServiceStub struct {
	items []recommendation.Recommendation
	err   error
}

func (s recommendationServiceStub) List(context.Context, int64) ([]recommendation.Recommendation, error) {
	return s.items, s.err
}

func TestRecommendationEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := recommendationServiceStub{items: []recommendation.Recommendation{{
		ID: 7, AnalysisJobID: 3, ChangedSymbolID: 5, Title: "Duplicate email",
		ExpectedBehavior: "returns ErrEmailExists", Status: recommendation.StatusPending,
	}}}
	router := NewRouterWithServices(logger, checkerStub{}, project.NewService(&repositoryStub{}),
		nil, nil, nil, service)
	tests := []struct {
		path   string
		status int
	}{
		{path: "/api/analyses/3/recommendations", status: http.StatusOK},
		{path: "/api/analyses/bad/recommendations", status: http.StatusBadRequest},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.status || test.status == http.StatusOK &&
			(!strings.Contains(response.Body.String(), "Duplicate email") ||
				!strings.Contains(response.Body.String(), "ErrEmailExists")) {
			t.Fatalf("GET %s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}

	notFoundRouter := NewRouterWithServices(logger, checkerStub{}, project.NewService(&repositoryStub{}),
		nil, nil, nil, recommendationServiceStub{err: job.ErrNotFound})
	response := httptest.NewRecorder()
	notFoundRouter.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/analyses/99/recommendations", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("not found status=%d body=%s", response.Code, response.Body.String())
	}
}
