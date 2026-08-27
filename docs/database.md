# Database

PostgreSQL stores metadata and will later host the `pgvector` knowledge index.
Phase 1 defines `projects`, `gitlab_connections`, and `analysis_jobs`. Phase 2
adds auditable webhook metadata to analysis jobs plus normalized `changed_files`
with raw unified diff text, size flags, and addition/deletion counts.

Migrations are in `backend/migrations` and use the `golang-migrate` filename
format. Apply them with `make migrate-up`; roll back one migration with
`make migrate-down`.
