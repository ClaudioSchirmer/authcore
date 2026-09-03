# QA contract report — authcore

❌ **RED** — 1 of 2 suites — logs: `qa/.logs/20260903-124644-27290/`

| | |
|---|---|
| Run | `20260903-124644-27290` |
| Started | 2026-09-03 12:46:44 EDT |
| Elapsed | 11s |
| Profile | `dev` via `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` |
| Built with | `-tags 'postgres'` |
| omnicore pin | `v0.72.0` |
| Data hygiene | throwaway `authcore_qa_db`, dropped and recreated per run |
| Suites declared | 2 |
| Plan | [`specs/qa/permission-catalog/plan.md`](../specs/qa/permission-catalog/plan.md) |
| Logs | `qa/.logs/20260903-124644-27290/` |

## Matrix

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| tenant | 200 | 1 | 0 | ❌ RED | 5s |
| permission | 245 | 0 | 0 | ✅ GREEN | 6s |

_A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a suite that passed._

## Failures

### tenant

- **J8 GraphQL is not a public route**
  - expected: `HTTP 401 carrying MissingAuthorizationNotification`
  - received: `HTTP 200`
  - body:

    ```json
    {"data":{"__typename":null},"errors":[{"message":"no resolver for field __typename","path":["__typename"]}]}
    ```

Full log: `qa/.logs/20260903-124644-27290/tenant.log` · server log: `qa/.logs/20260903-124644-27290/tenant-server.log`
