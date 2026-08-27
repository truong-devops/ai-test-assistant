# Architecture

The MVP is a modular monolith with two Go entry points: a synchronous HTTP API
and a background worker. Domain logic lives below `backend/internal`; entry
points only assemble dependencies.

The Phase 1 dependency direction is:

```text
HTTP request -> handler -> project service -> project repository -> PostgreSQL
                     \-> health checker -> PostgreSQL ping
```

Phase 2 adds an asynchronous database-backed queue:

```text
GitLab webhook -> verify + deduplicate -> PENDING analysis job -> HTTP 202
worker -> claim job -> GitLab MR API + paginated diffs -> changed_files
       -> ANALYZING_CHANGE (handoff point for Phase 3)
```

Worker claims use a bounded lease. Temporary GitLab/preparation failures are
retried with a delay up to `WORKER_MAX_ATTEMPTS`; an expired lease makes a job
recoverable after a worker crash. AI repair attempts in later phases remain a
separate counter.

Future phases add the Go symbol analyzer, knowledge/RAG, LLM, validation,
repair, and review modules behind interfaces. Generated code must only run in
an isolated sandbox, never in the API or worker process.

## Decisions

- Standard `net/http` routing keeps the initial API small.
- `pgx` provides PostgreSQL pooling and context-aware operations.
- Numeric database IDs avoid introducing identity infrastructure before auth.
- Migrations are explicit and run before the API starts in Docker Compose.
- The sample project is isolated in its own Go module so it can later be
  checked out and analyzed as a target repository.
