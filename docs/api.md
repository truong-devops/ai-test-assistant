# API

> This page documents the API currently implemented. Document-driven Phases
> 2–10 and the Phase 11 rollout controls run beside the code-first baseline.
> XLSX/Markdown export, approved baseline mapping, document-grounded Go
> automation and typed sandbox execution are implemented. Follow
> [DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md](DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md)
> for the remaining target API.

All responses use JSON. Legacy errors have the shape `{"error":"message"}`.
Workflow command errors additionally return stable `code`, `retryable`,
`request_id`, `blocked_by`, `next_action` and optional structured `details`.

## Authentication and roles

Production endpoints require `Authorization: Bearer <API_AUTH_TOKEN>`. The
trusted frontend/reverse proxy also sends `X-Authenticated-Role` with one of:

- `viewer`: read endpoints;
- `editor`: create/upload/extract/generate mutations;
- `reviewer`: approval, classification, execution, export, repair, lifecycle
  and pipeline-mode decisions;
- `admin`: all operations; physical purge preview/execution requires this role.

`/health`, `/ready`, and SCM webhooks are exempt because webhooks use their own
provider signature/secret. `X-Authenticated-Role` is trusted only after the
service bearer token succeeds. This is service-to-service authorization; an
OIDC-aware reverse proxy remains responsible for authenticating actual users.

## Document-driven sandbox execution

- `GET /api/test-runs/{id}` returns the run, every attempt, Expected/Actual,
  original taxonomy, bounded evidence, source SHA, image digest and environment
  fingerprint.
- `POST /api/test-runs/{id}/execute` with `{"requested_by":"QA"}` queues a
  pending run. Approval of the final required artifact also queues the run
  automatically.
- `POST /api/test-run-items/{id}/classification` accepts `status`,
  `reviewer_name` and mandatory `reason`; the override is append-audited.
- `POST /api/test-run-items/{id}/repair` queues bounded technical repair only
  when the current status is exactly `AUTOMATION_ERROR`.
- `GET /api/test-run-items/{id}/repairs` returns the candidate-level history,
  allowed-change policy, immutable expected hash/assertions, before/after hash,
  model, prompt version, token/cost usage and draft artifact link.

The worker runs the repository baseline before adding each approved artifact.
Only an approved assertion mismatch can become `PRODUCT_FAILED`. Compilation or
test setup failures become `AUTOMATION_ERROR`; runtime/container/dependency
failures become `INFRA_ERROR`; timeout, missing automation and successful runs
remain separate outcomes. Infra errors use the execution retry queue, not LLM
repair.

Repair never accepts `PRODUCT_FAILED`, `INFRA_ERROR`, or `PASSED`. Provider
output that changes expected hash, test-case identity, semantic assertions,
target path, package, or production source becomes `UNREPAIRABLE`. A valid
technical change creates a new `DRAFT` artifact in `WAITING_REVIEW`; it is not
silently approved. Approval appends a new run-item attempt and queues a sandbox
rerun without overwriting the earlier failure or its evidence.

## Phase 12 evidence

### `GET /api/analyses/{id}/evidence`

Returns lightweight AI-call metadata for the review UI: phase, status, model,
prompt/configuration hashes, tokens, latency, estimated cost, and historical
index/chunk counts. Full prompt/source payloads are intentionally omitted.

### `GET /api/analyses/{id}/export`

Downloads an `ai-provenance-v1` JSON bundle containing the analysis plus exact
instructions, prompts, schemas, provider responses, and denormalized context
snapshots. The response uses an attachment filename. Until Phase 18 adds
end-user OIDC, this endpoint must remain behind the authenticated service and
trusted private reverse proxy.

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
- `GET /api/document-metrics` returns aggregate parse, extraction, approval,
  suite/artifact and execution counters for the document-first overview.
- `POST /api/document-sets/{id}/lifecycle` updates `ACTIVE`/`ARCHIVED` and
  retention days (30–3650) plus total token/cost budgets with mandatory
  actor/reason audit. Archive is recoverable and blocks new uploads.
- `GET /api/document-sets/{id}/ai-budget` returns total, used, actively reserved,
  and remaining tokens and micro-USD.
- `GET /api/document-sets/{id}/purge` is admin-only and previews retention
  eligibility, object totals, exact confirmation text, and immutable-reference
  blockers. `POST` to the same route permanently deletes an eligible archived
  set and its document objects while retaining the independent purge audit.
- `POST /api/document-sets/{id}/documents` accepts a multipart upload and
  returns HTTP 202 because parsing is asynchronous.
- `GET /api/document-sets/{id}/documents` lists logical documents with their
  latest version.
- `POST /api/document-sets/{id}/documents/{documentID}/versions` uploads one
  new version to that exact logical document (HTTP 202). Both IDs must belong
  together; otherwise HTTP 404. Filename changes do not change document identity,
  name or type.
- `GET /api/document-sets/{id}/documents/{documentID}/versions` returns
  `{"versions":[...]}` newest first, with the same ownership check.
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
- `new_document=true` (workspace new-file upload): reject a duplicate document
  name with HTTP 409 instead of implicitly appending a version. Omission retains
  legacy name-based behavior; new clients should use explicit version routes;
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
XLSX import is not supported in Phase 2; XLSX report export is implemented as
an output-only Phase 6 capability.

## Unified document workflow (workflow/versioning UV-04)

- `GET /api/document-sets/{id}/workflow` returns source revision, five workflow
  steps, role-derived capabilities, machine-readable blockers, active/recent
  jobs and `next_action`.
- `POST /api/document-sets/{id}/workflow-operations` accepts
  `INDEX_DOCUMENTS`, `EXTRACT_REQUIREMENTS`, or `GENERATE_TESTCASES`. It requires
  `Idempotency-Key`, freezes the effective input and returns HTTP 202 with
  `job`, `status_url`, `Location`, `ETag`, and `Retry-After`.
- `GET /api/document-workflow-jobs/{id}` returns durable progress, unit counts,
  attempts, error/retry state, output references and attributed token/cost use.
- `POST /api/document-workflow-jobs/{id}/retry` and `/cancel` accept
  `expected_revision`; stale concurrent commands return a revision conflict.

The browser polls the returned status URL with backoff and stops at
`SUCCEEDED`, `PARTIAL_FAILED`, `FAILED`, or `CANCELED`. Reusing an idempotency
key with the same command returns the original job even if current source or
budget state has since changed; reusing it for a different command returns a
conflict. Legacy synchronous mutation routes remain available during migration
and advertise deprecation/link headers pointing clients to this API.

### Selected generation input (UV-08 foundation)

`POST /api/document-sets/{id}/workflow-operations` accepts an optional
`requirement_ids` field for `GENERATE_TESTCASES` only:

```json
{"operation":"GENERATE_TESTCASES","requirement_ids":[42,43]}
```

Send `Idempotency-Key` as usual. A nonempty selection must contain 1–100 unique
positive IDs, all owned by this document set, current, approved and unblocked;
the server never silently drops invalid selections. Invalid IDs/duplicates or
more than 100 return 400; unavailable/foreign/unapproved items return
`422 GENERATION_SELECTION_BLOCKED`. Reordered identical selections replay the
same job; changing the selection or switching to all-baseline with the same key
returns 409. Omitted/empty selection retains the legacy all-approved-current scope.

The snapshot stores the explicit selection, exact revision/source metadata and
input hash. The worker validates this snapshot and then loads only its recorded
IDs, not a new live inventory. It preloads all requirements before calling a
provider. A requirement approved outside an explicit selection does not expand
the job. New drafts include server-built generation provider/model/prompt-version,
prompt/response hashes and workflow input provenance; the existing AI-call log
retains the actual prompt/schema/response. Model output cannot set this provenance.

This is a foundation, not the full regeneration proposal API: generation still
saves drafts through the existing service. Affected-case scope, classifications,
review/apply endpoints and the scope UI are pending. No new migration is required.

### Source review and guided workspace (UV-05)

`GET /api/document-sets/{id}/workflow` also returns `source_intents` (latest 20)
and `can_upload`/`can_manage` capabilities. The frontend maps the five technical
steps into four business steps; reads and browser polling never start AI work.
Every upload persists an `INDEX` intent atomically with its version. The worker
waits for parsing and prepares the index automatically, without approving sources.

`POST /api/document-sets/{id}/source-review` requires `Idempotency-Key` and returns
HTTP 202 with `{"intent":{...}}`. Example:

```json
{
  "command": "APPROVE_AND_EXTRACT",
  "expected_source_revision": 7,
  "selected": [{"version_id": 42, "sha256": "<exact SHA-256 from version>", "approval_status": "DRAFT"}],
  "excluded_version_ids": [43],
  "reviewer_name": "QA display name"
}
```

- `APPROVE`/`APPROVE_AND_EXTRACT`: reviewer role, nonempty explicit selection
  (at most 100), current latest version/hash/approval status, parsed content.
  Approvals, audit and continuation commit together or all roll back.
- `INDEX`/`EXTRACT`: editor role, no selected approvals. `EXTRACT` requires all
  included latest sources to be parsed and approved. Exclusions are explicit
  latest version IDs (at most 100), never implicit suppression of failed files.
- Authenticated actor is stored in review audit; the display name is only
  contextual information and cannot grant permissions.
- Changed source revision/hash/status returns HTTP 409; unmet source conditions
  return 422; missing permission returns 403. Same key/body replays the original
  intent even after a later upload; different body with that key returns 409.
- Worker persists `WAITING_PARSE → INDEXING → EXTRACTING → SUCCEEDED` (approval
  or index only skips extraction). Failures retain approvals. A source/scope
  change before extraction yields `SUPERSEDED`; an already queued extraction
  keeps its frozen input. Retrying a failed child job reopens its failed intent
  only while its source revision remains current.

The workspace is `/documents/{id}?step=documents|requirements|test-cases|use-export`.
Old index, requirements, testcase and source-version URLs remain readable and
link back to the guided workspace. Apply migration 27 before deploying both API
and worker; neither a frontend-only deployment nor a stopped worker can provide
the automatic continuation.

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

- `POST /api/document-sets/{id}/requirements/extract` validates the ready index,
  enqueues a durable extraction job and returns HTTP 202 with `job` and
  `status_url`.
- `GET /api/document-sets/{id}/requirements/extraction` returns the latest job,
  index generation, chunk progress, created/reused/conflict/open-question
  counts, attempts and terminal error.
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
version is not approved, or a conflict/TBD item has not been explicitly resolved.
Editing title/risk or rejecting with an edit cannot bypass clarification.
Retry is idempotent by source snapshot/version and extraction fingerprint.

### Grouped requirement review and source comparison (UV-06)

Inventory returns only current, unsuperseded revisions; each includes
`review_hash`, `review_blockers` and `source_state`. Direct detail URLs still
read historical revisions. `status=APPROVED` selects only eligible, unblocked
requirements. New extraction output becomes current only after the complete
extraction reconciles against the still-current index/source revision.

`POST /api/document-sets/{id}/requirement-review/bulk-review` requires reviewer
role and `Idempotency-Key` (1–160 bytes), with this body:

```json
{
  "items": [{"id": 123, "expected_hash": "<64-character review_hash>"}],
  "decision": "APPROVED",
  "reviewer_name": "QA display name",
  "comment": "Reviewed against source"
}
```

Accepts 1–100 unique positive IDs, decision `APPROVED`/`REJECTED`, display name
up to 160 bytes and comment up to 4000 bytes. Audit identity comes from the
authenticated server actor, never the display name. HTTP 200 returns
`{"results":[{"id":123,"requirement_id":123,"status":"APPLIED"}]}`; blocked
items have `status: BLOCKED`, `code` (`NOT_FOUND`, `STALE_REVISION` or
`REVIEW_BLOCKED`) and `message`. Foreign-set IDs do not reveal their details.
Each successful decision and its receipt commit atomically. Retry the identical
body/key after transport errors: prior successes replay without extra audits,
remaining items are checked again. Reusing a key with another body/actor gives
409; invalid envelope gives 400. A partial batch is not rolled back as a whole.

`POST /api/document-sets/{id}/requirement-review/clarification` accepts
`{"kind":"QUESTION","id":456,"resolution":"Concrete answer here"}`;
kind is `QUESTION` or `CONFLICT`, trimmed resolution 10–8000 bytes. Requires
reviewer and ownership. The same answer can be replayed; a different answer to
an already closed issue gives 409. Stores trusted-actor audit and updates linked
CONFLICT/TBD revisions to DRAFT only when no open issues remain; never auto-approves.
The reviewer remains responsible for the answer's business correctness.

Viewer-readable GETs under `/api/document-sets/{id}/requirement-review/`:

- `clarification-history` returns `history` with kind, subject ID, actor,
  resolution and timestamp.
- `source-comparisons` returns the latest ten `comparisons`, with old/new source
  snapshot IDs and items containing `classification`, `before_ids`, `after_ids`,
  `reason`, `affected_test_case_ids`. Classifications: ADDED, CHANGED, REMOVED,
  UNCHANGED, AMBIGUOUS. No automatic approval transfer, including UNCHANGED.

Legacy single review also accepts `expected_hash` and optional `Idempotency-Key`.
For compatibility it does not require a hash, but always checks current revision,
evidence and clarification guards. Current UI supplies both. Edits return the
new `requirement.id`; clients must navigate to it. Reviewed requirement content,
steps and evidence cannot be overwritten or extended in place.

Testcase list/detail exposes `needs_source_review` when a linked requirement is
superseded or outside the current source scope. Publishing a new release rejects
blocked requirement links. Existing release replay and run snapshots remain intact.

With `LLM_PROVIDER=disabled`, the worker uses a conservative deterministic draft
extractor suitable for local development. `openai` or `gemini` enables the
strict JSON-schema extractor. LLM calls run under
`REQUIREMENT_EXTRACTION_TIMEOUT`; the browser polls status, so reverse-proxy
HTTP write timeouts no longer turn a long extraction into 502. The job is bound
to the index generation captured at enqueue and fails safely if that generation
changes.

## Business test cases and coverage (Phase 5)

- `POST /api/document-sets/{id}/test-cases/generate` generates from the latest
  approved cited requirement baseline only.
- `POST /api/document-sets/{id}/test-cases/regenerate` repeats generation
  idempotently and reports created/reused/suppressed counts.
- `GET /api/document-sets/{id}/test-cases` returns latest test-case versions and
  the latest execution for that exact revision when one exists. A successor
  never inherits the actual result or status of its predecessor.
- `GET /api/document-sets/{id}/test-case-families` returns stable testcase
  identities with separate latest and latest-approved revisions.
- `GET /api/test-case-families/{id}` and `.../{id}/versions` return one family
  and its immutable revision history.
- `POST /api/test-case-families/{id}/versions` creates a draft revision and
  requires `Idempotency-Key` plus the expected head revision/token.
- `GET /api/test-case-families/{id}/diff?from={revisionId}&to={revisionId}` returns
  a structured field/step/evidence diff.
- `POST /api/test-case-families/{id}/restore` copies an old revision into a new
  draft; `POST .../{id}/archive` archives the identity without deleting history.
- `GET /api/test-cases/{id}` returns steps, requirement links, approved source
  excerpts, and review history.
- `POST /api/test-cases/{id}/review` accepts reviewer, decision, comment and
  optional edited fields; editing appends a version and preserves links.
- `POST /api/test-cases/bulk-review` accepts at most 200 IDs with one reviewer,
  decision and comment.
- `GET /api/document-sets/{id}/coverage` deterministically rebuilds the
  requirement ↔ test-case matrix from database links and returns separate
  `DESIGNED`, `PUBLISHED`, `AUTOMATED`, and `EXECUTED` layers. Every layer
  exposes its numerator/denominator; release layers also expose source snapshot,
  immutable release ID, and release number.

Expected results must equal an approved requirement statement or an explicit
expected result from its approved flow step. Invented LLM expectations are
rejected. Coverage exposes the approved denominator, covered/uncovered,
conflict, TBD, rejected and duplicate counts separately. `baseline_complete`
stays false while any approved requirement is uncovered or any current
conflict/TBD exists—even if the approved-only ratio is 100%.

## Export, execution scope and automation (Phases 6–8)

### Testcase version workspace (UV-07)

- `GET /api/test-case-families/{id}/history?before=0&limit=20` returns `entries`
  containing `revision` and `releases` (`id`, `number`), plus optional
  `next_before`. Cursor is the exclusive version number; descending version/ID
  ordering is stable. Limit is 1–50; release references are the newest 50 per entry.
- `GET /api/test-case-families/{id}/comparison?from=1&to=2&offset=0&limit=20`
  returns revision metadata, `changes`, `total` and optional `next_offset`.
  Both revisions must belong to the family. Changes have deterministic field
  order; ordered collections use one-based paths such as `steps[2]`. Limit is
  1–50 changed entries, not a byte cap on historical fields. Legacy `/diff` and
  `/versions` remain unchanged. Identity-list pagination is currently client-side.
- `GET /api/test-cases/{id}/runs?scope=revision&before=0` returns `runs` and
  optional `next_before` (exclusive run-item ID), 20 entries newest first.
  `scope=family` explicitly includes the identity's other revisions, each with
  its exact testcase ID/version. Default scope does not inherit a predecessor's PASS.
- `POST /api/document-sets/{id}/suite-releases/preview` is reviewer-only and
  accepts the publish selection (`test_suite_id`, exact `revision_ids`, optional
  `source_snapshot_id`, `published_by`, `scope_decision`). It returns a release-shaped
  preview with `preview_hash`, `manifest_hash`, exact items, source snapshot,
  scope status and covered/uncovered counts. It creates no release/receipt and
  needs no idempotency key. Partial-scope reason is mandatory only at publish.
- Publish the reviewed selection at `/suite-releases` with `Idempotency-Key`
  and `expected_preview_hash`. The server revalidates approval, ownership,
  archive/evidence/source blockers, manifest and current coverage in the publish
  transaction. Changed preview yields `409 RELEASE_PREVIEW_CHANGED`; clients
  must preview again. Legacy clients may omit the hash but still receive normal
  publish validation. Existing idempotent/manifest replays do not create a new release.

The new editor uses `/versions` for saving a draft and `/review` separately with
`expected_content_hash`; HTTP audit actor comes from authenticated server context,
not the reviewer display name. Repeating the same current decision does not add
a duplicate review event. Archived families and blocked source requirements cannot
be approved. A changed top-level expected result must equal an approved linked
requirement statement or flow-step expected result; missing grounding returns
`422 TESTCASE_FIELD_INVALID` with `field`, `error`, and `next_action`.
New editor payloads allow at most 200 steps/requirement IDs, 500 citations,
100 assumptions, and 16000 bytes per business field/step value. These size checks
do not invalidate historical reads or restore. Head conflicts preserve the usual
`409 TESTCASE_HEAD_CONFLICT` with current head ID/token; the UI retains local edits
and requires explicit comparison/rebase, never silently overwrites the new head.

- `POST /api/document-sets/{id}/suite-releases` publishes an immutable manifest
  of approved `revision_ids`. It requires `test_suite_id`, `published_by` and an
  `Idempotency-Key`; a partial approved-requirement scope also requires
  `scope_decision`.
- `GET /api/document-sets/{id}/suite-releases` and
  `GET /api/test-suite-releases/{id}` return release metadata, exact revision
  items, source snapshot, coverage decision and manifest hash.

- `POST /api/document-sets/{id}/exports` creates an immutable XLSX or Markdown
  snapshot for a working selection, published `suite_release_id`, or optional
  run. The body requires `test_suite_id`, `format`, and `generated_by`; optional
  `test_case_ids` are only for working/run selection. A release export always
  uses the complete immutable manifest.
- `GET /api/document-sets/{id}/exports` lists artifact metadata and hashes.
- `GET /api/test-exports/{id}/download` returns stored bytes with
  `X-Content-SHA256` and attachment headers.
- `GET|POST /api/projects/{id}/document-baseline` lists/selects a published
  `suite_release_id` used by future webhooks. Document-set/suite fields remain
  in the request as ownership guards and for legacy-client compatibility.
- `GET|POST /api/analyses/{id}/test-scope` returns the immutable baseline
  snapshot, selection reasons/confidence and technical signals, or audits a
  manual testcase include/exclude decision.
- `POST /api/analyses/{id}/automation/generate` accepts one approved
  `test_case_id`. It returns `BLOCKED` instead of inventing an API when Go
  technical context or the LLM provider is unavailable.
- `GET /api/test-cases/{id}/automation` lists versioned artifacts and reviews.
- `POST /api/automation-artifacts/{id}/review` accepts `APPROVED` or `REJECTED`
  with reviewer provenance.

The automation schema is Go-unit-test-only in Phase 8. Business context comes
from the approved baseline snapshot; repository chunks are separately marked
untrusted technical context. Expected-result hashes and production paths cannot
be changed by provider output.

## Projects

- `POST /api/projects` creates a project and returns HTTP 201.
- `GET /api/projects` returns `{"projects": [...]}`.
- `GET /api/projects/{id}` returns a project or HTTP 404.
- `POST /api/projects/{id}/pipeline-mode` accepts `DOCUMENT_DRIVEN` or `LEGACY`
  plus mandatory `actor` and `reason`. Existing projects migrate as `LEGACY`;
  newly created projects default to `DOCUMENT_DRIVEN`.

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

Legacy evaluation/generated-test/legacy-repair compatibility endpoints remain
readable but return `Deprecation: true` and a `Link` header pointing to
`/api/document-sets`. They do not fabricate requirement citations for old data.

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

Both decision routes accept exactly one JSON object. They are legacy-compatible
routes protected by the service token/RBAC layer. The reviewer name defaults to
`local-reviewer` when omitted; the comment is optional:

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
