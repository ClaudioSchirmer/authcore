# Role — generation report

Generated from `specs/omnicore-gen/role.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you already wrote

Written by hand — `kind: manual`, or a composite with `written: manual` — and already in the project. The generator did not open them and cannot tell whether what they enforce still matches what the spec says they enforce — listed so a description that moved does not leave a stale rule behind it:

- **`RoleKey`** — `internal/domain/vos/role_key.go`. A role's stable machine handle: a lowercase slug of 2-64 runes, groups separated by single hyphens so a hyphen can never lead, trail or double. Reuses this project's shared anti-junk predicates. No reserved list and no derivation — those two rules belong to the tenant handle. No normalization: a value that does not already comply is refused, never repaired.

The backing stays a contract across every run: the mappers convert with `vos.<Name>(x)` and read back with `.Value()`, so changing the underlying type of one of these breaks call sites that name neither this report nor the spec.

### `internal/domain/role_rules_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are implemented. It lists them so you can check the file still covers what the spec declares, which is where a rule added to the spec later goes unnoticed.

**`tenant-must-exist`**

> The owning tenant must exist and must not be archived. Ask TenantIsUnavailable; a read join cannot answer this — the predicate is always fk = target.id and roles.tenant_id now points at tenants.id, so a traversal IS expressible — but on an insert a joined field is blank anyway.

- fires under `IfInsert` · raise `RoleTenantDoesNotExistNotification{}` · attach it to `TenantID`

**`granted-permissions-are-in-the-catalog`**

> Every permission this write ADDS must exist in the catalog and still be active. Walk the ADDED entries only (GetAddedItemsOf, not GetCurrentItemsOf) and ask PermissionIsNotInCatalog per entry. Do NOT read the entry's ArchivedAt join field: an added entry carries nil there, which is the same nil a live permission carries, so that test would wave through every entry being added — fail-open on exactly the path this rule exists to close. Judging only ADDED entries is deliberate: a permission retired AFTER a grant must not make the role impossible to rename.

- fires under `IfInsertOrUpdate` · raise `PermissionNotInCatalogNotification{}` · attach it to `Permissions`

**`no-wildcard-grant`**

> A permission with a wildcard in either part cannot be granted on any role. Runs BEFORE the no-escalation rule and that order is load-bearing: Identity.HasPermission PANICS on any argument containing '*', so this rule is what removes the input that would crash the request into a 500 — on exactly the case the escalation rule exists to stop. Over the ADDED entries only; ask PermissionIsWildcard per entry.

- fires under `IfInsertOrUpdate` · raise `CannotGrantWildcardPermissionNotification{}` · attach it to `Permissions`

**`no-privilege-escalation`**

> A caller may only grant a permission they themselves hold. Over the ADDED entries only; ask CallerDoesNotHoldPermission per entry, which runs AFTER the wildcard refusal so every key reaching it is concrete. The *:* superadmin exemption is free: HasPermission returns true for any concrete permission when the claim set contains *:*. Two states, not one — ctx.Identity() nil (auth disabled, dev only, and the framework's own boot guard refuses that profile anywhere else) means NO identity gate; an identity present with an insufficient claim REFUSES. Collapsing them makes the entity unusable in dev.

- fires under `IfInsertOrUpdate` · raise `CannotGrantUnheldPermissionNotification{}` · attach it to `Permissions`

The tests for them are yours too, and the same check applies.

### `internal/infra/role_service_manual.go`

The spec marked these questions as ones the generator cannot answer, so it declared them on the port and left the bodies to you. **They panic until you write them** — the project still builds and boots, and the failure arrives the moment the rule asks, as a 500 with the write rolled back. The outcome being avoided is the other one: a query against the wrong source would compile, return, and mean nothing.

**`TenantIsUnavailable(tenantID domain.ID) bool`**

> Whether the owning tenant is missing, archived, or commercially SUSPENDED. Queries the tenants table by its primary key. A trial tenant is a live customer and is available.

**`PermissionIsNotInCatalog(permissionIDSet []domain.ID) map[domain.ID]bool`**

> Whether this permission id is absent from the catalog or points at an archived permission.

**`PermissionIsWildcard(permissionIDSet []domain.ID) map[domain.ID]bool`**

> Whether the catalog row behind this id carries a wildcard in either part. Answers true when the id is unknown, so an unresolvable grant never reaches the escalation probe.

**`CallerDoesNotHoldPermission(permissionIDSet []domain.ID) map[domain.ID]bool`**

> Whether the requesting caller lacks the permission behind this id. Reads ctx.Identity() through the request-scoped service. It GUARDS THE WILDCARD ITSELF and answers "does not hold" rather than calling through — defence in depth behind the wildcard rule, because a panic on a security rule is a 500.

**`CallerIsSuperAdmin() bool`**

> Whether the caller holds *:*. ctx.Identity().IsSuperAdmin() and nothing else — never HasPermission("*:*"), which panics by design, and never a hand-read of the claim, whose NAME is configurable via authorization.permissionsClaim. Note a resource wildcard is NOT a superadmin grant: role:* reports false.

The method returns a plain value and no error, so decide what an unavailable source means. Failing loudly is the safe default — returning a plausible answer skips the rule this exists to enforce.

**Before writing one of these against another TABLE, check the door.** The facts beside this file run over this entity's own repository, so a question about another aggregate's child table, a control table or a lookup cannot be asked there. If the pinned framework documents a DIRECT schema — one table, no aggregate behind it — that table gets its own anchor and the body keeps the same existence probe and aggregate DSL, in every dialect, inside the caller's transaction. Hand-written SQL and a whole aggregate declared for a table that is only ever counted are both the wrong answer to that question.

### `internal/infra/role_service_manual.go` — bodies the spec no longer asks for

This file still answers for questions the spec has stopped declaring. The generator did not open it and will not: it is yours. **Delete these — a body nothing calls is dead code the next reader has to rule out**, and it goes with the change that stranded it rather than later.

- `func (s *RoleServiceImpl) companions(...)`
- `func (s *RoleServiceImpl) catalogRows(...)`

⚠ **One of these can break the build rather than merely sit there.** A BATCHED per-entry fact (`perEntry`) takes a generated entry carrier declared beside the port, and that type is removed with the fact — so the body naming it stops compiling. The compiler will say `undefined: <Entity><Fact>Entry` and name a symbol; the decision behind it is this line. Deleting the body may also strand the `appdomain` import it was the only user of — the compiler names that one too.

### `internal/application/queries/utils/role_computed_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are filled. It lists them so you can check the file still covers what the spec declares, which is where a field added to the spec later goes unnoticed.

**If this file predates the one-function-per-field shape, the build will not find these.** The derivations used to be one function per READ SHAPE, each handed a whole Result, which meant writing the same derivation twice and keeping the two in step by hand. Each is now one exported function taking the sources it declared — the generator unwraps whatever the shape holds and calls it, and the WRITE responses call the same one. Move each body into the signature below and delete the old per-shape functions.

**`Permissions.Permission` (string)** ← `Resource`, `Action` — ONCE PER ENTRY of `Permissions`

> The permission as a token carries it and a route compares it: resource:action.

```go
func ComputeRoleRolePermissionPermission(ctx *configuration.AppContext, resource string, action string) (string, error)
```

**Until a body is written the field renders absent, and nothing says so** — unlike a manual fact, which panics. The read answers 200, the other columns are correct, and this one is empty on REST, on GraphQL and in the export at once. What the declaration already bought needs no code: `?fields=` on the field fetches its sources instead, `?orderBy=` on it is a typed 400, and the export keeps the column under its label.

### The migration — already yours

The SQL for this entity was written on an earlier run and **was not touched**:

- `migrations/postgres/0003_role_manual.down.sql`
- `migrations/postgres/0003_role_manual.up.sql`

That is permanent, and it is the same posture as the `_manual` rule files: created once, never regenerated. A migration is the only thing here whose effect outlives the file — once it has run anywhere, the framework's tracking table records it as applied, so rewriting the file would change what the file CLAIMS without changing a single table. A service that boots green and fails on the first query touching the change is the outcome being avoided.

**If the shape below no longer matches what that migration created, the fix is a NEW numbered pair in the same folder** — never an edit to one that may have run. Two things are worth being deliberate about, because they are where data is lost: adding a NOT NULL column to a table that already has rows fails unless it carries a default, and a rename done as drop-then-add takes the data with it.

If nothing about the storage changed this run, there is nothing to do here — read the shape as confirmation, not as a task.

**A changed `description:` is a storage change too, on postgres.** The description is stored IN the database — a COMMENT on postgres, mysql and oracle, an `MS_Description` extended property on sqlserver — so that someone holding a connection and not this repository can read it. The code regenerates from the spec; that catalogue entry does not. Rewording a description therefore needs a new pair carrying just the `COMMENT ON` / `sp_addextendedproperty` statements, or the database keeps answering with the old wording.

The shape the regenerated code expects, for `roles`:

**`roles`** — the aggregate root

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `tenant_id` | id | no |  |
| `role_key` | string(64) | no |  |
| `name` | string(120) | no |  |
| `description` | string(500) | no |  |
| `revision` | int64 | no | optimistic concurrency, maintained by the framework |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |
| `deleted_at` | time | yes | archive stamp |

Indexes it expects:

- `roles_tenant_id_role_key_key` — UNIQUE on (tenant_id, role_key), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as RoleKeyAlreadyExistsNotification
- `role_permissions_role_id_permission_id_key` — UNIQUE on (role_id, permission_id), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as RoleAlreadyGrantsPermissionNotification


**`role_permissions`** — the permissions collection (1:N)

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `role_id` | id | no | foreign key to roles |
| `permission_id` | id | no |  |
| `deleted_at` | time | yes | archive stamp |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |

A new pair goes in every dialect this service targets (postgres), numbered after the highest existing one. Every `.up.sql` needs its `.down.sql` or the service refuses to boot.

If this entity has NOT shipped anywhere yet — you are still the only one who ever ran it — deleting the pair above and regenerating writes it fresh from the current spec. That is safe exactly while that is true, and never after.

### The tenant is server-assigned, and the insert accepts one anyway

`TenantID` is declared `assignedFrom: identity-claim` with `bypassMaySet: true`, so it is filled from the caller's identity on every insert and is in no update or patch body. The INSERT body carries it as an OPTIONAL value, for one reason: a super-admin (`*:*`) crosses the row scope, and without a field to name the tenant in they could repair a customer's records and never create one.

**Check the guard, not the mapper.** The mapper applies whatever was sent, deliberately: what refuses a caller who may not state a tenant is `refuseForeignTenant` in `internal/domain/role.go`, which compares `TenantID` against the caller's own and stands down only for the bypass. Two things follow. A caller who names someone else's tenant gets the same refusal a write into that tenant gets — not a silent 201 filed under their own. And if that guard is ever removed or narrowed, this field becomes a tenant anyone can choose.

### Rules that END the validation pass

`tenant-is-a-usable-id` (in `IfInsertOrUpdate`) — declared `guard: true`. After each of them the pass stops if anything has already been rejected: the rules below it, this entity's automatic value-object validation, and the `BuildRules` and value objects of every collection. A clean write is unaffected — the barrier fires only where something was ALREADY refused — so what changes is the SHAPE of a 422: it carries what was found up to the barrier, not that plus every other field the write would have failed on. Check that against what the API consumers expect to receive in one response.

### Value objects validated with the rules, not after them

`TenantID` (in `IfInsertOrUpdate`) — declared `kind: valueObject`. The framework validates every value-object field on every write, but that pass runs AFTER `BuildRules`, so a value object cannot be the premise of the rules below it. Each field above is checked where it is declared instead — by the value object's own answer, never a second one — and excluded from the automatic pass in those verbs, so nothing is reported twice. Two consequences worth checking: the value is now validated BEFORE any barrier that follows, so a 422 that used to hide it behind another failure now carries both; and in the verbs this rule does not cover, the field is still validated at the end, exactly as before.

### Fields the server fills

- **`TenantID`** — written on insert from the `tenant_id` claim of the caller's token. It is **absent from every write request and command**: a client cannot set it, and an update does not touch it. Confirm the callers are authenticated on the insert route — with no identity the field stays empty, and nothing else will say so.

### Per-entry command tests are generated now

The verbs that address ONE entry — add, remove — have generated tests, each one beside the command it covers under `internal/application/commands/` (`<verb>_<collection>_command_test.go`): the entry is applied and projected back, a change keeps its id, an unknown id projects nothing.

**If you wrote your own tests for those mappers before this run**, the package will not compile until you delete them — Go reports it as `redeclared in this block`, which reads like a generator bug and is not one. The generated cases cover the same ground; anything yours asserts beyond them is worth keeping under a different name.

## What to check

### Read what was generated — it is a first draft, not a verdict

This tree is ordinary Go in your repository. **`// Code generated … DO NOT EDIT.` is the Go convention that tells linters to skip a file — it is not a rule that the code may not change.** Review it the way you would review a colleague's: for logic, and for the QUESTION each query asks.

Measure it against what the FRAMEWORK offers, not against what the spec language can say — the language is a subset of the framework and always will be, so "the generator does not emit that" is a fact about the generator and never a reason for the service to do the worse thing. If something here should be a single pass over the table instead of several, or a primitive the framework ships and this spec cannot name, that is worth changing.

Two ways to change it, and the only reason to prefer the first is cost:

1. **Change the spec and regenerate** — survives every later run and every upgrade, and leaves nothing to maintain. Check `omnicore-gen explain keys` before assuming the language cannot say it.
2. **Edit the file, then adopt it** — normal and expected when the framework can do it and the spec cannot say it:

   ```
   omnicore-gen adopt <path> -why '<what the spec could not express>'
   ```

   Adopting re-hashes the file as it stands, so regeneration KEEPS the edit; without it the next run stops rather than overwriting your work. The cost is real and worth saying out loud: an adopted file is PINNED — it stops tracking the spec, so a later framework version's improvements to it never arrive. Every later `generate` prints the file as adopted and `doctor` lists it, which is how it stays visible.

These are the decisions the spec made that are expensive to change later. Read them against what you actually meant.

| Decision | Value | Why it matters |
|---|---|---|
| Storage | flat table `roles` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Collection `Permissions` | `add` → `role:grant` (declared); `remove` → `role:grant` (declared) | These routes hang off `/roles/:id/permissions`. Gated on its own through `children[].permissions`, not by the root's update. Grant that permission before the routes go live — a holder of the root's update alone now gets a 403 here. Removing ONE entry ARCHIVES it (204, no body) and is one-way: there is no per-entry unarchive, so the only way back is a fresh add, with a NEW entry id. |
| Removal | archive (one-way: no unarchive is mounted) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `Key` — per TenantID, scope `active-only` (service-precheck+constraint) | an archived row frees it, so the value can be taken again; a duplicate is refused at the database and reported as `RoleKeyAlreadyExistsNotification`. |
| Data access | tenant | Callers are restricted to their tenant's rows. |
| Crossing the scope | `*:*` | Only a super-admin crosses the scope, and nothing new became grantable — what crosses is the claim they already carry. The wildcard cannot be handed to the framework's HasPermission (it panics on one), so the generated guard calls `Identity.IsSuperAdmin()` instead — the framework's own question for the `*:*` grant, nil-safe and honouring the configured permissions claim. A resource wildcard like `role:*` does NOT answer it. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. Nothing is materialised: there is no collection, no version and no rebuild — a shape change here needs no bump and no operational step. |
| Read join → Tenant | `InnerJoin` on `tenant_id` | An aggregate with no counterpart is NOT returned, on EVERY read through this repository — FindByID included, which the write handlers load through. Legal only because the foreign key is non-nullable. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. |
| Read join → Permission | `InnerJoin` on `permission_id`, from RolePermission | An entry with no counterpart is NOT returned — a silent hole in the collection, not a missing aggregate. Prefer left wherever the relationship is genuinely optional. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. On the entity and OFF the wire: Resource, Action — read by the rules, in no response body and in no export. |

### Where each endpoint answers

Surfaces enabled: **REST · GraphQL**. The three are independent, and every endpoint below is generated from ONE command with ONE permission — a surface is a way in, never a second implementation.

| endpoint | REST | GraphQL |
|---|---|---|
| Create a role | `POST /roles` | `createRole` |
| Update a role (partial) | `PATCH /roles/:id` | `patchRole` |
| Archive a role | `PATCH /roles/:id/archive` | `archiveRole` |
| List roles | `GET /roles` | `roles` |
| Get a role by id | `GET /roles/:id` | `role` |
| Add one `RolePermission` | `POST /roles/:id/permissions` | `addRolePermission` |
| Take out one `RolePermission` | `PATCH /roles/:id/permissions/:rolePermissionId/archive` | `removeRolePermission` |

## What was generated

| What | File |
|---|---|
| the RolePermission child value object | `internal/domain/aggregatevos/role_permission.go` |
| the Role aggregate root, its modes and its rules | `internal/domain/role.go` |
| tests for Role's rules | `internal/domain/role_test.go` |

**Left untouched** (yours, by design):

- `internal/application/queries/utils/role_computed_manual.go` — hand-written rules live here, by design
- `internal/domain/role_rules_manual.go` — hand-written rules live here, by design
- `internal/infra/role_service_manual.go` — hand-written rules live here, by design
- `migrations/postgres/0003_role_manual.down.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it
- `migrations/postgres/0003_role_manual.up.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it

45 file(s) were already up to date.

## What was NOT generated

Owned by other tools:

- the gRPC surface and its proto contract — `/omnicore:implement`
- integration events (publish/subscribe) — `/omnicore:implement`
- read models spanning more than this entity — `/omnicore:scaffold-view`
- changing this entity once it exists — `/omnicore:evolve-entity`, which edits this spec and regenerates. The CODE comes back from the spec; the DATABASE never does — the migration a change needs is written by hand, and that skill's impact map is what carries it, along with the orphans a shrinking spec leaves and everything outside this generator's ownership
- a table with NO aggregate behind it — a control table, a job queue, a lookup, an idempotency ledger. This generator writes aggregates and this spec language cannot say "not one"; that does not mean the framework has no answer. If the pinned version documents a DIRECT schema (one table, no entity), it is the door for those, and `/omnicore:implement` owns wiring it. Neither hand-written SQL nor an entity declared for a table that is only ever queried is the right shape

Read controls this listing does NOT serve: `?search=`. That is a contract, not an omission — sending one is answered with a typed 400 rather than being ignored.

## Framework compatibility and next steps

Verdict: **exact** (project pins v0.73.0)

framework v0.73.0 meets the required v0.73.0

Verify what was generated:

```
go build -tags 'postgres' ./...
go vet -tags 'postgres' ./...
go test -tags 'postgres' ./... -count=1
```

A service that builds with a transport tag (kafka, nats) needs it IN ADDITION to the engine tag on every command above — an engine tag alone may not select a buildable configuration there.

Then exercise the endpoints end to end — a green build proves the code compiles, not that the entity works.
