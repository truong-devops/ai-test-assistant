CREATE TABLE document_workflow_jobs (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    operation TEXT NOT NULL CHECK (operation IN
        ('INDEX_DOCUMENTS','EXTRACT_REQUIREMENTS','GENERATE_TESTCASES')),
    status TEXT NOT NULL DEFAULT 'QUEUED' CHECK (status IN
        ('QUEUED','RUNNING','SUCCEEDED','PARTIAL_FAILED','FAILED','CANCELED')),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    input_snapshot JSONB NOT NULL CHECK (jsonb_typeof(input_snapshot)='object'),
    input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
    requested_by TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    total_units INTEGER NOT NULL DEFAULT 1 CHECK (total_units >= 0),
    completed_units INTEGER NOT NULL DEFAULT 0 CHECK (completed_units >= 0),
    failed_units INTEGER NOT NULL DEFAULT 0 CHECK (failed_units >= 0),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 20),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_expires_at TIMESTAMPTZ,
    heartbeat_at TIMESTAMPTZ,
    cancel_requested_at TIMESTAMPTZ,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    retryable BOOLEAN NOT NULL DEFAULT FALSE,
    output_refs JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(output_refs)='object'),
    delegate_kind TEXT NOT NULL DEFAULT '' CHECK (delegate_kind IN ('','REQUIREMENT_EXTRACTION')),
    delegate_job_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_set_id,idempotency_key),
    CHECK ((delegate_kind='' AND delegate_job_id IS NULL) OR
           (delegate_kind='REQUIREMENT_EXTRACTION' AND delegate_job_id IS NOT NULL))
);

CREATE UNIQUE INDEX document_workflow_jobs_active_operation_idx
    ON document_workflow_jobs(document_set_id,operation)
    WHERE status IN ('QUEUED','RUNNING');
CREATE INDEX document_workflow_jobs_queue_idx
    ON document_workflow_jobs(next_attempt_at,created_at,id)
    WHERE status IN ('QUEUED','RUNNING') AND delegate_kind='';
CREATE INDEX document_workflow_jobs_set_history_idx
    ON document_workflow_jobs(document_set_id,created_at DESC,id DESC);

CREATE TABLE document_workflow_job_units (
    id BIGSERIAL PRIMARY KEY,
    workflow_job_id BIGINT NOT NULL REFERENCES document_workflow_jobs(id) ON DELETE CASCADE,
    unit_key TEXT NOT NULL,
    input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
    status TEXT NOT NULL DEFAULT 'QUEUED' CHECK (status IN
        ('QUEUED','RUNNING','SUCCEEDED','FAILED','CANCELED')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    output_ref JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(output_ref)='object'),
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workflow_job_id,unit_key)
);

ALTER TABLE requirement_extraction_jobs
    DROP CONSTRAINT requirement_extraction_jobs_status_check,
    ADD CONSTRAINT requirement_extraction_jobs_status_check CHECK
        (status IN ('PENDING','RUNNING','COMPLETED','FAILED','CANCELED')),
    ADD COLUMN workflow_job_id BIGINT REFERENCES document_workflow_jobs(id) ON DELETE SET NULL,
    ADD COLUMN workflow_unit_id BIGINT REFERENCES document_workflow_job_units(id) ON DELETE SET NULL;

ALTER TABLE document_workflow_jobs
    ADD CONSTRAINT document_workflow_jobs_delegate_fk
    FOREIGN KEY (delegate_job_id) REFERENCES requirement_extraction_jobs(id) ON DELETE RESTRICT;

ALTER TABLE document_ai_budget_reservations
    ADD COLUMN workflow_job_id BIGINT REFERENCES document_workflow_jobs(id) ON DELETE SET NULL,
    ADD COLUMN workflow_unit_id BIGINT REFERENCES document_workflow_job_units(id) ON DELETE SET NULL,
    ADD COLUMN workflow_attempt INTEGER CHECK (workflow_attempt IS NULL OR workflow_attempt > 0);

CREATE INDEX document_ai_budget_reservations_workflow_idx
    ON document_ai_budget_reservations(workflow_job_id,workflow_unit_id,status);
