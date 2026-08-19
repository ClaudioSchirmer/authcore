# Tenant — generation report

Generated from `specs/omnicore-gen/tenant.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you already wrote

Declared as `kind: manual` and already in the project. The generator did not open them and cannot tell whether what they enforce still matches what the spec says they enforce — listed so a description that moved does not leave a stale rule behind it:

- **`DisplayName`** — `internal/domain/vos/display_name.go`. A human-typed display name of a thing (not a person): 2 to 120 runes, at least one letter, at least min(3, length) distinct runes, no run of 4 or more identical runes, trimmed and single-spaced. No word count — single-word company names are ordinary.
- **`Description`** — `internal/domain/vos/description.go`. A human-typed description: 15 to 500 runes, at least two words (a word is a run of 2 or more Unicode letters), at least 5 distinct runes, no run of 4 or more identical runes, and at least one vowel — where any letter outside the Latin script counts as one, so a non-Latin description is never rejected as junk.
- **`TenantWorkspace`** — `internal/domain/vos/tenant_workspace.go`. The tenant's DNS-label handle: 3 to 63 runes matching ^[a-z0-9]+(-[a-z0-9]+)*$, at least 3 distinct runes, no run of 4 or more identical runes, and not a member of the reserved list (platform routes and phishing-prone words). No normalization — a value that does not already comply is refused, never quietly repaired. It also carries DeriveTenantID(), the UUIDv5 derivation that produces the tenant's public key.

The backing stays a contract across every run: the mappers convert with `vos.<Name>(x)` and read back with `.Value()`, so changing the underlying type of one of these breaks call sites that name neither this report nor the spec.

### Fields declared DERIVED, which nothing here computes

- **`TenantID`** — `assignedFrom: derived` took it out of every write request, command and OpenAPI request schema, so a client cannot set it. WRITING it is yours: a `rules.manual` entry scoped to insert, assigning it from the fields it derives from. Idempotent by construction when it is a pure function of an immutable field, which is the case this exists for.

### `internal/domain/tenant_rules_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are implemented. It lists them so you can check the file still covers what the spec declares, which is where a rule added to the spec later goes unnoticed.

**`derive-tenant-id`**

> On insert, set TenantID to Workspace.DeriveTenantID() — the UUIDv5 of the service's TenantIDNamespace over the workspace handle. The field is server-derived and reaches no write DTO, so this is the only place the value is ever produced.

- fires under `IfInsert`

**`tenant-id-matches-workspace`**

> TenantID must equal Workspace.DeriveTenantID(). The value is computed by the rule above, so this can only fail through a bug in that derivation or a hand-written row — which is exactly what it guards.

- fires under `IfInsertOrUpdate` · raise `TenantIDDerivationMismatchNotification{}` · attach it to `TenantID`

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
| `tenant_id` | id | no |  |
| `name` | string(120) | no |  |
| `workspace` | string(63) | no |  |
| `description` | string(500) | no |  |
| `status` | string(20) | no |  |
| `revision` | int64 | no | optimistic concurrency, maintained by the framework |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |
| `deleted_at` | time | yes | archive stamp |

Indexes it expects:

- `tenants_tenant_id_key` — UNIQUE on (tenant_id), over every row; a duplicate is reported as TenantIDAlreadyExistsNotification
- `tenants_workspace_key` — UNIQUE on (workspace), over every row; a duplicate is reported as TenantWorkspaceAlreadyExistsNotification


A new pair goes in every dialect this service targets (postgres), numbered after the highest existing one. Every `.up.sql` needs its `.down.sql` or the service refuses to boot.

If this entity has NOT shipped anywhere yet — you are still the only one who ever ran it — deleting the pair above and regenerating writes it fresh from the current spec. That is safe exactly while that is true, and never after.

### Fields the server fills


## What to check

These are the decisions the spec made that are expensive to change later. Read them against what you actually meant.

| Decision | Value | Why it matters |
|---|---|---|
| Storage | flat table `tenants` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `unarchive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Removal | archive (reversible) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `TenantID` — scope `all` (constraint-only) | an archived row keeps holding it, so the value is never free again; a duplicate is refused at the database and reported as `TenantIDAlreadyExistsNotification`. |
| Unique | `Workspace` — scope `all` (service-precheck+constraint) | an archived row keeps holding it, so the value is never free again; a duplicate is refused at the database and reported as `TenantWorkspaceAlreadyExistsNotification`. |
| Data access | anyone-with-permission | Any caller holding the permission sees and edits every row. If some callers should only see their own, this is the line to change. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. |

## What was generated

| What | File |
|---|---|
| the tenants feature (repository + view + mount) | `bootstrap/tenants_feature.go` |
| the archive command and result | `internal/application/commands/archive_tenant_command.go` |
| the insert command and result | `internal/application/commands/insert_tenant_command.go` |
| the patch command and result | `internal/application/commands/patch_tenant_command.go` |
| tests for the command mappers | `internal/application/commands/tenant_commands_test.go` |
| the unarchive command and result | `internal/application/commands/unarchive_tenant_command.go` |
| the by-id query and its result | `internal/application/queries/find_tenant_by_id_query.go` |
| the listing query and its result | `internal/application/queries/find_tenants_by_params_query.go` |
| the read criteria tests | `internal/application/queries/tenant_queries_test.go` |
| the translation coverage test — every notification must be translatable in every catalog | `internal/application/translations/tenant_translations_test.go` |
| the Tenant aggregate root, its modes and its rules | `internal/domain/tenant.go` |
| the Tenant service port (1 fact(s)) | `internal/domain/tenant_service.go` |
| tests for Tenant's rules | `internal/domain/tenant_test.go` |
| the vos package documentation | `internal/domain/vos/doc.go` |
| the TenantStatus enumeration (3 members) | `internal/domain/vos/tenant_status.go` |
| tests for 4 value object(s) | `internal/domain/vos/tenant_vos_test.go` |
| the tenants schema (5 columns) | `internal/infra/schemas/tenant_schema.go` |
| the schema builder tests — they run the builders, so a boot panic is a test failure | `internal/infra/schemas/tenant_schemas_test.go` |
| the Tenant repository and its constraint bindings | `internal/infra/tenant_repository.go` |
| the Tenant service implementation | `internal/infra/tenant_service.go` |
| the tenants view (relational-backed) | `internal/infra/views/tenant_view.go` |
| the view definition test — it builds the definition, so a boot panic is a test failure | `internal/infra/views/tenant_view_test.go` |
| the by-id request and response | `internal/web/requests/find_tenant_by_id.go` |
| the listing request and response | `internal/web/requests/find_tenants_by_params.go` |
| the insert request and response | `internal/web/requests/insert_tenant.go` |
| the patch request and response | `internal/web/requests/patch_tenant.go` |
| the request mapper tests | `internal/web/requests/tenant_requests_test.go` |
| the 6 tenant endpoints | `internal/web/tenant_routes.go` |

**Left untouched** (yours, by design):

- `internal/domain/tenant_rules_manual.go` — hand-written rules live here, by design
- `migrations/postgres/0001_tenant_manual.down.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it
- `migrations/postgres/0001_tenant_manual.up.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it

## What was NOT generated

Owned by other tools:

- the gRPC surface and its proto contract — `/omnicore:implement`
- integration events (publish/subscribe) — `/omnicore:implement`
- read models spanning more than this entity — `/omnicore:scaffold-view`
- changing this entity once it exists — `/omnicore:evolve-entity`, which edits this spec and regenerates. The CODE comes back from the spec; the DATABASE never does — the migration a change needs is written by hand, and that skill's impact map is what carries it, along with the orphans a shrinking spec leaves and everything outside this generator's ownership

Read controls this listing does NOT serve: `?search=`. That is a contract, not an omission — sending one is answered with a typed 400 rather than being ignored.

## Framework compatibility and next steps

Verdict: **exact** (project pins v0.54.0)

framework v0.54.0 meets the required v0.54.0

Verify what was generated:

```
go build -tags 'postgres' ./...
go vet -tags 'postgres' ./...
go test -tags 'postgres' ./... -count=1
```

A service that builds with a transport tag (kafka, nats) needs it IN ADDITION to the engine tag on every command above — an engine tag alone may not select a buildable configuration there.

Then exercise the endpoints end to end — a green build proves the code compiles, not that the entity works.
