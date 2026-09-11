package testcase

import "time"

type TestCase struct {
	ID                 int64     `json:"id"`
	TestSuiteID        int64     `json:"test_suite_id"`
	DocumentSetID      int64     `json:"document_set_id"`
	TestCaseKey        string    `json:"test_case_key"`
	VersionNumber      int       `json:"version_number"`
	Title              string    `json:"title"`
	TestType           string    `json:"test_type"`
	Risk               string    `json:"risk"`
	Actor              string    `json:"actor"`
	Precondition       string    `json:"precondition"`
	TestData           string    `json:"test_data"`
	ExpectedResult     string    `json:"expected_result"`
	ExpectedResultHash string    `json:"expected_result_hash"`
	Postcondition      string    `json:"postcondition"`
	Status             string    `json:"status"`
	AutomationStatus   string    `json:"automation_status"`
	Confidence         float64   `json:"confidence"`
	GeneratedBy        string    `json:"generated_by"`
	SupersedesID       *int64    `json:"supersedes_test_case_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Step struct {
	ID             int64     `json:"id"`
	TestCaseID     int64     `json:"test_case_id"`
	Ordinal        int       `json:"ordinal"`
	Action         string    `json:"action"`
	ExpectedResult string    `json:"expected_result"`
	CreatedAt      time.Time `json:"created_at"`
}

type RequirementLink struct {
	TestCaseID    int64     `json:"test_case_id"`
	RequirementID int64     `json:"requirement_id"`
	DocumentSetID int64     `json:"document_set_id"`
	CoverageType  string    `json:"coverage_type"`
	CreatedAt     time.Time `json:"created_at"`
}
