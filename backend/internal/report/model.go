package report

import "time"

// ExecutionRow is the provider-neutral contract used by the future XLSX and
// Markdown exporters. Phase 6 will implement rendering; defining it separately
// now prevents report fields from leaking into the legacy validation model.
type ExecutionRow struct {
	TestCaseID       string
	RequirementTrace string
	Objective        string
	Preconditions    string
	Steps            []string
	TestData         string
	Role             string
	ExpectedResult   string
	ActualResult     string
	Status           string
	Evidence         []string
}

type TestRun struct {
	ID                     int64      `json:"id"`
	TestSuiteID            int64      `json:"test_suite_id"`
	ProjectID              *int64     `json:"project_id,omitempty"`
	AnalysisJobID          *int64     `json:"analysis_job_id,omitempty"`
	SourceSHA              string     `json:"source_sha"`
	TargetSHA              string     `json:"target_sha"`
	Environment            string     `json:"environment"`
	EnvironmentFingerprint string     `json:"environment_fingerprint"`
	Status                 string     `json:"status"`
	RequestedAt            time.Time  `json:"requested_at"`
	StartedAt              *time.Time `json:"started_at,omitempty"`
	FinishedAt             *time.Time `json:"finished_at,omitempty"`
}
