ALTER TABLE analysis_jobs
    ADD CONSTRAINT analysis_jobs_id_project_id_unique UNIQUE (id, project_id);

CREATE TABLE llm_calls (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    phase TEXT NOT NULL CHECK (phase IN ('recommendation', 'generation', 'repair')),
    changed_symbol_id BIGINT CHECK (changed_symbol_id > 0),
    recommendation_id BIGINT CHECK (recommendation_id > 0),
    generated_test_id BIGINT CHECK (generated_test_id > 0),
    attempt_number INTEGER NOT NULL DEFAULT 1 CHECK (attempt_number > 0),
    source_sha TEXT NOT NULL,
    target_sha TEXT NOT NULL,
    provider TEXT NOT NULL CHECK (length(provider) BETWEEN 1 AND 64),
    model_name TEXT NOT NULL CHECK (length(model_name) <= 255),
    prompt_version TEXT NOT NULL CHECK (length(prompt_version) BETWEEN 1 AND 128),
    prompt_hash TEXT NOT NULL CHECK (length(prompt_hash) = 64),
    configuration_hash TEXT NOT NULL CHECK (length(configuration_hash) = 64),
    instructions TEXT NOT NULL CHECK (octet_length(instructions) BETWEEN 1 AND 65536),
    prompt_text TEXT NOT NULL CHECK (octet_length(prompt_text) BETWEEN 1 AND 2097152),
    schema_name TEXT NOT NULL CHECK (length(schema_name) BETWEEN 1 AND 128),
    request_schema JSONB NOT NULL CHECK (jsonb_typeof(request_schema) = 'object'),
    provider_response_id TEXT NOT NULL DEFAULT '' CHECK (length(provider_response_id) <= 255),
    response_text TEXT NOT NULL DEFAULT '' CHECK (octet_length(response_text) <= 4194304),
    status TEXT NOT NULL CHECK (status IN ('COMPLETED', 'FAILED', 'INVALID_OUTPUT')),
    error_message TEXT NOT NULL DEFAULT '' CHECK (octet_length(error_message) <= 16384),
    input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    total_tokens INTEGER NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    latency_ms BIGINT NOT NULL CHECK (latency_ms >= 0),
    estimated_cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (estimated_cost_usd >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, project_id),
    FOREIGN KEY (analysis_job_id, project_id)
        REFERENCES analysis_jobs(id, project_id) ON DELETE CASCADE,
    CHECK (
        (phase = 'recommendation' AND changed_symbol_id IS NOT NULL
            AND recommendation_id IS NULL AND generated_test_id IS NULL)
        OR (phase = 'generation' AND changed_symbol_id IS NULL
            AND recommendation_id IS NOT NULL AND generated_test_id IS NULL)
        OR (phase = 'repair' AND changed_symbol_id IS NULL
            AND recommendation_id IS NULL AND generated_test_id IS NOT NULL)
    )
);

CREATE INDEX llm_calls_analysis_created_idx
    ON llm_calls (analysis_job_id, created_at, id);
CREATE INDEX llm_calls_project_created_idx
    ON llm_calls (project_id, created_at, id);

CREATE TABLE context_snapshots (
    id BIGSERIAL PRIMARY KEY,
    llm_call_id BIGINT NOT NULL UNIQUE,
    project_id BIGINT NOT NULL,
    query_text TEXT NOT NULL CHECK (octet_length(query_text) BETWEEN 1 AND 65536),
    retrieval_query JSONB NOT NULL CHECK (jsonb_typeof(retrieval_query) = 'object'),
    retrieval_config JSONB NOT NULL CHECK (jsonb_typeof(retrieval_config) = 'object'),
    index_ref TEXT NOT NULL DEFAULT '',
    index_generation BIGINT NOT NULL DEFAULT 0 CHECK (index_generation >= 0),
    embedding_model TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, project_id),
    FOREIGN KEY (llm_call_id, project_id)
        REFERENCES llm_calls(id, project_id) ON DELETE CASCADE
);

CREATE INDEX context_snapshots_project_created_idx
    ON context_snapshots (project_id, created_at, id);

CREATE TABLE context_snapshot_items (
    id BIGSERIAL PRIMARY KEY,
    context_snapshot_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    knowledge_chunk_id BIGINT NOT NULL CHECK (knowledge_chunk_id > 0),
    chunk_key TEXT NOT NULL,
    file_path TEXT NOT NULL,
    package_name TEXT NOT NULL DEFAULT '',
    symbol_name TEXT NOT NULL DEFAULT '',
    chunk_type TEXT NOT NULL,
    content TEXT NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 1048576),
    content_hash TEXT NOT NULL CHECK (length(content_hash) = 64),
    start_line INTEGER NOT NULL CHECK (start_line > 0),
    end_line INTEGER NOT NULL CHECK (end_line >= start_line),
    embedding_model TEXT NOT NULL,
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (context_snapshot_id, ordinal),
    FOREIGN KEY (context_snapshot_id, project_id)
        REFERENCES context_snapshots(id, project_id) ON DELETE CASCADE
);

CREATE INDEX context_snapshot_items_snapshot_idx
    ON context_snapshot_items (context_snapshot_id, ordinal);

CREATE FUNCTION reject_ai_provenance_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'AI provenance records are immutable';
END;
$$;

CREATE TRIGGER llm_calls_immutable
    BEFORE UPDATE ON llm_calls
    FOR EACH ROW EXECUTE FUNCTION reject_ai_provenance_update();
CREATE TRIGGER context_snapshots_immutable
    BEFORE UPDATE ON context_snapshots
    FOR EACH ROW EXECUTE FUNCTION reject_ai_provenance_update();
CREATE TRIGGER context_snapshot_items_immutable
    BEFORE UPDATE ON context_snapshot_items
    FOR EACH ROW EXECUTE FUNCTION reject_ai_provenance_update();
