# QA report — authcore · tenant-contract + permission-contract + role-contract

- **run:** `20260907-175814-6573` · 2026-09-07 17:58:34 EDT
- **plans:** specs/qa/tenant-contract/plan.md · specs/qa/permission-contract/plan.md · specs/qa/role-contract/plan.md
- **profile:** `APP_PROFILE=qa` · config `qa/microservice.qa.yaml` · built with `-tags 'postgres'` (no transport tag — the yaml declares no `transport:` block)
- **omnicore pin:** `v0.74.0`
- **hygiene:** throwaway database `authcore_qa`, dropped and recreated before this run
- **lanes:** 9 declared, 9 selected

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| tenant | 111 | 0 | 0 | ✅ GREEN | 1s |
| tenant_graphql | 36 | 0 | 0 | ✅ GREEN | 1s |
| permission | 131 | 0 | 0 | ✅ GREEN | 2s |
| permission_graphql | 37 | 0 | 0 | ✅ GREEN | 1s |
| role | 208 | 0 | 4 | ✅ GREEN | 3s |
| role_graphql | 44 | 0 | 0 | ✅ GREEN | 1s |
| domain | 134 | 0 | 3 | ✅ GREEN | 3s |
| security | 116 | 0 | 2 | ✅ GREEN | 3s |
| audit | 38 | 0 | 0 | ✅ GREEN | 1s |

## Skipped — coverage this run did NOT prove

A security or domain family that never executed is the one place where "no failures" reads most like "we are safe". These are named here and counted in their own column, never folded into the pass count.

### lane: role

- **H5.8 the wrong-state 409 is UNREACHABLE on this aggregate**
  EntityIsNotActiveNotification / ConcurrentModificationNotification need a verb this surface does not mount: there is no PUT, no wire field carries a revision, and every wrong-state attempt is intercepted a layer earlier by LoadForWrite's ScopeActive, where it lands as 404 (H6). Asserting a 409 there would encode a promise the pin does not make

- **H6.9 the child stamp-scoped unarchive family**
  The family proves that a child removed on its own BEFORE the root's archive stays archived after the root comes back. This root never comes back: no unarchive mode, no route, no mutation. There is no restore for a stamp to be scoped to

- **H9.46 UnsupportedCapabilityNotification on this entity**
  That key needs a control the DTO DOES declare and the backing cannot serve. FindRolesRequest declares no Search field, so ?search= is refused at the wire wrapper before any engine is reached (H9.6); and nothing is declared under permissions.*, so the schema gate answers first there too (H9.9-H9.11). Asserting it here would assert a lie, and would go RED against a correct service

- **H10.16 the 403 mode-not-allowed shape**
  That shape needs a mode absent from Modes() while its route is still MOUNTED. Role's absent mode (unarchive) has no route at all, so it lands on the 404 arm (H10.1); its absent verb (DELETE) has a registered path, so it lands on 405 (H10.2). No ...NotAllowedNotification is reachable on this entity


### lane: domain

- **R11 the master tenant carries no archive guard**
  by the maintainer's decision (2026-09-06) archiving 'master' is a legitimate platform operation, so there is no refusal to assert; and calling it would suspend the tenant that owns the wildcard role and the bootstrap administrator, invalidating the token every later case depends on

- **P3e the immutability notification is UNREACHABLE from the wire**
  PermissionKeyIsImmutableNotification guards a door no mounted surface can open: PatchPermissionRequest declares only 'description' (patchExcludes: [Permission]) and GraphQL mounts the same shape, so no request can provoke the key. The rule is a belt-and-braces layer behind a structural cut; P3c/P3d assert the EFFECT instead, which is the only honest assertion available

- **RL7- the immutability notification is UNREACHABLE from the wire**
  RoleTenantIsImmutableNotification is declared and enforced in BuildRules, and no mounted request can provoke it: PatchRoleRequest carries key, name and description alone, on REST and on GraphQL both, so the declarative rule is a belt-and-braces layer behind a structural cut. The suite asserts the EFFECT above and does not assert a notification no request can reach


### lane: security

- **S4.6 identity-derived row and field rules on Permission**
  authz.dataAccess: anyone-with-permission (spec §10): the catalog is global, carries no tenant_id and has no owner. Both ToCriteria implementations return the criteria unchanged, BuildRules reads no principal field, and no Restrict is declared — so there is no per-row or per-field boundary on this entity to assert. S4.5 proves the positive form of the same decision

- **S5.7 field-level read authz on Role**
  spec.md §9 declares no ReadCriteria.Restrict: 'Every field a caller may see the row at all for, they may see entirely. Row-level isolation does the work here.' There is no column to find absent for one caller and present for another, no tabular export whose header could be pruned, and no __typename edge to assert — that edge exists only where a restricted field is in the selection. Asserting one would be inventing a rule


---

✅ ALL GREEN — 9/9 suites · 855 cases · 20s
