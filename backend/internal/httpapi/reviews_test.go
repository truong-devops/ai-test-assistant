package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/review"
)

type reviewServiceStub struct {
	result  review.Review
	reviews []review.Review
	err     error

	generatedTestID int64
	decision        string
	input           review.DecisionInput
}

func (s *reviewServiceStub) Decide(_ context.Context, generatedTestID int64, decision string,
	input review.DecisionInput,
) (review.Review, error) {
	s.generatedTestID, s.decision, s.input = generatedTestID, decision, input
	return s.result, s.err
}

func (s *reviewServiceStub) List(context.Context, int64) ([]review.Review, error) {
	return s.reviews, s.err
}

func TestReviewHandlerSavesDecision(t *testing.T) {
	service := &reviewServiceStub{result: review.Review{ID: 9, GeneratedTestID: 7,
		Decision: review.DecisionAccepted, ReviewerName: "Minh"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithPhaseNineServices(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, service, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/generated-tests/7/accept",
		strings.NewReader(`{"reviewer_name":"Minh","comment":"Looks good"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || service.generatedTestID != 7 ||
		service.decision != review.DecisionAccepted || service.input.Comment != "Looks good" {
		t.Fatalf("status=%d service=%#v body=%s", response.Code, service, response.Body.String())
	}
	var body struct {
		Review review.Review `json:"review"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Review.ID != 9 {
		t.Fatalf("body=%s error=%v", response.Body.String(), err)
	}
}

func TestReviewHandlerMapsErrorsAndRejectsInvalidBodies(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		body   string
		err    error
		status int
	}{
		{name: "missing generated test", path: "/api/generated-tests/7/reject", body: `{}`, err: job.ErrNotFound, status: http.StatusNotFound},
		{name: "not ready", path: "/api/generated-tests/7/accept", body: `{}`, err: review.ErrNotReady, status: http.StatusConflict},
		{name: "stale version", path: "/api/generated-tests/7/accept", body: `{}`, err: review.ErrStaleVersion, status: http.StatusConflict},
		{name: "validation failed", path: "/api/generated-tests/7/accept", body: `{}`, err: review.ErrValidationFailed, status: http.StatusConflict},
		{name: "already reviewed", path: "/api/generated-tests/7/accept", body: `{}`, err: review.ErrAlreadyReviewed, status: http.StatusConflict},
		{name: "invalid input", path: "/api/generated-tests/7/accept", body: `{}`, err: review.ErrInvalidInput, status: http.StatusBadRequest},
		{name: "invalid identifier", path: "/api/generated-tests/nope/accept", body: `{}`, status: http.StatusBadRequest},
		{name: "unknown JSON field", path: "/api/generated-tests/7/accept", body: `{"unknown":true}`, status: http.StatusBadRequest},
		{name: "multiple JSON values", path: "/api/generated-tests/7/accept", body: `{} {}`, status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &reviewServiceStub{err: test.err}
			handler := reviewHandler{service: service}
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.SetPathValue("id", strings.TrimPrefix(strings.Split(strings.TrimPrefix(test.path, "/api/generated-tests/"), "/")[0], "/"))
			response := httptest.NewRecorder()
			handler.decide(response, request, review.DecisionAccepted)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}
}

func TestReviewHandlerListsDecisions(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/analyses/3/reviews", nil)
	request.SetPathValue("id", "3")
	response := httptest.NewRecorder()
	reviewHandler{service: &reviewServiceStub{reviews: []review.Review{{ID: 1, GeneratedTestID: 2}}}}.list(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"reviews"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/analyses/3/reviews", nil)
	request.SetPathValue("id", "3")
	response = httptest.NewRecorder()
	reviewHandler{service: &reviewServiceStub{err: errors.New("database")}}.list(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
