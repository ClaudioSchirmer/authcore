# QA contract report — authcore

❌ **RED** — 1 of 1 suites — logs: `qa/.logs/20260902-104047-12031/`

| | |
|---|---|
| Run | `20260902-104047-12031` |
| Started | 2026-09-02 10:40:47 EDT |
| Elapsed | 7s |
| Profile | `dev` via `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` |
| Built with | `-tags 'postgres'` |
| omnicore pin | `v0.70.0` |
| Data hygiene | throwaway `authcore_qa_db`, dropped and recreated per run |
| Suites declared | 1 |
| Plan | [`specs/qa/id-address-and-filter-values/plan.md`](../specs/qa/id-address-and-filter-values/plan.md) |
| Logs | `qa/.logs/20260902-104047-12031/` |

## Matrix

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| tenant | 199 | 2 | 0 | ❌ RED | 7s |

_A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a suite that passed._

## Failures

### tenant

- **L1 a range operator on a temporal leaf**
  - expected: `HTTP 400 carrying InvalidFilterValueNotification`
  - received: `HTTP 500`
  - body:

    ```json
    {"success":false,"status":500,"description":"Internal Server Error","errors":[{"context":"Server","messages":[{"notificationKey":"InternalServerErrorNotification","message":"Internal server error.","semantic":"Internal"}]}]}
    ```
- **L2 equality on the same temporal leaf**
  - expected: `HTTP 400 carrying InvalidFilterValueNotification`
  - received: `HTTP 500`
  - body:

    ```json
    {"success":false,"status":500,"description":"Internal Server Error","errors":[{"context":"Server","messages":[{"notificationKey":"InternalServerErrorNotification","message":"Internal server error.","semantic":"Internal"}]}]}
    ```

Full log: `qa/.logs/20260902-104047-12031/tenant.log` · server log: `qa/.logs/20260902-104047-12031/tenant-server.log`
