package review

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type analysisGetterStub struct{ err error }

func (s analysisGetterStub) Get(context.Context, int64) (job.AnalysisJob, []job.ChangedFile, error) {
	return job.AnalysisJob{}, nil, s.err
}

type repositoryStub struct {
	generatedTestID int64
	decision        string
	reviewerName    string
	comment         string
	decideErr       error
	listResult      []Review
	listErr         error
}

func (s *repositoryStub) Decide(_ context.Context, generatedTestID int64, decision, reviewerName, comment string) (Review, error) {
	s.generatedTestID, s.decision, s.reviewerName, s.comment = generatedTestID, decision, reviewerName, comment
	return Review{GeneratedTestID: generatedTestID, Decision: decision, ReviewerName: reviewerName, Comment: comment}, s.decideErr
}

func (s *repositoryStub) List(context.Context, int64) ([]Review, error) {
	return s.listResult, s.listErr
}

func TestServiceNormalizesReviewInput(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(analysisGetterStub{}, repository)
	result, err := service.Decide(context.Background(), 7, " accepted ", DecisionInput{
		ReviewerName: "  Lan Nguyen  ", Comment: "  Covers the new validation branch.  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionAccepted || repository.generatedTestID != 7 ||
		repository.decision != DecisionAccepted || repository.reviewerName != "Lan Nguyen" ||
		repository.comment != "Covers the new validation branch." {
		t.Fatalf("result=%#v repository=%#v", result, repository)
	}
}

func TestServiceUsesLocalReviewerDefault(t *testing.T) {
	repository := &repositoryStub{}
	_, err := NewService(analysisGetterStub{}, repository).Decide(context.Background(), 8, DecisionRejected, DecisionInput{})
	if err != nil || repository.reviewerName != DefaultReviewerName {
		t.Fatalf("error=%v reviewer=%q", err, repository.reviewerName)
	}
}

func TestServiceRejectsInvalidReviewInput(t *testing.T) {
	service := NewService(analysisGetterStub{}, &repositoryStub{})
	tests := []struct {
		id       int64
		decision string
		input    DecisionInput
	}{
		{id: 0, decision: DecisionAccepted},
		{id: 1, decision: "MAYBE"},
		{id: 1, decision: DecisionAccepted, input: DecisionInput{ReviewerName: strings.Repeat("x", MaxReviewerNameRunes+1)}},
		{id: 1, decision: DecisionAccepted, input: DecisionInput{Comment: strings.Repeat("x", MaxCommentBytes+1)}},
	}
	for _, test := range tests {
		if _, err := service.Decide(context.Background(), test.id, test.decision, test.input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Decide(%#v) error=%v, want ErrInvalidInput", test, err)
		}
	}
}

func TestServiceListChecksAnalysisExists(t *testing.T) {
	repository := &repositoryStub{listResult: []Review{{ID: 1}}}
	service := NewService(analysisGetterStub{}, repository)
	if _, err := service.List(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.List(context.Background(), 0); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("List(0) error=%v", err)
	}
	if _, err := NewService(analysisGetterStub{err: job.ErrNotFound}, repository).List(context.Background(), 1); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("List missing analysis error=%v", err)
	}
}
