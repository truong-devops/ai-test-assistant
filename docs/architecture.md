# Architecture

The MVP is a modular monolith with two Go entry points: a synchronous HTTP API
and a background worker. Domain logic lives below `backend/internal`; entry
points only assemble dependencies.

The Phase 1 dependency direction is:

```text
HTTP request -> handler -> project service -> project repository -> PostgreSQL
                     \-> health checker -> PostgreSQL ping
```

Phases 2 and 3 add an asynchronous database-backed pipeline:

```text
GitLab webhook -> verify + deduplicate -> PENDING analysis job -> HTTP 202
worker -> claim job -> GitLab MR API + paginated diffs -> changed_files
       -> ANALYZING_CHANGE
change worker -> fetch target/source Go files -> Go AST + diff line mapping
              -> changed_symbols -> RETRIEVING_CONTEXT (Phase 4 handoff)
```

Worker claims use a bounded lease. Temporary GitLab/preparation failures are
retried with a delay up to `WORKER_MAX_ATTEMPTS`; an expired lease makes a job
recoverable after a worker crash. Retry counts reset at each phase handoff, so a
temporary source-fetch failure does not consume the analyzer retry budget. AI
repair attempts in later phases remain a separate counter.

The analyzer parses both target and source versions of modified or renamed Go
files. This lets it detect fully deleted symbols as well as additions and body
changes. Non-Go files are retained in `changed_files` but skipped by the AST
analyzer. Future phases add knowledge/RAG, LLM, validation, repair, and review
modules behind interfaces. Generated code must only run in an isolated sandbox,
never in the API or worker process.

## Decisions

- Standard `net/http` routing keeps the initial API small.
- `pgx` provides PostgreSQL pooling and context-aware operations.
- Numeric database IDs avoid introducing identity infrastructure before auth.
- Migrations are explicit and run before the API starts in Docker Compose.
- The sample project is isolated in its own Go module so it can later be
  checked out and analyzed as a target repository.
