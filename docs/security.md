# Phase 11 security review

This review records implemented controls and residual risks. It is not a claim
that the unauthenticated MVP is safe for public internet exposure.

It covers the legacy baseline and implemented document-driven Phases 2–5.
XLSX export, PR/MR execution mapping and application RBAC introduce additional
future trust boundaries tracked in
[DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md](DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md).

## Trust boundaries

| Component | Trust level | Important controls | Residual risk |
|---|---|---|---|
| API/frontend | Trusted application | input size/schema checks, rate limit, security headers, non-root/read-only containers | no user authentication/RBAC yet |
| Worker | Trusted control plane | non-root UID, dropped Linux capabilities, secret files, job leases | Docker socket can control the host daemon |
| Generated test sandbox | Hostile workload | no network, non-root, read-only root, no-new-privileges, all capabilities dropped, CPU/RAM/PID/time/output limits | Docker/kernel/runtime vulnerability remains possible |
| GitLab/GitHub repository content | Untrusted external input | response limits, path validation, AST parsing, sensitive-content filters | prompt injection and heuristic secret-filter gaps |
| Uploaded documents | Untrusted external input | exact body/file caps, private randomized object keys, SHA-256, passive parser, prompt-injection flag, DOCX archive/XML caps, traversal/macro/executable rejection | no malware scanner and no per-user authorization yet |
| LLM provider/output | External/untrusted | provider interface, timeout/size caps, strict JSON schema, untrusted-context boundary, service-owned citations, expected-result grounding | document/repository content leaves the deployment when provider is enabled |
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

## Phase 12 provenance findings

LLM evidence hashes only safe runtime configuration and never includes provider
API keys, SCM tokens, webhook secrets, or database credentials. Historical
prompts, responses, and denormalized source chunks are intentionally retained
for thesis reproducibility and therefore remain sensitive project data. The
full `/api/analyses/{id}/export` endpoint must remain behind the authenticated
private reverse proxy until Phase 18 implements application-level RBAC. A
retention/archive policy is still required before long-term production use.

## Document intake findings

Phase 2 accepts only `.docx`, `.md`, and `.markdown`, stores objects outside
PostgreSQL with owner-only permissions, and exposes no storage key or download
route. Markdown must be valid UTF-8 without NUL bytes. DOCX ZIP entry count,
individual XML size and total expanded size are bounded; unsafe paths,
`vbaProject.bin`, `.exe`, and `.dll` entries are rejected. Parsing never follows
links or executes embedded content, runs under a timeout, verifies the stored
size and checksum, and records failure without deleting the source version.

All uploads start as `DRAFT`; Phase 3–5 source, requirement and test-case review
routes are implemented but do not yet authenticate the reviewer identity. Keep
the UI/API on the trusted private boundary and treat reviewer names as audit
labels, not verified identities.
The `document_data` volume must be backed up with its matching PostgreSQL state;
the current backup script only covers PostgreSQL, so coordinated backup/restore
remains an explicit Phase 11 blocker for production recovery claims.

## Document AI and business-evidence findings

Semantic chunks retain raw uploaded text but embed normalized text. Retrieval is
filtered by document set and version policy in SQL. Prompt-like document text is
flagged and always enclosed as untrusted evidence; it is never interpreted as
system instructions. Every document AI call stores an immutable context snapshot
and raw validated response for audit.

The service, not the model, binds requirements to the processed document block.
Database triggers reject approving a requirement without evidence from an
approved source version and reject approving a test case without an approved,
cited requirement. LLM-generated expected results must exactly match approved
business evidence. These controls prevent code or model output from silently
becoming business truth, but they do not replace human review.

Index, extraction and generation actions are synchronous in the current MVP.
Request/provider timeouts bound individual calls, but large document sets can
still tie up API capacity; queue-based orchestration and per-user quotas remain
hardening work before public or multi-tenant deployment.

## Phase 13 impact-analysis findings

The worker materializes only bounded Go/module files from the exact source SHA.
Paths must be safe relative paths, file/count/total-size caps are enforced, SCM
access remains project-scoped, dependency downloads are disabled, and the
module cache is temporary. A `go.mod` local replacement may resolve only inside
the materialized snapshot; an escaping or absolute replacement forces the
non-executing AST fallback.
