# QA plan — `claim-contract`

- **Status:** APPROVED — maintainer (Cláudio Schirmer Guedes), 2026-09-08, at the plan gate ("sim"), including the W13 three-form disposition, the added CL7.11 row, and the six §0c prose corrections listed in §5.
- **Suite slug:** `claim-contract` — what this round proves: the whole wire contract of the
  `Claim` aggregate on both surfaces, the business rules its approved spec and the
  maintainer state, and the **four** things that make this entity unlike the six already
  proven:
  1. **the only entity whose value object owns a RESERVED PREFIX, caller-side** — `x_` is
     typed by the caller, stored verbatim, never prepended and never stripped; the double
     prefix is closed by construction. Nothing else in the service refuses a name for the
     shape of its first two runes;
  2. **the only entity with a rule that reads ANOTHER field to validate a value** —
     `DefaultValue` must parse as whatever `ValueType` declares, which is why it is a plain
     column and not a value object;
  3. **the only entity whose write guard is a QUESTION ASKED OF TWO OTHER AGGREGATES** —
     `applies-to-narrowing-refused` asks `user_claims` and `client_claims` whether an
     ACTIVE edge still holds a value for a kind the change would drop, and it must ask only
     about the kinds actually dropped;
  4. **the only entity with a TWO-BUCKET cap over one grouped count** — 20 definitions per
     tenant that admit a user and 20 that admit a client, `both` counting in both, the two
     buckets independent, and the guard firing only when a write ADDS a kind.
- **Written:** 2026-09-08
- **Pin:** omnicore **`v0.74.0`** · dialect postgres · read backing **relational**
  (read-your-writes — every read-back below is IMMEDIATE; a needed poll would itself be a
  failure). This round contains no temporal contract and no legitimate wait.
- **Surfaces in scope:** REST + GraphQL (`surfaces.graphql.enabled: true`) — full parity,
  **5 operations each** (`POST /claims` · `PATCH /claims/{id}` ·
  `PATCH /claims/{id}/archive` · `GET /claims` · `GET /claims/{id}`, and their GraphQL
  twins `createClaim` · `patchClaim` · `archiveClaim` · `claims` · `claim`).
- **Plan destination:** `specs/qa/claim-contract/plan.md` · **suite destination:** `qa/` at
  the project root · **verdict destination:** `qa/qa-report.md`
- **Prior rounds:** `tenant-contract` (APPROVED 2026-09-06), `permission-contract`,
  `role-contract` (2026-09-07), `group-contract`, `user-contract` (2026-09-07),
  `client-contract` (APPROVED 2026-09-08). This round **EXTENDS the same `qa/run.sh`** — no
  second entry point — and reopens none of the six approved plans. **It is the seventh and
  last entity round: with it, every aggregate this service ships has a contract suite.**

## 0. Where every expectation below comes from

Nothing here was read off a running service. The service was never called while this plan
was written. The one exception is an ENUMERATION and never a value that becomes an
expectation: the suite reads `GET /openapi.json` at boot to cross-check the verb inventory
(`W12`).

| Source | What it settled |
|---|---|
| `specs/scaffold-entity/claim/spec.md` (APPROVED 2026-08-28; superseded note 2026-09-06) | the model; §0 the two dependencies deliberately not inherited; §2 the fields and the `x_` decision in both halves (reserved prefix, caller-owned); §5 modes; §7 the R1–R8 rule table; §9 reads, the join and the filter vocabulary; §10 the three authorization layers |
| `specs/omnicore-gen/claim.omnicore.yaml` | fields, `assignedFrom: identity-claim` + `bypassMaySet` on `TenantID`, the three value objects, **thirteen** notifications, the **three-field** read join, modes, `update.shape: patch` + `patchExcludes`, the declarative and the five MANUAL rules, `service.facts`, `read.byParams`, `authz` |
| `specs/evolve-entity/user/spec.md` · `specs/evolve-entity/client/spec.md` | the two edge collections `user_claims` and `client_claims` — the aggregates §1b's narrowing rows ask their question of |
| `internal/domain/claim.go` · `claim_rules_manual.go` | `Modes()` = display/insert/update/archive (no unarchive, no delete); the gate ORDER note (`IfInsert` runs before the generated `tenant-is-a-usable-id` barrier, so `TenantIsUnavailable` fails closed on an unusable id); `claimsPerTenantCap = 20`; `ClaimValueMatchesValueType` — including its explicit `NaN`/`Inf` refusal on `number`; `ClaimAdmitsUsers`/`ClaimAdmitsClients` |
| `internal/domain/vos/claim_name.go` · `claim_value_type.go` · `claim_applies_to.go` · `text_predicates.go` | the EXACT `ClaimName` contract: total **4**–64 runes, `x_` prefix required, remainder ≥2 runes matching `^[a-z0-9]+(_[a-z0-9]+)*$`, remainder must not itself begin with `x_`, ≥1 letter, ≥2 distinct runes, no run of 4+ identical runes — and no normalization anywhere; the two closed enum sets |
| `internal/web/claim_routes.go` | the **5** REST endpoints and **5** GraphQL fields, and the permission literal on each |
| `internal/web/requests/find_claims_by_params.go` · `find_claim_by_id.go` · `insert_claim.go` · `patch_claim.go` | the DTO opt-in gate: the listing's filters/sorts/controls (**no `search`**); the by-id serving `includeArchived` alone; **`tenantArchivedAt` served and projectable but in NO filter and NO sort**; the INSERT body carrying `tenantID` as OPTIONAL; the PATCH body carrying **only** `appliesTo`, `defaultValue`, `description` |
| `internal/application/queries/find_claims_by_params_query.go` · `find_claim_by_id_query.go` `ToCriteria` | the FORCED tenant filter unless `IsSuperAdmin()` — Layer 3, and why a foreign row answers 404 |
| `migrations/postgres/0009_*claim*` · `0010_user_claims_manual.up.sql` · `0011_client_claims_manual.up.sql` · `0012_bootstrap_seed_manual.up.sql` | the partial unique index `(tenant_id, name) WHERE archived_at IS NULL`; the two edge tables, their FK into `claims` and the reverse index `ClaimIsHeldByAUser` reads; the four `claim:*` catalog rows `…0001`–`…0004` |
| `qa/microservice.qa.yaml` · `qa/run.sh` · `qa/lib/common.sh` | the posture (`auth.mode: jwt`, `authorization.enabled: true`, `tenant.required: true`, five `publicRoutes`); the helper API every lane speaks — including the `claim_name`/`new_claim` fixtures the user and client rounds already built |
| pin docs — `status-mapping`, `auto-handlers`, `auto-query-handlers`, `relational-view`, `read-joins`, `auth-middleware`, `authz-seams`, `graphql`, `rules-dsl`, `audit` | every status code, notification key and envelope shape asserted below |
| **the maintainer, asked 2026-09-08** (`AskUserQuestion`, two rounds, all eight answers on record) | the unreachable-immutability disposition; the narrowing at **six transitions × held/not-held**; the caps as **bucket independence + only-when-it-ADDS**; the four `ClaimName` negative families; the default×type **full matrix incl. the PATCH path**; tenant-must-exist **three refusals + trial + the insert-only positive**; the null-in-PATCH contract; the archive/rebirth cycle **including the edge that does not re-attach** |

### 0a. What this round INHERITS and does not repeat

Proven service-wide by the six approved rounds, against this exact posture: the whole
**401 family** (token shape, signature, `iss`, `aud`, `alg`, `exp`, expired-vs-invalid);
the **framework's appended public surfaces** and the **GraphQL introspection bypass**;
**both directions** of the public-route split and its exactness neighbours; the middleware
**tenant-claim gate**; the **boot, hygiene and report contracts**; and the generic
**relational read contract** (cursor walks, the only-total conflict matrix, the page
ceiling, the malformed-cursor family) — asserted here only where `Claim`'s own vocabulary
differs (`W7`, `W8`).

One row is inherited by NAME because another round already spends it: **`U15.2`** (user
round) provokes `TooManyUserClaimsInTenantNotification` incidentally, while proving the
user-side edge cap is unreachable. This round does not repeat that row; `CL8` proves what
`U15.2` cannot see — the CLIENT half, the independence of the two buckets, and the
only-when-it-ADDS firing condition.

### 0b. What is DELIBERATELY not in this round

- **Token emission is untouched, by the model's own decision** (`spec.md` §0, dependency 2).
  `buildClaims` does not read this catalog, so **no case here asserts anything about a
  minted token's contents.** The catalog can be filled, read and audited with no observable
  change to any token, and that is the property the suite respects rather than tests.
- **The two edge collections are exercised as FIXTURES, not owned.** `CL7` writes
  `user_claims` and `client_claims` entries because the narrowing rule is observable
  nowhere else; their own contract belongs to the user and client rounds, which already
  hold it (`U8`, `Q5.5`).
- **The platform's nine un-prefixed claims** (`identity_kind`, `tenant_id`, … ) are
  un-writable through this API by construction. `CL1a` asserts exactly that refusal for one
  representative bare name; the nine are not enumerated, because the rule is about the
  string's shape and not about a list.

### 0c. Stale prose in `specs/scaffold-entity/claim/spec.md` — six items, and the proposed disposition

All six are verifiable in the code, all six contradict or under-describe something this
suite asserts, and per the standing rule the fix is **in the same round, at the source,
with a dated supersession note** — the disposition the user and client rounds' §0c both
received. Approval of this plan approves these edits; they are listed in §5. **No
application code, no yaml and no migration is touched.**

| # | What `spec.md` says | What the code does | Where this suite pins it |
|---|---|---|---|
| a | §7's rule table stops at **R8**, and the notification list names **ten** keys, and the domain service names **two** facts | three further MANUAL rules exist and are wired (`applies-to-narrowing-refused`, `claims-per-tenant-cap-users`, `claims-per-tenant-cap-clients`), with **three** further notifications (`ClaimAppliesToCannotExcludeHeldValues…`, `TooManyUserClaimsInTenant…`, `TooManyClientClaimsInTenant…`) and **three** further facts (`ClaimIsHeldByAUser`, `ClaimIsHeldByAClient`, `ActiveClaimsByAppliesTo`) | `CL7`, `CL8` |
| b | §7: "`AppliesTo` stays MUTABLE … when they arrive the guard belongs beside them, **in the run that builds them**" and "there are no edges to strand yet" | the edges arrived (migrations 0010/0011, 2026-08-28) and the guard was built **here, on `Claim`**, not beside them — it asks the two edge tables through two facts of this aggregate's own service | `CL7` |
| c | §8: "`patchExcludes: [TenantID, Name, ValueType]`" | the yaml declares `patchExcludes: [Name, ValueType]` — `TenantID` was dropped by the 2026-08-28 amendment, which the banner records but §8's body still contradicts | `W13` |
| d | §9: the read join brings "`TenantWorkspace` (`workspace`) and `TenantStatus` (`status`)" | the yaml declares **three** joined fields; `TenantArchivedAt` is served and projectable too — and, unlike the other two, it is in **no** filter and **no** sort | `W2`, `W7.9`, `W8.11` |
| e | §2: `ClaimName` is "lowercase `[a-z][a-z0-9_]*`, **2..64 runes**" | the floor is **4** runes total (the two-rune prefix plus a remainder of ≥2), the remainder pattern is `^[a-z0-9]+(_[a-z0-9]+)*$`, and three anti-junk predicates apply (≥1 letter, ≥2 distinct runes, no run of 4+ identical) | `CL1` |
| f | §0: the two edge collections "are not this step"; "the seed of the platform's nine is NOT part of this run" | accurate when written, and the SECOND half still holds — but the edges exist now. A dated supersession note, not a rewrite: the scope decision stands as the record of what that run did | `CL7`, `CL10.4` |

### 0d. Corrections the first execution forced — in the SUITE, never in an expectation

Recorded because the difference matters. **No expectation below was weakened, and no case was
edited to make a run green.** Three cases were built wrong by this skill and were rebuilt
against the sources they should have come from in the first place; one plan sentence
contradicted the pin's own documentation and was corrected against that documentation, not
against anything the service answered.

| What was wrong | Where the right answer came from |
|---|---|
| `W7.7a` expected a `startCursor` on the head of a forward walk | `auto-query-handlers` at the pin states the biconditional explicitly — *"the first page of a forward walk carries no `startCursor`"* — and four earlier approved rounds pin it. The plan sentence was a misreading of the contract; the CONTRACT was never in doubt |
| `W2.4` addressed the listing with `?id.eq=`, borrowed from the client lane | the DTOs: `find_clients_by_params.go` and `find_users_by_params.go` declare an `ID` filter; `find_claims_by_params.go` does not, and `spec.md` §9 agrees. Rebuilt on `?name.eq=`, and the asymmetry itself became `W8.1b` |
| `W7.9` asserted the `tenantArchivedAt` KEY on a live owner | the response DTO: every listing field is `omitempty`, so a null value is absent by construction. Projectability is now proven where the stamp EXISTS, and the observation grew `W7.9b` and `W7.9c` |
| `CL10.4c` aimed its write at a principal that ALREADY held an active entry for that definition, so the duplicate index answered before the availability probe | the user round's own approved row `U10.3-`: a write against an archived definition is **422 `ClaimNotAvailableInTenantNotification`**. Rebuilt on a second, clean principal — the expectation came from an APPROVED plan, never from the 409 the mis-built case happened to see |
| `S9.3a2`/`S9.3b2` asserted isolation against a listing that was empty, because `S9.1j` had archived the only definition in M's tenant | the assertion's own reasoning: *"never an empty page, which is how a merge would masquerade as a scope"*. The fixture was missing, not the expectation — `S9.3a1` was added to prove the page is non-empty BEFORE the isolation rows read it |

---

## 1. Coverage matrix — the FRAMEWORK's promises

Entity **Claim** × surfaces **REST** (`qa/claim.sh`, family **W**) and **GraphQL**
(`qa/claim_graphql.sh`, family **X**). Read backing relational: every write→read-back is
immediate. Archive regime *kept-but-hidden*. No children, no siblings, one read join of
three fields.

Envelope asserted on REST: `errors[].messages[].notificationKey` + the HTTP status. On
GraphQL: HTTP always 200, the same key on `errors[].extensions.notificationKey`. Every
request pins `Accept-Language: en-US`.

The lane runs as the bootstrap admin (`*:*`); §1b and §3 are where the scoped principals do
the work.

### W1 — Happy path, one per served verb (5 routes)

| REST | GraphQL twin | Expected |
|---|---|---|
| `POST /claims` | `createClaim(input:)` | **201** · the record AS STORED |
| `PATCH /claims/{id}` | `patchClaim(id:, input:)` | **200** |
| `PATCH /claims/{id}/archive` | `archiveClaim(id:)` | **204, NO BODY** / `{success}` |
| `GET /claims` | `claims(...)` | **200** · `data` + `pagination` / the connection |
| `GET /claims/{id}` | `claim(id:)` | **200** · the full document |

### W2 — Golden record round-trip

Addressed on the listing side by `?name.eq=`, not by `?id.eq=`: **`id` is not a filter this
entity declares.** `Client` and `User` each declare one on their own listing DTO; `Claim`
declares none, and its `spec.md` §9 filter table agrees. A per-entity decision rather than a
framework universal — `W8.1b` pins it as a typed 400 so the asymmetry is asserted instead of
assumed.

One definition exercising EVERY declared field, written then read back field-by-field on
BOTH surfaces: `id` · `tenantID` · `name` · `valueType` · `appliesTo` · `defaultValue` ·
`description` · `createdAt` · `updatedAt` · `archivedAt` · and the **three** joined fields
`tenantWorkspace` · `tenantStatus` · `tenantArchivedAt`. Thirteen wire names; no composite
value object exists on this entity, so every one is a field of its own. This is the family
that catches a column silently dropped from a projection, and item **0c-d** is why it is
worth running: the third joined field is in no earlier spec sentence.

### W3 — The write/read asymmetry

`W3.1` the INSERT and PATCH responses carry the seven stored columns plus `id` and
**nothing traversed** — no `tenantWorkspace`, no `tenantStatus`, no `tenantArchivedAt` —
and **no managed stamp** (`createdAt`/`updatedAt`/`archivedAt` are absent from both
response DTOs). `W3.2` a READ carries all thirteen. The join is a READ contract, not a
write echo.

### W4 — Absent verbs, split three ways

- **`unarchive`** — not in `Modes()` and **no route mounted**: `PATCH /claims/{id}/unarchive`
  → **404** (no route matches the path). Not a 403: the 403 shape needs a mounted route
  whose mode is missing, and this service mounts none.
- **`DELETE /claims/{id}`** → **405** — the path is registered under `GET` and `PATCH` only.
  Verb truth: nothing soft rides behind `DELETE`.
- **`PUT /claims/{id}`** → **405**, same derivation (`update.shape: patch`).
- **The mode-missing-but-mounted 403 is `N/A` on this entity**, stated rather than skipped:
  `Modes()` is exactly the four verbs mounted, so no route can reach a mode the aggregate
  refuses.

### W5 — Not found

`GET /claims/{a-well-formed-uuid-nobody-owns}` → **404**, and its GraphQL twin → `null`
node with the canonical not-found in `errors[]`.

### W6 — A by-id address that is not a uuid, split by VERB (pin ≥ v0.70.0)

| Request | Expected |
|---|---|
| `GET /claims/not-a-uuid` | **404** `UnknownIDAddressNotification` |
| `PATCH /claims/not-a-uuid` | **400** `MalformedIDNotification` |
| `PATCH /claims/not-a-uuid/archive` | **400** `MalformedIDNotification` |

Both verbs, not just the read — that is the half an older suite misses. Identical on
GraphQL.

### W7 — Read vocabulary (the DTO's declared surface)

`W7.1` one filter per declared operator family, per field: `TenantID` eq/in · `Name`
eq/ne/in/startswith/istartswith/contains/icontains · `ValueType` eq/in · `AppliesTo` eq/in ·
`DefaultValue` eq/in/contains/icontains · `Description` contains/icontains ·
`TenantWorkspace` eq/in/startswith/istartswith · `TenantStatus` eq/in · `CreatedAt` and
`UpdatedAt` gte/lte.
`W7.2` `?orderBy=` on each of the seven declared sorts, both directions.
`W7.3` `?fields=` — a projection returning exactly the asked-for names and nothing else.
`W7.4` `?onlyTotal=true` — the count alone.
`W7.5` `?last=` alone — the TAIL window.
`W7.6` `?includeArchived=true` — with no unarchive verb, the listing is the ONLY way to see
a retired definition (`spec.md` §9 says so in as many words).
`W7.7` the PAGINATION ENVELOPE as a contract: `totalCount`/`hasNextPage`/`hasPreviousPage`
truthfulness, **the BICONDITIONAL** — an edge cursor is emitted only where its neighbouring
page exists, so `endCursor` exactly when `hasNextPage` and `startCursor` exactly when
`hasPreviousPage`, and the head of a forward walk therefore carries **no** `startCursor`
(`auto-query-handlers` at the pin says so in as many words, and the tenant, permission, role
and group rounds each pin it already) — page-2 disjointness, and a cursor WALK echoing
`endCursor` into `?after=` and `startCursor` into `?before=`.
`W7.8` the JOIN reaches the counterpart's value: filtering `?tenantWorkspace=` returns the
tenant's definitions in ONE call — the reach that distinguishes this backing.
`W7.9` `?fields=tenantArchivedAt` → **200** carrying the owning tenant's archive stamp:
served and projectable (item 0c-d), even though `W8.11` proves it is in no filter and no
sort. Asserted against a definition whose tenant was retired AFTER it was created — every
listing field is `omitempty`, so on a live owner the key is absent because the VALUE is null,
which would prove nothing either way. `W7.9c` adds the half that observation exposes: an
archived owner does not hide its definitions, because the join is inner on the foreign key
and not on the counterpart's archive state.

### W8 — Rejected reads: the WHOLE typed-400 guard family

| # | Request | Expected |
|---|---|---|
| W8.1 | `?bogus=1` — unknown filter field | **400** `SchemaViolationNotification` |
| W8.1b | `?id.eq=` — the filter this entity does not declare, though two of its siblings do | **400** `SchemaViolationNotification` |
| W8.2 | an operator outside a field's allowlist (`?description[eq]=`) | **400** |
| W8.3 | `?search=billing` — a RESERVED control the DTO never declared | **400** `SchemaViolationNotification` (the opt-in gate; and this posture could not serve it anyway) |
| W8.4 | `?onlyTotal=false` — the same undeclared-control gate on a control this DTO DOES declare, sent with a value that changes nothing: **PRESENCE is what trips a gate**, so this one is **200** and proves the gate is about declaration, not about truthiness | **200** |
| W8.5 | `?fields=bogus` — an unresolvable path | **400** `SchemaViolationNotification` naming `fields[bogus]` |
| W8.6 | `?tenantID=lixo` — a filter VALUE outside an identity column's kind | **400** `InvalidFilterValueNotification` (pin ≥ v0.70.0) |
| W8.7 | `?createdAt=abc` — the same, on a timestamp leaf | **400** `InvalidFilterValueNotification` |
| W8.8 | `?first=` above the view's ceiling | **400** |
| W8.9 | mixed directions: `first`+`last`, `first`+`before`, `after`+`before` | **400** each |
| W8.10 | `?onlyTotal=true` beside a page-shaping control — and its complement: beside a FILTER or `?includeArchived`, which stays **200** (counting a filtered subset is the point) | **400** / **200** |
| W8.11 | filter on `?tenantArchivedAt=` and `?orderBy=tenantArchivedAt` | **400** each — served, projectable, and in NO read vocabulary |
| W8.12 | `?orderBy=` on a field that is filterable but not sortable (`defaultValue`, `description`, `tenantStatus`) | **400** each |
| W8.13 | malformed `?after=` / `?before=` | **400** |
| W8.14 | cursor ↔ `orderBy` mismatch · cursor ↔ `includeArchived` mismatch | **400** each |

Surface idiom differs BY DESIGN: on GraphQL an unknown argument is a validation error, and
`fields`/`onlyTotal` are selection-natural and never gated — `X8` asserts the GraphQL
rendering, never the REST envelope cross-surface.

### W9 — Validation 422, one per shape

`InvalidClaimNameNotification` (one representative; the four negative families are §1b's
`CL1`) · `UnknownClaimValueTypeNotification` · `UnknownClaimAppliesToNotification` ·
`DefaultValueTooLongNotification` (257 runes; the `{max}` tvar renders **256**) · the
`Description` value object's own refusal · `RequiredFieldNotification` on an omitted
`name`. **No `required` rule is declared on the four VO-backed fields, deliberately**
(`spec.md` §7) — so the assertion is that the caller reads the complaint ONCE, not twice.

### W10 — The dual 409

**Duplicate flavor:** `ClaimNameAlreadyExistsNotification` — the same name twice in one
tenant, **409**. Its full contract (per-tenant, active-only, exclude-self) is §1b's `CL5`.

**The wrong-state 409 is `N/A` on this entity, by the same derivation the tenant,
permission, role and group rounds each recorded.** `Claim` has no state machine, no
transition rule, and no wire field carrying a revision a caller could send stale; a write
against an archived row is intercepted one layer earlier by the LOAD scope, where it lands
as **404**. `W10.3` asserts that 404 rather than asserting a 409 that cannot exist.

### W11 — Archive round-trip

`archive` → the row is **hidden** from the default listing and from `GET /claims/{id}` →
`?includeArchived=true` **reveals** it, with `archivedAt` set → **no unarchive** (`W4`) →
the NAME is freed and re-insertable (`CL5.3`). **No child carries an archive column** (this
aggregate has no collection), so the stamp-scoped unarchive family is `N/A` — stated, not
skipped.

### W12 — `/openapi.json` cross-check

The route inventory the framework auto-registers must enumerate exactly **5** claim routes,
each carrying its permission literal (`claim:insert` · `claim:update` · `claim:archive` ·
`claim:read` twice). An enumeration, never an expectation: a source-vs-openapi disagreement
is reported as a FINDING, not resolved silently.

### W13 — The immutable fields have NO DOOR, and that is what is asserted

*(maintainer, 2026-09-08 — "porta fechada + regra registrada UNPROVEN")*

`patchExcludes: [Name, ValueType]` removes two fields from the PATCH body, and
`assignedFrom: identity-claim` keeps `TenantID` out of every update body — so **the three
immutability rules R2/R3/R4 have no path through any mounted surface.** What the wire
actually promises, and what this suite pins:

- `W13.1` **REST** — `PATCH /claims/{id}` carrying `name`, `valueType` and `tenantID` in
  the body → **200**, and the three values **UNCHANGED** on the read-back. An unknown JSON
  key is ignored, so the caller's attempt is a no-op rather than an error.
- `W13.2` **GraphQL, the literal** — the same three keys written into the `patchClaim`
  input **in the document** → a **validation error** (unknown input field). Only the
  literal is validated.
- `W13.3` **GraphQL, the variable** — the same three keys passed through a VARIABLE →
  **200 and unchanged**: variable coercion drops an unknown field in silence. The two
  GraphQL halves answer differently BY DESIGN, and a suite asserting only one would read
  the other as a regression.
- **Recorded as UNPROVEN, not claimed:** `ClaimNameIsImmutableNotification`,
  `ClaimTenantIsImmutableNotification` and `ClaimValueTypeIsImmutableNotification` are
  **unreachable by construction through every mounted surface**. They are backstops behind
  a door closed one layer earlier — the same shape `P3e`, `RL7-` and `U15.2b` record for
  their own unreachable rules — and they print as SKIPPED with this reason, never folded
  into GREEN.

### X — GraphQL parity (family X)

Every W row above has an X twin unless noted. What X adds that no REST row can see:

`X1` the five fields resolve, `archiveClaim` answering `{success}` where REST answers 204.
`X2` handler invariance: `claim(id:)` and `GET /claims/{id}` return the same thirteen
values for the same record.
`X3` `__typename` beside every selected field changes NOTHING — same values, same
projection (pin ≥ v0.72.1; every mainstream client appends it, and a boundary that holds
only for documents no real client sends is not a boundary).
`X4` selection-natural controls: a narrowed selection set is the GraphQL `?fields=`, and it
is **never** gated the way `W8.5` gates the REST control.
`X8` the rejected-read family in GraphQL idiom: unknown argument → validation error;
`W6`'s by-id verb split → the same two keys.

---

## 1b. Domain expectations — what the BUSINESS requires

`qa/domain.sh`, family **CL**. Every row below carries its source; nothing here was read
off a running service. Ranked by the cost the maintainer named — the prefix decision and
the two guards that reach into other aggregates first.

### CL1 — The reserved prefix `x_`, caller-owned *(spec.md §2, both model-gate answers; maintainer 2026-09-08, all four negative families)*

**The rule:** a claim name is the exact string the token will carry. It must begin with
`x_`, typed by the caller. **Nothing is prepended and nothing is stripped**, and a
non-compliant value is REFUSED, never repaired — because the name is immutable and a caller
who believes they registered one string has no second chance.

| Case | Input | Expected |
|---|---|---|
| CL1+ | `x_cost_center` | **201** · stored VERBATIM, prefix included |
| CL1+b | `x_ab` — exactly the 4-rune floor | **201** · the floor ADMITS; it is not an off-by-one refusal |
| CL1a− | `cost_center` — no prefix | **422** `InvalidClaimNameNotification` — the whole caller-owned decision rests on this row |
| CL1b− | `x_x_cost_center` — the double prefix | **422** — the paste error a caller-owned prefix makes possible, closed in the VO |
| CL1c− | `x_Cost_Center` · `x_cost-center` | **422** each — NOTHING IS NORMALIZED |
| CL1d− | `x_a` (3 runes) · `x_aa` (1 distinct rune) · `x_aaaa` (a run of 4 identical) | **422** each — the shared anti-junk floor |
| CL2 | after CL1c−'s refusal, count rows under ANY casing of that name | **0** — nothing was quietly written or repaired |

**Pass** = the prefix contract is the wire contract. **Fail** = either a name entered the
catalog that a token could not carry, or the server silently rewrote a value a caller can
never correct.

### CL3 — `DefaultValue` must parse as `ValueType` *(rules.manual `default-value-matches-value-type`; maintainer: full matrix incl. the PATCH path)*

**The rule, in the spec's words:** *"the default must parse as the declared type: `number` →
a valid decimal number; `bool` → exactly `true` or `false`; `string` → any non-empty value.
A null default is always valid and skips the check."* This is the rule that reads ANOTHER
field, and the reason `DefaultValue` is a plain column.

| # | `valueType` | `defaultValue` | Expected |
|---|---|---|---|
| CL3.1 | `number` | `1000` · `-2.5` | **201** each |
| CL3.2 | `number` | `abc` | **422** `DefaultValueDoesNotMatchValueTypeNotification` |
| CL3.3 | `number` | `NaN` · `Inf` | **422** each — `ParseFloat` accepts both and the rule refuses them explicitly |
| CL3.4 | `bool` | `true` · `false` | **201** each |
| CL3.5 | `bool` | `TRUE` · `1` | **422** each — the comparison is neither case-insensitive nor numeric |
| CL3.6 | `string` | any non-empty | **201** |
| CL3.7 | any | **null** (key absent) | **201** — "no default" is a legitimate state; level 2 simply does not fire |
| CL3.8 | `number` (stored) | `PATCH {defaultValue:"abc"}` | **422** — the rule is `insertOrUpdate` and revalidates against the type ALREADY STORED |
| CL3.9 | `number` (stored) | `PATCH {defaultValue:"42"}` | **200** |

### CL4 — The claim-size budget *(rules.list `default-value-length`, `skipWhen: null`)*

`CL4.1` 256 runes → **201**. `CL4.2` 257 → **422** `DefaultValueTooLongNotification`, the
message rendering **256** through its `{max}` tvar. `CL4.3` a null default → **201**: the
rule is nil-safe and stands down rather than reading through a nil pointer.

### CL5 — Uniqueness: per tenant, ACTIVE-only, exclude-self *(spec.md §2; `unique.scope: active-only, within: [TenantID]`)*

`CL5.1` the same name twice in one tenant → **409** `ClaimNameAlreadyExistsNotification`.
`CL5.2` the same name in ANOTHER tenant → **201** — a token carries exactly one tenant, so
two customers naming a claim `x_region` is harmless.
`CL5.3` archive, then re-insert the SAME name → **201** — active-only is what makes a
retired definition's name reusable, which matters precisely because no unarchive is mounted.
`CL5.4` a PATCH that leaves `name` alone → **200**, never a self-collision: `excludeSelf`.

### CL6 — The owning tenant must be usable *(rules.manual `tenant-must-exist`, scope `insert`; maintainer: three refusals + trial + the insert-only positive)*

| # | Case | Expected |
|---|---|---|
| CL6.1 | insert into a tenant id nobody owns | **422** `ClaimTenantDoesNotExistNotification` |
| CL6.2 | insert into an ARCHIVED tenant | **422** same key |
| CL6.3 | insert into a SUSPENDED tenant | **422** same key |
| CL6.4 | insert into a **TRIAL** tenant | **201** — a trial tenant is a live customer |
| CL6.5 | a definition whose tenant is suspended AFTERWARDS: `PATCH /claims/{id}` | **200** — the rule is INSERT-only by decision, so a suspension can never make an existing definition impossible to correct |

`CL6.5` is the row that proves the decision the code calls deliberate; without it, an
implementation that widened the rule to `insertOrUpdate` would pass every other case here.

### CL7 — Narrowing `AppliesTo` against HELD values *(rules.manual `applies-to-narrowing-refused`; maintainer: all six transitions, with and without a held value)*

**The rule:** refuse a change to `AppliesTo` that would stop admitting an identity kind for
which an ACTIVE edge still holds a value — asking ONLY about the kinds the new value DROPS,
and only when the value actually changed. This is the only guard in the service that asks
its question of two OTHER aggregates (`user_claims`, `client_claims`), through two facts of
this entity's own domain service.

| # | Transition | Held value | Expected |
|---|---|---|---|
| CL7.1 | `both` → `user` | an ACTIVE **client** value | **422** `ClaimAppliesToCannotExcludeHeldValuesNotification` |
| CL7.2 | `both` → `user` | nothing | **200** — a definition nobody holds narrows freely |
| CL7.3 | `both` → `client` | an ACTIVE **user** value | **422** |
| CL7.4 | `both` → `client` | nothing | **200** |
| CL7.5 | `user` → `client` | an ACTIVE **user** value | **422** — the dropped kind is `user` |
| CL7.6 | `user` → `client` | nothing | **200** |
| CL7.7 | `client` → `user` | an ACTIVE **client** value | **422** |
| CL7.8 | `client` → `user` | nothing | **200** |
| CL7.9 | `user` → `both` | a **user** value held | **200** — a widening asks NOTHING and always passes |
| CL7.10 | `client` → `both` | a **client** value held | **200** — same |
| CL7.11 | `both` → `user` | an **ARCHIVED** client value | **200** — *added by this plan, for approval:* a value somebody removed must not freeze the definition's shape, and this is the predicate an implementation would most easily get wrong. It follows from the exhaustive choice; it is flagged rather than smuggled in |

`CL7.9`/`CL7.10` are the rows that keep the field from becoming immutable in practice —
the outcome the model gate decided against.

### CL8 — The two-bucket catalog cap *(rules.manual `claims-per-tenant-cap-*`, `claimsPerTenantCap = 20`; maintainer: bucket independence + only-when-it-ADDS)*

**The rule:** at most 20 ACTIVE definitions per tenant may admit a user (`user` + `both`),
and at most 20 may admit a client (`client` + `both`), counted from ONE grouped query. The
guard fires only when a write ADDS the kind.

Setup once, in a tenant of its own: 20 definitions with `appliesTo: user`.

| # | Write | Expected |
|---|---|---|
| CL8.1 | a 21st with `appliesTo: user` | **422** `TooManyUserClaimsInTenantNotification`, `{max}` rendering **20** |
| CL8.2 | a 21st with `appliesTo: both` | **422** same key — `both` admits users |
| CL8.3 | a 21st with `appliesTo: client` | **201** — **the buckets are independent**: a full user side must never block a definition that admits only clients |
| CL8.4 | at the cap, a PATCH changing only `description` | **200** — the write adds no kind, so the guard must ask nothing |
| CL8.5 | at the cap, `client` → `both` on the row CL8.3 created | **422** `TooManyUserClaims…` — a widening that ADDS the full kind is exactly when the guard fires |
| CL8.6 | in a second tenant, 20 with `appliesTo: client`, then a 21st `client` | **422** `TooManyClientClaimsInTenantNotification` — the half `U15.2` cannot see |

### CL9 — `defaultValue` cannot be returned to null through PATCH *(the endpoint's own documented contract; maintainer: assert it, both surfaces)*

`CL9.1` REST `PATCH {defaultValue: null}` → **200** and the PREVIOUS value intact — absence
and an explicit null cannot be told apart in a partial body, which the route's description
states in as many words. `CL9.2` the GraphQL twin answers identically. **Consequence,
recorded rather than filed as a defect:** mounted this way, a `defaultValue` once set cannot
be withdrawn by any route — only by archiving the definition and recreating it.

### CL10 — Archive, rebirth, and the edge that does not re-attach *(spec.md §5; maintainer: the whole cycle)*

`CL10.1` archive → hidden from the listing and from by-id; `?includeArchived=true` reveals
it with `archivedAt` set.
`CL10.2` `PATCH /claims/{id}/unarchive` → **404** — no unarchive is mounted, on purpose.
`CL10.3` re-inserting the SAME name → **201** with a **DIFFERENT id**.
`CL10.4` a `user_claims` entry written against the OLD id still carries the OLD `claimID`
after the rebirth — it does **not** silently re-attach to the replacement, and reaching the
new definition requires an explicit new entry. **This is the row that proves the decision
the whole edge design rests on** (`spec.md` §5: "that is the same property that made
`role_permissions` store the id and not the string").

### CL11 — The join is INNER, filled on every load, and read-only

`CL11.1` the three joined fields come back filled on every read, with the counterpart's
actual values. `CL11.2` sending `tenantWorkspace`/`tenantStatus`/`tenantArchivedAt` in a
POST or PATCH body changes nothing — they are read-only by construction, never written
through this aggregate.

### Rules recorded as UNPROVEN

- `ClaimNameIsImmutableNotification` · `ClaimTenantIsImmutableNotification` ·
  `ClaimValueTypeIsImmutableNotification` — unreachable through every mounted surface
  (`W13`). Printed as SKIPPED with that reason.

---

## 2. Data hygiene

**Inherited verbatim from `tenant-contract` §2.** Throwaway `authcore_qa`, dropped and
recreated per run; migration 0012 reseeds; no Mongo, no CDC, so the reset is a relational
one and there is no projection to drain. Residue within a run: archived definitions and the
fixtures, all namespaced by `$QA_RUN_ID` through the existing `claim_name` counter.

**Two new HUMAN principals, built through the service's own login flow:**

- **Principal M — `qa-claimop`**, in `$QA_TENANT_SCOPED`: the whole claim vocabulary
  (`claim:insert` · `claim:update` · `claim:archive` · `claim:read`, rows `…0001`–`…0004`)
  plus `tenant:read`, `user:read`/`user:set-claim` and `client:read`/`client:set-claim` —
  the last four are what `CL7` needs to write the edges its question is asked of.
- **Principal N — `qa-claimlim`**, same tenant: **`claim:read` only**. The only caller for
  which the four-way verb split is visible: it may list and open a definition and must be
  refused on insert, patch and archive.

**Three throwaway TENANTS beyond the shared ones**, so the caps and the tenant-state rows
do not disturb any other lane's counts: one for `CL8`'s user bucket, one for `CL8.6`'s
client bucket, and one **trial** tenant for `CL6.4`. `CL6.2`/`CL6.3` archive and suspend
tenants of their own.

A failure to build any principal or tenant is NOT fatal: the affected rows skip loudly into
the report's SKIP column.

---

## 3. Security — what this round ADDS

`qa/security.sh`, family **S9**. §3a and §3b are inherited (§0a). Every valid token comes
from the service's own login route (`auth.issuer.enabled: true` — the best of §3c's three
sources). Nothing is invented, no key is forged.

### S9.1 — Layer 1: the permission gate, per route and per surface

Five REST routes and five GraphQL fields, gated by **four** distinct literals. Per route,
both directions:

- **the negative** — a principal holding only `tenant:read` → **403** with the
  missing-permission key, on all five;
- **the complement** — principal M → **2xx** on all five (a gate that refuses everyone is
  also broken);
- **the split** — principal N (`claim:read` only): **2xx** on `GET /claims` and
  `GET /claims/{id}`; **403** on `POST /claims`, `PATCH /claims/{id}` and
  `PATCH /claims/{id}/archive`. N is the only caller that can see a write route mounted
  under the read literal.

**Per surface, not by analogy** — a route gated on REST is not thereby gated on GraphQL.

### S9.2 — Layer 2: `N/A`, and the reason is worth writing down

`Claim` declares **no identity-derived `BuildRules` clause**: there is no owner-check and
no "unless admin", because a claim definition confers nothing and there is no escalation
surface (`spec.md` §7: "No caller-identity fact"). What stands in its place is Layer 3
plus the `assignedFrom` seat, and `S9.3` is where both are proven. Stated, not skipped.

### S9.3 — Layer 3: tenant scoping, on reads AND writes

- **cross-tenant READ** — a foreign tenant's definition is absent from M's listing, and
  `GET /claims/{foreign-id}` answers **404**, not 403: it does not exist for this caller,
  which leaks nothing about who else exists (`spec.md` §10). *The isolation leak is a 200,
  which is exactly why this needs its own case.*
- **the FORCED filter** — M sends `?tenantID=<foreign>`: `ToCriteria` **overwrites** rather
  than merges, so the answer is M's own rows — never the foreign ones, and never an empty
  page that would let a merge masquerade as a scope.
- **cross-tenant WRITE** — `POST /claims` with a foreign `tenantID` → **403**
  `TenantMismatchNotification`. `bypassMaySet: true` means the mapper applies a stated value
  whoever sent it; **the guard, not the read filter, is what refuses.**
- **the `*:*` bypass** — the bootstrap admin creates a definition INSIDE the scoped tenant
  and reads across tenants; a resource wildcard (`claim:*`) does not cross.
- **`noIdentity: stand-down`** — recorded as UNPROVABLE in this posture and why: it is
  reachable only under `auth.mode: disabled`, which the framework's own boot guard permits
  in dev alone. Named in the plan and printed as SKIPPED, never asserted from an
  authenticated call.

### S9.4 — `Restrict`: none, and the posture that replaces it

`spec.md` §9: **no field-level read authz** — a definition is vocabulary, not a secret, and
any holder of `claim:read` in the tenant is entitled to every field including
`defaultValue`. So no `FieldAccessForbiddenNotification` exists to assert here, and the
`__typename` edge (`X3`) is a parity case rather than a boundary case. Asserted as the
posture: the full thirteen-field document comes back for principal **N**, the weakest
caller that may read at all.

### S9.5 — The tokenless direction, per entity

None of the five claim routes is in `publicRoutes`, so each answers **401** tokenless — on
both surfaces. This is the second direction of the public-route split applied to this
entity's own paths: the direction that catches an entry widened past its intent.

---

## 4. Audit — `qa/audit.sh`, `A76`+

Asserted through SQL, never through an endpoint: the `database` destination writes its row
inside the SAME transaction as the write it records.

`A76` `insert`, `update` and `archive` each write ONE row against the claim's own
`aggregate_id`, `entity_type = 'Claim'`, with the actor stamped as the `sub` of the token
the suite itself holds.
`A77` **no `unarchive` row can ever exist** for this entity — the absence is a contract, not
an omission (the shape the permission round established).
`A78` the configured `auditClaims` (`tenant_workspace`, `email`, `name`, `identity_kind`)
are present on the actor stamp of a claim write.
`A79` a PATCH's delta carries the `appliesTo` from/to pair — the field whose change the
narrowing guard exists to police, and therefore the one an access review looks for.

---

## 5. Files this round writes

| File | Action |
|---|---|
| `specs/qa/claim-contract/plan.md` | **new** — this document |
| `qa/claim.sh` | **new** — family **W**, REST |
| `qa/claim_graphql.sh` | **new** — family **X**, GraphQL |
| `qa/domain.sh` | **extend** — family **CL** (§1b) |
| `qa/security.sh` | **extend** — block **S9** (§3) |
| `qa/audit.sh` | **extend** — `A76`–`A79` (§4) |
| `qa/run.sh` | **extend** — `claim` and `claim_graphql` added to `LANES`; principals **M** and **N** and the three throwaway tenants built in the fixture phase |
| `qa/lib/common.sh` | **comment only** — *(deviation from this plan, recorded 2026-09-08)*: the three helpers promised here (`archive_claim`, `patch_claim`, `claims_of_tenant`) were **not** added. Every lane calls `api` directly for those three verbs, exactly as `qa/client.sh` does for its own, so the helpers would have been dead code the reconcile rule would then have to explain. The existing `claim_name`/`new_claim` carried the round unchanged, and the file's stale header — which said Claim had no lane — was corrected |
| `qa/run.sh` | **also** — the `PLANS` string and the report title now name seven rounds instead of six; without it a green report would still have introduced itself as the six-entity run |
| `specs/scaffold-entity/claim/spec.md` | **corrected** — the six §0c items, each with a dated supersession note. Prose only |

Nothing else is touched: no application code, no yaml, no migration.

---

## 6. Report contract

Unchanged and inherited: `qa/qa-report.md`, rendered LIVE after every lane, with the
abort trap, the SKIP column kept separate from the pass count, and the failures section
carrying real response bodies. This round adds **two rows** to the matrix (`claim`,
`claim_graphql`) and extends three existing lanes.

The SKIP column is where this round's honest gaps land, and they are named here in advance:
the three unreachable immutability rules (`W13`) and `noIdentity: stand-down` (`S9.3`).

---

## 7. Out of scope

Load and performance; UI; exports (`Claim` declares none — CSV/XLSX are not mounted);
gRPC (not wired); integration events (**no broker in this posture** — `spec.md` §9, and
`integration_events` carries no row for this aggregate, which `A76`'s row count would
notice if it changed). Token emission is out of scope by the model's own decision, per §0b.
