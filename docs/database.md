# Database

PostgreSQL stores metadata and the `pgvector` knowledge index.

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
