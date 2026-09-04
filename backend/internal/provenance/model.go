package provenance

import (
	"context"
	"encoding/json"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
)

const (
	SchemaVersion = "ai-provenance-v1"

	PhaseRecommendation = "recommendation"
	PhaseGeneration     = "generation"
	PhaseRepair         = "repair"

	StatusCompleted     = "COMPLETED"
	StatusFailed        = "FAILED"
	StatusInvalidOutput = "INVALID_OUTPUT"
)

type RuntimeConfig struct {
	Provider                string
	Model                   string
	InputCostPerMillionUSD  float64
	OutputCostPerMillionUSD float64
}

type RecordInput struct {
	Analysis       job.AnalysisJob
	Phase          string
	SubjectID      int64
	AttemptNumber  int
	PromptVersion  string
	RetrievalQuery knowledge.RetrievalQuery
	Contexts       []knowledge.KnowledgeChunk
	Request        llm.Request
	Response       llm.Response
	Status         string
	ErrorMessage   string
	Latency        time.Duration
}

type Call struct {
	ID                 int64            `json:"id"`
	AnalysisJobID      int64            `json:"analysis_job_id"`
	ProjectID          int64            `json:"project_id"`
	Phase              string           `json:"phase"`
	ChangedSymbolID    *int64           `json:"changed_symbol_id,omitempty"`
	RecommendationID   *int64           `json:"recommendation_id,omitempty"`
	GeneratedTestID    *int64           `json:"generated_test_id,omitempty"`
	AttemptNumber      int              `json:"attempt_number"`
	SourceSHA          string           `json:"source_sha"`
	TargetSHA          string           `json:"target_sha"`
	Provider           string           `json:"provider"`
	ModelName          string           `json:"model_name"`
	PromptVersion      string           `json:"prompt_version"`
	PromptHash         string           `json:"prompt_hash"`
	ConfigurationHash  string           `json:"configuration_hash"`
	Instructions       string           `json:"instructions"`
	PromptText         string           `json:"prompt_text"`
	SchemaName         string           `json:"schema_name"`
	RequestSchema      json.RawMessage  `json:"request_schema"`
	ProviderResponseID string           `json:"provider_response_id,omitempty"`
	ResponseText       string           `json:"response_text,omitempty"`
	Status             string           `json:"status"`
	ErrorMessage       string           `json:"error_message,omitempty"`
	InputTokens        int              `json:"input_tokens"`
	OutputTokens       int              `json:"output_tokens"`
	TotalTokens        int              `json:"total_tokens"`
	LatencyMS          int64            `json:"latency_ms"`
	EstimatedCostUSD   float64          `json:"estimated_cost_usd"`
	CreatedAt          time.Time        `json:"created_at"`
	Context            *ContextSnapshot `json:"context,omitempty"`
}

type ContextSnapshot struct {
	ID              int64                 `json:"id"`
	LLMCallID       int64                 `json:"llm_call_id"`
	ProjectID       int64                 `json:"project_id"`
	QueryText       string                `json:"query_text"`
	RetrievalQuery  json.RawMessage       `json:"retrieval_query"`
	RetrievalConfig json.RawMessage       `json:"retrieval_config"`
	IndexRef        string                `json:"index_ref"`
	IndexGeneration int64                 `json:"index_generation"`
	EmbeddingModel  string                `json:"embedding_model"`
	CreatedAt       time.Time             `json:"created_at"`
	Items           []ContextSnapshotItem `json:"items"`
}

type ContextSnapshotItem struct {
	ID                int64           `json:"id"`
	ContextSnapshotID int64           `json:"context_snapshot_id"`
	ProjectID         int64           `json:"project_id"`
	Ordinal           int             `json:"ordinal"`
	KnowledgeChunkID  int64           `json:"knowledge_chunk_id"`
	ChunkKey          string          `json:"chunk_key"`
	FilePath          string          `json:"file_path"`
	PackageName       string          `json:"package_name"`
	SymbolName        string          `json:"symbol_name"`
	ChunkType         string          `json:"chunk_type"`
	Content           string          `json:"content"`
	ContentHash       string          `json:"content_hash"`
	StartLine         int             `json:"start_line"`
	EndLine           int             `json:"end_line"`
	EmbeddingModel    string          `json:"embedding_model"`
	Score             float64         `json:"score"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         time.Time       `json:"created_at"`
}

type Bundle struct {
	SchemaVersion string          `json:"schema_version"`
	Analysis      job.AnalysisJob `json:"analysis"`
	Calls         []Call          `json:"llm_calls"`
}

type CallSummary struct {
	ID                 int64           `json:"id"`
	Phase              string          `json:"phase"`
	AttemptNumber      int             `json:"attempt_number"`
	SourceSHA          string          `json:"source_sha"`
	TargetSHA          string          `json:"target_sha"`
	Provider           string          `json:"provider"`
	ModelName          string          `json:"model_name"`
	PromptVersion      string          `json:"prompt_version"`
	PromptHash         string          `json:"prompt_hash"`
	ConfigurationHash  string          `json:"configuration_hash"`
	ProviderResponseID string          `json:"provider_response_id,omitempty"`
	Status             string          `json:"status"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	InputTokens        int             `json:"input_tokens"`
	OutputTokens       int             `json:"output_tokens"`
	TotalTokens        int             `json:"total_tokens"`
	LatencyMS          int64           `json:"latency_ms"`
	EstimatedCostUSD   float64         `json:"estimated_cost_usd"`
	CreatedAt          time.Time       `json:"created_at"`
	Context            *ContextSummary `json:"context,omitempty"`
}

type ContextSummary struct {
	ID              int64  `json:"id"`
	QueryText       string `json:"query_text"`
	IndexRef        string `json:"index_ref"`
	IndexGeneration int64  `json:"index_generation"`
	EmbeddingModel  string `json:"embedding_model"`
	ItemCount       int    `json:"item_count"`
}

type SummaryBundle struct {
	SchemaVersion string          `json:"schema_version"`
	Analysis      job.AnalysisJob `json:"analysis"`
	Calls         []CallSummary   `json:"llm_calls"`
}

type Recorder interface {
	Record(ctx context.Context, input RecordInput) (Call, error)
}
