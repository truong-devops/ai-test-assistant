CREATE TABLE requirement_extraction_jobs (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    index_generation INTEGER NOT NULL CHECK (index_generation > 0),
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK
        (status IN ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED')),
    total_chunks INTEGER NOT NULL CHECK (total_chunks >= 0),
    processed_chunks INTEGER NOT NULL DEFAULT 0 CHECK
        (processed_chunks >= 0 AND processed_chunks <= total_chunks),
    created_count INTEGER NOT NULL DEFAULT 0 CHECK (created_count >= 0),
    reused_count INTEGER NOT NULL DEFAULT 0 CHECK (reused_count >= 0),
    conflict_count INTEGER NOT NULL DEFAULT 0 CHECK (conflict_count >= 0),
    open_question_count INTEGER NOT NULL DEFAULT 0 CHECK (open_question_count >= 0),
    requested_by TEXT NOT NULL DEFAULT 'USER',
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_expires_at TIMESTAMPTZ,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX requirement_extraction_jobs_active_set_idx
    ON requirement_extraction_jobs (document_set_id)
    WHERE status IN ('PENDING', 'RUNNING');

CREATE INDEX requirement_extraction_jobs_queue_idx
    ON requirement_extraction_jobs (next_attempt_at, created_at, id)
    WHERE status IN ('PENDING', 'RUNNING');

CREATE INDEX requirement_extraction_jobs_set_history_idx
    ON requirement_extraction_jobs (document_set_id, created_at DESC, id DESC);
