# tasks — Role

Model authority: **`spec.md` (Status: APPROVED)**. Nothing in this directory re-decides the
model; where a task file and the spec disagree, the spec wins. Where a task file's
mechanical detail contradicts a `/docs` section or a layer convention, **the doc wins** —
apply it and record the deviation in the Notes column here.

Pin: omnicore **`v0.57.0`** · dialect: postgres · read backing: relational · surfaces:
REST + OpenAPI + GraphQL. Generator: `omnicore-gen` **0.33.1** (gate 1d, 2026-08-20) —
the task files below become the REVIEW CHECKLIST for the generated tree.

**Nothing here is built.** Every layer is pending and no code for this entity exists in the
repository.

**This is the first aggregate in the service with a 1:N child and the first with row-level
tenant isolation.** Both cut across every layer, so both are named again inside each task
rather than assumed from here.

| # | Layer | Task file | Status | Notes |
|---|---|---|---|---|
| 1 | domain | `task_domain.md` | **pending** | generated + `vos.RoleKey` and the 4 manual rules written by hand |
| 2 | application | `task_application.md` | **pending** | generated; identity translation is the generator's (`RequestingIdentityPresent` / `RequestingTenant` / `RequestingMayCrossScope`) |
| 3 | web | `task_web.md` | **pending** | generated; 5 REST routes + 2 child routes + 5 GraphQL operations |
| 4 | infra | `task_infra.md` | **pending** | generated + the 4 manual facts written by hand |
| 5 | migrations | `task_migrations.md` | **pending** | generated, then the 2 cross-aggregate FKs added by hand (D4) |
| 6 | bootstrap | `task_bootstrap.md` | **pending** | generated (`bootstrap/roles_feature.go` + `wire.go`) |
| 7 | tests | `task_tests.md` | **pending** | generated suite + 4 hand-written files; see D5 for the two measured deviations |
| 8 | docs | `task_docs.md` | **pending** | README: status table, `### Role` section, API shape. The scoping table needed no change |

## Deviations from `spec.md`, and why

*(filled during execution — one row per place the built tree differs from the approved
model, with the reason. An empty table at the end of a build means the tree matches the
model exactly; an unfilled one means nobody looked.)*

| # | Spec says | Built as | Why |
|---|---|---|---|

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
