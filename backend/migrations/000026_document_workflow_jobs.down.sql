DROP INDEX IF EXISTS document_ai_budget_reservations_workflow_idx;
ALTER TABLE document_ai_budget_reservations
    DROP COLUMN IF EXISTS workflow_attempt,
    DROP COLUMN IF EXISTS workflow_unit_id,
    DROP COLUMN IF EXISTS workflow_job_id;

ALTER TABLE document_workflow_jobs
    DROP CONSTRAINT IF EXISTS document_workflow_jobs_delegate_fk;

UPDATE requirement_extraction_jobs SET status='FAILED',
    error_message=CASE WHEN error_message='' THEN 'canceled workflow rolled back' ELSE error_message END,
    finished_at=COALESCE(finished_at,NOW())
WHERE status='CANCELED';
ALTER TABLE requirement_extraction_jobs
    DROP CONSTRAINT IF EXISTS requirement_extraction_jobs_status_check,
    DROP COLUMN IF EXISTS workflow_unit_id,
    DROP COLUMN IF EXISTS workflow_job_id,
    ADD CONSTRAINT requirement_extraction_jobs_status_check CHECK
        (status IN ('PENDING','RUNNING','COMPLETED','FAILED'));

DROP TABLE IF EXISTS document_workflow_job_units;
DROP TABLE IF EXISTS document_workflow_jobs;
