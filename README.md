# AI Test Assistant

AI Test Assistant is being refactored into a **document-driven testing system**:
approved requirements define test scenarios and expected results, while source
code at a GitHub Pull Request or GitLab Merge Request is the execution target.
Code may provide the technical context needed to build automation, but it must
not redefine the expected business behaviour.

The target flow is:

```text
Documents -> requirements -> reviewed test cases -> PR/MR automation
          -> isolated execution -> evidence -> XLSX/Markdown report
```

See [the Vietnamese system overview](docs.md) and
[the document-driven refactor plan](docs/DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md)
for the authoritative target direction and phase checklist.
The active UX/versioning follow-up is tracked in
[DOCUMENT_WORKFLOW_UX_AND_TESTCASE_VERSIONING_PLAN.md](docs/DOCUMENT_WORKFLOW_UX_AND_TESTCASE_VERSIONING_PLAN.md).

## Current implementation status

Document-driven refactor Phases 0–11 are now implemented alongside the earlier
Phases 0–13 baseline. Users can upload immutable DOCX/Markdown versions, approve
source versions, build a semantic document index, review cited requirements and
conflict/TBD items, generate grounded business test cases, and inspect a
deterministic requirement-to-test coverage matrix in the Next.js Documents
workspace. Testcase scenarios now have stable families and immutable revisions;
QA can publish an exact suite release, and projects bind that release rather
than a mutable “latest” query. Webhook analyses and test runs pin the release,
and approved cases can produce reviewed Go automation
artifacts plus immutable XLSX/Markdown exports. Approved artifacts now run at
the webhook source SHA in the isolated Docker sandbox; results retain the image
digest/environment fingerprint and distinguish product, automation, infra,
timeout and blocked outcomes. Requirement extraction is a durable background
job, so the browser receives HTTP 202 and polls progress instead of holding an
LLM request until a reverse proxy returns 502. Technical repair is restricted
to `AUTOMATION_ERROR`; database and prompt guardrails preserve expected hashes
and semantic assertions.

Indexing, requirement extraction and testcase generation now share one durable
workflow API/read model. Commands freeze their source input, return HTTP 202,
survive browser reloads and worker restarts, and expose progress, structured
blockers, retry/cancel state and attempt-level AI usage. Extraction is bridged
to its existing queue so there is only one scheduler for that work.

The earlier baseline remains operational:
GitLab/GitHub change capture, Go changed-symbol and impact analysis, a mixed
code/document project RAG index, AI recommendations, generated Go tests,
isolated Docker validation, bounded repair, provenance and human review. These
capabilities are being reused incrementally. Phase 11 now supplies a
per-project pipeline flag, document-first overview, service-token RBAC,
pipeline metrics, lifecycle audit, coordinated database/file backup, legacy
deprecation headers, a document-quality evaluation CLI, document-set AI budgets,
and guarded retention-based physical purge. A persisted-evidence verifier makes
production E2E acceptance explicit; it still requires a real SCM webhook, LLM,
and sandbox run before the environment-specific DoD can be checked.

[PROJECT_SPEC.md](PROJECT_SPEC.md) and the older graduation roadmap document
the implemented/historical code-first baseline. New implementation work should
follow the document-driven refactor plan.

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
curl -X POST http://localhost:8080/api/document-sets \
  -H 'Content-Type: application/json' \
  -d '{"name":"Checkout v1","product_name":"Storefront","scope":"UC-B08"}'
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
The API requires a file-mounted bearer token in production and enforces
`viewer`, `editor`, `reviewer`, and `admin` roles supplied by the trusted
frontend/reverse proxy. A real deployment must still let an identity-aware
reverse proxy or OIDC provider authenticate individual users; the shared token
is service authentication, not an end-user identity system.

## Repository map

- `backend/`: Go API, domain modules, storage, migrations, and tests.
- `frontend/`: Next.js review console and same-origin API proxy.
- `examples/go-microservices/`: deterministic Go services used by later analysis experiments.
- `infra/`: application Dockerfiles and local Compose stack.
- `docs/`: architecture, API, database, and development notes.
- [Document-driven refactor plan](docs/DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md):
  authoritative target architecture, backend/frontend phases and completion checklist.
- [Document-driven demo](docs/DOCUMENT_DRIVEN_DEMO.md): thao tác PTYC/URD từ
  upload đến webhook, sandbox và XLSX cùng các giới hạn phải trình bày trung thực.
- [Phase 1–10 follow-up register](docs/PHASE_1_10_FOLLOW_UPS.md): consolidated limitations and closure backlog.
- `scripts/`: small development helpers called by the Makefile.
- `.gitlab-ci.yml`: Phase 11 lint, test, integration, build, image, migration,
  dependency-audit, and sandbox jobs.

## Implemented baseline endpoints

Document-driven endpoints:

- `POST /api/document-sets`
- `GET /api/document-sets`
- `GET /api/document-sets/{id}`
- `GET /api/document-metrics`
- `POST /api/document-sets/{id}/lifecycle`
- `POST /api/document-sets/{id}/documents`
- `GET /api/document-sets/{id}/documents`
- `POST /api/document-sets/{id}/documents/{documentID}/versions`
- `GET /api/document-sets/{id}/documents/{documentID}/versions`
- `GET /api/documents/{id}/versions/{version}`
- `POST /api/document-versions/{id}/review`
- `GET /api/document-sets/{id}/workflow`
- `POST /api/document-sets/{id}/source-review`
- `POST /api/document-sets/{id}/workflow-operations`
- `GET /api/document-workflow-jobs/{id}`
- `POST /api/document-workflow-jobs/{id}/retry`
- `POST /api/document-workflow-jobs/{id}/cancel`
- `POST /api/document-sets/{id}/index`
- `GET /api/document-sets/{id}/index`
- `GET /api/document-sets/{id}/chunks`
- `POST /api/document-sets/{id}/retrieve`
- `POST /api/document-sets/{id}/requirements/extract`
- `GET /api/document-sets/{id}/requirements/extraction`
- `GET /api/document-sets/{id}/requirements`
- `GET /api/document-sets/{id}/requirement-conflicts`
- `GET /api/document-sets/{id}/open-questions`
- `GET /api/requirements/{id}`
- `POST /api/requirements/{id}/review`
- `POST /api/document-sets/{id}/test-cases/generate`
- `POST /api/document-sets/{id}/test-cases/regenerate`
- `GET /api/document-sets/{id}/test-cases`
- `GET /api/document-sets/{id}/coverage`
- `GET /api/test-cases/{id}`
- `POST /api/test-cases/{id}/review`
- `POST /api/test-cases/bulk-review`
- `GET /api/document-sets/{id}/test-case-families`
- `GET /api/test-case-families/{id}`
- `GET|POST /api/test-case-families/{id}/versions`
- `GET /api/test-case-families/{id}/diff`
- `POST /api/test-case-families/{id}/restore`
- `POST /api/test-case-families/{id}/archive`
- `POST|GET /api/document-sets/{id}/suite-releases`
- `GET /api/test-suite-releases/{id}`
- `POST|GET /api/document-sets/{id}/exports`
- `GET /api/test-exports/{id}/download`
- `GET|POST /api/projects/{id}/document-baseline`
- `GET|POST /api/analyses/{id}/test-scope`
- `POST /api/analyses/{id}/automation/generate`
- `GET /api/test-cases/{id}/automation`
- `POST /api/automation-artifacts/{id}/review`
- `GET /api/test-runs/{id}`
- `POST /api/test-runs/{id}/execute`
- `POST /api/test-run-items/{id}/classification`
- `POST /api/test-run-items/{id}/repair`
- `GET /api/test-run-items/{id}/repairs`
- `POST /api/projects/{id}/pipeline-mode`

Legacy-compatible endpoints:

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
- `GET /api/analyses/{id}/impact`
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
Set `LLM_PROVIDER` to `openai` or `gemini`, then provide `LLM_API_KEY` and an
explicit `LLM_MODEL` in a private `.env` or secret manager to enable it. Gemini
uses the Interactions API; OpenAI uses the Responses API.

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

Phase 13 materializes the repository at the analysis source SHA and builds a
bounded, explainable change-impact graph with `go/packages`, `go/types`, SSA,
and pinned CHA. It links callers, callees, type usage, interface
implementations, and existing tests. Repositories that do not type-check retain
their direct AST evidence through an explicit fallback result.

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
