# tasks — Role

Model authority: **`spec.md` (Status: APPROVED)**. Nothing in this directory re-decides the
model; where a task file and the spec disagree, the spec wins. Where a task file's
mechanical detail contradicts a `/docs` section or a layer convention, **the doc wins** —
apply it and record the deviation in the Notes column here.

Pin: omnicore **`v0.56.0`** · dialect: postgres · read backing: relational · surfaces:
REST + OpenAPI + GraphQL. Generation: **omnicore-gen** (gate 1d, 2026-08-20) —
the task files below become the REVIEW CHECKLIST for the generated tree.

**This is the first aggregate in the service with a 1:N child and the first with row-level
tenant isolation.** Both cut across every layer, so both are named again inside each task
rather than assumed from here.

| # | Layer | Task file | Status | Notes |
|---|---|---|---|---|
| 1 | domain | `task_domain.md` | **done** | generated + `vos.RoleKey` and the 4 manual rules written by hand |
| 2 | application | `task_application.md` | **done** | generated; identity translation is the generator's (`RequestingIdentityPresent` / `RequestingTenant` / `RequestingMayCrossScope`) |
| 3 | web | `task_web.md` | **done** | generated; 5 REST routes + 2 child routes + 5 GraphQL operations |
| 4 | infra | `task_infra.md` | **done** | generated + the 4 manual facts written by hand |
| 5 | migrations | `task_migrations.md` | **done** | generated, then the 2 cross-aggregate FKs added by hand (D4) |
| 6 | bootstrap | `task_bootstrap.md` | **done** | generated (`bootstrap/roles_feature.go` + `wire.go`) |
| 7 | tests | `task_tests.md` | **done** | generated suite + 4 hand-written files; see D5 for the two measured deviations |
| 8 | docs | `task_docs.md` | **done** | README: status table, `### Role` section, API shape. The scoping table needed no change |

## Deviations from `spec.md`, and why

Recorded per the precedence rule at the top: the doc/convention wins over a plan detail, and
the deviation is written down rather than absorbed.

**D1 — the `key` column is `role_key`.** `key` is a reserved word in the UNION of the five
engines the generator guards against, so it refused it. The EXPOSED name is untouched: every
filter, `?orderBy` token, OpenAPI parameter, GraphQL argument and JSON field still says
`key`. Exactly the move `Permission` already made for `resource` → `resource_name`. `spec.md`
§1's ER sketch says `key`; the table says `role_key`.

**D2 — the service facts are named for the PROBLEM, not the healthy state.** `spec.md` §7
sketched `TenantIsActive`, `ActivePermissionKeys` and `CallerHolds`. The generator's own rule
(`omnicore-gen explain rules`) is that a fact must be named for the problem, because the
generated suite stubs the service so every probe answers "nothing found": a fact spelled
`TenantIsActive` reads false under that stub and therefore means *the tenant is gone*,
turning a perfectly correct spec red on the day it is written. Final names:
`TenantIsUnavailable`, `PermissionIsNotInCatalog`, `PermissionIsWildcard`,
`CallerDoesNotHoldPermission`.

**D3 — R6 and R9a do NOT share one probe.** `spec.md` §7 planned one lookup answering both,
via `ActivePermissionKeys(ids) map[domain.ID]vos.PermissionKey`. A generator fact returns a
scalar (`bool`/`int64`/`float64`/`string`) and is asked once per entry, so a map-valued fact
over the whole collection is not expressible. It became three per-entry facts. The happy path
therefore costs 3 probes per grant, bounded by the 200-permission cap; the refusing paths
short-circuit. Mitigated in the domain by walking the collection ONCE
(`refuseUngrantablePermissions`) instead of three times, and in infra by funnelling all three
through one `findActivePermission`. **Surfaced to the maintainer** as a language gap to
file upstream: a fact that answers for a whole COLLECTION in one query, instead of one
scalar per entry.

**D4 — the two cross-aggregate FKs are hand-written.** The generator writes the PARENT key
(`role_permissions.role_id` → `roles.id`) because a collection's owner is part of the
aggregate it declares. A reference to ANOTHER aggregate is outside the spec language, so
`roles.tenant_id` → `tenants.tenant_id` and `role_permissions.permission_id` →
`permissions.id` were appended to the migration by hand. Legal and expected: the migration is
a HOOK file, and it had not run anywhere.

**D5 — two measured coverage deviations from the 95% floor** (`CLAUDE.md` rule 6):

- `internal/domain/role.go` `BuildRules` — **93.1%**. The single uncovered block is the
  generated `childDuplicate` backstop, and it is **unreachable through any public path**: the
  framework's own carrier refuses a same-business-identity add with
  `EntityAlreadyAddedNotification` before `BuildRules` ever sees two such entries. The
  reachable guarantee is asserted instead
  (`TestRole_TwoIdenticalGrantsCannotCoexist`). Every other function in the file is 100%.
- `internal/domain/aggregatevos/role_permission.go` `BuildRules` — **reported 0.0%, actually
  executed.** The body has ZERO statements (the entry declares no rule of its own), so
  `go tool cover -func` divides 0 by 0 and prints 0.0%. The profile row is
  `role_permission.go:59.97,61.2 0 1` — zero statements, count 1. It is covered by
  `TestRoleRolePermission_RaisesNothingOnItsOwn`.

**D6 — R6 / R9a / R9b judge the ADDED entries, not the whole collection.** Decided by the
maintainer at review on 2026-08-21, and amended into `spec.md` (§7, "What these three rules
judge"). The built tree walked `GetCurrentItemsOf`, which is the literal reading of
`scope: [insertOrUpdate]` over `Permissions[]` — and it made two ordinary operations
impossible: renaming a role whose permission the platform had since retired (422), and
revoking a grant from a role holding permissions the caller had since lost (403 on every
remaining entry). Now `domain.GetAddedItemsOf`. Insert is unchanged, since every entry of a
new role is an added one; a revoke asks nothing, since it adds nothing. Two cases cover the
new guarantee — `TestRole_StoredGrantsAreNotReJudgedOnAnUnrelatedUpdate` and
`TestRole_ANewGrantIsStillJudgedBesideStoredOnes`; `refuseUngrantablePermissions` stays at
100%. The `rules.manual[]` descriptions in `role.omnicore.yaml` were updated to match, so a
future regeneration hands the same instruction.

**D7 — the two cross-aggregate facts memoise per request, and their repositories are keyed
by the owning repository.** Also from the 2026-08-21 review, both inside the hook file
`internal/infra/role_service_manual.go`. `findActivePermission` caches its answer on the
`AppContext` (`Set`/`Get`, the framework's request-scoped store), so the three facts asking
about one entry cost ONE query instead of three — a role at the 200 cap drops from up to 600
round trips inside the write transaction to 200. D3's note that the three probes are
"funnelled through one `findActivePermission`" described the code path only; this is the
query count. Separately, the `sync.Once` that built the tenant and permission repositories
became a map keyed by the owning `*RoleRepository`: the previous form handed the FIRST
engine's repositories to every later service, which is a wrong answer to a security rule
rather than a visible failure. The key is a pointer deliberately — `core.RelationalEngine`
is an interface whose dynamic type is not guaranteed comparable, and a map key that can
panic has no place on the write path.

**Not a deviation — the project's established boundary.** `internal/infra/role_repository.go`,
`internal/infra/role_service.go`, `internal/infra/role_service_manual.go` and
`internal/web/role_routes.go` measure 0%, exactly as their `tenant_*` and `permission_*`
counterparts do: they need a database or a running app, and the boot plus `/omnicore:qa` are
what prove them.

Execution order is inside → out. Each task names the `/docs` sections that must be READ
before its layer is written; that read is mandatory at execution time, not optional, and is
what keeps the output correct against this pin rather than against memory.

## The three traps this model carries into every layer

Named once here, repeated in the tasks that can actually trip them:

1. **The root-archive auto handler archives the WHOLE aggregate.** Wired to the child
   revoke route it type-checks, boots and answers 200 while archiving the entire role.
   It is instantiated at most once per surface.
2. **`HasPermission` panics on any argument containing `*`.** The no-escalation rule is a
   pair precisely so no wildcard string ever reaches it (`spec.md` §7).
3. **An empty tenant claim must fail closed.** The service-wide authorization switch is off
   (Q4), so the claim can be absent; a rule that reads absent as "unconstrained" would serve
   every tenant's rows.

## Cross-cutting acceptance (the final gate, `SKILL.md` "Final verify")

1. The mechanical boot-trap checklist, run to a clean pass **before** anything boots.
2. `gofmt -l` silent · `go vet -tags postgres ./...` clean · `go build -tags postgres ./...`.
3. Unit tests **≥ 95% per generated file** (`CLAUDE.md` rule 6), from the cover profile,
   measured with `-coverpkg=./internal/...`.
4. The existing QA suite as a regression check.
5. Level 0 reconcile: walk `spec.md`'s promises with real command evidence.
