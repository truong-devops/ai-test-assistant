DROP TABLE IF EXISTS project_pipeline_mode_audit;
DROP TABLE IF EXISTS document_set_audit_log;
ALTER TABLE document_sets DROP COLUMN IF EXISTS archived_at, DROP COLUMN IF EXISTS retention_days;
ALTER TABLE projects DROP COLUMN IF EXISTS pipeline_mode;
