# tasks — Tenant

> **Superseded 2026-09-06** — omnicore v0.74.0 renamed the managed archive slot
> `DeletedAt` → `ArchivedAt` (builder, logical name and the `deletedAt` wire token),
> and this service renamed the physical column `deleted_at` → `archived_at` in the same
> run. The vocabulary below was rewritten accordingly; the decisions it records are
> unchanged. See `../../upgrade/v0.73.0-to-v0.74.0/migration-plan.md`.

Control file for the generation of the `Tenant` aggregate.

**Model authority: `spec.md` (Status: APPROVED).** Nothing here restates the model — when a
task and the spec disagree, the spec wins. When a task's mechanical detail contradicts a
`/docs` section or a layer convention, **the doc/convention wins** and the deviation is
recorded at the bottom of this file (a plan detail is a guess made before the layer's rules
were read).

Framework pin: **omnicore `v0.57.0`** · generator `omnicore-gen` (omnicore plugin **0.34.0**). Every doc read
below is against `v0.57.0`.

**Built on 2026-08-24**, on branch `feature/tenant-entity`, from the APPROVED `spec.md`.
Every layer below is done and the four verify levels have run; the results are at the
bottom of this file.

Dialect: `postgres` (single). Build tag: `postgres` (no transport block, so no transport tag).
Read-side posture: relational-served.

## Order

Inside → out. Each layer is executed from its own `task_<layer>.md`, which names the exact
`/docs` sections to read BEFORE generating anything in it.

| # | Layer | Task file | Status |
|---|---|---|---|
| 1 | domain | `task_domain.md` | **done** |
| 2 | application | `task_application.md` | **done** |
| 3 | web | `task_web.md` | **done** |
| 4 | infra | `task_infra.md` | **done** |
| 5 | migrations | `task_migrations.md` | **done** |
| 6 | bootstrap | `task_bootstrap.md` | **done** |
| 7 | tests | `task_tests.md` | **done** |
| 8 | docs refresh | `task_docs.md` | **done** |

**How layers 1–6 are built.** The 1d generation gateway was answered **`omnicore-gen`**
(recorded in `spec.md` as `Generation: omnicore-gen`), so those layers are emitted from
`../../omnicore-gen/tenant.omnicore.yaml` rather than written file by file. The task files are
not discarded: they are the REVIEW CHECKLIST the generated tree is read against, which
is what step 7 of the generator skill asks for. What the generator cannot express is
written by hand and is listed in `../../omnicore-gen/tenant.gen-report.md`:

- `../../../internal/domain/vos/display_name.go`, `description.go`, `tenant_workspace.go` — the
  three `kind: manual` value objects, plus `text_predicates.go`, the shared anti-junk
  helpers they compose;
- `../../../internal/domain/tenant_rules_manual.go` — the four rules, including the `tenant_id`
  derivation and the archive-forces-suspended mutation;
- the tests for both, which no generator writes.

Task 8 is not part of the entity; it is the `../../../README.md` reconciliation the maintainer asked
for, deliberately scheduled last so the README describes what exists rather than what was
intended.

## Final verify (after task 7, before task 8)

Four distinct levels, none merged into another:

1. **Mechanical boot-trap checklist** — the pre-boot greps. Every item is a boot panic a
   grep catches for free, so none of it waits for a boot.
2. **`gofmt -l` · `go vet -tags postgres` · `go build -tags postgres`** — format, vet,
   compile. Necessary, not sufficient. Use an output path that is not the repo root: a bare
   `go build ./...` collides with the `../../../bootstrap` directory and reports an error that is
   about file naming, not about the code.
3. **Unit tests ≥ 80% per generated file**, measured with `-coverpkg=./internal/...` and
   read per file from the cover profile. A bare package percentage does not satisfy this.
4. **Existing QA suite** — regression only. There is none yet, so this level is a no-op and
   is reported as such rather than silently skipped.

Functional e2e of the new endpoints is `/omnicore:qa`'s job, offered after the verify is
green — it is not part of this plan and must not be reported as covered by it.

## Deviations (plan vs what the docs/conventions actually required)

The model's own narrowings are named BEFORE generation, in **`spec.md` § "What generation
will have to write by hand, and where the model narrows"** — that is the model authority and
the one place a reviewer should read them. This table is for what the BUILD turns out to do
differently from the approved model, filled as it happens.

*(An empty table at the end of a build means the tree matches the model exactly; an unfilled
one means nobody looked.)*

| # | Spec says | Built as | Why |
|---|---|---|---|
| 1 | §C.3: the `tenant_id` derivation "runs in the insert command's `ToEntity` — the application-layer mapper — never in `BuildRules`, which is a validation pass and may run more than once" | An `IfInsert` closure in `internal/domain/tenant_rules_manual.go` | `assignedFrom: derived` is the generator's declared path for a server-filled field, and it is what removes `tenant_id` from every write request, command and OpenAPI request schema — which is what §9 requires and what `ToEntity` alone would not achieve. The generator writes no assignment for such a field and asks for a `rules.manual` entry scoped to insert. §C.3's objection is answered by idempotency rather than by placement: the derivation is a pure function of an immutable field, so a second pass cannot produce a second answer. `TestTenantIDDerivationIsIdempotent` pins that, and `TestTenantIDIsDerivedFromWorkspaceOnInsert` pins that a value the entity already carried is overwritten. |
| 2 | An earlier draft of §9 carried "View `maxLimit` = 200" | No `maxLimit` on the view; the framework default page ceiling (100) applies, overridable per environment through `query.maxLimit` in the service yaml | The 200 was a low-risk value this run chose, never a maintainer requirement. The maintainer decided during the build to follow the framework default: the ceiling is operational state, and pinning it on the view is the top of the cascade (view override > yaml > framework default), which takes the choice away from whoever runs the service. `spec.md` §"Decided at low risk" now records that. No yaml key is needed unless an environment wants something other than 100. |
| 3 | §9's filter/sort table marks `updatedAt` sortable, giving four ordering paths | Three: `Name`, `Workspace`, `CreatedAt` — `UpdatedAt` orders by nothing | Maintainer's decision during the build. `UpdatedAt` keeps its full filter set (`eq,gte,lte,gt,lt`), so "changed since" is still answerable; only the ordering is withdrawn. Narrowing the vocabulary is safe in this direction — an undeclared path is a typed 400 rather than a silent free-for-all — and widening it later costs one spec line and a regeneration. |


## Final verify — result

Run on 2026-08-24 against the built tree. Commands are quoted so each line can be re-run.

| Level | Result | Evidence |
|---|---|---|
| 1 — boot-trap checklist | **PASS** | Every applicable item ran BEFORE any boot. `.up.sql` ↔ `.down.sql` twin present; no `path:"id"` on any request; no `json:`/`db:` tag anywhere under `internal/domain/`; no regex or format check inline in a root rule (every one lives in a value object); `Modes()` Archive/Unarchive ⟺ `ArchivedAt("archived_at")` on the schema ⟺ `archived_at TIMESTAMPTZ NULL` in the migration; `?fields=` declared with every `FindTenantsResponse` field a pointer + `,omitempty`; the `sort:` tags are exactly the three declared ordering paths (`Name`, `Workspace`, `CreatedAt`), with `UpdatedAt` carrying `filter:` and no `sort:`; every scalar `query:`-tagged filter a pointer, so none renders REQUIRED in OpenAPI; `RequiresService() … true` matched by `NewTenantServiceImpl(repo)` in the feature and `Service:` on all 8 write handlers (4 REST + 4 GraphQL); one root schema per file. N/A here: native-id sweep (postgres only), SQLite constraint-name reflex, view `Version` bump (a relational read model has none — and the emitted view declares no `Version`), root-archive auto handler (no children). |
| 2 — format · vet · build | **PASS** | `gofmt -l bootstrap/ internal/` prints nothing · `go vet -tags postgres ./...` clean · `go build -tags postgres ./...` clean. |
| 3 — unit tests, per generated FILE | **PASS with one stated deviation** | `go test -tags postgres -coverpkg=./internal/... -coverprofile=… ./internal/...` — all suites green. Read per file from the profile: **27 of 30 files at 100.0%**, total 83.1% of statements. The three exceptions are at **0.0%** and are the ones `spec.md` named in advance: `internal/infra/tenant_repository.go`, `internal/infra/tenant_service.go` and `internal/web/tenant_routes.go` — a repository, a domain-service implementation and route mounts cannot be exercised without a live relational engine and a running app. That is the collision `spec.md` records between `CLAUDE.md` rule 6's 95% floor and the framework's own test division; `/omnicore:qa` is the route to closing it without touching production code. |
| 4 — existing QA suite | **NO-OP, reported rather than skipped** | The service has no QA suite yet, so there is no regression to prove. This level did not run because there was nothing to run, not because it was passed over. |

Functional e2e of the six endpoints — create → read back → CRUD round-trip → archive/unarchive
→ 409 on a stale revision → OpenAPI/GraphQL — is `/omnicore:qa`'s job and is **not** covered
by anything above. A green build proves the code compiles; it does not prove the entity works.
