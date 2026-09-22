package document

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	SetStatusActive   = "ACTIVE"
	SetStatusArchived = "ARCHIVED"
	SetStatusPurging  = "PURGING"

	TypeRequirements   = "REQUIREMENTS"
	TypeUserStory      = "USER_STORY"
	TypeSystemDesign   = "SYSTEM_DESIGN"
	TypeDatabaseDesign = "DATABASE_DESIGN"
	TypeAPIContract    = "API_CONTRACT"
	TypeBugHistory     = "BUG_HISTORY"
	TypeTestReference  = "TEST_REFERENCE"
	TypeOther          = "OTHER"

	ApprovalDraft    = "DRAFT"
	ApprovalApproved = "APPROVED"
	ApprovalRejected = "REJECTED"

	ParseUploaded = "UPLOADED"
	ParseParsing  = "PARSING"
	ParseParsed   = "PARSED"
	ParseFailed   = "FAILED"

	BlockHeading   = "HEADING"
	BlockParagraph = "PARAGRAPH"
	BlockList      = "LIST"
	BlockTable     = "TABLE"
	BlockCode      = "CODE"

	MediaTypeMarkdown = "text/markdown"
	MediaTypeDOCX     = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

	DefaultMaxUploadBytes int64 = 16 << 20
)

var (
	ErrNotFound        = errors.New("document resource not found")
	ErrInvalidInput    = errors.New("invalid document input")
	ErrAlreadyExists   = errors.New("document resource already exists")
	ErrUnsupported     = errors.New("unsupported document format")
	ErrFileTooLarge    = errors.New("document file is too large")
	ErrUnsafeDocument  = errors.New("unsafe document archive")
	ErrLeaseLost       = errors.New("document parse lease lost")
	ErrRetentionNotMet = errors.New("document retention period has not elapsed")
	ErrPurgeBlocked    = errors.New("document purge is blocked by immutable references")
)

type Set struct {
	ID                   int64      `json:"id"`
	Name                 string     `json:"name"`
	ProductName          string     `json:"product_name"`
	Scope                string     `json:"scope"`
	Description          string     `json:"description"`
	Status               string     `json:"status"`
	RetentionDays        int        `json:"retention_days"`
	AITokenBudget        int64      `json:"ai_token_budget"`
	AICostBudgetMicroUSD int64      `json:"ai_cost_budget_microusd"`
	ArchivedAt           *time.Time `json:"archived_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	SourceRevision       int64      `json:"source_revision"`
}

type LifecycleInput struct {
	Status               string `json:"status"`
	RetentionDays        int    `json:"retention_days"`
	AITokenBudget        int64  `json:"ai_token_budget"`
	AICostBudgetMicroUSD int64  `json:"ai_cost_budget_microusd"`
	Actor                string `json:"actor"`
	Reason               string `json:"reason"`
}

type AIBudgetStatus struct {
	DocumentSetID         int64 `json:"document_set_id"`
	TokenBudget           int64 `json:"token_budget"`
	UsedTokens            int64 `json:"used_tokens"`
	ReservedTokens        int64 `json:"reserved_tokens"`
	RemainingTokens       int64 `json:"remaining_tokens"`
	CostBudgetMicroUSD    int64 `json:"cost_budget_microusd"`
	UsedCostMicroUSD      int64 `json:"used_cost_microusd"`
	ReservedCostMicroUSD  int64 `json:"reserved_cost_microusd"`
	RemainingCostMicroUSD int64 `json:"remaining_cost_microusd"`
}

type PurgePreview struct {
	DocumentSetID      int64      `json:"document_set_id"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	RetentionDays      int        `json:"retention_days"`
	ArchivedAt         *time.Time `json:"archived_at,omitempty"`
	PurgeEligibleAt    *time.Time `json:"purge_eligible_at,omitempty"`
	Eligible           bool       `json:"eligible"`
	Blockers           []string   `json:"blockers"`
	StorageObjectCount int        `json:"storage_object_count"`
	StorageBytes       int64      `json:"storage_bytes"`
	Confirmation       string     `json:"confirmation"`
}

type PurgeInput struct {
	Actor        string `json:"actor"`
	Reason       string `json:"reason"`
	Confirmation string `json:"confirmation"`
}

type PurgePlan struct {
	AuditID       int64
	DocumentSetID int64
	StorageKeys   []string
}

type PurgeResult struct {
	AuditID        int64  `json:"audit_id"`
	DocumentSetID  int64  `json:"document_set_id"`
	Status         string `json:"status"`
	DeletedObjects int    `json:"deleted_objects"`
}

type Document struct {
	ID            int64     `json:"id"`
	DocumentSetID int64     `json:"document_set_id"`
	Name          string    `json:"name"`
	DocumentType  string    `json:"document_type"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LatestVersion *Version  `json:"latest_version,omitempty"`
}

type Version struct {
	ID               int64      `json:"id"`
	DocumentID       int64      `json:"document_id"`
	DocumentSetID    int64      `json:"document_set_id"`
	VersionNumber    int        `json:"version_number"`
	OriginalFilename string     `json:"original_filename"`
	MediaType        string     `json:"media_type"`
	SizeBytes        int64      `json:"size_bytes"`
	SHA256           string     `json:"sha256"`
	StorageKey       string     `json:"-"`
	ApprovalStatus   string     `json:"approval_status"`
	ParseStatus      string     `json:"parse_status"`
	ParseError       string     `json:"parse_error,omitempty"`
	BlockCount       int        `json:"block_count"`
	AttemptCount     int        `json:"attempt_count"`
	NextAttemptAt    time.Time  `json:"next_attempt_at"`
	LeaseExpiresAt   *time.Time `json:"lease_expires_at,omitempty"`
	UploadedAt       time.Time  `json:"uploaded_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	ParsedAt         *time.Time `json:"parsed_at,omitempty"`
}

type Block struct {
	ID                int64           `json:"id"`
	DocumentVersionID int64           `json:"document_version_id"`
	Ordinal           int             `json:"ordinal"`
	BlockType         string          `json:"block_type"`
	HeadingLevel      int             `json:"heading_level"`
	Content           string          `json:"content"`
	SourceLocator     string          `json:"source_locator"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         time.Time       `json:"created_at"`
}

type CreateSetInput struct {
	Name        string `json:"name"`
	ProductName string `json:"product_name"`
	Scope       string `json:"scope"`
	Description string `json:"description"`
}

type UploadInput struct {
	DocumentID     int64
	NewDocument    bool
	DocumentName   string
	DocumentType   string
	ApprovalStatus string
	Filename       string
	MediaType      string
}

type StoredFile struct {
	StorageKey string
	SizeBytes  int64
	SHA256     string
}

type ParsedBlock struct {
	BlockType     string
	HeadingLevel  int
	Content       string
	SourceLocator string
	Metadata      map[string]any
}

type PipelineMetrics struct {
	DocumentSets           int `json:"document_sets"`
	DocumentVersions       int `json:"document_versions"`
	ParsedVersions         int `json:"parsed_versions"`
	ParseFailures          int `json:"parse_failures"`
	ApprovedRequirements   int `json:"approved_requirements"`
	ExtractionJobs         int `json:"extraction_jobs"`
	ExtractionFailures     int `json:"extraction_failures"`
	TestSuites             int `json:"test_suites"`
	ApprovedTestCases      int `json:"approved_test_cases"`
	AutomationArtifacts    int `json:"automation_artifacts"`
	TestRuns               int `json:"test_runs"`
	RunsNeedingAttention   int `json:"runs_needing_attention"`
	PendingApprovalActions int `json:"pending_approval_actions"`
}

func ValidDocumentType(value string) bool {
	switch value {
	case TypeRequirements, TypeUserStory, TypeSystemDesign, TypeDatabaseDesign,
		TypeAPIContract, TypeBugHistory, TypeTestReference, TypeOther:
		return true
	default:
		return false
	}
}

func ValidApprovalStatus(value string) bool {
	return value == ApprovalDraft || value == ApprovalApproved || value == ApprovalRejected
}
