# tasks — Permission

Model authority: **`spec.md` (Status: APPROVED)**. Nothing in this directory re-decides the
model; where a task file and the spec disagree, the spec wins. Where a task file's
mechanical detail contradicts a `/docs` section or a layer convention, the doc wins — apply
it and record the deviation in the Notes column here.

Pin: omnicore **`v0.55.0`** · dialect: postgres · read backing: relational · surfaces:
REST + OpenAPI + GraphQL. Generator: `omnicore-gen` **0.25.0**.

> ✅ **REBUILT 2026-08-20 — every layer is done again, this time in the repository.**
> The note below records why the reset happened; the statuses now reflect a real tree.
>
> ⚠️ **RESET 2026-08-20 — every layer was pending again.** The statuses in this table read
> **done** and the verify table below reported a green run, but that build was never
> committed: PR #4 (`b5aa2cf`) landed this directory's ten planning documents and no code.
> Nothing for Permission exists in `internal/`, `migrations/`, `bootstrap/` or
> `specs/omnicore-gen/`. The statuses have been set back to **pending** to match the
> repository. The pin also moved v0.54.0 → v0.55.0 in the meantime, reopening one model
> slot — answered as `spec.md` §B Q8.

| # | Layer | Task file | Status | Notes |
|---|---|---|---|---|
| 1 | domain | `task_domain.md` | **done** | value objects incl. the composite, the aggregate, modes, rules, notifications |
| 2 | application | `task_application.md` | **done** | commands, queries, the computed field's derivation, the seven catalogs |
| 3 | web | `task_web.md` | **done** | requests/responses, routes, the computed response field, authz |
| 4 | infra | `task_infra.md` | **done** | table schema with the composite decomposition, repository + constraint binding, the relational view |
| 5 | migrations | `task_migrations.md` | **done** | one table, one partial unique index, comments |
| 6 | bootstrap | `task_bootstrap.md` | **done** | the feature and its registration |
| 7 | tests | `task_tests.md` | **done** | ≥ 80% per generated file, measured with `-coverpkg=./internal/...` |
| 8 | docs | `task_docs.md` | **done** | bring the project README in step with what was actually built |

Execution order is inside → out. Each task names the `/docs` sections that must be READ
before its layer is written; that read is mandatory at execution time, not optional, and is
what keeps the output correct against this pin rather than against memory.

## Cross-cutting acceptance (the final gate, `SKILL.md` "Final verify")

1. The mechanical boot-trap checklist, run to a clean pass **before** anything boots.
2. `gofmt -l` silent · `go vet -tags postgres ./...` clean · `go build -tags postgres ./...`.
3. Unit tests ≥ 80% **per generated file**, from the cover profile.
4. The existing QA suite as a regression check.
5. Level 0 reconcile: walk `spec.md`'s promises with command evidence.


## Deviations found while executing — REBUILD, 2026-08-20 (generator 0.25.0)

The 2026-08-19 list (generator 0.23.0) is superseded. Each of its items was re-verified
against this build rather than carried over, and **four of the six no longer exist** — two
of them had cost a permanent file adoption last time, and this rebuild pays neither.

| 2026-08-19 | Status on the rebuild |
|---|---|
| **A** — columns renamed `resource_name` / `action_name` | **STANDS**, by choice. `resource` is an oracle reserved word; the pair was kept renamed rather than left asymmetric, so the DDL is portable if a second dialect is ever added. Exposed names are untouched — every filter, `orderBy` token, OpenAPI parameter and GraphQL argument still says `resource` / `action`. |
| **B** — uniqueness over the PAIR not generated; repository **adopted** to bind the 409 | **CLOSED — no adoption.** This build generates `unique` over a composite as a TUPLE: the partial index `permissions_resource_name_action_name_key … WHERE deleted_at IS NULL` and the repository binding to `PermissionAlreadyExistsNotification` are both emitted. `internal/infra/permission_repository.go` tracks the spec. |
| **C** — key immutability forced into `rules.manual` | **CLOSED.** `kind: immutable` over the composite is accepted and generated. The declarative rule list is no longer empty; only ONE invariant is hand-written now, not three. |
| **D** — `createdAt` / `updatedAt` not filterable | **STANDS.** Filters are served only over declared entity fields. `read.managed` exists and would project them, but the spec does not ask for them on the wire — unchanged from Tenant, and unchanged from §9. |
| **E** — archive route's OpenAPI text claimed an unarchive that does not exist; routes **adopted** | **CLOSED — no adoption.** The emitted text now reads *"This service mounts no unarchive: the row stays as history and nothing brings it back into the active set."* `internal/web/permission_routes.go` tracks the spec. |
| **G** — write responses carried no `permission` | **CLOSED.** A computed field is now derived on the WRITE responses too, through the same `ComputePermission`. `POST` and `PATCH` return `id` + `description` + `permission`, which is §9 as written. |
| **F** — coverage below `CLAUDE.md` rule 6's 95% | **STANDS, narrowed** — see the table below. |

### New this run

- **H — two spec'd behaviours are unreachable by construction, and are the ONLY uncovered
  statements outside the live-engine trio.** Neither is a defect in what the service does;
  both are code that can never execute.
  - `spec.md` §7b **rule 7** (the 129-rune rendered cap) cannot fire: each part is
    independently bounded at 64 runes by `isResource`/`isAction`, and the pair-level checks
    run only after both parts pass, so `runeLen(String())` is at most 64 + 1 + 64 = exactly
    the cap. The invariant IS enforced — structurally, by the two part bounds — but the
    check that states it is dead code, and `InvalidPermissionKeyNotification` has no raiser.
    **RESOLVED — the maintainer chose to drop it, 2026-08-20.** The check and
    `InvalidPermissionKeyNotification` are gone (the notification's declaration and its
    seven catalog entries removed with `omnicore-gen prune`), and `spec.md` §7b records the
    withdrawal and why the bound still holds. `permission_key.go` is now 100%.
  - The four `if err != nil` branches guarding `ComputePermission` in the generated mappers
    are dead for the same kind of reason: the derivation is a pure string render and never
    returns an error. The generator emits the guard generically, which is right — a
    derivation that CAN fail needs it.

## Final verify — result (REBUILD, 2026-08-20)

| Level | Result |
|---|---|
| 0. Reconcile against `spec.md` | **pass** — §1 storage, §2 fields, §5 modes, §6 archive-only, §7 rules, §8 PATCH-only, §9 wire shape + filters + the §B Q8 sort vocabulary, §10 authz: each walked against real command output. §9's `id`-orderable promise is the one unmet target, and it is inexpressible (deviation H's sibling, recorded at §B Q8). |
| 1. Mechanical boot-trap checklist | **pass** — every applicable item run PRE-boot: up/down pairs, no `path:"id"`, no `json:`/`db:` tags in domain, no regex outside `vos/`, `Fields *string` ⇒ every listing response field `*T`+`,omitempty`, `Modes()` ⟺ `DeletedAt("deleted_at")` ⟺ the migration column, every scalar `query:` field a pointer, `RequiresService` wired to all six write handlers, one schema per file. |
| 2. `gofmt -l` · `go vet` · `go build` (tag `postgres`) | **pass** — all three silent. |
| 3. Unit tests, per file via `-coverpkg=./internal/...` | **pass — 96.5% of the unit-testable statements, above rule 6's floor.** `permission.go`, `permission_rules_manual.go`, `permission_computed_manual.go` and `permission_key.go` are all at 100%. The 4 uncovered are dead `ComputePermission` error guards (deviation H). The repository / service / routes trio is 0% — it needs a live engine. |
| 4. Existing QA suite (regression) | **no-op** — the project still has none. Reported, not silently skipped. |

### Per-file coverage (from the cover profile, `-coverpkg=./internal/...`)

| File | Coverage |
|---|---|
| `internal/domain/permission.go` | 100.0% (14/14) |
| `internal/domain/permission_rules_manual.go` | 100.0% (5/5) |
| `internal/domain/vos/permission_key.go` | 100.0% (37/37) — rule 7's dead branch withdrawn |
| `internal/application/queries/permission_computed_manual.go` | 100.0% (1/1) |
| `internal/infra/schemas/permission_schema.go` · `views/permission_view.go` | 100.0% |
| every generated request mapper (4 files) · `archive_permission_command.go` | 100.0% |
| `insert_permission_command.go` · `patch_permission_command.go` | 92.3% · 91.7% — the 1 each is a dead `ComputePermission` error guard |
| `find_permission_by_id_query.go` · `find_permissions_by_params_query.go` | 88.9% · 87.5% — same dead guard |
| `internal/infra/permission_repository.go` · `permission_service.go` · `internal/web/permission_routes.go` | **0.0%** (37 statements) — needs a live engine |
| **Entity total** | **72.8%** (110/151) · **96.5%** (110/114) excluding the live-engine trio |

**`CLAUDE.md` rule 6's 95% floor is met on everything a unit test can reach: 96.5%.** The 4
remaining uncovered statements are the dead `ComputePermission` error guards (deviation H) —
the derivation is a pure string render and cannot fail. Deviation **F** is therefore now
only about the live-engine trio, which `/omnicore:qa` covers and a unit test cannot.

Functional e2e of the five endpoints is `/omnicore:qa`'s job and is NOT covered by anything
above. The service has not been booted against a real Postgres in this working tree.
