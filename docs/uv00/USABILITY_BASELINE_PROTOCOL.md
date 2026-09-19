# UV-00 usability baseline protocol

This protocol records the optional three-person baseline requested by UV-00.
No participant run has been claimed yet. Fill one row per participant and attach
screen recording/notes only with their consent; do not store credentials or real
customer documents.

## Setup

- Use the synthetic UV-00 checkout fixtures.
- Give the participant the goal, not click-by-click instructions.
- Record machine wait time separately from active interaction time.
- A “help request” is any question equivalent to “what do I click/do next?”.
- Stop and mark a blocker if the participant must use SQL, logs, or a manually
  constructed URL to finish a normal product action.

## Tasks

1. Create a document set, upload v1, approve the source and requirements, generate
   and approve testcases, then export Excel.
2. Find one approved testcase, change its test data/steps, identify the old and
   new versions, and explain which one a project is using.
3. Upload document v2, find affected testcases, update one, and verify that an old
   run/report still shows the old revision.

## Observation sheet

| Participant | Task | Completed | Active time | Machine wait | Help requests | Manual refresh/re-index | Wrong turn/blocker | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| P1 | 1 | Pending | — | — | — | — | — | — |
| P1 | 2 | Pending | — | — | — | — | — | — |
| P1 | 3 | Pending | — | — | — | — | — | — |
| P2 | 1 | Pending | — | — | — | — | — | — |
| P2 | 2 | Pending | — | — | — | — | — | — |
| P2 | 3 | Pending | — | — | — | — | — | — |
| P3 | 1 | Pending | — | — | — | — | — | — |
| P3 | 2 | Pending | — | — | — | — | — | — |
| P3 | 3 | Pending | — | — | — | — | — | — |

## Baseline report

```text
Date / environment / commit:
Participant profiles (non-identifying):
Task completion counts:
Median active time by task:
Median machine wait by task:
Help requests by step:
Manual refresh/re-index count:
Top blockers:
Copy/navigation changes requested:
Evidence location:
```

Repeat the same tasks after UV-09. Do not compare total elapsed time without
separating provider/worker latency from interaction time.
