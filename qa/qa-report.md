# QA contract report — authcore

✅ **ALL GREEN** — 1/1 suites · 201 cases · 5s

| | |
|---|---|
| Run | `20260902-192156-59721` |
| Started | 2026-09-02 19:21:56 EDT |
| Elapsed | 5s |
| Profile | `dev` via `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` |
| Built with | `-tags 'postgres'` |
| omnicore pin | `v0.71.0` |
| Data hygiene | throwaway `authcore_qa_db`, dropped and recreated per run |
| Suites declared | 1 |
| Plan | [`specs/qa/id-address-and-filter-values/plan.md`](../specs/qa/id-address-and-filter-values/plan.md) |
| Logs | `qa/.logs/20260902-192156-59721/` |

## Matrix

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| tenant | 201 | 0 | 0 | ✅ GREEN | 5s |

_A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a suite that passed._
