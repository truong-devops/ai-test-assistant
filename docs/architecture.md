# Architecture

The MVP is a modular monolith with two Go entry points: a synchronous HTTP API
and a background worker. Domain logic lives below `backend/internal`; entry
points only assemble dependencies.

The Phase 1 dependency direction is:

```text
HTTP request -> handler -> project service -> project repository -> PostgreSQL
                     \-> health checker -> PostgreSQL ping
```

Phases 2 through 5 add asynchronous database-backed pipelines:

```text
GitLab webhook -> verify + deduplicate -> PENDING analysis job -> HTTP 202
worker -> claim job -> GitLab MR API + paginated diffs -> changed_files
       -> ANALYZING_CHANGE
change worker -> fetch target/source Go files -> Go AST + diff line mapping
              -> changed_symbols -> RETRIEVING_CONTEXT (Phase 4 handoff)

POST project index -> PENDING index generation -> index worker
index worker -> recursive GitLab tree at default branch
             -> path/content security filters -> semantic Go/docs chunks
             -> content-hash update + pgvector -> READY
retriever -> mandatory project filter + structural + full-text + cosine scores

recommendation worker -> claim RETRIEVING_CONTEXT as RECOMMENDING_TESTS
                      -> retrieve context per changed symbol
                      -> render recommend-test-v1 prompt
                      -> LLM provider + strict JSON schema validation
                      -> transactional test_recommendations
                      -> GENERATING_TESTS (Phase 6 handoff)
```

Worker claims use a bounded lease. Temporary GitLab/preparation failures are
retried with a delay up to `WORKER_MAX_ATTEMPTS`; an expired lease makes a job
recoverable after a worker crash. Retry counts reset at each phase handoff, so a
temporary source-fetch failure does not consume the analyzer retry budget. AI
repair attempts in later phases remain a separate counter.

The analyzer parses both target and source versions of modified or renamed Go
files. This lets it detect fully deleted symbols as well as additions and body
changes. Non-Go files are retained in `changed_files` but skipped by the AST
analyzer. Index requests carry a generation number so a newer request cannot be
overwritten by a stale worker. Unchanged content hashes retain existing vectors;
removed chunks are deleted transactionally. The local hash embedding provider
is deterministic for development and tests, while the embedding boundary can
be replaced by a remote model later. Phase 5 keeps vendor details behind the
`llm.Provider` boundary and ships one real OpenAI Responses API provider. It
sends only the compact diff and project-filtered retrieved chunks, uses a
versioned prompt, and rejects malformed or oversized output before persistence.
The provider is disabled unless explicitly configured. Future phases add test
generation, validation, repair, and review modules behind interfaces. Generated
code must only run in an isolated sandbox, never in the API or worker process.

## Decisions

- Standard `net/http` routing keeps the initial API small.
- `pgx` provides PostgreSQL pooling and context-aware operations.
- Numeric database IDs avoid introducing identity infrastructure before auth.
- Migrations are explicit and run before the API starts in Docker Compose.
- Hybrid retrieval always applies `project_id` inside the database query.
- Recommendation writes verify that every changed symbol belongs to the same
  analysis and commit recommendations with the state transition atomically.
- The sample project is isolated in its own Go module so it can later be
  checked out and analyzed as a target repository.
