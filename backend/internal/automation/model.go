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
)

var (
	ErrInvalidInput    = errors.New("invalid automation input")
	ErrNotFound        = errors.New("automation subject not found")
	ErrBlocked         = errors.New("automation generation blocked")
	ErrInvalidOutput   = errors.New("invalid automation provider output")
	ErrAlreadyReviewed = errors.New("automation artifact already reviewed")
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
