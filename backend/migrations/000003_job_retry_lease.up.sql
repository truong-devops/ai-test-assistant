ALTER TABLE analysis_jobs
    ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN lease_expires_at TIMESTAMPTZ;

CREATE INDEX analysis_jobs_claim_idx
    ON analysis_jobs (status, next_attempt_at, created_at);

