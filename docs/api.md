# API

> This page documents the API currently implemented. Document-driven Phases
> 2–5 now run beside the code-first baseline. XLSX/Markdown export and mapping
> approved business cases to PR/MR execution remain planned. Follow
> [DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md](DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md)
> for the remaining target API.

All responses use JSON. Errors have the shape `{"error":"message"}`.

## Phase 12 evidence

### `GET /api/analyses/{id}/evidence`

Returns lightweight AI-call metadata for the review UI: phase, status, model,
prompt/configuration hashes, tokens, latency, estimated cost, and historical
index/chunk counts. Full prompt/source payloads are intentionally omitted.

### `GET /api/analyses/{id}/export`

Downloads an `ai-provenance-v1` JSON bundle containing the analysis plus exact
instructions, prompts, schemas, provider responses, and denormalized context
snapshots. The response uses an attachment filename. Until Phase 18 adds
application authentication/RBAC, this endpoint must remain behind the same
trusted private boundary as the rest of the MVP.

## Phase 13 change impact

### `GET /api/analyses/{id}/impact`

Returns the source-SHA-bound impact run, ranked nodes, and explainable edges.
Every inferred node has one or more reason codes: `CALLER`, `CALLEE`,
`INTERFACE_IMPLEMENTATION`, `TYPE_USAGE`, or `EXISTING_TEST`. The run reports
`SSA` or `AST_FALLBACK`, the pinned algorithm, traversal limits, and any
fallback reason.

## Evaluations

- `GET /api/evaluations` returns `{"evaluation_runs": [...]}` ordered newest first.
- `GET /api/evaluations/{id}` returns the immutable run metadata plus its rebuilt
  group summaries and paired comparisons.

The detail endpoint recomputes the report from stored observations and rejects
a stored dataset whose SHA-256 no longer matches the run. Import is deliberately
performed by the offline `cmd/evaluate` CLI rather than an unauthenticated HTTP
write endpoint.

## Health

- `GET /health` returns liveness without checking dependencies.
- `GET /ready` checks PostgreSQL and returns HTTP 503 when unavailable.

## Document intake (document-driven Phase 2)

- `POST /api/document-sets` creates a source boundary and returns HTTP 201.
- `GET /api/document-sets` returns `{"document_sets":[...]}`.
- `GET /api/document-sets/{id}` returns one set or HTTP 404.
- `POST /api/document-sets/{id}/documents` accepts a multipart upload and
  returns HTTP 202 because parsing is asynchronous.
- `GET /api/document-sets/{id}/documents` lists logical documents with their
  latest version.
- `GET /api/documents/{id}/versions/{version}` returns document metadata,
  immutable version metadata, and ordered parsed blocks.

Create-set requests accept one strict JSON object:

```json
{
  "name": "Sàn TMĐT v1.0",
  "product_name": "Sàn thương mại điện tử",
  "scope": "UC-B08 - Đặt đơn hàng",
  "description": "PTYC và URD dùng cho baseline kiểm thử"
}
```

The upload body uses `multipart/form-data` with:

- `file` (required): `.docx`, `.md`, or `.markdown`;
- `document_name` (optional): defaults to the filename without its extension;
- `document_type` (optional): `REQUIREMENTS`, `USER_STORY`, `SYSTEM_DESIGN`,
  `DATABASE_DESIGN`, `API_CONTRACT`, `BUG_HISTORY`, `TEST_REFERENCE`, or
  `OTHER`; defaults to `REQUIREMENTS`.

Every upload starts with `approval_status=DRAFT`. The API derives the accepted
media type from the allow-listed extension, computes SHA-256 while streaming to
the shared file store, and never returns the internal storage key. The default
file limit is 16 MiB and is controlled by `DOCUMENT_MAX_UPLOAD_BYTES`.

The worker transitions parse state through `UPLOADED` → `PARSING` → `PARSED`
or `FAILED`. Parsed blocks preserve order, block type, content and a locator
such as `line:18`, `lines:20-25`, `word/body/p[12]`, or
`word/body/table[4]`. A parser failure leaves the original file/version intact.
XLSX import is not supported in Phase 2; XLSX report export is planned for
Phase 6.

## Document index and source review (Phase 3)

- `POST /api/document-versions/{id}/review` accepts `reviewer_name`, decision
  `APPROVED`/`REJECTED`, and `comment`.
- `POST /api/document-sets/{id}/index` builds or incrementally reuses the index.
- `GET /api/document-sets/{id}/index` returns status, generation, file/chunk/
  skipped/warning counts and embedding model.
- `GET /api/document-sets/{id}/chunks?type=&flow_type=&limit=` returns bounded
  inspector data. The maximum limit is 500.
- `POST /api/document-sets/{id}/retrieve` accepts `query`, optional
  `identifier`, `chunk_type`, `flow_type`, `limit`, and version policy `LATEST`,
  `LATEST_APPROVED`, or `ALL_INDEXED`.

Retrieval returns `{"results":[...]}`. Each result includes immutable source
identity, raw and normalized content, source locator, approval state, and exact,
lexical, semantic, authority and total scores. SQL always applies the document
set and version policy; callers cannot override ownership through the body.

Indexing is idempotent for the same version checksums, embedding model and
semantic-chunker version. Draft versions remain searchable under policies that
allow them but increase `warning_count`; approved sources rank higher. Document
text is untrusted prompt data and likely prompt-injection strings are flagged in
chunk metadata. Approval is blocked until parsing succeeds, and an approved
source cannot be revoked while an approved requirement depends on its evidence.

## Requirement inventory and review (Phase 4)

- `POST /api/document-sets/{id}/requirements/extract` extracts every semantic
  unit and returns created/reused/conflict/open-question counts.
- `GET /api/document-sets/{id}/requirements` accepts filters `document_id`,
  `type`, `status`, `risk`, `actor`, and `flow_type`.
- `GET /api/requirements/{id}` returns the requirement, evidence excerpts,
  ordered flow steps and review history.
- `POST /api/requirements/{id}/review` accepts reviewer, decision, comment and
  optional edited business fields. An edit appends a new version.
- `GET /api/document-sets/{id}/requirement-conflicts` returns source pairs.
- `GET /api/document-sets/{id}/open-questions` returns TBD questions for PO/BA.

Extraction requires a ready index. Every saved requirement is linked by the
service to the actual retrieved block/version; model-supplied citation IDs are
never trusted. Approval returns HTTP 409 if evidence is missing, its source
version is not approved, or a conflict/TBD item is accepted without an explicit
edit. Retry is idempotent by source and extraction fingerprint.

With `LLM_PROVIDER=disabled`, the API uses a conservative deterministic draft
extractor suitable for local development. `openai` or `gemini` enables the
strict JSON-schema extractor. The explicit extraction action currently waits
for completion and is bounded by HTTP/provider timeouts; it is not a queue-status
endpoint.

## Business test cases and coverage (Phase 5)

- `POST /api/document-sets/{id}/test-cases/generate` generates from the latest
  approved cited requirement baseline only.
- `POST /api/document-sets/{id}/test-cases/regenerate` repeats generation
  idempotently and reports created/reused/suppressed counts.
- `GET /api/document-sets/{id}/test-cases` returns latest test-case versions.
- `GET /api/test-cases/{id}` returns steps, requirement links, approved source
  excerpts, and review history.
- `POST /api/test-cases/{id}/review` accepts reviewer, decision, comment and
  optional edited fields; editing appends a version and preserves links.
- `POST /api/test-cases/bulk-review` accepts at most 200 IDs with one reviewer,
  decision and comment.
- `GET /api/document-sets/{id}/coverage` deterministically rebuilds the
  requirement ↔ test-case matrix from database links.

Expected results must equal an approved requirement statement or an explicit
expected result from its approved flow step. Invented LLM expectations are
rejected. Coverage exposes the approved denominator, covered/uncovered,
conflict, TBD, rejected and duplicate counts separately. `baseline_complete`
stays false while any approved requirement is uncovered or any current
conflict/TBD exists—even if the approved-only ratio is 100%.

## Projects

- `POST /api/projects` creates a project and returns HTTP 201.
- `GET /api/projects` returns `{"projects": [...]}`.
- `GET /api/projects/{id}` returns a project or HTTP 404.

Create request:

```json
{
  "name": "sample",
  "provider": "gitlab",
  "provider_project_id": 123,
  "repository_url": "https://gitlab.com/example/sample.git",
  "default_branch": "main",
  "language": "go"
}
```

`provider` accepts `gitlab` or `github`. For GitHub, `repository_url` must be an
`https://github.com/owner/repository` URL and `provider_project_id` is the
numeric repository ID returned by GitHub. The legacy `gitlab_project_id` input
is still accepted when `provider` is omitted.

When `provider_project_id` is omitted, the API resolves repository metadata
through the configured provider client. `provider` is inferred as `github` for
`github.com` URLs and otherwise defaults to `gitlab`; missing `name` and
`default_branch` are filled from the provider response. Private repositories
require the corresponding server-side token.

## GitLab webhook

`POST /api/webhooks/gitlab` accepts `Merge Request Hook` requests for `open`,
`reopen`, and `update`. Requests must include `X-Gitlab-Token` and
`X-Gitlab-Webhook-UUID`. Repeated deliveries with the same UUID return the
existing analysis job instead of creating another one.

## GitHub webhook

`POST /api/webhooks/github` accepts `pull_request` events for `opened`,
`reopened`, and `synchronize`. Requests must include `X-GitHub-Delivery` and a
valid HMAC-SHA256 `X-Hub-Signature-256` generated with
`GITHUB_WEBHOOK_SECRET`. Delivery IDs are namespaced before deduplication.

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
