# AI Test Assistant

AI Test Assistant is a graduation engineering project that turns GitLab Merge
Request and GitHub Pull Request changes into project-aware Go test recommendations and generated tests.
The full product pipeline is documented in [PROJECT_SPEC.md](PROJECT_SPEC.md).

This repository implements Phases 0-12: the backend foundation, GitLab capture
pipeline, deterministic Go changed-symbol analyzer, project-isolated
knowledge/RAG index, structured AI test recommendations and generated Go test
candidates, isolated Docker validation, a bounded repair loop, and the human
review console, a reproducible evaluation pipeline for the thesis experiments,
and production deployment, CI/CD, backup, and security hardening foundations.

## Prerequisites

- Go 1.26.6 or a newer security-patched release
- Docker with Docker Compose
- Node.js 18.18+ for running the frontend outside Docker
- `make`

## Start locally

```bash
cp .env.example .env
make dev-up
make smoke
```

The review console is available at `http://localhost:3000`; the API is
available at `http://localhost:8080`:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl -X POST http://localhost:8080/api/projects \
  -H 'Content-Type: application/json' \
  -d '{"name":"sample","provider":"gitlab","provider_project_id":123,"repository_url":"https://gitlab.com/example/sample.git","default_branch":"main","language":"go"}'
curl http://localhost:8080/api/projects
```

Stop the stack with `make dev-down`. Run all local tests with `make test`.
For frontend-only development, run `make frontend-install` once and then:

```bash
make frontend-typecheck
cd frontend && npm run dev
```

For a hardened single-host deployment, follow
[the Phase 11 deployment runbook](docs/deployment.md). It uses
`.env.production`, file-mounted secrets, a separate migration job, backup and
restore scripts, loopback-only ports, and hardened application containers.
The current MVP still requires an authenticated private reverse proxy because
application-level user authentication/RBAC remains a tracked backlog item.

## Repository map

- `backend/`: Go API, domain modules, storage, migrations, and tests.
- `frontend/`: Next.js review console and same-origin API proxy.
- `examples/go-microservices/`: deterministic Go services used by later analysis experiments.
- `infra/`: application Dockerfiles and local Compose stack.
- `docs/`: architecture, API, database, and development notes.
- [Phase 1–10 follow-up register](docs/PHASE_1_10_FOLLOW_UPS.md): consolidated limitations and closure backlog.
- `scripts/`: small development helpers called by the Makefile.
- `.gitlab-ci.yml`: Phase 11 lint, test, integration, build, image, migration,
  dependency-audit, and sandbox jobs.

## Current endpoints

- `GET /health`
- `GET /ready`
- `POST /api/projects`
- `GET /api/projects`
- `GET /api/projects/{id}`
- `POST /api/projects/{id}/index`
- `GET /api/projects/{id}/index/status`
- `POST /api/webhooks/gitlab`
- `POST /api/webhooks/github`
- `GET /api/analyses`
- `GET /api/analyses/{id}`
- `GET /api/analyses/{id}/changes`
- `GET /api/analyses/{id}/recommendations`
- `GET /api/analyses/{id}/generated-tests`
- `GET /api/analyses/{id}/validations`
- `GET /api/analyses/{id}/repairs`
- `GET /api/analyses/{id}/reviews`
- `GET /api/analyses/{id}/context`
- `GET /api/analyses/{id}/evidence`
- `GET /api/analyses/{id}/export`
- `GET /api/evaluations`
- `GET /api/evaluations/{id}`
- `POST /api/generated-tests/{id}/accept`
- `POST /api/generated-tests/{id}/reject`

Configure a GitLab Merge Request webhook to call `/api/webhooks/gitlab`, set
its secret to `GITLAB_WEBHOOK_SECRET`, and enable Merge request events. The API
returns HTTP 202 after enqueueing; the worker fetches authoritative MR metadata
and paginated diffs using `GITLAB_TOKEN`. A second worker phase fetches the old
and new Go source at the MR target/source SHAs, maps unified-diff lines to Go AST
symbols, persists them, and advances the job to `RETRIEVING_CONTEXT`.

For GitHub, register the project with `provider: "github"`, its numeric GitHub
repository ID as `provider_project_id`, and an `https://github.com/owner/repo`
URL. Configure a Pull request webhook at `/api/webhooks/github`, set its secret
to `GITHUB_WEBHOOK_SECRET`, and enable Pull requests. The worker reads public
repositories without credentials or uses `GITHUB_TOKEN` for private source and
Pull Request access.

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
jobs advance to `VALIDATING`. The Phase 7 worker downloads a private source-SHA
snapshot, inserts one candidate, copies it into an anonymous Docker volume, and
runs `go test -count=1 ./...` in the non-root sandbox. Network, capabilities,
privilege escalation, swap, CPU, memory, PIDs, output size, and wall time are
bounded. Passing jobs advance to `WAITING_REVIEW`; failed or timed-out tests
advance to `REPAIRING` for the bounded Phase 8 repair worker.

The Phase 8 worker uses the failed validation output plus exact project context
to create a replacement test-only version. Every repair links the previous
generated test, failed validation, new generated version, model, prompt version,
reason, and code hashes. Repaired versions return to `VALIDATING`; after
`MAX_REPAIR_ATTEMPTS` (default 2, hard maximum 3), a still-failing analysis moves
to `WAITING_REVIEW` so the loop always terminates.

The Phase 9 console keeps the MR diff, changed symbols, recommendation
rationale, generated test versions, sandbox output, and repair trail together
on one review screen. A decision is written once for each latest candidate:
only current versions can be accepted or rejected, and the analysis reaches
`ACCEPTED` only when every current candidate is accepted. A rejection makes
the final analysis status `REJECTED` after every current candidate has been
reviewed. The context panel deliberately identifies itself as the current
project-index retrieval; it is re-evaluated with the analysis project filter
and never mixes chunks from another project.

Phase 12 records an immutable provenance bundle for every recommendation,
generation, and repair LLM call, including failures and invalid structured
output. The review UI reads lightweight evidence metadata; the export endpoint
returns the exact prompt, response, safe configuration hashes, token usage,
latency, source/target SHA, index generation, and denormalized context snapshot.
Set the optional per-million-token cost variables to include an estimated USD
cost without storing provider credentials in evidence.

Phase 10 compares context impact, repair impact, and human effort with paired
scenario observations. Generate portable artifacts with `make evaluate`, or
run `make migrate-up && make evaluate-import` to also expose an immutable run
in the UI at `http://localhost:3000/evaluations`. The bundled dataset is a
controlled pipeline fixture, not thesis evidence; replace it with recorded
trials before reporting conclusions. See [docs/evaluation.md](docs/evaluation.md).

Build and exercise the real sandbox with `make sandbox-test`. Runtime dependency
downloads are intentionally disabled (`GOPROXY=off`); target repositories must
use the standard library, commit `vendor/`, or use a future trusted dependency
cache. The trusted worker needs access to the Docker daemon, but sandbox
containers never receive the Docker socket, worker filesystem, or host secrets.
