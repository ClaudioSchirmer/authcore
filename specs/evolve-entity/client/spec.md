# Evolve `Client` — the `claims` collection (level 1 of the claim chain, client side)

**Status: APPROVED** — 2026-08-28.
**Generation: omnicore-gen** — chosen at the same gate, and governs all three runs.
**Sibling run:** `specs/evolve-entity/user/spec.md` builds the same collection on `User`, and
carries the long form of every shared argument. This spec states each decision and points
there for the reasoning rather than repeating it, except where the client side genuinely
differs — §4e, §5 and the last section below.

---

## §1 — The change, in one paragraph

`Claim` shipped on 2026-08-28 as the catalog — **level 2** of a two-level chain, the tenant-wide
`default_value`. Level 1, the specialised value per principal, does not exist on either parent.
This run adds it on `Client`: a new owned collection `Claims` (table `client_claims`), one row
per claim definition this machine credential holds a value for, holding the definition's id and
the value. It is written through per-entry verbs gated on a **new** `client:set-claim`
permission, and validated against the definition it points at — same tenant, declared type, and
an `appliesTo` that admits a client. **No token changes in this run**: `buildClaims` is
untouched.

`Client` is the parent that most needed this. `backlog.md` opens the whole `Claim` entry with
it: *"`Client` cannot borrow `Group` for this — it has no groups"*, which is why a per-principal
edge was the only shape that could serve both identity kinds. And `POST /auth/client/token`
does not exist yet, so on this side the collection is doubly ahead of its consumer — the token
it will eventually feed has not been built either. That is not a reason to wait: the shape is
the same on both parents and learning it once is the point.

**Scope, decided at the gate (2026-08-28):** the collection only. Emission belongs to a separate
`/omnicore:implement` run — see the user spec §1.

---

## §2 — Impact map

`G` = generator-owned · `H` = hand-written on either path · `M` = a `_manual` hook.

| # | Artifact | Who | What changes |
|---|---|---|---|
| 1 | `specs/omnicore-gen/client.omnicore.yaml` | **H** | One `children[]` entry, one `joins[]` entry, one raw value object, six notifications, three service facts, one list rule, three manual rules |
| 2 | `migrations/postgres/0011_client_claims_manual.up.sql` + `.down.sql` | **H** | **New pair.** `client_claims`, its parent index, its active-only unique index, and by hand the FK into `claims` |
| 3 | `internal/infra/schemas/client_claim_schema.go` | G | New — the collection's TableSchema |
| 4 | `internal/infra/schemas/client_schemas_test.go` | G | Regenerated |
| 5 | `internal/domain/aggregatevos/client_claim.go` | G | New — the entry type |
| 6 | `internal/domain/vos/claim_value.go` | G | The `ClaimValue` raw VO — **shared with the `User` run**, written once (user spec §4c) |
| 7 | `internal/domain/client.go` | G | `Claims` collection; `BuildRules` gains the cap and the duplicate clause |
| 8 | `internal/domain/client_service.go` | G | Three new facts on the service port |
| 9 | `internal/domain/client_rules_manual.go` | **M** | Three new rule stubs (§4d) |
| 10 | `internal/domain/notifications.go`, `internal/domain/vos/notifications.go` | G | Six notification types |
| 11 | `internal/infra/client_service.go` | G | Regenerated service adapter |
| 12 | `internal/infra/client_service_manual.go` | **M** | Three fact bodies (§4d) |
| 13 | `internal/infra/client_repository.go` | G | The collection's mapping + the `inChild` read join into `claims` |
| 14 | `internal/infra/views/client_view.go` | G | The `claims` array on the read document |
| 15 | `internal/application/dtos/client_claim_input.go` | G | New — the write DTO |
| 16 | `internal/application/commands/client_claim_commands.go` | G | New — add / change / remove |
| 17 | `internal/application/commands/client_child_results.go` | G | The entry's result shape |
| 18 | `internal/application/queries/client_row_results.go` | G | The nested `claims` on the read result |
| 19 | `internal/web/requests/client_claim_requests.go` | G | New — the three request shapes |
| 20 | `internal/web/requests/client_children.go` | G | The collection's request wiring |
| 21 | `internal/web/client_routes.go` | G | Three new routes, each declaring `client:set-claim` |
| 22 | `bootstrap/clients_feature.go` | G | The collection's handlers registered |
| 23 | `internal/application/translations/{eng,ptbr,esp,fra,deu,ita,nld}.go` | G | Two field labels + two join labels + six notification texts × 7 catalogs (§6) |
| 24 | `internal/application/translations/client_translations_test.go` | G | Regenerated |
| 25 | `internal/domain/client_test.go`, `client_rules_manual_test.go` | G + **H** | Generated suite regenerated; the three manual rules' tests are hand-written (§7) |
| 26 | `internal/application/{commands,queries,dtos}/client_*_test.go`, `internal/web/requests/client_requests_test.go`, `internal/infra/views/client_view_test.go` | G | Regenerated |
| 27 | `README.md` | **H** | The `Client` row in the capability table; the route list; the permission-literal note gains `client:set-claim`; `client:*` goes from **seven** verbs to **eight** |
| 28 | `ACCESS_MATRIX.md` | **H** | Three route rows under `## Client` |
| 29 | `backlog.md` | **H** | The `Claim` entry: the client half of "the two edge collections" stops being open (the user run closes the other half) |

**Classes an evolution forgets, checked and found N/A:** `microservice.*.yaml` — no
`publicRoutes` change (the four public entries are the probes and the two user-token routes;
nothing under `/clients` is public), no `auditClaims` change (emission's question), no
`integration:` block in this service. No proto/gRPC surface. No hand-written GraphQL schema —
`surfaces.graphql.enabled: true` with no `mutations` narrowing carries the three verbs through.
No `specs/qa/` in this repo. No view of another entity embeds `Client`.

---

## §3 — Migration strategy

**Purely additive: one new table.** Nothing to backfill, nothing to default, no data at risk.

A **new numbered pair**, `0011_client_claims_manual.{up,down}.sql`, in `migrations/postgres/` —
the only target dialect. It sits after `0010` (the user run's pair); if the two runs land
separately the numbers swap accordingly, and the numbering is settled at execution time rather
than assumed here.

`up` — following `client_roles` verbatim except for the second column:

```sql
CREATE TABLE "client_claims" (
  "id"         UUID NOT NULL,
  "client_id"  UUID NOT NULL,
  "claim_id"   UUID NOT NULL,
  "value"      VARCHAR(256) NOT NULL,
  "deleted_at" TIMESTAMPTZ NULL,
  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT "client_claims_pkey" PRIMARY KEY ("id"),
  CONSTRAINT "client_claims_parent_fk" FOREIGN KEY ("client_id") REFERENCES "clients" ("id") ON DELETE CASCADE
);
CREATE INDEX "client_claims_parent_idx" ON "client_claims" ("client_id");
CREATE UNIQUE INDEX "client_claims_client_id_claim_id_key"
  ON "client_claims" ("client_id", "claim_id") WHERE "deleted_at" IS NULL;
```

Plus `COMMENT ON` for the table and every column. **No `revision` column** — the stamp lives on
`clients` and guards the aggregate as a whole, as on every other collection here.

**Hand-written below the generator's line:**

```sql
ALTER TABLE "client_claims"
  ADD CONSTRAINT "client_claims_claim_fk"
  FOREIGN KEY ("claim_id") REFERENCES "claims" ("id");
CREATE INDEX "client_claims_claim_idx" ON "client_claims" ("claim_id");
```

`NO ACTION` on delete: a claim definition is archived, never purged. The reverse index makes the
read join cheap and answers "who holds a value for this definition" when the emission run needs
it.

`down` — `DROP TABLE "client_claims"`. Structure only on a re-`up`.

---

## §4 — The collection, decision by decision

### 4a — Shape

```yaml
  - name: ClientClaim
    plural: Claims
    table: client_claims
    parentColumn: client_id
    ownedBy: root
    editStrategy: per-child
    operations: [add, change, remove]
    change:
      shape: patch
      patchExcludes: [ClaimID]
    softRemove: true
    archivedAt: deleted_at
    businessIdentity: [ClaimID]
    duplicateNotification: ClientAlreadyHoldsClaimNotification
    permissions:
      add:    client:set-claim
      change: client:set-claim
      remove: client:set-claim
    fields:
      - ClaimID  (id,     claim_id, unique: constraint-only / active-only)
      - Value    (string, value, length 256, vo: {kind: raw, ref: ClaimValue})
```

Byte-for-byte the `User` shape with the parent swapped. `operations` including `change`,
`businessIdentity: [ClaimID]` alone, and `softRemove: true` are argued in the user spec §4a and
hold identically here — the entry's identity is the definition, the value is a separate mutable
column, and every collection in this service soft-removes.

Worth one line specific to this parent: `ClientAllowedCIDR` sits right beside this collection
with a second mutable column (`Label`) and **no** `change` verb. The two are not in tension —
correcting a CIDR's label is cosmetic, correcting a claim's value is the entire purpose of the
row — but a reader will notice the asymmetry, so it is recorded rather than left to be
rediscovered.

### 4b — Reading it back

```yaml
  - kind: inner
    to: Claim
    inChild: Claims
    on: claim_id
    fields:
      - ClaimName      (column: name)
      - ClaimValueType (column: value_type)
```

`inner` is safe because `claim_id` is NOT NULL and FK-backed (§3) — inside a collection an inner
join drops the **entry**, so that condition is load-bearing. `AppliesTo`, `DefaultValue` and the
definition's `deleted_at` are not traversed, for the reasons in the user spec §4b. Load-only: a
filter over `claimName` is a typed 400.

### 4c — `ClaimValue`

The generated raw VO (`minLength: 1`, `maxLength: 256`,
`notification: InvalidClaimValueNotification`), declared **once** in `user.omnicore.yaml` and
reached here with `vo: {kind: reuse, ref: ClaimValue}`. This spec planned a verbatim
redeclaration in both files and `generate` refused it — see the user spec §4c for the
generator's argument, which is better than the plan's. Full reasoning for why level 1 gets a
value object where the catalog's level-2 `DefaultValue` is deliberately plain: same section.

### 4d — The rules

**Declarative:** one — `claims-cap`, `kind: groupCap`, `scope: [insertOrUpdate]`, **cap 20**,
`TooManyClaimsForClientNotification` with `tvars: [max]`. Twenty for the header-budget reason in
the user spec §4d; it is also the cap `allowed-cidrs-cap` already carries on this same entity.

**Manual — the three the maintainer confirmed on 2026-08-28**, each judging only the entries a
write adds or changes:

| id | scope | notification | attachTo |
|---|---|---|---|
| `claim-available-in-tenant` | `insertOrUpdate` | `ClaimNotAvailableInTenantNotification` | `Claims` |
| `claim-applies-to-client` | `insertOrUpdate` | `ClaimDoesNotApplyToClientNotification` | `Claims` |
| `claim-value-matches-value-type` | `insertOrUpdate` | `ClaimValueDoesNotMatchValueTypeNotification` | `Claims` |

1. **`claim-available-in-tenant`** — absent id, archived definition, or a definition belonging to
   another tenant: **one answer for all three**, as `role-available-in-tenant` already does on
   this entity. Asks `ClaimIsUnavailableInTenant`.
2. **`claim-applies-to-client`** — refuse when the definition's `AppliesTo` is `user`; `client`
   and `both` pass. The mirror of the user rule, and the pair is what makes `AppliesTo` mean
   something for the first time. Asks `ClaimDoesNotApplyToClient`.
3. **`claim-value-matches-value-type`** — the value must parse as the declared type, using the
   **same three readings** the catalog's own `default-value-matches-value-type` uses: `number` a
   valid decimal, `bool` exactly `"true"` or `"false"`, `string` any non-empty. Asks
   `ClaimValueDoesNotMatchValueType`.

Ordering: user spec §4d. The loop `continue`s after an availability refusal, so one bad id
gets one answer — matching the role loop this entity already ships, and correcting what that
section first predicted.

**No wildcard rule and no escalation rule.** `no-wildcard-role` and `no-escalation` exist on
this entity because a role confers permissions and *"a machine credential carrying `*:*` is the
single worst thing this entity could mint"*. A claim confers nothing inside this service: it is
not in `BuildRules`, it gates no route, and there is no permission set to compare a caller
against. A probe here would gate nothing.

### 4e — The verb, and the one interaction specific to this parent

**`client:set-claim`, a new literal**, decided at the gate — the argument is in the user spec
§4e, and its strongest precedent is on this very entity: `ClientAllowedCIDR` refused
`client:grant` and took `client:manage-network` three days ago. This takes `client:*` from
**seven** verbs to **eight** (`read` · `insert` · `update` · `archive` · `grant` ·
`rotate-secret` · `manage-network` · `set-claim`).

**Interaction with `client-rotates-only-its-own-secret`, checked:** none, and deliberately none.
That rule was narrowed on 2026-08-28 to the secret rotation alone, on the reading that
*"creating, editing, archiving and granting are ordinary tenant-scoped writes, gated by the
permission the caller carries"*, and that handing out a **credential** is the one act that must
stay on the caller's own row. Setting a claim value is administration, not credential issuance,
so it follows the ordinary reading: a client-subject caller holding `client:set-claim` may set
claims on its tenant's other clients, exactly as it may grant them roles today. This spec adds
no new clause to that rule.

### 4f — Facts

Three `kind: manual` facts, all **named for the problem** so the generated suite's stub reads
them as "nothing wrong":

| fact | returns | filters |
|---|---|---|
| `ClaimIsUnavailableInTenant` | `bool` | `[TenantID, Claims.ClaimID]` |
| `ClaimDoesNotApplyToClient` | `bool` | `[Claims.ClaimID]` |
| `ClaimValueDoesNotMatchValueType` | `bool` | `[Claims.ClaimID, Claims.Value]` |

Each answers **true** for an id it cannot resolve. One read of `claims` by primary key answers
all three — memoise per request.

---

## §4-view — View evolution

**No `Version(N)` to bump.** `read.backing: relational` on `Client` and on every entity in this
service: `internal/infra/views/client_view.go` is a relational projection, not a materialized
Mongo view, so there is no rebuild hash and no forgot-to-bump boot abort in this posture. The
nested `claims` array appears by regeneration alone. No `JoinView`, `ComposedView` or
`SharedBaseView` anywhere in this repo. Archive regime unchanged.

---

## §4b — Is this a column at all, or a read join?

**A column.** `Client` holds no foreign key to `claims` today, the value being stored exists
nowhere else (that is what "specialised value" means), and the reach is 1:N — one client, many
definitions. A read join answers a scalar hop across an existing key; this is neither. The read
join in §4b above is an addition on top, purely to name the definition on the way back out.

---

## §5 — API impact

**Purely additive. Nothing breaks.**

```
POST   /clients/{id}/claims                        set a value       — client:set-claim
PATCH  /clients/{id}/claims/{entryId}              correct the value — client:set-claim
PATCH  /clients/{id}/claims/{entryId}/archive      remove one        — client:set-claim
```

`GET /clients`, `GET /clients/{id}` and every write response gain a `claims` array on the client
document, each entry carrying `id`, `claimId`, `value`, `claimName`, `claimValueType` and the
managed stamps. Additive for any consumer that ignores unknown fields, and `?fields=claims`
becomes accepted. No existing field changes name, type, nullability or meaning; nothing is
removed; no validation is tightened on anything that already existed.

`POST /clients` and `PATCH /clients` do not accept claims inline — `editStrategy: per-child`.

**One thing worth saying out loud on this side**, in the spirit of the `POST /auth/client/token`
backlog entry: a value set here reaches **no token at all today**, and will only ever reach a
client token once *both* the emission run and `POST /auth/client/token` exist. Neither does.
Anyone reading a `client_claims` row as "this machine's token says `x_region=sa-east-1`" is
reading a promise, not a fact.

---

## §6 — Translations

All seven catalogs, real translations, no orphans. No key is removed or renamed.

**Field labels (2):** `ClientClaimClaimIDField`, `ClientClaimValueField`.
**Join-field labels (2):** `ClientClaimClaimNameField`, `ClientClaimClaimValueTypeField`.

**Notifications (6):**

| name | semantic | shared with `User`? |
|---|---|---|
| `InvalidClaimValueNotification` (package `vos`) | validation | yes — redeclared verbatim |
| `ClientAlreadyHoldsClaimNotification` | conflict | no |
| `TooManyClaimsForClientNotification` (`tvars: [max]`) | validation | no |
| `ClaimNotAvailableInTenantNotification` | validation | yes — redeclared verbatim |
| `ClaimDoesNotApplyToClientNotification` | validation | no — the `...ToUser` twin lives on `User` |
| `ClaimValueDoesNotMatchValueTypeNotification` | validation | yes — redeclared verbatim |

`ClaimValueDoesNotMatchValueTypeNotification` is a distinct type from the catalog's
`DefaultValueDoesNotMatchValueTypeNotification` — same question, two different subjects.

---

## §7 — Tests

**Existing tests that change:** none in *meaning*. Every `client_*_test.go` is generator-owned
and regenerated with the new collection. No assertion is weakened or edited to pass.

**New branches, hand-written** (`internal/domain/client_rules_manual_test.go`):

1. `claim-available-in-tenant` — absent · archived · another tenant's, all three producing the
   **same** notification (the sameness is the assertion) · a valid definition passes.
2. `claim-applies-to-client` — `AppliesTo: user` refuses · `client` passes · `both` passes.
3. `claim-value-matches-value-type` — `number` accepts `"1000"`, refuses `"abc"`; `bool` accepts
   `"true"`/`"false"`, refuses `"1"` and `"TRUE"`; `string` accepts any non-empty; an
   unresolvable id refuses rather than passing.
4. `claims-cap` — 20 pass, 21 refuse, `{max}` carries the bound.
5. Duplicate — two entries for one `ClaimID` in a write refuse; the same `ClaimID` under a
   different client is fine.
6. `change` — correcting `Value` keeps the row id.
7. Soft remove — the row survives with `deleted_at`; re-adding the same `ClaimID` is accepted.
8. `ClaimValue` — empty refuses, 256 runes pass, 257 refuse, counted in runes.
9. **No escalation probe is consulted** — `TestSettingAClaimValueDoesNotConsultTheCallersPermissions`
   asserts that setting a claim value asks neither `CallerLacksAnyPermissionOfRole` nor
   `RoleGrantsWildcard`, and is accepted under a service that would refuse a role grant. It
   fails if somebody later "fixes" the asymmetry with the roles loop beside it.

**Contract QA:** N/A — no `specs/qa/` in this repo.
**Coverage:** the 95% floor applies to what this run writes; no production code changed for
testability.

---

## §8 — Kind promotion

**N/A.** `Client` stays `storage.kind: flat`.

---

## §9 — Generation path

`omnicore-gen doctor -project .` runs **clean** across all seven entities at framework
`v0.62.0` — nothing to reconcile. `Client` **is** recorded in `specs/omnicore-gen/lock.json`, so
the codegen path is available.

Expressibility was checked against `explain keys` / `explain rules` rather than from memory, and
every piece is in the language; what lands by hand on either path is the migration pair (§3),
the three `rules.manual` bodies and their three fact bodies, their tests, and the three
documentation files (artifacts 27–29).

**The gateway is asked once for both parents** — see `specs/evolve-entity/user/spec.md` §9. The
answer recorded there governs this run too, and is written into both headers the moment it is
given.

---

## Promoted out of this impact map — `Claim`'s own run

Same item as the user spec, and it bites hardest on this side: **`AppliesTo` can still be
narrowed out from under a stored value.** Narrowing a definition from `both` to `user` while
`client_claims` rows hold values for it strands those rows — readable, still there, and
un-writable under `claim-applies-to-client`.

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
