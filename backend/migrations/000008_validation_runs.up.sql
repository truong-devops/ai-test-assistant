CREATE TABLE validation_runs (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    generated_test_id BIGINT NOT NULL REFERENCES generated_tests(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    command TEXT NOT NULL CHECK (length(command) BETWEEN 1 AND 1000),
    status TEXT NOT NULL CHECK (status IN ('PASSED', 'FAILED', 'TIMED_OUT')),
    exit_code INTEGER NOT NULL,
    duration_ms BIGINT NOT NULL CHECK (duration_ms >= 0),
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    output_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (generated_test_id, attempt_number)
);

CREATE INDEX validation_runs_analysis_job_id_idx
    ON validation_runs (analysis_job_id, created_at, id);
