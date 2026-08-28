#!/usr/bin/env sh
set -eu
umask 077

env_file="${ENV_FILE:-.env.production}"
compose_file="${COMPOSE_FILE:-infra/compose/docker-compose.prod.yml}"
backup_dir="${BACKUP_DIR:-backups}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
destination="$backup_dir/ai-test-assistant-$timestamp.dump"
temporary="$destination.partial"

compose() {
  docker compose --env-file "$env_file" -f "$compose_file" "$@"
}

mkdir -p "$backup_dir"
trap 'rm -f "$temporary"' EXIT HUP INT TERM
compose exec -T postgres sh -ec 'export PGPASSWORD="$(cat /run/secrets/postgres_password)"; exec pg_dump --format=custom --no-owner --no-privileges --username="${POSTGRES_USER}" --dbname="${POSTGRES_DB}"' > "$temporary"
test -s "$temporary"
mv "$temporary" "$destination"
openssl dgst -sha256 "$destination" > "$destination.sha256"
echo "backup written to $destination"
