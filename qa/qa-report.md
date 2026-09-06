# QA run report — authcore

- **When:** 2026-09-03 23:34:47 EDT
- **Plan:** `specs/qa/tenant-contract/plan.md + specs/qa/permission-contract/plan.md + specs/qa/role-contract/plan.md`
- **Pin:** omnicore `v0.72.1`
- **Built:** `go build -tags 'postgres'` — engine postgres, no transport tag
- **Profile:** `APP_PROFILE=dev` + `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` on port `8099`
- **Data hygiene:** dedicated throwaway database `authcore_qa_db`, dropped and recreated this run. `authcore_db` is never written to.
- **Run id:** `20260903-233433-6282` — logs under `qa/.logs/20260903-233433-6282/`

## Matrix

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| `tenant` | 155 | 0 | 7 | ✅ GREEN | 2s |
| `permission` | 172 | 0 | 6 | ✅ GREEN | 2s |
| `role` | 243 | 2 | 4 | ❌ RED | 3s |
| `security` | 106 | 0 | 4 | ✅ GREEN | 2s |
| `domain` | 118 | 0 | 2 | ✅ GREEN | 2s |
| **total** | **794** | **2** | **23** | | |

> A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a passing one.
> SKIP keeps its own column and is never folded into the pass count.

## Failures

### Suite `role`

#### D12a tenantID: empty

- **expected:** HTTP 422 · InvalidIDUUIDNotification · field=tenantID
- **received:** HTTP 422 · keys=[InvalidIDUUIDNotification] · field=TenantID

```json
{"success":false,"status":422,"description":"Unprocessable Entity","errors":[{"context":"Role","messages":[{"notificationKey":"InvalidIDUUIDNotification","field":"TenantID","message":"Invalid primary key.","semantic":"Validation"}]}]}
```

#### D12b tenantID: not a uuid — uuid.Parse refuses both through one call

- **expected:** HTTP 422 · InvalidIDUUIDNotification · field=tenantID
- **received:** HTTP 422 · keys=[InvalidIDUUIDNotification] · field=TenantID

```json
{"success":false,"status":422,"description":"Unprocessable Entity","errors":[{"context":"Role","messages":[{"notificationKey":"InvalidIDUUIDNotification","field":"TenantID","value":"tatu","message":"Invalid primary key.","semantic":"Validation"}]}]}
```


Full log: `qa/.logs/20260903-233433-6282/role.log`

## Verdict

❌ RED — 1 of 5 suites — logs: qa/.logs/20260903-233433-6282/
