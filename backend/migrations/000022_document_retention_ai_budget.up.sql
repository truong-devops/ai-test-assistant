ALTER TABLE document_sets
    DROP CONSTRAINT document_sets_status_check,
    ADD CONSTRAINT document_sets_status_check
        CHECK (status IN ('ACTIVE', 'ARCHIVED', 'PURGING')),
    ADD COLUMN ai_token_budget BIGINT NOT NULL DEFAULT 1000000
        CHECK (ai_token_budget BETWEEN 1000 AND 1000000000),
    ADD COLUMN ai_cost_budget_microusd BIGINT NOT NULL DEFAULT 10000000
        CHECK (ai_cost_budget_microusd BETWEEN 0 AND 1000000000000);

ALTER TABLE document_set_audit_log
    DROP CONSTRAINT document_set_audit_log_action_check,
    ADD CONSTRAINT document_set_audit_log_action_check CHECK
        (action IN ('ARCHIVED', 'RESTORED', 'RETENTION_CHANGED', 'BUDGET_CHANGED'));

CREATE TABLE document_ai_budget_reservations (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    phase TEXT NOT NULL,
    subject_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'RESERVED'
        CHECK (status IN ('RESERVED', 'COMPLETED', 'RELEASED')),
    reserved_tokens BIGINT NOT NULL CHECK (reserved_tokens > 0),
    reserved_cost_microusd BIGINT NOT NULL CHECK (reserved_cost_microusd >= 0),
    input_tokens BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    actual_cost_microusd BIGINT NOT NULL DEFAULT 0 CHECK (actual_cost_microusd >= 0),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX document_ai_budget_reservations_set_idx
    ON document_ai_budget_reservations (document_set_id, status, expires_at);

-- Historical token usage must not disappear when the hard budget is enabled.
-- Provider prices are runtime configuration, so old calls without a persisted
-- cost are conservatively backfilled with their tokens and zero unknown cost.
INSERT INTO document_ai_budget_reservations
    (document_set_id, phase, subject_key, status, reserved_tokens,
     reserved_cost_microusd, input_tokens, output_tokens, actual_cost_microusd,
     expires_at, created_at, completed_at)
SELECT document_set_id, phase, 'legacy-document-ai-call:' || id, 'COMPLETED',
       GREATEST(input_tokens + output_tokens, 1), 0, input_tokens, output_tokens,
       0, created_at, created_at, created_at
FROM document_ai_calls
WHERE input_tokens + output_tokens > 0;

INSERT INTO document_ai_budget_reservations
    (document_set_id, phase, subject_key, status, reserved_tokens,
     reserved_cost_microusd, input_tokens, output_tokens, actual_cost_microusd,
     expires_at, created_at, completed_at)
SELECT t.document_set_id, 'AUTOMATION_GENERATION',
       'legacy-automation-call:' || c.id, 'COMPLETED',
       GREATEST(c.input_tokens + c.output_tokens, 1), 0,
       c.input_tokens, c.output_tokens, 0, c.created_at, c.created_at, c.created_at
FROM automation_generation_calls c
JOIN test_cases t ON t.id = c.test_case_id
WHERE c.input_tokens + c.output_tokens > 0;

INSERT INTO document_ai_budget_reservations
    (document_set_id, phase, subject_key, status, reserved_tokens,
     reserved_cost_microusd, input_tokens, output_tokens, actual_cost_microusd,
     expires_at, created_at, completed_at)
SELECT t.document_set_id, 'AUTOMATION_REPAIR', 'legacy-repair-job:' || j.id,
       'COMPLETED', GREATEST(j.input_tokens + j.output_tokens, 1),
       j.estimated_cost_microusd, j.input_tokens, j.output_tokens,
       j.estimated_cost_microusd, j.created_at, j.created_at,
       COALESCE(j.finished_at, j.created_at)
FROM automation_repair_jobs j
JOIN automation_artifacts a ON a.id = j.source_artifact_id
JOIN test_cases t ON t.id = a.test_case_id
WHERE j.input_tokens + j.output_tokens > 0;

CREATE TABLE document_set_purge_audit (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL,
    document_set_name TEXT NOT NULL,
    actor TEXT NOT NULL,
    reason TEXT NOT NULL,
    confirmation TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'REQUESTED'
        CHECK (status IN ('REQUESTED', 'COMPLETED', 'FAILED')),
    storage_object_count INTEGER NOT NULL CHECK (storage_object_count >= 0),
    storage_bytes BIGINT NOT NULL CHECK (storage_bytes >= 0),
    database_snapshot JSONB NOT NULL CHECK (jsonb_typeof(database_snapshot) = 'object'),
    error_message TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX document_set_purge_audit_set_idx
    ON document_set_purge_audit (document_set_id, requested_at DESC, id DESC);
