package generation

import "time"

const (
	PromptVersion         = "generate-test-v1"
	InitialAttempt        = 1
	MaxGeneratedCodeBytes = 512 << 10
)

type GeneratedTest struct {
	ID                 int64     `json:"id"`
	AnalysisJobID      int64     `json:"analysis_job_id"`
	RecommendationID   int64     `json:"recommendation_id"`
	FilePath           string    `json:"file_path"`
	TestNames          []string  `json:"test_names"`
	Code               string    `json:"code"`
	CodeHash           string    `json:"code_hash"`
	ModelName          string    `json:"model_name"`
	PromptVersion      string    `json:"prompt_version"`
	ProviderResponseID string    `json:"provider_response_id,omitempty"`
	GenerationAttempt  int       `json:"generation_attempt"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ProposedGeneration struct {
	TargetFile string   `json:"target_file"`
	TestNames  []string `json:"test_names"`
	Code       string   `json:"code"`
}
