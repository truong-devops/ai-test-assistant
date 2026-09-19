package scope

import (
	"errors"
	"time"
)

const (
	ModeFullApproved   = "FULL_APPROVED"
	ModeMappedFallback = "MAPPED_WITH_FULL_FALLBACK"
	ModeExplicitTrace  = "EXPLICIT_TRACE"
	ModeFullFallback   = "FULL_BASELINE_FALLBACK"
)

var (
	ErrInvalidInput = errors.New("invalid test scope input")
	ErrNotFound     = errors.New("test scope not found")
	ErrUnsafeChange = errors.New("test scope item already executed and cannot be removed")
)

type Candidate struct {
	DocumentSetID      int64     `json:"document_set_id"`
	DocumentSetName    string    `json:"document_set_name"`
	TestSuiteID        int64     `json:"test_suite_id"`
	TestSuiteName      string    `json:"test_suite_name"`
	SuiteReleaseID     int64     `json:"suite_release_id"`
	ReleaseNumber      int       `json:"release_number"`
	ReleaseName        string    `json:"release_name"`
	ManifestHash       string    `json:"manifest_hash"`
	ReleasePublishedAt time.Time `json:"release_published_at"`
	ApprovedTestCases  int       `json:"approved_test_cases"`
}

type Baseline struct {
	ProjectID          int64     `json:"project_id"`
	DocumentSetID      int64     `json:"document_set_id"`
	DocumentSetName    string    `json:"document_set_name"`
	TestSuiteID        int64     `json:"test_suite_id"`
	TestSuiteName      string    `json:"test_suite_name"`
	SuiteReleaseID     int64     `json:"suite_release_id"`
	ReleaseNumber      int       `json:"release_number"`
	ReleaseName        string    `json:"release_name"`
	ManifestHash       string    `json:"manifest_hash"`
	ReleasePublishedAt time.Time `json:"release_published_at"`
	SelectionMode      string    `json:"selection_mode"`
	SelectedBy         string    `json:"selected_by"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type BaselineView struct {
	Bound      bool        `json:"bound"`
	Baseline   *Baseline   `json:"baseline,omitempty"`
	Candidates []Candidate `json:"candidates"`
}

type SelectInput struct {
	DocumentSetID  int64  `json:"document_set_id"`
	TestSuiteID    int64  `json:"test_suite_id"`
	SuiteReleaseID int64  `json:"suite_release_id"`
	SelectionMode  string `json:"selection_mode"`
	SelectedBy     string `json:"selected_by"`
}

type Item struct {
	ID                 int64     `json:"id"`
	TestCaseID         int64     `json:"test_case_id"`
	TestCaseKey        string    `json:"test_case_key"`
	Title              string    `json:"title"`
	TestType           string    `json:"test_type"`
	Risk               string    `json:"risk"`
	ExpectedResult     string    `json:"expected_result"`
	ExpectedResultHash string    `json:"expected_result_hash"`
	AutomationStatus   string    `json:"automation_status"`
	Included           bool      `json:"included"`
	SelectionReason    string    `json:"selection_reason"`
	Confidence         float64   `json:"confidence"`
	Explanation        string    `json:"explanation"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Decision struct {
	ID           int64     `json:"id"`
	ScopeItemID  int64     `json:"scope_item_id"`
	ReviewerName string    `json:"reviewer_name"`
	Included     bool      `json:"included"`
	Comment      string    `json:"comment"`
	CreatedAt    time.Time `json:"created_at"`
}

type Signal struct {
	ID          int64     `json:"id"`
	SignalType  string    `json:"signal_type"`
	SignalValue string    `json:"signal_value"`
	Confidence  float64   `json:"confidence"`
	Explanation string    `json:"explanation"`
	CreatedAt   time.Time `json:"created_at"`
}

type Bundle struct {
	AnalysisJobID       int64      `json:"analysis_job_id"`
	ProjectID           int64      `json:"project_id"`
	DocumentSetID       int64      `json:"document_set_id"`
	TestSuiteID         int64      `json:"test_suite_id"`
	SuiteReleaseID      int64      `json:"suite_release_id"`
	TestRunID           int64      `json:"test_run_id"`
	BaselineHash        string     `json:"baseline_hash"`
	ExplicitIdentifiers []string   `json:"explicit_identifiers"`
	SelectionMode       string     `json:"selection_mode"`
	MappingConfidence   float64    `json:"mapping_confidence"`
	Warning             string     `json:"warning,omitempty"`
	Items               []Item     `json:"items"`
	Decisions           []Decision `json:"decisions"`
	Signals             []Signal   `json:"signals"`
	CreatedAt           time.Time  `json:"created_at"`
}

type ManualInput struct {
	TestCaseID   int64  `json:"test_case_id"`
	Included     bool   `json:"included"`
	ReviewerName string `json:"reviewer_name"`
	Comment      string `json:"comment"`
}
