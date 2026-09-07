# QA report — authcore · tenant-contract + permission-contract

- **run:** `20260907-171343-77476` · 2026-09-07 17:13:57 EDT
- **plans:** specs/qa/tenant-contract/plan.md · specs/qa/permission-contract/plan.md
- **profile:** `APP_PROFILE=qa` · config `qa/microservice.qa.yaml` · built with `-tags 'postgres'` (no transport tag — the yaml declares no `transport:` block)
- **omnicore pin:** `v0.74.0`
- **hygiene:** throwaway database `authcore_qa`, dropped and recreated before this run
- **lanes:** 7 declared, 7 selected

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| tenant | 111 | 0 | 0 | ✅ GREEN | 1s |
| tenant_graphql | 36 | 0 | 0 | ✅ GREEN | 0s |
| permission | 131 | 0 | 0 | ✅ GREEN | 2s |
| permission_graphql | 37 | 0 | 0 | ✅ GREEN | 1s |
| domain | 87 | 0 | 2 | ✅ GREEN | 2s |
| security | 71 | 0 | 1 | ✅ GREEN | 2s |
| audit | 23 | 0 | 0 | ✅ GREEN | 1s |

## Skipped — coverage this run did NOT prove

A security or domain family that never executed is the one place where "no failures" reads most like "we are safe". These are named here and counted in their own column, never folded into the pass count.

### lane: domain

- **R11 the master tenant carries no archive guard**
  by the maintainer's decision (2026-09-06) archiving 'master' is a legitimate platform operation, so there is no refusal to assert; and calling it would suspend the tenant that owns the wildcard role and the bootstrap administrator, invalidating the token every later case depends on

- **P3e the immutability notification is UNREACHABLE from the wire**
  PermissionKeyIsImmutableNotification guards a door no mounted surface can open: PatchPermissionRequest declares only 'description' (patchExcludes: [Permission]) and GraphQL mounts the same shape, so no request can provoke the key. The rule is a belt-and-braces layer behind a structural cut; P3c/P3d assert the EFFECT instead, which is the only honest assertion available


### lane: security

- **S4.6 identity-derived row and field rules on Permission**
  authz.dataAccess: anyone-with-permission (spec §10): the catalog is global, carries no tenant_id and has no owner. Both ToCriteria implementations return the criteria unchanged, BuildRules reads no principal field, and no Restrict is declared — so there is no per-row or per-field boundary on this entity to assert. S4.5 proves the positive form of the same decision


---

✅ ALL GREEN — 7/7 suites · 496 cases · 14s
