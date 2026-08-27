# Development conventions

- Keep API and worker entry points limited to dependency assembly.
- HTTP handlers must call services rather than querying PostgreSQL directly.
- External systems must be represented by interfaces.
- Pass `context.Context` through I/O boundaries.
- Wrap errors with useful operation context and never log secrets.
- Add tests for domain behavior and externally visible HTTP behavior.
- Do not implement a later phase until the current phase Definition of Done is met.

