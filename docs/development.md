# Development conventions

- New feature work follows
  `docs/DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md`; older phase documents describe
  the implemented code-first baseline.
- Keep business evidence and technical code context in separate models, prompts
  and UI sections.
- A test-case expected result must cite an approved document version; code and
  diff must never overwrite it.
- Automation repair may change technical implementation only, never the approved
  requirement/scenario/expected-result snapshot.
- Coverage must be calculated from persisted requirement-flow/test-case links,
  not from an LLM claim.
- Document intake accepts DOCX and Markdown only. Keep parser input passive: never run
  macros, embedded executables, document links, or uploaded code. XLSX input is
  deferred; do not silently treat the Phase 6 report template as a requirement
  source.
- `DOCUMENT_STORAGE_PATH` must resolve to storage shared by API and worker.
  `DOCUMENT_MAX_UPLOAD_BYTES` defaults to 16 MiB (hard maximum 256 MiB), and
  `DOCUMENT_PARSE_TIMEOUT` defaults to 60 seconds (hard maximum 10 minutes).
  Docker Compose mounts the `document_data` volume for both processes.
- Every upload is a new immutable version and starts as `DRAFT`. Phase 3 adds an
  explicit source review endpoint; there is still no delete endpoint. Preserve
  original files after parser failure and include document storage in backups.
- Increment `SemanticChunkerVersion` whenever chunk identity/metadata semantics
  change so an unchanged document checksum cannot incorrectly reuse an old index.
- Document retrieval SQL must always apply `document_set_id` and an explicit
  version policy. Do not rely on filtering results in application memory.
- Never accept citation IDs from an LLM. Requirement evidence is attached by the
  service from the semantic chunk actually processed, and test expected results
  must match an approved requirement statement or its explicit flow-step result.
- Run parser tests against the maintained synthetic DOCX/Markdown fixtures. A
  local sample can also be checked without committing it:

  ```bash
  cd backend
  DOCUMENT_FIXTURE_DOCX=/path/to/requirements.docx \
  DOCUMENT_FIXTURE_MARKDOWN=/path/to/requirements.md \
  go test -count=1 -run ExternalFixtures -v ./internal/document
  ```
- Stop any locally running Compose worker before repository integration tests;
  otherwise it may legitimately claim the same test queue rows and make the
  test nondeterministic. Restart it after the suite:

  ```bash
  docker compose -f infra/compose/docker-compose.yml stop worker
  TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/ai_test_assistant?sslmode=disable \
    make test-integration
  docker compose -f infra/compose/docker-compose.yml start worker
  ```

- Keep API and worker entry points limited to dependency assembly.
- HTTP handlers must call services rather than querying PostgreSQL directly.
- UV-07 testcase browser checks and isolated schema-28 setup are documented in
  [UV07_TESTCASE_VERSIONING_VERIFICATION.md](UV07_TESTCASE_VERSIONING_VERIFICATION.md).
  The script retains fixtures; never run it against production. Playwright/Chromium
  must be installed separately until the UV-09 browser CI lane is established.
- External systems must be represented by interfaces.
- Pass `context.Context` through I/O boundaries.
- Wrap errors with useful operation context and never log secrets.
- Add tests for domain behavior and externally visible HTTP behavior.
- Do not implement a later phase until the current phase Definition of Done is met.
- Use `EMBEDDING_PROVIDER=local` and `EMBEDDING_MODEL=hash-v1` for deterministic
  development. Unsupported providers fail worker startup instead of silently
  sending repository content to an external service.
- New retrieval queries must apply `project_id` in SQL, not only after fetching
  results.
- Legacy PR/MR LLM calls belong in background processors behind `llm.Provider`.
  Requirement extraction is also a durable leased job: POST only enqueues and
  the UI polls its status/progress endpoint. Keep provider work out of page
  rendering. Test-case generation remains an explicit bounded review action.
- Keep `LLM_PROVIDER=disabled` when AI calls are not desired. In that mode,
  document extraction/test generation use deterministic conservative draft
  rules. To enable Phase 4–5 document AI and legacy AI stages,
  use `LLM_PROVIDER=openai` or `LLM_PROVIDER=gemini` with `LLM_API_KEY` and an
  explicit `LLM_MODEL` from a private environment or secret manager. Leave
  `LLM_BASE_URL` empty for the provider default. Never commit the key.
- Phase 12 records token usage for every LLM call. Set
  `LLM_INPUT_COST_PER_MTOK_USD` and `LLM_OUTPUT_COST_PER_MTOK_USD` to the pinned
  model prices used by an experiment; both default to zero so the system never
  guesses a price.
- Every AI task must have a versioned prompt and strict output validation. Code,
  diffs, and retrieved documentation are untrusted prompt data.
- Run the Phase 3–5 focused workflow tests with PostgreSQL available:

  ```bash
  cd backend
  TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/ai_test_assistant?sslmode=disable \
    go test -count=1 -tags=integration ./internal/document ./internal/requirement ./internal/testcase
  ```

  The integration fixture verifies UC-B08 main/two alternate/two exception
  flows, approved-source guards, idempotent retries and deterministic coverage.
- Document-driven Phase 8 parses generated Go source without touching production
  files. Phase 9 writes an approved artifact only into a bounded temporary
  source-SHA workspace, runs the clean baseline first, then executes inside the
  isolated Docker sandbox. The older numbered sandbox phases below describe the
  retained code-first pipeline.
- Keep project baseline snapshots, export snapshots and automation generation
  calls append-only. A missing explicit requirement mapping must select the full
  approved suite, never an inferred subset.
- Build and run both legacy and document-driven Docker cases with
  `make sandbox-test`. Sandbox
  containers must keep network disabled, run as non-root, drop all capabilities,
  use a read-only root filesystem, and enforce CPU/memory/PID/time limits.
- The Compose worker is a trusted control-plane process with Docker socket
  access. Never mount that socket, the worker filesystem, `.env`, or host secrets
  into a generated-test sandbox.
- Phase 7 uses `GOPROXY=off`; projects with external modules need committed
  `vendor/` content until a trusted immutable dependency-cache design is added.
- Phase 8 must append repaired generated-test versions and `repair_attempts` in
  one transaction. Never update the previous generated code in place.
- `MAX_REPAIR_ATTEMPTS` defaults to 2, accepts 0 to disable repairs, and has a
  hard maximum of 3. It is separate from `WORKER_MAX_ATTEMPTS`, which only
  controls infrastructure/provider retry behavior.
- Repair prompts must include failed validation feedback, preserve the target
  test path, forbid production-code changes, and reject unchanged code.
- The Phase 9 frontend is a Next.js application. Run `make frontend-install`
  before `make frontend-typecheck` or `make frontend-build`; use Node.js
  18.18+ locally. Docker builds it with Node 20.
- Keep `BACKEND_API_URL` server-only. Browser mutations must use the
  same-origin frontend proxy and must not expose GitLab/GitHub, LLM, or database
  credentials.
- Review decisions are immutable. The backend, not the UI, decides whether a
  candidate is current and whether the analysis is ready for review. Keep
  validation failures and repair history visible in the review screen.
- The review context endpoint remains a fresh project-filtered retrieval from
  the current index. Historical prompt context is available separately from the
  immutable Phase 12 evidence endpoint/export and must not be conflated with the
  current-index panel.
- Phase 13 impact analysis fetches Go source and module metadata at the exact
  analysis source SHA. Package loading uses `GOPROXY=off`, `GOSUMDB=off`,
  `GOWORK=off`, and `CGO_ENABLED=0`; missing external dependencies produce an
  explicit AST fallback rather than network access.
- Default impact limits are depth 3, 250 selected nodes, 2,000 source files,
  1 MiB per file, 64 MiB total materialized source, and two minutes wall time.
- The worker runtime image includes the same pinned Go toolchain used by its
  build stage because `go/packages` invokes `go list`; `GOTOOLCHAIN=local`
  prevents runtime toolchain downloads.
