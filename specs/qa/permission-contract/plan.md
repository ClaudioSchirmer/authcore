# QA plan — `permission-contract`

- **Status:** APPROVED — maintainer (Cláudio Schirmer Guedes), 2026-09-07. §3c **(i)** and **(ii)**
  both approved; the Permission scope question of §3c(ii) resolved in the same breath — see the
  note under §3c.
- **Suite slug:** `permission-contract` — what this round proves: the whole wire contract of the
  `Permission` catalog on both surfaces it is mounted on, the business rules its own specs
  declare, and the one thing that makes this entity unlike every other in the service —
  **three columns go in, two values come out**.
- **Written:** 2026-09-07
- **Pin:** omnicore **`v0.74.0`** · dialect postgres · read backing **relational**
  (read-your-writes — every read-back below is IMMEDIATE; a poll would itself be a failure)
- **Surfaces in scope:** REST + GraphQL (both mounted; `surfaces.graphql.enabled: true`)
- **Plan destination:** `specs/qa/permission-contract/plan.md` · **suite destination:** `qa/` at the
  project root · **verdict destination:** `qa/qa-report.md`
- **Prior round:** `specs/qa/tenant-contract/plan.md` (APPROVED 2026-09-06). This round
  **EXTENDS the same `qa/run.sh`** — it does not create a second entry point and it does not
  reopen that plan. What it inherits from it is named in §0b; what it ADDS is everything else.

## 0. Where every expectation below comes from

Nothing here was read off a running service. The service was never called while this plan was
written. The one exception is the same as last round's and is an ENUMERATION, never a value
that becomes an expectation: the runner reads `GET /openapi.json` at boot to cross-check the
verb inventory.

| Source | What it settled |
|---|---|
| `specs/scaffold-entity/permission/spec.md` (APPROVED 2026-08-19, superseded note 2026-09-06) | the model; §B Q1–Q8; the "three columns in, two out" doctrine; §7a/7b/7c the rules; §9 the read vocabulary; §10 the gate taxonomy |
| `specs/evolve-entity/permission/spec.md` (APPROVED 2026-09-03) | the `Key`→`Permission` rename, `echoValue: true`, and the three wire-visible envelope changes of its §5 — which are exactly what cases E2/E3 below pin |
| `specs/omnicore-gen/permission.omnicore.yaml` | fields, the composite VO, notifications, modes, rules, `service.facts`, `read.computed`, `read.managed`, `byParams`, `authz` |
| `internal/domain/permission.go` · `permission_rules_manual.go` · `vos/permission_key.go` | the rules as implemented — including that `UnmatchablePermissionKeyNotification` is emitted **under `Action`**, not under the pair |
| `internal/web/permission_routes.go` | the 5 REST endpoints + the 5 GraphQL fields, and the permission literal on each |
| `internal/web/requests/find_permissions_by_params.go` · `find_permission_by_id.go` · `insert_permission.go` · `patch_permission.go` | the DTO opt-in gate: which controls each read serves, which operators each leaf admits, which fields each body carries, and **which fields the Responses carry** — the read Response has no `Resource` and no `Action` |
| `internal/application/queries/find_permissions_by_params_query.go` · `find_permission_by_id_query.go` | `ToCriteria` returns the criteria unchanged on **both** reads — Permission carries no identity-derived row filter (this is what makes §3's layer 2 `N/A`) |
| `migrations/postgres/0012_bootstrap_seed_manual.up.sql` | the token source and the catalog the gate itself runs on |
| `qa/microservice.qa.yaml` · `qa/run.sh` · `qa/lib/common.sh` | the posture this suite runs under and the helper API every new lane speaks |
| pin docs — `status-mapping`, `auto-handlers`, `auto-query-handlers`, `authz-seams`, `auth-middleware`, `graphql`, `audit` | every status code, notification key and envelope shape asserted below |
| **the maintainer, asked 2026-09-07** (`AskUserQuestion`) | §1b rows 9 and 10; the criticality ranking; §4's seed decision |

### 0b. What this round INHERITS and does not repeat

The tenant round already proved these against this exact posture, and re-asserting them would
double the runtime without covering one more promise:

- the whole **401 family** (§3a there) — token shape, signature, `iss`, `aud`, `alg`, `exp`;
- the **framework's own appended public surfaces** (docs, `/openapi.json`, JWKS, playground)
  and the **GraphQL introspection bypass** with its edges;
- the **tenant gate** (`tenant.required: true` → 403 `TenantMissingNotification`), proven on
  the second boot with the suite-owned keypair;
- the **boot, hygiene and report contracts** (§2, §5, §6 there), reused verbatim.

What this round adds to the security lane is only what is Permission-specific: §3 below.

---

## 1. Coverage matrix — the FRAMEWORK's promises

Entity **Permission** × surfaces **REST** and **GraphQL**. Read backing **relational**, so every
write→read-back is immediate. Archive regime *kept-but-hidden* (no `DeleteOnArchive`). The
aggregate is **flat** — no children, no siblings, no shared base, and **no read join of its own**
(it is a traversal TARGET, never a source).

Envelope asserted on REST: `errors[].messages[].notificationKey` + the HTTP status, and the
`field` where the pin promises one. On GraphQL: HTTP is **always 200** and the same key rides
`errors[].extensions.notificationKey`. Every request pins `Accept-Language: en-US`.

**The shape that governs this whole matrix.** `resource` and `action` are stored, filterable
and orderable, and appear in **no response body on any surface**. `permission` is computed from
them, appears in every response body, and is **neither filterable nor orderable**. A case that
forgets which half of that sentence it is testing proves nothing, so each family below says
which side it is on.

### E1 — Happy path, one per served verb

`Modes()` declares exactly `display, insert, update, archive` — **four modes, five routes**, and
no unarchive anywhere. The inventory is cross-checked against `GET /openapi.json` at boot; a
source-vs-openapi disagreement is a FINDING, not something the suite reconciles.

| REST | GraphQL twin | Expected |
|---|---|---|
| `POST /permissions` | `createPermission(input:)` | **201** · `{id, description, permission}` |
| `GET /permissions/{id}` | `permission(id:)` | **200** · the full document |
| `GET /permissions` | `permissions(...)` | **200** · `data` + `pagination` / the Relay connection |
| `PATCH /permissions/{id}` | `patchPermission(id:, input:)` | **200** · `{id, description, permission}` |
| `PATCH /permissions/{id}/archive` | `archivePermission(id:)` | **204, NO BODY** / payload `{success, id}` |

### E2 — Golden-record round-trip, and its NEGATIVE half

The positive half: one record exercising every declared field, written then read back
field-by-field on all four read shapes (REST by-id, REST listing row, GraphQL node, GraphQL
connection edge):

`id` · `description` · `permission` · `createdAt` · `updatedAt` · `archivedAt`.

`archivedAt` is asserted `null` while active and **stamped after archive** (visible only via
`?includeArchived=true`). There is no unarchive, so it never returns to `null` — the stamp is
terminal on this entity, and that is asserted as such.

**The negative half is the family nobody writes, and on this entity it is the point:**
`resource` and `action` must appear in **no** response body — not in the insert response, not
in the patch response, not in the by-id read, not in a listing row, not in a GraphQL node.
Asserted as **key-absent**, not as null.

**The composite's own name reaches no request body either.** The write side speaks the EXPOSED
PARTS (`resource`, `action`) and never the composite field's name. Sending
`{"permission": "tenant:read", "description": "…"}` therefore supplies neither half →
**422 `RequiredFieldNotification`** on `resource` and on `action`.

### E3 — Validation, 422, notification KEY and FIELD asserted (never prose)

| Input | Key | field |
|---|---|---|
| `resource` empty | `RequiredFieldNotification` | `resource` |
| `action` empty | `RequiredFieldNotification` | `action` |
| `resource` = `Tenant` (uppercase — refused, never repaired) | `InvalidResourceNameNotification` | `resource` |
| `resource` = ` tenant ` (padded — refused, never trimmed) | `InvalidResourceNameNotification` | `resource` |
| `resource` = `ten*` (wildcard mixed into a slug) | `InvalidResourceNameNotification` | `resource` |
| `resource` = `user:*` (wildcard as a segment inside a path) | `InvalidResourceNameNotification` | `resource` |
| `resource` = `a` (1 rune, under the 2-rune floor) | `InvalidResourceNameNotification` | `resource` |
| `resource` = `aaaa` (run of 4 identical runes) | `InvalidResourceNameNotification` | `resource` |
| `resource` = `:tenant` · `tenant:` · `a::b` (empty segment) | `InvalidResourceNameNotification` | `resource` |
| `resource` 65 runes | `InvalidResourceNameNotification` | `resource` |
| `action` = `profile:read` (a colon — the action is exactly ONE segment) | `InvalidActionNameNotification` | `action` |
| `action` = `Read` | `InvalidActionNameNotification` | `action` |
| `resource` = `*` with `action` = `read` | `UnmatchablePermissionKeyNotification` | **`action`** — the half that has to change, per `permission_key.go`; asserting it on `permission` would be asserting a lie |
| `description` = `abc` (under 15 runes) | `InvalidDescriptionNotification` | `description` |
| **both halves malformed in one call** | **both** keys in one 422 — `IsValid` evaluates both parts before short-circuiting, on purpose | |

Positive shapes that must be **accepted**, each one a rule's own boundary:
`tenant`+`read` · `user:profile`+`read` (the hierarchy) · `3d-assets`+`read` (leading digit) ·
`*`+`*` (the super-admin row) · `tenant`+`*` · `client`+`rotate-secret` (hyphenated slug).

### E4 — The 409 family, and the two things this entity echoes back

| Case | Expected |
|---|---|
| Insert a pair held by an **ACTIVE** row | **409** `PermissionAlreadyExistsNotification`, semantic `"Conflict"` |
| the same 409 names **`permission`** as its field | `field == "permission"` — the `evolve-entity` §5.1 change, pinned so it cannot drift back to `key` |
| the same 409 carries the refused pair as its **value** | `value == "tenant:read"` — `unique.echoValue: true` rendering through `PermissionKey.String()`; §5.2 of that spec, and the half that used to be `null` |
| Insert a pair held only by an **ARCHIVED** row | **201** — see §1b row 1. **The opposite of Tenant**, and the single most important behavioural difference between the two catalogs |

**The wrong-state 409 is `N/A`, for the same reason it was on Tenant.**
`EntityIsNotActiveNotification` / `ConcurrentModificationNotification` are not reachable through
this aggregate's HTTP surface: no PUT verb, no wire field carries a revision, and every
wrong-state attempt is intercepted a layer earlier by the LOAD scope — where it lands as **404**
(E7). Asserting a 409 there would encode a promise the pin does not make.

### E5 — Archive, which on this entity is a ONE-WAY DOOR

`archive` → **204, no body** · by-id → **404** `RecordNotFoundNotification` · by-id
`?includeArchived=true` → **200** with `archivedAt` stamped · listing → row absent · listing
`?includeArchived=true` → row present.

**And then it stops.** There is no unarchive mode, no unarchive route and no unarchive mutation.
The way back is a fresh insert of the same pair, which answers **201 with a NEW id** — asserted
as a different id, because "it came back with the same id" would mean the one-way door has a
hinge. No `DeleteOnArchive`, so absence is never the expectation; no child table, so the
stamp-scoped child unarchive is `N/A — flat aggregate`.

### E6 — Read vocabulary (everything the DTO declares, and only that)

- **Filters, one per declared operator family** — over the two HIDDEN parts and the visible one:
  `resource` `eq,ne,in,contains,startswith` · `action` `eq,ne,in,contains` ·
  `description` `contains` · `createdAt` `gte,lte` · `updatedAt` `gte,lte`.
  Wire form `?resource.eq=tenant`; GraphQL `where: { resource: { eq: "tenant" } }`.
  **Every one of these filters a column the caller can never see in a response** — that is the
  assertion, not a side effect of it.
- **`?orderBy=`**: `resource`, `-resource`, `action`, `-action`, `description`, `-description` —
  the three the spec declares sortable, both directions. GraphQL:
  `orderBy: [{field: RESOURCE, direction: DESC}]`.
- **`?fields=`**: `?fields=permission` → the rendered pair and nothing else, proving the
  selection pushes `Resource` + `Action` down to the store while neither surfaces ·
  `?fields=id,description` → exactly those two.
- **`?onlyTotal=true`** → `{success, status, description, pagination: {totalCount}}` — no `data`,
  no cursors.
- **`?last=N` alone** → the TAIL window, `hasNextPage: false`.
- **Pagination envelope as a contract**: `totalCount` truthful against a known seeded+inserted
  set · `hasNextPage`/`hasPreviousPage` at both ends · `startCursor`/`endCursor` as WINDOW EDGES,
  walked by echoing `endCursor` into `?after=` · page-2 disjoint from page-1 · backward via
  `last` + `before` · **and the EMISSION rule**: `endCursor` exactly when `hasNextPage`,
  `startCursor` exactly when `hasPreviousPage`, so the head of a forward walk carries no
  `startCursor`.
- **`?includeArchived=true`** on both the listing and the by-id read.

### E7 — Rejected reads: the whole typed-400 guard family

Every row asserts the KEY **and**, where the pin promises one, the `field` the envelope names.

| Request | Status · key · field |
|---|---|
| `?bogus=x` | 400 · `SchemaViolationNotification` · `bogus` |
| `?description.eq=x` (operator outside the leaf's allowlist — `description` declares only `contains`) | 400 · `SchemaViolationNotification` |
| `?action.startswith=r` (`startswith` is declared on `resource`, not on `action`) | 400 · `SchemaViolationNotification` |
| **`?permission.eq=tenant:read`** | 400 · `SchemaViolationNotification` — **computed, so it backs no column; the caller filters the two sources instead** |
| `?search=foo` | 400 · `SchemaViolationNotification` on `search` — the DTO opt-in gate |
| `?fields=bogus` | 400 · `SchemaViolationNotification` · `fields[bogus]` |
| **`?fields=resource`** | 400 · `SchemaViolationNotification` — **stored, filterable, orderable, and NOT selectable.** The case that exists only on this entity |
| `?fields=id` on the **by-id** route (which declares only `includeArchived`) | 400 · `SchemaViolationNotification` · `fields` |
| `?createdAt=lixo` (value outside the leaf's kind) | 400 · `InvalidFilterValueNotification` — pin ≥ v0.70.0 |
| `?first=101` | 400 · `LimitExceededNotification`, `value` = **100** (framework default: no `maxLimit` on the view, none in the yaml) |
| `?first=5&last=5` · `?first=5&before=X` · `?after=X&before=Y` | 400 · `SchemaViolationNotification` |
| `?onlyTotal=true` beside `fields` / `orderBy` / `first` / `after` | 400 · `SchemaViolationNotification` · `onlyTotal[fields]`, `onlyTotal[orderBy]`, `onlyTotal[first]`, `onlyTotal[after]` |
| `?onlyTotal=1` · `?onlyTotal=` · `?includeArchived=1` | 400 on the control's own key — the booleans take exactly `true`/`false` |
| `?onlyTotal=false&first=10` | **200** — present-but-inactive never trips the conflict matrix |
| `?onlyTotal=true&resource.eq=tenant` · `&includeArchived=true` | **200** — counting a filtered subset is the canonical use |
| `?after=not-a-cursor` | 400 · `SchemaViolationNotification` |
| a cursor minted under the default order, replayed with `?orderBy=resource` | 400 · `SchemaViolationNotification` |
| a cursor minted without archived rows, replayed with `?includeArchived=true` | 400 · `SchemaViolationNotification` |
| **`?orderBy=permission`** | 400 · `SchemaViolationNotification` · `orderBy[permission]` — computed |
| `?orderBy=createdAt` (filterable, deliberately not sortable) | 400 · `SchemaViolationNotification` · `orderBy[createdAt]` |
| `?orderBy=id` (declarable — `sort: [ID]` is ordinary — and deliberately not declared) | 400 · `SchemaViolationNotification` · `orderBy[id]` |

**`UnsupportedCapabilityNotification` is `N/A` on this entity, by construction.** It needs a
control the DTO DOES declare and the relational backing cannot serve. `FindPermissionsRequest`
declares no `Search` field, so `?search=` is refused at the wire wrapper before any engine is
reached; and there is no 1:N child to filter or sort across. Asserting that key here would be
asserting a lie — the same conclusion the tenant round reached, for the same reason.

### E8 — Addressing and absent verbs: the three-way split, **including the arm Tenant could not offer**

| Request | Expected |
|---|---|
| **`PATCH /permissions/{id}/unarchive`** | **404 `RouteNotFoundNotification`** — no route is registered at that path at all. Tenant mounts unarchive, so this arm was unreachable last round; here it is the whole point of §1b row 1 |
| `DELETE /permissions/{id}` · `PUT /permissions/{id}` · `POST /permissions/{id}` | **405 `MethodNotAllowedNotification`** — the path is registered, the method is not |
| `GET /permissions/{id}/archive` | **405** — same path, wrong method |
| `GET /does-not-exist` | **404 `RouteNotFoundNotification`** |
| `GET /permissions/{unknown-uuid}` | **404 `RecordNotFoundNotification`** |
| `GET /permissions/not-a-uuid` (a **read** address) | **404 `UnknownIDAddressNotification`** |
| `PATCH /permissions/not-a-uuid` · `/archive` (a **write** intention) | **400 `MalformedIDNotification`** |
| `archive` on an **already-archived** row | **404** — `LoadForWrite` runs the default `ScopeActive` |
| `PATCH` on an **archived** row | **404** — same scope |

**The 403 mode-not-allowed shape is `N/A` here, and the reason is precise.** That shape needs a
mode absent from `Modes()` **while its route is still mounted**. Permission's absent mode
(`unarchive`) has no route at all, so it lands on the 404 arm above, and its absent verb
(`DELETE`) has a registered path, so it lands on 405. No `…NotAllowedNotification` is reachable
on this entity.

### E9 — The pair is structurally absent from PATCH, proven by EFFECT

`PatchPermissionRequest` carries **only** `description` (`patchExcludes: [Permission]`). PATCH is
the lenient handler, so an unknown key is ignored rather than refused.

- **Case**: `PATCH {"description": "…", "resource": "hijacked", "action": "stolen"}` → **200**,
  and a read-back proves `permission` is unchanged.
- **Consequence recorded honestly**: `PermissionKeyIsImmutableNotification` is **unreachable
  through any mounted surface** — the declarative `immutable` rule is a belt-and-braces layer
  behind a structural cut, exactly as the yaml says, and GraphQL mounts the same PATCH shape.
  The suite asserts the EFFECT and does **not** assert a notification no request can provoke.
  (`specs/evolve-entity/permission/spec.md` §5.3 says the same in the other direction.)

### E10 — GraphQL, where the idiom differs BY DESIGN

- Every rejection family of E3/E4/E7/E8 repeated over `POST /graphql`, asserting
  `errors[].extensions.notificationKey` at HTTP **200**.
- **The strongest cross-surface proof of "three in, two out" lives here.** The connection's
  `where:` argument accepts `resource` and `action` — they come from the Request DTO — while the
  `Permission` **type** has no `resource` and no `action` field, because it comes from the
  Response. So:
  - `permissions(where: {resource: {eq: "tenant"}}) { edges { node { permission } } }` → **works**;
  - `permission(id:) { resource }` → a **schema validation error** ("Cannot query field"), not a
    runtime 400, and carrying no `notificationKey`.
  One surface, one schema, and the two halves of the doctrine visible in the same document.
- An **undeclared argument** (`search:`) is cut out of the schema entirely — asserted as a
  gqlparser validation error, never as the REST envelope.
- `fields` and `onlyTotal` have no argument here — selection-natural. Selecting `totalCount`
  ALONE *is* the only-total mode, so a ceiling case must select `edges` too.
- `orderBy: [{field: PERMISSION}]` → a schema-validation error (the value is not in the order
  enum, because the computed path is not in the `sort:` vocabulary).
- **No `unarchivePermission` field exists** → "Cannot query field" on the mutation type. The
  GraphQL twin of E8's 404 arm.
- **`__typename` beside a normal selection must answer identically** — the v0.72.1 regression
  guard, kept as a standing case.
- **Handler invariance**: the same operation on both surfaces produces the same effect on a
  read-back and the same notification key.

---

## 1b. Domain expectations — what the BUSINESS requires

The only rows in this plan the framework never had an opinion about. Source named per row;
`asked` means the maintainer answered on **2026-09-07** and the answer is recorded verbatim.
Ranked by the cost the maintainer named — rows 1–4 were **all four** called critical.

| # | The rule | Source | POSITIVE case | NEGATIVE case | Rank |
|---|---|---|---|---|---|
| 1 | "An archived `tenant:export` does not block a fresh one — re-inserting is the ONLY route back now that `/unarchive` does not exist." | `spec.md` §B Q3 + §B Q5 + `service.facts.PermissionKeyTaken activeOnly: true` | archive a pair, insert it again → **201**, and the new row carries a **DIFFERENT id** | insert a pair held by an **ACTIVE** row → **409** `PermissionAlreadyExistsNotification`, field `permission`, value the rendered pair | **critical** |
| 2 | "`*` is legal only as the ENTIRE part, and a `*` resource forces a `*` action — the claim matcher honours exactly three shapes, so anything else is a row that matches nothing while reading like a grant." | `spec.md` §7a rule 2 + §7b rule 6 + `vos/permission_key.go` | `*`+`*` → **201** · `tenant`+`*` → **201** | `*`+`read` → **422** `UnmatchablePermissionKeyNotification` on **`action`** · `user:*`+`read` and `ten*`+`read` → **422** `InvalidResourceNameNotification` | **critical** |
| 3 | "The pair IS the permission's identity in every issued token and every existing grant; editing it would rewrite what all of them mean, retroactively and invisibly." | `spec.md` §B Q2 + `rules.list.key-immutable` + `update.patchExcludes` | PATCH `description` → **200**, pair unchanged | PATCH carrying `resource`/`action` → **200** and the pair **still unchanged** (E9: the notification is unreachable by design, on both surfaces) | **critical** |
| 4 | "Three columns go in, two go out." | `spec.md` header + §B Q7 + §B Q8 + the Request/Response DTOs | filter and order by `resource` and `action` → **200** with the expected rows · `?fields=permission` → the render alone | `resource`/`action` absent from **every** response body on **both** surfaces · `?fields=resource` → **400** · `?orderBy=permission` and `?permission.eq=` → **400** · `permission(id:){resource}` → a GraphQL schema error | **critical** |
| 5 | "NOTHING IS NORMALIZED. A value that does not already comply is refused, never repaired — it matters more here than anywhere, because the rendered pair is compared byte-for-byte against a token claim." | `vos/permission_key.go` | `3d-assets`+`read` → **201**, stored exactly as sent | `Tenant`, ` tenant `, `TENANT` → **422**, **and no normalized variant is ever stored** (proven by a follow-up listing that finds nothing) | high |
| 6 | "The description must explain the permission, not repeat it — under a fold that drops whitespace, colons and hyphens." | `rules.manual.description-does-not-echo-key` + `permission_rules_manual.go` | a real sentence about the permission → **201** | `tenant-read` · `Tenant Read` · `tenant:read` · `TENANTREAD`, each against the pair `tenant:read` → **422** `PermissionDescriptionEchoesKeyNotification` on `description` | high |
| 7 | "The resource may be a colon-joined PATH; the action is exactly ONE segment — which is what makes the LAST colon segment always the action, so `user:profile:read` parses back one way." | `spec.md` §7a rules 3–4 | `user:profile`+`read` → **201**, and the read-back renders `user:profile:read` | `action` = `profile:read` → **422** `InvalidActionNameNotification` | high |
| 8 | The reused `vos.Description` anti-junk floor: 15–500 runes, ≥2 words, ≥5 distinct runes, ≥1 vowel, no run of 4. | `spec.md` §2 + `vos/description.go` | a real operator description → **201** | the boundary value on each side → **422** with the matching key | medium |
| 9 | **"Archiving a permission still granted to a role is legal; the grant stays pointing at the archived row and that is HISTORY, not an error."** | **asked 2026-09-07** — *"Está certo"* | archive a pair a role still grants → **204**, and the `role_permissions` row **still exists** (asserted by SQL, since no endpoint exposes it) | — the negative is row 10: the grant surviving must NOT mean the power survives | **asked** |
| 10 | **"A retired permission must actually stop authorizing."** | **asked 2026-09-07** — *"Sim, com fixture própria"* | before the archive, the fixture principal reaches the route its permission gates → **2xx** | after the archive, a **freshly reissued** token for the same principal no longer carries the pair, and the same call answers **403** | **asked** |

**Rows 9 and 10 share one fixture, built by the suite and touching nothing seeded.** The chain
is: insert a catalog pair of the suite's own → create a role that grants only it → create a user
holding only that role → rotate its password → sign in → prove the reach → archive the pair →
prove the `role_permissions` row survives → sign in AGAIN → prove the reach is gone. **The
seeded `*:*` row and the bootstrap admin are never touched**, which is why this chain can sit
anywhere in the lane instead of having to be last and irreversible.

**Nothing in §1b is UNPROVEN this round** — every question this plan raised was answered on
2026-09-07 before it was written.

---

## 2. Data hygiene — inherited unchanged from `tenant-contract` §2

Same throwaway database (`authcore_qa`), same suite-owned `qa/microservice.qa.yaml` selected by
`OMNICORE_CONFIG_PATH` + `APP_PROFILE=qa`, same drop-and-create reset before every boot, same
port `:8099`, same signing key, same residue promise. **No new decision is asked here** — the
posture already approved covers this entity too, and this round changes nothing about it.

Two things this entity adds to the picture, stated because they are what a reader would check:

- **A permission pair is reusable after archive** (§1b row 1), so unlike a tenant handle it burns
  nothing permanently. The throwaway database is still right — the reset is what makes E6's
  exact-count assertions legitimate — but it is now belt-and-braces rather than load-bearing.
- **A suite invariant, stated so it cannot be violated by accident: no lane may archive a SEEDED
  catalog row.** Archiving `permission:read` would revoke the gate every later case depends on,
  and archiving `*:*` would revoke principal A entirely. Every archive case in this round
  operates on a pair the suite itself inserted, and §1b row 10's revocation chain uses its own
  principal for exactly that reason.

---

## 3. Security — never `N/A`

The 401 half, the framework's appended public surfaces, the introspection bypass and the tenant
gate are **inherited from `tenant-contract` §3** and not repeated (§0b). What follows is what is
Permission-specific, and it all lands in the existing `qa/security.sh` under a new `S4.x` block.

### 3a — Inherited. See `specs/qa/tenant-contract/plan.md` §3a.

### 3b — The public-route half: **direction 2 for the new routes**

`auth.publicRoutes` is byte-identical to the dev profile's and names no `/permissions` path.
Direction 1 (every declared entry answers tokenless) is inherited. What this round adds is the
direction that catches an entry widened past its intent, now for a surface it did not cover:

- `GET /permissions` · `GET /permissions/{id}` · `POST /permissions` · `PATCH /permissions/{id}` ·
  `PATCH /permissions/{id}/archive`, each **tokenless → 401**.
- The same for the five GraphQL fields, tokenless.

### 3c — The 403 half. The token source is unchanged: **this service issues its own.**

Principals A (`*:*`) and B (`tenant:read` only) already exist in `qa/run.sh` and are reused as
they are. The layer-1 families:

| Layer | Case | Expected |
|---|---|---|
| **1 — `RequirePermission`** | principal **B** (`tenant:read` only) → each of the five REST verbs | **403** `MissingPermissionNotification` |
| | **the complement** — principal **A** → each of the five | **2xx**: a gate that refuses everyone is also broken |
| | the same ten rows on **GraphQL**, where the key rides `errors[].extensions` — a route gated on REST is not thereby gated on GraphQL | |
| **2 — identity-derived `BuildRules` / `ToCriteria`** | — | **`N/A`, by decision and not by omission**: `authz.dataAccess: anyone-with-permission` (§10 of the spec — the catalog is global, has no `tenant_id` and no owner), both `ToCriteria` implementations return the criteria unchanged, `BuildRules` reads no principal field, and no `Restrict` is declared. There is no per-row or per-field boundary on Permission to assert |
| **3 — tenant scoping** | the tenant gate itself | inherited (§0b) |
| | a cross-tenant READ | **not a leak here — a PROMISE.** The catalog is global by design, so principal **D** (a different tenant, `permission:read`) must see exactly what principal **C** sees: same `totalCount`, same first page, on REST and on GraphQL. This is §3c(ii), and it is the inverse assertion of the one every scoped entity makes |

**⚠️ Two proposals in this section, both cheap, both for the maintainer to take or drop at this
gate.** Neither is assumed:

- **(i) Principal C — a caller holding exactly `permission:read`.** Principals A and B prove
  "everything" and "nothing". Neither can see the failure where all five permission routes are
  gated on the *same* literal: B is refused by all five either way, and A passes all five either
  way. C — read granted, writes not — is the only principal that separates them, and it answers
  **200** on the two reads and **403** on the three writes. Cost: one role + one user + one
  password rotation in `qa/run.sh`'s principal section, mirroring principal B exactly.
- **(ii) The catalog is the same for everyone.** The global-catalog decision (§10) is asserted
  today only by the *absence* of a filter in `ToCriteria`. A positive case would be: a principal
  in a **second** tenant, holding `permission:read`, sees the same `totalCount` and the same
  first page as a principal in `master`. Cost: one extra tenant + role + user + rotation. Drop
  it and the plan records the global catalog as proven only by code inspection, printed as SKIP.

**Both were taken** (maintainer, 2026-09-07). Principal **C** (`permission:read` only, in the
`master` tenant) and principal **D** (`permission:read` only, in a SECOND tenant the suite
creates) are built in `qa/run.sh`'s principal section, each mirroring principal B: a role, a
user, a password rotation, a sign-in. Both are constructible through the API because
`InsertUserRequest` and `InsertRoleRequest` each carry an optional `tenantID` that defaults to
the caller's claim — and principal A, holding `*:*`, crosses the scope to set it.

### The Permission scope question, raised and closed at this gate

The gate surfaced a real ambiguity and it is recorded rather than smoothed over. Commit
`33eb883` declares `authz.scopes: [{field: ID, from: tenant, applies: [read]}]` on **Tenant** and
migrates five further entities onto `dataAccess: scoped` — a genuine isolation fix, and the
reason §3c(ii) was worth asking at all. It did **not** touch Permission, and the maintainer
confirmed on 2026-09-07 that this was deliberate: *"Global mesmo — era o Tenant que eu
corrigi."*

Verified on disk before this line was written, because an expectation built on a misread is
worth nothing:

- `specs/omnicore-gen/permission.omnicore.yaml:478` — still `dataAccess: anyone-with-permission`;
  Permission is the ONE of the seven aggregates that is not `scoped`;
- `find_permissions_by_params_query.go:50` and `find_permission_by_id_query.go:54` — both still
  `return q.Criteria, nil`, no forced `Filter`;
- `migrations/postgres/0002_permission_manual.up.sql` — the `permissions` table has **no
  `tenant_id` column**, so tenant scoping is not expressible here without a migration and a
  supersession of the approved model (§B, §10, and the README's "what is scoped to a tenant and
  what is not"). That would be `/omnicore:evolve-entity` work, not QA's.

So §3c(ii) is asserted in its ORIGINAL direction and becomes the round's one design-proving
case: **principal D, in a different tenant, must see exactly what principal C sees** — same
`totalCount`, same first page. If the catalog is ever scoped, this case is the one that turns
RED first, which is precisely what it is for.

### 3d — `auth.mode` posture

`mode: jwt`, `authorization.enabled: true`. Not an excuse and not applicable as one.

### 3e — Coverage reported, not asserted in prose

Layer 2, cross-tenant row isolation (both `N/A` by decision) and whatever of (i)/(ii) is dropped
are printed in the SKIP column of the report with the reason, not merely written here.

---

## 4. Out of scope, named plainly

- **The seeded catalog as a contract** — **the maintainer's explicit decision, 2026-09-07:**
  *"Nenhum — a seed não é contrato desta rodada."* Neither the 39-row count nor the
  "every `RequirePermission` literal has an active row" coverage is asserted. Recorded here so a
  future round does not read the absence as an oversight: both are cheap to add later, and the
  second is the one that would catch a route gated on a pair nobody seeded.
- **Load and performance**, and any UI.
- **gRPC** — no `transport:` block, no transport tag, no procedures mounted.
- **Tabular exports** — `surfaces` declares no CSV/XLSX for Permission.
- **Integration events** — no broker in this posture; nothing is published. Not `⚠️ OPEN`: there
  is no delivery half to defer.
- **The `Role ↔ Permission` join contract** — `role_permissions` is owned by Role's aggregate and
  belongs to Role's own QA round. This round touches it in exactly two places, both read-only or
  fixture-scoped: §1b row 9's SQL assertion, and §1b row 10's fixture.
- **The read-join republishing of this catalog's `archived_at`** (`specs/scaffold-entity/role/spec.md` §2)
  — a Role-side promise, asserted from the Role side.
- **The other five aggregates' own contracts.**

### In scope, extending the existing lane: the audit trail

`qa/audit.sh` already proves the framework's in-TX `audit_events` promise for Tenant. The same
promise applies here and is provable by SQL on this posture, so this round extends that lane
rather than starting a new one (cases `A15+`):

- one row per `insert` / `update` / `archive`, with `entity_type = 'Permission'` and
  `aggregate_id` = the row's id;
- **no unarchive row exists to assert** — the verb does not exist, and its absence from the audit
  timeline is itself asserted;
- `actor` = the acting principal's `sub`; `tenant_id` = the actor's tenant claim (the actor is
  tenant-scoped even though the catalog is not);
- the `changes` block on the description update that persisted a change.

---

## 5. Runner contract — the SAME `qa/run.sh`, extended

**Still exactly one entry point.** This round adds two lanes and edits four existing files; the
lane list is where the addition becomes real.

```
qa/
├── run.sh                     ← EDITED: LANES gains two entries; the principal section gains C (and D, if 3c(ii) is taken)
├── lib/common.sh              ← EDITED: a permission_body / new_permission fixture pair, mirroring tenant_body / new_tenant
├── tenant.sh                  ← untouched
├── tenant_graphql.sh          ← untouched
├── permission.sh              ← NEW: E1–E9, REST
├── permission_graphql.sh      ← NEW: E10 + E1/E3/E4/E7/E8 in the GraphQL idiom
├── domain.sh                  ← EDITED: §1b rows 1–10 appended as P1–P10 (tenant's rows stay R1–R11)
├── security.sh                ← EDITED: §3b and §3c appended as S4.x (tenant's stay S3.x)
├── audit.sh                   ← EDITED: the Permission family appended as A15+
├── microservice.qa.yaml       ← untouched
└── microservice.qa-key.yaml   ← untouched
```

- **Lane list becomes**: `tenant tenant_graphql permission permission_graphql domain security audit`
  — the two new lanes sit beside their tenant twins, and the shared lanes stay last so a
  Permission domain rule and a Tenant one are read together.
  `./qa/run.sh permission permission_graphql` runs just this round's new surface work, on the same
  runner.
- **Everything else is inherited verbatim**: root resolution (`cd "$(dirname "$0")/.."` first in
  every lane), fail-fast by default with `--all` for the sweep, non-zero exit per lane,
  per-run namespacing of every temp file / log / binary / port, the boot sequence, SIGTERM-only
  shutdown with a wait on the drain, `Accept-Language: en-US` on every request, and a failed
  assertion printing the real response body.
- **CLAUDE.md rule 1 applies to the four EDITED files** as much as to the two new ones: they are
  named here, before anything is touched, and this gate is their approval.

---

## 6. Report contract — unchanged

`qa/qa-report.md`, rendered live and rewritten in full after every lane, same header / matrix /
failures / footer / abort-trap / SKIP-column rules as `tenant-contract` §6. The only change is
that the matrix grows from five rows to seven. A lane that never ran still prints `—`.

---

## Approval — closed 2026-09-07

| Gate item | Answer |
|---|---|
| The plan as a whole, including the five EDITED files of §5 (`CLAUDE.md` rule 1) | **Approved** — *"Aprovado — pode gerar e executar"* |
| §3c (i) — principal C, holding exactly `permission:read` | **Taken** |
| §3c (ii) — a second-tenant principal proving the catalog is global | **Taken**, in its original direction; the Permission-vs-Tenant scope ambiguity closed above |
| §1b rows 9 and 10 — the revocation chain, own fixture | **Taken** — *"Sim, com fixture própria"* |
| §1b row 9's premise — an archived permission keeps its grants | **Confirmed as correct** — *"Está certo"* |
| §4 — the seeded catalog as a contract | **Out of scope this round** — *"Nenhum — a seed não é contrato desta rodada"* |
| §1b ranking | Rows 1–4 all **critical** |
