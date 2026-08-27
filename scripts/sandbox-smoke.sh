#!/usr/bin/env sh
set -eu

image="${SANDBOX_IMAGE:-ai-test-assistant-sandbox:phase7}"
workspace="$(mktemp -d "${TMPDIR:-/tmp}/ai-test-sandbox-smoke.XXXXXX")"
container="ai-test-sandbox-smoke-$$"
cleanup() {
  docker rm --force --volumes "$container" >/dev/null 2>&1 || true
  rm -rf "$workspace"
}
trap cleanup EXIT INT TERM

chmod 0777 "$workspace"
cp -R examples/go-microservices/. "$workspace/"
chmod -R a+rwX "$workspace"

docker create --name "$container" --pull=never --network=none \
  --memory=512m --memory-swap=512m --cpus=1 --pids-limit=128 \
  --cap-drop=ALL --security-opt=no-new-privileges --read-only --user=65532:65532 \
  --mount=type=volume,destination=/workspace \
  --tmpfs=/tmp:rw,nosuid,nodev,exec,size=256m,mode=1777 --workdir=/workspace \
  "$image" go test -count=1 ./... >/dev/null

limits="$(docker inspect --format '{{.HostConfig.NetworkMode}}|{{.HostConfig.Memory}}|{{.HostConfig.MemorySwap}}|{{.HostConfig.NanoCpus}}|{{.HostConfig.PidsLimit}}|{{.HostConfig.ReadonlyRootfs}}' "$container")"
test "$limits" = "none|536870912|536870912|1000000000|128|true"
security="$(docker inspect --format '{{json .HostConfig.CapDrop}}|{{json .HostConfig.SecurityOpt}}' "$container")"
printf '%s' "$security" | grep 'ALL' >/dev/null
printf '%s' "$security" | grep 'no-new-privileges' >/dev/null
docker cp "$workspace/." "$container:/workspace"
docker start --attach "$container"
