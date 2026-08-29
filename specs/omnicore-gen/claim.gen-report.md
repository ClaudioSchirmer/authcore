# Claim — generation report

Generated from `specs/omnicore-gen/claim.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you already wrote

Written by hand — `kind: manual`, or a composite with `written: manual` — and already in the project. The generator did not open them and cannot tell whether what they enforce still matches what the spec says they enforce — listed so a description that moved does not leave a stale rule behind it:

- **`ClaimName`** — `internal/domain/vos/claim_name.go`. A claim's name as a token carries it: lowercase snake_case of 2-64 runes matching ^[a-z][a-z0-9_]*$, carrying the reserved x_ prefix, whose remainder must not itself begin with x_ (the paste error a caller-owned prefix makes possible). Reuses this project's shared anti-junk predicates. NO NORMALIZATION — a value that does not already comply is refused, never repaired, prepended or stripped: the name is immutable and a caller who believes they registered one string has no second chance.

The backing stays a contract across every run: the mappers convert with `vos.<Name>(x)` and read back with `.Value()`, so changing the underlying type of one of these breaks call sites that name neither this report nor the spec.

### `internal/domain/claim_rules_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are implemented. It lists them so you can check the file still covers what the spec declares, which is where a rule added to the spec later goes unnoticed.

**`tenant-must-exist`**

> The owning tenant must exist, must not be archived and must not be commercially SUSPENDED. Ask TenantIsUnavailable; a read join cannot answer this, because on an insert a joined field is blank. A TRIAL tenant is a live customer and passes.

- fires under `IfInsert` · raise `ClaimTenantDoesNotExistNotification{}` · attach it to `TenantID`

**`applies-to-narrowing-refused`**

> Refuse a change to AppliesTo that would stop admitting an identity kind for which an ACTIVE edge still holds a value. Ask only about the kinds the NEW value DROPS, and only when the value actually changed: if the new value no longer admits `client`, ask ClaimIsHeldByAClient; if it no longer admits `user`, ask ClaimIsHeldByAUser; refuse when either answers true. Six transitions are possible and four of them narrow — both->user and both->client drop one kind each, user->client and client->user drop one each as well, and user->both and client->both are widenings that must ask NOTHING and always pass. A definition no one holds a value for narrows freely, and that has to keep working: refusing every narrowing would make the field effectively immutable, which the model gate decided the other way. Read the enum member off AppliesTo rather than its raw string.

- fires under `IfUpdate` · raise `ClaimAppliesToCannotExcludeHeldValuesNotification{}` · attach it to `AppliesTo`

**`default-value-matches-value-type`**

> The default value must parse as the declared ValueType: `number` accepts a valid decimal number, `bool` accepts exactly "true" or "false", `string` accepts any non-empty value. A NULL default is ALWAYS valid and skips the check entirely — "no default" is a legitimate state, and level 2 of the chain simply does not fire. Read the enum member off ValueType rather than the raw string, so an unknown type (which the value object already refused) does not reach a second answer here.

- fires under `IfInsertOrUpdate` · raise `DefaultValueDoesNotMatchValueTypeNotification{}` · attach it to `DefaultValue`

**`claims-per-tenant-cap-users`**

> At most 20 ACTIVE claim definitions per tenant may admit a USER — that is, may carry AppliesTo `user` or `both`. Ask ActiveClaimsWithAppliesTo twice and add the two answers: the `user` member plus the `both` member. Fire ONLY when this write ADDS the user kind — on an insert whose AppliesTo admits users, and on an update whose OLD AppliesTo did not admit users while the new one does. A write that neither inserts nor widens into `user` must ask NOTHING and always pass, including a narrowing and an edit that leaves AppliesTo alone. Refuse when the bucket already holds 20 or more, since the row being written would be the 21st. Read the enum member off AppliesTo through ClaimAdmitsUsers rather than comparing raw strings, so an unknown member — which the value object already refused — reaches no second answer here. The bound reaches the message through the notification's {max} tvar.

- fires under `IfInsertOrUpdate` · raise `TooManyUserClaimsInTenantNotification{}` · attach it to `AppliesTo`

**`claims-per-tenant-cap-clients`**

> The client half of the same budget, and the same contract in every respect: at most 20 ACTIVE claim definitions per tenant may admit a machine CLIENT — AppliesTo `client` or `both`. Add the `client` member's count to the `both` member's, fire only when this write ADDS the client kind, read the member through ClaimAdmitsClients, and refuse at 20 or more. The two buckets are independent: a full user side must never block a definition that admits only clients.

- fires under `IfInsertOrUpdate` · raise `TooManyClientClaimsInTenantNotification{}` · attach it to `AppliesTo`

The tests for them are yours too, and the same check applies.

### `internal/infra/claim_service_manual.go`

The spec marked these questions as ones the generator cannot answer, so it declared them on the port and left the bodies to you. **They panic until you write them** — the project still builds and boots, and the failure arrives the moment the rule asks, as a 500 with the write rolled back. The outcome being avoided is the other one: a query against the wrong source would compile, return, and mean nothing.

**`TenantIsUnavailable(tenantID domain.ID) bool`**

> Whether the owning tenant is missing, archived, or commercially SUSPENDED. Queries the tenants table by its primary key. A trial tenant is a live customer and is available.

**`ClaimIsHeldByAUser(tenantID domain.ID, name string) bool`**

> Whether any ACTIVE user_claims row references the ACTIVE claim definition identified by this tenant and name. Join user_claims to claims on claim_id and require user_claims.deleted_at IS NULL AND claims.deleted_at IS NULL — archived edges do not count, because a value somebody removed must not freeze the definition's shape, and the archived-definition half is what keeps a retired row's leftovers out of the answer.

**`ClaimIsHeldByAClient(tenantID domain.ID, name string) bool`**

> Whether any ACTIVE client_claims row references the ACTIVE claim definition identified by this tenant and name. Same join and the same two archive predicates as its user twin.

The method returns a plain value and no error, so decide what an unavailable source means. Failing loudly is the safe default — returning a plausible answer skips the rule this exists to enforce.

### The migration — already yours

The SQL for this entity was written on an earlier run and **was not touched**:

- `migrations/postgres/0009_claim_manual.down.sql`
- `migrations/postgres/0009_claim_manual.up.sql`

That is permanent, and it is the same posture as the `_manual` rule files: created once, never regenerated. A migration is the only thing here whose effect outlives the file — once it has run anywhere, the framework's tracking table records it as applied, so rewriting the file would change what the file CLAIMS without changing a single table. A service that boots green and fails on the first query touching the change is the outcome being avoided.

**If the shape below no longer matches what that migration created, the fix is a NEW numbered pair in the same folder** — never an edit to one that may have run. Two things are worth being deliberate about, because they are where data is lost: adding a NOT NULL column to a table that already has rows fails unless it carries a default, and a rename done as drop-then-add takes the data with it.

If nothing about the storage changed this run, there is nothing to do here — read the shape as confirmation, not as a task.

**A changed `description:` is a storage change too, on postgres.** The description is stored IN the database — a COMMENT on postgres, mysql and oracle, an `MS_Description` extended property on sqlserver — so that someone holding a connection and not this repository can read it. The code regenerates from the spec; that catalogue entry does not. Rewording a description therefore needs a new pair carrying just the `COMMENT ON` / `sp_addextendedproperty` statements, or the database keeps answering with the old wording.

The shape the regenerated code expects, for `claims`:

**`claims`** — the aggregate root

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `tenant_id` | id | no |  |
| `name` | string(64) | no |  |
| `value_type` | string(16) | no |  |
| `applies_to` | string(16) | no |  |
| `default_value` | string(256) | yes |  |
| `description` | string(500) | no |  |
| `revision` | int64 | no | optimistic concurrency, maintained by the framework |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |
| `deleted_at` | time | yes | archive stamp |

Indexes it expects:

- `claims_tenant_id_name_key` — UNIQUE on (tenant_id, name), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as ClaimNameAlreadyExistsNotification


A new pair goes in every dialect this service targets (postgres), numbered after the highest existing one. Every `.up.sql` needs its `.down.sql` or the service refuses to boot.

If this entity has NOT shipped anywhere yet — you are still the only one who ever ran it — deleting the pair above and regenerating writes it fresh from the current spec. That is safe exactly while that is true, and never after.

### The tenant is server-assigned, and the insert accepts one anyway

`TenantID` is declared `assignedFrom: identity-claim` with `bypassMaySet: true`, so it is filled from the caller's identity on every insert and is in no update or patch body. The INSERT body carries it as an OPTIONAL value, for one reason: a super-admin (`*:*`) crosses the row scope, and without a field to name the tenant in they could repair a customer's records and never create one.

**Check the guard, not the mapper.** The mapper applies whatever was sent, deliberately: what refuses a caller who may not state a tenant is `refuseForeignTenant` in `internal/domain/claim.go`, which compares `TenantID` against the caller's own and stands down only for the bypass. Two things follow. A caller who names someone else's tenant gets the same refusal a write into that tenant gets — not a silent 201 filed under their own. And if that guard is ever removed or narrowed, this field becomes a tenant anyone can choose.

### Rules that END the validation pass

`tenant-is-a-usable-id` (in `IfInsertOrUpdate`) — declared `guard: true`. After each of them the pass stops if anything has already been rejected: the rules below it, this entity's automatic value-object validation, and the `BuildRules` and value objects of every collection. A clean write is unaffected — the barrier fires only where something was ALREADY refused — so what changes is the SHAPE of a 422: it carries what was found up to the barrier, not that plus every other field the write would have failed on. Check that against what the API consumers expect to receive in one response.

### Value objects validated with the rules, not after them

`TenantID` (in `IfInsertOrUpdate`) — declared `kind: valueObject`. The framework validates every value-object field on every write, but that pass runs AFTER `BuildRules`, so a value object cannot be the premise of the rules below it. Each field above is checked where it is declared instead — by the value object's own answer, never a second one — and excluded from the automatic pass in those verbs, so nothing is reported twice. Two consequences worth checking: the value is now validated BEFORE any barrier that follows, so a 422 that used to hide it behind another failure now carries both; and in the verbs this rule does not cover, the field is still validated at the end, exactly as before.

### Fields the server fills

- **`TenantID`** — written on insert from the `tenant_id` claim of the caller's token. It is **absent from every write request and command**: a client cannot set it, and an update does not touch it. Confirm the callers are authenticated on the insert route — with no identity the field stays empty, and nothing else will say so.

## What to check

These are the decisions the spec made that are expensive to change later. Read them against what you actually meant.

| Decision | Value | Why it matters |
|---|---|---|
| Storage | flat table `claims` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Removal | archive (one-way: no unarchive is mounted) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `Name` — per TenantID, scope `active-only` (service-precheck+constraint) | an archived row frees it, so the value can be taken again; a duplicate is refused at the database and reported as `ClaimNameAlreadyExistsNotification`. |
| Data access | tenant | Callers are restricted to their tenant's rows. |
| Crossing the scope | `*:*` | Only a super-admin crosses the scope, and nothing new became grantable — what crosses is the claim they already carry. The wildcard cannot be handed to the framework's HasPermission (it panics on one), so the generated guard calls `Identity.IsSuperAdmin()` instead — the framework's own question for the `*:*` grant, nil-safe and honouring the configured permissions claim. A resource wildcard like `role:*` does NOT answer it. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. Nothing is materialised: there is no collection, no version and no rebuild — a shape change here needs no bump and no operational step. |
| Read join → Tenant | `InnerJoin` on `tenant_id` | An aggregate with no counterpart is NOT returned, on EVERY read through this repository — FindByID included, which the write handlers load through. Legal only because the foreign key is non-nullable. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. |

### Where each endpoint answers

Surfaces enabled: **REST · GraphQL**. The three are independent, and every endpoint below is generated from ONE command with ONE permission — a surface is a way in, never a second implementation.

| endpoint | REST | GraphQL |
|---|---|---|
| Create a claim | `POST /claims` | `createClaim` |
| Update a claim (partial) | `PATCH /claims/:id` | `patchClaim` |
| Archive a claim | `PATCH /claims/:id/archive` | `archiveClaim` |
| List claims | `GET /claims` | `claims` |
| Get a claim by id | `GET /claims/:id` | `claim` |

## What was generated

| What | File |
|---|---|
| the translation coverage test — every notification must be translatable in every catalog | `internal/application/translations/claim_translations_test.go` |
| 2 DEU translation key(s) | `internal/application/translations/deu.go` |
| 2 ENG translation key(s) | `internal/application/translations/eng.go` |
| 2 ESP translation key(s) | `internal/application/translations/esp.go` |
| 2 FRA translation key(s) | `internal/application/translations/fra.go` |
| 2 ITA translation key(s) | `internal/application/translations/ita.go` |
| 2 NLD translation key(s) | `internal/application/translations/nld.go` |
| 2 PTBR translation key(s) | `internal/application/translations/ptbr.go` |
| the Claim service port (5 fact(s)) | `internal/domain/claim_service.go` |
| tests for Claim's rules | `internal/domain/claim_test.go` |
| 10 notification declaration(s) | `internal/domain/notifications.go` |
| the Claim service implementation | `internal/infra/claim_service.go` |

**Left untouched** (yours, by design):

- `internal/domain/claim_rules_manual.go` — hand-written rules live here, by design
- `internal/infra/claim_service_manual.go` — hand-written rules live here, by design
- `migrations/postgres/0009_claim_manual.down.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it
- `migrations/postgres/0009_claim_manual.up.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it

24 file(s) were already up to date.

## What was NOT generated

Owned by other tools:

- the gRPC surface and its proto contract — `/omnicore:implement`
- integration events (publish/subscribe) — `/omnicore:implement`
- read models spanning more than this entity — `/omnicore:scaffold-view`
- changing this entity once it exists — `/omnicore:evolve-entity`, which edits this spec and regenerates. The CODE comes back from the spec; the DATABASE never does — the migration a change needs is written by hand, and that skill's impact map is what carries it, along with the orphans a shrinking spec leaves and everything outside this generator's ownership

Read controls this listing does NOT serve: `?search=`. That is a contract, not an omission — sending one is answered with a typed 400 rather than being ignored.

## Framework compatibility and next steps

Verdict: **exact** (project pins v0.63.0)

framework v0.63.0 meets the required v0.63.0

Verify what was generated:

```
go build -tags 'postgres' ./...
go vet -tags 'postgres' ./...
go test -tags 'postgres' ./... -count=1
```

A service that builds with a transport tag (kafka, nats) needs it IN ADDITION to the engine tag on every command above — an engine tag alone may not select a buildable configuration there.

Then exercise the endpoints end to end — a green build proves the code compiles, not that the entity works.
