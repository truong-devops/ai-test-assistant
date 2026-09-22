package requirement

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	TypeFunctional          = "FUNCTIONAL"
	TypeNonFunctional       = "NON_FUNCTIONAL"
	TypeBusinessRule        = "BUSINESS_RULE"
	TypeAcceptanceCriterion = "ACCEPTANCE_CRITERION"
	TypeUseCase             = "USE_CASE"
	TypeRegression          = "REGRESSION"

	StatusDraft    = "DRAFT"
	StatusApproved = "APPROVED"
	StatusRejected = "REJECTED"
	StatusConflict = "CONFLICT"
	StatusTBD      = "TBD"

	DecisionApproved = "APPROVED"
	DecisionRejected = "REJECTED"

	ExtractionPending   = "PENDING"
	ExtractionRunning   = "RUNNING"
	ExtractionCompleted = "COMPLETED"
	ExtractionFailed    = "FAILED"
)

var (
	ErrNotFound            = errors.New("requirement not found")
	ErrInvalidInput        = errors.New("invalid requirement input")
	ErrNoIndex             = errors.New("document index must be ready before requirement extraction")
	ErrMissingEvidence     = errors.New("requirement evidence is required")
	ErrReviewBlocked       = errors.New("requirement cannot be approved")
	ErrRevisionConflict    = errors.New("requirement revision or review hash changed")
	ErrIdempotencyConflict = errors.New("requirement review idempotency key reused")
	ErrExtractionBusy      = errors.New("requirement extraction is already running")
	ErrLeaseLost           = errors.New("requirement extraction lease lost")
	ErrStaleIndex          = errors.New("document index changed after extraction was queued")
	ErrSourceNotApproved   = errors.New("all included document versions must be approved before requirement extraction")
)

type ExtractionJob struct {
	ID                int64      `json:"id"`
	DocumentSetID     int64      `json:"document_set_id"`
	IndexGeneration   int64      `json:"index_generation"`
	SourceSnapshotID  *int64     `json:"source_snapshot_id,omitempty"`
	SourceRevision    int64      `json:"source_revision"`
	IsCurrent         bool       `json:"is_current"`
	Status            string     `json:"status"`
	TotalChunks       int        `json:"total_chunks"`
	ProcessedChunks   int        `json:"processed_chunks"`
	CreatedCount      int        `json:"created_count"`
	ReusedCount       int        `json:"reused_count"`
	ConflictCount     int        `json:"conflict_count"`
	OpenQuestionCount int        `json:"open_question_count"`
	RequestedBy       string     `json:"requested_by"`
	AttemptCount      int        `json:"attempt_count"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	WorkflowJobID     *int64     `json:"workflow_job_id,omitempty"`
	WorkflowUnitID    *int64     `json:"workflow_unit_id,omitempty"`
}

type Requirement struct {
	ReviewHash        string          `json:"review_hash"`
	ReviewBlockers    []string        `json:"review_blockers"`
	SourceState       string          `json:"source_state"`
	ID                int64           `json:"id"`
	DocumentSetID     int64           `json:"document_set_id"`
	RequirementKey    string          `json:"requirement_key"`
	VersionNumber     int             `json:"version_number"`
	Title             string          `json:"title"`
	Statement         string          `json:"statement"`
	RequirementType   string          `json:"requirement_type"`
	FlowType          string          `json:"flow_type"`
	Actor             string          `json:"actor"`
	Precondition      string          `json:"precondition"`
	Postcondition     string          `json:"postcondition"`
	Priority          string          `json:"priority"`
	Risk              string          `json:"risk"`
	Status            string          `json:"status"`
	Confidence        float64         `json:"confidence"`
	SupersedesID      *int64          `json:"supersedes_requirement_id,omitempty"`
	Assumptions       json.RawMessage `json:"assumptions"`
	ExtractionKey     string          `json:"extraction_key,omitempty"`
	SourceFingerprint string          `json:"source_fingerprint,omitempty"`
	SourceSnapshotID  *int64          `json:"source_snapshot_id,omitempty"`
	DocumentIDs       []int64         `json:"document_ids,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type Evidence struct {
	ID                int64     `json:"id"`
	RequirementID     int64     `json:"requirement_id"`
	DocumentSetID     int64     `json:"document_set_id"`
	DocumentVersionID int64     `json:"document_version_id"`
	DocumentBlockID   int64     `json:"document_block_id"`
	DocumentName      string    `json:"document_name,omitempty"`
	VersionNumber     int       `json:"version_number,omitempty"`
	ApprovalStatus    string    `json:"approval_status,omitempty"`
	SourceLocator     string    `json:"source_locator"`
	Excerpt           string    `json:"excerpt,omitempty"`
	ExcerptHash       string    `json:"excerpt_hash"`
	CreatedAt         time.Time `json:"created_at"`
}

type FlowStep struct {
	ID             int64     `json:"id"`
	RequirementID  int64     `json:"requirement_id"`
	Ordinal        int       `json:"ordinal"`
	Action         string    `json:"action"`
	ExpectedResult string    `json:"expected_result"`
	CreatedAt      time.Time `json:"created_at"`
}

type Conflict struct {
	ID                 int64        `json:"id"`
	DocumentSetID      int64        `json:"document_set_id"`
	LeftRequirementID  int64        `json:"left_requirement_id"`
	RightRequirementID int64        `json:"right_requirement_id"`
	Reason             string       `json:"reason"`
	Status             string       `json:"status"`
	Resolution         string       `json:"resolution"`
	CreatedAt          time.Time    `json:"created_at"`
	ResolvedAt         *time.Time   `json:"resolved_at,omitempty"`
	Left               *Requirement `json:"left,omitempty"`
	Right              *Requirement `json:"right,omitempty"`
}

type OpenQuestion struct {
	ID            int64      `json:"id"`
	DocumentSetID int64      `json:"document_set_id"`
	RequirementID *int64     `json:"requirement_id,omitempty"`
	Question      string     `json:"question"`
	OwnerRole     string     `json:"owner_role"`
	Status        string     `json:"status"`
	Answer        string     `json:"answer"`
	CreatedAt     time.Time  `json:"created_at"`
	AnsweredAt    *time.Time `json:"answered_at,omitempty"`
}

type Review struct {
	ID            int64     `json:"id"`
	RequirementID int64     `json:"requirement_id"`
	ReviewerName  string    `json:"reviewer_name"`
	Decision      string    `json:"decision"`
	Comment       string    `json:"comment"`
	CreatedAt     time.Time `json:"created_at"`
}

type Detail struct {
	Requirement Requirement `json:"requirement"`
	Evidence    []Evidence  `json:"evidence"`
	FlowSteps   []FlowStep  `json:"flow_steps"`
	Reviews     []Review    `json:"reviews"`
}

type Filter struct {
	DocumentSetID   int64
	DocumentID      int64
	RequirementType string
	Status          string
	Risk            string
	Actor           string
	FlowType        string
}

type ReviewInput struct {
	ExpectedHash  string `json:"expected_hash,omitempty"`
	CommandKey    string `json:"-"`
	DocumentSetID int64  `json:"-"`
	ReviewerName  string `json:"reviewer_name"`
	Decision      string `json:"decision"`
	Comment       string `json:"comment"`
	Title         string `json:"title,omitempty"`
	Statement     string `json:"statement,omitempty"`
	Actor         string `json:"actor,omitempty"`
	Precondition  string `json:"precondition,omitempty"`
	Postcondition string `json:"postcondition,omitempty"`
	Priority      string `json:"priority,omitempty"`
	Risk          string `json:"risk,omitempty"`
}

type ExtractionSummary struct {
	DocumentSetID     int64  `json:"document_set_id"`
	SourceSnapshotID  *int64 `json:"source_snapshot_id,omitempty"`
	IndexGeneration   int64  `json:"index_generation"`
	ChunkCount        int    `json:"chunk_count"`
	ProcessedChunks   int    `json:"processed_chunks"`
	CreatedCount      int    `json:"created_count"`
	ReusedCount       int    `json:"reused_count"`
	ConflictCount     int    `json:"conflict_count"`
	OpenQuestionCount int    `json:"open_question_count"`
}

type Proposal struct {
	Identifier      string     `json:"identifier"`
	Title           string     `json:"title"`
	Statement       string     `json:"statement"`
	RequirementType string     `json:"requirement_type"`
	FlowType        string     `json:"flow_type"`
	Actor           string     `json:"actor"`
	Precondition    string     `json:"precondition"`
	Postcondition   string     `json:"postcondition"`
	Priority        string     `json:"priority"`
	Risk            string     `json:"risk"`
	Status          string     `json:"status"`
	Confidence      float64    `json:"confidence"`
	Assumptions     []string   `json:"assumptions"`
	Steps           []FlowStep `json:"steps"`
}

type AICall struct {
	DocumentSetID      int64
	Phase              string
	SubjectType        string
	SubjectKey         string
	ContextSnapshotID  *int64
	Provider           string
	ModelName          string
	PromptVersion      string
	Instructions       string
	PromptText         string
	RequestSchema      json.RawMessage
	ResponseText       string
	ProviderResponseID string
	Status             string
	ErrorMessage       string
	InputTokens        int
	OutputTokens       int
	LatencyMS          int64
}
