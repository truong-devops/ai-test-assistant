#!/usr/bin/env sh
set -eu

env_file="${ENV_FILE:-.env.production}"
compose_file="${COMPOSE_FILE:-infra/compose/docker-compose.prod.yml}"
restore_file="${RESTORE_FILE:-}"

if [ "$restore_file" = "" ] || [ ! -f "$restore_file" ]; then
  echo "RESTORE_FILE must identify an existing coordinated .tar backup" >&2
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

restore_dir="$(mktemp -d "${TMPDIR:-/tmp}/ai-test-assistant-restore.XXXXXX")"
services_stopped=0
cleanup() {
  rm -rf "$restore_dir"
  if [ "$services_stopped" = "1" ]; then
    compose up -d api worker >/dev/null
  fi
}
trap cleanup EXIT HUP INT TERM
tar -C "$restore_dir" -xf "$restore_file"
for required in database.dump documents.tar.gz SHA256SUMS; do
  if [ ! -f "$restore_dir/$required" ]; then
    echo "coordinated backup is missing $required" >&2
    exit 1
  fi
done
(cd "$restore_dir" && openssl dgst -sha256 database.dump documents.tar.gz > SHA256SUMS.actual)
if ! cmp -s "$restore_dir/SHA256SUMS" "$restore_dir/SHA256SUMS.actual"; then
  echo "coordinated backup member checksum mismatch" >&2
  exit 1
fi

compose stop api worker
services_stopped=1
compose exec -T postgres sh -ec 'export PGPASSWORD="$(cat /run/secrets/postgres_password)"; exec pg_restore --clean --if-exists --no-owner --no-privileges --single-transaction --username="${POSTGRES_USER}" --dbname="${POSTGRES_DB}"' < "$restore_dir/database.dump"
compose --profile tools run --rm -T document-maintenance sh -ec 'find /documents -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +; tar -C /documents -xzf -' < "$restore_dir/documents.tar.gz"
compose up -d api worker
services_stopped=0
echo "coordinated database + document restore completed from $restore_file"
