# Evolve `Claim` — refuse an `appliesTo` narrowing that would strand held values

**Status: APPROVED** — 2026-08-28, at the same gate that approved the two edge collections.
**Generation: omnicore-gen** — inherited from that gate; asked once for all three runs.
**Runs LAST.** The two facts this spec adds query `user_claims` and `client_claims`, which do
not exist until `specs/evolve-entity/user/spec.md` and `specs/evolve-entity/client/spec.md`
have landed. Executing this one first would produce a fact body with no table to read.

---

## §1 — The change, in one paragraph

`AppliesTo` is mutable on the catalog, and deliberately so: *"widening is the ordinary
operational move"* — a definition that started `user` becoming `both` is exactly what an
operator does when the machine side of an integration arrives. But the field is mutable in
**both directions**, and narrowing it is not symmetric with widening. Once level 1 exists, a
definition narrowed from `both` to `user` while `client_claims` rows hold values for it
**strands those rows**: they stay in the table, they stay readable on `GET /clients/:id`, and
they would be refused by `claim-applies-to-client` if anyone tried to write them today. Nothing
complains, and nothing ever will. This run closes it with one manual rule on `Claim` and the two
facts it asks.

Found while writing the two edge specs, flagged there as outside their impact maps, and promoted
to its own run at the gate rather than folded silently into either.

---

## §2 — Impact map

`G` = generator-owned · `H` = hand-written on either path · `M` = a `_manual` hook.

| # | Artifact | Who | What changes |
|---|---|---|---|
| 1 | `specs/omnicore-gen/claim.omnicore.yaml` | **H** | One `rules.manual` entry, two `service.facts` entries, one notification |
| 2 | `internal/domain/claim_service.go` | G | Two new facts on the service port |
| 3 | `internal/domain/claim_rules_manual.go` | **M** | One new rule stub to implement (§4) |
| 4 | `internal/domain/notifications.go` | G | One notification type |
| 5 | `internal/infra/claim_service.go` | G | Regenerated service adapter |
| 6 | `internal/infra/claim_service_manual.go` | **M** | Two fact bodies to implement (§5) |
| 7 | `internal/application/translations/{eng,ptbr,esp,fra,deu,ita,nld}.go` | G | One notification text × 7 catalogs |
| 8 | `internal/application/translations/claim_translations_test.go` | G | Regenerated |
| 9 | `internal/domain/claim_test.go` | G | Regenerated suite |
| 10 | `internal/domain/claim_rules_manual_test.go` | **H** | The new rule's branches (§7) |
| 11 | `README.md` | **H** | The `Claim` section: `appliesTo` stops being described as plainly "mutable" and gains the one-way qualifier |
| 12 | `backlog.md` | **H** | The `Claim` entry: record that the narrowing hole was found and closed with level 1 |

**No migration.** No column changes, no table is added, no constraint moves. The invariant is a
domain rule over rows that already exist by the time this runs — the database is untouched, and
saying so plainly is the point: there is no pair to write and none to number.

**No API shape change.** `AppliesTo` stays in the patch body and stays mutable; what changes is
that one class of value is now refused with a typed 422 instead of silently accepted. §6 weighs
whether that is breaking.

**Everything else N/A**, checked rather than assumed: no `microservice.*.yaml` change (no route
path moves, no new public route, `auditClaims` untouched), no proto/gRPC surface, no hand-written
GraphQL schema, no `specs/qa/`, no view of another entity embeds `Claim`, no `Version(N)` to bump
(`read.backing: relational` — `claims` is a relational projection, not a materialized view).

---

## §3 — What counts as a narrowing

Stated exhaustively, because "narrowing" is the whole rule and a half-stated definition is a
half-built guard. `AppliesTo` has three members; six transitions are possible.

| from → to | admits fewer? | user values at risk | client values at risk |
|---|---|---|---|
| `user` → `both` | no — widening | — | — |
| `client` → `both` | no — widening | — | — |
| `both` → `user` | **yes** | — | **yes** |
| `both` → `client` | **yes** | **yes** | — |
| `user` → `client` | **yes** | **yes** | — |
| `client` → `user` | **yes** | — | **yes** |

So the rule is not "refuse narrowing" in the abstract — it is **refuse a new value that no longer
admits an identity kind for which an active edge still holds a value**. A `both` → `user` change
on a definition no client has ever held a value for is perfectly legal and must stay legal:
refusing it would make the field effectively immutable, which is a decision the model gate
already took the other way.

**Not a `transition` rule.** The declarative kind exists and would be the reflex, but it
compiles a fixed state machine — it can say `both` may move to `user`, or that it may not, and
nothing in between. This rule's answer depends on **rows in two other aggregates' tables**, which
no edge list can express. It is a `rules.manual` entry for exactly the reason that section
exists.

---

## §4 — The rule

```yaml
  manual:
    - id: applies-to-narrowing-refused
      scope: [update]
      notification: ClaimAppliesToCannotExcludeHeldValuesNotification
      attachTo: AppliesTo
```

Description, which is what the generated stub and the report hand the implementer:

> Refuse a change to `AppliesTo` that would stop admitting an identity kind for which an ACTIVE
> edge still holds a value. Only the kinds the NEW value drops are asked about, and only when
> the value actually changed: if the new value no longer admits `client`, ask
> `ClaimIsHeldByAClient`; if it no longer admits `user`, ask `ClaimIsHeldByAUser`; refuse when
> either answers true. Widening (`user`→`both`, `client`→`both`) and staying put ask nothing and
> always pass. Read the enum member off `AppliesTo` rather than its raw string, so a value the
> value object already refused does not reach a second answer here.

**Scope is `update` alone.** Insert cannot narrow anything — a definition that has just been
created is held by nobody.

**Archive is deliberately not covered**, and that is a decision rather than a gap. Archiving a
definition while principals hold values for it is an **already-accepted** state: the catalog's
model settled that archive is one-way and that a retired definition comes back as a new row with
a new id precisely so that *"an edge holding the old id cannot silently re-attach to the
recreated definition"*. The edges keep pointing at an archived definition, new writes against it
are refused by `claim-available-in-tenant` on both parents, and the emission run will read a null
for them. Narrowing is different because it leaves the definition **live** and the values
**invisible** — the row is neither refused nor retired, it just quietly stops meaning anything.

**No guard, and no interaction with the existing rules.** `value-type-immutable` already refuses
the other retro-invalidation of the same family, and the two never fire on one field. This rule
does not need the tenant barrier that `tenant-is-a-usable-id` provides — it reads the row's own
id, not the tenant.

---

## §5 — The facts

Two new entries under `service.facts`, both `kind: manual` — a computed fact is a query over
this entity's own table, and both of these read another one.

| fact | returns | filters |
|---|---|---|
| `ClaimIsHeldByAUser` | `bool` | `[TenantID, Name]` |
| `ClaimIsHeldByAClient` | `bool` | `[TenantID, Name]` |

**Filtered by the natural key, not by the row id** — a correction to what this spec first
planned. `check` refused `filters: [ID]` with *"ID does not name a field of this entity"*: a
fact's filters must name declared fields, and the primary key is not one of them. `(TenantID,
Name)` identifies the definition exactly — both immutable, and `Name` unique per tenant.

That uniqueness is scoped to the **active** rows, which is load-bearing in the body rather than
a footnote: an archived definition may share the pair with the live one, so both queries join
with `claims.deleted_at IS NULL` as well. Without it a retired row's leftover edges would block
a narrowing on the row that replaced it, forever, with nothing saying why.

- `ClaimIsHeldByAUser` — whether any **active** `user_claims` row references this definition.
- `ClaimIsHeldByAClient` — whether any **active** `client_claims` row references this definition.

Active only, on both: a soft-removed edge is history, and history must not freeze a definition's
shape forever. That is the direct consequence of choosing `softRemove` on the two collections,
and it is worth naming — with a hard delete the question would not have arisen, and with
active-included the rule would refuse narrowings for values nobody holds any more.

Both query by `claim_id` against the index the two edge migrations add by hand
(`user_claims_claim_idx`, `client_claims_claim_idx`) — which is the second thing those indexes
are for, beside the read join.

**They are the only probes in this service that drop to SQL**, and the reason is that neither
an `AggregateLoader` criteria over `claims` nor one over `users` can phrase the question: it is
"is there an ENTRY of another aggregate's collection pointing at this row", and a collection's
columns are load-only. `Querier` is the framework's own seam for exactly that — *"the loader,
composer and the consumer's own custom reads"* — and both statements are strictly read-only,
with every identifier through `QuoteIdent`, every value through `EncodeArg` and the
placeholders from `Placeholder`.

**They count rather than stopping at the first match, and that is a bug fix rather than a
preference.** The first implementation was `SELECT 1 … LIMIT 1`, which was wrong in the
expensive direction: this project's `Querier` hands back the driver's own `Row`, so a SELECT
matching nothing fails the `Scan` with the DRIVER's no-rows sentinel (`pgx.ErrNoRows`), which
`isRecordNotFound` does not recognise — it looks for a framework `DomainError`. The panic would
then have fired on the ORDINARY case: a definition nobody holds a value for, which is precisely
the narrowing that must be ALLOWED. **Every legitimate narrowing would have answered 500**, and
no unit test would have caught it, because the rule's tests stub the service. An aggregate
always returns exactly one row, so `COUNT(*)` has no no-rows case to spell on any engine, any
remaining error is a genuine failure, and the statement stays plain ANSI with no `LIMIT`/`TOP`/
`FETCH FIRST` split.

**Named for the PROBLEM, and here that takes a moment's care.** The generated suite stubs the
service so every probe answers "nothing found": `ClaimIsHeldByAUser` reads **false** under the
stub, meaning "nobody holds it", so the narrowing is allowed and the generated happy path passes
on the day it is written. The healthy-state spelling (`ClaimIsFreeOfUserValues`) would read false
too — and would then mean "somebody holds it", turning a correct spec red. Same trap
`TenantIsUnavailable` documents on every entity in this service.

---

## §6 — API impact

**Not breaking, and the reasoning matters more than the verdict.**

No field changes name, type, nullability or position. No route moves. `AppliesTo` stays in the
patch body — nothing is added to `update.patchExcludes`, because the field genuinely stays
editable and hiding it would be a lie about a value that widens freely.

What changes is that a `PATCH /claims/:id` narrowing `appliesTo` against held values now answers
**422** where it previously answered **200**. That is a validation tightening on an existing
field, which is the shape that usually deserves a BREAKING flag — and here it does not, for a
reason specific to this change: **the writes it starts refusing are the ones that were silently
corrupting the chain.** There is no caller doing this on purpose today, because the collections
whose values get stranded do not exist yet. The rule ships in the same wave that creates the
thing it protects, so there is no window in which a client could have come to depend on the old
behaviour.

Stated rather than assumed, so the ordering constraint at the top of this document is understood
as part of the API argument and not just a build-order detail.

---

## §7 — Translations and tests

**One notification**, seven catalogs, real translations:
`ClaimAppliesToCannotExcludeHeldValuesNotification`, semantic `validation`.

> eng — The claim still has values set on principals of the identity kind being excluded.

No key is removed or renamed, so there is nothing to orphan.

**Tests** — `internal/domain/claim_rules_manual_test.go`, hand-written:

1. `both` → `user` with a client holding a value → refuses.
2. `both` → `user` with **no** client holding a value → passes. The case that keeps the field
   genuinely mutable, and the one a too-eager rule would break.
3. `both` → `client` with a user holding a value → refuses; with none → passes.
4. `user` → `client` with a user holding a value → refuses (the cross-narrowing, which a rule
   written as "did it lose `both`?" would miss).
5. `client` → `user` with a client holding a value → refuses.
6. `user` → `both` and `client` → `both` → pass, and **ask no fact at all** — asserted on the
   stub's call count, because a widening that queries two tables is a correctness bug that no
   assertion on the outcome would catch.
7. `AppliesTo` unchanged on an update that touches `DefaultValue` → passes, asks nothing.
8. An **archived** edge holding a value does not block the narrowing (the `activeOnly` half of
   §5).
9. Insert with any `AppliesTo` → the rule never fires.

**Coverage:** the project's 95% floor applies to what this run writes. No production code changed
to enable testability.

---

## §8 — Kind promotion

**N/A.** `Claim` stays `storage.kind: flat`.

---

## §9 — Generation path

**`Generation: omnicore-gen`**, inherited from the 2026-08-28 gate — not re-asked.

`Claim` is recorded in `specs/omnicore-gen/lock.json` and `doctor` runs clean, so the codegen
path is available. Every piece is in the spec language, checked against `explain rules` rather
than from memory: `rules.manual` with a description, notification and `attachTo`;
`service.facts[].kind: manual` with `returns` and `filters`. What lands **by hand on either
path**: the one rule body, the two fact bodies, their tests, and the two documentation files
(artifacts 11–12). No migration, on either path — there is nothing for one to do.
