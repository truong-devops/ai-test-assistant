CREATE TABLE automation_repair_jobs (
    id BIGSERIAL PRIMARY KEY,
    test_run_item_id BIGINT NOT NULL REFERENCES test_run_items(id) ON DELETE CASCADE,
    source_artifact_id BIGINT NOT NULL REFERENCES automation_artifacts(id) ON DELETE RESTRICT,
    repaired_artifact_id BIGINT REFERENCES automation_artifacts(id) ON DELETE SET NULL,
    attempt_number INTEGER NOT NULL CHECK (attempt_number BETWEEN 1 AND 3),
    status TEXT NOT NULL CHECK (status IN
        ('PENDING', 'RUNNING', 'WAITING_REVIEW', 'APPROVED', 'REJECTED',
         'UNREPAIRABLE', 'FAILED')),
    error_type TEXT NOT NULL CHECK (error_type = 'AUTOMATION_ERROR'),
    reason TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    allowed_change_policy JSONB NOT NULL CHECK (jsonb_typeof(allowed_change_policy) = 'object'),
    expected_result_hash TEXT NOT NULL CHECK (expected_result_hash ~ '^[0-9a-f]{64}$'),
    before_source_hash TEXT NOT NULL CHECK (before_source_hash ~ '^[0-9a-f]{64}$'),
    after_source_hash TEXT CHECK (after_source_hash IS NULL OR after_source_hash ~ '^[0-9a-f]{64}$'),
    before_assertions JSONB NOT NULL CHECK (jsonb_typeof(before_assertions) = 'array'),
    after_assertions JSONB CHECK (after_assertions IS NULL OR jsonb_typeof(after_assertions) = 'array'),
    model_name TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT 'document-automation-repair-v1',
    provider_response_id TEXT NOT NULL DEFAULT '',
    prompt_text TEXT NOT NULL DEFAULT '',
    response_text TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    max_output_tokens INTEGER NOT NULL CHECK (max_output_tokens BETWEEN 100 AND 10000),
    max_cost_microusd BIGINT NOT NULL CHECK (max_cost_microusd >= 0),
    estimated_cost_microusd BIGINT NOT NULL DEFAULT 0 CHECK (estimated_cost_microusd >= 0),
    queue_attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (queue_attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_expires_at TIMESTAMPTZ,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE (test_run_item_id, attempt_number),
    CHECK ((status IN ('WAITING_REVIEW', 'APPROVED', 'REJECTED')) = (repaired_artifact_id IS NOT NULL))
);

CREATE UNIQUE INDEX automation_repair_jobs_active_item_idx
    ON automation_repair_jobs (test_run_item_id)
    WHERE status IN ('PENDING', 'RUNNING', 'WAITING_REVIEW');

CREATE INDEX automation_repair_jobs_queue_idx
    ON automation_repair_jobs (status, next_attempt_at, id)
    WHERE status = 'PENDING';

CREATE INDEX automation_repair_jobs_history_idx
    ON automation_repair_jobs (test_run_item_id, attempt_number DESC, id DESC);

CREATE OR REPLACE FUNCTION prevent_automation_repair_guardrail_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.test_run_item_id <> OLD.test_run_item_id
       OR NEW.source_artifact_id <> OLD.source_artifact_id
       OR NEW.attempt_number <> OLD.attempt_number
       OR NEW.error_type <> OLD.error_type
       OR NEW.allowed_change_policy <> OLD.allowed_change_policy
       OR NEW.expected_result_hash <> OLD.expected_result_hash
       OR NEW.before_source_hash <> OLD.before_source_hash
       OR NEW.before_assertions <> OLD.before_assertions
       OR NEW.max_output_tokens <> OLD.max_output_tokens
       OR NEW.max_cost_microusd <> OLD.max_cost_microusd THEN
        RAISE EXCEPTION 'automation repair business guardrail is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER automation_repair_jobs_guardrail
BEFORE UPDATE ON automation_repair_jobs
FOR EACH ROW EXECUTE FUNCTION prevent_automation_repair_guardrail_update();
