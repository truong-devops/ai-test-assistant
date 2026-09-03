# Phase 11 deployment runbook

The development stack remains in `infra/compose/docker-compose.yml`. Production
uses `infra/compose/docker-compose.prod.yml`; it has no default passwords,
mounts runtime secrets as files, binds HTTP ports to loopback by default, rotates
container logs, and starts application containers with a read-only root
filesystem, dropped capabilities, and `no-new-privileges`.

Authentication/RBAC is not implemented yet. Until that backlog is closed, the
loopback ports must sit behind an authenticated private reverse proxy or remain
reachable only from a trusted network. Do not publish them directly to the
internet.

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
printf '%s\n' 'replace-with-real-key' > secrets/llm_api_key
```

Use the generated PostgreSQL password to create `secrets/database_url`. Because
`openssl rand -hex` is URL-safe, it can be placed directly in the URL:

```text
postgres://ai_test_assistant:<password>@postgres:5432/ai_test_assistant?sslmode=disable
```

If GitHub private repositories are not used, `secrets/github_token` may be an
empty file. If `LLM_PROVIDER=disabled`, `secrets/llm_api_key` may be an empty file. Set every
secret file to mode `0600`. Set `DOCKER_GID` in `.env.production` to the group ID
that owns `/var/run/docker.sock` on the deployment host; Docker Desktop commonly
works with `0`, while Linux hosts often require the `docker` group ID.

Validate and deploy:

```bash
make prod-config
make prod-up
API_URL=http://127.0.0.1:8080 FRONTEND_URL=http://127.0.0.1:3000 make smoke
```

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

`make backup` creates a PostgreSQL custom-format dump plus SHA-256 checksum in
`backups/`. Copy both files to encrypted storage outside the Docker host and
apply an explicit retention policy.

Restore is intentionally guarded and destructive:

```bash
RESTORE_FILE=backups/ai-test-assistant-YYYYMMDDTHHMMSSZ.dump \
RESTORE_CONFIRM=RESTORE_AI_TEST_ASSISTANT make restore
```

The script verifies the checksum when present, stops API/worker writers, runs
`pg_restore --clean --single-transaction`, and restarts them. Test restore on a
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
curl --fail http://127.0.0.1:3000/evaluations
```

Also deliver signed GitLab and GitHub test webhooks, process one controlled
MR/PR through review, and confirm a sandbox container has no network, host bind
mounts, Docker socket, or secrets.
