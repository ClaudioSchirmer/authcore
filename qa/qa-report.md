# QA contract report — authcore

✅ **ALL GREEN** — 2/2 suites · 450 cases · 13s

| | |
|---|---|
| Run | `20260903-160110-43663` |
| Started | 2026-09-03 16:01:10 EDT |
| Elapsed | 13s |
| Profile | `dev` via `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` |
| Built with | `-tags 'postgres'` |
| omnicore pin | `v0.72.1` |
| Data hygiene | throwaway `authcore_qa_db`, dropped and recreated per run |
| Suites declared | 2 |
| Plan | [`specs/qa/permission-catalog/plan.md`](../specs/qa/permission-catalog/plan.md) |
| Logs | `qa/.logs/20260903-160110-43663/` |

## Matrix

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| tenant | 205 | 0 | 0 | ✅ GREEN | 7s |
| permission | 245 | 0 | 0 | ✅ GREEN | 6s |

_A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a suite that passed._
