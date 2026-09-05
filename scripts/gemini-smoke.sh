#!/usr/bin/env bash
set -euo pipefail

umask 077

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$repo_root"

env_file="${ENV_FILE:-.env.production}"
key_file="${LLM_KEY_FILE:-secrets/llm_api_key}"

if [[ ! -f "$env_file" ]]; then
	echo "missing $env_file" >&2
	exit 1
fi
if [[ ! -s "$key_file" ]]; then
	echo "missing or empty Gemini key file: $key_file" >&2
	exit 1
fi

env_value() {
	awk -F= -v wanted="$1" '$1 == wanted { sub(/^[^=]*=/, ""); value=$0 } END { print value }' "$env_file"
}

base_url="${GEMINI_SMOKE_BASE_URL:-$(env_value LLM_BASE_URL)}"
model="${GEMINI_SMOKE_MODEL:-$(env_value LLM_MODEL)}"
base_url="${base_url%/}"

if [[ ! "$base_url" =~ ^https://generativelanguage\.googleapis\.com/v1beta$ ]]; then
	echo "unexpected Gemini base URL: $base_url" >&2
	exit 1
fi
if [[ ! "$model" =~ ^[a-zA-Z0-9._-]+$ ]]; then
	echo "invalid Gemini model name" >&2
	exit 1
fi

api_key="$(tr -d '\r\n' < "$key_file")"
if [[ ! "$api_key" =~ ^[a-zA-Z0-9._-]{20,4096}$ ]]; then
	echo "Gemini key file has an unexpected format (AIza and AQ auth keys are supported)" >&2
	exit 1
fi

curl_config="$(mktemp)"
cleanup() {
	rm -f "$curl_config"
}
trap cleanup EXIT HUP INT TERM

printf 'header = "Content-Type: application/json"\n' > "$curl_config"
printf 'header = "x-goog-api-key: %s"\n' "$api_key" >> "$curl_config"
unset api_key

echo "Gemini smoke request: ${base_url}/interactions model=${model}"
curl --config "$curl_config" --silent --show-error --connect-timeout 10 --max-time 60 \
	--request POST "${base_url}/interactions" \
	--data-raw "{\"model\":\"${model}\",\"input\":\"Return a JSON object whose status is ok.\",\"generation_config\":{\"max_output_tokens\":64},\"response_format\":{\"type\":\"text\",\"mime_type\":\"application/json\",\"schema\":{\"type\":\"object\",\"properties\":{\"status\":{\"type\":\"string\"}},\"required\":[\"status\"],\"additionalProperties\":false}}}" \
	--write-out $'\nHTTP_STATUS=%{http_code}\n'
