#!/usr/bin/env sh
set -eu
docker compose -f infra/compose/docker-compose.yml up --build -d

