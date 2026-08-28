# User — generation report

Generated from `specs/omnicore-gen/user.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you already wrote

Written by hand — `kind: manual`, or a composite with `written: manual` — and already in the project. The generator did not open them and cannot tell whether what they enforce still matches what the spec says they enforce — listed so a description that moved does not leave a stale rule behind it:

- **`PersonName`** — `internal/domain/vos/person_name.go`. A person's name in two halves, each human-typed rather than keyboard junk: 1-75 runes, at least one letter, no run of four identical runes, trimmed and single-spaced. No word count and no distinct-rune floor — a family name of two runes is an ordinary name. The type also renders the two halves as one full name.
- **`Password`** — `internal/domain/vos/password.go`. A password of 8 to 128 runes carrying at least one lowercase letter, one uppercase letter, one digit and one symbol, judged by Unicode classes rather than ASCII, with no leading or trailing whitespace.

The backing stays a contract across every run: the mappers convert with `vos.<Name>(x)` and read back with `.Value()`, so changing the underlying type of one of these breaks call sites that name neither this report nor the spec. For a composite the contract is its FIELD SET instead: the mappers build it as a `vos.<Name>{Part: v, …}` literal, so a part renamed or retyped there breaks the same way, and the spec's `parts` are what says which names those are.

### Fields declared DERIVED, which nothing here computes

- **`EmailVerifiedAt`** — `assignedFrom: derived` took it out of every write request, command and OpenAPI request schema, so a client cannot set it. WRITING it is yours: the column starts NULL and stays null until something writes it — a `rules.manual` entry on the verb that produces the value, or hand-written code beyond this spec. Null is a state the response shows honestly (the field is simply absent), so check that the write exists rather than that the insert fills it.
- **`PasswordHash`** — `assignedFrom: derived` took it out of every write request, command and OpenAPI request schema, so a client cannot set it. WRITING it is yours: a `rules.manual` entry scoped to insert, assigning it from the fields it derives from. Idempotent by construction when it is a pure function of an immutable field, which is the case this exists for.
- **`PasswordChangedAt`** — `assignedFrom: derived` took it out of every write request, command and OpenAPI request schema, so a client cannot set it. WRITING it is yours: a `rules.manual` entry scoped to insert, assigning it from the fields it derives from. Idempotent by construction when it is a pure function of an immutable field, which is the case this exists for.
- **`MustChangePassword`** — `assignedFrom: derived` took it out of every write request, command and OpenAPI request schema, so a client cannot set it. WRITING it is yours: a `rules.manual` entry scoped to insert, assigning it from the fields it derives from. Idempotent by construction when it is a pure function of an immutable field, which is the case this exists for.

### `internal/domain/user_rules_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are implemented. It lists them so you can check the file still covers what the spec declares, which is where a rule added to the spec later goes unnoticed.

**`credential-derivation`**

> Fill the server-assigned fields on creation. Hash Password with the PasswordHasher port (Argon2id, OWASP baseline, PHC-encoded) into PasswordHash; set PasswordChangedAt to now; set MustChangePassword to true, because the creator knows the password they chose. Leave EmailVerifiedAt at the zero instant — no verification flow exists. The plaintext must never be logged, on any path.

- fires under `IfInsert`

**`password-echoes-identity`**

> Refuse a password that contains the e-mail's local part, or any word of four runes or more from either half of the name, compared case-insensitively and over runes.

- fires under `IfInsert` · raise `PasswordEchoesIdentityNotification{}` · attach it to `Password`

**`tenant-available`**

> Refuse a user whose owning tenant is missing, archived or commercially SUSPENDED. A trial tenant is a live customer and passes — "unavailable" is not "not active". Asks TenantIsUnavailable.

- fires under `IfInsert` · raise `UserTenantDoesNotExistNotification{}` · attach it to `TenantID`

**`group-available-in-tenant`**

> For every group this write ADDS: refuse when the id is absent from the groups table, points at an archived group, or belongs to a tenant other than this user's. ONE answer for all three — a distinct "belongs to another tenant" reply is an existence oracle over a competitor's org chart. Asks GroupIsUnavailableInTenant, and judges domain.GetAddedItemsOf rather than the stored set.

- fires under `IfInsertOrUpdate` · raise `GroupNotAvailableInTenantNotification{}` · attach it to `Groups`

**`group-wildcard-refused`**

> For every group this write ADDS: refuse when any role the group confers grants a permission carrying a wildcard in either part. Runs BEFORE the escalation rule and removes its panic input. Asks GroupGrantsWildcard, which answers true for an unresolvable id. Consequence, accepted: the platform's own superadmin user is seeded by migration, not created through this API.

- fires under `IfInsertOrUpdate` · raise `CannotJoinWildcardGroupNotification{}` · attach it to `Groups`

**`group-no-escalation`**

> For every group this write ADDS: refuse unless the caller holds EVERY permission of EVERY role that group confers — three hops, a set and not one key. A *:* superadmin passes by construction. Asks CallerLacksAnyPermissionOfGroup, which guards the wildcard itself and answers "lacks" rather than calling through, because a panic on a security rule is a 500.

- fires under `IfInsertOrUpdate` · raise `CannotJoinGroupWithUnheldPermissionsNotification{}` · attach it to `Groups`

**`role-available-in-tenant`**

> The same three questions one hop shorter, for every role this write ADDS directly: absent, archived, or another tenant's. Asks RoleIsUnavailableInTenant. Reuses the notification Group already declared and translated.

- fires under `IfInsertOrUpdate` · raise `RoleNotAvailableInTenantNotification{}` · attach it to `Roles`

**`role-wildcard-refused`**

> For every role this write ADDS: refuse when it grants any permission carrying a wildcard. Runs before the escalation rule, same interlock as the group pair. Asks RoleGrantsWildcard.

- fires under `IfInsertOrUpdate` · raise `CannotGrantWildcardRoleNotification{}` · attach it to `Roles`

**`role-no-escalation`**

> For every role this write ADDS: refuse unless the caller holds every permission it grants. Asks CallerLacksAnyPermissionOfRole.

- fires under `IfInsertOrUpdate` · raise `CannotGrantRoleWithUnheldPermissionsNotification{}` · attach it to `Roles`

**`archive-forces-suspended`**

> Archiving forces Status to suspended. Set the field and raise nothing. Verbatim the shape Tenant already ships.

- fires under `IfArchive`

The tests for them are yours too, and the same check applies.

### `internal/infra/user_service_manual.go`

The spec marked these questions as ones the generator cannot answer, so it declared them on the port and left the bodies to you. **They panic until you write them** — the project still builds and boots, and the failure arrives the moment the rule asks, as a 500 with the write rolled back. The outcome being avoided is the other one: a query against the wrong source would compile, return, and mean nothing.

**`HashPassword(password string) string`**

> The irreversible Argon2id hash of the plaintext, PHC-encoded so the parameters travel with the value. Pure CPU, no query. It must never log its input.

**`PasswordIsUnchanged(password string, passwordHash string) bool`**

> Whether the plaintext given verifies against the hash already stored — the refusal behind "the new password must differ from the current one". Never logs its input.

**`TenantIsUnavailable(tenantID domain.ID) bool`**

> Whether the owning tenant is missing, archived, or commercially SUSPENDED. Queries the tenants table by its primary key. A trial tenant is a live customer and is available.

**`GroupIsUnavailableInTenant(tenantID domain.ID, groupID domain.ID) bool`**

> Whether this group id is absent from the groups table, points at an archived group, or belongs to a tenant OTHER than the one passed. One answer for all three — the caller-facing message must not distinguish them.

**`GroupGrantsWildcard(groupID domain.ID) bool`**

> Whether any role this group confers grants a permission carrying a wildcard in either part. Answers TRUE when the id is unknown, so an unresolvable membership never reaches the escalation probe.

**`CallerLacksAnyPermissionOfGroup(groupID domain.ID) bool`**

> Whether the requesting caller fails to hold at least one permission conferred by any role in this group — the transitive, three-hop half of the escalation rule. Shares the single resolution GroupGrantsWildcard performs. GUARDS THE WILDCARD ITSELF and answers "lacks" rather than calling through, because a panic on a security rule is a 500. A *:* superadmin passes by construction.

**`RoleIsUnavailableInTenant(tenantID domain.ID, roleID domain.ID) bool`**

> Whether this role id is absent from the roles table, points at an archived role, or belongs to a tenant OTHER than the one passed. One answer for all three.

**`RoleGrantsWildcard(roleID domain.ID) bool`**

> Whether the role behind this id grants any permission carrying a wildcard. Resolve it through RoleRepository.Loader, whose declared read join fills every grant's resource and action. Answers TRUE for an unknown id.

**`CallerLacksAnyPermissionOfRole(roleID domain.ID) bool`**

> Whether the requesting caller fails to hold at least one of the permissions this role grants. Shares RoleGrantsWildcard's single read, and guards the wildcard itself.

The method returns a plain value and no error, so decide what an unavailable source means. Failing loudly is the safe default — returning a plausible answer skips the rule this exists to enforce.

### `internal/application/queries/user_computed_manual.go`

This file already exists and is YOURS — the generator did not open it and cannot tell whether these are filled. It lists them so you can check the file still covers what the spec declares, which is where a field added to the spec later goes unnoticed.

**If this file predates the one-function-per-field shape, the build will not find these.** The derivations used to be one function per READ SHAPE, each handed a whole Result, which meant writing the same derivation twice and keeping the two in step by hand. Each is now one exported function taking the sources it declared — the generator unwraps whatever the shape holds and calls it, and the WRITE responses call the same one. Move each body into the signature below and delete the old per-shape functions.

**`FullName` (string)** ← `GivenName`, `FamilyName`

> The person's given and family names on one line, for listings.

```go
func ComputeUserFullName(ctx *configuration.AppContext, givenName string, familyName string) (string, error)
```

**Until a body is written the field renders absent, and nothing says so** — unlike a manual fact, which panics. The read answers 200, the other columns are correct, and this one is empty on REST, on GraphQL and in the export at once. What the declaration already bought needs no code: `?fields=` on the field fetches its sources instead, `?orderBy=` on it is a typed 400, and the export keeps the column under its label.

### Fields nothing generated fills

Declared `runtime: true` with `source: manual`. Each one is on the aggregate so your rules can read it, and on nothing else: no write request DTO, no command, no mapper, no OpenAPI schema — and no column, so no migration, `TableSchema`, outbox payload, audit event or response either. **No generated code puts a value there.** That is the declaration, not an omission.

| field | type | what it is for |
|---|---|---|
| `CurrentPassword` | `string` | The password the caller currently holds, proved by the change and never stored. |

Write the assignment in the operation that owns it — the hand-written command whose mapper has both the request and the entity. The shape this exists for is an operation that dispatches the same mode a generated verb does and is told apart by its action name: it needs the value on the aggregate, and the ordinary write bodies must not grow a field for it.

**Until something assigns it, the field is the zero value on every write, and nothing says so.** A rule reading it does not fail — it judges `""` (or `false`, or `0`) and answers accordingly, which for a possession check is the answer that looks like a pass. Value objects are the one part already handled: the automatic pass would judge this field on every generated write, so its validation is excluded under every gate, and what checks the value is the rule you write.

### The migration — already yours

The SQL for this entity was written on an earlier run and **was not touched**:

- `migrations/postgres/0005_user_manual.down.sql`
- `migrations/postgres/0005_user_manual.up.sql`

That is permanent, and it is the same posture as the `_manual` rule files: created once, never regenerated. A migration is the only thing here whose effect outlives the file — once it has run anywhere, the framework's tracking table records it as applied, so rewriting the file would change what the file CLAIMS without changing a single table. A service that boots green and fails on the first query touching the change is the outcome being avoided.

**If the shape below no longer matches what that migration created, the fix is a NEW numbered pair in the same folder** — never an edit to one that may have run. Two things are worth being deliberate about, because they are where data is lost: adding a NOT NULL column to a table that already has rows fails unless it carries a default, and a rename done as drop-then-add takes the data with it.

If nothing about the storage changed this run, there is nothing to do here — read the shape as confirmation, not as a task.

**A changed `description:` is a storage change too, on postgres.** The description is stored IN the database — a COMMENT on postgres, mysql and oracle, an `MS_Description` extended property on sqlserver — so that someone holding a connection and not this repository can read it. The code regenerates from the spec; that catalogue entry does not. Rewording a description therefore needs a new pair carrying just the `COMMENT ON` / `sp_addextendedproperty` statements, or the database keeps answering with the old wording.

The shape the regenerated code expects, for `users`:

**`users`** — the aggregate root

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `tenant_id` | id | no |  |
| `given_name` | string(75) | no |  |
| `family_name` | string(75) | no |  |
| `email` | string(254) | no |  |
| `email_verified_at` | time | yes |  |
| `password_hash` | string(255) | no |  |
| `password_changed_at` | time | no |  |
| `must_change_password` | bool | no |  |
| `status` | string(16) | no |  |
| `revision` | int64 | no | optimistic concurrency, maintained by the framework |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |
| `deleted_at` | time | yes | archive stamp |

Indexes it expects:

- `users_email_key` — UNIQUE on (email), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as UserEmailAlreadyExistsNotification
- `user_groups_user_id_group_id_key` — UNIQUE on (user_id, group_id), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as UserAlreadyInGroupNotification
- `user_roles_user_id_role_id_key` — UNIQUE on (user_id, role_id), over the ACTIVE rows only — an archived one frees the value; a duplicate is reported as UserAlreadyGrantsRoleNotification


**`user_groups`** — the groups collection (1:N)

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `user_id` | id | no | foreign key to users |
| `group_id` | id | no |  |
| `deleted_at` | time | yes | archive stamp |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |

**`user_roles`** — the roles collection (1:N)

| Column | Type | Null | Note |
|---|---|---|---|
| `id` | id | no | primary key |
| `user_id` | id | no | foreign key to users |
| `role_id` | id | no |  |
| `deleted_at` | time | yes | archive stamp |
| `created_at` | time | no |  |
| `updated_at` | time | no |  |

A new pair goes in every dialect this service targets (postgres), numbered after the highest existing one. Every `.up.sql` needs its `.down.sql` or the service refuses to boot.

If this entity has NOT shipped anywhere yet — you are still the only one who ever ran it — deleting the pair above and regenerating writes it fresh from the current spec. That is safe exactly while that is true, and never after.

### Fields whose copies carry a mask

The real value stays in the COLUMN and in the hydrated entity — the rules read it, the writes store it. What is masked is every copy the framework makes of the row:

| field | in the sync (payload → topic → consumers → document) | in the audit event |
|---|---|---|
| `User.PasswordHash` | replaced by `***` | replaced by `***` |

Three things to check, in the order they bite.

1. **The read side is NOT covered by this.** Redaction governs what the framework copies; what this service's own API returns is yours. This read model is RELATIONAL — it selects the columns, which hold the real value — so every redacted field it projects is served in the clear unless it is `hidden: true` or behind `read.fieldRestrict`. Check each row of the table above against that.
2. **A rebuild is what fixes documents already written.** Declaring a redaction, or changing a redactor or its parameter, changes the projected shape: the framework's own drift check fires, `read.view.version` must be bumped, and the resulting rebuild is what replaces the values an earlier policy already projected. Without it, turning a field redacted protects future writes while every document already in the read model keeps its plaintext.
3. **A `hook` is invisible to that check.** A closure has no portable identity, so the hash mixes in only the KIND — changing what the function RETURNS is a shape change nobody detects for you. Bump the version yourself when you change one.

It is forward-only, and it does not protect the database: the column holds the real value, and anyone with SQL access reads it.

### Fields nobody receives

`PasswordHash` — declared `hidden: true`, so stored, filterable and writable, and absent from every response: the by-id read, each row of the listing, the write responses, and the CSV/XLSX exports that render the listing. This is not `read.fieldRestrict`, which returns the field to callers holding a permission; nobody receives these. Check that a client is not expected to read back what it just wrote.

### Fields the caller sends and nothing stores

Declared `runtime: true` with `source: body`. Each one crosses the write request, the command and the aggregate — so the rules can read it — and stops there. There is no column, so it is in no migration, no `TableSchema`, no outbox payload, no audit event and no response.

| field | carried by | what to check |
|---|---|---|
| `Password` | insert | `vos.Password` judges the value's shape on the verbs above, and nothing else does |
| `PasswordConfirmation` | insert | nothing validates it — every check on this value is a rule you declared |

Two consequences worth reading twice. **Nothing compares it for you**: the value object on the field checks the value's SHAPE, and "the confirmation matches the password" is a rule — declare it (`kind: comparison`) or the field is collected and ignored. And **the verbs it does not name skip its value object entirely**, because a write that never carried the field has nothing to judge; if a verb must require it, name that verb under the field's `modes`.

### What this entity asks about the caller

Declared `runtime: true` with an identity `source`. The domain is handed no request and no `ctx`, so each of these rides onto the aggregate in the command mapper — `if id := ctx.Identity(); id != nil { … }` — and the rules read it from there. None of them is stored: no column, no migration, no `TableSchema`, no outbox payload, no audit event, no response.

| field | asks | answered by |
|---|---|---|
| `RequestingUserID` | who the caller is | `Identity().Subject` |

The claim NAMES behind these are the framework's to resolve, not this code's: the tenant claim is `authorization.tenant.claim` and the permissions claim is `authorization.permissionsClaim`. The generated feed calls the accessor and never spells either name — the generated unit tests build their fixture Identity under the framework's DEFAULTS, which is the only name a test with no yaml can honestly use.

### The tenant is server-assigned, and the insert accepts one anyway

`TenantID` is declared `assignedFrom: identity-claim` with `bypassMaySet: true`, so it is filled from the caller's identity on every insert and is in no update or patch body. The INSERT body carries it as an OPTIONAL value, for one reason: a super-admin (`*:*`) crosses the row scope, and without a field to name the tenant in they could repair a customer's records and never create one.

**Check the guard, not the mapper.** The mapper applies whatever was sent, deliberately: what refuses a caller who may not state a tenant is `refuseForeignTenant` in `internal/domain/user.go`, which compares `TenantID` against the caller's own and stands down only for the bypass. Two things follow. A caller who names someone else's tenant gets the same refusal a write into that tenant gets — not a silent 201 filed under their own. And if that guard is ever removed or narrowed, this field becomes a tenant anyone can choose.

### Rules that END the validation pass

`tenant-valid` (in `IfInsertOrUpdate`), `tenant-valid` (in `IfArchive`) — declared `guard: true`. After each of them the pass stops if anything has already been rejected: the rules below it, this entity's automatic value-object validation, and the `BuildRules` and value objects of every collection. A clean write is unaffected — the barrier fires only where something was ALREADY refused — so what changes is the SHAPE of a 422: it carries what was found up to the barrier, not that plus every other field the write would have failed on. Check that against what the API consumers expect to receive in one response.

### Value objects validated with the rules, not after them

`TenantID` (in `IfInsertOrUpdate`), `TenantID` (in `IfArchive`) — declared `kind: valueObject`. The framework validates every value-object field on every write, but that pass runs AFTER `BuildRules`, so a value object cannot be the premise of the rules below it. Each field above is checked where it is declared instead — by the value object's own answer, never a second one — and excluded from the automatic pass in those verbs, so nothing is reported twice. Two consequences worth checking: the value is now validated BEFORE any barrier that follows, so a 422 that used to hide it behind another failure now carries both; and in the verbs this rule does not cover, the field is still validated at the end, exactly as before.

### Fields the server fills

- **`TenantID`** — written on insert from the `tenant_id` claim of the caller's token. It is **absent from every write request and command**: a client cannot set it, and an update does not touch it. Confirm the callers are authenticated on the insert route — with no identity the field stays empty, and nothing else will say so.

### Per-entry command tests are generated now

The verbs that address ONE entry — add, remove — have generated tests in `internal/application/commands/user_commands_test.go`: the entry is applied and projected back, a change keeps its id, an unknown id projects nothing.

**If you wrote your own tests for those mappers before this run**, the package will not compile until you delete them — Go reports it as `redeclared in this block`, which reads like a generator bug and is not one. The generated cases cover the same ground; anything yours asserts beyond them is worth keeping under a different name.

## What to check

- **Is the set of Email really open?** They are declared as shapes (`kind: raw`), so anything matching the pattern is accepted. If the valid values are FINITE and known — a state code, a status, a category — it is an `enum` instead: the caller gets the list in OpenAPI, the code gets named constants, and an out-of-set value converges to Unknown rather than being stored.

- **Are `givenName`, `familyName` the names you want on the wire?** They are the parts of the composite value object `PersonName`, and they are the ONLY names the outside world ever sees — the filter, `?fields=`, `orderBy`, the JSON field, the export column and the projected document key — because nothing above the schema learns a composite exists. Renaming one later is a wire break, not a refactor. The value object is mandatory — it is always there, and each part follows its own type.

These are the decisions the spec made that are expensive to change later. Read them against what you actually meant.

| Decision | Value | Why it matters |
|---|---|---|
| Storage | flat table `users` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Collection `Groups` | `add` → `user:grant` (declared); `remove` → `user:grant` (declared) | These routes hang off `/users/:id/groups`. Gated on its own through `children[].permissions`, not by the root's update. Grant that permission before the routes go live — a holder of the root's update alone now gets a 403 here. Removing ONE entry ARCHIVES it (204, no body) and is one-way: there is no per-entry unarchive, so the only way back is a fresh add, with a NEW entry id. |
| Collection `Roles` | `add` → `user:grant` (declared); `remove` → `user:grant` (declared) | These routes hang off `/users/:id/roles`. Gated on its own through `children[].permissions`, not by the root's update. Grant that permission before the routes go live — a holder of the root's update alone now gets a 403 here. Removing ONE entry ARCHIVES it (204, no body) and is one-way: there is no per-entry unarchive, so the only way back is a fresh add, with a NEW entry id. |
| Removal | archive (one-way: no unarchive is mounted) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `Email` — across the whole table, scope `active-only` (service-precheck+constraint) | an archived row frees it, so the value can be taken again; a duplicate is refused at the database and reported as `UserEmailAlreadyExistsNotification`. |
| Data access | tenant | Callers are restricted to their tenant's rows. |
| Crossing the scope | `*:*` | Only a super-admin crosses the scope, and nothing new became grantable — what crosses is the claim they already carry. The wildcard cannot be handed to the framework's HasPermission (it panics on one), so the generated guard calls `Identity.IsSuperAdmin()` instead — the framework's own question for the `*:*` grant, nil-safe and honouring the configured permissions claim. A resource wildcard like `role:*` does NOT answer it. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. Nothing is materialised: there is no collection, no version and no rebuild — a shape change here needs no bump and no operational step. |
| Read join → Tenant | `InnerJoin` on `tenant_id` | An aggregate with no counterpart is NOT returned, on EVERY read through this repository — FindByID included, which the write handlers load through. Legal only because the foreign key is non-nullable. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. |
| Read join → Group | `InnerJoin` on `group_id`, from UserGroup | An entry with no counterpart is NOT returned — a silent hole in the collection, not a missing aggregate. Prefer left wherever the relationship is genuinely optional. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. |
| Read join → Role | `InnerJoin` on `role_id`, from UserRole | An entry with no counterpart is NOT returned — a silent hole in the collection, not a missing aggregate. Prefer left wherever the relationship is genuinely optional. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. |

## What was generated

| What | File |
|---|---|
| tests for the command mappers | `internal/application/commands/user_commands_test.go` |
| the per-entry commands for user_groups | `internal/application/commands/user_group_commands.go` |
| the per-entry commands for user_roles | `internal/application/commands/user_role_commands.go` |
| the per-entry wire types for user_groups | `internal/web/requests/user_group_requests.go` |
| the request mapper tests | `internal/web/requests/user_requests_test.go` |
| the per-entry wire types for user_roles | `internal/web/requests/user_role_requests.go` |
| the 5 user endpoints | `internal/web/user_routes.go` |

**Left untouched** (yours, by design):

- `internal/application/queries/user_computed_manual.go` — hand-written rules live here, by design
- `internal/domain/user_rules_manual.go` — hand-written rules live here, by design
- `internal/infra/user_service_manual.go` — hand-written rules live here, by design
- `migrations/postgres/0005_user_manual.down.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it
- `migrations/postgres/0005_user_manual.up.sql` — created once and never rewritten — a migration that ran cannot be taken back by editing it

36 file(s) were already up to date.

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
