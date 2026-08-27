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
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
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
	items []job.AnalysisJob
	files []job.ChangedFile
	err   error
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
		files: []job.ChangedFile{{ID: 4, NewPath: "service.go"}}}
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
	}
}
