#!/usr/bin/env sh
set -eu
docker compose -f infra/compose/docker-compose.yml down --remove-orphans

