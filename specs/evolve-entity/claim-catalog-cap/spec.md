# Evolve `Claim` — cap the catalog at 20 ACTIVE definitions per tenant, per identity kind

**Status: APPROVED** — 2026-08-28. All three open items were answered at the gate, each as
recommended: §3 asks only about the kinds a write ADDS · §6 uses the single declarative
`kind: count` fact · §10 answered **1**.

**Generation: omnicore-gen**

**Why this folder and not `specs/evolve-entity/claim/`.** That path is taken by an APPROVED
spec from 2026-08-28 (the `appliesTo` narrowing guard). Overwriting it would destroy the record
the dev is meant to be able to read the diff against, so this run gets its own folder.

---

## §1 — The change, in one paragraph

The catalog has **no cap on definitions per tenant**, and that is a hole `backlog.md` already
records as open — *"a tenant can still create more claims carrying defaults than a token can
hold. The loud answer — refusing that definition at the moment an operator creates it — is a
change to `Claim` and a run of its own. **Recorded here as open.**"* This run is that answer:
at most **20 ACTIVE claim definitions per tenant admitting a given identity kind**, refused at
the write with a typed 422 instead of silently truncated at token-emission time. A `both`
definition spends a slot on **both** sides, so the two buckets are the overlapping pair the
request names — `(user, both)` and `(client, both)` — not three disjoint counts of the enum's
three members.

**It closes the hole exactly, and that is arithmetic rather than a hope.** The emission's
candidate set is `ClaimDefinitionsOfTenant`, which is literally
`Eq(TenantID) AND In(AppliesTo, user, both)` under the ACTIVE scope
(`internal/infra/authentication_reader_manual.go:146`) — *the same predicate this cap counts*.
Bound that set at 20 and the walk in `resolveCustomClaims` can never have more than 20
candidates, so the `Warn`-and-drop truncation stops being reachable through the API at all. It
stays in place as the seatbelt for rows written by migration or direct SQL, which is what it
was always for. The client half of the same predicate is what `POST /auth/client/token` will
read when it exists; capping it now costs nothing and means that route arrives already bounded.

---

## §2 — Impact map

`G` = generator-owned (rewritten from the spec YAML, never hand-edited) · `M` = a `_manual`
hook (written once, mine forever) · `H` = hand-written on either path.

| Artifact | Own | What changes |
|---|---|---|
| `specs/omnicore-gen/claim.omnicore.yaml` | **H** | **this IS the change**: +1 `service.facts` entry, +1 `rules.manual` entry, +2 `notifications` |
| `internal/domain/claim_service.go` | G | +`ActiveClaimsWithAppliesTo(tenantID, appliesTo) int64` on the port |
| `internal/infra/claim_service.go` | G | +its implementation — a `CountEntities` over a criteria, active scope by default |
| `internal/domain/notifications.go` | G (registration site: appended, never rewritten wholesale) | +2 notification types |
| `internal/application/translations/{eng,ptbr,esp,fra,deu,ita,nld}.go` | G (registration sites) | +2 keys × 7 catalogs |
| `internal/domain/claim_rules_manual.go` | **M** | +the cap rule inside `customRules`, on `insertOrUpdate` |
| `internal/domain/claim.go` | G | rewritten; `BuildRules` is unchanged in substance — a `rules.manual` entry only moves the `customRules` call, which is already there |
| `internal/domain/claim_rules_manual_test.go` | **H** | +the cases in §8 |
| `internal/domain/claim_test.go`, `internal/application/{commands,queries,translations}/claim_*_test.go` | G | regenerated; the stub answers `0` to a count, so every existing happy path stays green |
| the rest of Claim's generated tree (commands, queries, web requests, routes, schema, view, feature) | G | rewritten byte-identical in substance — no field, no route, no column moved |
| `README.md` (status rows 49–50 and `### Claim`), `backlog.md` (the recorded-open item) | **H** | the hole named there closes; the docs say so |
| `ACCESS_MATRIX.md` | **H** | only if it enumerates Claim's refusals — checked at execution, edited only if it does |

**Not touched, each for a stated reason** — this list is the contract, and anything absent from
the table above is out of scope by construction:

- **No migration, in any dialect.** The cap adds no column, no constraint and no index. It is a
  rule over rows that already have a home, and the predicate it counts
  (`tenant_id`, plus `applies_to`) is served by the leading column of
  `claims_tenant_id_name_key` — the same index `ClaimDefinitionsOfTenant` already rides.
  No table or column DESCRIPTION changes either, so there is no `COMMENT ON` pair to write.
- **No view change and no `Version` bump.** The projected shape does not move, and this
  service's posture is `backing: relational` with no Mongo — the view has no `Version`,
  no registry row and no rebuild to miss (README, *Architecture posture*).
- **No OpenAPI break.** No field, no route, no verb. The new answer is a **422**, which is
  already the declared semantic of five of this entity's rules, so the error surface gains a
  message and not a status.
- **`microservice.*.yaml`, the proto contract, the GraphQL schema** — no surface moved,
  no route path moved, nothing starts publishing.
- **`specs/qa/*.sh`** — not generated for this service yet (README status row 57).
- **`internal/infra/claim_service_manual.go`** — untouched. The new fact is declarative, so it
  lands in the generated `claim_service.go` beside `ClaimNameTaken` rather than in the hook.

---

## §3 — What the cap gates, verb by verb

The bucket a write consumes is read off `AppliesTo` with the two helpers the narrowing guard
already exports — `ClaimAdmitsUsers` / `ClaimAdmitsClients`, `internal/domain/claim_rules_manual.go`.
That is deliberate reuse: "which kinds does this value admit" is asked in three places already,
and a fourth spelling of it is a comparison that can drift.

- **`insert`** — ask about **every** kind the new value admits. The row is not in the table
  yet, so the count is of the others and the test is `count + 1 > 20`, i.e. `count >= 20`.
- **`archive`** — nothing. Archiving frees a slot; a cap that fired on the way *down* would be
  a bug. (There is no `unarchive` on this entity, so there is no re-entry door to guard.)
- **`update`** — this is the open one.

**DECIDED at the gate — only the kinds the write ADDS.** The reasoning that carried it:

> Only the kinds the write ADDS (i.e. only a *widening* — `user`→`both`,
> `client`→`both`, and the two cross moves that swap one kind for another).
>
> Three things fall out of it, and all three are why I recommend it:
> 1. **It is the exact mirror of the narrowing guard**, which asks only about the kinds a
>    change *drops*. Two rules on the same field, one reading each direction, with the same
>    shape and the same helpers.
> 2. **A tenant already over budget stays repairable.** Nothing stops a tenant from holding 25
>    user-admitting rows today — seeded by migration, or written before this rule existed. The
>    strict reading would refuse *every* update to *every* one of those rows, including fixing
>    a typo in a `description`, until an operator archived five. The cap would land as a
>    lockout rather than as a budget.
> 3. **A write that consumes no new slot asks the database nothing** — a `description` edit
>    costs zero queries.
>
> **The alternative — refuse any write that leaves a bucket over 20** — is simpler to state
> ("the invariant holds after every write, always") and would force an over-budget tenant back
> under the line. It buys a stronger guarantee at the cost of item 2. Say the word and it
> becomes the rule instead; the code shape is the same, only narrower in what it skips.

Note what neither reading does: the cap counts **definitions**, not definitions that would
actually mint. A row with a null `defaultValue` and nobody holding a value for it still spends
its slot. Capping only the ones that mint today would make the budget move under the operator's
feet — a slot could vanish because somebody set a value on a user in another screen — and would
make `PATCH .../claims/{id}` setting a `defaultValue` a second place the cap has to fire.

---

## §4 — Is the "new field" a column at all?

`N/A — this change adds no field.` It adds a rule and the fact it asks. There is no value to
store, so there is nothing to migrate, backfill or keep in step. The read-join question does not
arise either: the traversal into `Tenant` this entity already declares fills `TenantWorkspace`
and `TenantStatus` on every load, and neither is involved here.

---

## §5 — View evolution

`N/A — the projected shape does not move.` No field is added to, removed from or re-sourced on
`claims`; no leg, no embedder, no `Fields()` allowlist anywhere in the service names anything
new. The archive regime is untouched — `Archive` was already in `Modes()` and the column is
already in the schema; this rule only *reads* that regime, by counting the ACTIVE rows.

---

## §6 — The fact, and the notifications

### The fact

**DECIDED at the gate — ONE declarative fact, generator-written:**

```yaml
- name: ActiveClaimsWithAppliesTo
  kind: count
  filters: [TenantID, AppliesTo]
  activeOnly: true
```

→ `ActiveClaimsWithAppliesTo(tenantID domain.ID, appliesTo vos.ClaimAppliesTo) int64`, and the
domain sums the bucket: `user + both`, `client + both`. **Verified, not assumed** — this exact
block was run through `omnicore-gen check` against this project before this spec was written:
`✓ this spec can be generated`, with the tree restored untouched afterwards.

Why the sum lives in the domain rather than in the query: `filters` compiles to **equality**,
so `applies_to IN ('user','both')` is not sayable in the DSL — and that is fine here, because
the bucket definition *already lives in the domain*, as `ClaimAdmitsUsers` / `ClaimAdmitsClients`.
Putting the arithmetic beside them keeps one reading of "what a `both` admits" instead of a
second one written in SQL.

Cost: an insert of a `both` definition asks three counts (`user`, `client`, `both`); a widening
asks two; a `description` edit asks none. Every one is a `COUNT(*)` on the leading column of an
existing index, on an operator path — the same argument the narrowing guard's probes already
make for themselves.

**The alternative that was weighed and dropped — two hand-written `kind: manual` facts:**

> `ActiveClaimsAdmittingUsers(tenantID) int64` / `...AdmittingClients(tenantID)`, each an
> `In("AppliesTo", …)` count in `claim_service_manual.go`. **One query per bucket instead of
> two or three**, and the fact body would then be the *literal same predicate* as
> `ClaimDefinitionsOfTenant` — which is a real coherence argument, not just a query count.
>
> The cost is hand-written infra where none is needed: two more bodies in the hook file, two
> more places to keep the ACTIVE scope right, and a concept ("bucket") that then exists in two
> layers instead of one. Close call; the declarative fact won it at the gate.

### The notifications — two, not one

`TooManyUserClaimsInTenantNotification` and `TooManyClientClaimsInTenantNotification`, both
`semantic: validation` (422 — matching `TooManyClaimsForUserNotification` and its siblings on
the two principals), both `tvars: [max]`, both `attachTo: AppliesTo`.

**Two rather than one**, because the write that most needs a clear answer is exactly the one a
single message cannot serve: an insert of `appliesTo: both` into a tenant with 20 user-admitting
rows and 3 client-admitting ones is refused, and "which side is full?" is the operator's next
question. `attachTo: AppliesTo` plus a single generic text would echo `both` back and leave that
unanswered. Collapsing them into one message with a `{kind}` tvar was considered and dropped:
the interpolated value would be a raw enum word riding untranslated inside seven translated
sentences.

`tvars: [max]` carries the number into the seven texts once, the way `DefaultValueTooLongNotification`
already does on this entity — the literal `20` is written in the spec YAML and nowhere else.

### Where the number 20 lives — decided, not open

Hard-coded in the spec, like every other cap in this service. It is not policy to be tuned per
deployment; it is arithmetic, and it is the *same* arithmetic already written down twice:
`user.omnicore.yaml`'s `claims-cap` and `client.omnicore.yaml`'s `claims-cap` are both 20,
*"a claim VALUE caps at 256 runes, so 20 entries is ~5 KB … that fits the 8 KB header buffer
nginx and most proxies default to"*, and the token budget in `resolveCustomClaims` is the same
20. A configurable cap would let a deployment set a number the header buffer cannot honour.

---

## §7 — API impact

Wire-compatible: **no field, no route, no schema change, no status code that was not already
reachable.** What changes is behaviour, and it is worth being honest about who feels it:

- `POST /claims` and `PATCH /claims/{id}` gain a 422 they could not answer before. Any client
  that already handles this entity's five existing 422s handles it.
- **A tenant sitting at 20 in a bucket can no longer create the 21st.** That is the point of the
  run, and it is a behaviour break for exactly the callers the hole was hurting — they were
  previously accepted and then silently truncated at sign-in.
- **A tenant already OVER 20** (seeded rows, direct SQL) is governed by §3's open item. Under
  the recommendation they keep working except for widenings; under the alternative they are
  frozen out of updates until they archive back under the line.

No consumer of the token sees any difference. The emission is untouched by this run.

---

## §8 — Tests

**Changed by design, and planned here rather than discovered later:** nothing. The generated
suite stubs the service so every fact answers its zero value, and `0` for an `int64` count means
"an empty bucket" — so every existing happy path passes unchanged, which is the whole reason the
name-for-the-problem convention exists.

**New, in `internal/domain/claim_rules_manual_test.go` (hand-written, `H`):**

1. Insert of a `user` definition, bucket at 19 → accepted (the boundary from below: 19 + 1 = 20).
2. Insert of a `user` definition, bucket at 20 → refused, `TooManyUserClaimsInTenantNotification`,
   attached to `AppliesTo`.
3. Insert of a `both` definition with the user bucket at 20 and the client bucket at 0 → refused
   on the **user** side. The mirror case refuses on the client side. This is the pair the
   two-notification decision exists for.
4. Insert of a `client` definition with the user bucket at 20 → **accepted**. The buckets are
   independent; a full user side must not block the client side.
5. Update `user` → `both` with the client bucket at 20 → refused. **The widening hole**: without
   this the cap is bypassed in two writes.
6. Update `both` → `user` (a narrowing) with both buckets full → asks **nothing** and passes the
   cap. Asserted on the stub's *call count*, not on the outcome — a narrowing that queries is a
   correctness bug no assertion about the answer would catch, the same way the narrowing guard's
   own widening case is asserted.
7. Update touching only `description` / `defaultValue`, buckets full → accepted, zero calls.
   This is the §3 recommendation's load-bearing case; under the alternative it becomes a refusal
   and this test inverts.
8. `both` counted on both sides: the fact stub returns `user=10, client=5, both=10` and the
   insert of a `user` row is refused (10 + 10 = 20). The arithmetic itself, isolated.

**Coverage:** CLAUDE.md rule 6 sets the floor at 95%. The new branch surface is the one rule
plus the generated count body; the eight cases above walk every branch of the former, and the
latter is generator-written and covered by the regenerated suite. Verified with
`go test ./... -cover` at the gate, not asserted here.

---

## §9 — Kind promotion

`N/A — Claim stays a flat aggregate.` No base table, no natural key, no role split.

---

## §10 — Generation path — the GATEWAY: answered **1**

`omnicore-gen doctor` was run before this spec was written and comes back **clean for every
entity** — no hand edits on owned files, no spec-changed-since-last-generation drift, no missing
specs. Claim **is** in the lock, so the codegen path is genuinely available, and nothing has to
be reconciled before it.

The change is **expressible** — verified by running `check`, not from memory (§6). What lands by
hand either way is the `rules.manual` body in `claim_rules_manual.go`, its tests, and the three
markdown docs.

> ⏸️ **How should this change be applied?** Two options, and only these two:
>
> **1. Change the spec and regenerate — `omnicore-gen` (beta) + review by me.**
> The change goes into `claim.omnicore.yaml`; `check` validates it and `generate` rewrites every
> file the generator owns — the port, the count implementation, the two notification types, the
> fourteen catalog entries and its own tests — in seconds and at a fraction of the tokens. There
> is **no migration to write on this one**, which is the usual hand-off and does not arise here:
> §2 establishes the change adds no DDL. I then write the rule in the hook file and its tests,
> update the three docs, review the emitted tree against this spec, run `prune`, and prove it
> with build + vet + tests + a real boot. If I find the generator wrong, **I stop and ask you
> before touching a single generated file.** Beta: the gate covers a lot but can still hit a case
> nobody has hit; when that happens I say so and it gets fixed upstream.
>
> **2. By hand, file by file, by me.**
> I edit every file myself, reading the pinned `/docs` before each layer. Slower, far more
> tokens, and on this entity it carries a permanent cost: every generator-owned file I touch —
> `claim_service.go`, `infra/claim_service.go`, `notifications.go`, the seven catalogs — stops
> tracking the spec. `doctor` reports each as a hand edit, the next regeneration refuses it until
> it is adopted or forced, and later emitter fixes never reach it. I would list what I touched
> and offer `adopt … -why`, which makes `doctor` tell the truth afterwards but does not undo the
> divergence.
>
> Reply **1** or **2**.

Neither is marked recommended while the generator is in beta.

---

## §11 — What this run does NOT do

- It does not touch `resolveCustomClaims` or the 20-per-token truncation. That code stays as the
  seatbelt for rows written by migration or direct SQL — the door the cap deliberately does not
  guard, being an API rule.
- It does not cap the **total** rows per tenant. Two buckets of 20 with no overlap is 40 rows,
  and that is correct: a `user`-only definition costs a client token nothing. If a flat
  per-tenant ceiling is also wanted, that is a different rule and a different run — say so and
  it joins this spec before approval rather than after.
- It does not touch the client token path, which does not exist. It only means that path arrives
  already bounded.

---

## §12 — Verification record (filled at execution, 2026-08-28)

Every promise above, walked with its evidence.

| Promise | Evidence |
|---|---|
| The spec YAML change validates | `omnicore-gen check` — `✓ this spec can be generated` |
| Generator-owned files regenerate | `generate` — `updated 12 · unchanged 24 · kept as-is 4 (yours, by design)`, no refusals |
| No hand edit drifted during the change | `omnicore-gen doctor` — clean for all seven entities, no `!` lines |
| Nothing orphaned, no dead translation key | `omnicore-gen prune` — *"Nothing to prune: everything this entity's spec ever produced is still something it produces"* |
| 2 keys × 7 catalogs | `grep -c` on each of `{eng,ptbr,esp,fra,deu,ita,nld}.go` → `2` on every one |
| No migration written or needed | `git status` — `migrations/` untouched |
| No view `Version` bump needed | relational backing, no Mongo; the projected shape did not move |
| `gofmt` · `go vet` · `go build` (incl. `-tags postgres`) | all clean |
| Unit tests, none weakened | `go test ./...` — every package `ok` |
| Coverage did not regress | `internal/domain` 78.6% → **79.0%**; the new `refuseCatalogBudgetExceeded` at **94.1%** (the uncovered statement is the `service == nil` early return, matching the neighbouring narrowing guard's own uncovered branch) |
| A real boot | built `-tags postgres`, `APP_PROFILE=dev`, logged to a file: `migrations applied` · `relational read models registered count=7` · `feature registered *main.ClaimsFeature` · `http listening :8080` · `/readyz` **200**. SIGTERM → `shutdown complete`, zero panics |
| The generated count really is ACTIVE-only | `aggregate.go:180` — *"scope gate (active rows by default)"*; the emitted body adds no scope override, matching `activeOnly: true` |

**One deviation from §2, in the harmless direction.** The impact map predicted
`internal/domain/claim.go` would be rewritten. It was reported **unchanged** — correct, because
both new rules are `rules.manual` and `BuildRules` already carried the `customRules` call. No
generated file changed that the map did not list.

**Two observations OUTSIDE this run's impact map, recorded and NOT acted on:**

1. **`claim_rules_manual.go`'s header comment on gate order is wrong at v0.63.0.** It claims
   *"the framework dispatches gates in a fixed order — every `IfInsert` clause runs before every
   `IfInsertOrUpdate` one, whatever the declaration order"*. It does not: `BuildRules` is invoked
   **once** (`domain/entity_base.go:696`) and each `IfX` calls its closure inline
   (`domain/rules.go:209-256`), so this file's `IfInsert` block runs AFTER the generated
   `IfInsertOrUpdate` barrier, not before it. The consequence is only that
   `TenantIsUnavailable`'s fail-closed UUID guard is defensive rather than load-bearing — no
   behaviour is wrong. Left alone: correcting a comment on a rule this run did not touch is
   outside the approved map.
2. **`ACCESS_MATRIX.md`'s "Exceed a cap" row is incomplete.** It lists 50 roles, 50 groups and 20
   CIDRs but not the 20 claim VALUES per user and per client, which have existed since
   2026-08-28. This run added its own line beside it and did not backfill the missing ones.

---

## Addendum — 2026-08-29: §6's fact superseded by a GROUPED count

**The record above stands as written; this section says what later replaced part of it.**

§6 chose one `kind: count` fact filtered `[TenantID, AppliesTo]`, asked once per enum member,
and justified the arithmetic living in the domain with this sentence: *"`filters` compiles to
**equality**, so `applies_to IN ('user','both')` is not sayable in the DSL"*. That was true of
the generator build this run used. **It is no longer true.** The build shipping with plugin
0.50.0 (framework v0.63.0) accepts the full criteria vocabulary in `filters` — `in`, `nin`,
`isnull`, `notnull`, the text operators and nested `any`/`all`/`not` — plus `groupBy:` and
multi-answer `aggregates:`. The limitation the paragraph rests on is gone, so the paragraph
is wrong to read today and the cost line under it (*"an insert of a `both` definition asks
three counts"*) no longer describes the code.

**What replaced it, on 2026-08-29:**

```yaml
- name: ActiveClaimsByAppliesTo
  kind: count
  groupBy: [AppliesTo]
  filters: [TenantID]
  activeOnly: true
```

→ `ActiveClaimsByAppliesTo(tenantID domain.ID) []ClaimActiveClaimsByAppliesToGroup`, one
`COUNT(*) … GROUP BY applies_to` through the framework's `AggregateBy`, and
`refuseCatalogBudgetExceeded` folds the groups into the two overlapping buckets through
`ClaimAdmitsUsers` / `ClaimAdmitsClients`. **One query for every write that consumes a slot**
(three before, on an insert of `both`; two on a widening) and still **zero** for a write that
consumes none — the early return is untouched and its tests are untouched with it.

The alternative weighed and dropped a second time: two facts filtered
`{field: AppliesTo, op: in, values: [User, Both]}` and `[Client, Both]`. Sayable now, two
queries rather than one, and it would put the definition of a bucket in the YAML beside the
one the domain already exports — the very duplication §6 refused. The grouped shape is
cheaper AND keeps that single reading, so §6's conclusion survives its own premise.

Nothing else from §2's impact map moved: no migration, no notification, no translation key, no
route, no column, no view version. The three generated artifacts (`claim_service.go` on both
sides, `claim_test.go`) were rewritten by the generator; `claim_rules_manual.go` and
`claim_rules_manual_test.go` were hand-edited, the latter now pinning **one** round trip on an
insert of `both` where it used to pin three.
