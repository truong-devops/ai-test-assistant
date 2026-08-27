#!/usr/bin/env sh
set -eu
docker compose -f infra/compose/docker-compose.yml run --rm migrate

