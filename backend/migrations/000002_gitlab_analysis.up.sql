ALTER TABLE analysis_jobs
    ADD COLUMN source_branch TEXT NOT NULL DEFAULT '',
    ADD COLUMN target_branch TEXT NOT NULL DEFAULT '',
    ADD COLUMN title TEXT NOT NULL DEFAULT '',
    ADD COLUMN web_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN webhook_uuid TEXT NOT NULL DEFAULT '',
    ADD COLUMN raw_event JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE UNIQUE INDEX analysis_jobs_webhook_uuid_uidx
    ON analysis_jobs (webhook_uuid)
    WHERE webhook_uuid <> '';

CREATE TABLE changed_files (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    old_path TEXT NOT NULL,
    new_path TEXT NOT NULL,
    change_type TEXT NOT NULL,
    additions INTEGER NOT NULL DEFAULT 0,
    deletions INTEGER NOT NULL DEFAULT 0,
    diff TEXT NOT NULL DEFAULT '',
    new_file BOOLEAN NOT NULL DEFAULT FALSE,
    renamed_file BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_file BOOLEAN NOT NULL DEFAULT FALSE,
    collapsed BOOLEAN NOT NULL DEFAULT FALSE,
    too_large BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX changed_files_analysis_job_id_idx ON changed_files (analysis_job_id);

