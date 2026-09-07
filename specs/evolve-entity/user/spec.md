# Evolve `User` — the `claims` collection (level 1 of the claim chain, user side)

> **Superseded 2026-09-06** — omnicore v0.74.0 renamed the managed archive slot
> `DeletedAt` → `ArchivedAt` (builder, logical name and the `deletedAt` wire token),
> and this service renamed the physical column `deleted_at` → `archived_at` in the same
> run. The vocabulary below was rewritten accordingly; the decisions it records are
> unchanged. See `../../upgrade/v0.73.0-to-v0.74.0/migration-plan.md`.

**Status: APPROVED** — 2026-08-28.
**Generation: omnicore-gen** — chosen at the same gate, and governs all three runs.
**Sibling run:** `specs/evolve-entity/client/spec.md` builds the same collection on `Client`.
The two are one shape learned once; every decision below is shared with it, and where a
decision is genuinely per-parent it says so.

---

## §1 — The change, in one paragraph

`Claim` shipped on 2026-08-28 as the catalog: the vocabulary of names a tenant's tokens may
carry, with the type its values must have, which identity kinds may hold one, and a
tenant-wide `default_value`. That is **level 2** of a two-level chain, and level 1 — the
specialised value, per principal — does not exist yet, on either parent. This run adds it on
`User`: a new owned collection `Claims` (table `user_claims`), one row per claim definition
the user holds a value for, holding the definition's id and the value. It is written through
per-entry verbs gated on a **new** `user:set-claim` permission, and it is validated against
the definition it points at — same tenant, declared type, and an `appliesTo` that admits a
user. **No token changes in this run**, by decision: `buildClaims` is not touched, so the
collection can be filled, read and audited with no observable effect on any issued token.

**Scope, decided at the gate (2026-08-28):** the collection only. Emission — resolving the
two-level chain and merging it into `buildClaims`
(`internal/application/commands/authentication_commands_manual.go:572`) — is a **separate
run**, and belongs to `/omnicore:implement` rather than here: that file is a capability, not
an entity, and the two questions it drags in (the token size budget, and `auth.auditClaims`)
are still open in `backlog.md`. This spec makes emission reachable; it does not do it.

---

## §2 — Impact map

Every file this change touches, and **who writes it**. Nothing outside this list gets edited.
`G` = generator-owned (rewritten from the spec YAML, never hand-edited) · `H` = hand-written
on either path · `M` = a `_manual` hook the generator creates once and never rewrites.

| # | Artifact | Who | What changes |
|---|---|---|---|
| 1 | `specs/omnicore-gen/user.omnicore.yaml` | **H** | The change itself on the codegen path: one `children[]` entry, one `joins[]` entry, one raw value object, six notifications, three service facts, two list rules, three manual rules |
| 2 | `migrations/postgres/0010_user_claims_manual.up.sql` + `.down.sql` | **H** | **New pair.** `user_claims`, its parent index, its active-only unique index, and by hand the FK into `claims`. Postgres is the only target dialect (`relational.dialect` in both `microservice.*.yaml`) |
| 3 | `internal/infra/schemas/user_claim_schema.go` | G | New — the collection's TableSchema |
| 4 | `internal/infra/schemas/user_schemas_test.go` | G | Regenerated with the new collection |
| 5 | `internal/domain/aggregatevos/user_claim.go` | G | New — the entry type |
| 6 | `internal/domain/vos/claim_value.go` | G | New — the `ClaimValue` raw VO (§4c) |
| 7 | `internal/domain/user.go` | G | `Claims` collection on the aggregate; `BuildRules` gains the cap and the duplicate clause |
| 8 | `internal/domain/user_service.go` | G | Three new facts on the service port |
| 9 | `internal/domain/user_rules_manual.go` | **M** | Three new rule stubs to implement (§4d) |
| 10 | `internal/domain/notifications.go`, `internal/domain/vos/notifications.go` | G | Six notification types |
| 11 | `internal/infra/user_service.go` | G | Regenerated service adapter |
| 12 | `internal/infra/user_service_manual.go` | **M** | Three fact bodies to implement (§4d) |
| 13 | `internal/infra/user_repository.go` | G | The collection's mapping + the `inChild` read join into `claims` |
| 14 | `internal/infra/views/user_view.go` | G | The `claims` array on the read document |
| 15 | `internal/application/dtos/user_claim_input.go` | G | New — the write DTO |
| 16 | `internal/application/commands/user_claim_commands.go` | G | New — add / change / remove |
| 17 | `internal/application/commands/user_child_results.go` | G | The entry's result shape |
| 18 | `internal/application/queries/user_row_results.go` | G | The nested `claims` on the read result |
| 19 | `internal/web/requests/user_claim_requests.go` | G | New — the three request shapes |
| 20 | `internal/web/requests/user_children.go` | G | The collection's request wiring |
| 21 | `internal/web/user_routes.go` | G | Three new routes, each declaring `user:set-claim` |
| 22 | `bootstrap/users_feature.go` | G | The collection's handlers registered |
| 23 | `internal/application/translations/{eng,ptbr,esp,fra,deu,ita,nld}.go` | G | Two field labels + six notification texts × 7 catalogs (§6) |
| 24 | `internal/application/translations/user_translations_test.go` | G | Regenerated |
| 25 | `internal/domain/user_test.go`, `user_rules_manual_test.go`, `user_children_manual_test.go` | G + **H** | Generated suite regenerated; the three manual rules' tests are hand-written (§7) |
| 26 | `internal/application/{commands,queries,dtos}/user_*_test.go`, `internal/web/requests/user_requests_test.go`, `internal/infra/views/user_view_test.go` | G | Regenerated |
| 27 | `README.md` | **H** | The `User` row in the capability table; the route list; the `Claim` section's "neither exists yet"; the permission-literal note gains `user:set-claim` |
| 28 | `ACCESS_MATRIX.md` | **H** | Three route rows under `## User`; the `## Claim` note that the value-setting verb now exists |
| 29 | `backlog.md` | **H** | The `Claim` entry: the user half of "the two edge collections" stops being open |

**Classes an evolution forgets, checked one by one and found N/A here:**

- `microservice.dev.yaml` / `microservice.prd.yaml` — **no change.** `auth.publicRoutes` lists
  four entries and none is a `/users` path; the new routes are authenticated like every other
  `/users` route. `auth.auditClaims` is emission's question, not this one, and stays at its
  two entries. No `integration:` block exists — this service publishes nothing.
- **Proto / gRPC** — N/A. `surfaces` on `User` declares `rest` and `graphql` only; there is no
  proto contract in this repo.
- **GraphQL schema** — no hand-written schema file: `surfaces.graphql.enabled: true` with no
  `mutations` narrowing, so the three per-entry verbs reach the schema from the generator with
  no second edit. Artifact 21/22 covers it.
- `specs/qa/*.sh` — **N/A, there is no qa suite in this repo** (`specs/qa` does not exist).
  Nothing to update and nothing to regenerate.
- **Views of other entities embedding `User`** — none. `grep` for a `JoinView`/`Link*` leg over
  users returns nothing outside `internal/infra/views/user_view.go` itself.

---

## §3 — Migration strategy

**Purely additive: one new table, no change to any existing one.** Nothing to backfill,
nothing to default, no data at risk — `user_claims` is born empty and every column is NOT NULL
with a value the writer always supplies.

A **new numbered pair**, `0010_user_claims_manual.{up,down}.sql`, in `migrations/postgres/` —
the only target dialect. `0009` is the claim catalog; nothing before it is touched, and no pair
that may have run is edited.

`up` — the shape, following `user_groups` verbatim except for the second column:

```sql
CREATE TABLE "user_claims" (
  "id"         UUID NOT NULL,
  "user_id"    UUID NOT NULL,
  "claim_id"   UUID NOT NULL,
  "value"      VARCHAR(256) NOT NULL,
  "archived_at" TIMESTAMPTZ NULL,
  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT "user_claims_pkey" PRIMARY KEY ("id"),
  CONSTRAINT "user_claims_parent_fk" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);
CREATE INDEX "user_claims_parent_idx" ON "user_claims" ("user_id");
CREATE UNIQUE INDEX "user_claims_user_id_claim_id_key"
  ON "user_claims" ("user_id", "claim_id") WHERE "archived_at" IS NULL;
```

Plus `COMMENT ON` for the table and every column — the per-dialect spelling postgres uses, and
the reason this project's DDL is self-describing in the catalogue.

**No `revision` column.** A collection entry is not an entity: the optimistic-concurrency stamp
lives on `users` and guards the aggregate as a whole. `user_groups`, `user_roles`,
`client_roles` and `client_allowed_cidrs` all agree.

**Hand-written below the generator's line, the same append the four existing collections carry:**

```sql
ALTER TABLE "user_claims"
  ADD CONSTRAINT "user_claims_claim_fk"
  FOREIGN KEY ("claim_id") REFERENCES "claims" ("id");
CREATE INDEX "user_claims_claim_idx" ON "user_claims" ("claim_id");
```

`NO ACTION` on delete, for the reason written into every other FK in this repo: a claim
definition is archived and never purged, so there is no delete for a cascade to follow — and
refusing a hard delete while principals still hold values for it is the correct answer. The
reverse index is what makes the read join into `claims` cheap and what answers "who holds a
value for this definition" when the emission run needs it.

`down` — `DROP TABLE "user_claims"`. It recreates **structure only** on a re-`up`: every value
any principal held is gone. Said plainly rather than implied, though the risk here is as low as
it gets — the table is new in this pair, so a `down` can only lose data written after it.

---

## §4 — The collection, decision by decision

### 4a — Shape

```yaml
  - name: UserClaim
    plural: Claims
    table: user_claims
    parentColumn: user_id
    ownedBy: root
    editStrategy: per-child
    operations: [add, change, remove]
    change:
      shape: patch
      patchExcludes: [ClaimID]
    softRemove: true
    archivedAt: archived_at
    businessIdentity: [ClaimID]
    duplicateNotification: UserAlreadyHoldsClaimNotification
    permissions:
      add:    user:set-claim
      change: user:set-claim
      remove: user:set-claim
    fields:
      - ClaimID  (id,     claim_id, unique: constraint-only / active-only)
      - Value    (string, value, length 256, vo: {kind: raw, ref: ClaimValue})
```

**`operations` includes `change`, and it is the only collection in this service that does.**
Settled in `backlog.md` and carried here with its reason: on `role_permissions`, `user_roles`
and `client_roles` the single stored column **is** the business identity, so a change would turn
grant A into grant B while keeping A's row id — which an audit trail reads as one grant
*becoming* another. Here the identity is `ClaimID` and `Value` is a separate mutable column, so
"correct this cost center" is genuinely **one entry changing** rather than a remove and an add.
`ClientAllowedCIDR` also carries a second mutable column (`Label`) and still refused `change`;
that is not a counter-precedent so much as an unexamined one — there, correcting a label is
cosmetic, here correcting a value is the entire purpose of the row.

**`businessIdentity: [ClaimID]` alone**, deliberately, and for the reason `UserGroup` spells
out: the entry will carry read-only join fields (§4b) that are **blank on a freshly added
entry**, so comparing everything would answer "different" for an addition that duplicates a
stored one — the duplicate guard failing open. `Value` is excluded for a second reason of its
own: two entries for the same definition with different values is exactly the collision the
whole two-level chain exists to make impossible.

**`softRemove: true`** — decided at the gate. Matches `user_groups`, `user_roles`,
`client_roles` and `client_allowed_cidrs`: every collection in this service. The backlog's
argument for a hard delete was "a claim value is not a privilege", and it falls with the verb
decision below: authcore cannot know what a consumer does with `x_plan_tier`, and if a
consumer authorizes on it then an access review has to be able to read what a past value
meant.

### 4b — Reading it back

One `inChild` read join into `Claim`, mirroring `UserGroup`'s into `Group` exactly:

```yaml
  - kind: inner
    to: Claim
    inChild: Claims
    on: claim_id
    fields:
      - ClaimName      (column: name)
      - ClaimValueType (column: value_type)
```

`inner` is safe **only** because `claim_id` is NOT NULL and FK-backed — inside a collection an
inner join drops the **entry**, which would be a silent hole in the array rather than a missing
user. Both conditions hold as of §3.

`ClaimName` is the point: `GET /users/:id` names the claims a person holds without a second
call, and the name is what a consumer greps for. `ClaimValueType` rides along because a value
of `"true"` means nothing without knowing whether the definition is a `bool` or a `string`.
`AppliesTo`, `DefaultValue` and the definition's `archived_at` are **not** traversed: the first
two belong to the catalog's own read, and republishing an owning aggregate's archive stamp
through a neighbour is a back door to a decision already made the other way — the same line
`UserGroup` and `UserRole` both hold.

**Load-only**, like every collection join in this service: a filter over `claimName` is a typed
400. That is the 1:N boundary, not the backing.

### 4c — `ClaimValue`, and why it is a value object here when `DefaultValue` is plain

The catalog's `DefaultValue` is deliberately **plain** — its rule is "parse as whatever
`ValueType` says", a rule that reads another field, which a value object cannot see — and its
256-rune budget is enforced by a `length` rule in `rules.list`.

On a **child field that path is not available**: `rules.list[].fields` addresses a collection by
its name for `groupCap`/`childDuplicate`, and every child field in this project that needs
validating gets it from a value object instead (`ClientAllowedCIDR.CIDR` → `CIDRBlock`,
`.Label` → `DisplayName`). So `Value` gets a **generated raw VO**:

```yaml
  - name: ClaimValue
    kind: raw
    backing: string
    minLength: 1
    maxLength: 256
    notification: InvalidClaimValueNotification
```

It owns exactly the two invariants that do **not** depend on the definition: the value is
present (never null and never empty on the edge — the backlog's own words) and it fits the
**claim-size budget**, which is the same 256 the catalog's default carries because they are two
levels of one chain feeding one header. Everything type-dependent stays a manual rule (§4d),
where it can read the other aggregate.

The asymmetry with `DefaultValue` is real and is stated here rather than hidden: level 2 gets
its budget from a list rule with `skipWhen: null`, level 1 from a value object, because level 2
may legitimately be null and level 1 may not.

**DEVIATION, recorded 2026-08-28.** This spec planned to declare `ClaimValue` identically in
both parents' specs, on the redeclaration idiom the project uses for
`RoleNotAvailableInTenantNotification`. `generate` **refused** it: *"the project already
declares a value object named ClaimValue → reuse it instead; a second copy is a rule that can
drift from the first"*. The generator's argument is better than the plan's, and the
distinction it draws is real — a notification is TEXT, so three verbatim copies cannot
disagree about anything that matters; a value object is a RULE. So `ClaimValue` is declared
once, in `user.omnicore.yaml`, and `client.omnicore.yaml` reaches it with
`vo: {kind: reuse, ref: ClaimValue}`. One generated file, one owner.

### 4d — The rules

**Declarative (`rules.list`):**

| id | kind | scope | what |
|---|---|---|---|
| `claims-cap` | `groupCap` | `insertOrUpdate` | at most **20** entries, counted over the whole collection, `TooManyClaimsForUserNotification` with `tvars: [max]` |

Twenty, decided at the gate. `Value` caps at 256 runes, so 20 entries is ~5 KB of level-1
values alone — before the claim names, and before `permissions`, `groups` and `roles` which are
already in the token. That fits the 8 KB header buffer nginx and most proxies default to; 50
(the cap `groups` and `roles` carry) would be ~12.8 KB and would stop being a cap in any real
sense. It is also the cap `ClientAllowedCIDR` already uses.

There is **no duplicate rule to declare**: `businessIdentity` + `duplicateNotification` on the
collection is that check, and the active-only unique index is its concurrent backstop.

**Manual (`rules.manual`) — the three the maintainer confirmed on 2026-08-28**, each judging
only the entries a write **adds or changes**, never the ones already stored, and each mirroring
the shape `role-available-in-tenant` already ships:

| id | scope | notification | attachTo |
|---|---|---|---|
| `claim-available-in-tenant` | `insertOrUpdate` | `ClaimNotAvailableInTenantNotification` | `Claims` |
| `claim-applies-to-user` | `insertOrUpdate` | `ClaimDoesNotApplyToUserNotification` | `Claims` |
| `claim-value-matches-value-type` | `insertOrUpdate` | `ClaimValueDoesNotMatchValueTypeNotification` | `Claims` |

1. **`claim-available-in-tenant`** — refuse when the id is absent from `claims`, points at an
   archived definition, **or belongs to a tenant other than this user's**. **One answer for all
   three**, exactly as `Group` and `Role` do: a distinct "belongs to another tenant" reply is an
   existence oracle over a competitor's claim vocabulary. Asks `ClaimIsUnavailableInTenant`.

2. **`claim-applies-to-user`** — refuse when the definition's `AppliesTo` is `client`. `user`
   and `both` pass. This is what finally makes `AppliesTo` mean something: the README says today
   it "states as data something the code does not yet enforce", and this rule (with its twin on
   `Client`) is the enforcement. Asks `ClaimDoesNotApplyToUser`.

3. **`claim-value-matches-value-type`** — refuse when the value does not parse as the
   definition's declared `ValueType`: `number` accepts a valid decimal, `bool` accepts exactly
   `"true"` or `"false"`, `string` accepts any non-empty value. Deliberately **the same three
   readings** the catalog's own `default-value-matches-value-type` uses — two levels of one
   chain must not disagree about what a `bool` is. Read the enum member off the definition
   rather than its raw string. Asks `ClaimValueDoesNotMatchValueType`.

**Ordering, and a correction to what this spec first said.** Availability runs first, the way
the group triple does. This section originally predicted all three notifications firing for one
unresolvable id; the group and role loops it was modelled on do **not** do that — each
`continue`s after the availability refusal, on the reasoning written into
`user_rules_manual.go`: *"nothing below can say anything true about a group that is not there
… reporting all three for one bad id would be noise"*. The implementation follows the
neighbours rather than the paragraph, so one bad id gets one answer. Every fact still answers
"the problem is present" for an id it cannot resolve, which is the safe direction and what
makes the interlock a tidiness measure rather than the only thing standing between a bad id and
a pass.

**No escalation rule, and that is a decision.** `Group` and `Role` both carry a
"caller must already hold every permission this confers" rule because they confer permissions.
A claim confers nothing inside this service — it is not in `BuildRules`, it gates no route, and
there is no permission set to compare a caller against. Adding a wildcard probe or an
escalation probe here would gate nothing. What answers the same worry is the **verb**:

### 4e — The verb

**`user:set-claim`, a new literal**, decided at the gate. The precedent is this service's own,
opened three days ago: `ClientAllowedCIDR` refused `client:grant` and took
`client:manage-network`, on the argument that folding a security control into the role verb
hands everyone who manages roles a different blast radius. The same argument applies with one
turn more — authcore cannot see what a consumer does with `x_cost_center`, so setting a value
is potentially conferring privilege *in a way this service cannot audit*, and "may hand out
roles" and "may set any claim on anybody" are not obviously the same job.

**One verb for all three operations**, as on every other collection in this service. Splitting
add from remove would make this the only collection whose verbs disagree, and would leave the
operator who set a wrong value unable to take it back.

**Cost, stated:** `user:set-claim` and `client:set-claim` join `role:grant`, `group:grant`,
`user:grant`, `user:reset-password` and `user:change-password` on the README's list of literals
nobody guesses from the pattern, taking it from five to seven. Both need a catalog row before
they gate anything, which is an operator insert like every other literal in this service.

### 4f — Facts

Three new entries under `service.facts`, all `kind: manual` — a computed fact is a query over
this entity's own table, and every column these read is on another one. All three are **named
for the problem**, so the generated suite's stub (which answers "nothing found") reads them as
"nothing wrong" and the happy path passes on the day it is written.

| fact | returns | filters |
|---|---|---|
| `ClaimIsUnavailableInTenant` | `bool` | `[TenantID, Claims.ClaimID]` |
| `ClaimDoesNotApplyToUser` | `bool` | `[Claims.ClaimID]` |
| `ClaimValueDoesNotMatchValueType` | `bool` | `[Claims.ClaimID, Claims.Value]` |

Each answers **true** for an id it cannot resolve. One read of `claims` by primary key answers
all three, so memoise per request — the same note `GroupGrantsWildcard` and
`CallerLacksAnyPermissionOfGroup` already carry.

---

## §4-view — View evolution

**No `Version(N)` to bump, on any view in this repo.** `read.backing: relational` on `User`,
on `Client` and on every other entity here: `internal/infra/views/user_view.go` is a relational
projection, not a materialized Mongo view, so there is no rebuild hash and no forgot-to-bump
boot abort in this posture. The nested `claims` array appears on the read document by
regeneration alone.

No `JoinView` embedder, no `ComposedView`, no `SharedBaseView` anywhere in this service — so the
whole embedder-coupling class of misses is N/A rather than merely unlikely.

**Archive regime:** unchanged. The collection is `softRemove: true` on a relational backing,
which serves no `DeleteOnArchive()` — there is no archive mode being newly enabled on the
aggregate, so nothing to elicit.

---

## §4b — Is this a column at all, or a read join?

Asked deliberately, because the skill's trap is exactly this shape. **It is a column**, and here
is why the join answer is wrong:

- The value being added is **not** a value that belongs to `Claim` and can be reached across an
  existing foreign key. `User` holds no FK to `claims` today, and what is being stored is a
  value that exists **nowhere else** — the specialised value, per principal.
- The reach is genuinely **1:N**: one user holds values for many definitions. A read join
  answers a scalar hop, not a collection.
- So: a new table, a new collection, and a read join **in addition** (§4b above) purely to name
  the definition on the way back out.

---

## §5 — API impact

**Purely additive. Nothing breaks.**

Three new REST routes, and their GraphQL mutations:

```
POST   /users/{id}/claims                        set a value       — user:set-claim
PATCH  /users/{id}/claims/{entryId}              correct the value — user:set-claim
PATCH  /users/{id}/claims/{entryId}/archive      remove one        — user:set-claim
```

**Wire-visible additions to existing responses**, which is the one thing worth reading twice:
`GET /users`, `GET /users/{id}` and every write response gain a `claims` array on the user
document, each entry carrying `id`, `claimId`, `value`, `claimName`, `claimValueType` and the
managed stamps. Additive for any consumer that ignores unknown fields — which is every consumer
of a JSON API that has not opted into strictness — and it also **widens `?fields=`**: `claims`
becomes an accepted value. No existing field changes name, type, nullability or meaning; no
field is removed; no validation is tightened on anything that already existed.

`POST /users` and `PATCH /users` do **not** accept claims inline: `editStrategy: per-child` is
the whole point, and an omitted collection on a wholesale write must never de-provision
anything.

**No token changes.** Restated here because it is the thing a reader of this spec will most
expect and not find: `buildClaims` is untouched, `auth.auditClaims` is untouched, and a token
minted after this run is byte-identical to one minted before it.

---

## §6 — Translations

All seven catalogs (`eng`, `ptbr`, `esp`, `fra`, `deu`, `ita`, `nld`), real translations, no
orphans. **No key is removed or renamed by this change**, so there is nothing to prune on the
user side beyond what §Verify checks.

**Field labels (2):** `UserClaimClaimIDField`, `UserClaimValueField`.
**Join-field labels (2):** `UserClaimClaimNameField`, `UserClaimClaimValueTypeField`.

**Notifications (6):**

| name | semantic | shared with `Client`? |
|---|---|---|
| `InvalidClaimValueNotification` (package `vos`) | validation | yes — redeclared verbatim |
| `UserAlreadyHoldsClaimNotification` | conflict | no — `Client` declares its own |
| `TooManyClaimsForUserNotification` (`tvars: [max]`) | validation | no — `Client` declares its own |
| `ClaimNotAvailableInTenantNotification` | validation | yes — redeclared verbatim |
| `ClaimDoesNotApplyToUserNotification` | validation | no — `Client` declares its `...ToClient` twin |
| `ClaimValueDoesNotMatchValueTypeNotification` | validation | yes — redeclared verbatim |

The three shared ones follow this project's existing idiom: declared **identically** in both
specs, exactly as `group`, `user` and `client` each carry their own verbatim copy of
`RoleNotAvailableInTenantNotification`.

Note `ClaimValueDoesNotMatchValueTypeNotification` is a **distinct** type from the catalog's
`DefaultValueDoesNotMatchValueTypeNotification`: same question, two different subjects, and
collapsing them would attach a level-1 refusal to a level-2 field name.

---

## §7 — Tests

**Existing tests that change:** none of them change *meaning*. Every `user_*_test.go` file is
generator-owned and gets regenerated with the new collection in it; no assertion is weakened,
edited, or relaxed to pass. If a regenerated suite goes red, the spec is wrong, not the test.

**New branches that need coverage, hand-written** (`internal/domain/user_rules_manual_test.go`,
and the collection's own `user_children_manual_test.go`):

1. `claim-available-in-tenant` — absent id · archived definition · definition of another tenant
   (all three must produce the **same** notification, and that sameness is itself the assertion)
   · a valid definition of this tenant passes.
2. `claim-applies-to-user` — `AppliesTo: client` refuses · `user` passes · `both` passes.
3. `claim-value-matches-value-type` — per declared type: `number` accepts `"1000"` and refuses
   `"abc"`; `bool` accepts `"true"`/`"false"` and refuses `"1"` and `"TRUE"`; `string` accepts
   any non-empty. Plus: a value for an unresolvable id refuses rather than passing.
4. `claims-cap` — 20 entries pass, 21 refuse, and the bound reaches the message through `{max}`.
5. Duplicate — two entries for the same `ClaimID` in one write refuse via
   `UserAlreadyHoldsClaimNotification`; the same `ClaimID` under a *different* user is fine.
6. `change` — correcting `Value` on an existing entry keeps the row id (the property that
   justified the verb existing at all).
7. Soft remove — a removed entry leaves the row with `archived_at` set, and re-adding the same
   `ClaimID` afterwards is accepted (the active-only index releasing the value).
8. `ClaimValue` — empty refuses, 256 runes pass, 257 refuse, counted in **runes** not bytes.

**Contract QA:** N/A — `specs/qa/` does not exist in this repo. Nothing planned, nothing
skipped.

**Coverage:** the project's 95% floor applies to what this run writes. No production code is
changed to enable testability.

---

## §8 — Kind promotion

**N/A.** `User` stays `storage.kind: flat`. Nothing about this change touches the base/role
split, and no natural key, FK model or data move is involved.

---

## §9 — Generation path: the gateway

`omnicore-gen doctor -project .` runs **clean**: all seven entities resolve against their specs
at framework `v0.62.0`, no `! was edited by hand`, no `· carries a hand edit adopted at`, no
`! the spec changed since the last generation`, no missing spec. There is nothing to reconcile
before regenerating.

`User` **is** recorded in `specs/omnicore-gen/lock.json`, so the codegen path is available and
the choice is real.

**Expressibility, checked against `explain keys` and `explain rules` rather than from memory:**
every piece of this change is in the language — `children[].operations` accepts `change`;
`children[].permissions` keys per verb; `joins[].inChild` hangs a traversal off a collection;
`valueObjects[].kind: raw` with `minLength`/`maxLength`/`notification`;
`service.facts[].filters` accepts `<collection>.<field>` on `kind: manual`. What lands **by
hand on either path**, named up front: the migration pair (§3), the three `rules.manual` bodies
and their three fact bodies (§4d/§4f), their tests, and the three documentation files
(artifacts 27–29).

---

## ⏸️ Approval

Two answers are needed, and the second is asked once for both parents.

**A. Is the change above right?** — this spec and its sibling
`specs/evolve-entity/client/spec.md`, which is the same shape on the other parent.

**B. How should it be applied?**

**1. Change the spec and regenerate — `omnicore-gen` (beta) + review by me.**
The change goes into `specs/omnicore-gen/user.omnicore.yaml` and
`specs/omnicore-gen/client.omnicore.yaml`; `check` validates them and `generate` rewrites every
file the generator owns — 20 of the 29 artifacts above — in seconds and at a fraction of the
tokens the by-hand path costs. It also catches statically what an evolution forgets. **What it
does not do is touch the database:** the two migration pairs are written by hand, by me,
against the shape the generator's report prints — a migration is written once and never
regenerated. Same for the three manual rules, the three facts, their tests, and the three
docs. I then review the emitted code against this spec — not for plausibility, against the spec
that produced it, because the generator can be wrong too and its mistakes compile. If I find
one, **I stop and ask before touching a single generated file**. Beta: its gate covers a lot but
can still hit a case nobody has hit; when that happens I say so, work around it, and it gets
fixed upstream.

**2. By hand, file by file, by me.**
I edit all 29 artifacts myself, reading the pinned `/docs` before each layer. Slower, far more
tokens, same review discipline — but nothing depends on the generator. **On these two entities
it carries a permanent cost:** every generator-owned file I edit stops tracking its spec.
`doctor` reports it as a hand edit, the next regeneration refuses it until adopted or forced,
and later emitter fixes never reach it. Given that 20 of 29 artifacts here are generator-owned
— including `user.go`, the repository, the routes and the feature wiring — this path would
detach most of the `User` and `Client` trees permanently. I would list every file touched and
offer to record each with `adopt … -why`, which makes `doctor` tell the truth afterwards but
does not undo the divergence.

Neither is marked recommended while the generator is in beta.

---

## Promoted out of this impact map — `Claim`'s own run

**`AppliesTo` can still be narrowed out from under a stored value.** `AppliesTo` is mutable on
the catalog — "widening is the ordinary operational move", and it is. But *narrowing* `both` →
`user` while `client_claims` rows exist for that definition strands those values: they stay in
the table, they stay readable, and they would violate `claim-applies-to-client` if written
today.

Flagged here, and **promoted at the gate on 2026-08-28** rather than deferred: it is
`specs/evolve-entity/claim/spec.md`, a third run adding one `rules.manual` entry and two facts
to `Claim`. It stays out of THIS impact map because it is a third entity's evolution, and it
**runs last** — its two facts query `user_claims` and `client_claims`, which do not exist until
this run and its sibling have landed.

---

## Correction, 2026-08-28 — the `change` verb was removed after review

The approved design mounted `operations: [add, change, remove]`, on `backlog.md`'s argument
that here the identity is the `ClaimID` and `Value` is a separate mutable column, so
correcting a value is *"genuinely one entry changing rather than a remove and an add"*.

**That argument does not survive contact with the verb the generator emits.** A `change`
mounts as `PUT /{parent}s/:id/claims/:entryId` whose body is the entry's WHOLE shape —
`claimID` included. Two consequences, and the maintainer caught both:

1. **The caller re-sends a value the server already knows.** `claimID` identifies the entry's
   definition and never changes; asking for it on every correction is asking the caller to
   repeat the server's own state.
2. **It makes the business identity mutable.** `PUT …/claims/{entryX}` with a different
   `claimID` turns "the value for claim A" into "the value for claim B" keeping entry X's row
   id — verbatim the audit failure `backlog.md` cites when it refuses `change` on
   `role_permissions`. The traced path is
   `ChangeClientClaimByID(cmd.ClientClaimID, {ClaimID: cmd.ClaimID, Value: cmd.Value})`.

The right shape is a PATCH carrying **only** `value`, with the definition copied from the
stored entry — no field to send wrong, rather than a rule refusing it when it is.

**The spec language cannot express that, and the asymmetry is what makes it a gap rather than
a key nobody guessed:** the ROOT has `update.shape: patch` and `update.patchExcludes`; a
collection entry has neither, and `children[].operations` only selects WHICH verbs mount,
never the shape of their body. `rules.list` cannot reach a child's field either — `check`
answers *"`Claims.ClaimID` does not name a field in this scope"* — so even the weaker
"validate it" fallback is not declarative.

**Resolution: `operations: [add, remove]`, everything through the generator, nothing by hand.**
Correcting a value is archive + add. That is what the other four collections in this service
do, and it is the BETTER audit trail rather than a worse one: `value` is a single column with
no history, so an in-place change OVERWRITES the previous value, while an archived row keeps
it readable.

The one-call correction returns when omnicore-gen can emit a partial change. Until then the
rule already covers it — `refuseUnsettableClaims` judges added **and** changed entries, and the
test for the changed state is kept deliberately, because that is the assertion which stops a
corrected value from reaching the column unjudged on the day the verb comes back.

### Follow-up, same day — the verb came back as a PATCH

`omnicore-gen` 0.49.0 (on framework v0.63.0) added the key this section said was missing:
`children[].change`, with `shape: patch | put | both` and `patchExcludes` — the entry-level
twin of the root's `update` block, in the same two words. So the gap is closed and the
collection now declares:

```yaml
    operations: [add, change, remove]
    change:
      shape: patch
      patchExcludes: [ClaimID]
```

`PATCH /{parent}s/:id/claims/:entryId` takes `{"value": "…"}` and nothing else — verified on
the running service's OpenAPI, where the request schema has one optional property. The
generated command reads the entry AS STORED and overlays only what the body carried, so the
definition an entry belongs to comes from the row and never from the caller. **Nothing to
send wrong, rather than a rule refusing it when it is** — the same posture `Claim` already
takes with its own `name` and `valueType`.

**The framework's v0.63.0 guard is the floor, not the fix, and it is worth being precise
about which is which.** It refuses a child change whose replacement would COLLIDE with another
ACTIVE entry's business identity (`EntityAlreadyAddedNotification`, 409). It deliberately does
NOT freeze identity fields — the changelog says so in as many words, because an aggregate keyed
by a natural key edits exactly those fields through that primitive. So the guard closes the
duplicate hole for every consumer; `patchExcludes` is what closes THIS entity's identity hole.

Two corrections to what this section said before, both material:

- **"An in-place change overwrites the previous value" was wrong.** The framework's audit
  writes a changed child as op `"updated"` with a `changes` block — a diff against the
  pre-mutation child, paired by `GetID()` (`docs/content/sections/old-state.html`). The
  previous value is in `audit_events`. The column has no history; the trail does. The
  archive+add argument this section leaned on does not hold.
- **The guard proposed to the framework maintainer was declined in favour of a better one.**
  It asked for `changeAggregateItem` to refuse a replacement whose business identity differs
  from the original's. What shipped refuses only the COLLISION, and keeps identity edits legal
  — which is the correct call: freezing them would have broken every natural-key aggregate.
