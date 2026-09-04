package analyzer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/impact"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

var ErrDiffUnavailable = errors.New("changed Go file diff is unavailable")

type ProjectGetter interface {
	GetByID(ctx context.Context, id int64) (project.Project, error)
}

type AnalysisReader interface {
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
}

type SymbolSaver interface {
	SaveSymbols(ctx context.Context, analysisID int64, expectedAttempt int, symbols []job.ChangedSymbol) error
}

type ImpactAnalyzer interface {
	AnalyzeRepository(ctx context.Context, source scm.Client, repository scm.Repository,
		sourceSHA string, direct []impact.DirectSymbol) (impact.Result, error)
}

type ImpactSaver interface {
	SaveAnalysis(ctx context.Context, analysisID, projectID int64, expectedAttempt int,
		symbols []job.ChangedSymbol, graph impact.Result) error
}

type Processor struct {
	projects       ProjectGetter
	source         scm.Client
	analyses       AnalysisReader
	results        SymbolSaver
	impactAnalyzer ImpactAnalyzer
	impactResults  ImpactSaver
}

func NewProcessorWithImpact(projects ProjectGetter, source scm.Client, analyses AnalysisReader,
	impactAnalyzer ImpactAnalyzer, results ImpactSaver,
) *Processor {
	return &Processor{projects: projects, source: source, analyses: analyses,
		impactAnalyzer: impactAnalyzer, impactResults: results}
}

func NewProcessor(projects ProjectGetter, source scm.Client, analyses AnalysisReader, results SymbolSaver) *Processor {
	return &Processor{projects: projects, source: source, analyses: analyses, results: results}
}

func (p *Processor) Process(ctx context.Context, claimed job.AnalysisJob) error {
	registeredProject, err := p.projects.GetByID(ctx, claimed.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	analysisJob, files, err := p.analyses.Get(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("get fetched analysis: %w", err)
	}
	symbols := make([]job.ChangedSymbol, 0)
	direct := make([]impact.DirectSymbol, 0)
	for _, file := range files {
		if !isGoFile(file) {
			continue
		}
		if file.Collapsed || file.TooLarge {
			return fmt.Errorf("%w: %s", ErrDiffUnavailable, displayPath(file))
		}
		lines, err := ParseChangedLines(file.Diff)
		if err != nil {
			return fmt.Errorf("parse diff for %q: %w", displayPath(file), err)
		}
		oldFile, err := p.loadVersion(ctx, registeredProject.RepositoryRef(), file.OldPath,
			analysisJob.TargetSHA, !file.NewFile && strings.HasSuffix(file.OldPath, ".go"))
		if err != nil {
			return fmt.Errorf("load target version of %q: %w", file.OldPath, err)
		}
		newFile, err := p.loadVersion(ctx, registeredProject.RepositoryRef(), file.NewPath,
			analysisJob.SourceSHA, !file.DeletedFile && strings.HasSuffix(file.NewPath, ".go"))
		if err != nil {
			return fmt.Errorf("load source version of %q: %w", file.NewPath, err)
		}
		for _, change := range MapChangedSymbols(oldFile, newFile, lines) {
			symbol := job.ChangedSymbol{
				ChangedFileID: file.ID, SymbolName: change.Name, SymbolKind: change.Kind,
				ReceiverName: change.Receiver, PackageName: change.PackageName,
				StartLine: change.StartLine, EndLine: change.EndLine,
				ChangeType: change.ChangeType, ChangeSummary: change.Summary,
			}
			symbols = append(symbols, symbol)
			direct = append(direct, impact.DirectSymbol{FilePath: displayPath(file), Symbol: symbol})
		}
	}
	if p.impactAnalyzer != nil && p.impactResults != nil {
		graph, err := p.impactAnalyzer.AnalyzeRepository(ctx, p.source,
			registeredProject.RepositoryRef(), analysisJob.SourceSHA, direct)
		if err != nil {
			return fmt.Errorf("analyze change impact: %w", err)
		}
		return p.impactResults.SaveAnalysis(ctx, claimed.ID, claimed.ProjectID,
			claimed.AttemptCount, symbols, graph)
	}
	return p.results.SaveSymbols(ctx, claimed.ID, claimed.AttemptCount, symbols)
}

func (p *Processor) loadVersion(ctx context.Context, repository scm.Repository, filePath, ref string, required bool) (*ParsedFile, error) {
	if !required {
		return nil, nil
	}
	source, err := p.source.GetFileRaw(ctx, repository, filePath, ref)
	if err != nil {
		return nil, err
	}
	parsed, err := ParseGoFile(filePath, source)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func isGoFile(file job.ChangedFile) bool {
	return strings.HasSuffix(file.OldPath, ".go") || strings.HasSuffix(file.NewPath, ".go")
}

func displayPath(file job.ChangedFile) string {
	if file.DeletedFile {
		return file.OldPath
	}
	return file.NewPath
}
