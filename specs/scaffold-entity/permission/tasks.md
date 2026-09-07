# tasks — Permission

> **Superseded 2026-09-06** — omnicore v0.74.0 renamed the managed archive slot
> `DeletedAt` → `ArchivedAt` (builder, logical name and the `deletedAt` wire token),
> and this service renamed the physical column `deleted_at` → `archived_at` in the same
> run. The vocabulary below was rewritten accordingly; the decisions it records are
> unchanged. See `../../upgrade/v0.73.0-to-v0.74.0/migration-plan.md`.

Model authority: **`spec.md` (Status: APPROVED)**. Nothing in this directory re-decides the
model; where a task file and the spec disagree, the spec wins. Where a task file's
mechanical detail contradicts a `/docs` section or a layer convention, the doc wins — apply
it and record the deviation in the Notes column here.

Pin: omnicore **`v0.57.0`** · dialect: postgres · read backing: relational · surfaces:
REST + OpenAPI + GraphQL. Generator: `omnicore-gen` **0.33.1**.

**Built on 2026-08-24**, on branch `feature/permission-catalog`, via the codegen path
(`Generation: omnicore-gen`, recorded in `spec.md`). The generator wrote 28 files and
updated 11; three things were hand-written, which is exactly the set the spec predicted:
the composite value object, the one manual rule, and the read derivation.

| # | Layer | Task file | Status | Notes |
|---|---|---|---|---|
| 1 | domain | `task_domain.md` | **done** | aggregate, modes, both rules, 6 notifications. `vos.PermissionKey` hand-written (composite, `written: manual`) |
| 2 | application | `task_application.md` | **done** | 3 commands, 2 queries, `ComputePermission` hand-written, 14 keys × 7 catalogs |
| 3 | web | `task_web.md` | **done** | 4 request/response pairs, 5 REST endpoints + 5 GraphQL fields, the computed `permission`, authz per operation |
| 4 | infra | `task_infra.md` | **done** | schema with `Composite(...)`, repository + 2 constraint bindings, relational view |
| 5 | migrations | `task_migrations.md` | **done** | `0002_permission_manual` up/down; table + partial unique index over the tuple + comments |
| 6 | bootstrap | `task_bootstrap.md` | **done** | `permissions_feature.go` + `wire.go`; the service is constructed and reaches all 6 write handlers |
| 7 | tests | `task_tests.md` | **done** | generator suite + 2 hand-written files for the hand-written code; see the verify table |
| 8 | docs | `task_docs.md` | **done** | README scope table, generated-code row and enforcement row brought in step; the section itself and the seeding claim were already correct |

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
| 1 | §9 "capped at 200 rows per page" (§B Q8) | the framework default, **100** | `read.view.maxLimit` was deliberately left undeclared, mirroring Tenant's approved reasoning that a page-size ceiling is OPERATIONAL state belonging in `microservice.*.yaml` (`query.maxLimit`), not a per-view override pinning every environment. `query.maxLimit` is unset in this project, so the effective cap is 100. The number only ever appeared in §9 as support for admitting two blocking sorts, and 100 is stricter than 200, so that argument holds unchanged. **Maintainer's call to accept or to set `query.maxLimit: 200`.** |
| 2 | §7b rule 6 raises `UnmatchablePermissionKeyNotification` | raised, attached to **`Action`** | The spec named the notification but not the part to attach it to. `Action` is the half the caller has to change (`*:read` → `*:*`), so it is the actionable one. One line to move if the other half is preferred. |
| 3 | §8 "PATCH only", rule 9 as the sole guard | PATCH only **plus `patchExcludes: [Key]`** | Maintainer's call during the build: with the rule alone, the PATCH contract advertised `resource`/`action` as editable, accepted them, and assigned them to the entity before the 422. Now the pair is not a member of the partial body at all, and rule 9 remains as the second layer. `spec.md` §8 updated; regenerated (4 files updated, the 4 `_manual` hooks untouched). |
| 4 | Pin `v0.57.0` | built against **`v0.57.1`** | The project pin moved before this run. `v0.57.1` is fix-only — it stops the `query.sortable` boot advisory from naming Mongo — and touches nothing this entity declares. `spec.md` updated to record the pin actually built against. |

## Final verify — result

Run on 2026-08-24 against pin `v0.57.1`, dialect postgres.

| Level | Result | Evidence |
|---|---|---|
| 1 — boot-trap checklist | **PASS** | No `CHAR(36)`-family id column (postgres-only service). Every `.up.sql` has its `.down.sql`. No `path:"id"`. No `json:`/`db:` tag anywhere in `internal/domain/`. No regex outside `internal/domain/vos/` (the exempt path). `Modes()` lists Archive ⟺ schema declares `ArchivedAt("archived_at")` ⟺ migration carries the column. Every field of the listing Response is `*T` + `,omitempty` (`?fields=` is on). Every scalar `query:` field is a pointer, so none renders as required in OpenAPI. `RequiresService() == true` and all 6 write handlers (3 REST + 3 GraphQL) set `Service: svc`. One schema per file. No children, so the root-archive trap does not apply. Not a SQLite service. |
| 2 — gofmt · vet · build | **PASS** | `gofmt -l internal/ bootstrap/` silent · `go vet -tags postgres ./...` clean · `go build -tags postgres ./...` ok |
| 3 — per-file coverage | **PASS with one accepted deviation** | Every hand-written file at **100%**: `vos/permission_key.go`, `permission_rules_manual.go`, `permission_computed_manual.go`. `permission.go` 100% (incl. the duplicate-key branch, covered by a hand-written stub the generated suite cannot reach). The frozen pair is pinned half by half — the generated test replaces the whole `Key`, so hand-written cases prove `Resource` alone, `Action` alone and a case-only change are each refused, plus that `Description` stays editable. Four generated files at **94.4–94.5%**: the ONLY uncovered statements in the whole entity are the four `if err != nil` guards around `ComputePermission`, which `spec.md` already declares dead by construction — the derivation is a pure string render and cannot fail. Infra repository, domain service and routes at **0%**: the constraint `spec.md` records as unreachable by unit test (they need a live engine and a running app — `/omnicore:qa`'s territory). |
| 4 — regression | **PASS** | `go test -tags postgres ./... -count=1` green across all 8 test packages. No pre-existing QA suite to regress. |
| 0 — reconcile with `spec.md` | **PASS** | §2 acceptance: no colon-joining expression exists outside `permission_key.go`, and all three consumers of the format render through `String()`. §5/§6: no `DELETE` verb, no unarchive route, mutation and command anywhere. §9: `?search=` not served; view is `query.RelationalView("permissions", loader)` with no `Version`. §9 wire shape: the by-id and listing responses carry `id`, `description`, `createdAt`, `updatedAt`, `permission` — `resource` and `action` are absent from every response while remaining filterable AND sortable (`filter:` + `sort:` tags on the Request DTO). §10: the four permission strings gate the operations as declared. Storage: unique index is over the TUPLE `(resource_name, action_name)` `WHERE archived_at IS NULL`, bound to `PermissionAlreadyExistsNotification`. Three deviations recorded above. |

**Not yet done, and not claimed:** the entity has not been booted against Postgres and no
endpoint has been called for real. A green build proves the code compiles, not that the
entity works — that is `/omnicore:run` plus `/omnicore:qa`.
