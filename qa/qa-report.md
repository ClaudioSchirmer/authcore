# QA run report — authcore

- **When:** 2026-09-03 17:22:28 EDT
- **Plan:** `specs/qa/tenant-contract/plan.md`
- **Pin:** omnicore `v0.72.1`
- **Built:** `go build -tags 'postgres'` — engine postgres, no transport tag
- **Profile:** `APP_PROFILE=dev` + `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` on port `8099`
- **Data hygiene:** dedicated throwaway database `authcore_qa_db`, dropped and recreated this run. `authcore_db` is never written to.
- **Run id:** `20260903-172221-65908` — logs under `qa/.logs/20260903-172221-65908/`

## Matrix

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| `tenant` | 155 | 0 | 7 | ✅ GREEN | 1s |
| `domain` | 48 | 0 | 1 | ✅ GREEN | 1s |
| `security` | 41 | 0 | 3 | ✅ GREEN | 1s |
| **total** | **244** | **0** | **11** | | |

> A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a passing one.
> SKIP keeps its own column and is never folded into the pass count.

## Verdict

✅ ALL GREEN — 3/3 suites · 244 cases · 7s
