ALTER TABLE projects
    ADD COLUMN pipeline_mode TEXT NOT NULL DEFAULT 'DOCUMENT_DRIVEN'
        CHECK (pipeline_mode IN ('DOCUMENT_DRIVEN', 'LEGACY'));

-- Existing analyses keep their original code-first semantics. No requirement or
-- citation is fabricated during migration; operators opt each legacy project in
-- after selecting an approved document baseline. Newly created projects use the
-- column default above.
UPDATE projects SET pipeline_mode = 'LEGACY';

ALTER TABLE document_sets
    ADD COLUMN retention_days INTEGER NOT NULL DEFAULT 365 CHECK (retention_days BETWEEN 30 AND 3650),
    ADD COLUMN archived_at TIMESTAMPTZ;

CREATE TABLE document_set_audit_log (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    actor TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('ARCHIVED', 'RESTORED', 'RETENTION_CHANGED')),
    reason TEXT NOT NULL,
    before_state JSONB NOT NULL CHECK (jsonb_typeof(before_state) = 'object'),
    after_state JSONB NOT NULL CHECK (jsonb_typeof(after_state) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX document_set_audit_log_set_idx
    ON document_set_audit_log (document_set_id, created_at DESC, id DESC);

CREATE TABLE project_pipeline_mode_audit (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    actor TEXT NOT NULL,
    previous_mode TEXT NOT NULL CHECK (previous_mode IN ('DOCUMENT_DRIVEN', 'LEGACY')),
    new_mode TEXT NOT NULL CHECK (new_mode IN ('DOCUMENT_DRIVEN', 'LEGACY')),
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX project_pipeline_mode_audit_project_idx
    ON project_pipeline_mode_audit (project_id, created_at DESC, id DESC);
