CREATE TABLE generated_tests (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    recommendation_id BIGINT NOT NULL REFERENCES test_recommendations(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL CHECK (
        length(file_path) BETWEEN 1 AND 512
        AND file_path !~ '(^/|(^|/)\.\.?(/|$)|\\)'
        AND file_path ~ '(^|/)[A-Za-z0-9][A-Za-z0-9_]*_test\.go$'
    ),
    test_names JSONB NOT NULL CHECK (jsonb_typeof(test_names) = 'array'),
    code TEXT NOT NULL CHECK (octet_length(code) BETWEEN 1 AND 524288),
    code_hash TEXT NOT NULL CHECK (length(code_hash) = 64),
    model_name TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    provider_response_id TEXT NOT NULL DEFAULT '',
    generation_attempt INTEGER NOT NULL DEFAULT 1 CHECK (generation_attempt > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (recommendation_id, generation_attempt)
);

CREATE INDEX generated_tests_analysis_job_id_idx
    ON generated_tests (analysis_job_id, created_at, id);
