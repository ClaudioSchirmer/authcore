# QA report — authcore · tenant-contract

- **run:** `20260906-235521-97189` · 2026-09-06 23:55:29 EDT
- **plan:** `specs/qa/tenant-contract/plan.md`
- **profile:** `APP_PROFILE=qa` · config `qa/microservice.qa.yaml` · built with `-tags 'postgres'` (no transport tag — the yaml declares no `transport:` block)
- **omnicore pin:** `v0.74.0`
- **hygiene:** throwaway database `authcore_qa`, dropped and recreated before this run
- **lanes:** 5 declared, 5 selected

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| tenant | 111 | 0 | 0 | ✅ GREEN | 2s |
| tenant_graphql | 36 | 0 | 0 | ✅ GREEN | 0s |
| domain | 39 | 0 | 1 | ✅ GREEN | 1s |
| security | 39 | 0 | 2 | ✅ GREEN | 2s |
| audit | 14 | 0 | 0 | ✅ GREEN | 0s |

## Skipped — coverage this run did NOT prove

A security or domain family that never executed is the one place where "no failures" reads most like "we are safe". These are named here and counted in their own column, never folded into the pass count.

### lane: domain

- **R11 the master tenant carries no archive guard**
  by the maintainer's decision (2026-09-06) archiving 'master' is a legitimate platform operation, so there is no refusal to assert; and calling it would suspend the tenant that owns the wildcard role and the bootstrap administrator, invalidating the token every later case depends on


### lane: security

- **S3c.11 an identity-derived row or field rule on Tenant**
  authz.dataAccess is 'anyone-with-permission' (spec.md §B Q4: anyone holding the permission sees and edits every row). FindTenantsByParamsQuery.ToCriteria returns the criteria unchanged, BuildRules reads no principal field and no Restrict is declared, so there is no per-row or per-field boundary on this aggregate to assert

- **S3c.12 cross-tenant isolation on Tenant**
  Tenant is the platform registry every other aggregate is scoped BY; it carries no tenant scope of its own, so a cross-tenant read or write is not a boundary this entity has. The isolation cases belong to the scoped aggregates (user, role, group, client), which are out of this round


---

✅ ALL GREEN — 5/5 suites · 239 cases · 8s
