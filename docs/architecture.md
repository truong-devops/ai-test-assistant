# Architecture

> **Architecture status – 2026-09-21:** this document records the architecture
> implemented by document-driven Phases 0–10, Phase 11 rollout controls,
> workflow/versioning UV-00–UV-06 (UV-05 human usability acceptance pending), and the still-running code-first
> baseline. It is retained so maintainers can safely migrate the system. The
> target architecture and its ordered backend/frontend work are defined in
> [DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md](DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md).

## Target authority model

The UV-05 document workspace presents four business steps over the shared
workflow read model. Upload transactionally schedules source preparation;
explicit source review transactionally records exact version/hash approvals
and a durable continuation. The worker, not a page GET or browser timer,
advances index and requested extraction. Role-derived capabilities shape UI
actions; authenticated server identity remains authoritative. See
[UV-05 verification](UV05_WORKSPACE_VERIFICATION.md) for coverage and limits.

UV-06 adds exact revision/hash requirement batch review with per-item transaction
receipts and explicit clarification audit. Completed extraction compares stable
identifier + logical source + flow against the latest reviewed revision chain;
only the current generation activates its new inventory. Ambiguous mappings do
not transfer decisions. Reviewed content/evidence stay immutable, old IDs remain
readable, and affected testcase revisions must be reviewed before a new release.
Existing release/run snapshots are unchanged. See
[UV-06 verification](UV06_REQUIREMENT_REVIEW_VERIFICATION.md).

The refactor does not remove SCM automation or sandbox execution. It separates
business truth from technical execution:

```text
Approved documents -> requirements -> reviewed test cases
                                            |
PR/MR source SHA -> technical automation -> sandbox run -> execution report
```

- Documents determine the scenario and expected result.
- Code/diff may select technical scope and help create compilable automation.
- Code must never be used to rewrite an approved expected result.
- A run must distinguish product failure, automation error, infrastructure
  error, timeout and missing/contradictory specification.
- Repair may change automation implementation but not the requirement or
  expected-result snapshot.

Phases 2–10 implement document intake, semantic retrieval, requirement inventory,
human source/requirement review, grounded business test cases, deterministic
coverage, PR/MR baseline snapshots, Go automation artifacts and immutable
reports, source-SHA sandbox execution, typed results and evidence. Legacy
project/analysis tables remain in use as the SCM trigger and technical pipeline.
Phase 11 adds project migration mode, document-first metrics/navigation,
service-token roles, lifecycle audit, coordinated backup and evaluation tooling.

## Implemented document-driven architecture

```text
Next.js Documents form -> same-origin proxy -> Go document handler/service
                                           -> SHA-256 local volume object
                                           -> immutable document version (UPLOADED)

document parse worker -> leased version -> checksum verification
                      -> passive DOCX/Markdown parser
                      -> ordered blocks + source locators -> PARSED/FAILED

explicit workflow intent -> HTTP 202 durable job + frozen input snapshot
workflow worker -> lease + heartbeat + bounded retry + atomic output publish
                -> shared progress/error/usage read model
explicit index action -> semantic chunks per identifier/flow/table row
                      -> normalized embedding + retained raw evidence
                      -> document-set/version filtered hybrid retrieval
                      -> immutable context snapshots

explicit extraction -> delegated durable job bound to index generation
extraction worker -> retrieve per semantic unit -> strict schema/rule extractor
       -> progress + evidence-bound requirement + flow steps
       -> deterministic dedupe -> conflict/TBD inventory
       -> PO/BA review + immutable requirement version

approved requirements -> generator per requirement/flow
                      -> grounded expected result + steps + source links
                      -> exact/semantic dedupe -> QA review/versioning
                      -> database-derived coverage matrix

approved testcase revisions -> immutable suite release manifest
project -> selected suite release -> webhook-time immutable baseline snapshot
PR/MR identifiers -> explicit testcase scope; uncertain mapping -> full suite
changed path/module/symbol -> technical signals only, never expected behavior
approved testcase + Go context -> versioned/reviewed automation artifact
approved artifacts complete -> leased document execution run
per case -> source-SHA workspace -> clean baseline -> artifact overlay
         -> isolated Docker run -> typed result + bounded/redacted evidence
         -> infra-only retry or completed run
automation error -> guarded repair queue -> semantic assertion/hash check
                 -> draft artifact -> mandatory technical review
suite/run snapshot -> Actual/Status/Evidence -> XLSX/Markdown + snapshot hashes
```

API and worker use the same `document_data` volume. PostgreSQL contains only
metadata and normalized text blocks, not uploaded binary objects. DOCX handling
limits archive entries, expanded bytes and XML size; it rejects traversal paths,
macros and embedded executable extensions. Markdown requires UTF-8 without NUL
bytes. Both parsers honor cancellation and the worker's bounded parse timeout.

Parsing itself does not call an LLM and none of Phases 2–6 inspect repository
code. All uploads start as `DRAFT`. Indexing may include draft sources with an
explicit warning, but requirement/test-case approval is guarded by database
triggers requiring approved source evidence. When the LLM is enabled,
extraction/generation use context marked as untrusted, strict output schemas and
persisted raw call/snapshot provenance. The disabled-provider development path
uses deterministic draft generation.

Index, requirement extraction and testcase generation are explicit background
workflow operations. One read model reports role-derived capabilities,
machine-readable blockers, active/recent jobs and the next action. The shared
workflow job owns idempotency, input pinning, retry/cancel and usage reporting;
requirement extraction delegates to its established queue rather than adding a
second scheduler. Provider latency is therefore independent of HTTP write
timeouts. Initial automation generation remains an explicit review action and
is bounded by provider timeouts.

## Implemented baseline architecture

The MVP is a modular monolith with two Go entry points: a synchronous HTTP API
and a background worker. Domain logic lives below `backend/internal`; entry
points only assemble dependencies.

The Phase 1 dependency direction is:

```text
HTTP request -> handler -> project service -> project repository -> PostgreSQL
                     \-> health checker -> PostgreSQL ping
```

Phases 2 through 7 add asynchronous database-backed pipelines:

```text
GitLab/GitHub webhook -> verify + deduplicate -> PENDING analysis job -> HTTP 202
worker -> claim job -> provider MR/PR API + normalized paginated diffs -> changed_files
       -> ANALYZING_CHANGE
change worker -> fetch target/source Go files -> Go AST + diff line mapping
              -> changed_symbols -> RETRIEVING_CONTEXT (Phase 4 handoff)

POST project index -> PENDING index generation -> index worker
index worker -> provider repository tree at default branch
             -> path/content security filters -> semantic Go/docs chunks
             -> content-hash update + pgvector -> READY
retriever -> mandatory project filter + structural + full-text + cosine scores

recommendation worker -> claim RETRIEVING_CONTEXT as RECOMMENDING_TESTS
                      -> retrieve context per changed symbol
                      -> render recommend-test-v1 prompt
                      -> LLM provider + strict JSON schema validation
                      -> transactional test_recommendations
                      -> GENERATING_TESTS (Phase 6 handoff)

generation worker -> claim GENERATING_TESTS with renewable lease
                  -> recommendation + exact RAG context + generate-test-v1
                  -> strict JSON/path/test-name/Go syntax validation
                  -> transactional generated_tests + code hash
                  -> VALIDATING

validation worker -> fetch source-SHA snapshot into a bounded private workspace
                  -> overlay one generated candidate without overwriting source
                  -> copy snapshot to an anonymous Docker volume
                  -> non-root `go test` with no network + CPU/memory/PID/time caps
                  -> transactional validation_runs
                  -> pass: WAITING_REVIEW / fail or timeout: REPAIRING

repair worker -> read latest failed version + its exact validation feedback
              -> retrieve project-filtered interfaces/tests/mocks
              -> repair-test-v1 structured LLM request
              -> immutable production code + fixed target path checks
              -> transactional generated_tests version + repair_attempts audit
              -> VALIDATING, or WAITING_REVIEW when the hard limit is reached

Next.js review console -> server-side reads from Go API
                       -> same-origin /api/backend proxy for browser actions
                       -> evidence screen: diff, symbols, context, recommendations,
                          generated code, validations, repairs, human decision

review POST -> lock analysis + current generated version
            -> insert one immutable test_reviews row
            -> aggregate latest candidates
            -> ACCEPTED when all accept; otherwise REJECTED after all decide

evaluation-v1 dataset -> strict paired-metric validation -> SHA-256 identity
                      -> JSON + CSV + Markdown + SVG artifacts
                      -> optional immutable PostgreSQL run
                      -> read-only API + Next.js evaluation ledger

every LLM call -> immutable llm_calls record
               -> exact prompt/schema/response + usage/latency/config hashes
               -> immutable context snapshot + denormalized chunk contents
               -> lightweight evidence UI / complete JSON audit export

source SHA -> bounded source materializer
           -> go/packages + go/types -> SSA + pinned CHA
           -> calls / implementations / type-usage graph
           -> bounded traversal from direct changed symbols
           -> impact nodes + explainable edges + existing-test links
           -> AST_FALLBACK when the repository cannot type-check
```

Worker claims use a bounded lease. Temporary SCM/preparation failures are
retried with a delay up to `WORKER_MAX_ATTEMPTS`; an expired lease makes a job
recoverable after a worker crash. Retry counts reset at each phase handoff, so a
temporary source-fetch failure does not consume the analyzer retry budget. AI
repair attempts in later phases remain a separate counter.

Direct diff overlap remains authoritative in Phase 13. Inferred impact is
separately labelled and scored; failure to type-check never discards direct AST
results.

The analyzer parses both target and source versions of modified or renamed Go
files. This lets it detect fully deleted symbols as well as additions and body
changes. Non-Go files are retained in `changed_files` but skipped by the AST
analyzer. Index requests carry a generation number so a newer request cannot be
overwritten by a stale worker. Unchanged content hashes retain existing vectors;
removed chunks are deleted transactionally. The local hash embedding provider
is deterministic for development and tests, while the embedding boundary can
be replaced by a remote model later. Phase 5 keeps vendor details behind the
`llm.Provider` boundary and supports OpenAI Responses API and Gemini Interactions
API adapters. Both send only the compact diff and project-filtered retrieved
chunks, use a versioned prompt, and reject malformed or oversized output before persistence.
The provider is disabled unless explicitly configured. Phase 6 stores one
initial candidate per recommendation and keeps worker retry count separate from
the candidate's generation attempt. It parses generated Go syntax but never
executes the code. Phase 7 executes only inside ephemeral Docker containers.
The trusted worker controls Docker, but it transfers the snapshot through an
anonymous volume instead of bind-mounting its filesystem; the sandbox never
receives the Docker socket. Runtime dependency downloads are disabled, output
is bounded and redacted, and the container plus workspace are removed after
every run. Phase 8 keeps AI repair attempts separate from infrastructure retry
counts and caps them at three. The repair worker can only append a new generated
test version; it cannot mutate repository production files. Phase 9 adds a
human-review console and persisted Accept/Reject decisions.
Phase 10 keeps evaluation outside the asynchronous product worker: the offline
CLI validates versioned observations and deterministically builds thesis-ready
artifacts. PostgreSQL is optional for artifact generation and only serves the
read-only evaluation UI after import.

## Implemented baseline decisions

- Standard `net/http` routing keeps the initial API small.
- `scm.Client` routes GitLab and GitHub through one normalized source boundary;
  provider-specific tokens and webhook verification remain isolated.
- `pgx` provides PostgreSQL pooling and context-aware operations.
- Numeric database IDs avoid introducing identity infrastructure before auth.
- Migrations are explicit and run before the API starts in Docker Compose.
- Hybrid retrieval always applies `project_id` inside the database query.
- Recommendation writes verify that every changed symbol belongs to the same
  analysis and commit recommendations with the state transition atomically.
- Generated-test writes verify recommendation ownership and atomically commit
  candidates with the transition to `VALIDATING`.
- Validation writes verify candidate ownership and atomically commit results
  with the transition to `WAITING_REVIEW` or `REPAIRING`.
- Repair writes verify source-version, validation, recommendation, and analysis
  ownership before atomically appending a new generated version and returning
  the analysis to `VALIDATING`.
- The Phase 9 review decision locks the analysis and target generated-test
  version in one transaction. Only the latest version of a recommendation can
  be decided, and all latest candidates must be reviewed before the analysis
  reaches `ACCEPTED` or `REJECTED`.
- The Next.js console uses server-side `BACKEND_API_URL` for reads and a
  same-origin proxy for browser POST actions, so the browser never needs an
  externally visible backend address. The context panel recomputes a
  project-filtered view of the current index; it is not presented as a frozen
  historical LLM prompt.
- Phase 10 datasets require one baseline and one treatment observation for each
  scenario/replicate pair. Syntax, compile, execution, coverage, and human
  acceptance keep separate numerators and denominators.
- The sample project is isolated in its own Go module so it can later be
  checked out and analyzed as a target repository.
- Phase 12 snapshots context content instead of retaining foreign keys to live
  knowledge chunks. Index refresh can replace live chunks without changing the
  historical prompt evidence associated with an LLM call.
