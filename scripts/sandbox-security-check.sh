#!/usr/bin/env sh
set -eu

runner="backend/internal/validation/runner.go"
for required in \
  '"--pull=never"' \
  '"--network=none"' \
  '"--cap-drop=ALL"' \
  '"--security-opt=no-new-privileges"' \
  '"--read-only"' \
  '"--user=65532:65532"' \
  '"--pids-limit="' \
  '"--memory="' \
  '"--cpus="' \
  '"--env=GOPROXY=off"'; do
  if ! grep -F -- "$required" "$runner" >/dev/null; then
    echo "sandbox security invariant missing: $required" >&2
    exit 1
  fi
done

for dockerfile in infra/docker/Dockerfile.api infra/docker/Dockerfile.worker infra/docker/Dockerfile.sandbox; do
  if ! grep -E '^USER 65532:65532$' "$dockerfile" >/dev/null; then
    echo "$dockerfile must end its runtime setup as uid/gid 65532" >&2
    exit 1
  fi
done

if grep -E 'privileged:[[:space:]]*true' infra/compose/docker-compose.prod.yml >/dev/null; then
  echo "production Compose must not enable privileged containers" >&2
  exit 1
fi

echo "sandbox and runtime security invariants verified"
