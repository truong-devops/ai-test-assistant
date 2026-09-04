#!/usr/bin/env bash
set -euo pipefail

umask 077

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$repo_root"

env_file="${ENV_FILE:-.env.production}"
secret_dir="${SECRET_DIR:-secrets}"
secret_gid="${SECRET_GID-65532}"
key_file="$secret_dir/llm_api_key"

if [[ ! -f "$env_file" ]]; then
	echo "missing $env_file; copy .env.production.example first" >&2
	exit 1
fi

mkdir -p "$secret_dir"
chmod 0700 "$secret_dir"

read -r -s -p "Gemini API key: " gemini_key
printf '\n'
if [[ -z "$gemini_key" ]]; then
	echo "Gemini API key must not be empty" >&2
	exit 1
fi

temporary_key="$(mktemp "$secret_dir/.llm_api_key.XXXXXX")"
temporary_env=""
cleanup() {
	rm -f "$temporary_key"
	if [[ -n "$temporary_env" ]]; then
		rm -f "$temporary_env"
	fi
}
trap cleanup EXIT HUP INT TERM

printf '%s' "$gemini_key" > "$temporary_key"
unset gemini_key
chmod 0640 "$temporary_key"
mv -f "$temporary_key" "$key_file"
temporary_key=""

if [[ -n "$secret_gid" ]]; then
	sudo chgrp "$secret_gid" "$key_file"
fi
chmod 0640 "$key_file"

set_env_value() {
	local key="$1"
	local value="$2"
	temporary_env="$(mktemp "${env_file}.XXXXXX")"
	awk -v key="$key" -v value="$value" '
		BEGIN { updated = 0 }
		index($0, key "=") == 1 {
			if (!updated) print key "=" value
			updated = 1
			next
		}
		{ print }
		END { if (!updated) print key "=" value }
	' "$env_file" > "$temporary_env"
	mv -f "$temporary_env" "$env_file"
	temporary_env=""
}

set_env_value "LLM_PROVIDER" "gemini"
set_env_value "LLM_BASE_URL" "https://generativelanguage.googleapis.com/v1beta2"
set_env_value "LLM_MODEL" "gemini-3.6-flash"
set_env_value "LLM_FALLBACK_MODELS" "gemini-3.5-flash-lite,gemini-2.5-flash-lite"

echo "Gemini configured in $env_file"
echo "API key stored in $key_file (value hidden)"
