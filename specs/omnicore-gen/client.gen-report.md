# Client — generation report

Generated from `specs/omnicore-gen/client.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you already wrote

Written by hand — `kind: manual`, or a composite with `written: manual` — and already in the project. The generator did not open them and cannot tell whether what they enforce still matches what the spec says they enforce — listed so a description that moved does not leave a stale rule behind it:

- **`CIDRBlock`** — `internal/domain/vos/cidr_block.go`. An IPv4 or IPv6 network range in CIDR notation, accepted only in its CANONICAL form — host bits masked away. 203.0.113.5/24 is refused and the answer names 203.0.113.0/24, because the two are one range written two ways and a collection that stores both has a unique index that cannot see the duplicate. Refusing beats normalising silently: the mappers convert straight to this type, so there is no seat to rewrite the value in, and a caller told the canonical spelling learns something a silent rewrite hides. It also refuses 0.0.0.0/0 and ::/0 outright — an empty collection is how "no restriction" is spelled, and two spellings for it is how a reviewer comes to believe a client is restricted when it is not. Raises InvalidCIDRBlockNotification for a value netip.ParsePrefix rejects, CIDRHasHostBitsSetNotification for a well-formed range that is not masked, and UniversalCIDRNotAllowedNotification for a universal prefix.

The backing stays a contract across every run: the mappers convert with `vos.<Name>(x)` and read back with `.Value()`, so changing the underlying type of one of these breaks call sites that name neither this report nor the spec.

### Fields declared DERIVED, which nothing here computes

- **`SecretHash`** — `assignedFrom: derived` took it out of every write request, command and OpenAPI request schema, so a client cannot set it. WRITING it is yours: a `rules.manual` entry scoped to insert, assigning it from the fields it derives from. Idempotent by construction when it is a pure function of an immutable field, which is the case this exists for.
- **`SecretChangedAt`** — `assignedFrom: derived` took it out of every write request, command and OpenAPI request schema, so a client cannot set it. WRITING it is yours: a `rules.manual` entry scoped to insert, assigning it from the fields it derives from. Idempotent by construction when it is a pure function of an immutable field, which is the case this exists for.
- **`PreviousSecretHash`** — `assignedFrom: derived` took it out of every write request, command and OpenAPI request schema, so a client cannot set it. WRITING it is yours: the column starts NULL and stays null until something writes it — a `rules.manual` entry on the verb that produces the value, or hand-written code beyond this spec. Null is a state the response shows honestly (the field is simply absent), so check that the write exists rather than that the insert fills it.
- **`PreviousSecretExpiresAt`** — `assignedFrom: derived` took it out of every write request, command and OpenAPI request schema, so a client cannot set it. WRITING it is yours: the column starts NULL and stays null until something writes it — a `rules.manual` entry on the verb that produces the value, or hand-written code beyond this spec. Null is a state the response shows honestly (the field is simply absent), so check that the write exists rather than that the insert fills it.

### `internal/domain/client_rules_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are implemented. It lists them so you can check the file still covers what the spec declares, which is where a rule added to the spec later goes unnoticed.

**`credential-minting`**

> Mint the credential on creation. Draw 32 bytes from crypto/rand, render them base64url WITHOUT padding, prefix them with "acs_", and put the result in Secret; put HashSecret(Secret) in SecretHash; set SecretChangedAt to now. Leave PreviousSecretHash and PreviousSecretExpiresAt absent — there is nothing retiring on a first issue. Derive ONLY after every other check has passed: writing a hash for a secret the rules refused is one refactor away otherwise. The plaintext must never be logged, on any path.

- fires under `IfInsert`

**`tenant-available`**

> Refuse a client whose owning tenant is missing, archived or commercially SUSPENDED. A trial tenant is a live customer and passes — "unavailable" is not "not active". Asks TenantIsUnavailable.

- fires under `IfInsert` · raise `ClientTenantDoesNotExistNotification{}` · attach it to `TenantID`

**`role-available-in-tenant`**

> For every role this write ADDS: refuse when the id is absent from the roles table, points at an archived role, or belongs to a tenant other than this client's. ONE answer for all three — a distinct "belongs to another tenant" reply is an existence oracle over a competitor's catalogue. Asks RoleIsUnavailableInTenant, and judges the ADDED entries only, never the ones already stored.

- fires under `IfInsertOrUpdate` · raise `RoleNotAvailableInTenantNotification{}` · attach it to `Roles`

**`no-wildcard-role`**

> For every role this write ADDS: refuse a role granting a permission that carries a wildcard in either part. Asks RoleGrantsWildcard. It runs BEFORE the escalation probe below and that order is load-bearing — the framework's HasPermission PANICS on a wildcard argument, and a panic on a security rule is a 500. A machine credential carrying *:* is the single worst thing this entity could mint.

- fires under `IfInsertOrUpdate` · raise `CannotGrantWildcardRoleNotification{}` · attach it to `Roles`

**`no-escalation`**

> For every role this write ADDS: refuse when the requesting caller does not hold every permission that role confers. Asks CallerLacksAnyPermissionOfRole. Without it, anyone holding client:grant mints a non-expiring, non-interactive credential carrying privileges they do not have themselves. A *:* caller passes by construction.

- fires under `IfInsertOrUpdate` · raise `CannotGrantRoleWithUnheldPermissionsNotification{}` · attach it to `Roles`

**`claim-available-in-tenant`**

> For every claim value this write ADDS or CHANGES: refuse when the claim id is absent from the claims table, points at an archived definition, or belongs to a tenant other than this client's. ONE answer for all three — a distinct "belongs to another tenant" reply is an existence oracle over a competitor's claim vocabulary. Asks ClaimIsUnavailableInTenant, and judges the ADDED and CHANGED entries only.

- fires under `IfInsertOrUpdate` · raise `ClaimNotAvailableInTenantNotification{}` · attach it to `Claims`

**`claim-applies-to-client`**

> For every claim value this write ADDS or CHANGES: refuse when the definition declares appliesTo: user. client and both pass. This rule and its twin on User are what make appliesTo mean something — the README notes today that it states as data something the code does not yet enforce. Asks ClaimDoesNotApplyToClient, which answers true for an id it cannot resolve.

- fires under `IfInsertOrUpdate` · raise `ClaimDoesNotApplyToClientNotification{}` · attach it to `Claims`

**`claim-value-matches-value-type`**

> For every claim value this write ADDS or CHANGES: refuse when the value does not parse as the ValueType the definition declares. `number` accepts a valid decimal number, `bool` accepts exactly "true" or "false", `string` accepts any non-empty value — deliberately the SAME three readings the catalog's own default-value-matches-value-type uses, because two levels of one chain must not disagree about what a bool is. Read the enum member off the definition rather than its raw string. Asks ClaimValueDoesNotMatchValueType.

- fires under `IfInsertOrUpdate` · raise `ClaimValueDoesNotMatchValueTypeNotification{}` · attach it to `Claims`

**`archive-forces-suspended`**

> Archiving a client sets Status to suspended. Mirrors Tenant rule 13 and User's U15: a row that is gone must not read as active anywhere it is still listed. There is no Unarchive on this entity, so this is a one-way door by construction.

- fires under `IfArchive`

**`client-rotates-only-its-own-secret`**

> When RequestingIdentityKind is exactly "client", refuse a SECRET ROTATION whose row id is not RequestingClientID. Narrowed to the rotation on 2026-08-28: it used to cover every update and the archive, which left a client-subject caller unable to administer its tenant's other clients at all — too closed, and not the boundary that matters. Creating, editing, archiving and granting are ordinary tenant-scoped writes, gated by the permission the caller carries; handing out a CREDENTIAL is the act that must stay on the caller's own row, because a machine that can rotate another machine's secret can lock it out and take its place. Stands down when the claim is absent (it reads as a user), and when no identity is present at all. THIS RULE IS INERT UNTIL POST /auth/client/token MINTS THE CLAIM, and that is deliberate — do not add a fallback that infers the subject kind from another claim's absence.

- fires under `IfUpdate` · raise `ClientMayOnlyRotateItsOwnSecretNotification{}` · attach it to `ID`

The tests for them are yours too, and the same check applies.

### `internal/infra/client_service_manual.go`

The spec marked these questions as ones the generator cannot answer, so it declared them on the port and left the bodies to you. **They panic until you write them** — the project still builds and boots, and the failure arrives the moment the rule asks, as a 500 with the write rolled back. The outcome being avoided is the other one: a query against the wrong source would compile, return, and mean nothing.

**`HashSecret(secret string) string`**

> The SHA-256 of the secret, lowercase hex. NOT Argon2id, and the reason is in spec §B-Q4: memory-hardness buys nothing against 256 random bits and costs 19 MiB per verify on an unauthenticated endpoint. No salt — a rainbow table over 2^256 random values cannot exist. Pure CPU, no query. It must never log its input, and whatever verifies against it must compare with crypto/subtle.

**`TenantIsUnavailable(tenantID domain.ID) bool`**

> Whether the owning tenant is missing, archived, or commercially SUSPENDED. Queries the tenants table by its primary key. A trial tenant is a live customer and is available.

**`RoleIsUnavailableInTenant(tenantID domain.ID, roleIDSet []domain.ID) map[domain.ID]bool`**

> Whether this role id is absent from the roles table, points at an archived role, or belongs to a tenant OTHER than the one passed. One answer for all three — the caller-facing message must not distinguish them.

**`RoleGrantsWildcard(roleIDSet []domain.ID) map[domain.ID]bool`**

> Whether this role grants a permission carrying a wildcard in either part. Answers TRUE when the id is unknown, so an unresolvable grant never reaches the escalation probe — the framework's HasPermission panics on a wildcard argument.

**`CallerLacksAnyPermissionOfRole(roleIDSet []domain.ID) map[domain.ID]bool`**

> Whether the requesting caller fails to hold at least one permission this role confers — the escalation half. GUARDS THE WILDCARD ITSELF and answers "lacks" rather than calling through, because a panic on a security rule is a 500. A *:* superadmin passes by construction.

**`ClaimIsUnavailableInTenant(tenantID domain.ID, claimIDSet []domain.ID) map[domain.ID]bool`**

> Whether this claim id is absent from the claims table, points at an archived definition, or belongs to a tenant OTHER than the one passed. One answer for all three — the caller-facing message must not distinguish them.

**`ClaimDoesNotApplyToClient(claimIDSet []domain.ID) map[domain.ID]bool`**

> Whether the definition behind this id declares appliesTo: user, so no machine client may hold a value for it. client and both answer false. Answers TRUE for an unknown id.

**`ClaimValueDoesNotMatchValueType(entries []ClientClaimValueDoesNotMatchValueTypeEntry) map[domain.ID]bool`**

> Whether the value fails to parse as the ValueType the definition declares: `number` wants a valid decimal number, `bool` wants exactly "true" or "false", `string` wants any non-empty value. The same three readings the catalog's own default-value check uses. Answers TRUE for an unknown id.

The method returns a plain value and no error, so decide what an unavailable source means. Failing loudly is the safe default — returning a plausible answer skips the rule this exists to enforce.

**Before writing one of these against another TABLE, check the door.** The facts beside this file run over this entity's own repository, so a question about another aggregate's child table, a control table or a lookup cannot be asked there. If the pinned framework documents a DIRECT schema — one table, no aggregate behind it — that table gets its own anchor and the body keeps the same existence probe and aggregate DSL, in every dialect, inside the caller's transaction. Hand-written SQL and a whole aggregate declared for a table that is only ever counted are both the wrong answer to that question.

### `internal/infra/client_service_manual.go` — bodies the spec no longer asks for

This file still answers for questions the spec has stopped declaring. The generator did not open it and will not: it is yours. **Delete these — a body nothing calls is dead code the next reader has to rule out**, and it goes with the change that stranded it rather than later.

- `func (s *ClientServiceImpl) companions(...)`
- `func (s *ClientServiceImpl) roleRows(...)`
- `func (s *ClientServiceImpl) claimRows(...)`

⚠ **One of these can break the build rather than merely sit there.** A BATCHED per-entry fact (`perEntry`) takes a generated entry carrier declared beside the port, and that type is removed with the fact — so the body naming it stops compiling. The compiler will say `undefined: <Entity><Fact>Entry` and name a symbol; the decision behind it is this line. Deleting the body may also strand the `appdomain` import it was the only user of — the compiler names that one too.

### Fields nothing generated fills

Declared `runtime: true` with `source: manual`. Each one is on the aggregate so your rules can read it, and on nothing else: no write request DTO, no command, no mapper, no OpenAPI schema — and no column, so no migration, `TableSchema`, outbox payload, audit event or response either. **No generated code puts a value there.** That is the declaration, not an omission.

| field | type | what it is for |
|---|---|---|
| `Secret` | `string` | The plaintext secret, minted by the rules and rendered by the operation that minted it — and nowhere else, ever. No column, no payload, no audit event, no listing. |
| `GracePeriodSeconds` | `int` | How long the retiring secret stays valid, in seconds. Zero is an immediate kill and is a legitimate answer, not an omission. |

**One of these leaves the service in a response, and it is the only place it ever will.** `renderIn` puts the value the rules minted into the write verb's own answer — the Result reads it off the entity after the write, the Response renders it, and a GraphQL mutation reusing that Response renders it too. Nothing else does: no read, no listing, no `?fields=`, no export column, no audit event, no sync payload. Whatever the row keeps of the value — a hash, normally — is a separate, persisted field.

- `Secret` — rendered by: insert

So the assignment below is not optional for these: a verb that mints nothing answers with the zero value, and the caller receives an empty credential from a `201` that looks like every other one. Check the response of that verb against a real request before calling it done — it is the whole reason the field is declared.


Write the assignment in the operation that owns it — the hand-written command whose mapper has both the request and the entity. The shape this exists for is an operation that dispatches the same mode a generated verb does and is told apart by its action name: it needs the value on the aggregate, and the ordinary write bodies must not grow a field for it.

**Until something assigns it, the field is the zero value on every write, and nothing says so.** A rule reading it does not fail — it judges `""` (or `false`, or `0`) and answers accordingly, which for a possession check is the answer that looks like a pass. Value objects are the one part already handled: the automatic pass would judge this field on every generated write, so its validation is excluded under every gate, and what checks the value is the rule you write.

### The migration — already yours

The SQL for this entity was written on an earlier run and **was not touched**:

- `migrations/postgres/0008_client_manual.down.sql`
- `migrations/postgres/0008_client_manual.up.sql`

That is permanent, and it is the same posture as the `_manual` rule files: created once, never regenerated. A migration is the only thing here whose effect outlives the file — once it has run anywhere, the framework's tracking table records it as applied, so rewriting the file would change what the file CLAIMS without changing a single table. A service that boots green and fails on the first query touching the change is the outcome being avoided.

**If the shape below no longer matches what that migration created, the fix is a NEW numbered pair in the same folder** — never an edit to one that may have run. Two things are worth being deliberate about, because they are where data is lost: adding a NOT NULL column to a table that already has rows fails unless it carries a default, and a rename done as drop-then-add takes the data with it.

If nothing about the storage changed this run, there is nothing to do here — read the shape as confirmation, not as a task.

**A changed `description:` is a storage change too, on postgres.** The description is stored IN the database — a COMMENT on postgres, mysql and oracle, an `MS_Description` extended property on sqlserver — so that someone holding a connection and not this repository can read it. The code regenerates from the spec; that catalogue entry does not. Rewording a description therefore needs a new pair carrying just the `COMMENT ON` / `sp_addextendedproperty` statements, or the database keeps answering with the old wording.

The shape the regenerated code expects, for `clients`:

**`clients`** — the aggregate root

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `tenant_id` | id | no |  |
| `name` | string(120) | no |  |
| `description` | string(500) | no |  |
| `secret_hash` | string(64) | no |  |
| `secret_changed_at` | time | no |  |
| `previous_secret_hash` | string(64) | yes |  |
| `previous_secret_expires_at` | time | yes |  |
| `status` | string(16) | no |  |
| `revision` | int64 | no | optimistic concurrency, maintained by the framework |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |
| `archived_at` | time | yes | archive stamp |

Indexes it expects:

- `clients_tenant_id_name_key` — UNIQUE on (tenant_id, name), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as ClientNameAlreadyExistsNotification
- `client_roles_client_id_role_id_key` — UNIQUE on (client_id, role_id), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as ClientAlreadyGrantsRoleNotification
- `client_allowed_cidrs_client_id_cidr_key` — UNIQUE on (client_id, cidr), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as ClientAlreadyAllowsCIDRNotification
- `client_claims_client_id_claim_id_key` — UNIQUE on (client_id, claim_id), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as ClientAlreadyHoldsClaimNotification


**`client_roles`** — the roles collection (1:N)

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `client_id` | id | no | foreign key to clients |
| `role_id` | id | no |  |
| `archived_at` | time | yes | archive stamp |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |

**`client_allowed_cidrs`** — the allowedCIDRs collection (1:N)

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `client_id` | id | no | foreign key to clients |
| `cidr` | string(43) | no |  |
| `label` | string(120) | no |  |
| `archived_at` | time | yes | archive stamp |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |

**`client_claims`** — the claims collection (1:N)

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `client_id` | id | no | foreign key to clients |
| `claim_id` | id | no |  |
| `value` | string(256) | no |  |
| `archived_at` | time | yes | archive stamp |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |

A new pair goes in every dialect this service targets (postgres), numbered after the highest existing one. Every `.up.sql` needs its `.down.sql` or the service refuses to boot.

If this entity has NOT shipped anywhere yet — you are still the only one who ever ran it — deleting the pair above and regenerating writes it fresh from the current spec. That is safe exactly while that is true, and never after.

### Yours in a shared file, and out of step with the spec

The notification declarations and the seven translation catalogs are shared by every entity of the project. The generator maintains what IT wrote there — it records a hash of each declaration and each message, so a spec that moves takes its own text with it. These did NOT match what it recorded, which means somebody edited them, or they predate that record. Either way they are not the generator's to overwrite, so they were left exactly as they are:

- ClientArchivedAtField — internal/application/translations/deu.go
- ClientArchivedAtField — internal/application/translations/esp.go
- ClientArchivedAtField — internal/application/translations/fra.go
- ClientArchivedAtField — internal/application/translations/ita.go
- ClientArchivedAtField — internal/application/translations/nld.go
- ClientArchivedAtField — internal/application/translations/ptbr.go
- ClientCreatedAtField — internal/application/translations/deu.go
- ClientCreatedAtField — internal/application/translations/esp.go
- ClientCreatedAtField — internal/application/translations/fra.go
- ClientCreatedAtField — internal/application/translations/ita.go
- ClientCreatedAtField — internal/application/translations/nld.go
- ClientCreatedAtField — internal/application/translations/ptbr.go
- ClientUpdatedAtField — internal/application/translations/deu.go
- ClientUpdatedAtField — internal/application/translations/esp.go
- ClientUpdatedAtField — internal/application/translations/fra.go
- ClientUpdatedAtField — internal/application/translations/ita.go
- ClientUpdatedAtField — internal/application/translations/nld.go
- ClientUpdatedAtField — internal/application/translations/ptbr.go

A notification DECLARATION on this list can stop the package compiling, and the error will not point here: if the spec gave it a `tvars` entry, the rules emitted for it now write `N{Max: "50"}` and the struct has no such field — add it, and it goes away. A translation on this list is cosmetic by comparison: the end user simply reads the older wording. If your version is the better one, put it in the spec; the two will then agree and it drops off this list.

### Fields whose copies carry a mask

The real value stays in the COLUMN and in the hydrated entity — the rules read it, the writes store it. What is masked is every copy the framework makes of the row:

| field | in the sync (payload → topic → consumers → document) | in the audit event |
|---|---|---|
| `Client.SecretHash` | replaced by `***` | replaced by `***` |
| `Client.PreviousSecretHash` | replaced by `***` | replaced by `***` |

Three things to check, in the order they bite.

1. **The read side is NOT covered by this.** Redaction governs what the framework copies; what this service's own API returns is yours. This read model is RELATIONAL — it selects the columns, which hold the real value — so every redacted field it projects is served in the clear unless it is `hidden: true` or behind `read.fieldRestrict`. Check each row of the table above against that.
2. **A rebuild is what fixes documents already written.** Declaring a redaction, or changing a redactor or its parameter, changes the projected shape: the framework's own drift check fires, `read.view.version` must be bumped, and the resulting rebuild is what replaces the values an earlier policy already projected. Without it, turning a field redacted protects future writes while every document already in the read model keeps its plaintext.
3. **A `hook` is invisible to that check.** A closure has no portable identity, so the hash mixes in only the KIND — changing what the function RETURNS is a shape change nobody detects for you. Bump the version yourself when you change one.

It is forward-only, and it does not protect the database: the column holds the real value, and anyone with SQL access reads it.

### Fields nobody receives

`SecretHash`, `PreviousSecretHash` — declared `hidden: true`, so stored, filterable and writable, and absent from every response: the by-id read, each row of the listing, the write responses, and the CSV/XLSX exports that render the listing. This is not `read.fieldRestrict`, which returns the field to callers holding a permission; nobody receives these. Check that a client is not expected to read back what it just wrote.

### What this entity asks about the caller

Declared `runtime: true` with an identity `source`. The domain is handed no request and no `ctx`, so each of these rides onto the aggregate in the command mapper — `if id := ctx.Identity(); id != nil { … }` — and the rules read it from there. None of them is stored: no column, no migration, no `TableSchema`, no outbox payload, no audit event, no response.

| field | asks | answered by |
|---|---|---|
| `RequestingClientID` | who the caller is | `Identity().Subject` |

The claim NAMES behind these are the framework's to resolve, not this code's: the tenant claim is `authorization.tenant.claim` and the permissions claim is `authorization.permissionsClaim`. The generated feed calls the accessor and never spells either name — the generated unit tests build their fixture Identity under the framework's DEFAULTS, which is the only name a test with no yaml can honestly use.

### TenantID is server-assigned, and the insert accepts one anyway

`TenantID` is declared `assignedFrom: identity-claim` with `bypassMaySet: true`, so it is filled from the caller's identity on every insert and is in no update or patch body. The INSERT body carries it as an OPTIONAL value, for one reason: a super-admin (`*:*`) crosses the row scope, and without a field to name the scope in they could repair a customer's records and never create one.

**Check the guard, not the mapper.** The mapper applies whatever was sent, deliberately: what refuses a caller who may not state one is `refuseForeignTenant` in `internal/domain/client.go`, which compares `TenantID` against the caller's own and stands down only for the bypass. Two things follow. A caller who names someone else's scope gets the same refusal a write into that scope gets — not a silent 201 filed under their own. And if that guard is ever removed or narrowed, this field becomes one anyone can choose.

### Rules that END the validation pass

`tenant-valid` (in `IfInsertOrUpdate`), `tenant-valid` (in `IfArchive`) — declared `guard: true`. After each of them the pass stops if anything has already been rejected: the rules below it, this entity's automatic value-object validation, and the `BuildRules` and value objects of every collection. A clean write is unaffected — the barrier fires only where something was ALREADY refused — so what changes is the SHAPE of a 422: it carries what was found up to the barrier, not that plus every other field the write would have failed on. Check that against what the API consumers expect to receive in one response.

### Value objects validated with the rules, not after them

`TenantID` (in `IfInsertOrUpdate`), `TenantID` (in `IfArchive`) — declared `kind: valueObject`. The framework validates every value-object field on every write, but that pass runs AFTER `BuildRules`, so a value object cannot be the premise of the rules below it. Each field above is checked where it is declared instead — by the value object's own answer, never a second one — and excluded from the automatic pass in those verbs, so nothing is reported twice. Two consequences worth checking: the value is now validated BEFORE any barrier that follows, so a 422 that used to hide it behind another failure now carries both; and in the verbs this rule does not cover, the field is still validated at the end, exactly as before.

### Fields the server fills

- **`TenantID`** — written on insert from the `tenant_id` claim of the caller's token. It is **absent from every write request and command**: a client cannot set it, and an update does not touch it. Confirm the callers are authenticated on the insert route — with no identity the field stays empty, and nothing else will say so.

### Per-entry command tests are generated now

The verbs that address ONE entry — add, change, remove — have generated tests, each one beside the command it covers under `internal/application/commands/` (`<verb>_<collection>_command_test.go`): the entry is applied and projected back, a change keeps its id, an unknown id projects nothing.

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
| Storage | flat table `clients` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Collection `Roles` | `add` → `client:grant` (declared); `remove` → `client:grant` (declared) | These routes hang off `/clients/:id/roles`. Gated on its own through `children[].permissions`, not by the root's update. Grant that permission before the routes go live — a holder of the root's update alone now gets a 403 here. Removing ONE entry ARCHIVES it (204, no body) and is one-way: there is no per-entry unarchive, so the only way back is a fresh add, with a NEW entry id. |
| Collection `AllowedCIDRs` | `add` → `client:manage-network` (declared); `remove` → `client:manage-network` (declared) | These routes hang off `/clients/:id/allowedCIDRs`. Gated on its own through `children[].permissions`, not by the root's update. Grant that permission before the routes go live — a holder of the root's update alone now gets a 403 here. Removing ONE entry ARCHIVES it (204, no body) and is one-way: there is no per-entry unarchive, so the only way back is a fresh add, with a NEW entry id. |
| Collection `Claims` | `add` → `client:set-claim` (declared); `change` → `client:set-claim` (declared); `remove` → `client:set-claim` (declared) | These routes hang off `/clients/:id/claims`. Gated on its own through `children[].permissions`, not by the root's update. Grant that permission before the routes go live — a holder of the root's update alone now gets a 403 here. Changing ONE entry is PARTIAL (`PATCH`): what the body leaves out is read off the STORED entry, and what `change.patchExcludes` names — `ClaimID` — never reaches the wire, so it cannot move. Removing ONE entry ARCHIVES it (204, no body) and is one-way: there is no per-entry unarchive, so the only way back is a fresh add, with a NEW entry id. |
| Removal | archive (one-way: no unarchive is mounted) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `Name` — per TenantID, scope `active-only` (service-precheck+constraint) | an archived row frees it, so the value can be taken again; a duplicate is refused at the database and reported as `ClientNameAlreadyExistsNotification`. |
| Data access | scoped | Callers reach only the rows their identity places them in — see the scopes below. |
| Row scope | `TenantID` = `Identity().TenantID()` — whichever claim `authorization.tenant.claim` names | Enforced on: read, insert, update, archive. Both halves are generated: the read filter in the query, and a guard in BuildRules. |
| Crossing the scope | `*:*` | Only a super-admin crosses the scope, and nothing new became grantable — what crosses is the claim they already carry. The wildcard cannot be handed to the framework's HasPermission (it panics on one), so the generated guard calls `Identity.IsSuperAdmin()` instead — the framework's own question for the `*:*` grant, nil-safe and honouring the configured permissions claim. A resource wildcard like `role:*` does NOT answer it. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. Nothing is materialised: there is no collection, no version and no rebuild — a shape change here needs no bump and no operational step. |
| Read join → Tenant | `InnerJoin` on `tenant_id` | An aggregate with no counterpart is NOT returned, on EVERY read through this repository — FindByID included, which the write handlers load through. Legal only because the foreign key is non-nullable. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. On the entity and OFF the wire: TenantStatus — read by the rules, in no response body and in no export. |
| Read join → Role | `InnerJoin` on `role_id`, from ClientRole | An entry with no counterpart is NOT returned — a silent hole in the collection, not a missing aggregate. Prefer left wherever the relationship is genuinely optional. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. |
| Read join → Claim | `InnerJoin` on `claim_id`, from ClientClaim | An entry with no counterpart is NOT returned — a silent hole in the collection, not a missing aggregate. Prefer left wherever the relationship is genuinely optional. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. |

### Where each endpoint answers

Surfaces enabled: **REST · GraphQL**. The three are independent, and every endpoint below is generated from ONE command with ONE permission — a surface is a way in, never a second implementation.

| endpoint | REST | GraphQL |
|---|---|---|
| Create a client | `POST /clients` | `createClient` |
| Update a client (partial) | `PATCH /clients/:id` | `patchClient` |
| Archive a client | `PATCH /clients/:id/archive` | `archiveClient` |
| List clients | `GET /clients` | `clients` |
| Get a client by id | `GET /clients/:id` | `client` |
| Add one `ClientRole` | `POST /clients/:id/roles` | `addClientRole` |
| Take out one `ClientRole` | `PATCH /clients/:id/roles/:clientRoleId/archive` | `removeClientRole` |
| Add one `ClientAllowedCIDR` | `POST /clients/:id/allowedCIDRs` | `addClientAllowedCIDR` |
| Take out one `ClientAllowedCIDR` | `PATCH /clients/:id/allowedCIDRs/:clientAllowedCIDRId/archive` | `removeClientAllowedCIDR` |
| Add one `ClientClaim` | `POST /clients/:id/claims` | `addClientClaim` |
| Update one `ClientClaim` | `PATCH /clients/:id/claims/:clientClaimId` | `patchClientClaim` |
| Take out one `ClientClaim` | `PATCH /clients/:id/claims/:clientClaimId/archive` | `removeClientClaim` |

## What was generated

| What | File |
|---|---|
| the add command for one client_allowed_cidrs entry | `internal/application/commands/add_client_allowed_cidr_command.go` |
| tests for add_client_allowed_cidr_command.go | `internal/application/commands/add_client_allowed_cidr_command_test.go` |
| the add command for one client_claims entry | `internal/application/commands/add_client_claim_command.go` |
| tests for add_client_claim_command.go | `internal/application/commands/add_client_claim_command_test.go` |
| the add command for one client_roles entry | `internal/application/commands/add_client_role_command.go` |
| tests for add_client_role_command.go | `internal/application/commands/add_client_role_command_test.go` |
| the archive command for one client_allowed_cidrs entry | `internal/application/commands/archive_client_allowed_cidr_command.go` |
| tests for archive_client_allowed_cidr_command.go | `internal/application/commands/archive_client_allowed_cidr_command_test.go` |
| the archive command for one client_claims entry | `internal/application/commands/archive_client_claim_command.go` |
| tests for archive_client_claim_command.go | `internal/application/commands/archive_client_claim_command_test.go` |
| the archive command and result | `internal/application/commands/archive_client_command.go` |
| tests for archive_client_command.go | `internal/application/commands/archive_client_command_test.go` |
| the archive command for one client_roles entry | `internal/application/commands/archive_client_role_command.go` |
| tests for archive_client_role_command.go | `internal/application/commands/archive_client_role_command_test.go` |
| the insert command and result | `internal/application/commands/insert_client_command.go` |
| tests for insert_client_command.go | `internal/application/commands/insert_client_command_test.go` |
| the patch command for one client_claims entry | `internal/application/commands/patch_client_claim_command.go` |
| tests for patch_client_claim_command.go | `internal/application/commands/patch_client_claim_command_test.go` |
| the patch command and result | `internal/application/commands/patch_client_command.go` |
| tests for patch_client_command.go | `internal/application/commands/patch_client_command_test.go` |
| the by-id query and its result | `internal/application/queries/find_client_by_id_query.go` |
| the listing query and its result | `internal/application/queries/find_clients_by_params_query.go` |
| the read tests for find_clients_by_params_query.go | `internal/application/queries/find_clients_by_params_query_test.go` |
| the Client aggregate root, its modes and its rules | `internal/domain/client.go` |
| tests for Client's rules | `internal/domain/client_test.go` |

**Left untouched** (yours, by design):

- `internal/domain/client_rules_manual.go` — hand-written rules live here, by design
- `internal/infra/client_service_manual.go` — hand-written rules live here, by design
- `migrations/postgres/0008_client_manual.down.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it
- `migrations/postgres/0008_client_manual.up.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it

65 file(s) were already up to date.

## What was NOT generated

Owned by other tools:

- the gRPC surface and its proto contract — `/omnicore:implement`
- integration events (publish/subscribe) — `/omnicore:implement`
- read models spanning more than this entity — `/omnicore:scaffold-view`
- changing this entity once it exists — `/omnicore:evolve-entity`, which edits this spec and regenerates. The CODE comes back from the spec; the DATABASE never does — the migration a change needs is written by hand, and that skill's impact map is what carries it, along with the orphans a shrinking spec leaves and everything outside this generator's ownership
- a table with NO aggregate behind it — a control table, a job queue, a lookup, an idempotency ledger. This generator writes aggregates and this spec language cannot say "not one"; that does not mean the framework has no answer. If the pinned version documents a DIRECT schema (one table, no entity), it is the door for those, and `/omnicore:implement` owns wiring it. Neither hand-written SQL nor an entity declared for a table that is only ever queried is the right shape

Read controls this listing does NOT serve: `?search=`. That is a contract, not an omission — sending one is answered with a typed 400 rather than being ignored.

## Framework compatibility and next steps

Verdict: **exact** (project pins v0.74.0)

framework v0.74.0 meets the required v0.74.0

Verify what was generated:

```
go build -tags 'postgres' ./...
go vet -tags 'postgres' ./...
go test -tags 'postgres' ./... -count=1
```

A service that builds with a transport tag (kafka, nats) needs it IN ADDITION to the engine tag on every command above — an engine tag alone may not select a buildable configuration there.

Then exercise the endpoints end to end — a green build proves the code compiles, not that the entity works.
