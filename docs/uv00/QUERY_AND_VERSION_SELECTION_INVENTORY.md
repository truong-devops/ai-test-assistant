# UV-00 query and version-selection inventory

- Audited at commit: `b6c88c3`
- Date: 17/09/2026
- Scope: document, requirement, testcase, scope, automation, execution, report
- Method: repository-wide search for `version_number`, `supersedes_*`,
  `APPROVED`, latest-ordering, and the related service guards

This is the cutover checklist for UV-01 through UV-03. Line numbers may move;
the function/query description is the stable locator. The audit covers production
read/write paths in the named scope. Tests/migrations are listed separately where
they enforce the same semantics.

## 1. Selection vocabulary required at cutover

Every touched query must declare one of these policies in its repository method
or input. A bare “latest” is not accepted.

| Policy | Meaning |
| --- | --- |
| `LATEST_SOURCE_VERSION` | Greatest document version for a logical document, regardless of approval |
| `LATEST_APPROVED_SOURCE_VERSION` | Greatest approved source version; a newer draft does not hide it |
| `SOURCE_SNAPSHOT_MEMBERSHIP` | Exact document/chunk IDs pinned in a source/index snapshot |
| `LATEST_TESTCASE_REVISION` | Greatest revision in one family, regardless of review status |
| `LATEST_APPROVED_TESTCASE_REVISION` | Greatest approved revision in one family |
| `WORKING_TESTCASE_REVISION` | Explicit revision selected for the working set |
| `RELEASE_TESTCASE_REVISION` | Exact revision in an immutable suite release |
| `ANALYSIS_TESTCASE_REVISION` | Exact revision pinned by analysis snapshot |
| `RUN_TESTCASE_REVISION` | Exact revision referenced by a run item/snapshot |
| `HISTORICAL_ALL` | All versions ordered for audit/history only |

## 2. Document and index

| Location | Current selection | Consumer/purpose | Risk | Replacement policy |
| --- | --- | --- | --- | --- |
| `document.PostgresRepository.ListDocuments` | `DISTINCT ON (document_id)` ordered version descending | Document-set list shows newest upload | Valid for display only; not authority | `LATEST_SOURCE_VERSION`, with approved/current snapshot fields beside it |
| `document.IndexRepository.LoadSources` | Newest version per document; skips non-parsed versions | Builds index | A new unparsed v2 hides parsed v1, skipped count can produce partial-looking index | Explicit source snapshot; fail/require exclusion decision for missing members |
| `document.IndexRepository.GetStatus` | One mutable row per set | UI/extraction readiness | Upload does not change it, so old generation stays `READY` | Compare status source snapshot to requested working/approved snapshot |
| `document.IndexRepository.Begin` | Fingerprint of loaded IDs/SHA/model | Incremental index | Only refreshed when user calls index; approval warning freshness is mixed with content fingerprint | Index generation keyed by source snapshot/content fingerprint; authority read separately |
| `document.IndexRepository.ListChunks` | All chunks in set, no source version/generation predicate | Inspector and `AllChunks` | Extraction iterates historical v1/v2 chunks | `SOURCE_SNAPSHOT_MEMBERSHIP`; history endpoint separate |
| `document.IndexRepository.Retrieve` CTE `latest` | Greatest version per document | `VersionLatest` retrieval | Works independently from extraction subject and can mix subject from old version with current context | Generation membership first; version policy constrained inside pinned snapshot |
| `document.IndexRepository.Retrieve` CTE `latest_approved` | Greatest approved version per document | Authority retrieval | Newer approved source may not be indexed in current generation | Approved source snapshot + generation membership |
| `document.IndexRepository.SaveSnapshot` | Stores index generation and retrieved items | AI provenance | Good snapshot, but input generation can already be stale/mixed | Retain immutability; add source snapshot/generation constraints |
| `document.indexFingerprint` | version ID + SHA + chunker/model | Re-index identity | Does not include selected scope; approval warning not refreshed | Source snapshot fingerprint + content embedding fingerprint separated |
| `document.IndexRepository.ReviewVersion` | Updates source approval in place | Source authority | Index warning/status is not invalidated/recomputed | Record review event; workflow authority projection refreshes without vector rebuild |

Characterization: `backend/internal/document/uv00_characterization_integration_test.go`.

## 3. Requirement

| Location | Current selection | Consumer/purpose | Risk | Replacement policy |
| --- | --- | --- | --- | --- |
| `requirement.Service.RequestExtraction` | Checks only index status `READY` | Enqueue extraction | Accepts a READY generation for an older source upload | Require exact approved source snapshot/current generation |
| `requirement.Service.extract` | Checks queued generation number, then calls `AllChunks` | Extraction loop | Generation number does not constrain chunk membership | Load subject chunks by queued generation membership |
| `requirement.IndexService.RetrieveAndSnapshot` call | Retrieval policy `LATEST` | Context for each subject | An old subject may receive current-version context | Subject and context from same generation; always include subject |
| `requirement.Repository.SaveProposal` conflict key | Extraction key from identifier/flow/statement | Idempotent persistence | Same requirement from new authority may reuse row and append evidence to old revision | Requirement identity + immutable revision/provenance contract |
| `requirement.Repository.List` | Excludes any row with successor | Inventory and approved input for test generation | New draft/TBD successor hides older approved revision | Explicit latest, latest-approved, working, or snapshot policy |
| `requirement.Repository.Review` | Edit creates successor and applies decision in one transaction | Human review | Client remains on old URL; edit and decision semantics are coupled | Create revision then review exact revision/hash as separate commands |
| `requirement` approval trigger | Approval needs at least one evidence from an approved source | Integrity | “At least one” may leave other claimed sources invalid; source changes can alter live approval metadata | Validate complete declared evidence policy for revision/source snapshot |
| conflict/open-question list | All stored status rows by set | Review UI | Resolved/obsolete revision relationship is not part of selection contract | Filter/relate to working requirement revisions and retain history separately |

## 4. Testcase

| Location | Current selection | Consumer/purpose | Risk | Replacement policy |
| --- | --- | --- | --- | --- |
| `testcase.Repository.Save` previous lookup | Same suite + generated logical key, highest version | Reuse/create revision | Logical key collisions turn distinct scenarios into versions | Server-assigned family; matching is explicit/reviewable |
| `testcase.logicalCaseKey` | Hash of test type + requirement keys | Testcase identity | Two NEGATIVE/BOUNDARY cases for one requirement collide | Persistent family/public key; no content-derived identity |
| `testcase.generationKey` + `proposalIdentity` | Requirement IDs + type/title/case expected | Reuse detection | Ignores steps, step expected, data, conditions, assumptions, evidence | Full canonical content/provenance hash |
| `testcase.Repository.Save` reused branch | Adds requirement links to existing testcase row | Dedupe/source merge | Can mutate provenance of an approved revision | Immutable revision links; new provenance creates/reuses exact immutable revision |
| `testcase.Repository.List` | Excludes row with any successor | Main testcase workspace | Draft/rejected successor hides approved predecessor | Family summary exposes latest/latest-approved/working/release revision |
| `testcase.Repository.Get` | Exact row ID | Detail page | Good revision lookup, but no family/history navigation | Retain exact lookup and add family/version endpoints |
| `testcase.Repository.Review` | Edit creates successor then approves/rejects it | QA review | Combined command; FE refreshes old ID; blank values cannot always be intentional edits | Separate create revision and exact-hash review commands |
| `testcase.Repository.Coverage` | Latest leaf testcase per requirement; any status except rejected counts | Coverage | Draft counts as coverage; approved predecessor hidden by draft | Report designed/approved/released/automated/executed separately |
| `testcase.Service.Generate` | Latest approved requirements returned by requirement list policy | Test generation | Requirement draft successor can remove approved source unexpectedly | Generate from explicit approved requirement snapshot |
| testcase approval trigger | At least one linked approved requirement/source | Integrity | Does not establish all expected/step assertions are grounded | Revision validator over all declared links/assertions |

Characterization: `backend/internal/testcase/uv00_characterization_test.go` and
`uv00_characterization_integration_test.go`.

## 5. Scope and project baseline

| Location | Current selection | Consumer/purpose | Risk | Replacement policy |
| --- | --- | --- | --- | --- |
| `scope.Repository.BaselineView` | Counts approved testcase leaf rows in mutable suite | Baseline candidates | Draft successor makes approved count drop | Count approved revisions in latest published release |
| `scope.Repository.Select` validation | Suite has at least one approved leaf testcase | Bind project | Binding has no immutable testcase manifest | Require exact `release_id` and audit previous binding |
| `scope.Repository.SnapshotForAnalysis` requirements | Approved requirement leaf rows | Analysis business snapshot | New drafts can hide approved requirements | Pin approved requirement/source snapshot from release |
| `scope.Repository.SnapshotForAnalysis` cases | Approved testcase leaf rows | Analysis testcase snapshot | Mutable set is recomputed at webhook time | Copy `RELEASE_TESTCASE_REVISION` manifest atomically |
| `scope.Repository.SnapshotForAnalysis` document versions | Approved version only if no newer version of any status | Source snapshot | New draft v2 hides approved v1, producing no document entry | Use release's approved source snapshot |
| `scope.Repository.Decide` | Exact scope item/testcase ID, requires current testcase approved | Manual scope decision | Live status can diverge from analysis snapshot | Validate against analysis-pinned revision/snapshot, not mutable row status alone |

## 6. Automation

| Location | Current selection | Consumer/purpose | Risk | Replacement policy |
| --- | --- | --- | --- | --- |
| `automation.Repository.LoadSubject` | Exact testcase ID from analysis scope and approved status | Generation input | Mostly pinned correctly; needs release/revision content hash and full steps provenance | `ANALYSIS_TESTCASE_REVISION` with release/content hash |
| `automation.Repository.SaveArtifact` | Next artifact version within exact testcase ID | Artifact revision | Correct per current revision; copy/reuse to new testcase revision is undefined | Artifact remains pinned to revision; reuse creates new artifact with origin |
| `automation.Repository.History` | All artifact versions for exact testcase ID | Detail history | Good for revision, but UI currently presents it as whole testcase history | Label revision-specific; family-wide view explicit |
| `automation.Repository.RequestExecution` | Exact scope/testcase, latest approved artifact for that testcase | Prepare run | Correct exact testcase, but release binding not exposed | Validate analysis/release/revision/artifact hashes together |
| repair subject/retry queries | Exact run item, testcase, artifact | Technical repair | Expected hash guard exists; full case/steps hash is absent | Add full revision content hash guard; never rebind old artifact |

## 7. Execution

| Location | Current selection | Consumer/purpose | Risk | Replacement policy |
| --- | --- | --- | --- | --- |
| `execution.Repository.Claim` | Exact test run items; latest approved artifact for each exact testcase | Sandbox claim | Artifact selection may change between run creation and claim | Pin artifact at run creation/approval handoff; claim exact ID/hash |
| `execution.Repository.SaveOutcome` retry | Copies exact testcase/artifact/expected snapshot | Retry | Good snapshot behavior | Retain; add full testcase content/release IDs where needed |
| `execution.Repository.Get` | Exact run items and snapshots | Run detail | Good basis for historical display | Retain; avoid joining mutable latest fields for semantics |
| classification review | Exact run item and prior status | Human classification | Good audit basis | Persist authenticated actor separately from declared reviewer |

## 8. Report/export

| Location | Current selection | Consumer/purpose | Risk | Replacement policy |
| --- | --- | --- | --- | --- |
| `report.Repository.BuildSnapshot` document versions | Newest version by number | Export metadata | Old run may show source uploaded after it ran | Use run/release source snapshot |
| `report.Repository.BuildSnapshot` testcase rows | Leaf testcase rows in suite | Test Cases sheet | Old run exports draft/new revision instead of executed revision | `RUN_TESTCASE_REVISION` when run supplied; release/explicit selection otherwise |
| report join to run item | Lateral match for currently selected testcase | Actual/status | If selected latest differs from run revision, returns `NOT_RUN` | Start from run items and join exact revision |
| report join to reviews | Direct join all reviews | Reviewer metadata | Multiple reviews can duplicate testcase rows | Choose relevant review event or aggregate deterministically |
| report run history | Exact historical run items joined testcase keys | Run History sheet | Generally stable; family/public key may need explicit snapshot | Preserve exact IDs/keys and add revision/release columns |
| `report.Repository.Save` | Immutable JSON snapshot and artifact hashes | Stored export | Correct immutable boundary | Retain; downloads return stored bytes, regeneration is a new record |

Characterization: `backend/internal/report/uv00_characterization_integration_test.go`.

## 9. Database constraints and migration dependencies

| Current constraint/trigger | Keep/change in later phase |
| --- | --- |
| Unique `(test_suite_id, test_case_key, version_number)` | Keep for compatibility; add family-based unique counter |
| `supersedes_test_case_id` self-FK | Retain as legacy/parent lineage during backfill; family ID becomes primary grouping |
| Testcase approval evidence trigger | Replace/extend with full immutable revision evidence validation |
| Approved testcase content immutability trigger | Extend to child steps/requirement/evidence links; ordinary updates cannot bypass parent trigger |
| Test run item expected snapshot immutability | Keep; add release/revision/full-content provenance |
| Artifact expected hash constraint | Keep; add testcase full-content hash or revision snapshot guard |
| Document/requirement evidence immutability | Keep; coordinate with controlled graph purge and source snapshots |

## 10. Audit completion criteria

- [x] Every production package named in the UV-00 scope appears above.
- [x] Every current leaf/latest/approved query found by the repository search is
  assigned a replacement policy or explicitly retained as exact/history behavior.
- [x] Current triggers that affect approval/version immutability are recorded.
- [x] DATA-01/02 and VER-02/03/04/06 have executable characterization locations.
- [ ] Re-run this inventory after UV-03 and close each risk with a test/commit link.
