# API

All responses use JSON. Errors have the shape `{"error":"message"}`.

## Health

- `GET /health` returns liveness without checking dependencies.
- `GET /ready` checks PostgreSQL and returns HTTP 503 when unavailable.

## Projects

- `POST /api/projects` creates a project and returns HTTP 201.
- `GET /api/projects` returns `{"projects": [...]}`.
- `GET /api/projects/{id}` returns a project or HTTP 404.

Create request:

```json
{
  "name": "sample",
  "gitlab_project_id": 123,
  "repository_url": "https://gitlab.com/example/sample.git",
  "default_branch": "main",
  "language": "go"
}
```

## GitLab webhook

`POST /api/webhooks/gitlab` accepts `Merge Request Hook` requests for `open`,
`reopen`, and `update`. Requests must include `X-Gitlab-Token` and
`X-Gitlab-Webhook-UUID`. Repeated deliveries with the same UUID return the
existing analysis job instead of creating another one.

## Analyses

- `GET /api/analyses` lists analysis jobs.
- `GET /api/analyses/{id}` returns metadata, normalized changed files, and
  deterministic changed-symbol records.
- `GET /api/analyses/{id}/changes` returns `changed_files` and
  `changed_symbols`.

Each changed symbol includes its file ID, name, kind, optional method receiver,
package, line range, change type (`added`, `modified`, or `deleted`), and a
compact summary. Jobs successfully analyzed in Phase 3 have status
`RETRIEVING_CONTEXT`.
