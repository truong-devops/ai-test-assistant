package automation

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	FrameworkGoTest = "GO_TEST"
	StatusDraft     = "DRAFT"
	StatusApproved  = "APPROVED"
	StatusRejected  = "REJECTED"

	RepairPending       = "PENDING"
	RepairRunning       = "RUNNING"
	RepairWaitingReview = "WAITING_REVIEW"
	RepairApproved      = "APPROVED"
	RepairRejected      = "REJECTED"
	RepairUnrepairable  = "UNREPAIRABLE"
	RepairFailed        = "FAILED"
	RepairPromptVersion = "document-automation-repair-v1"
)

var (
	ErrInvalidInput      = errors.New("invalid automation input")
	ErrNotFound          = errors.New("automation subject not found")
	ErrBlocked           = errors.New("automation generation blocked")
	ErrInvalidOutput     = errors.New("invalid automation provider output")
	ErrAlreadyReviewed   = errors.New("automation artifact already reviewed")
	ErrRepairNotEligible = errors.New("only AUTOMATION_ERROR is eligible for technical repair")
	ErrRepairLimit       = errors.New("automation repair attempt limit reached")
	ErrRepairLeaseLost   = errors.New("automation repair lease lost")
)

// Artifact is a technical implementation of an approved, versioned test case.
// ExpectedResultHash is copied from that business specification and must remain
// stable across generation and repair.
type Artifact struct {
	ID                   int64           `json:"id"`
	TestCaseID           int64           `json:"test_case_id"`
	VersionNumber        int             `json:"version_number"`
	Framework            string          `json:"framework"`
	FilePath             string          `json:"file_path"`
	Source               string          `json:"source"`
	SourceHash           string          `json:"source_hash"`
	ExpectedResultHash   string          `json:"expected_result_hash"`
	Status               string          `json:"status"`
	ModelName            string          `json:"model_name"`
	PromptVersion        string          `json:"prompt_version"`
	ProviderResponseID   string          `json:"provider_response_id,omitempty"`
	AnalysisJobID        *int64          `json:"analysis_job_id,omitempty"`
	Setup                string          `json:"setup"`
	Assertions           []string        `json:"assertions"`
	TestCaseSnapshot     json.RawMessage `json:"test_case_snapshot"`
	BusinessContext      json.RawMessage `json:"business_context"`
	TechnicalContext     json.RawMessage `json:"technical_context"`
	BusinessContextHash  string          `json:"business_context_hash"`
	TechnicalContextHash string          `json:"technical_context_hash"`
	CreatedAt            time.Time       `json:"created_at"`
}

type GenerateInput struct {
	TestCaseID int64 `json:"test_case_id"`
}

type GenerationResult struct {
	Status   string    `json:"status"`
	Message  string    `json:"message,omitempty"`
	Artifact *Artifact `json:"artifact,omitempty"`
}

type ReviewInput struct {
	Decision     string `json:"decision"`
	ReviewerName string `json:"reviewer_name"`
	Comment      string `json:"comment"`
}

type Review struct {
	ID                   int64     `json:"id"`
	AutomationArtifactID int64     `json:"automation_artifact_id"`
	ReviewerName         string    `json:"reviewer_name"`
	Decision             string    `json:"decision"`
	Comment              string    `json:"comment"`
	CreatedAt            time.Time `json:"created_at"`
}

type ArtifactHistory struct {
	Artifacts []Artifact `json:"artifacts"`
	Reviews   []Review   `json:"reviews"`
}

// RepairJob is a candidate-level technical repair. Its expected-result hash,
// original assertions and change policy are immutable database guardrails.
type RepairJob struct {
	ID                    int64           `json:"id"`
	TestRunItemID         int64           `json:"test_run_item_id"`
	SourceArtifactID      int64           `json:"source_artifact_id"`
	RepairedArtifactID    *int64          `json:"repaired_artifact_id,omitempty"`
	AttemptNumber         int             `json:"attempt_number"`
	Status                string          `json:"status"`
	ErrorType             string          `json:"error_type"`
	Reason                string          `json:"reason"`
	RequestedBy           string          `json:"requested_by"`
	AllowedChangePolicy   json.RawMessage `json:"allowed_change_policy"`
	ExpectedResultHash    string          `json:"expected_result_hash"`
	BeforeSourceHash      string          `json:"before_source_hash"`
	AfterSourceHash       string          `json:"after_source_hash,omitempty"`
	BeforeAssertions      []string        `json:"before_assertions"`
	AfterAssertions       []string        `json:"after_assertions,omitempty"`
	ModelName             string          `json:"model_name,omitempty"`
	PromptVersion         string          `json:"prompt_version"`
	ProviderResponseID    string          `json:"provider_response_id,omitempty"`
	InputTokens           int             `json:"input_tokens"`
	OutputTokens          int             `json:"output_tokens"`
	MaxOutputTokens       int             `json:"max_output_tokens"`
	MaxCostMicroUSD       int64           `json:"max_cost_microusd"`
	EstimatedCostMicroUSD int64           `json:"estimated_cost_microusd"`
	QueueAttemptCount     int             `json:"queue_attempt_count"`
	ErrorMessage          string          `json:"error_message,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	StartedAt             *time.Time      `json:"started_at,omitempty"`
	FinishedAt            *time.Time      `json:"finished_at,omitempty"`
}

type RepairRequest struct {
	RequestedBy string `json:"requested_by"`
}

type RepairSubject struct {
	Job          RepairJob
	Artifact     Artifact
	ActualResult string
	Evidence     json.RawMessage
	TestCaseID   int64
	AnalysisID   int64
	PackageName  string
}

type proposedArtifact struct {
	Framework          string   `json:"framework"`
	TargetFile         string   `json:"target_file"`
	PackageName        string   `json:"package_name"`
	Setup              string   `json:"setup"`
	Assertions         []string `json:"assertions"`
	ExpectedResultHash string   `json:"expected_result_hash"`
	TestCaseIDs        []int64  `json:"test_case_ids"`
	Code               string   `json:"code"`
}
