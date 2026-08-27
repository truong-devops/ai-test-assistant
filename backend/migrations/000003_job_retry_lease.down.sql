DROP INDEX IF EXISTS analysis_jobs_claim_idx;
ALTER TABLE analysis_jobs
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS next_attempt_at,
    DROP COLUMN IF EXISTS attempt_count;

