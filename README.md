# AI Test Assistant

AI Test Assistant is a graduation engineering project that turns GitLab Merge
Request changes into project-aware Go test recommendations and generated tests.
The full product pipeline is documented in [PROJECT_SPEC.md](PROJECT_SPEC.md).

This repository currently implements Phases 0-3: the backend foundation,
GitLab capture pipeline, and deterministic Go changed-symbol analyzer. RAG,
LLM generation, sandbox execution, repair, and the frontend are intentionally
not implemented yet.

## Prerequisites

- Go 1.26.6 or a newer security-patched release
- Docker with Docker Compose
- `make`

## Start locally

```bash
cp .env.example .env
make dev-up
make smoke
```

The API is available at `http://localhost:8080`:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl -X POST http://localhost:8080/api/projects \
  -H 'Content-Type: application/json' \
  -d '{"name":"sample","gitlab_project_id":123,"repository_url":"https://gitlab.com/example/sample.git","default_branch":"main","language":"go"}'
curl http://localhost:8080/api/projects
```

Stop the stack with `make dev-down`. Run all local tests with `make test`.

## Repository map

- `backend/`: Go API, domain modules, storage, migrations, and tests.
- `examples/go-microservices/`: deterministic Go services used by later analysis experiments.
- `infra/`: application Dockerfiles and local Compose stack.
- `docs/`: architecture, API, database, and development notes.
- `scripts/`: small development helpers called by the Makefile.

## Current endpoints

- `GET /health`
- `GET /ready`
- `POST /api/projects`
- `GET /api/projects`
- `GET /api/projects/{id}`
- `POST /api/webhooks/gitlab`
- `GET /api/analyses`
- `GET /api/analyses/{id}`
- `GET /api/analyses/{id}/changes`

Configure a GitLab Merge Request webhook to call `/api/webhooks/gitlab`, set
its secret to `GITLAB_WEBHOOK_SECRET`, and enable Merge request events. The API
returns HTTP 202 after enqueueing; the worker fetches authoritative MR metadata
and paginated diffs using `GITLAB_TOKEN`. A second worker phase fetches the old
and new Go source at the MR target/source SHAs, maps unified-diff lines to Go AST
symbols, persists them, and advances the job to `RETRIEVING_CONTEXT`.
