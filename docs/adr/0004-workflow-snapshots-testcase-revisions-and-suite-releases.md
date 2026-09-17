# ADR 0004: Workflow snapshots, testcase revisions, and suite releases

- Status: Accepted for implementation planning
- Date: 2026-09-17
- Owners: Product/BA for source and requirement decisions; QA/Test Lead for
  testcase revisions and suite releases; engineering for job/execution integrity
- Supersedes: no prior ADR; extends ADR 0003

## Context

The document-driven pipeline already stores immutable document versions,
requirement versions, testcase rows with `version_number`, automation versions,
run snapshots, and export snapshots. The current read paths nevertheless use
several different meanings of “latest”:

- the newest document version by number;
- the testcase row that has no `supersedes_test_case_id` child;
- the newest approved artifact by version number;
- the set of approved testcases visible at webhook time;
- the newest testcase rows when an old run is exported again.

Those choices are not equivalent. A new draft testcase currently hides an older
approved testcase in several read paths. A document upload can leave the old
index marked `READY`. Re-indexing retains historical chunks, while extraction
loads all stored chunks. The UI exposes these technical stages as separate pages
and expects the user to discover the order.

The product needs two guarantees before the workflow is simplified:

1. every long-running operation uses one explicit, immutable input snapshot;
2. every execution and report names the exact testcase revision selected by an
   immutable suite release or run snapshot.

## Decision

### Source snapshots and index generations

An upload changes the working source revision of a document set. A source
snapshot records the selected document version IDs, checksums, inclusion state,
and fingerprint for one operation. Index generations record their source
snapshot and exact chunk membership.

The default extraction input is an approved source snapshot. A job queued for
snapshot S continues to refer to S if a later upload creates S+1. Its result is
shown as historical/outdated and is never silently published into S+1.

The extraction subject and all retrieved context must belong to the same index
generation. Stored chunks from older versions remain available for audit, but
they are not part of the current generation unless the snapshot explicitly
contains them.

Approval is authority metadata. If content and embedding inputs are unchanged,
approval changes may update eligibility/authority without recomputing vectors.
The workflow status must still stop presenting a stale warning count as current.

### Testcase identity and revision

A testcase identity, called a family in the implementation plan, represents one
stable business scenario. A testcase revision is an immutable payload for that
identity at one point in time. `test_cases.id` remains the revision ID so current
foreign keys and historical references remain valid.

The identity is server-assigned and does not derive solely from testcase type,
title, expected result, or requirement key. Two negative scenarios for one
requirement are separate identities. Regeneration may propose a match, but an
ambiguous match requires a reviewer decision before lineage is changed.

The canonical revision payload contains:

- title, type, risk, actor, preconditions, test data, postconditions;
- ordered steps and step-level expected results;
- case-level expected result and assumptions;
- exact requirement revision IDs and evidence/source references;
- provenance needed to distinguish a source update from an identical replay.

The content hash excludes timestamps, database-generated IDs, review status,
run results, and automation status. Normalization must not change case-sensitive
test data or literal strings.

Revision content, steps, and evidence references are immutable after creation.
Editing or restoring creates a new draft revision. Approval and rejection are
review events over an exact revision ID and content hash. A review does not edit
content or increment the version number.

A draft or rejected successor does not revoke or hide an older approved revision.
Archiving an identity prevents it from entering future releases while retaining
all revisions and historical references.

### Suite releases and project baselines

A suite release is an immutable manifest mapping each testcase identity to one
approved testcase revision. It also records the source/requirement snapshot,
publisher, reason, timestamp, and manifest hash.

A project baseline selects a suite release, not a mutable suite whose contents
are recomputed using “latest”. Webhook analysis snapshots, automation artifacts,
test runs, and exports retain the release and exact revision IDs they used.

Creating or approving a new revision does not move a project baseline. Publishing
a new suite release also does not move it. Binding the new release to a project is
an explicit, audited action.

Run exports start from the run's pinned items/snapshot. They do not replace an
executed revision with a newer leaf revision. A stored export remains immutable;
creating another export produces another artifact with its own hash.

### Workflow operations

The default product workflow has four user-facing steps:

1. Tài liệu
2. Yêu cầu
3. Testcase
4. Sử dụng/Xuất

Parse, index, extraction, and generation are durable operations with pinned
inputs and persisted progress. The UI receives a workflow read model containing
step states, capabilities, blocking reasons, current jobs, and one recommended
next action. The server rechecks every mutation condition.

Calling an LLM requires an explicit recorded intent and scope. Technical stages
may continue automatically after that intent, but opening a page or performing a
GET request never invokes the provider.

### Roles and actor provenance

The role policy for new mutations is:

| Operation | Minimum role |
| --- | --- |
| Upload source, create draft revision, enqueue index/extract/generate | `editor` |
| Approve/reject source, requirement, testcase; publish release; bind project release; export | `reviewer` |
| Change budget/retention, permanent purge, migration recovery | `admin` |
| Read workflow, history, diff, jobs, releases, runs, exports | `viewer` |

Authorization uses the authenticated role at the server. `X-Authenticated-Actor`
is the authenticated service/session actor when supplied by the trusted proxy.
Until end-user sessions exist, a submitted `reviewer_name` is stored as a declared
display identity alongside the service actor; it is not proof of authentication
and never grants a role. Audit records must distinguish these two fields.

Route policy for new endpoints is explicit by route/operation. Substring matching
such as “path contains `/review`” is not sufficient for publish, restore, bulk
review, or release binding.

## Source update policy

- A draft source version creates a new working state but does not replace a
  published baseline.
- A rejected source version remains in history and is excluded from a new approved
  source snapshot.
- A source removed from the working scope remains in historical snapshots. Its
  requirements/testcases become review candidates; they are not deleted.
- If requirement/testcase matching is ambiguous, the item is `NEEDS_MAPPING` and
  is excluded from automatic lineage or retirement decisions.
- A source update never edits an approved testcase revision in place and never
  changes the expected result of an existing run.

## Compatibility and migration

Schema changes use expand/backfill/cutover. Current row IDs and foreign keys remain
valid. Legacy successor chains become provisional families. Chains that may mix
multiple scenarios are reported for review; migration does not invent a split,
actor, historical release, or approval event.

For a currently bound mutable suite, migration may create a transition release
from the exact state selected by the old logic at migration time. It is labelled
`MIGRATED_CURRENT_STATE`, not presented as a release that existed historically.

Old read endpoints for revision IDs remain available during the transition. Old
mutation endpoints must call the new revision services so they cannot bypass
content immutability, concurrency checks, or release rules.

## Consequences

- The implementation needs new source snapshot, index membership, testcase
  family, suite release, and audit data.
- “Latest”, “latest approved”, “working candidate”, and “pinned in release” become
  separate queries and UI labels.
- Job and release hashes add storage and implementation work, but make retry,
  export, and historical analysis deterministic.
- The current combined edit-and-approve command becomes a compatibility adapter;
  the new contract separates save from review.
- New testcase drafts no longer interrupt the approved release used by projects.
- The workflow can hide technical detail from the main path without hiding its
  persisted evidence or troubleshooting information.

## Rejected alternatives

### Continue using leaf rows as the active testcase

Rejected because a draft successor removes the approved predecessor from baseline
queries and makes “active” depend on edit timing rather than publication.

### Derive testcase identity from type and requirement key

Rejected because one requirement commonly needs multiple negative, boundary, or
permission scenarios.

### Automatically switch projects to the newest approved revision

Rejected because webhook analyses and runs would change without an explicit,
audited baseline decision.

### Rewrite old runs and exports to the newest revision

Rejected because this destroys reproducibility and can report expected behavior
that was not used during the original execution.

### Let AI choose lineage whenever similarity is high

Rejected because an incorrect lineage decision can hide a distinct scenario or
transfer approval/automation state to the wrong test. Similarity remains a review
suggestion only.
