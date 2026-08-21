# Role — generation report

Generated from `specs/omnicore-gen/role.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you already wrote

Written by hand — `kind: manual`, or a composite with `written: manual` — and already in the project. The generator did not open them and cannot tell whether what they enforce still matches what the spec says they enforce — listed so a description that moved does not leave a stale rule behind it:

- **`RoleKey`** — `internal/domain/vos/role_key.go`. The stable machine handle of a role: 2-64 runes, a single lowercase slug of letters, digits and single hyphens, never leading, trailing or doubled. It carries no reserved list and no derivation — it is a handle within one tenant, not a public key. The same anti-junk predicates the other human-typed handles use apply, so a held key and a one-character masher are refused.

The backing stays a contract across every run: the mappers convert with `vos.<Name>(x)` and read back with `.Value()`, so changing the underlying type of one of these breaks call sites that name neither this report nor the spec.

### `internal/domain/role_rules_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are implemented. It lists them so you can check the file still covers what the spec declares, which is where a rule added to the spec later goes unnoticed.

**`tenant-must-be-active`**

> The owner tenant must exist and not be archived. Ask the TenantIsUnavailable fact with the role's TenantID; when it answers true, refuse. On insert only — the tenant is immutable afterwards, so a later archive of the tenant must not freeze edits to roles that already exist under it.

- fires under `IfInsert` · raise `RoleTenantDoesNotExistNotification{}` · attach it to `TenantID`

**`no-wildcard-grant`**

> No granted permission may carry a wildcard in either part. For each entry this write ADDS to Permissions, ask the PermissionIsWildcard fact with the entry's PermissionID and refuse when it answers true. This rule MUST be evaluated before caller-must-hold-granted-permission, and is what guarantees no wildcard string ever reaches Identity.HasPermission, which panics on one. Consequence, accepted: the platform's own *:* role is seeded by migration beside the reserved platform tenant, not created through this API.

- fires under `IfInsertOrUpdate` · raise `CannotGrantWildcardPermissionNotification{}` · attach it to `Permissions`

**`granted-permission-must-be-in-catalog`**

> Every granted permission must exist in the catalog and be active. For each entry this write ADDS to Permissions, ask the PermissionIsNotInCatalog fact with the entry's PermissionID and refuse when it answers true. The database foreign key already guarantees EXISTENCE; this rule earns its keep for the ACTIVE half, and for turning a violation into a readable 422 instead of a raw constraint error.

- fires under `IfInsertOrUpdate` · raise `PermissionNotInCatalogNotification{}` · attach it to `Permissions`

**`caller-must-hold-granted-permission`**

> No privilege escalation: a caller may only grant a permission they themselves hold. For each entry this write ADDS to Permissions, ask the CallerDoesNotHoldPermission fact with the entry's PermissionID and refuse when it answers true. Evaluate this AFTER no-wildcard-grant, so every key reaching it is concrete. A *:* super-admin needs no special case — Identity.HasPermission answers true for any concrete permission when the claim set carries *:*. TWO absent states, and collapsing them breaks one profile or the other: no Identity at all (auth.mode: disabled, dev only) means this rule stands down; an Identity present whose claim is empty or insufficient is REFUSED, fail closed.

- fires under `IfInsertOrUpdate` · raise `CannotGrantUnheldPermissionNotification{}` · attach it to `Permissions`

The tests for them are yours too, and the same check applies.

### `internal/infra/role_service_manual.go`

The spec marked these questions as ones the generator cannot answer, so it declared them on the port and left the bodies to you. **They panic until you write them** — the project still builds and boots, and the failure arrives the moment the rule asks, as a 500 with the write rolled back. The outcome being avoided is the other one: a query against the wrong source would compile, return, and mean nothing.

**`TenantIsUnavailable(tenantID domain.ID) bool`**

> Whether the owner tenant is unknown or archived. Not answerable from this entity's own table — it is a question about the tenants table, so the implementation holds the tenant repository beside its own.

**`PermissionIsNotInCatalog(permissionID domain.ID) bool`**

> Whether this granted permission is absent from the catalog or archived. Asked once per entry. Not answerable declaratively — the column lives on the permissions table, not on this entity's.

**`PermissionIsWildcard(permissionID domain.ID) bool`**

> Whether the catalog row this grant points at carries a wildcard in either part. Asked once per entry, and answered by resolving the id to its vos.PermissionKey. It is what keeps every wildcard string away from Identity.HasPermission, which panics on one — so it must answer TRUE for an unknown id too, rather than let an unresolvable grant fall through to the caller-holds question.

**`CallerDoesNotHoldPermission(permissionID domain.ID) bool`**

> Whether the authenticated caller lacks the permission this grant points at. Asked once per entry, and only for concrete keys — no-wildcard-grant has already refused the wildcards. Implementation: resolve the id to its key; return false when ctx.Identity() is nil (auth disabled, dev only) so the rule stands down; otherwise answer NOT Identity.HasPermission(key). Guard the wildcard here as well and answer true rather than calling through — a panic on a security rule is a 500.

The method returns a plain value and no error, so decide what an unavailable source means. Failing loudly is the safe default — returning a plausible answer skips the rule this exists to enforce.

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

### Per-entry command tests are generated now

The verbs that address ONE entry — add, remove — have generated tests in `internal/application/commands/role_commands_test.go`: the entry is applied and projected back, a change keeps its id, an unknown id projects nothing.

**If you wrote your own tests for those mappers before this run**, the package will not compile until you delete them — Go reports it as `redeclared in this block`, which reads like a generator bug and is not one. The generated cases cover the same ground; anything yours asserts beyond them is worth keeping under a different name.

## What to check

These are the decisions the spec made that are expensive to change later. Read them against what you actually meant.

| Decision | Value | Why it matters |
|---|---|---|
| Storage | flat table `roles` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Removal | archive (reversible) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `Key` — per TenantID, scope `active-only` (service-precheck+constraint) | an archived row frees it, so the value can be taken again; a duplicate is refused at the database and reported as `RoleKeyAlreadyExistsNotification`. |
| Data access | tenant | Callers are restricted to their tenant's rows. |
| Crossing the scope | `*:*` | Only a super-admin crosses the scope, and nothing new became grantable — what crosses is the claim they already carry. The wildcard cannot be handed to the framework's HasPermission (it panics on one), so the generated guard calls `Identity.IsSuperAdmin()` instead — the framework's own question for the `*:*` grant, nil-safe and honouring the configured permissions claim. A resource wildcard like `role:*` does NOT answer it. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. |

## What was generated

| What | File |
|---|---|
| tests for the command mappers | `internal/application/commands/role_commands_test.go` |
| the per-entry commands for role_permissions | `internal/application/commands/role_permission_commands.go` |
| the Role aggregate root, its modes and its rules | `internal/domain/role.go` |
| the vos package documentation | `internal/domain/vos/doc.go` |

**Left untouched** (yours, by design):

- `internal/domain/role_rules_manual.go` — hand-written rules live here, by design
- `internal/infra/role_service_manual.go` — hand-written rules live here, by design
- `migrations/postgres/0003_role_manual.down.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it
- `migrations/postgres/0003_role_manual.up.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it

31 file(s) were already up to date.

## What was NOT generated

Owned by other tools:

- the gRPC surface and its proto contract — `/omnicore:implement`
- integration events (publish/subscribe) — `/omnicore:implement`
- read models spanning more than this entity — `/omnicore:scaffold-view`
- changing this entity once it exists — `/omnicore:evolve-entity`, which edits this spec and regenerates. The CODE comes back from the spec; the DATABASE never does — the migration a change needs is written by hand, and that skill's impact map is what carries it, along with the orphans a shrinking spec leaves and everything outside this generator's ownership

Read controls this listing does NOT serve: `?search=`. That is a contract, not an omission — sending one is answered with a typed 400 rather than being ignored.

## Framework compatibility and next steps

Verdict: **exact** (project pins v0.56.1)

framework v0.56.1 meets the required v0.56.0

Verify what was generated:

```
go build -tags 'postgres' ./...
go vet -tags 'postgres' ./...
go test -tags 'postgres' ./... -count=1
```

A service that builds with a transport tag (kafka, nats) needs it IN ADDITION to the engine tag on every command above — an engine tag alone may not select a buildable configuration there.

Then exercise the endpoints end to end — a green build proves the code compiles, not that the entity works.
