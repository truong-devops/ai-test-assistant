#!/usr/bin/env sh
set -eu

api_url="${API_URL:-http://localhost:8080}"
frontend_url="${FRONTEND_URL:-http://localhost:3000}"
curl --fail --silent --show-error "${api_url}/health"
curl --fail --silent --show-error "${api_url}/ready"
curl --fail --silent --show-error "${api_url}/api/evaluations" >/dev/null
curl --fail --silent --show-error "${frontend_url}/" >/dev/null
curl --fail --silent --show-error "${frontend_url}/projects" >/dev/null
curl --fail --silent --show-error "${frontend_url}/analyses" >/dev/null
curl --fail --silent --show-error "${frontend_url}/evaluations" >/dev/null

security_headers="$(curl --fail --silent --show-error --head "${frontend_url}/")"
printf '%s' "${security_headers}" | grep -qi '^content-security-policy:'
printf '%s' "${security_headers}" | grep -qi '^x-content-type-options: nosniff'
printf '%s' "${security_headers}" | grep -qi '^x-frame-options: DENY'
