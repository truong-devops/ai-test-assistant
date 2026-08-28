# Phase 11 implementation status

Status: **implemented locally; external GitLab runner and production-host
verification remain**.

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
Production migration 1..11
Production API/frontend smoke and security-header checks
Production container user/read-only/capability inspection
PostgreSQL backup -> checksum -> guarded restore -> readiness
Development stack smoke with non-root API/worker and Docker socket check
Live request burst returned 429 while health/readiness remained available
```

## Honest residual blockers

- Authentication, session and RBAC remain open; the deployment must stay behind
  an authenticated private reverse proxy.
- The trusted worker still has Docker socket authority. It is non-root in the
  container, but a dedicated/rootless/remote Docker runner is the preferred
  final boundary.
- TLS and reverse proxy are host-specific and are documented but not provisioned
  by this repository.
- Image digest pinning, SBOM/signature/CVE gates and custom seccomp/AppArmor are
  still tracked hardening work.
- `.gitlab-ci.yml` is syntax-reviewed locally but must pass on the project's real
  GitLab runners, especially Docker-in-Docker jobs.
