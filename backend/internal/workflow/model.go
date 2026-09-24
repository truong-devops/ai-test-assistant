package workflow

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	OperationIndex    = "INDEX_DOCUMENTS"
	OperationExtract  = "EXTRACT_REQUIREMENTS"
	OperationGenerate = "GENERATE_TESTCASES"

	StatusQueued        = "QUEUED"
	StatusRunning       = "RUNNING"
	StatusSucceeded     = "SUCCEEDED"
	StatusPartialFailed = "PARTIAL_FAILED"
	StatusFailed        = "FAILED"
	StatusCanceled      = "CANCELED"

	UnitQueued    = "QUEUED"
	UnitRunning   = "RUNNING"
	UnitSucceeded = "SUCCEEDED"
	UnitFailed    = "FAILED"
	UnitCanceled  = "CANCELED"
)

var (
	ErrNotFound            = errors.New("document workflow job not found")
	ErrInvalidInput        = errors.New("invalid document workflow input")
	ErrIdempotencyConflict = errors.New("workflow idempotency key was reused with different input")
	ErrOperationActive     = errors.New("document workflow operation is already active")
	ErrRevisionConflict    = errors.New("document workflow job revision changed")
	ErrNotRetryable        = errors.New("document workflow job is not retryable")
	ErrNotCancelable       = errors.New("document workflow job is not cancelable")
	ErrLeaseLost           = errors.New("document workflow job lease lost")
	ErrCanceled            = errors.New("document workflow job was canceled")
	ErrInputStale          = errors.New("workflow input snapshot is no longer current")
	ErrForbidden           = errors.New("workflow action is not allowed for this role")
)

type Job struct {
	ID              int64           `json:"id"`
	DocumentSetID   int64           `json:"document_set_id"`
	Operation       string          `json:"operation"`
	Status          string          `json:"status"`
	Revision        int             `json:"revision"`
	InputSnapshot   json.RawMessage `json:"input_snapshot"`
	InputHash       string          `json:"input_hash"`
	RequestedBy     string          `json:"requested_by"`
	IdempotencyKey  string          `json:"idempotency_key"`
	TotalUnits      int             `json:"total_units"`
	CompletedUnits  int             `json:"completed_units"`
	FailedUnits     int             `json:"failed_units"`
	AttemptCount    int             `json:"attempt_count"`
	MaxAttempts     int             `json:"max_attempts"`
	NextAttemptAt   time.Time       `json:"next_attempt_at"`
	LeaseExpiresAt  *time.Time      `json:"lease_expires_at,omitempty"`
	HeartbeatAt     *time.Time      `json:"heartbeat_at,omitempty"`
	CancelRequested *time.Time      `json:"cancel_requested_at,omitempty"`
	ErrorCode       string          `json:"error_code,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	Retryable       bool            `json:"retryable"`
	OutputRefs      json.RawMessage `json:"output_refs"`
	DelegateKind    string          `json:"delegate_kind,omitempty"`
	DelegateJobID   *int64          `json:"delegate_job_id,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Units           []Unit          `json:"units"`
	Usage           Usage           `json:"usage"`
}

type Unit struct {
	ID            int64           `json:"id"`
	WorkflowJobID int64           `json:"workflow_job_id"`
	UnitKey       string          `json:"unit_key"`
	InputHash     string          `json:"input_hash"`
	Status        string          `json:"status"`
	AttemptCount  int             `json:"attempt_count"`
	OutputRef     json.RawMessage `json:"output_ref"`
	ErrorCode     string          `json:"error_code,omitempty"`
	ErrorMessage  string          `json:"error_message,omitempty"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type Usage struct {
	ReservedTokens       int64 `json:"reserved_tokens"`
	InputTokens          int64 `json:"input_tokens"`
	OutputTokens         int64 `json:"output_tokens"`
	ReservedCostMicroUSD int64 `json:"reserved_cost_microusd"`
	ActualCostMicroUSD   int64 `json:"actual_cost_microusd"`
}

type OperationInput struct {
	GenerationScope    string  `json:"generation_scope,omitempty"`
	ReviewProposals    bool    `json:"review_proposals,omitempty"`
	Operation          string  `json:"operation"`
	RequirementIDs     []int64 `json:"requirement_ids,omitempty"`
	RequestedBy        string  `json:"requested_by,omitempty"`
	ExcludedVersionIDs []int64 `json:"excluded_version_ids,omitempty"`
}

type MutationInput struct {
	ExpectedRevision int `json:"expected_revision"`
}

type BlockingReason struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Step       string `json:"step"`
	NextAction string `json:"next_action,omitempty"`
}

type Capabilities struct {
	CanUpload       bool `json:"can_upload"`
	CanManage       bool `json:"can_manage"`
	CanIndex        bool `json:"can_index"`
	CanExtract      bool `json:"can_extract"`
	CanGenerate     bool `json:"can_generate"`
	CanReview       bool `json:"can_review"`
	CanPublish      bool `json:"can_publish"`
	CanRetryJob     bool `json:"can_retry_job"`
	CanCancelJob    bool `json:"can_cancel_job"`
	CanAdjustBudget bool `json:"can_adjust_budget"`
}

type Step struct {
	Key             string           `json:"key"`
	State           string           `json:"state"`
	CompletedUnits  int              `json:"completed_units"`
	TotalUnits      int              `json:"total_units"`
	BlockingReasons []BlockingReason `json:"blocking_reasons"`
}

type ReadModel struct {
	GenerationScope GenerationScope  `json:"generation_scope"`
	SourceIntents   []SourceIntent   `json:"source_intents"`
	DocumentSetID   int64            `json:"document_set_id"`
	SourceRevision  int64            `json:"source_revision"`
	Steps           []Step           `json:"steps"`
	Capabilities    Capabilities     `json:"capabilities"`
	BlockingReasons []BlockingReason `json:"blocking_reasons"`
	ActiveJobs      []Job            `json:"active_jobs"`
	RecentJobs      []Job            `json:"recent_jobs"`
	NextAction      string           `json:"next_action"`
}

type BlockedError struct {
	Code       string
	Message    string
	BlockedBy  []BlockingReason
	NextAction string
	Details    any
}

func (e *BlockedError) Error() string { return e.Message }

func Terminal(status string) bool {
	return status == StatusSucceeded || status == StatusPartialFailed ||
		status == StatusFailed || status == StatusCanceled
}
