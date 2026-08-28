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

### Review context

`GET /api/analyses/{id}/context` returns the current project-filtered retrieval
for the analysis's changed symbols. Each chunk includes its source path,
line range, kind, content, and retrieval score:

```json
{
  "context": [
    {
      "id": 42,
      "project_id": 3,
      "file_path": "internal/user/service.go",
      "symbol_name": "CreateUser",
      "chunk_type": "implementation",
      "content": "func (s *Service) CreateUser(...) ...",
      "start_line": 18,
      "end_line": 61,
      "score": 13.4
    }
  ]
}
```

This read endpoint does not invoke an LLM or modify the index. It intentionally
shows the current indexed evidence rather than claiming to be an immutable
historical prompt snapshot.

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

## Generated tests

`GET /api/analyses/{id}/generated-tests` returns syntax-checked candidate test
files, or HTTP 404 when the analysis does not exist:

```json
{
  "generated_tests": [
    {
      "id": 8,
      "analysis_job_id": 3,
      "recommendation_id": 7,
      "file_path": "internal/user/service_generated_test.go",
      "test_names": ["TestService_CreateUser_DuplicateEmail"],
      "code": "package user\n...",
      "code_hash": "sha256...",
      "model_name": "configured-model",
      "prompt_version": "generate-test-v1",
      "generation_attempt": 1
    }
  ]
}
```

This endpoint does not execute code or write to the target GitLab repository.

## Validation runs

`GET /api/analyses/{id}/validations` returns the persisted Phase 7 sandbox
results, or HTTP 404 when the analysis does not exist:

```json
{
  "validation_runs": [
    {
      "id": 9,
      "analysis_job_id": 3,
      "generated_test_id": 8,
      "attempt_number": 1,
      "command": "go test -count=1 -timeout=55s ./...",
      "status": "PASSED",
      "exit_code": 0,
      "duration_ms": 2140,
      "stdout": "ok ...",
      "stderr": "",
      "output_truncated": false
    }
  ]
}
```

The HTTP API only reads stored results. Generated code is executed by the
background worker inside the isolated Docker sandbox.

## Repair attempts

`GET /api/analyses/{id}/repairs` returns the auditable Phase 8 repair history,
or HTTP 404 when the analysis does not exist. Each item contains the failed
generated-test ID, validation-run ID, repaired generated-test ID, repair number,
previous/repaired source and hashes, model/prompt trace, and repair reason.

The endpoint never invokes the LLM. Repairs run asynchronously, append a new
generated-test version, and are bounded by `MAX_REPAIR_ATTEMPTS`.

## Human reviews

- `GET /api/analyses/{id}/reviews` returns persisted human decisions for an
  analysis.
- `POST /api/generated-tests/{id}/accept` stores an `ACCEPTED` decision.
- `POST /api/generated-tests/{id}/reject` stores a `REJECTED` decision.

Both decision routes accept exactly one JSON object. The reviewer name is
optional in this unauthenticated MVP and defaults to `local-reviewer`; the
comment is optional:

```json
{
  "reviewer_name": "mai.nguyen",
  "comment": "Covers the duplicate-email branch and follows the existing table-test style."
}
```

Decisions are immutable. The API returns HTTP 409 if the analysis is not in
`WAITING_REVIEW`, the generated test has been superseded by a repair, or that
candidate was already reviewed. For analyses with multiple recommendations, a
decision is recorded per latest candidate. The analysis is terminal only after
all current candidates have a decision: all accepted becomes `ACCEPTED`; one
or more rejections becomes `REJECTED`.
