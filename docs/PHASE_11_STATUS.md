# Phase 11 implementation status

> Historical implementation evidence for the legacy code-first deployment.
> Document-driven deployment changes are tracked separately in
> [DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md](DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md).
> In particular, the current stack has service-token authentication and
> role-gated mutations plus coordinated database/file backup. End-user identity,
> OIDC/session handling and production reverse-proxy verification remain rollout
> responsibilities; the older snapshot below must not be read as current scope.

Status: **implementation complete; external runner and real production E2E
acceptance remain environment-specific verification gates**.

## Delivered

- production Dockerfiles for API, worker, frontend, healthcheck, and hostile Go
  sandbox workloads;
- non-root runtime users, OCI metadata, read-only/no-new-privileges production
  containers, dropped capabilities, bounded PIDs/tmpfs, healthchecks and log
  rotation;
- separate production Compose with loopback ports, isolated database network,
  file-mounted secrets and one-shot migration service;
- configuration support for Docker secret files, log levels, HTTP timeouts,
  maximum header bytes and bounded-memory request rate limiting;
- API/frontend security headers and strict direct-peer rate-limit behavior;
- PostgreSQL custom-format backup, SHA-256 checksum and guarded restore scripts;
- GitLab CI stages for vet/typecheck, unit/race tests, PostgreSQL integration,
  migration round-trip, builds, four Docker images, dependency audit, sandbox
  policy and real sandbox execution;
- deployment and security runbooks;
- total document-set AI token/cost budgets with atomic reservations;
- admin-only, retention-gated physical purge with reference guards and durable
  post-deletion audit;
- persisted-evidence E2E verifier covering source, RAG, LLM, reviews, webhook,
  automation, sandbox evidence and XLSX export;
- production Compose smoke, backup and restore drill on an isolated temporary
  stack.

## Evidence run locally

```text
docker compose config --quiet
scripts/sandbox-security-check.sh
Docker builds: API, worker, frontend, sandbox
Sandbox smoke + validation/repair Docker tests
Go unit/race tests + full PostgreSQL integration suite
Frontend typecheck/build + production dependency audit (0 vulnerabilities)
Production migration 1..22
Production API/frontend smoke and security-header checks
Production container user/read-only/capability inspection
PostgreSQL backup -> checksum -> guarded restore -> readiness
Development stack smoke with non-root API/worker and Docker socket check
Live request burst returned 429 while health/readiness remained available
```

## Honest residual blockers

- Service-token RBAC is implemented. Individual user identity/session/CSRF
  remains the responsibility of an authenticated reverse proxy or OIDC layer.
- The trusted worker still has Docker socket authority. It is non-root in the
  container, but a dedicated/rootless/remote Docker runner is the preferred
  final boundary.
- TLS and reverse proxy are host-specific and are documented but not provisioned
  by this repository.
- Image digest pinning, SBOM/signature/CVE gates and custom seccomp/AppArmor are
  still tracked hardening work.
- `.gitlab-ci.yml` is syntax-reviewed locally but must pass on the project's real
  GitLab runners, especially Docker-in-Docker jobs.
- Phase 11's environment DoD remains unchecked until
  `make prod-document-e2e-verify` returns `"passed": true` for IDs created by a
  real SCM webhook, configured LLM and sandbox run.
