package recommendation

import "time"

const (
	StatusPending         = "PENDING"
	StatusUseful          = "USEFUL"
	StatusPartiallyUseful = "PARTIALLY_USEFUL"
	StatusNotUseful       = "NOT_USEFUL"

	PriorityLow    = "low"
	PriorityMedium = "medium"
	PriorityHigh   = "high"

	PromptVersion = "recommend-test-v1"
)

type Recommendation struct {
	ID                 int64     `json:"id"`
	AnalysisJobID      int64     `json:"analysis_job_id"`
	ChangedSymbolID    int64     `json:"changed_symbol_id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Priority           string    `json:"priority"`
	Rationale          string    `json:"rationale"`
	Scenario           string    `json:"scenario"`
	ExpectedBehavior   string    `json:"expected_behavior"`
	Status             string    `json:"status"`
	ModelName          string    `json:"model_name"`
	PromptVersion      string    `json:"prompt_version"`
	ProviderResponseID string    `json:"provider_response_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ProposedRecommendation struct {
	Title            string `json:"title"`
	Description      string `json:"description"`
	Priority         string `json:"priority"`
	Rationale        string `json:"rationale"`
	Scenario         string `json:"scenario"`
	ExpectedBehavior string `json:"expected_behavior"`
}

type ProposedResponse struct {
	Recommendations []ProposedRecommendation `json:"recommendations"`
}
