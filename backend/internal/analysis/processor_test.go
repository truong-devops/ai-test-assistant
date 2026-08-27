package analysis

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

type projectGetterStub struct{ item project.Project }

func (s projectGetterStub) GetByID(context.Context, int64) (project.Project, error) {
	return s.item, nil
}

func TestProcessorRetriesWhenMergeRequestIsNotReady(t *testing.T) {
	client := &gitLabStub{mergeRequest: gitlab.MergeRequest{Title: "preparing"}}
	processor := NewProcessor(projectGetterStub{item: project.Project{GitLabProjectID: 88}}, client, &saverStub{})
	err := processor.Process(context.Background(), job.AnalysisJob{ID: 1, ProjectID: 2, MergeRequestIID: 3})
	if !errors.Is(err, ErrMergeRequestNotReady) {
		t.Fatalf("Process() error = %v, want ErrMergeRequestNotReady", err)
	}
}

func TestChangeType(t *testing.T) {
	tests := []struct {
		diff gitlab.FileDiff
		want string
	}{
		{diff: gitlab.FileDiff{NewFile: true}, want: "added"},
		{diff: gitlab.FileDiff{DeletedFile: true}, want: "deleted"},
		{diff: gitlab.FileDiff{RenamedFile: true}, want: "renamed"},
		{diff: gitlab.FileDiff{}, want: "modified"},
	}
	for _, test := range tests {
		if got := changeType(test.diff); got != test.want {
			t.Fatalf("changeType(%+v) = %q, want %q", test.diff, got, test.want)
		}
	}
}

type gitLabStub struct {
	mergeRequest gitlab.MergeRequest
	diffs        []gitlab.FileDiff
	projectID    int64
}

func (s *gitLabStub) GetMergeRequest(_ context.Context, projectID, _ int64) (gitlab.MergeRequest, error) {
	s.projectID = projectID
	return s.mergeRequest, nil
}
func (s *gitLabStub) GetMergeRequestDiff(context.Context, int64, int64) ([]gitlab.FileDiff, error) {
	return s.diffs, nil
}
func (s *gitLabStub) GetFileRaw(context.Context, int64, string, string) ([]byte, error) {
	return nil, nil
}
func (s *gitLabStub) ListRepositoryTree(context.Context, int64, string) ([]gitlab.RepositoryEntry, error) {
	return nil, nil
}

type saverStub struct {
	metadata job.MergeRequestMetadata
	files    []job.ChangedFile
}

func (s *saverStub) SaveFetched(_ context.Context, _ int64, _ int, metadata job.MergeRequestMetadata, files []job.ChangedFile) error {
	s.metadata, s.files = metadata, files
	return nil
}

func TestProcessorFetchesAndNormalizesChanges(t *testing.T) {
	client := &gitLabStub{
		mergeRequest: gitlab.MergeRequest{Title: "change", SourceBranch: "feature", TargetBranch: "main",
			DiffRefs: gitlab.DiffRefs{HeadSHA: "head", StartSHA: "target"}},
		diffs: []gitlab.FileDiff{{OldPath: "old.go", NewPath: "new.go", RenamedFile: true,
			Diff: "--- a/old.go\n+++ b/new.go\n-old\n+new\n+another"}},
	}
	saver := &saverStub{}
	processor := NewProcessor(projectGetterStub{item: project.Project{GitLabProjectID: 88}}, client, saver)
	if err := processor.Process(context.Background(), job.AnalysisJob{ID: 1, ProjectID: 2, MergeRequestIID: 3}); err != nil {
		t.Fatal(err)
	}
	if client.projectID != 88 || saver.metadata.SourceSHA != "head" {
		t.Fatalf("client project = %d metadata = %+v", client.projectID, saver.metadata)
	}
	if len(saver.files) != 1 || saver.files[0].ChangeType != "renamed" || saver.files[0].Additions != 2 || saver.files[0].Deletions != 1 {
		t.Fatalf("files = %+v", saver.files)
	}
}

func FuzzCountChangedLines(f *testing.F) {
	f.Add("@@ -1 +1 @@\n-old\n+new")
	f.Add("")
	f.Add("+++ header\n--- header\n+one\n-two")
	f.Fuzz(func(t *testing.T, diff string) {
		additions, deletions := countChangedLines(diff)
		if additions < 0 || deletions < 0 {
			t.Fatalf("negative counts: additions=%d deletions=%d", additions, deletions)
		}
		lineCount := len(strings.Split(diff, "\n"))
		if additions+deletions > lineCount {
			t.Fatalf("counts exceed lines: additions=%d deletions=%d lines=%d", additions, deletions, lineCount)
		}
	})
}
