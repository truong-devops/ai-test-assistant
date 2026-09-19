# Phase 11 security review

This review records implemented controls and residual risks. It is not a claim
that service-token authentication replaces an end-user identity system or makes
the application safe for direct public internet exposure.

It covers the legacy baseline and implemented document-driven Phases 2–11.
XLSX export, PR/MR execution mapping, technical repair and service RBAC are
included; remaining production gates are tracked in
[DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md](DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md).

## Trust boundaries

| Component | Trust level | Important controls | Residual risk |
|---|---|---|---|
| API/frontend | Trusted application | bearer service authentication, role checks, input caps, rate limit, security headers, non-root/read-only containers | shared token is not individual identity; OIDC proxy is still required |
| Worker | Trusted control plane | non-root UID, dropped Linux capabilities, secret files, job leases | Docker socket can control the host daemon |
| Generated test sandbox | Hostile workload | no network, non-root, read-only root, no-new-privileges, all capabilities dropped, CPU/RAM/PID/time/output limits | Docker/kernel/runtime vulnerability remains possible |
| GitLab/GitHub repository content | Untrusted external input | response limits, path validation, AST parsing, sensitive-content filters | prompt injection and heuristic secret-filter gaps |
| Uploaded documents | Untrusted external input | exact body/file caps, private randomized object keys, SHA-256, passive parser, prompt-injection flag, DOCX archive/XML caps, traversal/macro/executable rejection | no malware scanner and no per-user authorization yet |
| LLM provider/output | External/untrusted | provider interface, timeout/size caps, strict JSON schema, untrusted-context boundary, service-owned citations, expected-result grounding | document/repository content leaves the deployment when provider is enabled |
| PostgreSQL | Trusted state | isolated Compose network, file secret, migrations, immutable business guards, coordinated backup checksum | DB role separation and off-host immutable audit storage are pending |

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

Production API routes require a constant-time checked bearer token. A role
header maps to `viewer`, `editor`, `reviewer`, or `admin`; review, execute,
classification, export, repair, lifecycle, baseline and pipeline-mode mutations
require reviewer or admin. Health/readiness and provider-signed webhooks bypass
this token by design.

The limiter keys on the direct peer address and does not trust
`X-Forwarded-For`. A trusted reverse proxy must authenticate actual users,
derive the role/actor, normalize forwarded headers, enforce client-level limits,
terminate TLS, and protect state-changing routes against CSRF. Possession of
the shared service token alone does not establish a human identity.

## Phase 12 provenance findings

LLM evidence hashes only safe runtime configuration and never includes provider
API keys, SCM tokens, webhook secrets, or database credentials. Historical
prompts, responses, and denormalized source chunks are intentionally retained
for thesis reproducibility and therefore remain sensitive project data. The
full `/api/analyses/{id}/export` endpoint remains behind the authenticated
service and private reverse proxy. Physical purge is admin-only, waits for the
configured retention period, requires exact typed confirmation, refuses project
baselines, immutable analysis snapshots and test runs, deletes storage idempotently, and
keeps its audit row outside the deleted document-set foreign-key graph.

## Document intake findings

Phase 2 accepts only `.docx`, `.md`, and `.markdown`, stores objects outside
PostgreSQL with owner-only permissions, and exposes no storage key or download
route. Markdown must be valid UTF-8 without NUL bytes. DOCX ZIP entry count,
individual XML size and total expanded size are bounded; unsafe paths,
`vbaProject.bin`, `.exe`, and `.dll` entries are rejected. Parsing never follows
links or executes embedded content, runs under a timeout, verifies the stored
size and checksum, and records failure without deleting the source version.

All uploads start as `DRAFT`; source, requirement and test-case reviews require
the service role, but reviewer names remain audit labels until the reverse proxy
binds them to an authenticated user. Archive blocks new uploads and records
actor/reason. Backup now quiesces writers and packages PostgreSQL with the exact
matching `document_data` snapshot plus inner/outer checksums.

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

Index and requirement extraction use durable worker queues. Extraction captures
the index generation, renews a lease, persists per-chunk progress and retries
without holding an HTTP request, removing the reverse-proxy 502 failure mode.
Per-call output limits, HTTP rate limits and per-repair cost limits are combined
with a document-set token/cost budget. Every LLM call reserves a conservative
amount under a row lock before it starts, records actual usage on success, and
releases failed or expired reservations. Per-user quotas remain future work for
a multi-tenant identity model.

## Guarded technical repair findings

Only a run item classified exactly as `AUTOMATION_ERROR` and bound to an
approved artifact with the same expected hash can enter repair. A database
trigger freezes expected hash, semantic assertions, source hash, allowed-change
policy and budgets. Provider output is schema/Go/path/package checked and its
normalized assertions must match the original. Unsafe or unchanged output is a
candidate-level `UNREPAIRABLE`; `PRODUCT_FAILED` never enters the repair queue.

## Phase 13 impact-analysis findings

The worker materializes only bounded Go/module files from the exact source SHA.
Paths must be safe relative paths, file/count/total-size caps are enforced, SCM
access remains project-scoped, dependency downloads are disabled, and the
module cache is temporary. A `go.mod` local replacement may resolve only inside
the materialized snapshot; an escaping or absolute replacement forces the
non-executing AST fallback.
