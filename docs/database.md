# Database

PostgreSQL stores metadata and will later host the `pgvector` knowledge index.
Phase 1 defines `projects`, `gitlab_connections`, and `analysis_jobs`. Phase 2
adds auditable webhook metadata to analysis jobs plus normalized `changed_files`
with raw unified diff text, size flags, and addition/deletion counts. Phase 3
adds `changed_symbols`, linked to `changed_files` with cascade deletion. It stores
symbol kind, method receiver, package, line range, change type, and deterministic
summary metadata.

Migrations are in `backend/migrations` and use the `golang-migrate` filename
format. Apply them with `make migrate-up`; roll back one migration with
`make migrate-down`.
