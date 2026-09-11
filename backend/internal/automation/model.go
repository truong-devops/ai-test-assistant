package automation

import "time"

// Artifact is a technical implementation of an approved, versioned test case.
// ExpectedResultHash is copied from that business specification and must remain
// stable across generation and repair.
type Artifact struct {
	ID                 int64     `json:"id"`
	TestCaseID         int64     `json:"test_case_id"`
	VersionNumber      int       `json:"version_number"`
	Framework          string    `json:"framework"`
	FilePath           string    `json:"file_path"`
	Source             string    `json:"source"`
	SourceHash         string    `json:"source_hash"`
	ExpectedResultHash string    `json:"expected_result_hash"`
	Status             string    `json:"status"`
	ModelName          string    `json:"model_name"`
	PromptVersion      string    `json:"prompt_version"`
	ProviderResponseID string    `json:"provider_response_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}
