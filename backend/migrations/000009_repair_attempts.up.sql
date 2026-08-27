CREATE TABLE repair_attempts (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    generated_test_id BIGINT NOT NULL REFERENCES generated_tests(id) ON DELETE CASCADE,
    validation_run_id BIGINT NOT NULL REFERENCES validation_runs(id) ON DELETE CASCADE,
    repaired_generated_test_id BIGINT NOT NULL REFERENCES generated_tests(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL CHECK (attempt_number BETWEEN 1 AND 3),
    previous_code TEXT NOT NULL CHECK (octet_length(previous_code) BETWEEN 1 AND 524288),
    repaired_code TEXT NOT NULL CHECK (octet_length(repaired_code) BETWEEN 1 AND 524288),
    previous_code_hash TEXT NOT NULL CHECK (length(previous_code_hash) = 64),
    repaired_code_hash TEXT NOT NULL CHECK (length(repaired_code_hash) = 64),
    model_name TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    provider_response_id TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 8000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (generated_test_id, attempt_number),
    UNIQUE (validation_run_id),
    UNIQUE (repaired_generated_test_id),
    CHECK (generated_test_id <> repaired_generated_test_id),
    CHECK (previous_code_hash <> repaired_code_hash)
);

CREATE INDEX repair_attempts_analysis_job_id_idx
    ON repair_attempts (analysis_job_id, created_at, id);
