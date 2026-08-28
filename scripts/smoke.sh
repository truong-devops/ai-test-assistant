#!/usr/bin/env sh
set -eu

api_url="${API_URL:-http://localhost:8080}"
frontend_url="${FRONTEND_URL:-http://localhost:3000}"
curl --fail --silent --show-error "${api_url}/health"
curl --fail --silent --show-error "${api_url}/ready"
curl --fail --silent --show-error "${api_url}/api/evaluations" >/dev/null
curl --fail --silent --show-error "${frontend_url}/" >/dev/null
curl --fail --silent --show-error "${frontend_url}/evaluations" >/dev/null
