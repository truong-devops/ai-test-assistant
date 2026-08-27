package repair

import (
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
)

const (
	PromptVersion     = "repair-test-v1"
	MaxPromptLogRunes = 16_000
	MaxReasonRunes    = 8_000
)

type Attempt struct {
	ID                      int64     `json:"id"`
	AnalysisJobID           int64     `json:"analysis_job_id"`
	GeneratedTestID         int64     `json:"generated_test_id"`
	ValidationRunID         int64     `json:"validation_run_id"`
	RepairedGeneratedTestID int64     `json:"repaired_generated_test_id"`
	AttemptNumber           int       `json:"attempt_number"`
	PreviousCode            string    `json:"previous_code"`
	RepairedCode            string    `json:"repaired_code"`
	PreviousCodeHash        string    `json:"previous_code_hash"`
	RepairedCodeHash        string    `json:"repaired_code_hash"`
	ModelName               string    `json:"model_name"`
	PromptVersion           string    `json:"prompt_version"`
	ProviderResponseID      string    `json:"provider_response_id,omitempty"`
	Reason                  string    `json:"reason"`
	CreatedAt               time.Time `json:"created_at"`
}

type ProposedRepair struct {
	SourceGeneratedTestID int64
	ValidationRunID       int64
	AttemptNumber         int
	Generated             generation.GeneratedTest
	Reason                string
}
