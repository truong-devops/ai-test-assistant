# Evaluation report: controlled-baseline-v1

Controlled deterministic fixture for exercising the Phase 10 evaluation pipeline. Replace these observations with recorded trials before drawing thesis conclusions.

- Schema: `evaluation-v1`
- Dataset SHA-256: `c6ee60f08dba38e073b35f6cc4adc1d6c646daa3b20d8a27cc7f34788e060c03`

A passing test is not treated as automatic evidence of usefulness. Syntax, compile, execution, coverage, and human acceptance remain separate metrics.

## Variant summaries

| Experiment | Variant | N | Syntax | Compile | Execution | Human acceptance | First pass | Repair success | Final success | Mean time (s) | Coverage Δ (pp) |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| CONTEXT_IMPACT | DIFF_ONLY | 2 | 100.00% (2/2) | 50.00% (1/2) | 0.00% (0/2) | 0.00% (0/2) | — | — | — | — | 0.50 |
| CONTEXT_IMPACT | DIFF_RAG | 2 | 100.00% (2/2) | 100.00% (2/2) | 100.00% (2/2) | 100.00% (2/2) | — | — | — | — | 4.00 |
| REPAIR_IMPACT | GENERATE_ONLY | 2 | 50.00% (1/2) | 50.00% (1/2) | 50.00% (1/2) | — | 50.00% (1/2) | — | 50.00% (1/2) | — | — |
| REPAIR_IMPACT | GENERATE_REPAIR | 2 | 100.00% (2/2) | 100.00% (2/2) | 100.00% (2/2) | — | 50.00% (1/2) | 100.00% (1/1) | 100.00% (2/2) | — | — |
| HUMAN_EFFORT | MANUAL | 2 | — | — | — | 100.00% (2/2) | — | — | — | 1950.00 | 3.50 |
| HUMAN_EFFORT | AI_ASSISTED | 2 | — | — | — | 100.00% (2/2) | — | — | — | 660.00 | 4.50 |

## Paired comparisons

### CONTEXT_IMPACT

Treatment `DIFF_RAG` compared with baseline `DIFF_ONLY`.

| Metric | Change |
|---|---:|
| Syntactic validity | +0.00 pp |
| Compile validity | +50.00 pp |
| Execution validity | +100.00 pp |
| Human acceptance | +100.00 pp |
| Mean coverage delta | +3.50 pp |

### REPAIR_IMPACT

Treatment `GENERATE_REPAIR` compared with baseline `GENERATE_ONLY`.

| Metric | Change |
|---|---:|
| Syntactic validity | +50.00 pp |
| Compile validity | +50.00 pp |
| Execution validity | +50.00 pp |
| First-pass success | +0.00 pp |
| Repair success rate | +100.00 % |
| Final success | +50.00 pp |

### HUMAN_EFFORT

Treatment `AI_ASSISTED` compared with baseline `MANUAL`.

| Metric | Change |
|---|---:|
| Human acceptance | +0.00 pp |
| Mean coverage delta | +1.00 pp |
| Mean time reduction | +1290.00 s |
| Mean time reduction | +66.15 % |

