#!/usr/bin/env sh
set -eu

env_file="${ENV_FILE:-.env.production}"
compose_file="${COMPOSE_FILE:-infra/compose/docker-compose.prod.yml}"

if [ ! -f "$env_file" ]; then
  echo "missing $env_file; copy .env.production.example and configure it" >&2
  exit 1
fi

compose() {
  docker compose --env-file "$env_file" -f "$compose_file" "$@"
}

compose config --quiet
compose build sandbox api worker frontend
compose --profile tools run --rm migrate
compose up -d postgres api worker frontend
compose ps
