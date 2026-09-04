# ADR 0001: Immutable AI provenance and context snapshots

- Status: Accepted
- Date: 2026-09-04
- Phase: 12

## Context

Recommendation, generation, and repair previously stored their final domain
records with model and prompt-version labels, but did not store the exact prompt,
context set, token usage, latency, or safe runtime configuration. The review UI
retrieved context again from the current project index. A later re-index could
therefore show evidence different from the evidence originally sent to the LLM.

The thesis experiments require reproducibility and project isolation. Provider
failures and structurally invalid model responses must also remain observable,
even when they do not create a recommendation or generated test.

## Decision

Create three append-only tables:

- `llm_calls` stores one row for every actual provider call;
- `context_snapshots` stores the query and index generation used by that call;
- `context_snapshot_items` stores a denormalized copy of each retrieved chunk.

Each call is associated with exactly one phase subject:

```text
recommendation -> changed_symbol_id
generation     -> recommendation_id
repair         -> generated_test_id
```

Subject identifiers are historical references rather than cascading foreign
keys. Ownership is checked transactionally when the record is created. This
allows evidence to survive replacement of generated domain results during a
worker retry. The call and snapshot tables still belong to an analysis and
project, and are removed if that parent analysis/project is intentionally
deleted.

Database triggers reject updates to all three provenance tables. Deletes remain
available only so the existing parent cascade and future retention policy can
operate deliberately.

The exact instructions, prompt, JSON schema, provider response, and retrieved
chunk contents are persisted. `prompt_hash` covers instructions, prompt, and
schema. `configuration_hash` covers only non-secret runtime information:
provider, model, prompt version, output limit, schema, and retrieval query.
Provider API keys, SCM tokens, webhook secrets, and database credentials are
never part of either record or hash input.

The evidence API has two views:

- `/evidence` returns lightweight metadata for the review UI;
- `/export` returns the complete evidence bundle for authorized audit and
  research export. Application authentication is scheduled for Phase 18, so
  Phase 12 deployments must retain the existing trusted-network boundary.

## Commit-aware index decision

Phase 12 records the exact `project_indexes.ref`, generation, and embedding
model visible when retrieval runs. It does not yet change indexing behavior.
Phase 14 will either:

1. build indexes keyed by immutable commit SHA; or
2. retain a base index and build an immutable PR/MR overlay.

The selected Phase 14 design must preserve the Phase 12 snapshot contract.

## Consequences

Positive:

- Historical review evidence no longer changes after re-indexing.
- Failed and invalid-output calls can be included in reliability/cost analysis.
- Thesis datasets can cite exact source, prompt, model, context, and usage.
- The review UI can display provenance without loading full source payloads.

Costs and risks:

- Full prompts, responses, and context snapshots increase database storage.
- Repository source may contain sensitive data even after current filters.
- Full export must be protected by authentication/RBAC in Phase 18.
- A retention and archival policy is still required before long-term production
  use.

## Alternatives considered

### Store only prompt and context hashes

Rejected because a hash proves identity but cannot reconstruct the exact input
when the source repository or index later changes.

### Keep foreign keys to mutable knowledge chunks

Rejected because index refresh removes stale chunks and would either delete
historical evidence or block normal indexing.

### Store provenance only after a domain result is saved

Rejected because provider failures and invalid structured output would remain
invisible. Calls are recorded immediately after provider completion and before
the phase result is committed.

## Verification

- Unit tests cover stable hashing, cost calculation, validation, and all three
  processor recorder integrations.
- PostgreSQL integration tests cover round trip, re-index stability, update
  rejection, and cross-project context rollback.
- API tests cover summary, export, not-found, and internal-error behavior.
