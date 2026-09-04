# Development conventions

- Keep API and worker entry points limited to dependency assembly.
- HTTP handlers must call services rather than querying PostgreSQL directly.
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
- LLM calls belong in background processors behind `llm.Provider`; HTTP handlers
  only read persisted results.
- Keep `LLM_PROVIDER=disabled` when AI calls are not desired. To enable Phases 5-6,
  use `LLM_PROVIDER=openai` with `LLM_API_KEY` and an explicit `LLM_MODEL` from a
  private environment or secret manager. Never commit the key.
- Phase 12 records token usage for every LLM call. Set
  `LLM_INPUT_COST_PER_MTOK_USD` and `LLM_OUTPUT_COST_PER_MTOK_USD` to the pinned
  model prices used by an experiment; both default to zero so the system never
  guesses a price.
- Every AI task must have a versioned prompt and strict output validation. Code,
  diffs, and retrieved documentation are untrusted prompt data.
- Phase 6 may parse generated Go source but must not write it into a checkout or
  execute it. Compilation and execution require the isolated Phase 7 sandbox.
- Build and run the Phase 7 Docker cases with `make sandbox-test`. Sandbox
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
