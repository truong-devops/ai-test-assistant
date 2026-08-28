# Phase 10 evaluation protocol

Phase 10 has three paired comparisons:

| Experiment | Baseline | Treatment | Primary measures |
|---|---|---|---|
| Context impact | Diff only | Diff + project RAG | compile, execution, human acceptance |
| Repair impact | Generate only | Generate, validate, repair | first pass, repair, final success |
| Human effort | Manual writing | AI-assisted generation and review | elapsed effort, acceptance, coverage delta |

Each pair must share `experiment`, `scenario`, and `replicate`. The importer
rejects missing or duplicate variants, invalid percentages, impossible outcome
dependencies, unknown JSON fields, and incomplete experiment metrics. Rates
retain successes and denominators; absent metrics are not silently treated as
failures.

## Reproduce the controlled fixture

```bash
make evaluate
```

This validates `evaluation/datasets/controlled-v1.json` and writes deterministic
`summary.json`, `summary.csv`, `report.md`, and `charts.svg` files under
`evaluation/results/controlled-v1/`. The SHA-256 hash identifies the exact
dataset contents.

To persist the run after PostgreSQL migrations:

```bash
make migrate-up
make evaluate-import
```

Override `EVALUATION_DATASET`, `EVALUATION_OUTPUT`, or
`EVALUATION_DATABASE_URL` when using real trials or a non-default database.
Importing an identical hash is idempotent. Stored observations preserve their
original order so rebuilding a report verifies the same hash.

## Collect real thesis observations

Use the same pinned repository revision, task list, prompt/model configuration,
sandbox limits, and warm-up policy for both variants. Randomize variant order,
record multiple replicates, and have the same review rubric applied without
revealing the variant where practical. Measure active human time consistently.

The bundled controlled fixture only proves that the pipeline and reports are
reproducible. It must not be presented as empirical evidence. Replace its
observations with recorded trials and retain the raw versioned dataset with the
generated hash and artifacts.

Passing execution is not equivalent to a useful test. Report syntactic,
compile, execution, coverage contribution, and human acceptance separately.
