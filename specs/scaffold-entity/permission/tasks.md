# tasks — Permission

Model authority: **`spec.md` (Status: APPROVED)**. Nothing in this directory re-decides the
model; where a task file and the spec disagree, the spec wins. Where a task file's
mechanical detail contradicts a `/docs` section or a layer convention, the doc wins — apply
it and record the deviation in the Notes column here.

Pin: omnicore **`v0.57.0`** · dialect: postgres · read backing: relational · surfaces:
REST + OpenAPI + GraphQL. Generator: `omnicore-gen` **0.33.1**.

**Nothing here is built.** Every layer is pending and no code for this entity exists in
`internal/`, `migrations/`, `bootstrap/` or `specs/omnicore-gen/`.

| # | Layer | Task file | Status | Notes |
|---|---|---|---|---|
| 1 | domain | `task_domain.md` | **pending** | value objects incl. the composite, the aggregate, modes, rules, notifications |
| 2 | application | `task_application.md` | **pending** | commands, queries, the computed field's derivation, the seven catalogs |
| 3 | web | `task_web.md` | **pending** | requests/responses, routes, the computed response field, authz |
| 4 | infra | `task_infra.md` | **pending** | table schema with the composite decomposition, repository + constraint binding, the relational view |
| 5 | migrations | `task_migrations.md` | **pending** | one table, one partial unique index, comments |
| 6 | bootstrap | `task_bootstrap.md` | **pending** | the feature and its registration |
| 7 | tests | `task_tests.md` | **pending** | ≥ 80% per generated file, measured with `-coverpkg=./internal/...` |
| 8 | docs | `task_docs.md` | **pending** | bring the project README in step with what was actually built |

Execution order is inside → out. Each task names the `/docs` sections that must be READ
before its layer is written; that read is mandatory at execution time, not optional, and is
what keeps the output correct against this pin rather than against memory.

## Cross-cutting acceptance (the final gate, `SKILL.md` "Final verify")

1. The mechanical boot-trap checklist, run to a clean pass **before** anything boots.
2. `gofmt -l` silent · `go vet -tags postgres ./...` clean · `go build -tags postgres ./...`.
3. Unit tests **≥ 95% per generated file** (`CLAUDE.md` rule 6), from the cover profile,
   measured with `-coverpkg=./internal/...`.
4. The existing QA suite as a regression check.
5. Level 0 reconcile: walk `spec.md`'s promises with command evidence.


## Deviations found while executing

*(filled during execution — one row per place the built tree differs from the approved
model, with the reason. An empty table at the end of a build means the tree matches the
model exactly; an unfilled one means nobody looked.)*

| # | Spec says | Built as | Why |
|---|---|---|---|

## Final verify — result

Filled after the five levels above have actually run. Empty until then: a verify table is
evidence, and there is nothing to be evidence of yet.
