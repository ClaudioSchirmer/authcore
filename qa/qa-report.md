# QA report — authcore · the seven entity contracts (tenant, permission, role, group, user, client, claim)

- **run:** `20260908-124311-39992` · 2026-09-08 12:44:26 EDT
- **plans:** specs/qa/tenant-contract/plan.md · specs/qa/permission-contract/plan.md · specs/qa/role-contract/plan.md · specs/qa/group-contract/plan.md · specs/qa/user-contract/plan.md · specs/qa/client-contract/plan.md · specs/qa/claim-contract/plan.md
- **profile:** `APP_PROFILE=qa` · config `qa/microservice.qa.yaml` · built with `-tags 'postgres'` (no transport tag — the yaml declares no `transport:` block)
- **omnicore pin:** `v0.74.0`
- **hygiene:** throwaway database `authcore_qa`, dropped and recreated before this run
- **lanes:** 17 declared, 17 selected

| Suite | Pass | Fail | Skip | Verdict | Time |
|---|---:|---:|---:|---|---:|
| tenant | 111 | 0 | 0 | ✅ GREEN | 2s |
| tenant_graphql | 36 | 0 | 0 | ✅ GREEN | 1s |
| permission | 131 | 0 | 0 | ✅ GREEN | 2s |
| permission_graphql | 37 | 0 | 0 | ✅ GREEN | 1s |
| role | 208 | 0 | 4 | ✅ GREEN | 4s |
| role_graphql | 44 | 0 | 0 | ✅ GREEN | 1s |
| group | 214 | 0 | 4 | ✅ GREEN | 3s |
| group_graphql | 51 | 0 | 0 | ✅ GREEN | 2s |
| user | 152 | 0 | 0 | ✅ GREEN | 4s |
| user_graphql | 46 | 0 | 0 | ✅ GREEN | 1s |
| client | 139 | 0 | 0 | ✅ GREEN | 3s |
| client_graphql | 44 | 0 | 0 | ✅ GREEN | 1s |
| claim | 99 | 0 | 4 | ✅ GREEN | 2s |
| claim_graphql | 43 | 0 | 0 | ✅ GREEN | 1s |
| domain | 453 | 0 | 8 | ✅ GREEN | 30s |
| security | 329 | 0 | 13 | ✅ GREEN | 8s |
| audit | 87 | 0 | 0 | ✅ GREEN | 4s |

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


### lane: group

- **K5.8 the wrong-state 409 is N/A on this aggregate**
  EntityIsNotActiveNotification / ConcurrentModificationNotification are not reachable through this aggregate's HTTP surface: there is no PUT, no wire field carries a revision, and the archived-root cases of K6 answer 404 because LoadForWrite runs ScopeActive. Asserting a 409 there would encode a promise the pin does not make

- **K6.12 child stamp-scoped unarchive is N/A**
  Group mounts no unarchive: no mode, no route, no mutation. There is no restore for a child stamp to be scoped to, and GR13 is where the cost of that is asserted instead

- **K9.45 UnsupportedCapabilityNotification is N/A on this entity**
  FindGroupsRequest declares no Search field, so ?search= is refused at the wire wrapper before any engine is consulted (K9.5), and roles.* is declared nowhere, so the schema gate answers first there too (K9.8-K9.11). Asserting UnsupportedCapabilityNotification here would assert a lie, and would go RED against a correct service

- **K10.15 the 403 mode-not-allowed shape is N/A**
  Group's absent mode (unarchive) has no route at all, so it lands on the 404 arm (K10.1); its absent verb (DELETE) has a registered path, so it lands on 405 (K10.2). No …NotAllowedNotification is reachable on this entity


### lane: claim

- **W4.4 the mode-missing-but-mounted 403 is N/A on this entity**
  Modes() is exactly display/insert/update/archive — the four verbs mounted — so no route can reach a mode the aggregate refuses. The 403 shape needs a mounted route whose mode is absent, and this service mounts none for Claim

- **W10.2 the wrong-state 409 is N/A on this entity**
  Claim has no state machine, no transition rule and no wire field carrying a revision a caller could send stale. Every wrong-state attempt is intercepted a layer earlier by the LOAD scope, where it lands as 404 — which W10.3 asserts instead. The same derivation the tenant, permission, role and group rounds each recorded

- **W11.3 no child carries an archive column**
  Claim has no collection and no sibling (spec.md §3, §4), so the stamp-scoped unarchive family — a child archived on its own before the root, staying archived after it returns — has nothing to run against on this aggregate

- **W13.2 the three immutability rules are UNREACHABLE through every mounted surface**
  ClaimNameIsImmutableNotification, ClaimTenantIsImmutableNotification and ClaimValueTypeIsImmutableNotification cannot be provoked through any route this service mounts: patchExcludes [Name, ValueType] removes two fields from the PATCH body and assignedFrom: identity-claim keeps TenantID out of every update body, so the door is closed one layer before the rule. They are backstops behind a closed door — the same shape P3e, RL7- and U15.2b record for their own unreachable rules. W13.1b pins what the wire DOES promise


### lane: domain

- **R11 the master tenant carries no archive guard**
  by the maintainer's decision (2026-09-06) archiving 'master' is a legitimate platform operation, so there is no refusal to assert; and calling it would suspend the tenant that owns the wildcard role and the bootstrap administrator, invalidating the token every later case depends on

- **P3e the immutability notification is UNREACHABLE from the wire**
  PermissionKeyIsImmutableNotification guards a door no mounted surface can open: PatchPermissionRequest declares only 'description' (patchExcludes: [Permission]) and GraphQL mounts the same shape, so no request can provoke the key. The rule is a belt-and-braces layer behind a structural cut; P3c/P3d assert the EFFECT instead, which is the only honest assertion available

- **RL7- the immutability notification is UNREACHABLE from the wire**
  RoleTenantIsImmutableNotification is declared and enforced in BuildRules, and no mounted request can provoke it: PatchRoleRequest carries key, name and description alone, on REST and on GraphQL both, so the declarative rule is a belt-and-braces layer behind a structural cut. The suite asserts the EFFECT above and does not assert a notification no request can reach

- **GR8-b GroupKeyIsImmutableNotification is UNREACHABLE from the wire**
  update.patchExcludes: [Key] removes the field from the PATCH body on REST and from PatchGroupInput on GraphQL (L1.4c), so no mounted request can provoke GroupKeyIsImmutableNotification. GR8- asserts the EFFECT instead, which is the only honest assertion available

- **GR8b-b GroupTenantIsImmutableNotification is UNREACHABLE from the wire**
  PatchGroupRequest carries name and description alone, so nothing can move tenantID through a PATCH. The rule stands as the backstop that answers if the field ever rejoins the body; GR8b- asserts the effect

- **U13.4- a group carrying a WILDCARD-bearing role**
  U13.4 — CannotJoinWildcardGroupNotification is UNREACHABLE from the wire: Group's own rule refuses attaching the wildcard-bearing seeded role to any group, no other wildcard-bearing role can be created, and migration 0012 seeds the wildcard onto a role and onto no group. The guard is defence-in-depth behind a state no request can produce

- **U15.2b and the User cap is recorded as UNREACHABLE rather than claimed**
  TooManyClaimsForUserNotification cannot be provoked through this API: a user's values must point at definitions in its own tenant, and Claim's catalog budget stops the twentieth-first definition from existing (U15.2). The guard is a backstop behind a boundary the caller meets one level earlier — the same shape P3e, RL7- and GR8-b record for their own unreachable rules. U15.1 proves the passing side at exactly 20

- **CL9.3 CONSEQUENCE, recorded rather than filed as a defect**
  Mounted this way, a defaultValue once set cannot be withdrawn by any route: PATCH cannot express null and no PUT is mounted. Removing it means archiving the definition and recreating it — which CL10 shows costs a new id and an explicit re-set on every edge. Recorded at the maintainer's instruction, 2026-09-08


### lane: security

- **S4.6 identity-derived row and field rules on Permission**
  authz.dataAccess: anyone-with-permission (spec §10): the catalog is global, carries no tenant_id and has no owner. Both ToCriteria implementations return the criteria unchanged, BuildRules reads no principal field, and no Restrict is declared — so there is no per-row or per-field boundary on this entity to assert. S4.5 proves the positive form of the same decision

- **S5.7 field-level read authz on Role**
  spec.md §9 declares no ReadCriteria.Restrict: 'Every field a caller may see the row at all for, they may see entirely. Row-level isolation does the work here.' There is no column to find absent for one caller and present for another, no tabular export whose header could be pruned, and no __typename edge to assert — that edge exists only where a restricted field is in the selection. Asserting one would be inventing a rule

- **S6.8 field-level read authz on Group**
  spec.md §9 declares no ReadCriteria.Restrict for Group: 'Every field a caller may see the row at all for, they may see entirely. Row-level isolation does the work.' There is no column to find absent for one caller and present for another, no tabular export whose header could be pruned, and no __typename edge to assert — that edge exists only where a restricted field is in the selection. S6.7 proves the row-level boundary that does the work instead

- **S7.4b there is therefore no FieldAccessForbiddenNotification to assert**
  spec.md §9 declares no ReadCriteria.Restrict for User. The hash is kept off the wire by the Response DTOs declaring no such member, and out of the framework's own copies by RedactedField(InSync/InAudit) — two mechanisms, neither of which is a per-caller decision. S7.4a proves the boundary holds for the most privileged principal in the service; qa/domain.sh U25 proves the audit half

- **S7.5a the user:change-password deployment story**
  spec.md §10 records that user:change-password must reach EVERY user or nobody can rotate their own credential once authorization is on. The suite proves the RESTRICTED token always carries it (qa/domain.sh U23, where the grant is EMBEDDED rather than filtered — which is what keeps the flow from deadlocking), but it does NOT prove any tenant's default role grants it, because no such default role exists in this service to inspect. S7.1n above shows the gate closing on a principal that lacks it

- **S7.5b the externalValidator path**
  auth.externalValidator is configured in no profile, so a locally-valid token is never refused by a second opinion. There is nothing to assert and nothing is claimed

- **S8.4b there is therefore no FieldAccessForbiddenNotification to assert**
  spec.md §9 declares no ReadCriteria.Restrict for Client: the hashes are off the wire because no Response DTO declares them (and RedactedField keeps them out of the framework's own copies — qa/audit.sh A69+ proves that half), and tenantStatus is hidden at the join. S8.4a proves the boundary holds for the most privileged principal in the service

- **S8.6a the trusted-proxy half of the allow-list**
  No profile configures a trusted proxy, so the mint judges the SOCKET address — which is what made C-MINT provable from localhost. Whether a spoofed X-Forwarded-For could walk through a REAL deployment behind a load balancer is /omnicore:configure territory (spec.md §F prerequisite 1), and until it is configured the allow-list constrains local semantics only. Prerequisite 2 stands with it: the list constrains where a token is OBTAINED, never where it is USED

- **S8.6b the client token's own contract**
  The claim vocabulary (identity_kind, name, permissions, x_* values reaching the token), the deliberate absence of a lockout on this route, and the absent /refresh companion belong to the token route's own round (plan §0b, maintainer 2026-09-08: exercised, not owned). C-SEC1 proves the one bit this round cannot avoid: a minted secret signs in and its token says client

- **S9.2 Claim declares no identity-derived BuildRules clause**
  There is no owner-check and no 'unless admin' on this aggregate: a claim definition confers nothing, so there is no escalation surface and spec.md §7 declares no caller-identity fact. The layer that would carry one is empty BY DESIGN. What stands in its place is Layer 3 plus the assignedFrom seat, and S9.3 is where both are proven

- **S9.3e a RESOURCE wildcard does not cross the row scope**
  authz.bypass is the literal *:*; claim:* is an ordinary permission that opens the four verbs and crosses no tenant. Provoking it needs a principal holding claim:* and nothing else, which no round has provisioned — recorded as UNPROVEN rather than inferred from the yaml

- **S9.3f noIdentity: stand-down is UNPROVABLE in this posture**
  ctx.Identity() nil is reachable only under auth.mode: disabled, which the framework's own boot guard permits in dev alone — and every profile this suite boots runs jwt. The branch is real (find_claims_by_params_query.go) and it serves EVERY row by design, so a scoped entity is usable on the machine it is first tried on; asserting it would need a third boot on a disabled profile, which this round did not take on

- **S9.4a no field-level read authz exists on this entity**
  spec.md §9: a definition is vocabulary, not a secret, and defaultValue is the only field carrying a business value at all — any holder of claim:read in the tenant is entitled to it. So no ToCriteria calls Restrict, no FieldAccessForbiddenNotification can be provoked, and the __typename edge of X3 is a parity case rather than a boundary case. S9.4b asserts the posture instead of the absence


---

✅ ALL GREEN — 17/17 suites · 2224 cases · 75s
