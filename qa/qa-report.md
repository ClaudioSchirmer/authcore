# QA contract report — authcore

✅ **ALL GREEN** — 2/2 suites · 446 cases · 12s

| | |
|---|---|
| Run | `20260903-001415-89755` |
| Started | 2026-09-03 00:14:15 EDT |
| Elapsed | 12s |
| Profile | `dev` via `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` |
| Built with | `-tags 'postgres'` |
| omnicore pin | `v0.71.0` |
| Data hygiene | throwaway `authcore_qa_db`, dropped and recreated per run |
| Suites declared | 2 |
| Plan | [`specs/qa/permission-catalog/plan.md`](../specs/qa/permission-catalog/plan.md) |
| Logs | `qa/.logs/20260903-001415-89755/` |

## Matrix

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| tenant | 201 | 0 | 0 | ✅ GREEN | 7s |
| permission | 245 | 0 | 0 | ✅ GREEN | 5s |

_A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a suite that passed._
