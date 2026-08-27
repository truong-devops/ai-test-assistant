# Database

PostgreSQL stores metadata and the `pgvector` knowledge index.
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

Migrations are in `backend/migrations` and use the `golang-migrate` filename
format. Apply them with `make migrate-up`; roll back one migration with
`make migrate-down`.
