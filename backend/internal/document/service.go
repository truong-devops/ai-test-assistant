package document

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/aibudget"
)

const (
	maxSetNameRunes      = 200
	maxProductNameRunes  = 200
	maxScopeRunes        = 1000
	maxDescriptionRunes  = 4000
	maxDocumentNameRunes = 240
	maxFilenameRunes     = 255
)

type Repository interface {
	CreateSet(context.Context, CreateSetInput) (Set, error)
	ListSets(context.Context) ([]Set, error)
	GetSet(context.Context, int64) (Set, error)
	CreateVersion(context.Context, int64, UploadInput, StoredFile) (Document, Version, error)
	ListDocuments(context.Context, int64) ([]Document, error)
	GetVersion(context.Context, int64, int) (Document, Version, error)
	ListBlocks(context.Context, int64) ([]Block, error)
}

type Service struct {
	repository Repository
	files      FileStore
	budget     *aibudget.Manager
}

func (s *Service) ConfigureAIBudget(manager *aibudget.Manager) *Service {
	s.budget = manager
	return s
}

func NewService(repository Repository, files FileStore) *Service {
	return &Service{repository: repository, files: files}
}

func (s *Service) CreateSet(ctx context.Context, input CreateSetInput) (Set, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.ProductName = strings.TrimSpace(input.ProductName)
	input.Scope = strings.TrimSpace(input.Scope)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > maxSetNameRunes ||
		strings.ContainsRune(input.Name, '\x00') {
		return Set{}, fmt.Errorf("%w: name must contain 1-%d characters", ErrInvalidInput, maxSetNameRunes)
	}
	if utf8.RuneCountInString(input.ProductName) > maxProductNameRunes ||
		strings.ContainsRune(input.ProductName, '\x00') {
		return Set{}, fmt.Errorf("%w: product_name is too long or invalid", ErrInvalidInput)
	}
	if utf8.RuneCountInString(input.Scope) > maxScopeRunes || strings.ContainsRune(input.Scope, '\x00') {
		return Set{}, fmt.Errorf("%w: scope is too long or invalid", ErrInvalidInput)
	}
	if utf8.RuneCountInString(input.Description) > maxDescriptionRunes ||
		strings.ContainsRune(input.Description, '\x00') {
		return Set{}, fmt.Errorf("%w: description is too long or invalid", ErrInvalidInput)
	}
	return s.repository.CreateSet(ctx, input)
}

func (s *Service) ListSets(ctx context.Context) ([]Set, error) {
	return s.repository.ListSets(ctx)
}

func (s *Service) GetSet(ctx context.Context, id int64) (Set, error) {
	if id <= 0 {
		return Set{}, ErrNotFound
	}
	return s.repository.GetSet(ctx, id)
}

func (s *Service) Metrics(ctx context.Context) (PipelineMetrics, error) {
	repository, ok := s.repository.(interface {
		Metrics(context.Context) (PipelineMetrics, error)
	})
	if !ok {
		return PipelineMetrics{}, ErrUnsupported
	}
	return repository.Metrics(ctx)
}

func (s *Service) UpdateLifecycle(ctx context.Context, id int64, input LifecycleInput) (Set, error) {
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	input.Actor = strings.TrimSpace(input.Actor)
	input.Reason = strings.TrimSpace(input.Reason)
	if id <= 0 || input.Actor == "" || input.Reason == "" ||
		(input.Status != SetStatusActive && input.Status != SetStatusArchived) ||
		input.RetentionDays < 30 || input.RetentionDays > 3650 ||
		(input.AITokenBudget != 0 && (input.AITokenBudget < 1000 || input.AITokenBudget > 1000000000)) ||
		input.AICostBudgetMicroUSD < 0 || input.AICostBudgetMicroUSD > 1000000000000 {
		return Set{}, ErrInvalidInput
	}
	repository, ok := s.repository.(interface {
		UpdateLifecycle(context.Context, int64, LifecycleInput) (Set, error)
	})
	if !ok {
		return Set{}, ErrUnsupported
	}
	return repository.UpdateLifecycle(ctx, id, input)
}

func (s *Service) PurgePreview(ctx context.Context, id int64) (PurgePreview, error) {
	if id <= 0 {
		return PurgePreview{}, ErrNotFound
	}
	repository, ok := s.repository.(interface {
		PurgePreview(context.Context, int64) (PurgePreview, error)
	})
	if !ok {
		return PurgePreview{}, ErrUnsupported
	}
	return repository.PurgePreview(ctx, id)
}

func (s *Service) Purge(ctx context.Context, id int64, input PurgeInput) (PurgeResult, error) {
	input.Actor = strings.TrimSpace(input.Actor)
	input.Reason = strings.TrimSpace(input.Reason)
	input.Confirmation = strings.TrimSpace(input.Confirmation)
	if id <= 0 || input.Actor == "" || input.Reason == "" || input.Confirmation == "" {
		return PurgeResult{}, ErrInvalidInput
	}
	repository, ok := s.repository.(interface {
		BeginPurge(context.Context, int64, PurgeInput) (PurgePlan, error)
		CompletePurge(context.Context, PurgePlan) error
		FailPurge(context.Context, PurgePlan, error) error
	})
	if !ok {
		return PurgeResult{}, ErrUnsupported
	}
	plan, err := repository.BeginPurge(ctx, id, input)
	if err != nil {
		return PurgeResult{}, err
	}
	for _, key := range plan.StorageKeys {
		if err = s.files.Delete(ctx, key); err != nil {
			_ = repository.FailPurge(context.WithoutCancel(ctx), plan, err)
			return PurgeResult{}, fmt.Errorf("delete document object: %w", err)
		}
	}
	if err = repository.CompletePurge(ctx, plan); err != nil {
		_ = repository.FailPurge(context.WithoutCancel(ctx), plan, err)
		return PurgeResult{}, err
	}
	return PurgeResult{AuditID: plan.AuditID, DocumentSetID: id, Status: "COMPLETED",
		DeletedObjects: len(plan.StorageKeys)}, nil
}

func (s *Service) AIBudgetStatus(ctx context.Context, id int64) (AIBudgetStatus, error) {
	if id <= 0 {
		return AIBudgetStatus{}, ErrNotFound
	}
	if s.budget == nil {
		return AIBudgetStatus{}, ErrUnsupported
	}
	if _, err := s.repository.GetSet(ctx, id); err != nil {
		return AIBudgetStatus{}, err
	}
	status, err := s.budget.Status(ctx, id)
	if err != nil {
		return AIBudgetStatus{}, err
	}
	return AIBudgetStatus{DocumentSetID: status.DocumentSetID, TokenBudget: status.TokenBudget,
		UsedTokens: status.UsedTokens, ReservedTokens: status.ReservedTokens,
		RemainingTokens: status.RemainingTokens, CostBudgetMicroUSD: status.CostBudgetMicroUSD,
		UsedCostMicroUSD: status.UsedCostMicroUSD, ReservedCostMicroUSD: status.ReservedCostMicroUSD,
		RemainingCostMicroUSD: status.RemainingCostMicroUSD}, nil
}

func (s *Service) Upload(ctx context.Context, setID int64, input UploadInput,
	source io.Reader,
) (Document, Version, error) {
	if setID <= 0 {
		return Document{}, Version{}, ErrNotFound
	}
	if _, err := s.repository.GetSet(ctx, setID); err != nil {
		return Document{}, Version{}, err
	}
	input.Filename = strings.TrimSpace(input.Filename)
	if input.Filename == "" || filepath.Base(input.Filename) != input.Filename ||
		utf8.RuneCountInString(input.Filename) > maxFilenameRunes ||
		strings.ContainsRune(input.Filename, '\x00') {
		return Document{}, Version{}, fmt.Errorf("%w: filename is invalid", ErrInvalidInput)
	}
	mediaType, err := supportedMediaType(input.Filename)
	if err != nil {
		return Document{}, Version{}, err
	}
	input.MediaType = mediaType
	input.DocumentName = strings.TrimSpace(input.DocumentName)
	if input.DocumentName == "" {
		input.DocumentName = strings.TrimSuffix(input.Filename, filepath.Ext(input.Filename))
	}
	if input.DocumentName == "" || utf8.RuneCountInString(input.DocumentName) > maxDocumentNameRunes ||
		strings.ContainsRune(input.DocumentName, '\x00') {
		return Document{}, Version{}, fmt.Errorf("%w: document_name must contain 1-%d characters",
			ErrInvalidInput, maxDocumentNameRunes)
	}
	input.DocumentType = strings.ToUpper(strings.TrimSpace(input.DocumentType))
	if input.DocumentType == "" {
		input.DocumentType = TypeRequirements
	}
	if !ValidDocumentType(input.DocumentType) {
		return Document{}, Version{}, fmt.Errorf("%w: unsupported document_type", ErrInvalidInput)
	}
	input.ApprovalStatus = strings.ToUpper(strings.TrimSpace(input.ApprovalStatus))
	if input.ApprovalStatus == "" {
		input.ApprovalStatus = ApprovalDraft
	}
	if input.ApprovalStatus != ApprovalDraft {
		return Document{}, Version{}, fmt.Errorf("%w: uploaded documents must start as DRAFT", ErrInvalidInput)
	}

	stored, err := s.files.Save(ctx, source)
	if err != nil {
		return Document{}, Version{}, err
	}
	documentItem, version, err := s.repository.CreateVersion(ctx, setID, input, stored)
	if err != nil {
		_ = s.files.Delete(context.WithoutCancel(ctx), stored.StorageKey)
		return Document{}, Version{}, err
	}
	return documentItem, version, nil
}

func (s *Service) ListDocuments(ctx context.Context, setID int64) ([]Document, error) {
	if setID <= 0 {
		return nil, ErrNotFound
	}
	return s.repository.ListDocuments(ctx, setID)
}

func (s *Service) GetVersion(ctx context.Context, documentID int64, versionNumber int) (Document, Version, []Block, error) {
	if documentID <= 0 || versionNumber <= 0 {
		return Document{}, Version{}, nil, ErrNotFound
	}
	documentItem, version, err := s.repository.GetVersion(ctx, documentID, versionNumber)
	if err != nil {
		return Document{}, Version{}, nil, err
	}
	blocks, err := s.repository.ListBlocks(ctx, version.ID)
	if err != nil {
		return Document{}, Version{}, nil, err
	}
	return documentItem, version, blocks, nil
}

func supportedMediaType(filename string) (string, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".md", ".markdown":
		return MediaTypeMarkdown, nil
	case ".docx":
		return MediaTypeDOCX, nil
	default:
		return "", fmt.Errorf("%w: only DOCX and Markdown are accepted", ErrUnsupported)
	}
}
