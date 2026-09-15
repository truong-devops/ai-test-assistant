# Phase 11 deployment runbook

> This runbook deploys the legacy-compatible system plus document-driven Phases
> 0–10 and the implemented Phase 11 rollout controls. Compose provisions shared
> document storage; workers run parsing, requirement extraction, code analysis,
> sandbox execution and guarded repair queues.
> Follow the deployment/hardening checklist in
> [DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md](DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md)
> before treating a future document-driven build as production-ready.

The development stack remains in `infra/compose/docker-compose.yml`. Production
uses `infra/compose/docker-compose.prod.yml`; it has no default passwords,
mounts runtime secrets as files, binds HTTP ports to loopback by default, rotates
container logs, and starts application containers with a read-only root
filesystem, dropped capabilities, and `no-new-privileges`.

The production API requires a file-mounted bearer token and enforces service
roles. It does not authenticate end-user identities: keep loopback ports behind
an OIDC/authenticated reverse proxy, let only that trusted proxy/frontend know
the token, and do not publish the API directly to the internet.

## Clean-machine deployment

Requirements:

- Docker Engine with the Compose v2 plugin;
- access to a Docker daemon for the trusted worker;
- `openssl` for backup checksums;
- a TLS reverse proxy managed outside this Compose file.

Create configuration and secrets:

```bash
cp .env.production.example .env.production
mkdir -p secrets
chmod 700 secrets
openssl rand -hex 32 > secrets/postgres_password
openssl rand -hex 32 > secrets/gitlab_webhook_secret
printf '%s\n' 'glpat-replace-with-real-token' > secrets/gitlab_token
openssl rand -hex 32 > secrets/github_webhook_secret
printf '%s\n' 'github_pat_replace-with-real-token' > secrets/github_token
openssl rand -hex 32 > secrets/api_auth_token
read -rsp "LLM API key: " LLM_KEY
printf '%s' "$LLM_KEY" > secrets/llm_api_key
unset LLM_KEY
echo
```

Use the generated PostgreSQL password to create `secrets/database_url`. Because
`openssl rand -hex` is URL-safe, it can be placed directly in the URL:

```text
postgres://ai_test_assistant:<password>@postgres:5432/ai_test_assistant?sslmode=disable
```

If GitHub private repositories are not used, `secrets/github_token` may be an
empty file. If `LLM_PROVIDER=disabled`, `secrets/llm_api_key` may be an empty file.
To use Gemini, set the following values in `.env.production` after writing the
key. The same generic secret file is used for every LLM provider:

```dotenv
LLM_PROVIDER=gemini
LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta
LLM_MODEL=gemini-3.6-flash
LLM_FALLBACK_MODELS=gemini-3.5-flash-lite,gemini-2.5-flash-lite
LLM_REQUEST_TIMEOUT=45s
```

The API and worker run as UID/GID `65532`, so file-backed Compose secrets must be
group-readable by GID `65532` while remaining private from other host users:

```bash
sudo chgrp 65532 secrets/*
chmod 0640 secrets/*
```

Set `DOCKER_GID` in `.env.production` to the group ID that owns
`/var/run/docker.sock`; Docker Desktop commonly works with `0`, while Linux hosts
often require the `docker` group ID.

Validate and deploy:

```bash
make prod-config
make prod-up
API_URL=http://127.0.0.1:8080 FRONTEND_URL=http://127.0.0.1:3000 make smoke
```

`scripts/smoke.sh` reads `secrets/api_auth_token` by default (or
`API_AUTH_TOKEN_FILE`) for its protected API check and never prints the token.

For the first Gemini deployment, the following single command prompts for the
API key without echoing it, updates `.env.production`, validates Compose, then
rebuilds and recreates only the worker:

```bash
make prod-gemini-up
```

To verify the configured endpoint, key and model without creating a pull request
analysis, run `make prod-gemini-smoke`. The command never prints the API key.

For later deployments, choose the smallest matching rebuild:

| Changed area | Command |
|---|---|
| API only | `make prod-rebuild-api` |
| API and frontend, followed by `/ready` check | `make rebuild` |
| Worker, AI, indexing, analysis | `make rebuild-worker` |
| Shared backend code or migrations | `make rebuild-be` |
| Next.js frontend only | `make rebuild-fe` |
| API, worker and frontend | `make prod-rebuild-app` |
| Dependencies, sandbox or infrastructure | `make rebuild-all` |

A secret or environment-only change needs only `make prod-worker-restart` when
it affects the worker. Inspect the result with `make prod-status` and
`make prod-worker-logs`; use `make prod-worker-logs-follow` for live logs and
stop it with Ctrl+C. Run `make prod-help` to list the production shortcuts.

`prod-up` builds all runtime images and the sandbox, waits for PostgreSQL, runs
all migrations once, then starts the API, worker, and frontend. Migration is a
separate one-shot service and is never run concurrently by every application
replica.

## Upgrade and rollback

Before an upgrade:

```bash
make backup
make prod-migrate
```

Build and replace application containers only after migrations succeed. Prefer
roll-forward migrations. Use a down migration only after checking that no newer
application has written incompatible data. Image rollback does not automatically
roll back the database.

## Backup and restore

`make backup` stops API/worker writers, creates a PostgreSQL custom dump and a
compressed snapshot of `document_data`, verifies member checksums, then packs
both into one `backups/ai-test-assistant-<UTC>.tar` with an outer SHA-256 file.
Writers are restarted even if backup fails. Copy both `.tar` and `.tar.sha256`
to encrypted storage outside the Docker host and apply the approved retention
policy.

Restore is intentionally guarded and destructive:

```bash
RESTORE_FILE=backups/ai-test-assistant-YYYYMMDDTHHMMSSZ.tar \
RESTORE_CONFIRM=RESTORE_AI_TEST_ASSISTANT make restore
```

The script verifies outer/member checksums, stops API/worker writers, restores
PostgreSQL with `--clean --single-transaction`, replaces the scoped document
volume content, and restarts writers. Test this combined restore on a
non-production host regularly; an untested backup is not a recovery strategy.

## Reverse proxy and logs

Route `/api/*`, `/health`, `/ready`, and both SCM webhooks to port 8080; route
the review UI to port 3000. Terminate TLS, authenticate users, set request-size
limits, and perform client-IP rate limiting at that proxy. The API's built-in
rate limiter uses the direct socket peer and deliberately ignores spoofable
forwarding headers.

API and worker logs are structured JSON on stdout. Compose uses the `local`
driver with five 10 MiB files per container. Forward logs to protected central
storage before relying on them for audit or incident response.

## Post-deploy checks

```bash
docker compose --env-file .env.production -f infra/compose/docker-compose.prod.yml ps
curl --fail http://127.0.0.1:8080/health
curl --fail http://127.0.0.1:8080/ready
curl --fail http://127.0.0.1:3000/
```

Also deliver signed GitLab and GitHub test webhooks, process one controlled
MR/PR through review, and confirm a sandbox container has no network, host bind
mounts, Docker socket, or secrets.
