#!/usr/bin/env sh
set -eu
umask 077

env_file="${ENV_FILE:-.env.production}"
compose_file="${COMPOSE_FILE:-infra/compose/docker-compose.prod.yml}"
backup_dir="${BACKUP_DIR:-backups}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
destination="$backup_dir/ai-test-assistant-$timestamp.tar"
temporary="$destination.partial"

compose() {
  docker compose --env-file "$env_file" -f "$compose_file" "$@"
}

mkdir -p "$backup_dir"
work_dir="$(mktemp -d "$backup_dir/.ai-test-assistant-backup.XXXXXX")"
services_stopped=0
cleanup() {
  rm -f "$temporary"
  rm -rf "$work_dir"
  if [ "$services_stopped" = "1" ]; then
    compose up -d api worker >/dev/null
  fi
}
trap cleanup EXIT HUP INT TERM

# Quiesce both database writers and document-file writers so the two snapshots
# describe one consistent application point in time.
compose stop api worker
services_stopped=1
compose exec -T postgres sh -ec 'export PGPASSWORD="$(cat /run/secrets/postgres_password)"; exec pg_dump --format=custom --no-owner --no-privileges --username="${POSTGRES_USER}" --dbname="${POSTGRES_DB}"' > "$work_dir/database.dump"
compose --profile tools run --rm -T document-maintenance tar -C /documents -czf - . > "$work_dir/documents.tar.gz"
test -s "$work_dir/database.dump"
test -s "$work_dir/documents.tar.gz"
(cd "$work_dir" && openssl dgst -sha256 database.dump documents.tar.gz > SHA256SUMS)
tar -C "$work_dir" -cf "$temporary" database.dump documents.tar.gz SHA256SUMS
test -s "$temporary"
mv "$temporary" "$destination"
openssl dgst -sha256 "$destination" > "$destination.sha256"
compose up -d api worker >/dev/null
services_stopped=0
echo "coordinated database + document backup written to $destination"
