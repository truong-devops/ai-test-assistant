#!/usr/bin/env sh
set -eu

api_url="${API_URL:-http://localhost:8080}"
frontend_url="${FRONTEND_URL:-http://localhost:3000}"
api_token="${API_AUTH_TOKEN:-}"
api_token_file="${API_AUTH_TOKEN_FILE:-secrets/api_auth_token}"
if [ "$api_token" = "" ] && [ -f "$api_token_file" ]; then
  api_token="$(cat "$api_token_file")"
fi
api_curl() {
  if [ "$api_token" != "" ]; then
    curl --fail --silent --show-error -H "Authorization: Bearer $api_token" \
      -H "X-Authenticated-Role: viewer" "$@"
  else
    curl --fail --silent --show-error "$@"
  fi
}
api_curl "${api_url}/health"
api_curl "${api_url}/ready"
api_curl "${api_url}/api/document-metrics" >/dev/null
curl --fail --silent --show-error "${frontend_url}/" >/dev/null
curl --fail --silent --show-error "${frontend_url}/projects" >/dev/null
curl --fail --silent --show-error "${frontend_url}/analyses" >/dev/null
curl --fail --silent --show-error "${frontend_url}/evaluations" >/dev/null

security_headers="$(curl --fail --silent --show-error --head "${frontend_url}/")"
printf '%s' "${security_headers}" | grep -qi '^content-security-policy:'
printf '%s' "${security_headers}" | grep -qi '^x-content-type-options: nosniff'
printf '%s' "${security_headers}" | grep -qi '^x-frame-options: DENY'
