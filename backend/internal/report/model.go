package report

import (
	"errors"
	"time"
)

const (
	FormatXLSX     = "XLSX"
	FormatMarkdown = "MARKDOWN"
)

var (
	ErrInvalidInput = errors.New("invalid export input")
	ErrNotFound     = errors.New("export not found")
)

type ExportInput struct {
	TestSuiteID int64   `json:"test_suite_id"`
	TestRunID   *int64  `json:"test_run_id,omitempty"`
	Format      string  `json:"format"`
	GeneratedBy string  `json:"generated_by"`
	TestCaseIDs []int64 `json:"test_case_ids,omitempty"`
	SortBy      string  `json:"sort_by,omitempty"`
}

type DocumentVersionRef struct {
	DocumentName   string `json:"document_name"`
	VersionNumber  int    `json:"version_number"`
	SHA256         string `json:"sha256"`
	ApprovalStatus string `json:"approval_status"`
}

type ExecutionRow struct {
	TestCaseID       int64    `json:"test_case_id"`
	TestCaseKey      string   `json:"test_case_key"`
	RequirementTrace string   `json:"requirement_trace"`
	Objective        string   `json:"objective"`
	Preconditions    string   `json:"preconditions"`
	Steps            []string `json:"steps"`
	TestData         string   `json:"test_data"`
	Role             string   `json:"role"`
	ExpectedResult   string   `json:"expected_result"`
	Priority         string   `json:"priority"`
	Environment      string   `json:"environment"`
	ActualResult     string   `json:"actual_result"`
	Status           string   `json:"status"`
	Evidence         []string `json:"evidence"`
	Notes            string   `json:"notes"`
	AutomationStatus string   `json:"automation_status"`
	SourceStatus     string   `json:"source_status"`
}

type RunHistoryRow struct {
	TestCaseKey  string    `json:"test_case_key"`
	RunID        int64     `json:"run_id"`
	Attempt      int       `json:"attempt"`
	SourceSHA    string    `json:"source_sha"`
	Environment  string    `json:"environment"`
	ActualResult string    `json:"actual_result"`
	Status       string    `json:"status"`
	Evidence     []string  `json:"evidence"`
	CreatedAt    time.Time `json:"created_at"`
}

type Snapshot struct {
	DocumentSetID    int64                `json:"document_set_id"`
	DocumentSetName  string               `json:"document_set_name"`
	ProductName      string               `json:"product_name"`
	Scope            string               `json:"scope"`
	TestSuiteID      int64                `json:"test_suite_id"`
	TestSuiteName    string               `json:"test_suite_name"`
	TestRunID        *int64               `json:"test_run_id,omitempty"`
	GeneratedAt      time.Time            `json:"generated_at"`
	GeneratedBy      string               `json:"generated_by"`
	Reviewer         string               `json:"reviewer"`
	TestCaseFilter   []int64              `json:"test_case_filter"`
	SortBy           string               `json:"sort_by"`
	DocumentVersions []DocumentVersionRef `json:"document_versions"`
	Rows             []ExecutionRow       `json:"rows"`
	RunHistory       []RunHistoryRow      `json:"run_history"`
}

type ExportArtifact struct {
	ID            int64     `json:"id"`
	DocumentSetID int64     `json:"document_set_id"`
	TestSuiteID   int64     `json:"test_suite_id"`
	TestRunID     *int64    `json:"test_run_id,omitempty"`
	Format        string    `json:"format"`
	Filename      string    `json:"filename"`
	ContentType   string    `json:"content_type"`
	ContentHash   string    `json:"content_hash"`
	SnapshotHash  string    `json:"snapshot_hash"`
	RowCount      int       `json:"row_count"`
	GeneratedBy   string    `json:"generated_by"`
	CreatedAt     time.Time `json:"created_at"`
	DownloadURL   string    `json:"download_url"`
	Content       []byte    `json:"-"`
}
