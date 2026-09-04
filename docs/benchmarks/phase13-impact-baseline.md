# Phase 13 impact-analyzer baseline

Date: 2026-09-04

## Controlled corpus

The frozen development corpus in
`backend/internal/analyzer/testdata/impact_labels.json` contains three seeded
changes and 13 labelled inferred symbols:

- cross-package direct call, transitive callers, and existing test;
- interface implementation and interface-typed consumer;
- generic type usage.

Current result: precision `1.000`, recall `1.000` on this deliberately small
controlled corpus. This verifies implementation behavior only and must not be
reported as real-world thesis effectiveness.

## Microbenchmark

Command:

```bash
go test -run '^$' -bench '^BenchmarkImpactEngine$' -benchmem -benchtime=3x ./internal/analyzer
```

Environment: Darwin/amd64, VirtualApple 2.50 GHz, Go toolchain from `backend/go.mod`.

```text
BenchmarkImpactEngine-8  3  348782736 ns/op  8247496 B/op  71788 allocs/op
```

The benchmark includes package loading, type checking, SSA/CHA construction,
and bounded traversal for the fixture repository. Phase 19 must rerun it on the
frozen thesis environment and a representative multi-package corpus.

## Known gaps

- complex rename and deleted-symbol dependency reconstruction;
- build-tag matrices, cgo, generated files, and multiple nested modules;
- large external-dependency graphs (offline load intentionally falls back);
- real-repository precision/recall and confidence intervals.
