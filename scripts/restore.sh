#!/usr/bin/env sh
set -eu

env_file="${ENV_FILE:-.env.production}"
compose_file="${COMPOSE_FILE:-infra/compose/docker-compose.prod.yml}"
restore_file="${RESTORE_FILE:-}"

if [ "$restore_file" = "" ] || [ ! -f "$restore_file" ]; then
  echo "RESTORE_FILE must identify an existing pg_dump custom-format backup" >&2
  exit 1
fi
if [ "${RESTORE_CONFIRM:-}" != "RESTORE_AI_TEST_ASSISTANT" ]; then
  echo "set RESTORE_CONFIRM=RESTORE_AI_TEST_ASSISTANT to authorize destructive restore" >&2
  exit 1
fi
if [ -f "$restore_file.sha256" ]; then
  expected="$(awk '{print $NF}' "$restore_file.sha256")"
  actual="$(openssl dgst -sha256 "$restore_file" | awk '{print $NF}')"
  if [ "$expected" != "$actual" ]; then
    echo "backup checksum mismatch" >&2
    exit 1
  fi
fi

compose() {
  docker compose --env-file "$env_file" -f "$compose_file" "$@"
}

compose stop api worker
compose exec -T postgres sh -ec 'export PGPASSWORD="$(cat /run/secrets/postgres_password)"; exec pg_restore --clean --if-exists --no-owner --no-privileges --single-transaction --username="${POSTGRES_USER}" --dbname="${POSTGRES_DB}"' < "$restore_file"
compose up -d api worker
echo "restore completed from $restore_file"
