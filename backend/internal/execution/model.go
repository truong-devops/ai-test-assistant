package execution

import (
	"errors"
	"time"
)

const (
	RunPending   = "PENDING"
	RunRunning   = "RUNNING"
	RunCompleted = "COMPLETED"
	RunFailed    = "FAILED"

	StatusNotRun          = "NOT_RUN"
	StatusPassed          = "PASSED"
	StatusProductFailed   = "PRODUCT_FAILED"
	StatusAutomationError = "AUTOMATION_ERROR"
	StatusInfraError      = "INFRA_ERROR"
	StatusTimedOut        = "TIMED_OUT"
	StatusBlocked         = "BLOCKED"
)

var (
	ErrNotFound       = errors.New("test run not found")
	ErrInvalidInput   = errors.New("invalid test run input")
	ErrNotRequestable = errors.New("test run cannot be requested in its current state")
	ErrLeaseLost      = errors.New("test run lease lost")
)

type Run struct {
	ID                     int64                  `json:"id"`
	TestSuiteID            int64                  `json:"test_suite_id"`
	ProjectID              *int64                 `json:"project_id,omitempty"`
	AnalysisJobID          *int64                 `json:"analysis_job_id,omitempty"`
	SourceSHA              string                 `json:"source_sha"`
	TargetSHA              string                 `json:"target_sha"`
	Environment            string                 `json:"environment"`
	EnvironmentFingerprint string                 `json:"environment_fingerprint"`
	ImageReference         string                 `json:"image_reference"`
	ImageDigest            string                 `json:"image_digest"`
	Status                 string                 `json:"status"`
	ExecutionRequestedAt   *time.Time             `json:"execution_requested_at,omitempty"`
	ExecutionRequestedBy   string                 `json:"execution_requested_by"`
	AttemptCount           int                    `json:"attempt_count"`
	ErrorMessage           string                 `json:"error_message,omitempty"`
	RequestedAt            time.Time              `json:"requested_at"`
	StartedAt              *time.Time             `json:"started_at,omitempty"`
	FinishedAt             *time.Time             `json:"finished_at,omitempty"`
	Items                  []Item                 `json:"items"`
	Reviews                []ClassificationReview `json:"classification_reviews"`
}

type Artifact struct {
	ID                 int64
	FilePath           string
	Source             string
	SourceHash         string
	ExpectedResultHash string
	Status             string
}

type Item struct {
	ID                   int64      `json:"id"`
	TestRunID            int64      `json:"test_run_id"`
	TestCaseID           int64      `json:"test_case_id"`
	TestCaseKey          string     `json:"test_case_key"`
	Title                string     `json:"title"`
	ExpectedResult       string     `json:"expected_result"`
	ExpectedResultHash   string     `json:"expected_result_hash"`
	AutomationArtifactID *int64     `json:"automation_artifact_id,omitempty"`
	AutomationSourceHash string     `json:"automation_source_hash"`
	AttemptNumber        int        `json:"attempt_number"`
	Status               string     `json:"status"`
	ActualResult         string     `json:"actual_result"`
	Command              string     `json:"command"`
	ExitCode             *int       `json:"exit_code,omitempty"`
	DurationMS           int64      `json:"duration_ms"`
	OutputTruncated      bool       `json:"output_truncated"`
	CreatedAt            time.Time  `json:"created_at"`
	Artifact             *Artifact  `json:"-"`
	Evidence             []Evidence `json:"evidence"`
}

type Evidence struct {
	ID           int64     `json:"id"`
	EvidenceType string    `json:"evidence_type"`
	Content      string    `json:"content,omitempty"`
	StorageKey   string    `json:"storage_key,omitempty"`
	ContentHash  string    `json:"content_hash"`
	CreatedAt    time.Time `json:"created_at"`
}

type RequestInput struct {
	RequestedBy string `json:"requested_by"`
}

type ClassificationInput struct {
	Status       string `json:"status"`
	ReviewerName string `json:"reviewer_name"`
	Reason       string `json:"reason"`
}

type ClassificationReview struct {
	ID             int64     `json:"id"`
	TestRunItemID  int64     `json:"test_run_item_id"`
	ReviewerName   string    `json:"reviewer_name"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	Reason         string    `json:"reason"`
	CreatedAt      time.Time `json:"created_at"`
}

type Outcome struct {
	ItemID               int64
	ArtifactID           *int64
	AutomationSourceHash string
	Status               string
	ActualResult         string
	Command              string
	ExitCode             *int
	DurationMS           int64
	OutputTruncated      bool
	Evidence             []Evidence
}
