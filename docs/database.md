# Database

> This page documents the schema currently implemented by migrations 1–26.
> Migration 15 establishes the document-driven foundation beside the legacy
> tables; migration 16 completes persistence and guards for semantic indexing,
> requirement extraction/review and grounded test-case generation/coverage.
> Migration 17 adds immutable exports, execution-scope snapshots and separated
> automation provenance; migrations 18–21 add typed execution, asynchronous
> extraction, guarded repair, lifecycle and rollout controls. Migration 22 adds
> physical-retention purge and document-set AI budgets. Migrations 23–25 add
> exact document-source snapshots, stable testcase families/revisions and
> immutable suite releases pinned by downstream consumers. Migration 26 adds
> the shared durable workflow queue, attempt-level usage attribution and the
> bridge to the existing requirement-extraction scheduler.

PostgreSQL stores metadata and the `pgvector` knowledge index.

## Document-driven foundation (migration 15)

`document_sets` is the ownership boundary for a product/scope. `documents`
represents a logical source, while `document_versions` stores append-only file
identity: version number, filename, allow-listed media type, byte size, SHA-256,
private storage key, approval state, parse queue state and timestamps. File
bytes live in the shared local/object-store abstraction, not in PostgreSQL.

`document_blocks` stores the ordered parser result (`HEADING`, `PARAGRAPH`,
`LIST`, `TABLE`, or `CODE`) with source locator and structural metadata.
Identity fields on document versions and parsed block/evidence rows reject
in-place updates. Set/version composite foreign keys prevent evidence from
crossing document-set boundaries.

The same migration introduced normalized, ownership-safe domain tables:

- `document_chunks` for set-filtered full-text/vector retrieval;
- `requirements`, `requirement_evidence`, `requirement_conflicts`,
  `open_questions`, and `requirement_reviews`;
- `test_suites`, versioned `test_cases`, ordered `test_case_steps`, deterministic
  requirement links, and `test_case_reviews`;
- versioned `automation_artifacts` separated from business test cases;
- `test_runs`, `test_run_items`, and typed `test_run_evidence`.

An automation artifact can only reference an approved test case and must copy
the exact expected-result hash. Once a run item exists, its test-case reference,
expected-result snapshot and expected hash cannot be updated.

## Document RAG, requirements and test cases (migration 16)

`document_index_status` tracks a set-owned index generation, input fingerprint,
embedding model and file/chunk/warning counts. `document_chunks` now retains the
source block, parent/flow metadata and raw content beside normalized embedded
content. `document_chunk_source_blocks` supports traceability when a semantic
unit spans multiple parser blocks.

`document_context_snapshots` and `document_context_snapshot_items` store an
append-only retrieval result with query/configuration, generation, model,
content and scores. Re-indexing may replace live chunks while snapshot items
remain historical. `document_ai_calls` stores instructions, prompt, strict
schema, raw provider response, status, usage and latency for requirement and
test-case phases. Update triggers make all three provenance tables immutable.

Requirements gain extraction/source fingerprints, risk, assumptions and raw
validated payload. `requirement_flow_steps` stores ordered business steps.
Unique partial indexes make extraction retries idempotent. Approval triggers
require at least one evidence link whose document version is `APPROVED`; an
approved business version cannot have its meaning edited in place.

Test cases gain generation identity, assumptions and deterministic dedupe
records. Approval requires a link to an approved requirement with evidence.
Approved title/scenario/expected fields are immutable, while review edits append
a superseding version and copy evidence links. Coverage is not stored as an AI
summary: the API rebuilds it from current requirements and test-case links.

## Source snapshots, testcase revisions and suite releases (migrations 23–25)

`document_source_snapshots` and their items freeze the exact document version
membership used by an index/extraction generation. Historical chunks stay
queryable but cannot become subjects of a newer extraction unless they belong
to that generation.

`test_case_families` is the stable scenario identity. Every `test_cases` row is
an immutable revision with canonical content hash, parent/restore provenance,
exact requirement/evidence links and an optimistic head token. Revision command
and audit tables provide idempotency, CAS conflict reporting and actor history;
legacy chains that may contain mixed scenarios are reported for manual review.

`test_suite_releases` freezes one source snapshot and a manifest of
`family → test_case revision`. `test_suite_release_items` duplicates identity,
revision, content and expected-result hashes so DB constraints can reject
cross-set/family references. Published rows are immutable. Project baselines,
analysis snapshots, test runs and exports carry `suite_release_id`; a new draft
or R2 therefore cannot change an R1 analysis/run/export. Migration 25 creates a
clearly labelled `MIGRATED_CURRENT_STATE` release for every legacy project
binding without claiming it was historically published by a user.

## Durable document workflow jobs (migration 26)

`document_workflow_jobs` is the common status and command boundary for index,
requirement extraction and testcase generation. Every job stores a canonical
input snapshot/hash, idempotency key, optimistic revision, unit progress,
attempt/lease/heartbeat state, structured terminal error and output references.
A partial unique index allows only one queued/running operation of each kind per
document set. `document_workflow_job_units` provides the atomic checkpoint used
when an output is published.

Index and testcase generation are claimed directly with `FOR UPDATE SKIP
LOCKED`. Requirement extraction keeps its existing scheduler and is linked as a
delegated job, so migration 26 does not create a competing consumer. Its status
and chunk progress are reconciled into the common workflow read model.

AI budget reservations now identify workflow job, unit and attempt. A worker
restart can therefore retry safely while completed or uncertain provider usage
remains attributable. Running cancellation is cooperative: publication checks
the lease/cancel state, while retry and cancel commands use the job revision to
prevent two attempts from becoming authoritative.

## Export, execution scope and automation (migration 17)

`test_exports` stores XLSX/Markdown bytes with an immutable JSON snapshot,
content hash and snapshot hash. Reports can target a suite/run and retain all
execution rounds on a separate history sheet.

`project_document_baselines` binds a repository to one approved document
set/test suite. On webhook enqueue, `analysis_baseline_snapshots` copies the
approved document versions, requirements and test cases before technical
analysis. `analysis_test_scope_items` stores selection reasons/confidence,
`analysis_scope_signals` records path/module/symbol evidence, and
`analysis_scope_decisions` audits manual changes. Uncertain mapping uses a full
approved-suite fallback instead of silently narrowing coverage.

`automation_generation_calls` stores business and technical context separately.
`automation_artifacts` stores the immutable test-case snapshot, setup/assertions
and separate context hashes. Contract fields cannot be updated; regeneration
creates a new artifact version. `automation_artifact_reviews` records approval.

## Document-driven execution (migration 18)

Migration 18 turns `test_runs` into an independently leased execution queue.
It stores who/when requested execution, retry/lease state, immutable local image
ID, image reference, error and environment fingerprint. `test_run_items` keeps
one immutable expected/artifact snapshot per attempt; an approved artifact may
be bound exactly once while the item is still `NOT_RUN`.

`test_run_evidence` retains bounded/redacted stdout, stderr and artifact or
screenshot references. `test_run_classification_reviews` append-audits every
manual result override with previous/new taxonomy, reviewer and reason. Infra
retry appends a new item attempt instead of overwriting execution history.

## Requirement extraction queue (migration 19)

`requirement_extraction_jobs` moves LLM extraction outside the API request. A
job captures its document-index generation, total/processed chunk counts,
created/reused/conflict/question counts, request actor, lease, retry schedule and
terminal error. A partial unique index permits only one pending/running job per
document set. Claim uses `FOR UPDATE SKIP LOCKED`; generation checks prevent a
job from writing against a newer index.

## Guarded automation repair (migration 20)

`automation_repair_jobs` can reference only one failed `test_run_item` and
records candidate status independently. The repository admits only
`AUTOMATION_ERROR`; product/infra/pass outcomes cannot enter this queue. Each
row keeps its source/repaired artifact, failure evidence link, before/after
hash/assertions, model/prompt, token/cost budget, lease/retry and reviewer state.

A trigger makes the test-run item, source artifact, attempt, allowed-change
policy, expected-result hash, source hash, original assertions and budgets
immutable. Valid provider output creates a new draft artifact; approval/reject
of that artifact advances the corresponding repair job. Approval also appends
a `NOT_RUN` item attempt and requeues its run; repair attempt limits are counted
across the run/test-case chain so a rerun cannot reset the budget.

## Pipeline rollout and document lifecycle (migration 21)

`projects.pipeline_mode` is `DOCUMENT_DRIVEN` or `LEGACY`. Existing projects are
backfilled to `LEGACY` without inventing requirement evidence; new projects use
the document-driven default. `project_pipeline_mode_audit` records every
operator change.

Document sets gain bounded `retention_days` and `archived_at`.
`document_set_audit_log` stores archive/restore/retention changes with actor,
reason and before/after state. Archive is recoverable and prevents uploads;
coordinated backup stores PostgreSQL and document bytes at the same quiesced
application point.

## Retention purge and AI budget (migration 22)

`document_sets` gains `PURGING`, `ai_token_budget`, and
`ai_cost_budget_microusd`. `document_ai_budget_reservations` provides an atomic
ledger: active calls reserve conservative token/cost capacity, completed calls
store actual usage, failed calls release capacity, and expired reservations no
longer consume the limit. Historical document/automation token usage is
backfilled so an upgrade does not silently reset the token total; historical
cost without a persisted rate remains zero rather than being fabricated.

`document_set_purge_audit` intentionally has no foreign key to `document_sets`,
so its actor, reason, confirmation, object count/bytes and final status survive
successful deletion. Purge first locks the archived set, verifies elapsed
retention and rejects project baselines, analysis snapshots or test runs. It
then enters `PURGING`, deletes file objects idempotently, cascades the unpinned
document graph, and marks the independent audit complete. A failed attempt
leaves `PURGING` for a safe retry rather than restoring a partially deleted set.

## Phase 12 AI provenance

`llm_calls` stores every actual recommendation, generation, and repair provider
call, including failed calls and invalid structured output. A phase check ensures
that each row references one historical subject ID. Ownership is validated at
insert time, while the IDs intentionally remain snapshot references so retry or
result replacement cannot cascade-delete evidence.

`context_snapshots` stores the retrieval query, configuration, index ref,
generation, and embedding model. `context_snapshot_items` denormalizes the
retrieved chunk content and content hash. It does not reference live
`knowledge_chunks`, because re-indexing is allowed to replace those rows.

All three tables reject `UPDATE` through a database trigger. They can only be
appended during normal operation. Parent deletion remains available for a future
explicit retention policy.

## Phase 13 impact graph

- `impact_analysis_runs`: one source-SHA-bound analyzer run per analysis,
  including mode, pinned algorithm, traversal limits, package count, and
  fallback reason.
- `impact_nodes`: direct and inferred symbols with file location, depth, score,
  test marker, and reason-code array.
- `impact_edges`: bounded `CALLS`, `IMPLEMENTS`, and `USES_TYPE` relations with
  an explainable reason code and score.

The analyzer replaces the graph and direct `changed_symbols` atomically while
the analysis owns its worker lease. Composite edge foreign keys prevent a node
from another run/project being attached to the graph.
Phase 1 defines `projects`, `gitlab_connections`, and `analysis_jobs`. Phase 2
adds auditable webhook metadata to analysis jobs plus normalized `changed_files`
with raw unified diff text, size flags, and addition/deletion counts. Phase 3
adds `changed_symbols`, linked to `changed_files` with cascade deletion. It stores
symbol kind, method receiver, package, line range, change type, and deterministic
summary metadata.

Phase 4 enables the `vector` extension and adds:

- `project_indexes`: asynchronous request status, generation, retry lease, and
  file/chunk counts.
- `knowledge_chunks`: project-owned semantic chunks, content hashes, metadata,
  generated full-text vectors, and 384-dimensional embeddings.

`knowledge_chunks` has GIN full-text and HNSW cosine indexes. The unique
`(project_id, chunk_key)` key supports incremental updates without duplicating
unchanged content, and project deletion cascades through both Phase 4 tables.

Phase 5 adds `test_recommendations`. Each record belongs to an analysis and a
changed symbol and stores the validated title, description, priority, rationale,
scenario, expected behavior, human quality status, model name, prompt version,
and provider response ID. The repository verifies symbol ownership during the
insert and commits the records together with the analysis transition from
`RECOMMENDING_TESTS` to `GENERATING_TESTS`.

Phase 6 adds `generated_tests`. Each candidate belongs to both an analysis and a
recommendation and stores its safe relative `_test.go` path, declared test names,
complete source, SHA-256 code hash, model, versioned prompt, provider response
ID, and generation attempt. A unique `(recommendation_id, generation_attempt)`
constraint prevents duplicate initial generations. Candidate writes and the
analysis transition to `VALIDATING` share one transaction.

Phase 7 adds `validation_runs`. Each record belongs to an analysis and generated
test version and stores the exact command, status (`PASSED`, `FAILED`, or
`TIMED_OUT`), exit code, bounded/redacted stdout and stderr, duration, and an
output-truncation marker. Candidate ownership is checked inside the insert.
Results and the analysis transition to `WAITING_REVIEW` (all pass) or
`REPAIRING` (any failure/timeout) commit atomically.

Phase 8 adds `repair_attempts`. Each row links the failed generated version and
validation run to the repaired generated version. It stores the attempt number,
previous/repaired code and hashes, model, prompt version, provider response ID,
and a bounded reason derived from validation feedback. New generated versions
use increasing `generation_attempt` values. Repair records and the transition
back to `VALIDATING` commit atomically; when no attempt remains, the analysis
instead transitions to `WAITING_REVIEW` with its failed validation history.

Phase 9 adds `test_reviews`. A review belongs to exactly one generated-test
version and records a bounded reviewer name, immutable decision
(`ACCEPTED` or `REJECTED`), optional bounded comment, and timestamp. The
unique generated-test key prevents double-clicks and concurrent review requests
from creating conflicting final decisions. Deleting an analysis cascades through
its generated tests and review records.

Phase 10 adds `evaluation_runs` and `evaluation_observations`. A run stores the
dataset schema, SHA-256 hash, description, count, and creation time. The hash is
unique, making imports idempotent. Observations retain an explicit ordinal and
raw validated JSON payload; the ordinal preserves exact dataset order so a
report rebuilt from PostgreSQL must reproduce the stored hash.

Migration 12 generalizes projects from `gitlab_project_id` to the unique pair
`(provider, provider_project_id)`. Existing rows become `gitlab`; new rows may
use `gitlab` or `github` without changing downstream project ownership.

Migrations are in `backend/migrations` and use the `golang-migrate` filename
format. Apply them with `make migrate-up`; roll back one migration with
`make migrate-down`.
