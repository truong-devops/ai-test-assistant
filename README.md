# AI Test Assistant

AI Test Assistant is a graduation engineering project that turns GitLab Merge
Request changes into project-aware Go test recommendations and generated tests.
The full product pipeline is documented in [PROJECT_SPEC.md](PROJECT_SPEC.md).

This repository currently implements Phases 0-6: the backend foundation,
GitLab capture pipeline, deterministic Go changed-symbol analyzer, and
project-isolated knowledge/RAG index plus structured AI test recommendations
and generated Go test candidates. Sandbox execution, repair, and the frontend
are intentionally not implemented yet.

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
- `POST /api/projects/{id}/index`
- `GET /api/projects/{id}/index/status`
- `POST /api/webhooks/gitlab`
- `GET /api/analyses`
- `GET /api/analyses/{id}`
- `GET /api/analyses/{id}/changes`
- `GET /api/analyses/{id}/recommendations`
- `GET /api/analyses/{id}/generated-tests`

Configure a GitLab Merge Request webhook to call `/api/webhooks/gitlab`, set
its secret to `GITLAB_WEBHOOK_SECRET`, and enable Merge request events. The API
returns HTTP 202 after enqueueing; the worker fetches authoritative MR metadata
and paginated diffs using `GITLAB_TOKEN`. A second worker phase fetches the old
and new Go source at the MR target/source SHAs, maps unified-diff lines to Go AST
symbols, persists them, and advances the job to `RETRIEVING_CONTEXT`.

Requesting a project index returns HTTP 202. The index worker reads the default
branch snapshot, excludes sensitive/generated/unsupported files, creates
symbol-aware chunks for Go implementation, tests, mocks, and selected Markdown,
then stores hybrid-search embeddings in pgvector. Development uses the local
deterministic `hash-v1` embedding provider and requires no external AI key.

The recommendation worker retrieves a compact project-specific context for
each changed symbol, renders the versioned `recommend-test-v1` prompt, validates
strict structured output, stores the result, and advances the analysis to
`GENERATING_TESTS` for the Phase 6 handoff. LLM access is disabled by default.
Set `LLM_PROVIDER=openai`, `LLM_API_KEY`, and `LLM_MODEL` in a private `.env` or
secret manager to enable the real OpenAI provider.

The generation worker uses each stored recommendation with the exact retrieved
interfaces, implementation, mocks, and closest tests. It validates the target
path, declared test names, package, build constraints, size, and Go syntax
before storing the candidate with a code hash and trace metadata. Successful
jobs advance to `VALIDATING`. Generated code is not written to GitLab and is not
executed by Phase 6; isolated execution belongs to Phase 7.
