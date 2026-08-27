CREATE TABLE projects (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    gitlab_project_id BIGINT NOT NULL UNIQUE,
    repository_url TEXT NOT NULL,
    default_branch TEXT NOT NULL DEFAULT 'main',
    language TEXT NOT NULL DEFAULT 'go',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT projects_language_check CHECK (language = 'go')
);

CREATE TABLE gitlab_connections (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
    base_url TEXT NOT NULL,
    token_encrypted BYTEA NOT NULL,
    webhook_secret_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE analysis_jobs (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    merge_request_iid BIGINT NOT NULL,
    source_sha TEXT NOT NULL,
    target_sha TEXT NOT NULL,
    status TEXT NOT NULL,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX analysis_jobs_project_id_created_at_idx
    ON analysis_jobs (project_id, created_at DESC);

