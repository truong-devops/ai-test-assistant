DROP TABLE IF EXISTS changed_files;
DROP INDEX IF EXISTS analysis_jobs_webhook_uuid_uidx;
ALTER TABLE analysis_jobs
    DROP COLUMN IF EXISTS raw_event,
    DROP COLUMN IF EXISTS webhook_uuid,
    DROP COLUMN IF EXISTS web_url,
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS target_branch,
    DROP COLUMN IF EXISTS source_branch;

