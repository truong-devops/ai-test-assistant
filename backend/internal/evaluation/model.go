package evaluation

import "time"

const (
	SchemaVersionV1 = "evaluation-v1"

	ExperimentContextImpact = "CONTEXT_IMPACT"
	ExperimentRepairImpact  = "REPAIR_IMPACT"
	ExperimentHumanEffort   = "HUMAN_EFFORT"

	VariantDiffOnly       = "DIFF_ONLY"
	VariantDiffRAG        = "DIFF_RAG"
	VariantGenerateOnly   = "GENERATE_ONLY"
	VariantGenerateRepair = "GENERATE_REPAIR"
	VariantManual         = "MANUAL"
	VariantAIAssisted     = "AI_ASSISTED"
)

type Dataset struct {
	SchemaVersion string        `json:"schema_version"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Observations  []Observation `json:"observations"`
}

type Observation struct {
	Key               string   `json:"key"`
	Scenario          string   `json:"scenario"`
	Experiment        string   `json:"experiment"`
	Variant           string   `json:"variant"`
	Replicate         int      `json:"replicate"`
	SyntacticValid    *bool    `json:"syntactic_valid,omitempty"`
	CompileValid      *bool    `json:"compile_valid,omitempty"`
	ExecutionValid    *bool    `json:"execution_valid,omitempty"`
	HumanAccepted     *bool    `json:"human_accepted,omitempty"`
	FirstPassSuccess  *bool    `json:"first_pass_success,omitempty"`
	RepairAttempted   *bool    `json:"repair_attempted,omitempty"`
	RepairSuccess     *bool    `json:"repair_success,omitempty"`
	FinalSuccess      *bool    `json:"final_success,omitempty"`
	DurationSeconds   *float64 `json:"duration_seconds,omitempty"`
	CoverageBeforePct *float64 `json:"coverage_before_pct,omitempty"`
	CoverageAfterPct  *float64 `json:"coverage_after_pct,omitempty"`
	Notes             string   `json:"notes,omitempty"`
}

type RateMetric struct {
	Successes int     `json:"successes"`
	Total     int     `json:"total"`
	RatePct   float64 `json:"rate_pct"`
}

type GroupSummary struct {
	Experiment          string      `json:"experiment"`
	Variant             string      `json:"variant"`
	ObservationCount    int         `json:"observation_count"`
	SyntacticValidity   *RateMetric `json:"syntactic_validity,omitempty"`
	CompileValidity     *RateMetric `json:"compile_validity,omitempty"`
	ExecutionValidity   *RateMetric `json:"execution_validity,omitempty"`
	HumanAcceptance     *RateMetric `json:"human_acceptance,omitempty"`
	FirstPassSuccess    *RateMetric `json:"first_pass_success,omitempty"`
	RepairSuccess       *RateMetric `json:"repair_success,omitempty"`
	FinalSuccess        *RateMetric `json:"final_success,omitempty"`
	MeanDurationSeconds *float64    `json:"mean_duration_seconds,omitempty"`
	MeanCoverageDeltaPP *float64    `json:"mean_coverage_delta_pp,omitempty"`
}

type Comparison struct {
	Experiment                string   `json:"experiment"`
	BaselineVariant           string   `json:"baseline_variant"`
	TreatmentVariant          string   `json:"treatment_variant"`
	SyntacticValidityDeltaPP  *float64 `json:"syntactic_validity_delta_pp,omitempty"`
	CompileValidityDeltaPP    *float64 `json:"compile_validity_delta_pp,omitempty"`
	ExecutionValidityDeltaPP  *float64 `json:"execution_validity_delta_pp,omitempty"`
	HumanAcceptanceDeltaPP    *float64 `json:"human_acceptance_delta_pp,omitempty"`
	FirstPassSuccessDeltaPP   *float64 `json:"first_pass_success_delta_pp,omitempty"`
	RepairSuccessRatePct      *float64 `json:"repair_success_rate_pct,omitempty"`
	FinalSuccessDeltaPP       *float64 `json:"final_success_delta_pp,omitempty"`
	MeanCoverageDeltaChangePP *float64 `json:"mean_coverage_delta_change_pp,omitempty"`
	MeanDurationReductionSec  *float64 `json:"mean_duration_reduction_seconds,omitempty"`
	MeanDurationReductionPct  *float64 `json:"mean_duration_reduction_pct,omitempty"`
}

type Report struct {
	SchemaVersion string         `json:"schema_version"`
	DatasetName   string         `json:"dataset_name"`
	DatasetHash   string         `json:"dataset_hash"`
	Description   string         `json:"description"`
	Groups        []GroupSummary `json:"groups"`
	Comparisons   []Comparison   `json:"comparisons"`
}

type Run struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	SchemaVersion    string    `json:"schema_version"`
	DatasetHash      string    `json:"dataset_hash"`
	Description      string    `json:"description"`
	ObservationCount int       `json:"observation_count"`
	CreatedAt        time.Time `json:"created_at"`
}

type StoredReport struct {
	Run    Run    `json:"run"`
	Report Report `json:"report"`
}
