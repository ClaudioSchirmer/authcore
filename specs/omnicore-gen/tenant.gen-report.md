# Tenant — generation report

Generated from `specs/omnicore-gen/tenant.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you already wrote

Written by hand — `kind: manual`, or a composite with `written: manual` — and already in the project. The generator did not open them and cannot tell whether what they enforce still matches what the spec says they enforce — listed so a description that moved does not leave a stale rule behind it:

- **`DisplayName`** — `internal/domain/vos/display_name.go`. A human-typed display name of a thing (not a person): 2 to 120 runes, at least one letter, at least min(3, length) distinct runes, no run of 4 or more identical runes, trimmed and single-spaced. No word count — single-word company names are ordinary.
- **`Description`** — `internal/domain/vos/description.go`. A human-typed description: 15 to 500 runes, at least two words (a word is a run of 2 or more Unicode letters), at least 5 distinct runes, no run of 4 or more identical runes, and at least one vowel — where any letter outside the Latin script counts as one, so a non-Latin description is never rejected as junk.
- **`TenantWorkspace`** — `internal/domain/vos/tenant_workspace.go`. The tenant's DNS-label handle: 3 to 63 runes matching ^[a-z0-9]+(-[a-z0-9]+)*$, at least 3 distinct runes, no run of 4 or more identical runes, and not a member of the reserved list (platform routes and phishing-prone words). No normalization — a value that does not already comply is refused, never quietly repaired. It also carries the platform's reserved-handle list, which no other text type has.

The backing stays a contract across every run: the mappers convert with `vos.<Name>(x)` and read back with `.Value()`, so changing the underlying type of one of these breaks call sites that name neither this report nor the spec.

### `internal/domain/tenant_rules_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are implemented. It lists them so you can check the file still covers what the spec declares, which is where a rule added to the spec later goes unnoticed.

**`description-differs-from-name-and-workspace`**

> The description must differ from both Name and Workspace under a normalized comparison — case-folded, with whitespace and hyphens collapsed — which catches the pasted-name description.

- fires under `IfInsertOrUpdate` · raise `TenantDescriptionMustDifferNotification{}` · attach it to `Description`

**`archive-forces-suspended`**

> Archiving forces Status to suspended. This is a MUTATION, not a validation: set the field and raise nothing. An archived tenant is never commercially active, so archived+active becomes an unrepresentable state, and unarchiving brings the tenant back suspended by consequence.

- fires under `IfArchive`

The tests for them are yours too, and the same check applies.

### The migration — already yours

The SQL for this entity was written on an earlier run and **was not touched**:

- `migrations/postgres/0001_tenant_manual.down.sql`
- `migrations/postgres/0001_tenant_manual.up.sql`

That is permanent, and it is the same posture as the `_manual` rule files: created once, never regenerated. A migration is the only thing here whose effect outlives the file — once it has run anywhere, the framework's tracking table records it as applied, so rewriting the file would change what the file CLAIMS without changing a single table. A service that boots green and fails on the first query touching the change is the outcome being avoided.

**If the shape below no longer matches what that migration created, the fix is a NEW numbered pair in the same folder** — never an edit to one that may have run. Two things are worth being deliberate about, because they are where data is lost: adding a NOT NULL column to a table that already has rows fails unless it carries a default, and a rename done as drop-then-add takes the data with it.

If nothing about the storage changed this run, there is nothing to do here — read the shape as confirmation, not as a task.

**A changed `description:` is a storage change too, on postgres.** The description is stored IN the database — a COMMENT on postgres, mysql and oracle, an `MS_Description` extended property on sqlserver — so that someone holding a connection and not this repository can read it. The code regenerates from the spec; that catalogue entry does not. Rewording a description therefore needs a new pair carrying just the `COMMENT ON` / `sp_addextendedproperty` statements, or the database keeps answering with the old wording.

The shape the regenerated code expects, for `tenants`:

**`tenants`** — the aggregate root

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `name` | string(120) | no |  |
| `workspace` | string(63) | no |  |
| `description` | string(500) | no |  |
| `status` | string(20) | no |  |
| `revision` | int64 | no | optimistic concurrency, maintained by the framework |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |
| `deleted_at` | time | yes | archive stamp |

Indexes it expects:

- `tenants_workspace_key` — UNIQUE on (workspace), over every row; a duplicate is reported as TenantWorkspaceAlreadyExistsNotification


A new pair goes in every dialect this service targets (postgres), numbered after the highest existing one. Every `.up.sql` needs its `.down.sql` or the service refuses to boot.

If this entity has NOT shipped anywhere yet — you are still the only one who ever ran it — deleting the pair above and regenerating writes it fresh from the current spec. That is safe exactly while that is true, and never after.

## What to check

These are the decisions the spec made that are expensive to change later. Read them against what you actually meant.

| Decision | Value | Why it matters |
|---|---|---|
| Storage | flat table `tenants` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `unarchive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Removal | archive (reversible) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `Workspace` — across the whole table, scope `all` (service-precheck+constraint) | an archived row keeps holding it, so the value is never free again; a duplicate is refused at the database and reported as `TenantWorkspaceAlreadyExistsNotification`. |
| Data access | anyone-with-permission | Any caller holding the permission sees and edits every row. If some callers should only see their own, this is the line to change. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. Nothing is materialised: there is no collection, no version and no rebuild — a shape change here needs no bump and no operational step. |

## What was generated

**Left untouched** (yours, by design):

- `internal/domain/tenant_rules_manual.go` — hand-written rules live here, by design
- `migrations/postgres/0001_tenant_manual.down.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it
- `migrations/postgres/0001_tenant_manual.up.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it

28 file(s) were already up to date.

## What was NOT generated

Owned by other tools:

- the gRPC surface and its proto contract — `/omnicore:implement`
- integration events (publish/subscribe) — `/omnicore:implement`
- read models spanning more than this entity — `/omnicore:scaffold-view`
- changing this entity once it exists — `/omnicore:evolve-entity`, which edits this spec and regenerates. The CODE comes back from the spec; the DATABASE never does — the migration a change needs is written by hand, and that skill's impact map is what carries it, along with the orphans a shrinking spec leaves and everything outside this generator's ownership

Read controls this listing does NOT serve: `?search=`. That is a contract, not an omission — sending one is answered with a typed 400 rather than being ignored.

## Framework compatibility and next steps

Verdict: **exact** (project pins v0.59.0)

framework v0.59.0 meets the required v0.59.0

Verify what was generated:

```
go build -tags 'postgres' ./...
go vet -tags 'postgres' ./...
go test -tags 'postgres' ./... -count=1
```

A service that builds with a transport tag (kafka, nats) needs it IN ADDITION to the engine tag on every command above — an engine tag alone may not select a buildable configuration there.

Then exercise the endpoints end to end — a green build proves the code compiles, not that the entity works.
