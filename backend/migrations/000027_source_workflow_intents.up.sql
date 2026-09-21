CREATE TABLE document_source_intents (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    source_revision BIGINT NOT NULL,
    command TEXT NOT NULL CHECK(command IN ('INDEX','APPROVE','APPROVE_AND_EXTRACT','EXTRACT')),
    request JSONB NOT NULL,
    idempotency_key TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'WAITING_PARSE' CHECK(status IN
        ('WAITING_PARSE','INDEXING','EXTRACTING','SUCCEEDED','FAILED','SUPERSEDED')),
    index_job_id BIGINT REFERENCES document_workflow_jobs(id) ON DELETE SET NULL,
    extraction_job_id BIGINT REFERENCES document_workflow_jobs(id) ON DELETE SET NULL,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(document_set_id,idempotency_key)
);
CREATE INDEX document_source_intents_pending_idx ON document_source_intents(id)
    WHERE status IN ('WAITING_PARSE','INDEXING','EXTRACTING');
