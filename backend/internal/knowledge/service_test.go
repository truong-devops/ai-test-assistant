package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type indexRequesterStub struct {
	requestedRef string
	result       IndexJob
	err          error
}

func (s *indexRequesterStub) RequestIndex(_ context.Context, _ int64, ref string) (IndexJob, error) {
	s.requestedRef = ref
	return s.result, s.err
}
func (s *indexRequesterStub) GetIndex(context.Context, int64) (IndexJob, error) {
	return s.result, s.err
}

func TestServiceUsesProjectDefaultBranch(t *testing.T) {
	indexes := &indexRequesterStub{result: IndexJob{Status: IndexStatusPending}}
	service := NewService(indexProjectStub{project.Project{ID: 2, DefaultBranch: "develop"}}, indexes)
	if _, err := service.RequestIndex(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if indexes.requestedRef != "develop" {
		t.Fatalf("requested ref=%q", indexes.requestedRef)
	}
}

func TestServiceReturnsNotIndexedStatus(t *testing.T) {
	indexes := &indexRequesterStub{err: ErrIndexNotFound}
	service := NewService(indexProjectStub{project.Project{ID: 2, DefaultBranch: "main"}}, indexes)
	result, err := service.GetIndexStatus(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != IndexStatusNotIndexed || result.Ref != "main" {
		t.Fatalf("result=%#v", result)
	}
	body, err := json.Marshal(result)
	if err != nil || strings.Contains(string(body), "0001-01-01") {
		t.Fatalf("not-indexed JSON=%s error=%v", body, err)
	}
	if !errors.Is(indexes.err, ErrIndexNotFound) {
		t.Fatal("stub error changed unexpectedly")
	}
}
