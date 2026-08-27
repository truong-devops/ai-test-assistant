CREATE TABLE test_recommendations (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    changed_symbol_id BIGINT NOT NULL REFERENCES changed_symbols(id) ON DELETE CASCADE,
    title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    description TEXT NOT NULL CHECK (length(description) BETWEEN 1 AND 4000),
    priority TEXT NOT NULL CHECK (priority IN ('low', 'medium', 'high')),
    rationale TEXT NOT NULL CHECK (length(rationale) BETWEEN 1 AND 4000),
    scenario TEXT NOT NULL CHECK (length(scenario) BETWEEN 1 AND 4000),
    expected_behavior TEXT NOT NULL CHECK (length(expected_behavior) BETWEEN 1 AND 4000),
    status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'USEFUL', 'PARTIALLY_USEFUL', 'NOT_USEFUL')),
    model_name TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    provider_response_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (analysis_job_id, changed_symbol_id, title)
);

CREATE INDEX test_recommendations_analysis_job_id_idx
    ON test_recommendations (analysis_job_id, created_at, id);
