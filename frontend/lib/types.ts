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
  pipeline_mode: "DOCUMENT_DRIVEN" | "LEGACY";
  created_at: string;
  updated_at: string;
};

export type DocumentSet = {
  id: number;
  name: string;
  product_name: string;
  scope: string;
  description: string;
  status: "ACTIVE" | "ARCHIVED" | "PURGING";
  retention_days: number;
  ai_token_budget: number;
  ai_cost_budget_microusd: number;
  archived_at?: string;
  created_at: string;
  updated_at: string;
};

export type AIBudgetStatus = {
  document_set_id: number;
  token_budget: number;
  used_tokens: number;
  reserved_tokens: number;
  remaining_tokens: number;
  cost_budget_microusd: number;
  used_cost_microusd: number;
  reserved_cost_microusd: number;
  remaining_cost_microusd: number;
};

export type DocumentPipelineMetrics = {
  document_sets: number;
  document_versions: number;
  parsed_versions: number;
  parse_failures: number;
  approved_requirements: number;
  extraction_jobs: number;
  extraction_failures: number;
  test_suites: number;
  approved_test_cases: number;
  automation_artifacts: number;
  test_runs: number;
  runs_needing_attention: number;
  pending_approval_actions: number;
};

export type DocumentVersion = {
  id: number;
  document_id: number;
  document_set_id: number;
  version_number: number;
  original_filename: string;
  media_type: string;
  size_bytes: number;
  sha256: string;
  approval_status: "DRAFT" | "APPROVED" | "REJECTED";
  parse_status: "UPLOADED" | "PARSING" | "PARSED" | "FAILED";
  parse_error?: string;
  block_count: number;
  attempt_count: number;
  next_attempt_at: string;
  lease_expires_at?: string;
  uploaded_at: string;
  started_at?: string;
  parsed_at?: string;
};

export type SourceDocument = {
  id: number;
  document_set_id: number;
  name: string;
  document_type: string;
  created_at: string;
  updated_at: string;
  latest_version?: DocumentVersion;
};

export type DocumentBlock = {
  id: number;
  document_version_id: number;
  ordinal: number;
  block_type: "HEADING" | "PARAGRAPH" | "LIST" | "TABLE" | "CODE";
  heading_level: number;
  content: string;
  source_locator: string;
  metadata: Record<string, unknown>;
  created_at: string;
};

export type DocumentVersionDetail = {
  document: SourceDocument;
  version: DocumentVersion;
  blocks: DocumentBlock[];
};

// Phase 1 domain contracts. Their APIs and full workspaces arrive in Phases
// 4–9; keeping them separate from legacy generated tests prevents the UI from
// treating automation source as a business test case.
export type Requirement = {
  id: number;
  document_set_id: number;
  requirement_key: string;
  version_number: number;
  title: string;
  statement: string;
  requirement_type: string;
  flow_type: "NONE" | "MAIN" | "ALTERNATE" | "EXCEPTION";
  actor: string;
  precondition: string;
  postcondition: string;
  priority: "LOW" | "MEDIUM" | "HIGH";
  risk: "LOW" | "MEDIUM" | "HIGH";
  status: "DRAFT" | "APPROVED" | "REJECTED" | "CONFLICT" | "TBD";
  confidence: number;
  assumptions: string[];
  supersedes_requirement_id?: number;
  document_ids?: number[];
};

export type RequirementExtractionJob = {
  id: number;
  document_set_id: number;
  index_generation: number;
  status: "PENDING" | "RUNNING" | "COMPLETED" | "FAILED";
  total_chunks: number;
  processed_chunks: number;
  created_count: number;
  reused_count: number;
  conflict_count: number;
  open_question_count: number;
  requested_by: string;
  attempt_count: number;
  error_message?: string;
  created_at: string;
};

export type RequirementEvidence = {
  id: number;
  requirement_id: number;
  document_set_id: number;
  document_version_id: number;
  document_block_id: number;
  document_name: string;
  version_number: number;
  approval_status: "DRAFT" | "APPROVED" | "REJECTED";
  source_locator: string;
  excerpt: string;
  excerpt_hash: string;
};

export type RequirementFlowStep = {
  id: number;
  requirement_id: number;
  ordinal: number;
  action: string;
  expected_result: string;
};

export type RequirementDetail = {
  requirement: Requirement;
  evidence: RequirementEvidence[];
  flow_steps: RequirementFlowStep[];
  reviews: Array<{ id: number; reviewer_name: string; decision: string; comment: string; created_at: string }>;
};

export type RequirementConflict = {
  id: number;
  document_set_id: number;
  left_requirement_id: number;
  right_requirement_id: number;
  reason: string;
  status: "OPEN" | "RESOLVED" | "DISMISSED";
  resolution: string;
};

export type OpenQuestion = {
  id: number;
  document_set_id: number;
  requirement_id?: number;
  question: string;
  owner_role: string;
  status: "OPEN" | "ANSWERED" | "DISMISSED";
  answer: string;
};

export type BusinessTestCase = {
  id: number;
  test_suite_id: number;
  document_set_id: number;
  test_case_key: string;
  version_number: number;
  title: string;
  test_type: string;
  risk: "LOW" | "MEDIUM" | "HIGH";
  actor: string;
  precondition: string;
  test_data: string;
  expected_result: string;
  expected_result_hash: string;
  postcondition: string;
  status: "DRAFT" | "APPROVED" | "REJECTED";
  automation_status: "MANUAL" | "AUTOMATABLE" | "AUTOMATED" | "BLOCKED";
  confidence: number;
  generated_by: string;
  assumptions: string[];
  supersedes_test_case_id?: number;
};

export type BusinessTestCaseDetail = {
  test_case: BusinessTestCase;
  steps: Array<{ id: number; ordinal: number; action: string; expected_result: string }>;
  requirements: Array<{
    requirement_id: number;
    requirement_key: string;
    requirement_title: string;
    coverage_type: string;
    flow_type: string;
  }>;
  evidence: RequirementEvidence[];
  reviews: Array<{ id: number; reviewer_name: string; decision: string; comment: string; created_at: string }>;
};

export type DocumentIndexStatus = {
  document_set_id: number;
  status: "NOT_INDEXED" | "INDEXING" | "READY" | "FAILED";
  generation: number;
  input_fingerprint?: string;
  version_count: number;
  skipped_version_count: number;
  chunk_count: number;
  warning_count: number;
  embedding_model: string;
  error_message?: string;
  requested_at?: string;
  started_at?: string;
  finished_at?: string;
  updated_at?: string;
};

export type SemanticChunk = {
  id: number;
  document_set_id: number;
  document_version_id: number;
  document_block_id: number;
  chunk_key: string;
  parent_chunk_key: string;
  chunk_type: string;
  flow_type: "NONE" | "MAIN" | "ALTERNATE" | "EXCEPTION";
  identifier: string;
  title: string;
  content: string;
  raw_content: string;
  content_hash: string;
  source_locator: string;
  embedding_model: string;
  metadata: Record<string, unknown>;
  approval_status: "DRAFT" | "APPROVED" | "REJECTED";
  exact_score?: number;
  lexical_score?: number;
  semantic_score?: number;
  authority_score?: number;
  score?: number;
};

export type CoverageReport = {
  document_set_id: number;
  approved_denominator: number;
  covered_count: number;
  coverage_percent: number;
  baseline_complete: boolean;
  conflict_count: number;
  tbd_count: number;
  rejected_count: number;
  duplicate_count: number;
  uncovered_count: number;
  matrix: Array<{
    requirement_id: number;
    requirement_key: string;
    title: string;
    status: string;
    flow_type: string;
    test_case_ids: number[];
    test_types: string[];
    covered: boolean;
    warnings: string[];
  }>;
};

export type AutomationArtifact = {
  id: number;
  test_case_id: number;
  version_number: number;
  framework: string;
  file_path: string;
  source: string;
  source_hash: string;
  expected_result_hash: string;
  status: "DRAFT" | "APPROVED" | "REJECTED" | "UNREPAIRABLE";
  analysis_job_id?: number;
  setup: string;
  assertions: string[];
  test_case_snapshot: Record<string, unknown>;
  business_context: Record<string, unknown>;
  technical_context: KnowledgeChunk[];
  business_context_hash: string;
  technical_context_hash: string;
  model_name: string;
  prompt_version: string;
  created_at: string;
};

export type TestExport = {
  id: number;
  document_set_id: number;
  test_suite_id: number;
  test_run_id?: number;
  format: "XLSX" | "MARKDOWN";
  filename: string;
  content_type: string;
  content_hash: string;
  snapshot_hash: string;
  row_count: number;
  generated_by: string;
  created_at: string;
  download_url: string;
};

export type ProjectDocumentBaseline = {
  project_id: number;
  document_set_id: number;
  document_set_name: string;
  test_suite_id: number;
  test_suite_name: string;
  selection_mode: "FULL_APPROVED" | "MAPPED_WITH_FULL_FALLBACK";
  selected_by: string;
  created_at: string;
  updated_at: string;
};

export type BaselineView = {
  bound: boolean;
  baseline?: ProjectDocumentBaseline;
  candidates: Array<{ document_set_id: number; document_set_name: string; test_suite_id: number; test_suite_name: string; approved_test_cases: number }>;
};

export type AnalysisTestScope = {
  analysis_job_id: number;
  project_id: number;
  document_set_id: number;
  test_suite_id: number;
  test_run_id: number;
  baseline_hash: string;
  explicit_identifiers: string[];
  selection_mode: "FULL_APPROVED" | "EXPLICIT_TRACE" | "FULL_BASELINE_FALLBACK";
  mapping_confidence: number;
  warning?: string;
  items: Array<{ id: number; test_case_id: number; test_case_key: string; title: string; test_type: string; risk: string; expected_result: string; expected_result_hash: string; automation_status: string; included: boolean; selection_reason: string; confidence: number; explanation: string; updated_at: string }>;
  decisions: Array<{ id: number; scope_item_id: number; reviewer_name: string; included: boolean; comment: string; created_at: string }>;
	signals: Array<{ id: number; signal_type: "PATH" | "MODULE" | "SYMBOL"; signal_value: string; confidence: number; explanation: string; created_at: string }>;
  created_at: string;
};

export type AutomationHistory = {
  artifacts: AutomationArtifact[];
  reviews: Array<{ id: number; automation_artifact_id: number; reviewer_name: string; decision: string; comment: string; created_at: string }>;
};

export type AutomationRepairJob = {
  id: number;
  test_run_item_id: number;
  source_artifact_id: number;
  repaired_artifact_id?: number;
  attempt_number: number;
  status: "PENDING" | "RUNNING" | "WAITING_REVIEW" | "APPROVED" | "REJECTED" | "UNREPAIRABLE" | "FAILED";
  error_type: "AUTOMATION_ERROR";
  reason: string;
  requested_by: string;
  allowed_change_policy: { mutable: string[]; immutable: string[] };
  expected_result_hash: string;
  before_source_hash: string;
  after_source_hash?: string;
  before_assertions: string[];
  after_assertions?: string[];
  model_name?: string;
  prompt_version: string;
  provider_response_id?: string;
  input_tokens: number;
  output_tokens: number;
  max_output_tokens: number;
  max_cost_microusd: number;
  estimated_cost_microusd: number;
  queue_attempt_count: number;
  error_message?: string;
  created_at: string;
};

export type TestRun = {
  id: number;
  test_suite_id: number;
  project_id?: number;
  analysis_job_id?: number;
  source_sha: string;
  target_sha: string;
  environment: string;
  environment_fingerprint: string;
	image_reference: string;
	image_digest: string;
  status: "PENDING" | "RUNNING" | "COMPLETED" | "FAILED";
	execution_requested_at?: string;
	execution_requested_by: string;
	attempt_count: number;
	error_message?: string;
  requested_at: string;
  started_at?: string;
  finished_at?: string;
};

export type TestRunItem = {
	id: number;
	test_run_id: number;
	test_case_id: number;
	test_case_key: string;
	title: string;
	expected_result: string;
	expected_result_hash: string;
	automation_artifact_id?: number;
	automation_source_hash: string;
	attempt_number: number;
	status: "NOT_RUN" | "PASSED" | "PRODUCT_FAILED" | "AUTOMATION_ERROR" | "INFRA_ERROR" | "TIMED_OUT" | "BLOCKED";
	actual_result: string;
	command: string;
	exit_code?: number;
	duration_ms: number;
	output_truncated: boolean;
	created_at: string;
	evidence: Array<{ id: number; evidence_type: string; content?: string; storage_key?: string; content_hash: string; created_at: string }>;
};

export type TestRunDetail = TestRun & {
	items: TestRunItem[];
	classification_reviews: Array<{ id: number; test_run_item_id: number; reviewer_name: string; previous_status: string; new_status: string; reason: string; created_at: string }>;
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

export type ImpactRun = {
  id: number;
  analysis_job_id: number;
  project_id: number;
  source_sha: string;
  mode: "SSA" | "AST_FALLBACK";
  algorithm: string;
  max_depth: number;
  max_nodes: number;
  package_count: number;
  fallback_reason?: string;
  created_at: string;
};

export type ImpactNode = {
  id: number;
  stable_key: string;
  package_path: string;
  package_name: string;
  symbol_name: string;
  receiver_name?: string;
  symbol_kind: string;
  file_path: string;
  start_line: number;
  end_line: number;
  direct_change: boolean;
  existing_test: boolean;
  depth: number;
  score: number;
  reason_codes: string[];
};

export type ImpactEdge = {
  id: number;
  from_node_id: number;
  to_node_id: number;
  relation: "CALLS" | "IMPLEMENTS" | "USES_TYPE";
  reason_code: "CALLER" | "CALLEE" | "INTERFACE_IMPLEMENTATION" | "TYPE_USAGE" | "EXISTING_TEST";
  depth: number;
  score: number;
};

export type ImpactBundle = { run: ImpactRun; nodes: ImpactNode[]; edges: ImpactEdge[] };
