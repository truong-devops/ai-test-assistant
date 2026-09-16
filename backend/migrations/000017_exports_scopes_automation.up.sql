ALTER TABLE test_runs
    ADD CONSTRAINT test_runs_id_suite_unique UNIQUE (id, test_suite_id);

CREATE UNIQUE INDEX test_runs_analysis_unique_idx
    ON test_runs (analysis_job_id) WHERE analysis_job_id IS NOT NULL;

CREATE TABLE test_exports (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    test_suite_id BIGINT NOT NULL,
    test_run_id BIGINT,
    format TEXT NOT NULL CHECK (format IN ('XLSX', 'MARKDOWN')),
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    content BYTEA NOT NULL,
    content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
    snapshot_hash TEXT NOT NULL CHECK (snapshot_hash ~ '^[0-9a-f]{64}$'),
    row_count INTEGER NOT NULL CHECK (row_count >= 0),
    generated_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (test_suite_id, document_set_id)
        REFERENCES test_suites(id, document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (test_run_id, test_suite_id)
        REFERENCES test_runs(id, test_suite_id) ON DELETE RESTRICT
);

CREATE INDEX test_exports_set_created_idx
    ON test_exports (document_set_id, created_at DESC, id DESC);

CREATE TABLE project_document_baselines (
    project_id BIGINT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE RESTRICT,
    test_suite_id BIGINT NOT NULL,
    selection_mode TEXT NOT NULL DEFAULT 'MAPPED_WITH_FULL_FALLBACK' CHECK
        (selection_mode IN ('FULL_APPROVED', 'MAPPED_WITH_FULL_FALLBACK')),
    selected_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (test_suite_id, document_set_id)
        REFERENCES test_suites(id, document_set_id) ON DELETE RESTRICT
);

CREATE TABLE analysis_baseline_snapshots (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL UNIQUE REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE RESTRICT,
    test_suite_id BIGINT NOT NULL,
    baseline_hash TEXT NOT NULL CHECK (baseline_hash ~ '^[0-9a-f]{64}$'),
    document_versions JSONB NOT NULL CHECK (jsonb_typeof(document_versions) = 'array'),
    requirements JSONB NOT NULL CHECK (jsonb_typeof(requirements) = 'array'),
    test_cases JSONB NOT NULL CHECK (jsonb_typeof(test_cases) = 'array'),
    explicit_identifiers TEXT[] NOT NULL DEFAULT '{}',
    selection_mode TEXT NOT NULL CHECK
        (selection_mode IN ('FULL_APPROVED', 'EXPLICIT_TRACE', 'FULL_BASELINE_FALLBACK')),
    mapping_confidence DOUBLE PRECISION NOT NULL CHECK (mapping_confidence BETWEEN 0 AND 1),
    warning TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (test_suite_id, document_set_id)
        REFERENCES test_suites(id, document_set_id) ON DELETE RESTRICT,
    UNIQUE (id, analysis_job_id)
);

CREATE INDEX analysis_baseline_snapshots_project_idx
    ON analysis_baseline_snapshots (project_id, created_at DESC);

CREATE TABLE analysis_test_scope_items (
    id BIGSERIAL PRIMARY KEY,
    baseline_snapshot_id BIGINT NOT NULL,
    analysis_job_id BIGINT NOT NULL,
    test_case_id BIGINT NOT NULL REFERENCES test_cases(id) ON DELETE RESTRICT,
    included BOOLEAN NOT NULL DEFAULT TRUE,
    selection_reason TEXT NOT NULL CHECK (selection_reason IN
        ('FULL_APPROVED', 'EXPLICIT_TRACE', 'ISSUE_LINK', 'PATH_MAPPING',
         'IMPACT_INFERENCE', 'MANUAL', 'FULL_BASELINE_FALLBACK')),
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    explanation TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (baseline_snapshot_id, analysis_job_id)
        REFERENCES analysis_baseline_snapshots(id, analysis_job_id) ON DELETE CASCADE,
    UNIQUE (analysis_job_id, test_case_id),
    UNIQUE (id, analysis_job_id)
);

CREATE INDEX analysis_test_scope_items_analysis_idx
    ON analysis_test_scope_items (analysis_job_id, included, id);

CREATE TABLE analysis_scope_signals (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    signal_type TEXT NOT NULL CHECK (signal_type IN ('PATH', 'MODULE', 'SYMBOL')),
    signal_value TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    explanation TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (analysis_job_id, signal_type, signal_value)
);

CREATE INDEX analysis_scope_signals_analysis_idx
    ON analysis_scope_signals (analysis_job_id, signal_type, id);

CREATE TABLE analysis_scope_decisions (
    id BIGSERIAL PRIMARY KEY,
    scope_item_id BIGINT NOT NULL REFERENCES analysis_test_scope_items(id) ON DELETE CASCADE,
    analysis_job_id BIGINT NOT NULL REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    reviewer_name TEXT NOT NULL,
    included BOOLEAN NOT NULL,
    comment TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (scope_item_id, analysis_job_id)
        REFERENCES analysis_test_scope_items(id, analysis_job_id) ON DELETE CASCADE
);

ALTER TABLE automation_artifacts
    ADD COLUMN analysis_job_id BIGINT REFERENCES analysis_jobs(id) ON DELETE SET NULL,
    ADD COLUMN setup_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN assertions JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(assertions) = 'array'),
    ADD COLUMN test_case_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(test_case_snapshot) = 'object'),
    ADD COLUMN business_context JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(business_context) = 'object'),
    ADD COLUMN technical_context JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(technical_context) = 'array'),
    ADD COLUMN business_context_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN technical_context_hash TEXT NOT NULL DEFAULT '';

ALTER TABLE automation_artifacts
    ADD CONSTRAINT automation_business_context_hash_check CHECK
        (business_context_hash = '' OR business_context_hash ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT automation_technical_context_hash_check CHECK
        (technical_context_hash = '' OR technical_context_hash ~ '^[0-9a-f]{64}$');

CREATE INDEX automation_artifacts_analysis_idx
    ON automation_artifacts (analysis_job_id, test_case_id, version_number DESC);

CREATE TABLE automation_generation_calls (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    test_case_id BIGINT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    artifact_id BIGINT REFERENCES automation_artifacts(id) ON DELETE SET NULL,
    provider TEXT NOT NULL,
    model_name TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    instructions TEXT NOT NULL,
    prompt_text TEXT NOT NULL,
    request_schema JSONB NOT NULL CHECK (jsonb_typeof(request_schema) = 'object'),
    response_text TEXT NOT NULL DEFAULT '',
    provider_response_id TEXT NOT NULL DEFAULT '',
    business_context JSONB NOT NULL CHECK (jsonb_typeof(business_context) = 'object'),
    technical_context JSONB NOT NULL CHECK (jsonb_typeof(technical_context) = 'array'),
    expected_result_hash TEXT NOT NULL CHECK (expected_result_hash ~ '^[0-9a-f]{64}$'),
    status TEXT NOT NULL CHECK (status IN ('COMPLETED', 'FAILED', 'INVALID_OUTPUT', 'BLOCKED')),
    error_message TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    latency_ms BIGINT NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (artifact_id, test_case_id)
        REFERENCES automation_artifacts(id, test_case_id) ON DELETE SET NULL (artifact_id)
);

CREATE INDEX automation_generation_calls_subject_idx
    ON automation_generation_calls (analysis_job_id, test_case_id, created_at DESC);

CREATE TABLE automation_artifact_reviews (
    id BIGSERIAL PRIMARY KEY,
    automation_artifact_id BIGINT NOT NULL REFERENCES automation_artifacts(id) ON DELETE CASCADE,
    reviewer_name TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('APPROVED', 'REJECTED')),
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (automation_artifact_id)
);

CREATE INDEX automation_artifact_reviews_artifact_idx
    ON automation_artifact_reviews (automation_artifact_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION prevent_phase_6_8_snapshot_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% rows are immutable', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER test_exports_immutable
BEFORE UPDATE ON test_exports
FOR EACH ROW EXECUTE FUNCTION prevent_phase_6_8_snapshot_update();

CREATE TRIGGER analysis_baseline_snapshots_immutable
BEFORE UPDATE ON analysis_baseline_snapshots
FOR EACH ROW EXECUTE FUNCTION prevent_phase_6_8_snapshot_update();

CREATE TRIGGER automation_generation_calls_immutable
BEFORE UPDATE ON automation_generation_calls
FOR EACH ROW EXECUTE FUNCTION prevent_phase_6_8_snapshot_update();

CREATE OR REPLACE FUNCTION prevent_automation_artifact_contract_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.test_case_id <> OLD.test_case_id
       OR NEW.version_number <> OLD.version_number
       OR NEW.framework <> OLD.framework
       OR NEW.file_path <> OLD.file_path
       OR NEW.source <> OLD.source
       OR NEW.source_hash <> OLD.source_hash
       OR NEW.expected_result_hash <> OLD.expected_result_hash
       OR NEW.test_case_snapshot <> OLD.test_case_snapshot
       OR NEW.business_context <> OLD.business_context
       OR NEW.technical_context <> OLD.technical_context
       OR NEW.business_context_hash <> OLD.business_context_hash
       OR NEW.technical_context_hash <> OLD.technical_context_hash THEN
        RAISE EXCEPTION 'automation artifact contract is immutable; create a new version';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER automation_artifacts_immutable_contract
BEFORE UPDATE ON automation_artifacts
FOR EACH ROW EXECUTE FUNCTION prevent_automation_artifact_contract_update();
