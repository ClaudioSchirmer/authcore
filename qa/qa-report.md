# QA run report — authcore

- **When:** 2026-09-06 15:46:41 EDT
- **Plan:** `specs/qa/tenant-contract/plan.md + specs/qa/permission-contract/plan.md + specs/qa/role-contract/plan.md`
- **Pin:** omnicore `v0.73.0`
- **Built:** `go build -tags 'postgres'` — engine postgres, no transport tag
- **Profile:** `APP_PROFILE=dev` + `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` on port `8099`
- **Data hygiene:** dedicated throwaway database `authcore_qa_db`, dropped and recreated this run. `authcore_db` is never written to.
- **Run id:** `20260906-154626-22802` — logs under `qa/.logs/20260906-154626-22802/`

## Matrix

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| `tenant` | 155 | 0 | 7 | ✅ GREEN | 2s |
| `permission` | 172 | 0 | 6 | ✅ GREEN | 3s |
| `role` | 245 | 0 | 4 | ✅ GREEN | 3s |
| `security` | 106 | 0 | 4 | ✅ GREEN | 1s |
| `domain` | 118 | 0 | 2 | ✅ GREEN | 3s |
| **total** | **796** | **0** | **23** | | |

> A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a passing one.
> SKIP keeps its own column and is never folded into the pass count.

## Verdict

✅ ALL GREEN — 5/5 suites · 796 cases · 15s
