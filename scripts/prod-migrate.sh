#!/usr/bin/env sh
set -eu

env_file="${ENV_FILE:-.env.production}"
compose_file="${COMPOSE_FILE:-infra/compose/docker-compose.prod.yml}"
docker compose --env-file "$env_file" -f "$compose_file" --profile tools run --rm migrate
