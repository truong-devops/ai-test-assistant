export type Project = {
  id: number;
  name: string;
  provider: "gitlab" | "github";
  provider_project_id: number;
  gitlab_project_id?: number;
  repository_url: string;
  default_branch: string;
  language: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type IndexStatus = {
  project_id: number;
  ref: string;
  status: string;
  generation: number;
  attempt_count: number;
  file_count: number;
  skipped_file_count: number;
  chunk_count: number;
  embedding_model: string;
  error_message?: string;
  requested_at?: string;
  started_at?: string;
  finished_at?: string;
  updated_at?: string;
};

export type Analysis = {
  id: number;
  project_id: number;
  merge_request_iid: number;
  source_sha: string;
  target_sha: string;
  source_branch: string;
  target_branch: string;
  title: string;
  web_url: string;
  status: string;
  error_message?: string;
  attempt_count: number;
  started_at?: string;
  finished_at?: string;
  created_at: string;
};

export type ChangedFile = {
  id: number;
  analysis_job_id: number;
  old_path: string;
  new_path: string;
  change_type: string;
  additions: number;
  deletions: number;
  diff: string;
  new_file: boolean;
  renamed_file: boolean;
  deleted_file: boolean;
  collapsed: boolean;
  too_large: boolean;
};

export type ChangedSymbol = {
  id: number;
  changed_file_id: number;
  symbol_name: string;
  symbol_kind: string;
  receiver_name?: string;
  package_name: string;
  start_line: number;
  end_line: number;
  change_type: string;
  change_summary: string;
};

export type Recommendation = {
  id: number;
  analysis_job_id: number;
  changed_symbol_id: number;
  title: string;
  description: string;
  priority: string;
  rationale: string;
  scenario: string;
  expected_behavior: string;
  status: string;
  model_name: string;
  prompt_version: string;
  created_at: string;
  updated_at: string;
};

export type GeneratedTest = {
  id: number;
  analysis_job_id: number;
  recommendation_id: number;
  file_path: string;
  test_names: string[];
  code: string;
  code_hash: string;
  model_name: string;
  prompt_version: string;
  provider_response_id?: string;
  generation_attempt: number;
  created_at: string;
  updated_at: string;
};

export type ValidationRun = {
  id: number;
  analysis_job_id: number;
  generated_test_id: number;
  attempt_number: number;
  command: string;
  status: string;
  exit_code: number;
  duration_ms: number;
  stdout: string;
  stderr: string;
  output_truncated: boolean;
  created_at: string;
};

export type RepairAttempt = {
  id: number;
  analysis_job_id: number;
  generated_test_id: number;
  validation_run_id: number;
  repaired_generated_test_id: number;
  attempt_number: number;
  previous_code: string;
  repaired_code: string;
  previous_code_hash: string;
  repaired_code_hash: string;
  model_name: string;
  prompt_version: string;
  provider_response_id?: string;
  reason: string;
  created_at: string;
};

export type Review = {
  id: number;
  generated_test_id: number;
  reviewer_name: string;
  decision: string;
  comment: string;
  created_at: string;
};

export type KnowledgeChunk = {
  id: number;
  project_id: number;
  chunk_key: string;
  file_path: string;
  package_name: string;
  symbol_name: string;
  chunk_type: string;
  content: string;
  content_hash: string;
  start_line: number;
  end_line: number;
  embedding_model: string;
  score?: number;
  created_at: string;
  updated_at: string;
};

export type AnalysisDetail = {
  analysis: Analysis;
  changed_files: ChangedFile[];
  changed_symbols: ChangedSymbol[];
};

export type EvaluationRun = {
  id: number;
  name: string;
  schema_version: string;
  dataset_hash: string;
  description: string;
  observation_count: number;
  created_at: string;
};

export type RateMetric = { successes: number; total: number; rate_pct: number };

export type EvaluationGroup = {
  experiment: string;
  variant: string;
  observation_count: number;
  syntactic_validity?: RateMetric;
  compile_validity?: RateMetric;
  execution_validity?: RateMetric;
  human_acceptance?: RateMetric;
  first_pass_success?: RateMetric;
  repair_success?: RateMetric;
  final_success?: RateMetric;
  mean_duration_seconds?: number;
  mean_coverage_delta_pp?: number;
};

export type EvaluationComparison = {
  experiment: string;
  baseline_variant: string;
  treatment_variant: string;
  syntactic_validity_delta_pp?: number;
  compile_validity_delta_pp?: number;
  execution_validity_delta_pp?: number;
  human_acceptance_delta_pp?: number;
  first_pass_success_delta_pp?: number;
  repair_success_rate_pct?: number;
  final_success_delta_pp?: number;
  mean_coverage_delta_change_pp?: number;
  mean_duration_reduction_seconds?: number;
  mean_duration_reduction_pct?: number;
};

export type EvaluationReport = {
  schema_version: string;
  dataset_name: string;
  dataset_hash: string;
  description: string;
  groups: EvaluationGroup[];
  comparisons: EvaluationComparison[];
};

export type StoredEvaluation = { run: EvaluationRun; report: EvaluationReport };

export type ProvenanceContext = {
  id: number;
  query_text: string;
  index_ref: string;
  index_generation: number;
  embedding_model: string;
  item_count: number;
};

export type ProvenanceCall = {
  id: number;
  phase: "recommendation" | "generation" | "repair";
  attempt_number: number;
  source_sha: string;
  target_sha: string;
  provider: string;
  model_name: string;
  prompt_version: string;
  prompt_hash: string;
  configuration_hash: string;
  provider_response_id?: string;
  status: "COMPLETED" | "FAILED" | "INVALID_OUTPUT";
  error_message?: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  latency_ms: number;
  estimated_cost_usd: number;
  created_at: string;
  context?: ProvenanceContext;
};

export type ProvenanceBundle = {
  schema_version: string;
  analysis: Analysis;
  llm_calls: ProvenanceCall[];
};
