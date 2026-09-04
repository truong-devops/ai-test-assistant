package analyzer

import (
	"context"
	"errors"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/impact"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

type fakeProjects struct{ result project.Project }

func (f fakeProjects) GetByID(context.Context, int64) (project.Project, error) { return f.result, nil }

type fakeAnalyses struct {
	result job.AnalysisJob
	files  []job.ChangedFile
}

func (f fakeAnalyses) Get(context.Context, int64) (job.AnalysisJob, []job.ChangedFile, error) {
	return f.result, f.files, nil
}

type fakeGitLab struct{ files map[string][]byte }

func (f fakeGitLab) GetMergeRequest(context.Context, scm.Repository, int64) (gitlab.MergeRequest, error) {
	return gitlab.MergeRequest{}, nil
}
func (f fakeGitLab) GetMergeRequestDiff(context.Context, scm.Repository, int64) ([]gitlab.FileDiff, error) {
	return nil, nil
}
func (f fakeGitLab) GetFileRaw(_ context.Context, _ scm.Repository, path, ref string) ([]byte, error) {
	result, ok := f.files[ref+":"+path]
	if !ok {
		return nil, errors.New("file not found")
	}
	return result, nil
}
func (f fakeGitLab) ListRepositoryTree(context.Context, scm.Repository, string) ([]gitlab.RepositoryEntry, error) {
	return nil, nil
}

type symbolCapture struct{ symbols []job.ChangedSymbol }

func (s *symbolCapture) SaveSymbols(_ context.Context, _ int64, _ int, symbols []job.ChangedSymbol) error {
	s.symbols = symbols
	return nil
}

type impactAnalyzerStub struct {
	result impact.Result
	direct []impact.DirectSymbol
}

func (s *impactAnalyzerStub) AnalyzeRepository(_ context.Context, _ scm.Client, _ scm.Repository,
	_ string, direct []impact.DirectSymbol,
) (impact.Result, error) {
	s.direct = direct
	return s.result, nil
}

type impactCapture struct {
	analysisID, projectID int64
	symbols               []job.ChangedSymbol
	graph                 impact.Result
}

func (s *impactCapture) SaveAnalysis(_ context.Context, analysisID, projectID int64, _ int,
	symbols []job.ChangedSymbol, graph impact.Result,
) error {
	s.analysisID, s.projectID, s.symbols, s.graph = analysisID, projectID, symbols, graph
	return nil
}

func TestProcessorMapsBothFileVersions(t *testing.T) {
	oldSource := []byte("package service\n\nfunc Keep() {\n\tprintln(\"old\")\n}\n\nfunc Remove() {}\n")
	newSource := []byte("package service\n\nfunc Keep() {\n\tprintln(\"new\")\n}\n\nfunc Add() {}\n")
	files := []job.ChangedFile{{
		ID: 11, OldPath: "service.go", NewPath: "service.go",
		Diff: "@@ -3,5 +3,5 @@\n func Keep() {\n-\tprintln(\"old\")\n+\tprintln(\"new\")\n }\n \n-func Remove() {}\n+func Add() {}",
	}}
	capture := &symbolCapture{}
	processor := NewProcessor(fakeProjects{project.Project{GitLabProjectID: 99}},
		fakeGitLab{map[string][]byte{"target:service.go": oldSource, "source:service.go": newSource}},
		fakeAnalyses{job.AnalysisJob{SourceSHA: "source", TargetSHA: "target"}, files}, capture)
	if err := processor.Process(context.Background(), job.AnalysisJob{ID: 7, ProjectID: 2, AttemptCount: 1}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Keep": "modified", "Remove": "deleted", "Add": "added"}
	if len(capture.symbols) != len(want) {
		t.Fatalf("symbols = %#v", capture.symbols)
	}
	for _, symbol := range capture.symbols {
		if symbol.ChangedFileID != 11 || want[symbol.SymbolName] != symbol.ChangeType {
			t.Errorf("unexpected symbol: %#v", symbol)
		}
	}
}

func TestProcessorBuildsAndSavesImpactGraph(t *testing.T) {
	oldSource := []byte("package service\n\nfunc Load() { println(\"old\") }\n")
	newSource := []byte("package service\n\nfunc Load() { println(\"new\") }\n")
	files := []job.ChangedFile{{ID: 11, OldPath: "service.go", NewPath: "service.go",
		Diff: "@@ -3 +3 @@\n-func Load() { println(\"old\") }\n+func Load() { println(\"new\") }"}}
	engine := &impactAnalyzerStub{result: impact.Result{SourceSHA: "source", Mode: impact.ModeSSA,
		Algorithm: ImpactAlgorithm, MaxDepth: 3, MaxNodes: 250}}
	capture := &impactCapture{}
	processor := NewProcessorWithImpact(fakeProjects{project.Project{GitLabProjectID: 99}},
		fakeGitLab{map[string][]byte{"target:service.go": oldSource, "source:service.go": newSource}},
		fakeAnalyses{job.AnalysisJob{SourceSHA: "source", TargetSHA: "target"}, files}, engine, capture)
	if err := processor.Process(context.Background(), job.AnalysisJob{ID: 7, ProjectID: 2, AttemptCount: 1}); err != nil {
		t.Fatal(err)
	}
	if capture.analysisID != 7 || capture.projectID != 2 || len(capture.symbols) != 1 ||
		len(engine.direct) != 1 || engine.direct[0].FilePath != "service.go" || capture.graph.Mode != impact.ModeSSA {
		t.Fatalf("capture=%#v direct=%#v", capture, engine.direct)
	}
}

func TestProcessorSkipsNonGoFiles(t *testing.T) {
	capture := &symbolCapture{}
	processor := NewProcessor(fakeProjects{project.Project{GitLabProjectID: 99}}, fakeGitLab{},
		fakeAnalyses{job.AnalysisJob{SourceSHA: "source", TargetSHA: "target"},
			[]job.ChangedFile{{ID: 1, OldPath: "README.md", NewPath: "README.md"}}}, capture)
	if err := processor.Process(context.Background(), job.AnalysisJob{ID: 7, ProjectID: 2, AttemptCount: 1}); err != nil {
		t.Fatal(err)
	}
	if len(capture.symbols) != 0 {
		t.Fatalf("symbols = %#v", capture.symbols)
	}
}

func TestProcessorTreatsGoExtensionRenameAsAddOrDelete(t *testing.T) {
	tests := []struct {
		name       string
		file       job.ChangedFile
		files      map[string][]byte
		wantChange string
	}{
		{
			name: "renamed away from Go", wantChange: "deleted",
			file: job.ChangedFile{ID: 1, OldPath: "service.go", NewPath: "service.txt", RenamedFile: true,
				Diff: "@@ -1,3 +1 @@\n-package service\n-\n-func Remove() {}\n+plain text"},
			files: map[string][]byte{"target:service.go": []byte("package service\n\nfunc Remove() {}\n")},
		},
		{
			name: "renamed to Go", wantChange: "added",
			file: job.ChangedFile{ID: 1, OldPath: "service.txt", NewPath: "service.go", RenamedFile: true,
				Diff: "@@ -1 +1,3 @@\n-plain text\n+package service\n+\n+func Add() {}"},
			files: map[string][]byte{"source:service.go": []byte("package service\n\nfunc Add() {}\n")},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := &symbolCapture{}
			processor := NewProcessor(fakeProjects{project.Project{GitLabProjectID: 99}}, fakeGitLab{test.files},
				fakeAnalyses{job.AnalysisJob{SourceSHA: "source", TargetSHA: "target"}, []job.ChangedFile{test.file}}, capture)
			if err := processor.Process(context.Background(), job.AnalysisJob{ID: 7, ProjectID: 2, AttemptCount: 1}); err != nil {
				t.Fatal(err)
			}
			if len(capture.symbols) != 1 || capture.symbols[0].ChangeType != test.wantChange {
				t.Fatalf("symbols = %#v", capture.symbols)
			}
		})
	}
}

func TestProcessorRejectsUnavailableGoDiff(t *testing.T) {
	processor := NewProcessor(fakeProjects{project.Project{GitLabProjectID: 99}}, fakeGitLab{},
		fakeAnalyses{job.AnalysisJob{}, []job.ChangedFile{{OldPath: "main.go", NewPath: "main.go", Collapsed: true}}},
		&symbolCapture{})
	err := processor.Process(context.Background(), job.AnalysisJob{ID: 7, ProjectID: 2, AttemptCount: 1})
	if !errors.Is(err, ErrDiffUnavailable) {
		t.Fatalf("error = %v, want ErrDiffUnavailable", err)
	}
}
