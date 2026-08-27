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
- Keep `LLM_PROVIDER=disabled` when AI calls are not desired. To enable Phase 5,
  use `LLM_PROVIDER=openai` with `LLM_API_KEY` and an explicit `LLM_MODEL` from a
  private environment or secret manager. Never commit the key.
- Every AI task must have a versioned prompt and strict output validation. Code,
  diffs, and retrieved documentation are untrusted prompt data.
