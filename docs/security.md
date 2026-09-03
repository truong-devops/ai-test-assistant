# Phase 11 security review

This review records implemented controls and residual risks. It is not a claim
that the unauthenticated MVP is safe for public internet exposure.

## Trust boundaries

| Component | Trust level | Important controls | Residual risk |
|---|---|---|---|
| API/frontend | Trusted application | input size/schema checks, rate limit, security headers, non-root/read-only containers | no user authentication/RBAC yet |
| Worker | Trusted control plane | non-root UID, dropped Linux capabilities, secret files, job leases | Docker socket can control the host daemon |
| Generated test sandbox | Hostile workload | no network, non-root, read-only root, no-new-privileges, all capabilities dropped, CPU/RAM/PID/time/output limits | Docker/kernel/runtime vulnerability remains possible |
| GitLab/GitHub repository content | Untrusted external input | response limits, path validation, AST parsing, sensitive-content filters | prompt injection and heuristic secret-filter gaps |
| LLM provider/output | External/untrusted | provider interface, timeout/size caps, strict JSON schema, path/source validation | repository content leaves the deployment when provider is enabled |
| PostgreSQL | Trusted state | isolated Compose network, file secret, migrations, backup checksum | DB role separation and immutable audit storage are pending |

## Sandbox findings

Implemented:

- sandbox image and runtime use UID/GID `65532`;
- Docker uses `--pull=never`, `--network=none`, `--read-only`,
  `--cap-drop=ALL`, `no-new-privileges`, bounded tmpfs and anonymous workspace
  volume;
- Docker socket, worker filesystem, `.env`, and application secrets are not
  mounted into the generated-test container;
- `GOPROXY`, `GOSUMDB`, toolchain download, CGO, core dumps, file descriptors,
  output bytes, wall time, processes, CPU, RAM, and swap are restricted;
- cleanup force-removes the container and volume after every result;
- `scripts/sandbox-security-check.sh` fails CI if core policy flags disappear.

Open findings:

- isolate Docker execution on a dedicated/rootless or remote runner so worker
  compromise cannot control the application host;
- pin runtime images by digest and add SBOM, signature, and CVE gates;
- evaluate a custom seccomp/AppArmor profile on the target Linux host;
- build a trusted immutable dependency cache if non-vendored projects are in
  scope;
- expand output redaction tests and minimize all secrets present on the worker.

## HTTP findings

The API now applies no-store, CSP, frame denial, MIME sniffing prevention,
referrer and permissions headers. Configurable server timeouts, maximum header
bytes, bounded request bodies, strict JSON decoding, and a bounded-memory token
bucket provide basic abuse resistance. Health/readiness endpoints bypass the
rate limiter so orchestration can recover.

The limiter keys on the direct peer address and does not trust
`X-Forwarded-For`. A trusted reverse proxy must authenticate users, normalize
forwarded headers, enforce client-level limits, terminate TLS, and protect
state-changing routes against CSRF. These controls do not replace the open
authentication/RBAC backlog.
