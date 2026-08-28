ALTER TABLE projects
    RENAME COLUMN gitlab_project_id TO provider_project_id;

ALTER TABLE projects
    ADD COLUMN provider TEXT NOT NULL DEFAULT 'gitlab';

ALTER TABLE projects
    DROP CONSTRAINT projects_gitlab_project_id_key;

ALTER TABLE projects
    ADD CONSTRAINT projects_provider_check CHECK (provider IN ('gitlab', 'github')),
    ADD CONSTRAINT projects_provider_project_id_unique UNIQUE (provider, provider_project_id);

