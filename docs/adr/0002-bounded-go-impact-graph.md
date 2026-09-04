# ADR 0002: Bounded Go impact graph with typed and AST modes

- Status: Accepted
- Date: 2026-09-04
- Phase: 13

## Context

Changed-line overlap identifies direct edits but cannot explain which callers,
interfaces, types, or tests may be affected. Repositories may also fail to
type-check because a dependency is unavailable, a PR is temporarily broken, or
the materialized snapshot deliberately has no network access.

## Decision

Materialize eligible Go/module files from the SCM provider at the immutable
analysis `source_sha`. Load all module packages with `go/packages`, type-check
with `go/types`, construct SSA, and run CHA. The implementation is pinned as
`cha-v1+x-tools-v0.49.0`.

Build structural `CALLS`, `IMPLEMENTS`, and `USES_TYPE` relations, then traverse
both directions from direct changed symbols. Persist only the bounded result:
depth 3, at most 250 nodes, and two minutes wall time by default. Each inferred
edge records a reason code and score. Test functions discovered through incoming
call paths receive `EXISTING_TEST` evidence.

Package loading has SCM and workspace size limits and disables dependency
network access. Any package/type error switches the run to `AST_FALLBACK`.
Fallback parses the same snapshot, retains direct changed symbols, and adds only
syntactically resolvable calls and type uses. The failure reason is persisted.

Changed symbols and the impact graph are committed in one transaction guarded
by the analysis worker lease.

The worker runtime contains the same pinned Go toolchain as the build stage.
`GOTOOLCHAIN=local` prevents runtime downloads; the toolchain is required
because `go/packages` invokes `go list` rather than operating purely in-process.

## Consequences

- Direct evidence never gets confused with inferred impact.
- Cross-package and interface effects become explainable and queryable.
- CHA intentionally over-approximates dynamic dispatch, so scores are ranking
  hints rather than probabilities.
- Offline loading makes the analysis deterministic but dependency-incomplete
  repositories may use the less precise AST fallback.
- Source materialization and SSA have measurable cost and strict caps; Phase 19
  must report runtime and memory on the final corpus.
