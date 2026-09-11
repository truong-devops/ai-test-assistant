# ADR 0003: Document authority, immutable versions, and MVP boundary

- Status: Accepted
- Date: 2026-09-11
- Owners: Product/BA for source baselines; QA/Test Lead for business test cases;
  Developer or QA Automation reviewer for executable artifacts

## Context

The legacy pipeline starts from a PR/MR and can infer scenarios and expected
behaviour from implementation code. That is useful for technical automation,
but it is not an objective source for business correctness. The refactor needs
a stable authority boundary before document RAG or test generation is added.

## Decision

Business scenarios and expected results are derived only from approved product
evidence. Implementation code, a diff, or an impact graph may select execution
scope and provide compile/runtime context, but cannot overwrite that evidence.

The authority order is:

1. Approved requirements, use cases, user stories, acceptance criteria, and
   explicit business rules.
2. Approved API/database/system designs for technical constraints they actually
   specify.
3. Historical defects for regression risk and reproduction evidence.
4. Existing test cases as reference material only; they do not become approved
   truth automatically.

Conflicting or incomplete sources remain `CONFLICT` or `TBD` until a human
reviewer decides them. Every new upload starts as `DRAFT`; Phase 2 deliberately
does not expose an unauthenticated approval endpoint.

Each uploaded file creates an immutable `document_versions` row and an original
file object identified by SHA-256. Uploading the same logical document again
appends a version instead of overwriting the previous one. Parsed blocks and
requirement evidence cannot be updated in place. A later explicit retention
workflow may delete an entire owned graph, but ordinary product flows are
append-only.

The first automation target remains Go unit tests because the legacy sandbox,
source materializer, and test generator can be reused once approved business
test cases exist. Black-box API automation requires an approved API contract,
base URL, credentials, and seed/cleanup policy and is deferred. UI automation
is deferred until a locator and test-account contract exists.

The Phase 2 upload MVP accepts DOCX and Markdown. XLSX is an export format in
Phase 6. XLSX import is deferred until its sheet/range/merged-header semantics
are specified and tested against a stable input template.

For the MVP, original files and their database evidence are retained for the
lifetime of the document set and included in backup scope. There is no public
delete endpoint. Formal time-based retention, legal deletion, and RBAC are
Phase 11 work; until then the application remains behind the documented trusted
private boundary.

## Approval responsibilities

- Product Owner or Business Analyst approves document baselines and normalized
  requirements.
- QA or Test Lead approves business test cases and their expected results.
- Developer or QA Automation reviewer approves executable artifacts.
- A sandbox pass never substitutes for any of these decisions.

The implementation request that introduced Phases 0–2 records product-owner
acceptance of this technical direction. Any separate academic mentor approval
for thesis title or evaluation criteria remains project-governance evidence,
not an application runtime dependency.

## Consequences

- The legacy API and PR/MR pipeline remain operational during migration.
- Document and business-test models use new tables/packages rather than
  re-labelling `generated_tests` as business test cases.
- Automation artifacts are rejected unless their test case is approved and the
  copied expected-result hash matches.
- Local persistent volume storage is sufficient for the MVP, but API and worker
  must share it and production backup must include it with PostgreSQL.
- Phase 3 may index parsed blocks but cannot weaken set/version ownership or
  source citations.
