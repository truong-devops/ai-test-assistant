ALTER TABLE projects
    DROP CONSTRAINT projects_provider_project_id_unique,
    DROP CONSTRAINT projects_provider_check,
    DROP COLUMN provider;

ALTER TABLE projects
    RENAME COLUMN provider_project_id TO gitlab_project_id;

ALTER TABLE projects
    ADD CONSTRAINT projects_gitlab_project_id_key UNIQUE (gitlab_project_id);

