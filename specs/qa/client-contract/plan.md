# QA plan — `client-contract`

- **Status:** APPROVED — maintainer (Cláudio Schirmer Guedes), 2026-09-08, at the plan gate ("Aprovar — gerar e rodar"), including the three §0c prose corrections and the §5 file list.
- **Suite slug:** `client-contract` — what this round proves: the whole wire contract of the
  `Client` aggregate and its **three** collections on both surfaces, the business rules its
  approved specs and the maintainer state, and the five things that make this entity unlike
  every other one already proven:
  1. **it is the only aggregate that MINTS a credential and reveals it exactly once** — two
     reveal seats (`POST /clients` and `POST /clients/{id}/secret`), a plaintext no column
     holds, and a hash kept out of every copy the framework makes of the row;
  2. **the rotation is a TEMPORAL contract** — an overlap window (0…604800 s, 86400 when
     omitted) in which BOTH secrets sign in, a zero window that kills immediately, and an
     expiry that closes for real — provable only through `POST /auth/client/token`;
  3. **eight permission verbs, the widest split in the service** — each collection carries
     its own (`grant` / `manage-network` / `set-claim`), plus the hand-written
     `rotate-secret`, all seeded (`…0005`–`…000c`);
  4. **this is the first round in which a MACHINE is a caller** — the token route mints
     `identity_kind: "client"`, so `C14b` (a client token rotates only its own secret) is
     live, and its 2026-08-28 NARROWING (ordinary writes on sibling clients are permitted)
     gets a sentinel;
  5. **the two status machines split by DESIGN**: `InvalidClientStatusTransition` declares
     `SemanticValidation` → **422** (like `Tenant`, unlike `User`), while
     `ClientMustBeActiveToRotate` declares `SemanticStateConflict` → **409** — the
     state-conflict flavor the user round predicted this lane would exercise
     (`internal/domain/client_test.go:275,285` pins both semantics).
- **Written:** 2026-09-08
- **Pin:** omnicore **`v0.74.0`** · dialect postgres · read backing **relational**
  (read-your-writes — every read-back below is IMMEDIATE; a needed poll would itself be a
  failure). The ONE legitimate wait in this round is `C13c`'s grace-window expiry, which is
  a bounded wait on a TEMPORAL contract, not a poll on a projection.
- **Surfaces in scope:** REST + GraphQL (`surfaces.graphql.enabled: true`) — full parity,
  **13 operations each** (5 generated root/read + 7 collection verbs + the hand-written
  rotation).
- **Plan destination:** `specs/qa/client-contract/plan.md` · **suite destination:** `qa/` at
  the project root · **verdict destination:** `qa/qa-report.md`
- **Prior rounds:** `tenant-contract` (APPROVED 2026-09-06), `permission-contract`,
  `role-contract`, `group-contract`, `user-contract` (all APPROVED 2026-09-07). This round
  **EXTENDS the same `qa/run.sh`** — no second entry point — and none of the five approved
  plans is reopened.

## 0. Where every expectation below comes from

Nothing here was read off a running service. The service was never called while this plan
was written. The one exception is an ENUMERATION and never a value that becomes an
expectation: the suite reads `GET /openapi.json` at boot to cross-check the verb inventory
(`Q11`).

| Source | What it settled |
|---|---|
| `specs/scaffold-entity/client/spec.md` (APPROVED 2026-08-26, six gate rounds; superseded notes 2026-09-06) | the model; §B Q1–Q8e; §7 the C0–C18 rule table; §9 reads and permissions; §10 the three authorization layers; §E the three entity-specific verify checks (secret-never-twice, constant-time compare, zero-window kills) |
| `specs/scaffold-entity/client/tasks.md` | the eight build deviations — including **deviation 5** (`previousSecretExpiresAt` IS in the `?fields=` vocabulary, out of filter and sort) and the **closed 2026-09-01** item: the create DOES hand back the secret |
| `specs/evolve-entity/client/spec.md` (the `claims` collection) | the `ClientClaim` shape, its `change` verb as a PATCH excluding `ClaimID`, the cap of 20, the three manual rules and their one-answer interlock, the `client:set-claim` verb |
| `specs/omnicore-gen/client.omnicore.yaml` | fields, the three collections, the three joins (incl. `TenantStatus` **hidden — rules-only**), notifications and their semantics, modes, rules (declarative + manual) and their ORDER, `service.facts`, `read.byParams`, `authz` |
| `specs/implement/client-credentials-token/plan.md` (APPROVED 2026-09-01) | what the token route decides vs what the AGGREGATE decides; the generic-401-vs-500 asymmetry; the two dormant rules waking; the trusted-proxy honest-no |
| `internal/domain/client.go` · `client_rules_manual.go` | `Modes()` = display/insert/update/archive (no unarchive); the transition map `active↔suspended`; the three caps; the manual rules C7/C2/C8/C9/C10/claim-triple/C6/C14b as wired |
| `internal/web/client_routes.go` · `client_secret_routes_manual.go` | the **13** REST endpoints and **13** GraphQL fields, the permission literal on each; the rotation as a POST answering **200 with a body**; `gracePeriodSeconds` nullable ON PURPOSE on both surfaces (zero ≠ absent) |
| `internal/web/requests/find_clients_by_params.go` · `find_client_by_id.go` · `insert_client.go` · `patch_client.go` · `rotate_client_secret.go` · the six collection requests + `patch_client_claim.go` · `dtos/client_*.go` | the DTO opt-in gate: the listing's filters/controls (no `search`); the by-id serving **`includeArchived` alone**; `InsertClientResponse.Secret` and `RotateClientSecretResponse.Secret` as the ONLY two seats of the plaintext; no request or response DTO declaring `secretHash`/`previousSecretHash` |
| `internal/web/requests/issue_client_token.go` | the token exchange body (`clientId`, `clientSecret`) — spoken by the domain lane as a FIXTURE flow, never asserted as a contract of its own |
| `internal/application/queries/find_clients_*` `ToCriteria` | the tenant filter unless `IsSuperAdmin()` — Layer 3, and why a foreign row answers 404 |
| `migrations/postgres/0007/0011_*client*` · `0012_bootstrap_seed_manual.up.sql` | the partial unique indexes (`(tenant_id, name)`, `(client_id, role_id)`, `(client_id, cidr)`, `(client_id, claim_id)`, all `WHERE archived_at IS NULL`); the eight `client:*` catalog rows `…0005`–`…000c` |
| `qa/microservice.qa.yaml` · `qa/run.sh` · `qa/lib/common.sh` | the posture; `POST /auth/client/token` in `publicRoutes`; the helper API every lane speaks |
| pin docs — `status-mapping`, `auto-handlers`, `auto-query-handlers`, `read-joins`, `relational-view`, `auth-middleware`, `authz-seams`, `graphql`, `rules-dsl`, `audit`, `token-issuance` | every status code, notification key and envelope shape asserted below |
| **the maintainer, asked 2026-09-08** (`AskUserQuestion`, two rounds, all answers on record) | token route **exercised, not owned**; grace window proven all three ways (overlap + zero-kill + 1 s expiry with a ≤3 s bounded wait); the CIDR mint gate **both directions + the relax door**; `C14b` **negative + the narrowing's positive**; caps in **HYBRID** form; the audit lane extended **with the secret sweep** |

### 0a. What this round INHERITS and does not repeat

Proven service-wide by the five approved rounds, against this exact posture: the whole
**401 family** (token shape, signature, `iss`, `aud`, `alg`, `exp`, expired-vs-invalid);
the **framework's appended public surfaces** and the **GraphQL introspection bypass**;
**both directions** of the public-route split and its exactness neighbours; the middleware
**tenant-claim gate**; the **boot, hygiene and report contracts**; and the generic
**relational read contract** (cursor walks, the only-total conflict matrix, the page
ceiling, the malformed-cursor family) — asserted here only where `Client`'s own vocabulary
differs (`Q7`, `Q8`).

### 0b. What is DELIBERATELY not in this round

- **`POST /auth/client/token` is exercised, not owned** (maintainer, 2026-09-08). The
  domain lane calls it because the credential lifecycle is observable nowhere else, and it
  asserts only what the **aggregate and its stored rows decide**: which secret verifies,
  when the old one stops, who is eligible, which address may mint. The route's OWN contract
  — the claim vocabulary (`identity_kind`, `name`, `permissions`, `x_*`), the lockout
  counters, the single generic 401's key, the absent `/refresh` — is a later
  `authentication-contract` round.
- **`Claim` has no lane yet.** It is touched only as the fixture the `claims` collection
  points at; its own contract is the last remaining entity round.

### 0c. Stale prose in the Client specs — three items, and the proposed disposition

All three are verifiable in the code, all three contradict something this suite asserts,
and per the standing rule the fix is **in the same round, at the source, with a dated
supersession note** — the same disposition the user round's §0c received. Approval of this
plan approves these edits; they are listed in §5.

| # | What `spec.md` says | What the code does | Where this suite pins it |
|---|---|---|---|
| a | the 2026-08-26 amendment banner: "**One promise is not met and is OPEN**: `POST /clients` does not hand back a secret" | `tasks.md` closed it 2026-09-01: `InsertClientResponse` carries `secret`, filled from the entity after the write — the create IS a reveal seat | `Q3`, `C-SEC1` |
| b | §3 and §9 name the collection routes `/clients/{id}/allowed-cidrs` | the mounted path is `/clients/{id}/allowedCIDRs` (`client_routes.go:173,188`) — camelCase like every other collection segment | `Q1` |
| c | §2 property 5 / §9: `previousSecretExpiresAt` "appears in no filter, no sort **and no `?fields=` vocabulary**" | it IS projectable (`tasks.md` deviation 5, reasoned and accepted): out of every filter and sort, but served and selectable — an operator asking "until when does the old secret work" should not have to derive it | `Q8.7` (filter/sort → 400) · `Q7.6` (`?fields=` → 200) |

---

## 1. Coverage matrix — the FRAMEWORK's promises

Entity **Client** × surfaces **REST** (`qa/client.sh`, family **Q**) and **GraphQL**
(`qa/client_graphql.sh`, family **V**). Read backing relational: every write→read-back is
immediate. Archive regime *kept-but-hidden*. Three collections, three read joins.

Envelope asserted on REST: `errors[].messages[].notificationKey` + the HTTP status. On
GraphQL: HTTP always 200, the same key on `errors[].extensions.notificationKey`. Every
request pins `Accept-Language: en-US`.

The lane runs as the bootstrap admin (`*:*`); §1b and §3 are where the scoped and MACHINE
principals do the work.

### Q1 — Happy path, one per served verb (13 routes)

| REST | GraphQL twin | Expected |
|---|---|---|
| `POST /clients` | `createClient(input:)` | **201** · the record AS STORED, **`secret` present** (`Q3`) |
| `PATCH /clients/{id}` | `patchClient(id:, input:)` | **200** |
| `PATCH /clients/{id}/archive` | `archiveClient(id:)` | **204, NO BODY** / `{success}` |
| `GET /clients` | `clients(...)` | **200** · `data` + `pagination` / the connection |
| `GET /clients/{id}` | `client(id:)` | **200** · the full document |
| `POST /clients/{id}/secret` | `rotateClientSecret(id:, input:)` | **200** · `{id, secret, secretChangedAt, previousSecretExpiresAt}` — a POST answering a BODY, the one credential-minting response of the service |
| `POST /clients/{id}/roles` | `addClientRole(id:, input:)` | **201** · `{clientId, clientRole:{id, roleID}}` |
| `PATCH /clients/{id}/roles/{rid}/archive` | `archiveClientRole(id:, input:)` | **204** / `{success}` |
| `POST /clients/{id}/allowedCIDRs` | `addClientAllowedCIDR(id:, input:)` | **201** · `{id, cidr, label}` under the owner |
| `PATCH /clients/{id}/allowedCIDRs/{cid}/archive` | `archiveClientAllowedCIDR(id:, input:)` | **204** / `{success}` |
| `POST /clients/{id}/claims` | `addClientClaim(id:, input:)` | **201** · `{id, claimID, value}` under the owner |
| `PATCH /clients/{id}/claims/{cid}` | `patchClientClaim(id:, input:)` | **200** · the entry, **id KEPT** |
| `PATCH /clients/{id}/claims/{cid}/archive` | `archiveClientClaim(id:, input:)` | **204** / `{success}` |

`Q1.14` — **`patchClientClaim` keeps the entry's id** and changes ONLY `value`
(`change.shape: patch`, `patchExcludes: [ClaimID]`).

### Q2 — The write/read asymmetry

`Q2.1` a collection write response carries the stored column(s) and **nothing traversed**
(no `roleKey`, no `claimName`, no `*ArchivedAt`). `Q2.2` a READ carries the traversal.
`Q2.3` `POST /clients` answers **no `createdAt`/`updatedAt`/`archivedAt` and no
`tenantWorkspace`/`tenantArchivedAt`** — the insert Response declares none of them.
`Q2.4` the same for the PATCH. `Q2.5` the ROTATE response deliberately carries **no
client row** — exactly four keys, nothing more (`rotate_client_secret.go`'s own contract).

### Q3 — Golden record: every declared field, written then read back

One client exercising every field, read back field-by-field on both surfaces:

- **`secret` in the CREATE response, once, matching `^acs_[A-Za-z0-9_-]{43}$`** — and
  ABSENT (`assert_absent`, never present-and-null) from the by-id read, every listing row,
  the PATCH response, and every GraphQL selection thereafter. §E check 1, first face.
- **`secretHash` and `previousSecretHash` appear in NO body of ANY operation** on either
  surface — no Response DTO declares them.
- **`previousSecretExpiresAt` is `null`/absent on a freshly created client** — nothing is
  retiring on a first issue (`credential-minting`).
- **`secretChangedAt` is present and recent** on a freshly created client.
- **the joined fields ride the read**: `tenantWorkspace`, `tenantArchivedAt` on the root;
  `roleKey`/`roleName`/`roleArchivedAt` per role entry;
  `claimName`/`claimValueType`/`claimArchivedAt` per claim entry; and the `allowedCIDRs`
  entries carry **exactly** `{id, cidr, label}` — that collection declares NO join.
- **`tenantStatus` appears NOWHERE** — see `Q10`/`S8.4`; the rules-only join field is the
  leak case nobody writes.
- **an empty `allowedCIDRs: []` is SERVED** on the by-id read and on listing rows — the
  visible spelling of "unrestricted", standing in for the refused `ipRestricted` computed
  field (`tasks.md` deviation 4).

### Q4 — Validation, 422, asserting the KEY

One representative per VO and shape: `InvalidDisplayNameNotification` (name, and the CIDR
entry's label) · `InvalidDescriptionNotification` (under 15 runes) ·
`UnknownClientStatusNotification` · the three CIDR answers (`C15`/`C16` in §1b carry the
full family) · `InvalidClaimValueNotification` (empty / over-256 value) ·
`RequiredFieldNotification` on an absent required field · `InvalidIDUUIDNotification` on a
malformed `tenantID` — and, because `tenant-valid` is a **guard**, a request carrying a
malformed `tenantID` AND a second problem reports the id ALONE.

### Q5 — The dual 409 — and the split the user round predicted

**Duplicate flavor (`SemanticConflict`), four paths:** `ClientNameAlreadyExists` (service
pre-check + partial unique backstop) · `ClientAlreadyGrantsRole` · `ClientAlreadyAllowsCIDR`
· `ClientAlreadyHoldsClaim`. Each asserted through a second POST, and through one insert
body carrying the same value twice.

**Wrong-state flavor (`SemanticStateConflict`) — `ClientMustBeActiveToRotate` → 409** on
rotating a suspended client (§1b `C13f`). **And the deliberate asymmetry pinned:**
`InvalidClientStatusTransition` is `SemanticValidation` → **422** (`Q5.6`: an out-of-map
move like `active → lixo` is `UnknownClientStatus` 422; a well-formed illegal edge does not
exist in a two-state machine, so the transition rule's negative is the enum's own refusal —
stated here so nobody expects a 409 from this status machine the way `User`'s gives one).
`Q5.7` both edges pass and a no-op PATCH re-sending the current status passes.

### Q6 — Archive, one-way by design

`Q6.1` archive → 204. `Q6.2` gone from the listing. `Q6.3` `?includeArchived=true` reveals
it, `archivedAt` non-null — **and `status: "suspended"`** even when archived while active
(`C6`, archive-forces-suspended, asserted on the ROW). `Q6.4` by-id 404 without the flag,
200 with it. `Q6.5` `PATCH /clients/{id}/unarchive` → **404 `RouteNotFoundNotification`** —
no unarchive exists, and `Tenant` remains the only aggregate that mounts one.
`Q6.6` **the name comes back**: `POST /clients` reusing the archived client's exact name in
the same tenant → 201 (the partial index releases the handle — the same release that makes
an unarchive unmountable). `Q6.7` the root archive stamps every ACTIVE entry of all three
collections (SQL). `Q6.8` the stamp is SCOPED: an entry archived before the root keeps its
original stamp. `Q6.9` every write onto an archived client → **404** (`ScopeActive`), one
case per verb — the rotation included: an archived client's secret is not rotatable, which
is half of what "a revocation nobody can un-revoke" means (the other half is `C-MINT4`:
its secret no longer signs in).

### Q7 — Read vocabulary, per declared operator family

From `FindClientsRequest`'s tags: `tenantID` (`eq,in`) · `name`
(`eq,ne,in,startswith,istartswith,contains,icontains`) · `description`
(`contains,icontains`) · `status` (`eq,in`) · `secretChangedAt` (`gte,lte`) ·
**`tenantWorkspace`** (`eq,in,startswith,istartswith,contains,icontains`) — the root-join
reach that distinguishes this backing · `createdAt`/`updatedAt` (`gte,lte`) · `id`
(`eq,in`).

`Q7.5` sort, one per declared leaf. `Q7.6` `?fields=` — including
`?fields=previousSecretExpiresAt` → **200** (§0c item c) and
`?fields=roles.roleKey,claims.claimName` → 200 (child-join projection).
`Q7.7` `?onlyTotal=true` alone and beside a filter. `Q7.8` `?includeArchived`.
`Q7.9` the pagination envelope as a contract (truthfulness, page-2 disjointness, a forward
walk via `?after=`, `?last=` alone serving the tail).
`Q7.10` **the by-id read serves `includeArchived` and NOTHING else** — its Request declares
one control, so `?fields=` presence there is a typed 400 (`Q8.9`).

### Q8 — Rejected reads: the typed-400 family, and the credential oracle

`Q8.1` unknown filter key · `Q8.2` operator outside the allowlist (`?status.contains=`) ·
`Q8.3` `?search=` — undeclared → 400 (`spec.md` §9's recorded decision: no text index) ·
`Q8.4` an unresolvable `?fields=` path (`fields[bogus]`) · `Q8.5` a filter VALUE outside
the leaf's kind (`?secretChangedAt.gte=lixo`, `?tenantID.eq=lixo` →
`InvalidFilterValueNotification`) · `Q8.6` `?first=` above the ceiling · mixed directions ·
only-total conflicts · malformed cursor · cursor↔`orderBy` and cursor↔`includeArchived`
mismatches (one representative each; the full matrix is inherited).

`Q8.7` **`previousSecretExpiresAt`**: filter (`.gte=`) → 400, `?orderBy=` → 400 — while
`Q7.6` shows it projectable. The three-way split of §0c item c, pinned.

**The credential oracle — the four faces, on the listing:**
`Q8.8a` `?secretHash.eq=9f86…` → **400 `SchemaViolationNotification`** ·
`Q8.8b` `?previousSecretHash.startswith=9f` → 400 · `Q8.8c` `?orderBy=secretHash` → 400 ·
`Q8.8d` `?fields=secretHash` (and `previousSecretHash`) → **400, never a silent `200 {}`**.
These are what stands between a stored hash and a character-at-a-time walk; they fail the
day somebody adds a `filter:` tag, which is why they exist.

`Q8.9` an undeclared reserved control on the by-id read: `?fields=name` → 400 — PRESENCE
trips the DTO gate (`Q7.10`).

`Q8.10` **the 1:N boundary**: `?roles.roleKey.eq=…` and `?claims.claimName.eq=…` → 400 —
served and projectable is not filterable. **And the rules-only root field**:
`?tenantStatus.eq=active` → 400, `?fields=tenantStatus` → 400 — hidden means OUT of the
vocabulary, not merely out of the row.

### Q9 — Absent verbs, wrong addresses, malformed ids

`Q9.1` no route at all (`PATCH /clients/{id}/unarchive`, `DELETE /clients/{id}`) → 404
`RouteNotFoundNotification` · `Q9.2` a mounted path under another method only → 405 ·
`Q9.3` a valid-but-absent uuid → 404 per read and per write — the rotation included ·
`Q9.4` **a non-uuid address split by VERB**: `GET /clients/lixo` → 404
`UnknownIDAddressNotification`; `PATCH /clients/lixo` and `POST /clients/lixo/secret` →
400 `MalformedIDNotification`; the same split on a child address. Identical on both
surfaces. `Q9.5` a child id that is a valid uuid but belongs to ANOTHER client → 404 —
entries are addressed inside their owner.

### Q10 — The three read joins, and the rule-vs-wire split

| join | kind | fields | served? | filterable? | projectable? |
|---|---|---|---|---|---|
| Tenant | root inner on `tenant_id` | `tenantWorkspace` | yes | **yes** (`Q7`) | yes |
| | | **`tenantStatus`** | **NO — rules-only** (`hidden: true`) | no (400) | no (400) |
| | | `tenantArchivedAt` | yes | **no — absent by choice** (400) | yes |
| Role | in-child inner on `role_id` | `roleKey`, `roleName`, `roleArchivedAt` | yes | no (`Q8.10`) | yes (`Q7.6`) |
| Claim | in-child inner on `claim_id` | `claimName`, `claimValueType`, `claimArchivedAt` | yes | no | yes |

`Q10.4` **the counterpart's stamp is the entry's account of outliving its target**: archive
a Role still granted → the entry SURVIVES with `roleArchivedAt` non-null and
`roleKey`/`roleName` still resolved; same for an archived Claim definition. `?includeArchived`
governs ROOTS, never what a traversal reaches — the yaml says so in as many words.
`Q10.5` `ClientAllowedCIDR` declares NO join: its entries carry exactly their two stored
columns plus `id`, asserted in `Q3`.

### Q11 — Route inventory cross-check

`GET /openapi.json` enumerates **13** `/clients…` operations, each carrying the permission
literal the two mount files declare — `client:insert`, `client:update`, `client:archive`,
`client:read` (×2), `client:grant` (×2), `client:manage-network` (×2), `client:set-claim`
(×3), `client:rotate-secret`. An enumeration, never an expectation; a disagreement is a
FINDING.

### V — GraphQL, the same operations under the surface's own idiom

`V1` the 13 fields resolve · `V2` the golden record round-trips identically — the secret
present in `createClient`'s and `rotateClientSecret`'s payloads and in NO other field's,
`secretHash`/`previousSecretHash`/`tenantStatus` in no selection at all · `V3` a typed
refusal rides `errors[].extensions.notificationKey`, HTTP 200 · `V4` an undeclared
argument/out-of-schema value is cut by the parser BEFORE any resolver, no `notificationKey`
— the surface's idiom, never the REST envelope · `V5` **`gracePeriodSeconds` is nullable in
the schema and the two absences differ**: the mutation WITHOUT the argument answers a
`previousSecretExpiresAt` ≈ now+86400, WITH `0` answers none — the collapse a non-null Int
would cause, asserted as a contract · `V6` `fields`/`onlyTotal` are selection-natural here,
never gated.

---

## 1b. Domain expectations — from the specs, and from the maintainer

Family **C** in `qa/domain.sh`, beside `T`/`P`/`RL`/`GR`/`U`. Rows ranked by the cost the
maintainer's spec names (§7: "C9 and C10 are the rules that matter most and the easiest to
skip"; the §E triple). Every row has BOTH cases; machine-token fixtures come from the
service's own routes (§2). `POST /auth/client/token` is called ONLY as the observation
instrument the maintainer approved (§0b).

| # | The rule | Source | POSITIVE | NEGATIVE → status · key |
|---|---|---|---|---|
| **C-SEC1** | **The secret is revealed exactly once per mint, and its HASH reaches no surface** | §2 "the secret, end to end", §E-1; **asked — full sweep approved** | the create's `secret` matches `^acs_[A-Za-z0-9_-]{43}$` and MINTS A TOKEN at `/auth/client/token` — the credential is real, not decorative | absent from by-id/listing/patch bodies and every GraphQL selection (`Q3`); the four oracle 400s (`Q8.8`); `audit_events.payload` carries `***` for both hash columns and the plaintext appears NOWHERE in that table (SQL, `A` lane); the SERVER LOG of the run contains no `acs_` token (grep over `$SERVER_LOG`, the "never logged, on any path" clause) |
| **C13a** | **Rotation OVERLAPS: during the window BOTH secrets sign in** | §B-Q3; the rotate route's own doc; **asked — approved** | rotate (default window): the response's new secret mints a token AND the old secret still mints one; `previousSecretExpiresAt` ≈ now+86400 | after `C13b`'s zero-kill, the killed secret is refused with the SAME generic 401 a wrong secret gets — no oracle distinguishing "expired" from "wrong" on an unauthenticated route |
| **C13b** | **A zero window is an immediate kill, not a zero-length overlap** | §7 C13, §E-3 | rotate with `gracePeriodSeconds: 0` → 200, response carries **no** `previousSecretExpiresAt` | the old secret is refused IMMEDIATELY; SQL: both `previous_*` columns are **NULL** — cleared, not stamped with a past instant |
| **C13c** | **The expiry CLOSES for real** | §B-Q3 "the old secret retires itself"; **asked — 1 s window + bounded ≤3 s wait approved** | rotate with `gracePeriodSeconds: 1`: within the second, the old secret still mints | after the bounded wait, the old secret is refused; the new one still mints |
| **C13d** | **The window is bounded: 0…604800** | §B-Q3b | `0` and `604800` are accepted | `604801` and `-1` → **422 `InvalidGracePeriodNotification`** (`max: 604800`) |
| **C13e/f** | **Only an ACTIVE, live client rotates** | `ClientMustBeActiveToRotate`; `ScopeActive` | an active client rotates → 200 | suspended → **409 `ClientMustBeActiveToRotateNotification`** (the service's second StateConflict, first time exercised); archived → **404** |
| **C14b** | **A CLIENT token rotates only ITS OWN secret — and ONLY the rotation is so bound** | §7 C14b (narrowed 2026-08-28); `client_rules_manual.go`; the claim now MINTED; **asked — negative + positive approved** | machine A rotates A's secret → 200; **the narrowing's sentinel:** machine A, holding the permissions, PATCHes and archives sibling client B → 2xx — the case that fails if the pre-narrowing breadth ever comes back | machine A rotates B's secret → **403 `ClientMayOnlyRotateItsOwnSecretNotification`**; and a USER token with `client:rotate-secret` rotates any tenant row → 200 (the rule stands down when `identity_kind` is not `client`) |
| **C-MINT1..4** | **The CIDR allow-list gates WHERE a token is minted — fail-open when empty, and the relax door is real** | §C-3, §C-3a; **asked — both directions + relax approved** | empty list → the mint passes; list containing `127.0.0.1/32` → passes (the suite calls from localhost, the socket IP) | list containing ONLY `203.0.113.0/24` → the mint is refused with the same generic 401 (no oracle); **C-MINT3, the relax door:** archiving the LAST entry reopens the mint — "empty = any address", proven from the dangerous side; **C-MINT4:** an ARCHIVED client's secret no longer mints at all |
| **C15/C16** | **The CIDR VO: canonical only, and one spelling for "no restriction"** | §2's `vos.CIDRBlock` (amended: REFUSES, names the canonical) | `203.0.113.0/24`, `203.0.113.5/32` and `2001:db8::/32` are accepted and stored as sent | `203.0.113.5/24` → **422 `CIDRHasHostBitsSetNotification`**; `lixo`, `203.0.113.0` (no mask) → **422 `InvalidCIDRBlockNotification`**; `0.0.0.0/0` AND `::/0` → **422 `UniversalCIDRNotAllowedNotification`** |
| **C8** | **A granted role must exist, be active, and belong to THIS client's tenant — ONE answer for all three** | §7 C8; `RoleIsUnavailableInTenant` | a live role of the tenant attaches → 201 | a foreign tenant's role id, an ARCHIVED role, and an absent-but-well-formed uuid all answer **422 `RoleNotAvailableInTenantNotification`** — no existence oracle over another tenant's catalogue |
| **C9/C10** | **No escalation and no wildcard onto a MACHINE — and the wildcard check fires FIRST** | §7 ("the rules that matter most"); the panic-ordering note | principal K grants a role conferring only what K holds → 201 | a role conferring `permission:archive` (which K lacks) → **403 `CannotGrantRoleWithUnheldPermissionsNotification`**; the `master` (`*:*`) role → **403 `CannotGrantWildcardRoleNotification`** — and it must be THIS key, the order that keeps `HasPermission` from panicking into a 500 |
| **C-CL1..3** | **The claim triple: available in tenant · applies to clients · value parses as the declared type — and the interlock gives ONE answer for an unresolvable id** | evolve spec §4d mirrored; the three facts | `appliesTo: client` and `both` attach; `number`←`"1000"`, `bool`←`"true"`, `string`←non-empty; the PATCH corrects a value → 200 | `appliesTo: user` → **422 `ClaimDoesNotApplyToClientNotification`**; `number`←`"abc"`, `bool`←`"yes"` → **422 `ClaimValueDoesNotMatchValueTypeNotification`** — and the PATCH is judged too (ADDED and CHANGED); an unresolvable id → `ClaimNotAvailableInTenantNotification` **alone**, per the no-guard one-pass design **(the yaml's own comment warns an unresolvable id raises all three — the suite asserts the interlock the evolve spec and User's twin established; if the wire answers three keys, that is a FINDING to route, not a case to soften)** |
| **C2** | **The owning tenant must be available — trial PASSES, suspended refuses** | `tenant-available`; User's U3 twin | a client in an `active` and in a `trial` tenant → 201 | an archived and a `suspended` tenant → **422 `ClientTenantDoesNotExistNotification`**, one answer for the three causes |
| **C1/C1b** | **Tenant isolation on writes, archive included; only `*:*` states a tenant** | inherited C1; `assignedFrom: identity-claim` + `bypassMaySet` | principal K writes inside its tenant; the admin states `tenantID` explicitly → 201 in the named tenant | K sending ANOTHER tenant's id → **403 `TenantMismatchNotification`**; K archiving a foreign client → 403/404 per the read-side scope |
| **C4** | **`name` unique per tenant over ACTIVE rows** | §2 Unique | the same name in TWO tenants → both 201; archived releases the name (`Q6.6`) | the same name twice in one tenant → **409 `ClientNameAlreadyExistsNotification`**, via pre-check and via the constraint backstop |
| **C5/C6** | **`active ⇄ suspended`, and archive FORCES suspended** | §7 C5/C6 | both edges and a no-op pass | an unknown status → 422 (`Q5.6`); after archiving an ACTIVE client, `?includeArchived` shows `status: "suspended"` — the rule reached the ROW |
| **C11/C17/CL-cap** | **The caps: 50 roles · 20 CIDRs · 20 claims — HYBRID form** (maintainer, 2026-09-08) | §3 caps; evolve cap | the **20th** CIDR and the **20th** claim each attach → 201 (end-to-end) | the **21st** CIDR → **422 `TooManyAllowedCIDRsForClientNotification`** (`max: "20"`); roles at the BOUNDARY only — one insert carrying 51 entries → **422 `TooManyRolesForClientNotification`** — and this plan records that the passing side at exactly 50 was not exercised. **Addendum 2026-09-08, discovered while generating and sourced from `specs/evolve-entity/claim-catalog-cap/spec.md` (never from a response): the claims-cap NEGATIVE is unreachable through the API.** The catalog caps ACTIVE definitions at 20 per tenant per identity kind — the same number as the per-client cap — so a 21st distinct attachable definition cannot exist and `TooManyClaimsForClientNotification` can never fire over the wire: defense in depth, not dead code. What the suite asserts instead (`C-CLcap`): the client-side count reaches exactly 20 and the CATALOG refuses the definition that would be needed to overflow the client |
| **C-READ1** | **READ scope: a tenant JWT sees only its own clients; a foreign row is 404, not 403** | `ToCriteria`; §10 layer 3 | K lists its tenant's clients; the admin lists across | the foreign row is simply NOT in K's listing (an isolation leak is a 200 — its own case), and K's by-id of a foreign client → **404** |

**Rows left UNPROVEN, named rather than quietly asserted:**

- **`C3` (tenant immutability) and the claim entry's definition-immutability** may be
  unreachable through this API: `PatchClientRequest` declares `name`/`description`/`status`
  and `PatchClientClaimRequest` declares `value` — no body field exists for the
  notifications to fire on. The suite sends the extra key anyway and asserts the **DTO
  gate** as a disjunction stated NOW, before any request: either the unknown key is
  rejected, or it is silently dropped and the read-back proves nothing moved — in the
  second case `ClientTenantIsImmutableNotification` is recorded as unreachable, never as
  covered.
- **The trusted-proxy half of the allow-list.** No profile configures one, so the mint
  judges the SOCKET address; the suite proves localhost semantics only. Whether a spoofed
  `X-Forwarded-For` could walk through a real deployment is `/omnicore:configure`'s
  question, named here because §F says the feature is theatre without it.
- **The allow-list constrains where a token is OBTAINED, not where it is USED** — asserted
  nowhere because it is not assertable here; repeated from §F so nobody reads `C-MINT` as
  more than it is.
- **`?search=`, exports, gRPC** — not wired; `Q8.3` proves the refusal.
- **The token's claim vocabulary** (`identity_kind`, `name`, `x_*` values reaching the
  token) — deliberately deferred with the route's own round (§0b), except the ONE bit
  `C14b` cannot avoid depending on: that a machine token drives the row rule, which the
  403/200 pair proves from the outside.

---

## 2. Data hygiene

**Inherited verbatim from `tenant-contract` §2.** Throwaway `authcore_qa`, dropped and
recreated per run; migration 0012 reseeds; no Mongo, no CDC. Residue within a run: archived
clients/entries and the fixtures, all namespaced by `$QA_RUN_ID`.

**Two new HUMAN principals, built through the service's own flow:**

- **Principal K — `qa-clientop`**, in `$QA_TENANT_SCOPED`: the whole client vocabulary —
  all EIGHT verbs (`…0005`–`…000c`) — plus `role:read`/`role:insert`,
  `claim:read`/`claim:insert`, `tenant:read`, and **deliberately NOT `permission:archive`**
  (exactly what `C9`'s escalation negative needs it to lack; the role CONFERRING
  `permission:archive` is created by the admin, and the question is whether K may ATTACH
  it).
- **Principal L — `qa-clientlim`**, same tenant: `client:read` + `client:update` and NONE
  of `grant`/`manage-network`/`set-claim`/`rotate-secret`/`insert`/`archive`. The only
  caller for which the eight-way verb split is visible: it may relabel a client and must be
  refused on the six collection routes AND on the rotation.

**Two MACHINE principals — the first in any round:** clients **A** and **B**, created by K
in `$QA_TENANT_SCOPED`; their secrets captured from the create responses; their tokens
minted at `POST /auth/client/token` (a declared public route — nothing invented). Client A
additionally holds a role, created by K, conferring `client:read`/`client:update`/
`client:archive`/`client:rotate-secret` — the set `C14b`'s positive and negative both need,
and one K may grant because K holds every permission it confers. A failure to build any
principal is NOT fatal: the affected rows skip loudly into the report's SKIP column.

---

## 3. Security — what this round ADDS

`qa/security.sh`, family **S8**. §3a and §3b are inherited (§0a). Every valid HUMAN token
comes from the service's own login route; every MACHINE token from `/auth/client/token`
with a secret the service itself minted. Nothing is invented, no key is forged.

### S8.1 — Layer 1: the permission gate, per route and per surface

Thirteen REST routes and thirteen GraphQL fields, gated by **eight** distinct literals —
the widest vocabulary in the service. Per route, both directions:

- **the negative** — principal B (`tenant:read` only) → 403 with the missing-permission
  key, on all thirteen;
- **the complement** — principal K → 2xx on all thirteen (a gate that refuses everyone is
  also broken);
- **the split** — principal L: 2xx on `GET /clients`, `GET /clients/{id}`,
  `PATCH /clients/{id}`; **403** on the two role routes, the two CIDR routes, the three
  claim routes and the rotation. A and B answer identically on all thirteen either way; L
  is the only caller that can see a collection route mounted under the wrong literal —
  including the deliberate design that **`client:grant` does NOT open the network or claim
  routes**, and vice versa (three collections, three verbs, zero overlap).

**Per surface, not by analogy** — a route gated on REST is not thereby gated on GraphQL.

### S8.2 — Layer 2: the identity-derived row rule

`C14b` as a BOUNDARY: two calls differing only in WHO — machine A on its own rotation (200)
vs machine A on B's (403 `ClientMayOnlyRotateItsOwnSecret`) — plus the stand-down half: a
USER token holding `client:rotate-secret` rotating either row (200). The only `BuildRules`
clause in this service that reads `identity_kind`.

### S8.3 — Layer 3: tenant scoping

Cross-tenant READ (the foreign row absent from K's listing; by-id 404 — the leak that is a
200); cross-tenant WRITE (403 `TenantMismatch`); the `*:*` bypass acts INSIDE another
tenant, never across two — the admin creates a client in the scoped tenant and may attach
only THAT tenant's roles and claims (`C8`/`C-CL1` refuse the mix).

### S8.4 — `Restrict`: none — and what replaces it is asserted as a posture

`spec.md` §9: no field-level read authz; the hashes are off the wire BY CONSTRUCTION, which
is stronger. So no `FieldAccessForbiddenNotification` exists to assert. What replaces it:
the credential-oracle 400s (`Q8.8`) and the `tenantStatus` 400s (`Q8.10`) run **as the
admin too** — unreachable for everyone is the claim, so the strongest caller must get the
same four refusals principal B gets.

### S8.5 — The mint gate as a security boundary

`C-MINT1..4` live in the domain lane; `S8.5` adds the refusal-shape assertion: a wrong
secret, a killed secret, an out-of-range address and a suspended client are answered by the
SAME generic 401 — the mint route confirms nothing about WHICH check failed, on a route
that is public by design. (The lockout counters behind repeated failures belong to the
token round — the suite spaces its deliberate failures to stay under the threshold, a
constraint `qa/lib` already respects for the user login.)

### S8.6 — What stays UNPROVEN, printed as SKIP

The trusted-proxy question (§1b); the token's own claim vocabulary and lockout (§0b);
`externalValidator` (not configured).

---

## 4. Out of scope, named plainly

Load/performance · UI · gRPC (not wired) · exports (not wired) · integration events (no
broker) · the `Claim` aggregate's own contract · the token route's own contract (§0b).

**The audit lane IS extended** (`qa/audit.sh`, from `A68`; maintainer-approved with the
secret sweep): one `audit_events` row per write — insert, patch, archive, rotation, one per
collection verb — with the right `verb`/`kind`/`actor`/`tenant_id`; the insert's and the
rotation's payloads carrying `***` where BOTH hash columns would be; and the plaintext
(`acs_%`) appearing **nowhere in the table** (SQL) — the fourth face of `C-SEC1`.

---

## 5. Runner contract, and the files this round touches

**There is still exactly ONE runner.**

**CREATED:**

| file | what |
|---|---|
| `qa/client.sh` | family **Q** — the REST contract |
| `qa/client_graphql.sh` | family **V** — the GraphQL twins |

**EDITED:**

| file | change |
|---|---|
| `qa/run.sh` | `LANES=(… user user_graphql client client_graphql domain security audit)` — inserted after `user_graphql`, keeping the entity order; principals **K** and **L**; the header comment gaining this round's plan |
| `qa/domain.sh` | family **C** — the §1b rows, including the machine-principal fixtures and the ONE bounded wait (`C13c`) |
| `qa/security.sh` | family **S8** |
| `qa/audit.sh` | the §4 extension, from `A68` |
| `qa/lib/common.sh` | `client_body`, `new_client` (returns id AND captured secret), `mint_client_token`, `attach_client_role`/`_cidr`/`_claim` — the fixtures families Q, C and S8 all speak |

**EDITED, prose (§0c, approved with this plan):**

| file | change |
|---|---|
| `specs/scaffold-entity/client/spec.md` (amendment banner) | the OPEN item → closed 2026-09-01, pointing at `tasks.md`'s closure, dated supersession note |
| `specs/scaffold-entity/client/spec.md` §3 + §9 | `/allowed-cidrs` → `/allowedCIDRs` (three spots), dated note |
| `specs/scaffold-entity/client/spec.md` §2 property 5 + §9 | `previousSecretExpiresAt`: out of filter and sort, **in** `?fields=` — aligned with `tasks.md` deviation 5, dated note |

Everything else — root resolution, fail-fast + `--all`, per-run namespacing, SIGTERM +
drain, non-zero exit per lane — inherited unchanged.

---

## 6. Report contract

Inherited unchanged. The matrix grows by two rows (`client`, `client_graphql`).

---

## 7. The deliberate-RED meta-case

Mandatory: **`Q8.8d` (`?fields=secretHash` → 400) flipped to expect 200**, run, watched
FAIL — proving in the same breath that the report renders the RED lane, names the case,
prints the real body and stamps a RED footer — then restored.
