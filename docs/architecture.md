# Architecture

The MVP is a modular monolith with two Go entry points: a synchronous HTTP API
and a background worker. Domain logic lives below `backend/internal`; entry
points only assemble dependencies.

The Phase 1 dependency direction is:

```text
HTTP request -> handler -> project service -> project repository -> PostgreSQL
                     \-> health checker -> PostgreSQL ping
```

Phases 2 through 7 add asynchronous database-backed pipelines:

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

generation worker -> claim GENERATING_TESTS with renewable lease
                  -> recommendation + exact RAG context + generate-test-v1
                  -> strict JSON/path/test-name/Go syntax validation
                  -> transactional generated_tests + code hash
                  -> VALIDATING

validation worker -> fetch source-SHA snapshot into a bounded private workspace
                  -> overlay one generated candidate without overwriting source
                  -> copy snapshot to an anonymous Docker volume
                  -> non-root `go test` with no network + CPU/memory/PID/time caps
                  -> transactional validation_runs
                  -> pass: WAITING_REVIEW / fail or timeout: REPAIRING

repair worker -> read latest failed version + its exact validation feedback
              -> retrieve project-filtered interfaces/tests/mocks
              -> repair-test-v1 structured LLM request
              -> immutable production code + fixed target path checks
              -> transactional generated_tests version + repair_attempts audit
              -> VALIDATING, or WAITING_REVIEW when the hard limit is reached
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
The provider is disabled unless explicitly configured. Phase 6 stores one
initial candidate per recommendation and keeps worker retry count separate from
the candidate's generation attempt. It parses generated Go syntax but never
executes the code. Phase 7 executes only inside ephemeral Docker containers.
The trusted worker controls Docker, but it transfers the snapshot through an
anonymous volume instead of bind-mounting its filesystem; the sandbox never
receives the Docker socket. Runtime dependency downloads are disabled, output
is bounded and redacted, and the container plus workspace are removed after
every run. Phase 8 keeps AI repair attempts separate from infrastructure retry
counts and caps them at three. The repair worker can only append a new generated
test version; it cannot mutate repository production files. Future phases add
human review actions and the UI.

## Decisions

- Standard `net/http` routing keeps the initial API small.
- `pgx` provides PostgreSQL pooling and context-aware operations.
- Numeric database IDs avoid introducing identity infrastructure before auth.
- Migrations are explicit and run before the API starts in Docker Compose.
- Hybrid retrieval always applies `project_id` inside the database query.
- Recommendation writes verify that every changed symbol belongs to the same
  analysis and commit recommendations with the state transition atomically.
- Generated-test writes verify recommendation ownership and atomically commit
  candidates with the transition to `VALIDATING`.
- Validation writes verify candidate ownership and atomically commit results
  with the transition to `WAITING_REVIEW` or `REPAIRING`.
- Repair writes verify source-version, validation, recommendation, and analysis
  ownership before atomically appending a new generated version and returning
  the analysis to `VALIDATING`.
- The sample project is isolated in its own Go module so it can later be
  checked out and analyzed as a target repository.
