package report

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Store interface {
	BuildSnapshot(context.Context, int64, ExportInput, time.Time) (Snapshot, error)
	Save(context.Context, ExportArtifact, Snapshot) (ExportArtifact, error)
	List(context.Context, int64) ([]ExportArtifact, error)
	Get(context.Context, int64) (ExportArtifact, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

func (s *Service) Export(ctx context.Context, setID int64, input ExportInput) (ExportArtifact, error) {
	input.Format = strings.ToUpper(strings.TrimSpace(input.Format))
	input.GeneratedBy = strings.TrimSpace(input.GeneratedBy)
	input.SortBy = strings.ToUpper(strings.TrimSpace(input.SortBy))
	if input.SortBy == "" {
		input.SortBy = "KEY"
	}
	if setID <= 0 || input.TestSuiteID <= 0 || input.GeneratedBy == "" ||
		(input.Format != FormatXLSX && input.Format != FormatMarkdown) ||
		(input.SortBy != "KEY" && input.SortBy != "TITLE" && input.SortBy != "RISK") ||
		input.SuiteReleaseID != nil && (*input.SuiteReleaseID <= 0 || len(input.TestCaseIDs) > 0) {
		return ExportArtifact{}, ErrInvalidInput
	}
	seen := map[int64]bool{}
	for _, id := range input.TestCaseIDs {
		if id <= 0 || seen[id] {
			return ExportArtifact{}, ErrInvalidInput
		}
		seen[id] = true
	}
	if input.TestCaseIDs == nil {
		input.TestCaseIDs = []int64{}
	}
	now := s.now().UTC()
	snapshot, err := s.store.BuildSnapshot(ctx, setID, input, now)
	if err != nil {
		return ExportArtifact{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return ExportArtifact{}, fmt.Errorf("encode snapshot: %w", err)
	}
	contentType, extension := "text/markdown; charset=utf-8", "md"
	content := RenderMarkdown(snapshot)
	if input.Format == FormatXLSX {
		contentType, extension = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx"
		content, err = RenderXLSX(snapshot)
		if err != nil {
			return ExportArtifact{}, err
		}
	}
	artifact := ExportArtifact{
		DocumentSetID: setID, TestSuiteID: input.TestSuiteID, TestRunID: input.TestRunID,
		SuiteReleaseID: snapshot.SuiteReleaseID,
		Format:         input.Format, Filename: fmt.Sprintf("%s-%s.%s", slug(snapshot.TestSuiteName), now.Format("20060102-150405"), extension),
		ContentType: contentType, Content: content, ContentHash: digest(content), SnapshotHash: digest(snapshotJSON),
		RowCount: len(snapshot.Rows), GeneratedBy: input.GeneratedBy,
	}
	return s.store.Save(ctx, artifact, snapshot)
}

func (s *Service) List(ctx context.Context, setID int64) ([]ExportArtifact, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.List(ctx, setID)
}

func (s *Service) Download(ctx context.Context, id int64) (ExportArtifact, error) {
	if id <= 0 {
		return ExportArtifact{}, ErrInvalidInput
	}
	artifact, err := s.store.Get(ctx, id)
	if err == nil && digest(artifact.Content) != artifact.ContentHash {
		return ExportArtifact{}, fmt.Errorf("stored export content hash mismatch")
	}
	return artifact, err
}

func digest(value []byte) string { hash := sha256.Sum256(value); return hex.EncodeToString(hash[:]) }

var unsafeSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(value string) string {
	value = strings.Trim(unsafeSlug.ReplaceAllString(strings.ToLower(value), "-"), "-")
	if value == "" {
		return "test-cases"
	}
	return value
}
