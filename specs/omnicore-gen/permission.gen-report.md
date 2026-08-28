# Permission — generation report

Generated from `specs/omnicore-gen/permission.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you already wrote

Written by hand — `kind: manual`, or a composite with `written: manual` — and already in the project. The generator did not open them and cannot tell whether what they enforce still matches what the spec says they enforce — listed so a description that moved does not leave a stale rule behind it:

- **`PermissionKey`** — `internal/domain/vos/permission_key.go`. A resource together with what may be done to it — the pair a JWT claim carries and a route compares against, byte for byte. The resource is a colon-joined path of 2-64 rune lowercase slugs (tenant, user:profile); the action is exactly one such slug, so the LAST colon segment is always the action and the rendering parses back one way. The wildcard `*` is accepted as an ENTIRE part only, never inside a path and never mixed into a slug, and a `*` resource forces a `*` action — the claim matcher honors exactly three shapes (exact, resource:*, *:*) and anything else would be a row that matches nothing while reading like a grant. No normalization: a value that does not already comply is refused, never repaired.

The backing stays a contract across every run: the mappers convert with `vos.<Name>(x)` and read back with `.Value()`, so changing the underlying type of one of these breaks call sites that name neither this report nor the spec. For a composite the contract is its FIELD SET instead: the mappers build it as a `vos.<Name>{Part: v, …}` literal, so a part renamed or retyped there breaks the same way, and the spec's `parts` are what says which names those are.

### `internal/domain/permission_rules_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are implemented. It lists them so you can check the file still covers what the spec declares, which is where a rule added to the spec later goes unnoticed.

**`description-does-not-echo-key`**

> The description must differ from the rendered resource:action string under a normalized comparison — case-folded, with whitespace, colons and hyphens collapsed — which catches the lazy paste and nothing more. Render the key through PermissionKey.String(); do not concatenate the parts here, the value object is the single home of the separator.

- fires under `IfInsertOrUpdate` · raise `PermissionDescriptionEchoesKeyNotification{}` · attach it to `Description`

The tests for them are yours too, and the same check applies.

### `internal/application/queries/permission_computed_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are filled. It lists them so you can check the file still covers what the spec declares, which is where a field added to the spec later goes unnoticed.

**If this file predates the one-function-per-field shape, the build will not find these.** The derivations used to be one function per READ SHAPE, each handed a whole Result, which meant writing the same derivation twice and keeping the two in step by hand. Each is now one exported function taking the sources it declared — the generator unwraps whatever the shape holds and calls it, and the WRITE responses call the same one. Move each body into the signature below and delete the old per-shape functions.

**`Permission` (string)** ← `Resource`, `Action`

> The permission as a token carries it and a route compares it: resource:action.

```go
func ComputePermissionPermission(ctx *configuration.AppContext, resource string, action string) (string, error)
```

**Until a body is written the field renders absent, and nothing says so** — unlike a manual fact, which panics. The read answers 200, the other columns are correct, and this one is empty on REST, on GraphQL and in the export at once. What the declaration already bought needs no code: `?fields=` on the field fetches its sources instead, `?orderBy=` on it is a typed 400, and the export keeps the column under its label.

### The migration — already yours

The SQL for this entity was written on an earlier run and **was not touched**:

- `migrations/postgres/0002_permission_manual.down.sql`
- `migrations/postgres/0002_permission_manual.up.sql`

That is permanent, and it is the same posture as the `_manual` rule files: created once, never regenerated. A migration is the only thing here whose effect outlives the file — once it has run anywhere, the framework's tracking table records it as applied, so rewriting the file would change what the file CLAIMS without changing a single table. A service that boots green and fails on the first query touching the change is the outcome being avoided.

**If the shape below no longer matches what that migration created, the fix is a NEW numbered pair in the same folder** — never an edit to one that may have run. Two things are worth being deliberate about, because they are where data is lost: adding a NOT NULL column to a table that already has rows fails unless it carries a default, and a rename done as drop-then-add takes the data with it.

If nothing about the storage changed this run, there is nothing to do here — read the shape as confirmation, not as a task.

**A changed `description:` is a storage change too, on postgres.** The description is stored IN the database — a COMMENT on postgres, mysql and oracle, an `MS_Description` extended property on sqlserver — so that someone holding a connection and not this repository can read it. The code regenerates from the spec; that catalogue entry does not. Rewording a description therefore needs a new pair carrying just the `COMMENT ON` / `sp_addextendedproperty` statements, or the database keeps answering with the old wording.

The shape the regenerated code expects, for `permissions`:

**`permissions`** — the aggregate root

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `resource_name` | string(64) | no |  |
| `action_name` | string(64) | no |  |
| `description` | string(500) | no |  |
| `revision` | int64 | no | optimistic concurrency, maintained by the framework |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |
| `deleted_at` | time | yes | archive stamp |

Indexes it expects:

- `permissions_resource_name_action_name_key` — UNIQUE on (resource_name, action_name), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as PermissionAlreadyExistsNotification


A new pair goes in every dialect this service targets (postgres), numbered after the highest existing one. Every `.up.sql` needs its `.down.sql` or the service refuses to boot.

If this entity has NOT shipped anywhere yet — you are still the only one who ever ran it — deleting the pair above and regenerating writes it fresh from the current spec. That is safe exactly while that is true, and never after.

### Fields nobody receives

`Resource`, `Action` — declared `hidden: true`, so stored, filterable and writable, and absent from every response: the by-id read, each row of the listing, the write responses, and the CSV/XLSX exports that render the listing. This is not `read.fieldRestrict`, which returns the field to callers holding a permission; nobody receives these. Check that a client is not expected to read back what it just wrote.

## What to check

- **Are `resource`, `action` the names you want on the wire?** They are the parts of the composite value object `PermissionKey`, and they are the ONLY names the outside world ever sees — the filter, `?fields=`, `orderBy`, the JSON field, the export column and the projected document key — because nothing above the schema learns a composite exists. Renaming one later is a wire break, not a refactor. The value object is mandatory — it is always there, and each part follows its own type.

These are the decisions the spec made that are expensive to change later. Read them against what you actually meant.

| Decision | Value | Why it matters |
|---|---|---|
| Storage | flat table `permissions` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Removal | archive (one-way: no unarchive is mounted) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `PermissionKey` (`Resource` + `Action`) — across the whole table, scope `active-only` (service-precheck+constraint) | an archived row frees it, so the value can be taken again; a duplicate is refused at the database and reported as `PermissionAlreadyExistsNotification`. Unique as a TUPLE: the parts identify together, so a row differing in either one is a different value — a constraint over a single part would refuse rows the domain accepts. |
| Data access | anyone-with-permission | Any caller holding the permission sees and edits every row. If some callers should only see their own, this is the line to change. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. Nothing is materialised: there is no collection, no version and no rebuild — a shape change here needs no bump and no operational step. |

## What was generated

**Left untouched** (yours, by design):

- `internal/application/queries/permission_computed_manual.go` — hand-written rules live here, by design
- `internal/domain/permission_rules_manual.go` — hand-written rules live here, by design
- `migrations/postgres/0002_permission_manual.down.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it
- `migrations/postgres/0002_permission_manual.up.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it

25 file(s) were already up to date.

## What was NOT generated

Owned by other tools:

- the gRPC surface and its proto contract — `/omnicore:implement`
- integration events (publish/subscribe) — `/omnicore:implement`
- read models spanning more than this entity — `/omnicore:scaffold-view`
- changing this entity once it exists — `/omnicore:evolve-entity`, which edits this spec and regenerates. The CODE comes back from the spec; the DATABASE never does — the migration a change needs is written by hand, and that skill's impact map is what carries it, along with the orphans a shrinking spec leaves and everything outside this generator's ownership

Read controls this listing does NOT serve: `?search=`. That is a contract, not an omission — sending one is answered with a typed 400 rather than being ignored.

## Framework compatibility and next steps

Verdict: **exact** (project pins v0.62.0)

framework v0.62.0 meets the required v0.62.0

Verify what was generated:

```
go build -tags 'postgres' ./...
go vet -tags 'postgres' ./...
go test -tags 'postgres' ./... -count=1
```

A service that builds with a transport tag (kafka, nats) needs it IN ADDITION to the engine tag on every command above — an engine tag alone may not select a buildable configuration there.

Then exercise the endpoints end to end — a green build proves the code compiles, not that the entity works.
