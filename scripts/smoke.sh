#!/usr/bin/env sh
set -eu

api_url="${API_URL:-http://localhost:8080}"
curl --fail --silent --show-error "${api_url}/health"
curl --fail --silent --show-error "${api_url}/ready"

