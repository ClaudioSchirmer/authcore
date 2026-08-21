# tasks — Role

Model authority: **`spec.md` (Status: APPROVED)**. Nothing in this directory re-decides the
model; where a task file and the spec disagree, the spec wins. Where a task file's
mechanical detail contradicts a `/docs` section or a layer convention, **the doc wins** —
apply it and record the deviation in the Notes column here.

Pin: omnicore **`v0.55.0`** · dialect: postgres · read backing: relational · surfaces:
REST + OpenAPI + GraphQL. Generation: **omnicore-gen** (gate 1d, 2026-08-20) —
the task files below become the REVIEW CHECKLIST for the generated tree.

**This is the first aggregate in the service with a 1:N child and the first with row-level
tenant isolation.** Both cut across every layer, so both are named again inside each task
rather than assumed from here.

| # | Layer | Task file | Status | Notes |
|---|---|---|---|---|
| 1 | domain | `task_domain.md` | pending | the new role-key value object, the aggregate, the child aggregate value object, modes, the ten rules, the service port, notifications |
| 2 | application | `task_application.md` | pending | root commands, the two child commands, the identity translation, queries with the tenant filter, the seven catalogs |
| 3 | web | `task_web.md` | pending | requests/responses, root routes, the two child routes, the permission gate |
| 4 | infra | `task_infra.md` | pending | table schema with the child declaration, repository + constraint bindings, the cross-aggregate service implementation, the relational view |
| 5 | migrations | `task_migrations.md` | pending | two tables in FK order, two partial unique indexes, comments |
| 6 | bootstrap | `task_bootstrap.md` | pending | the feature, its cross-repository construction, registration |
| 7 | tests | `task_tests.md` | pending | **≥ 95% per generated file** — the project floor, not the skill's 80% |
| 8 | docs | `task_docs.md` | pending | bring the project README in step with what was built |

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
