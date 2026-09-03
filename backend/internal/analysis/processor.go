package analysis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

var ErrMergeRequestNotReady = errors.New("merge request diff is not ready")

type ProjectGetter interface {
	GetByID(ctx context.Context, id int64) (project.Project, error)
}

type ResultSaver interface {
	SaveFetched(ctx context.Context, id int64, expectedAttempt int, metadata job.MergeRequestMetadata, files []job.ChangedFile) error
}

type Processor struct {
	projects ProjectGetter
	source   scm.Client
	results  ResultSaver
}

func NewProcessor(projects ProjectGetter, source scm.Client, results ResultSaver) *Processor {
	return &Processor{projects: projects, source: source, results: results}
}

func (p *Processor) Process(ctx context.Context, analysisJob job.AnalysisJob) error {
	registeredProject, err := p.projects.GetByID(ctx, analysisJob.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	repository := registeredProject.RepositoryRef()
	mergeRequest, err := p.source.GetMergeRequest(ctx, repository, analysisJob.MergeRequestIID)
	if err != nil {
		return err
	}
	sourceSHA := mergeRequest.DiffRefs.HeadSHA
	if sourceSHA == "" {
		sourceSHA = mergeRequest.SHA
	}
	targetSHA := mergeRequest.DiffRefs.StartSHA
	if targetSHA == "" {
		targetSHA = mergeRequest.DiffRefs.BaseSHA
	}
	if sourceSHA == "" || targetSHA == "" {
		return ErrMergeRequestNotReady
	}
	diffs, err := p.source.GetMergeRequestDiff(ctx, repository, analysisJob.MergeRequestIID)
	if err != nil {
		return err
	}
	if len(diffs) == 0 {
		return ErrMergeRequestNotReady
	}

	files := make([]job.ChangedFile, 0, len(diffs))
	for _, diff := range diffs {
		additions, deletions := countChangedLines(diff.Diff)
		if diff.Additions > 0 || diff.Deletions > 0 {
			additions, deletions = diff.Additions, diff.Deletions
		}
		files = append(files, job.ChangedFile{
			OldPath: diff.OldPath, NewPath: diff.NewPath, ChangeType: changeType(diff),
			Additions: additions, Deletions: deletions, Diff: diff.Diff, NewFile: diff.NewFile,
			RenamedFile: diff.RenamedFile, DeletedFile: diff.DeletedFile,
			Collapsed: diff.Collapsed, TooLarge: diff.TooLarge,
		})
	}
	return p.results.SaveFetched(ctx, analysisJob.ID, analysisJob.AttemptCount, job.MergeRequestMetadata{
		SourceSHA: sourceSHA, TargetSHA: targetSHA, SourceBranch: mergeRequest.SourceBranch,
		TargetBranch: mergeRequest.TargetBranch, Title: mergeRequest.Title, WebURL: mergeRequest.WebURL,
	}, files)
}

func changeType(diff scm.FileDiff) string {
	switch {
	case diff.NewFile:
		return "added"
	case diff.DeletedFile:
		return "deleted"
	case diff.RenamedFile:
		return "renamed"
	default:
		return "modified"
	}
}

func countChangedLines(diff string) (additions, deletions int) {
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			additions++
		case strings.HasPrefix(line, "-"):
			deletions++
		}
	}
	return additions, deletions
}
