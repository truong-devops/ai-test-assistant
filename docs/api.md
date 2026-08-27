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

## Project knowledge index

- `POST /api/projects/{id}/index` requests an asynchronous index of the
  project's default branch and returns HTTP 202.
- `GET /api/projects/{id}/index/status` returns `NOT_INDEXED`, `PENDING`,
  `INDEXING`, `READY`, or `FAILED`, plus file/chunk counts and retry metadata.

Repeated POST requests create a new index generation. A newer generation safely
invalidates an older worker lease. Repository content and embeddings are not
returned by these endpoints.

## Analyses

- `GET /api/analyses` lists analysis jobs.
- `GET /api/analyses/{id}` returns metadata, normalized changed files, and
  deterministic changed-symbol records.
- `GET /api/analyses/{id}/changes` returns `changed_files` and
  `changed_symbols`.

Each changed symbol includes its file ID, name, kind, optional method receiver,
package, line range, change type (`added`, `modified`, or `deleted`), and a
compact summary. Jobs successfully analyzed in Phase 3 have status
`RETRIEVING_CONTEXT`; the Phase 5 worker advances successfully recommended jobs
to `GENERATING_TESTS`.

## Recommendations

`GET /api/analyses/{id}/recommendations` returns the stored structured
recommendations for an analysis, or HTTP 404 when the analysis does not exist:

```json
{
  "recommendations": [
    {
      "id": 7,
      "analysis_job_id": 3,
      "changed_symbol_id": 5,
      "title": "Duplicate email",
      "description": "Cover the newly added duplicate-email branch.",
      "priority": "high",
      "rationale": "No existing test covers this branch.",
      "scenario": "Repository lookup returns an existing user.",
      "expected_behavior": "CreateUser returns ErrEmailExists and does not call Create.",
      "status": "PENDING",
      "model_name": "configured-model",
      "prompt_version": "recommend-test-v1"
    }
  ]
}
```

An empty array is valid for an analysis with no changed Go symbols. HTTP
handlers never call the LLM; generation happens asynchronously in the worker.
