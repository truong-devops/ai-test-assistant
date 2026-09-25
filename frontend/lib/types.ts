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
  source_revision: number;
};

export type AIBudgetStatus = {
  unreconciled_reservations?: number;
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
  review_hash: string;
  review_blockers: string[];
  source_state: "CURRENT" | "HISTORICAL" | "REMOVED";
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
  source_snapshot_id?: number;
  source_revision: number;
  is_current: boolean;
  status: "PENDING" | "RUNNING" | "COMPLETED" | "FAILED" | "CANCELED";
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

export type DocumentWorkflowOperation = "INDEX_DOCUMENTS" | "EXTRACT_REQUIREMENTS" | "GENERATE_TESTCASES";
export type DocumentWorkflowStatus = "QUEUED" | "RUNNING" | "SUCCEEDED" | "PARTIAL_FAILED" | "FAILED" | "CANCELED";

export type WorkflowBlockingReason = {
  code: string;
  message: string;
  step: string;
  next_action?: string;
};

export type DocumentWorkflowJob = {
	units?: Array<{ id:number; unit_key:string; status:string; attempt_count:number; error_message?:string }>;
  id: number;
  document_set_id: number;
  operation: DocumentWorkflowOperation;
  status: DocumentWorkflowStatus;
  revision: number;
  requested_by: string;
  total_units: number;
  completed_units: number;
  failed_units: number;
  attempt_count: number;
  max_attempts: number;
  heartbeat_at?: string;
  cancel_requested_at?: string;
  error_code?: string;
  error_message?: string;
  retryable: boolean;
  output_refs: Record<string, unknown>;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  updated_at: string;
  usage: {
    reserved_tokens: number;
    input_tokens: number;
    output_tokens: number;
    reserved_cost_microusd: number;
    actual_cost_microusd: number;
  };
};

export type DocumentWorkflow = {
	generation_scope?: {default_scope:string; affected_requirement_ids:number[]; affected_family_ids:number[]; removed_family_ids:number[]; blocked_requirements:number; changes:Array<{classification:string; reason:string; before_ids:number[]; after_ids:number[]; affected_test_case_ids:number[]}>};
  source_intents: SourceIntent[];
  document_set_id: number;
  source_revision: number;
  steps: Array<{
    key: string;
    state: "READY" | "BLOCKED" | "IN_PROGRESS" | "COMPLETE" | "FAILED";
    completed_units: number;
    total_units: number;
    blocking_reasons: WorkflowBlockingReason[];
  }>;
  capabilities: {
    can_upload: boolean;
    can_manage: boolean;
    can_index: boolean;
    can_extract: boolean;
    can_generate: boolean;
    can_review: boolean;
    can_publish: boolean;
    can_retry_job: boolean;
    can_cancel_job: boolean;
    can_adjust_budget: boolean;
  };
  blocking_reasons: WorkflowBlockingReason[];
  active_jobs: DocumentWorkflowJob[];
  recent_jobs: DocumentWorkflowJob[];
  next_action: string;
};

export type SourceIntent = {
  id: number; document_set_id: number; source_revision: number;
  command: "INDEX" | "APPROVE" | "APPROVE_AND_EXTRACT" | "EXTRACT";
  status: "WAITING_PARSE" | "INDEXING" | "EXTRACTING" | "SUCCEEDED" | "FAILED" | "SUPERSEDED";
  request: { excluded_version_ids?: number[] };
  index_job_id?: number; extraction_job_id?: number; error_message?: string;
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
  needs_source_review: boolean;
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
  family_id: number;
  parent_revision_id?: number;
  restored_from_revision_id?: number;
  content_hash: string;
  created_by: string;
  change_reason: string;
  source_snapshot_id?: number;
  provenance: Record<string, unknown>;
  sealed_at?: string;
  created_at: string;
  updated_at: string;
  latest_execution?: {
    test_run_id: number;
    status: "NOT_RUN" | "PASSED" | "PRODUCT_FAILED" | "AUTOMATION_ERROR" | "INFRA_ERROR" | "TIMED_OUT" | "BLOCKED";
    actual_result: string;
    run_at: string;
  };
};

export type TestCaseFamily = {
  id: number;
  test_suite_id: number;
  document_set_id: number;
  public_key: string;
  legacy_key?: string;
  archived: boolean;
  revision_counter: number;
  head_revision_id?: number;
  head_token: string;
  needs_identity_review: boolean;
  created_at: string;
  updated_at: string;
  latest_revision?: BusinessTestCase;
  latest_approved_revision?: BusinessTestCase;
};

export type SuiteReleaseItem = {
  release_id: number;
  family_id: number;
  test_case_id: number;
  ordinal: number;
  public_key: string;
  revision_number: number;
  content_hash: string;
  expected_result_hash: string;
  revision: BusinessTestCase;
};

export type SuiteRelease = {
  id: number;
  test_suite_id: number;
  document_set_id: number;
  release_number: number;
  name: string;
  source_snapshot_id?: number;
  manifest_hash: string;
  scope_status: "COMPLETE" | "PARTIAL";
  approved_requirement_count: number;
  covered_requirement_count: number;
  uncovered_requirement_ids: number[];
  scope_decision: string;
  published_by: string;
  origin: "USER_PUBLISHED" | "MIGRATED_CURRENT_STATE";
  published_at: string;
  created_at: string;
  items: SuiteReleaseItem[];
};

export type TestCaseStepInput = { action: string; expected_result: string };

export type TestCaseEvidenceRef = {
  requirement_revision_id: number;
  requirement_evidence_id: number;
  document_version_id: number;
  document_block_id: number;
  source_locator: string;
  excerpt_hash: string;
};

export type TestCaseRevisionContent = {
  title: string;
  test_type: string;
  risk: string;
  actor: string;
  precondition: string;
  test_data: string;
  steps: TestCaseStepInput[];
  expected_result: string;
  postcondition: string;
  assumptions: string[];
  requirement_revision_ids: number[];
  evidence_refs: TestCaseEvidenceRef[];
  source_snapshot_id?: number;
};

export type TestCaseRevisionDiff = {
  family_id: number;
  from: BusinessTestCase;
  to: BusinessTestCase;
  changes: Array<{ field: string; before: unknown; after: unknown }>;
};

export type GenerationTarget = { family_id: number; revision_id: number; head_token: string };
export type GenerationProposal = {
	workflow_unit_id?: number;
  id: number; document_set_id: number; workflow_job_id: number;
  classification: "NEW_CASE" | "NEW_REVISION" | "UNCHANGED" | "RETIRE_CANDIDATE" | "AMBIGUOUS_MATCH";
  reason: string; content: TestCaseRevisionContent; content_hash: string;
  generation: Record<string, unknown>; candidates: GenerationTarget[];
  source_revision: number; status: "PENDING" | "APPLIED" | "DISMISSED";
  decision: string; decision_reason: string; decided_by: string;
  result_revision_id?: number;
};
export type ProposalPage = { proposals: GenerationProposal[]; next_before?: number };
export type ProposalComparison = { target: GenerationTarget; before: TestCaseRevisionContent; after: TestCaseRevisionContent };

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
  reviews: Array<{ id: number; reviewer_name: string; decision: string; comment: string;
    content_hash: string; actor: string; created_at: string }>;
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
  source_snapshot_id?: number;
  indexed_source_revision: number;
  source_revision: number;
  freshness: "CURRENT" | "STALE";
  extraction_ready: boolean;
  content_warning_count: number;
  sources: DocumentIndexVersionRef[];
  pending_versions: DocumentIndexVersionRef[];
  generations: DocumentIndexGeneration[];
};

export type DocumentIndexVersionRef = {
  document_id: number;
  document_name: string;
  document_version_id: number;
  version_number: number;
  sha256?: string;
  parse_status: DocumentVersion["parse_status"];
  approval_status: DocumentVersion["approval_status"];
  included: boolean;
  exclusion_reason?: string;
};

export type DocumentIndexGeneration = {
  document_set_id: number;
  generation: number;
  source_snapshot_id?: number;
  source_revision: number;
  input_fingerprint: string;
  embedding_model: string;
  status: "INDEXING" | "READY" | "FAILED" | "SUPERSEDED";
  version_count: number;
  excluded_version_count: number;
  chunk_count: number;
  warning_count: number;
  content_warning_count: number;
  error_message?: string;
  requested_at?: string;
  started_at?: string;
  finished_at?: string;
  updated_at: string;
};

export type SemanticChunk = {
  id: number;
  document_set_id: number;
  document_version_id: number;
  document_version_number: number;
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
  layers: Array<{
    key: "DESIGNED" | "PUBLISHED" | "AUTOMATED" | "EXECUTED";
    label: string;
    numerator: number;
    denominator: number;
    percent: number;
    source_snapshot_id?: number;
    suite_release_id?: number;
    release_number?: number;
  }>;
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
  suite_release_id?: number;
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
  suite_release_id: number;
  release_number: number;
  release_name: string;
  manifest_hash: string;
  release_published_at: string;
  selection_mode: "FULL_APPROVED" | "MAPPED_WITH_FULL_FALLBACK";
  selected_by: string;
  created_at: string;
  updated_at: string;
};

export type BaselineView = {
  bound: boolean;
  baseline?: ProjectDocumentBaseline;
  candidates: Array<{ document_set_id: number; document_set_name: string;
    test_suite_id: number; test_suite_name: string; suite_release_id: number;
    release_number: number; release_name: string; manifest_hash: string;
    release_published_at: string;
    approved_test_cases: number }>;
};

export type AnalysisTestScope = {
  analysis_job_id: number;
  project_id: number;
  document_set_id: number;
  test_suite_id: number;
  suite_release_id: number;
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
  suite_release_id?: number;
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
