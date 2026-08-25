# tasks — Role

Model authority: **`spec.md` (Status: APPROVED)**. Nothing in this directory re-decides the
model; where a task file and the spec disagree, the spec wins. Where a task file's
mechanical detail contradicts a `/docs` section or a layer convention, **the doc wins** —
apply it and record the deviation in the Notes column here.

Pin: omnicore **`v0.57.1`** · dialect: postgres · read backing: relational · surfaces:
REST + OpenAPI + GraphQL. Generator: `omnicore-gen` **0.36.0** (gate 1d chose codegen on
2026-08-20; the build ran on 2026-08-24) — the task files below were the REVIEW CHECKLIST
for the generated tree.

**Built.** Every layer is done; see the status column and the deviations table below.

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

*(filled during execution — one row per place the built tree differs from the approved
model, with the reason. An empty table at the end of a build means the tree matches the
model exactly; an unfilled one means nobody looked.)*

| # | Spec says | Built as | Why |
|---|---|---|---|
| 1 | §2/§9: a per-grant `resource:action` token is "not expressible"; the read serves the two halves | ONE `permission` token per grant; `resource`/`action` are `hidden` and never reach the wire | The maintainer asked for it mid-build ("só o ID + `role:read`"). It was genuinely inexpressible at `omnicore-gen` 0.35.0 — and finding that out surfaced three generator defects, fixed at 0.36.0, which added `children[].computed`. **`spec.md` §2 and §9 were amended**, so the model and the tree agree |
| 2 | §10 Layer 1 and Layer 3 read as things this entity's code would carry | Declarative: `authz.tenantField` + `authz.bypass: "*:*"` + `authz.noIdentity: stand-down`, emitted by the generator | Better than the spec assumed, and it closes the §7 trap by construction: the emitted guard asks `Identity.IsSuperAdmin()`, never `HasPermission("*:*")`, which panics |
| 6 | — | The generated `childDuplicate` rule blames the field **`RolePermission`** (the entry TYPE) where every other refusal about that collection blames **`Permissions`** (the collection) | Noticed while testing, pinned by a test rather than papered over. Both reach the caller; it is two spellings for one place, and a client parsing field names should know |
| 3 | §7 declares `CallerIsSuperAdmin()` on the service port | Implemented, and **currently has no caller** | The generator answers the super-admin question itself wherever it is asked (commands and `ToCriteria`). The fact is left in place rather than removed — removing a declared port method is the maintainer's call, and `../group/spec.md` may want it |
| 4 | §1's ER sketch shows `UNIQUE (role_id, permission_id) WHERE deleted_at IS NULL` | Present, **hand-written** in the migration | The generator emits the repository's 409 binding for it but not the DDL: migrations are write-once hook files. The index NAME must match the binding exactly or the violation degrades to a raw 500 |
| 5 | `task_tests.md` floor: 95% per file | **Met** on every hand-written domain/application file (100%) and on `role.go` (99.1%). **Under the floor, measured and accepted:** `role.go` `BuildRules` 93.1% · `aggregatevos/role_permission.go` 66.7% · `internal/infra/role_service_manual.go` 47.9% · `role_repository.go`, `role_service.go`, `role_routes.go` 0% | Three different reasons, and only one is a gap. (a) `role_permission.go` is dragged down by an **empty** `BuildRules` — a function with no statements has no counters, so it reports 0% however often it is called; the test that calls it is there. (b) The repository, the service impl and the routes need a **live engine**, which `task_tests.md` already records as accepted; the service's pure predicates and identity paths WERE covered rather than waved through, which is why it reads 47.9% and not 0%. (c) `BuildRules` at 93.1% is the one real remainder: generated code, a handful of branches the generated suite and the hand-written cases together do not reach |

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
