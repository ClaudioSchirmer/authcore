# QA contract suite — `tenant-contract`

- **Status:** APPROVED (maintainer, 2026-09-03)
  Approved at the Phase 1 gate; §8 records the four decisions.
- **Scope:** the `Tenant` aggregate, surfaces REST + GraphQL, plus the security lane
  that guards them.
- **Pin:** omnicore **v0.72.1** (`go list -m github.com/ClaudioSchirmer/omnicore`).
- **Profile under test:** `dev` config shape, engine tag `postgres`, **no** transport tag.
- **Plan lives here; the runnable suite lives at `qa/` in the project root.** Neither is
  ever added to `.gitignore`.

Everything below is DERIVED — from `specs/omnicore-gen/tenant.omnicore.yaml`, from
`specs/scaffold-entity/tenant/spec.md` (Status: APPROVED), from the code actually
mounted, and from the pin's own docs (`status-mapping`, `auto-handlers`,
`auto-query-handlers`, `auth-middleware`, `authz-seams`, `graphql`, `relational-view`).
Where a promise could not be derived it is named `N/A — <why>` or `⚠️ OPEN`, never
guessed. **No expectation in this file was read off a live response**; the earlier
`qa/` tree was deleted by the maintainer in `cb78ed7` and is deliberately not consulted.

---

## 0. Surface inventory — what the service DECLARES

**Storage** — flat table `tenants`; managed `revision / created_at / updated_at /
deleted_at`. `delete.root: soft` — no hard-delete verb exists anywhere.

**Wire fields** — 4 written + 2 managed. No composite VO, no children, no siblings, no
SharedBase role, no read join.

| wire | VO | rule |
|---|---|---|
| `name` | `DisplayName` (manual) | 2–120 runes, anti-junk, no word count |
| `workspace` | `TenantWorkspace` (manual) | DNS label, reserved list, immutable, unique `scope: all` |
| `description` | `Description` (manual) | 15–500 runes, ≥2 words, ≥5 distinct, ≥1 vowel |
| `status` | `TenantStatus` (enum) | `trial \| active \| suspended`, mandatory on insert |
| `createdAt` / `updatedAt` | framework-managed | `read.managed`, read-only |

`deletedAt` is **not** projected — archived state is reached only through
`?includeArchived`.

**Modes** — `[display, insert, update, archive, unarchive]`. Update shape `patch`,
`patchExcludes: [Workspace]`.

**Routes mounted** (`internal/web/tenant_routes.go`), each behind `RequirePermission`:

| verb | route | success | permission |
|---|---|---|---|
| insert | `POST /tenants` | 201 + body | `tenant:insert` |
| patch | `PATCH /tenants/:id` | 200 + body | `tenant:update` |
| archive | `PATCH /tenants/:id/archive` | 204 | `tenant:archive` |
| unarchive | `PATCH /tenants/:id/unarchive` | 204 | `tenant:archive` |
| list | `GET /tenants` | 200 + page envelope | `tenant:read` |
| by-id | `GET /tenants/:id` | 200 + document | `tenant:read` |

GraphQL mirrors all six (`tenants`, `tenant`, `createTenant`, `patchTenant`,
`archiveTenant`, `unarchiveTenant`) — **same handlers, same permissions**, no second
implementation. Cross-checked against `GET /openapi.json` at run time (case `X1`); a
source-vs-openapi disagreement is reported as a FINDING, never reconciled silently.

**Read backing** — `RelationalView("tenants", loader)`, `read.backing: relational`.
Therefore **read-your-writes**: every read-back in this suite is IMMEDIATE, and a case
that only passes after a retry is itself RED. No CDC, no poll, no drain.

**Declared read controls** — `FindTenantsRequest` declares `first, last, after, before,
orderBy, fields, onlyTotal, includeArchived`. **`search` is NOT declared.**
`FindTenantByIDRequest` declares `includeArchived` and nothing else.

| field | filter operators | orderable |
|---|---|---|
| `name` | eq, in, startswith, contains, istartswith, icontains | asc, desc |
| `workspace` | eq, in, startswith, istartswith | asc, desc |
| `description` | contains, icontains | — |
| `status` | eq, in | — |
| `createdAt` | eq, gte, lte, gt, lt | asc, desc |
| `updatedAt` | eq, gte, lte, gt, lt | — |

Page ceiling: no `query:` block in the yaml and no per-view override →
`bootstrap.FrameworkDefaultMaxLimit = 100` (`bootstrap/config.go:896`).

**Read joins** — N/A: the repository declares none. Neither a served joined field to
assert nor a rules-only one to prove absent.

**Infra posture** — Postgres 17 only (`devops/docker-compose.yml`, one container). No
Mongo, no broker, no CDC relay, no `integration_events` table, no export surface, no
gRPC transport.

**Security posture** — read from the whole `auth:` block:
- `auth.mode: jwt`; `authorization.enabled: true`; `authorization.tenant.required: true`.
- `auth.jwt`: `algorithms: [RS256]`, `issuer` and `audience` from
  `${AUTH_SELF_URL:http://localhost:8080}` / `${AUTH_AUDIENCE:authcore}`, keys via
  `jwksUrl` (self-referential), no `leewaySeconds` declared → framework default.
- `auth.issuer.enabled: true` — **this service mints its own tokens.** That answers §3's
  hardest question outright.
- `auth.publicRoutes` (exact `METHOD /path`): `GET /livez`, `GET /readyz`,
  `POST /auth/user/token`, `POST /auth/user/token/refresh`, `POST /auth/client/token`.
  The probes are here by the project's own opt-in — they are NOT framework-public.
  Framework-appended at boot and needing no entry: `GET /docs`, `GET /openapi.json`,
  `GET /graphql/ui` (playground on), `GET /` (rootRedirect on), and the JWKS document.
  `graphql.introspection: true` → the request-shaped introspection bypass is live.
- `auth.externalValidator` — not set. A locally-valid token is accepted; nothing else
  can refuse it.
- **Identity-derived rules on Tenant: NONE.** `authz.dataAccess: anyone-with-permission`,
  and both `ToCriteria` implementations return the criteria unchanged — no tenant filter,
  no `Restrict`. Verified at `internal/application/queries/find_tenants_by_params_query.go:48`
  and `find_tenant_by_id_query.go:51`. So authz layers 2 and 3 are structurally absent on
  this aggregate; layer 1 is the whole gate, and §3 says so rather than implying coverage.

**Existing `qa/`** — none. `cb78ed7 "rebuild qa"` removed the previous tree; this round
builds from the live sources, not from what was deleted.

---

## 1. Coverage matrix — the framework's promises

Every family is a real case in `qa/tenant.sh` unless marked `N/A`. Assertions are
always **status + notification KEY (+ field where the pin names one)**, never prose.
Every request pins `Accept-Language: en`.

### B — Happy path, one per served verb (REST)

`B1` insert → **201** + body mirroring the STORED entity · `B2` by-id → **200** ·
`B3` list filtered to that record → **200**, exactly 1 row · `B4` patch
`name`+`description` → **200**, new values, `workspace` untouched · `B5` archive →
**204** · `B6` unarchive → **204**. Every read-back is IMMEDIATE.

`B7` insert with `status: "suspended"` → **201**. Spec §7 rule 11: on insert ANY member
is accepted; rule 12 gates transitions, not creation.

### C — Golden-record round-trip

One record exercising **every declared wire field**, written then read back
field-by-field on REST by-id, on the REST listing row, and on the GraphQL node.
`createdAt`/`updatedAt` present and RFC3339-parseable; `deletedAt` **absent** from every
body on every surface. This is the family that catches a field silently dropped from a
DTO or a projection.

### D — Validation 422 (one per rule shape, asserting the KEY)

| case | request | expected key |
|---|---|---|
| `D1` | `name: "aaaa"` (run of 4 identical) | `InvalidDisplayNameNotification` |
| `D2` | `name: "3M"` | **201** — positive control, `distinct ≥ min(3,len)` admits it |
| `D3` | `workspace: "Acme Corp"` | `InvalidTenantWorkspaceNotification` |
| `D4` | `description: "short"` | `InvalidDescriptionNotification` |
| `D5` | `status: "frozen"` | `UnknownTenantStatusNotification` |
| `D6` | `status: ""` | `UnknownTenantStatusNotification` — the enum's unknown member answers the empty value; no separate required rule (spec §7 rule 11) |
| `D7` | accented + non-Latin description | **201** — the vowel test is Unicode, spec §7 |

All **422**, `semantic: "Validation"`.

### E — 409, both flavors

`E1` duplicate `workspace` on insert → **409** `TenantWorkspaceAlreadyExistsNotification`,
`semantic: "Conflict"`, field `workspace`.
`E2` a patch that does not move `workspace` does not self-collide (`excludeSelf: true`)
→ **200**.

`SemanticStateConflict` — **N/A**: the entity declares no state-conflict notification,
and the framework's two (`EntityIsNotActiveNotification`,
`ConcurrentModificationNotification`) are unreachable through this surface — no verb
carries a revision precondition, and archive/unarchive misuse resolves as 404 (`F6`/`F7`),
which is what `LoadForWrite`/`LoadArchivedForWrite` produce.

### F — Archive round-trip (regime: kept-but-hidden; `DeleteOnArchive` not declared)

`F1` archive → by-id → **404** `RecordNotFoundNotification` ·
`F2` by-id `?includeArchived=true` → **200** ·
`F3` listing hides it, `?includeArchived=true` reveals it ·
`F4` unarchive → visible again ·
`F5` archive an already-archived tenant → **404** ·
`F6` unarchive an active tenant → **404**.

Child stamp-scoped unarchive — **N/A**: flat aggregate, no child table.

### G — Read vocabulary and the pagination envelope

`G1` eq · `G2` `?status.in=active,trial` · `G3` `?workspace.startswith=` and
`.istartswith=` · `G4` `?name.contains=` / `.icontains=` / `?description.icontains=` ·
`G5` `?createdAt.gte=` + `.lte=` · `G6` `?updatedAt.gte=` (filterable, not orderable) ·
`G7` `?orderBy=name` / `-name` / `workspace` / `createdAt` · `G8` `?fields=name,workspace`
→ those keys present, `description` absent · `G9` `?onlyTotal=true` → `totalCount` only,
no `data`, no cursors · `G10` `?last=2` alone → the TAIL window ·
`G11` **envelope truthfulness**: with a known seeded count and `?first=2` —
`totalCount`, `hasNextPage`, `hasPreviousPage`, `startCursor`, `endCursor`; echo
`endCursor` into `?after=` → page 2 DISJOINT from page 1, `hasPreviousPage == true`;
walk back with `?before=` → page 1 again · `G12` `?includeArchived=true` raises
`totalCount` by exactly the number of archived records.

`?search=` — **N/A as a capability case**: the DTO does not declare it, so the opt-in
gate answers first (`H3`) before any engine sees it.

### H — Rejected reads: the whole typed-400 guard family

All **400**, key `SchemaViolationNotification` unless stated.

| case | request | field named |
|---|---|---|
| `H1` | `?bogus=1` | `bogus` |
| `H2` | `?workspace.contains=x` (operator outside its allowlist) | `workspace` |
| `H3` | `?search=acme` (reserved control the DTO never declared) | `search` |
| `H4` | `?fields=bogus` | `fields[bogus]` |
| `H5` | `?orderBy=status` (filterable, never orderable) | `orderBy[status]` |
| `H6` | `?orderBy=updatedAt` (same) | `orderBy[updatedAt]` |
| `H7` | `?orderBy=bogus` | `orderBy[bogus]` |
| `H8` | `?orderBy=-workspace` | **200** — positive control, `desc` IS declared |
| `H9` | `?first=101` | `LimitExceededNotification`, effective max `100` |
| `H10` | `?first=0` | `first` |
| `H11` | `?first=2&last=2` · `?first=2&before=X` · `?last=2&after=X` · `?after=X&before=Y` | the backward-side key |
| `H12` | `?onlyTotal=true` with `&first=` / `&orderBy=` / `&fields=` / `&after=` | `onlyTotal[<conflict>]` |
| `H13` | `?onlyTotal=true` + a filter, and + `&includeArchived=true` | **200** — counting a filtered subset is the point |
| `H14` | `?after=not-a-cursor` | `after` |
| `H15` | cursor issued with no `orderBy`, replayed with `&orderBy=name` | the structural check |
| `H16` | same cursor replayed with `&includeArchived=true` | the context-hash check |
| `H17` | `?includeArchived=1` · `?onlyTotal=` (empty) | booleans take exactly `true`/`false` |
| `H18` | by-id gate: `?onlyTotal=false` · `?fields=name` | presence gates an undeclared control |
| `H19` | by-id `?includeArchived=true` | **200** — positive control for the one it declares |
| `L1` | `?createdAt.gte=not-a-date` | **400** `InvalidFilterValueNotification` — a value outside the temporal leaf's kind |
| `L2` | `?createdAt=not-a-date` | same |

`UnsupportedCapabilityNotification` — **N/A for this entity**: raised when a read engine
is asked for what the store cannot serve (`?search=`, a filter/sort on a 1:N child leg).
Tenant is FLAT and `search` is undeclared, so the DTO gate answers first with a
`SchemaViolationNotification`. Asserting the other key would assert a promise this
entity does not make.

### I — Routing, not-found, and the by-id ADDRESS contract (pin ≥ v0.70.0)

`I1` `GET /tenants/<unused uuid>` → **404** `RecordNotFoundNotification` ·
`I2` `DELETE /tenants/:id` → **405** `MethodNotAllowedNotification` — proves no hard
delete exists · `I3` `POST /tenants/:id` → **405** ·
`I4` `GET /tenants/:id/archive` → **405** (the path IS registered, under PATCH) ·
`I5` `GET /tenants/<id>/purge` (matches no route) → **404** `RouteNotFoundNotification`.

**The by-id address family, split by VERB, on both surfaces** — the promise
`status-mapping` makes at this pin:

| case | request | expected |
|---|---|---|
| `I6` | `GET /tenants/not-a-uuid` | **404** `UnknownIDAddressNotification` |
| `I7` | `PATCH /tenants/not-a-uuid` | **400** `MalformedIDNotification` |
| `I8` | `PATCH /tenants/not-a-uuid/archive` | **400** `MalformedIDNotification` |
| `I9` | `PATCH /tenants/not-a-uuid/unarchive` | **400** `MalformedIDNotification` |
| `I10` | `{ tenant(id: "not-a-uuid") }` | typed `UnknownIDAddressNotification` |
| `I11` | `mutation { archiveTenant(id: "not-a-uuid") }` | typed `MalformedIDNotification` |

Mode-missing-with-route-mounted **403** — **N/A**: every declared mode has its route
mounted and no route is mounted for an undeclared mode, so that arm of the three-way
split is unreachable. The reachable 403 is the permission gate, which `qa/security.sh`
owns.

### J — GraphQL (handler invariance)

`J1` `tenants(first: 2)` → `edges { node cursor } pageInfo totalCount`, node equal to the
REST listing row · `J2` `tenant(id:)` equals the REST by-id document field for field ·
`J3` `createTenant` → visible over **REST** immediately (one write, two surfaces) ·
`J4` `patchTenant` → same · `J5` `archiveTenant`/`unarchiveTenant` → payload
`{ success, id }`, effect confirmed over REST · `J6` `tenant(id: <unused>)` → HTTP 200
with `errors[0].extensions.notificationKey == "RecordNotFoundNotification"` (the GraphQL
idiom — the REST envelope is NOT asserted cross-surface) · `J7` duplicate workspace via
`createTenant` → `extensions.notificationKey == "TenantWorkspaceAlreadyExistsNotification"`,
`extensions.semantic == "Conflict"` · `J8` the DTO opt-in gate in GraphQL idiom:
`tenants(search: "x")` is an **unknown argument**, a gqlparser validation error, not the
REST 400 envelope · `J9` `where:`/`orderBy:` return the same set and order as their REST
twins · `J10` `__typename` beside every selection answers identically (pin ≥ v0.72.1).

gRPC — **N/A**: no transport wired. Exports — **N/A**: none declared.

### X — Suite meta

`X1` `GET /openapi.json` enumerates exactly the six tenant routes above, each carrying
its declared permission. A mismatch is a FINDING, not a silent reconciliation.

---

## 1b. Domain expectations — the business oracle

The rows below are the only ones in this plan not derived from the pin's docs, and the
only ones that can fail for a reason the framework never had an opinion about. They live
in their own lane, `qa/domain.sh`. Ranked by the cost the maintainer named.

| # | rule, as stated | source | POSITIVE case | NEGATIVE case | what a fail means |
|---|---|---|---|---|---|
| **R1** 🔴 | "Unarchiving returns the tenant `suspended`. Nothing sets it back… restoring a tenant must never silently resume a billable, functioning account." | `spec.md` §7 rule 13 + consequence 2; `rules.manual: archive-forces-suspended`; **maintainer: CRITICAL** | insert `active` → archive → read `?includeArchived=true` → `status == "suspended"` **on the row**, not merely in an audit line | unarchive → read → `status` is STILL `"suspended"`; a `"active"` here is the failure | a restored tenant silently resumed billing |
| **R2** 🔴 | "An archived remnant MUST keep blocking the handle, because it is what URLs, logs and support conversations carry." `scope: all`, never active-only. | `tenant.omnicore.yaml` `fields[].unique.scope: all`; `spec.md` §7 rule 5; **maintainer: CRITICAL** | a fresh workspace inserts → **201** | insert workspace `W`, archive that tenant, insert `W` again → **409** `TenantWorkspaceAlreadyExistsNotification` | a new tenant inherits a retired tenant's URLs, logs and support history |
| **R3** 🔴 | The reserved handle list — platform routes and phishing-prone words. Five of them are routes this service serves today. | `spec.md` §7 rule 4 + the reserved list; **maintainer: CRITICAL** | `workspace: "acme-comercio"` → **201** | each of `admin`, `api`, `docs`, `graphql`, `livez`, `readyz`, `tenants`, `www`, `root` → **422** `ReservedTenantWorkspaceNotification` | a tenant claims a handle that collides with a live platform route |
| **R4** | "A trial is a beginning — no tenant returns to it." Allowed: `trial→active`, `trial→suspended`, `active→suspended`, `suspended→active`, and any no-op. | `spec.md` §7 rule 12; `rules.list: status-transition` | `trial→active`, `active→suspended`, `suspended→active` each → **200**; and a no-op (patch the same value) → **200** | `active→trial` and `suspended→trial` → **422** `InvalidTenantStatusTransitionNotification` | a churned tenant is walked back into a trial it already consumed |
| **R5** | "The description must differ from both Name and Workspace under a normalized comparison — case-folded, whitespace- and hyphen-collapsed — which catches the pasted-name description." | `rules.manual: description-differs-from-name-and-workspace`; `spec.md` §7 rule 10 | a genuinely distinct description → **201** | description == name, and description == workspace with different case/spacing/hyphens (`"Acme  Comercio"` vs `"acme-comercio"`) → **422** `TenantDescriptionMustDifferNotification` | the normalization is not doing the work it was written for |
| **R6** | "No silent normalization. `" Acme "` and `"ACME-CORP"` are refused, never quietly repaired." | `spec.md` §7 rule 14 | `"acme-corp"` → **201** and reads back byte-identical | `"ACME-CORP"` and `" acme-corp "` → **422**; and after any accepted insert, the read-back equals the bytes sent | the caller believes they own a handle the server silently rewrote |
| **R7** | "It is one-way. Setting a tenant `active` does NOT unarchive it." | `spec.md` §7 rule 13 consequence 3 | — | archive a tenant, then `PATCH status: "active"` → the tenant is STILL archived (invisible without `?includeArchived`) | archiving and commercial state have quietly been fused |
| **R8** | **DECIDED by the maintainer (this gate):** a `workspace` key in a PATCH body changes nothing and is not an error. Immutability here is STRUCTURAL — the field is not a member of `PatchTenantRequest`. | maintainer, asked at this gate | `PATCH {"name": …, "workspace": "hijack"}` → **200**, and the read-back `workspace` is UNCHANGED | — (the maintainer decided the 200 is correct) | the handle moved through a door the DTO does not open |

**Stated, not silently skipped:** `TenantWorkspaceIsImmutableNotification` is declared in
the model and is **unreachable through REST and GraphQL**, because `patchExcludes:
[Workspace]` removes the field from the update DTO entirely — the belt-and-braces layer
sits behind a door no surface can open. No case asserts it. That is a fact about the
model, recorded here rather than discovered later.

**Nothing is listed UNPROVEN in this round** — every row above has both halves.

---

## 2. Data hygiene — DECIDED at this gate: a dedicated throwaway database

The suite asserts exact totals (`G9`, `G11`, `G12`) so it needs a clean, countable
baseline, and it must sign in — which on a fresh database means rotating the bootstrap
password, a one-way change. Writing that into `authcore_db` is not acceptable.

**`qa/microservice.qa.yaml`** — generated by this skill, living under `qa/`, selected
through `OMNICORE_CONFIG_PATH`. It is the dev profile with two changes and nothing else:
the DSN points at `authcore_qa_db`, and the HTTP address is `${QA_HTTP_ADDR::8099}` so
the lane never collides with a dev server on 8080. **The project's own
`microservice.dev.yaml` is never touched** — that is what makes a "dedicated profile"
respect this skill's never-edit-the-project-yaml rule.

**Reset per run**, and it is a precondition rather than a hope:
`docker compose exec -T postgres dropdb --if-exists authcore_qa_db` then `createdb`, then
`migrations.autoRun` rebuilds the schema AND the bootstrap seed. There is no Mongo and no
CDC, so reset is that one step and nothing races it — no view collection to clear, no
drain to wait out.

**Residue:** one throwaway database, dropped and recreated on the next run. Nothing is
ever written to `authcore_db`.
**Cost, stated:** the suite must be able to `docker compose exec` the bench container.
Without Docker reachable, the lane fails as a precondition — loudly, never as a skip.

---

## 3. Security — `qa/security.sh`, its own lane

**Where a valid token comes from: the service itself.** `auth.issuer.enabled: true`, so
the suite signs in through `POST /auth/user/token` with the credentials the tracked seed
migration (`0012_bootstrap_seed_manual.up.sql`) puts in the database —
`admin@authcore.local` / `admin`, documented in that file and in `README.md:147`.
**Nothing is invented, no keypair is minted, no credential is hardcoded that the
repository does not already hold.**

### 3a — The 401 half (needs no valid token)

Every case is built from a token meant to FAIL. Keys from `auth-middleware` at the pin —
and the **key** is asserted, not just the status, because the pin splits expired from
invalid precisely so a client can branch on refresh-vs-reauthenticate.

| case | request | expected key |
|---|---|---|
| `S1` | no `Authorization` header | `MissingAuthorizationNotification` |
| `S2` | `Authorization: Token abc` (wrong scheme) | `MissingAuthorizationNotification` |
| `S3` | `Authorization: Bearer` (empty value) | `MissingAuthorizationNotification` |
| `S4` | `Authorization: bearer <valid>` (lowercase scheme) | **200** — the match is case-insensitive; a PASS case, not a reject |
| `S5` | `Bearer not-a-jwt` | `InvalidTokenNotification` |
| `S6` | well-formed JWT signed with a foreign RSA key | `InvalidTokenNotification` |
| `S7` | wrong `iss` | `InvalidTokenNotification` |
| `S8` | `aud` missing the configured audience | `InvalidTokenNotification` |
| `S9` | **HS256-signed token where the allowlist is `[RS256]`** — the algorithm-confusion guard | `InvalidTokenNotification` |
| `S10` | `exp` in the past beyond leeway | `ExpiredTokenNotification` |

All **401**. `S6`–`S10` need a locally-minted JWT; the suite generates a throwaway RSA
keypair at run time for the forged ones. That invents no credential of anybody's — a
token meant to be REFUSED needs no secret, and asserting the refusal is the point.

### 3b — The public-route half, BOTH directions

`S11` every declared public route answers tokenless: `GET /livez`, `GET /readyz`,
`POST /auth/user/token` (reachable — a 422/401 on credentials still proves the route was
not gated), `POST /auth/user/token/refresh`, `POST /auth/client/token`.
`S12` **a route that is NOT declared answers 401 tokenless** — `GET /tenants`. This is the
direction that catches a `publicRoutes` entry widened past its intent.
`S13` exactness, because matching is not prefix-based: `POST /livez` (same path, other
method) → 401 or 405, never a tokenless 200; and `GET /livez/x` (sibling sharing a
prefix) → not tokenless-served.
`S14` the framework's own appended surfaces reachable tokenless: `GET /docs`,
`GET /openapi.json`, `GET /graphql/ui`, `GET /`, and the JWKS document.
`S15` **the GraphQL introspection bypass and its edges** (`introspection: true`):
an introspection-only document passes tokenless; and each of — a data field beside
`__schema`, a decoy introspection operation next to a real one, a root fragment spread,
and a mutation — does **not**. A boundary that holds only for documents no real client
sends is not a boundary.

### 3c — The 403 half: layer 1, which is the whole gate here

`S16` the bootstrap admin's FIRST token carries `user:change-password` and nothing else
(`EffectivePermissions` → `restrictToPasswordChange`,
`internal/application/commands/handlers/utils/authentication.go:182`) — `mustChangePassword == true`.
`S17` `GET /tenants` with that restricted token → **403** `MissingPermissionNotification`,
field `permission`, value `tenant:read`.
`S18` rotate the password, sign in again → a token carrying `*:*`,
`mustChangePassword == false`. **The complement**: the same `GET /tenants` now → **2xx**.
A gate that refuses everyone is also broken, so both halves are asserted.
`S19` the same 403 per verb on the write routes (`tenant:insert`, `tenant:update`,
`tenant:archive`) using the restricted token.
`S20` **per surface, because a route gated on REST is not thereby gated on GraphQL**:
`tenants` / `createTenant` under the restricted token → the typed 403 in the GraphQL
idiom.

**Layers 2 and 3 are structurally absent on this aggregate — stated, not skipped.**
`authz.dataAccess: anyone-with-permission`; both `ToCriteria` methods return the criteria
unchanged, with no tenant filter and no `Restrict`; `BuildRules` reads no principal
field. There is no owner-check to prove and no column to find absent. Anyone holding
`tenant:read` sees every row, by design (`spec.md` §B Q4).

**The middleware tenant-claim gate IS covered** (`S21`/`S21b`). `tenant.required: true`
makes an empty `TenantID()` a 403 `TenantMissingNotification` — the only non-401 outcome
the middleware itself produces. Reaching it needs a token that is valid in every respect
except the missing claim, which is the same technique `S7`–`S10` already use: signed by the
bench's own key, correct `iss`/`aud`/`exp`, differing from a good token in ONE thing. No
keypair of its own, no credential invented — like every forged token in this lane it exists
to be REFUSED. `S21b` sends the same token WITH the claim and asserts 200, because without
that pair a service refusing every token would pass `S21` for the wrong reason.

*(An earlier draft of this section deferred it, reasoning that the service's own issuer
never mints a claimless token. True and irrelevant: the suite does not need the issuer to
mint it.)*

---

## 4. Out of scope, named plainly

- **Load / performance / concurrency** — not covered. The wire carries no revision
  precondition on this entity, so there is no race case to write.
- **UI** — `/docs` and `/graphql/ui` are asserted reachable, never rendered.
- **Integration events — N/A, not skipped:** no `transport:` block, nothing published,
  no `integration_events` table. There is no in-TX outbox row to assert and no relay to
  prove delivery through. If a broker is ever wired, this becomes a real family.
- **Audit events** — the framework writes an `audit_events` row per write and it is
  provable in SQL. Deliberately out of this lane: it is cross-cutting and belongs in its
  own suite rather than duplicated per entity. ⚠️ Say so if you want it here instead.
- **The other 8 entities** — this round is scoped to `tenant`. The runner is built to
  grow: a new lane means a new `qa/<entity>.sh` AND its name in `SUITES`, same change.

---

## 5. Runner contract

**Exactly one entry point: `qa/run.sh`.** `./qa/run.sh` runs everything;
`./qa/run.sh tenant` runs a subset. There is no second runner and never one per entity.

- First act of `run.sh` and of every lane: `cd "$(dirname "$0")/.."`. The project root is
  resolved from the script's own location, so `devops/docker-compose.yml`, `migrations/`
  and the build resolve from any working directory.
- Lane list, explicit and in one place: `SUITES=(tenant domain security)`, deterministic
  order. `ls qa/*.sh` minus `run.sh` must equal it exactly.
- **Fail-fast by default**; `--all` runs the exhaustive sweep. Each lane exits non-zero
  when any case failed — without that, fail-fast can never trip.
- Per-run namespacing of every shared artifact: temp files, the compiled binary, the
  server log, and the HTTP port. Two lanes never share one.
- Boot: probe the effective port FIRST and free it with SIGTERM if something is listening
  (never assume the listener is yours); build `go build -tags 'postgres'` (dialect
  postgres, no `transport:` block → no transport tag); boot in background with
  `APP_PROFILE=dev` and `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml`, plus the
  `JWT_SIGNING_KEY`/`JWT_SIGNING_KID` pair `start.sh` already generates; poll `/readyz`
  READING the 503 reason.
- Shutdown: **SIGTERM only, never `kill -9`**, and `wait` on the server PID until the
  drain completes (budget 30s, matching `shutdown.drainTimeoutSeconds`) before the next
  lane binds the port. Cleanup on `EXIT`.
- Every request pins `Accept-Language: en`. Every case prints name, expectation and
  verdict; a failed assertion dumps the REAL response body. SKIPPED prints as SKIPPED and
  is never folded into GREEN.
- `jq` and `openssl` are preconditions (JSON assertions; the forged-token cases). Their
  absence is a loud failure, not a silent skip.

---

## 6. Report contract — `qa/qa-report.md`

The runner writes the verdict itself; a run whose only trace is terminal scrollback is a
run nobody saw.

- **Header**: timestamp · profile + the engine/transport actually BUILT · the omnicore
  pin · the §2 hygiene mode · suite count · `specs/qa/tenant-contract/plan.md`.
- **Matrix**, one row per suite: `| Suite | Pass | Fail | Skip | Verdict | Time |`. EVERY
  declared suite appears — one that never ran prints `—`, never vanishes.
- **Failures section, only when RED**: per failed case its name, expected vs received,
  and the REAL response body (first few per suite), pointing at
  `qa/.logs/<run-id>/`.
- **Footer**, one line, printed to stdout beside the report path:
  `✅ ALL GREEN — <n>/<n> suites · <cases> cases · <secs>s` or
  `❌ RED — <x> of <n> suites — logs: qa/.logs/<run-id>/`.
- **Rendered LIVE** — rewritten in full after EVERY suite, so a run killed halfway still
  leaves what it had proven.
- **A trap on `EXIT INT TERM` stamps `❌ RUN ABORTED — <reason>`**, disarmed only once the
  final verdict is on disk. A stale green report is worse than no report.
- It is a RUN ARTIFACT. `qa/qa-report.md` and `qa/.logs/` are **offered** as `.gitignore`
  lines at hand-off — the maintainer's call. This skill never edits `.gitignore`.

---

## 7. Artifacts this plan produces

| path | what |
|---|---|
| `qa/run.sh` | the ONE runner; `SUITES=(tenant domain security)`; `--all` disables fail-fast |
| `qa/tenant.sh` | §1 — the framework's promises on this entity |
| `qa/domain.sh` | §1b — the business oracle, R1–R8, positive and negative |
| `qa/security.sh` | §3 — 401, the public-route split, layer-1 403 |
| `qa/microservice.qa.yaml` | suite-owned config: dev profile, DSN on `authcore_qa_db`, port `${QA_HTTP_ADDR::8099}` |

Neither `qa/` nor `specs/qa/` is added to `.gitignore` — both are part of the project.

---

## 8. Gate decisions (2026-09-03)

- **Q1 — data hygiene → dedicated throwaway database.** `authcore_qa_db` via
  `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml`. Nothing is ever written to `authcore_db`.
- **Q2 — a `workspace` key in a PATCH body → 200, ignored, is CORRECT.** Immutability is
  structural. §1b `R8` asserts the 200 plus an unchanged read-back;
  `TenantWorkspaceIsImmutableNotification` is recorded as declared-and-unreachable.
- **Q3 — critical rules → `R1` (unarchive returns suspended), `R2` (an archived workspace
  still blocks the handle), `R3` (the reserved list).** `R4`–`R7` are covered at ordinary
  priority.
- **Q4 — security depth → 401 + the public-route split + layer-1 403**, with the
  tenant-claim gate named as SKIP in §3c.

---

## 9. Run record — 2026-09-03

`./qa/run.sh --all` · **GREEN 246 · RED 0 · SKIPPED 10**, three lanes.

| lane | pass | fail | skip |
|---|---:|---:|---:|
| `tenant` | 155 | 0 | 7 |
| `domain` | 48 | 0 | 1 |
| `security` | 43 | 0 | 2 |

**Reconcile:** every case family named in §1, §1b and §3 exists in the generated
suite and RAN — checked family by family against the run logs, nothing missing.
`ls qa/*.sh` minus `run.sh` equals `SUITES=(tenant domain security)` exactly. Every
§1b rule carries both halves except the two the plan already declares one-sided:
`R7` (negative only — there is no "positive" reading of *setting a status must not
unarchive*) and `R8` (positive only — the maintainer decided the 200 is correct).

**The suite can fail** — the mandatory meta-case. `R1`'s expectation was flipped by
hand to assert `active` where the service answers `suspended`; the run went RED,
the runner exited 1, the matrix showed `domain` RED, the failures section named the
case with its real response body, and the footer read `❌ RED`. The expectation was
then restored. A suite that cannot fail proves nothing, and a report that stays
green through a failing run hides every future failure.

**The abort stamp works** — `./qa/run.sh` under `SIGTERM` mid-run exits 130, leaves
`❌ RUN ABORTED — interrupted by signal` on disk in place of the previous verdict,
and releases port 8099 through the SIGTERM drain. (Testing this with `SIGINT` from a
background job proves nothing: POSIX has a non-interactive shell ignore SIGINT in
asynchronous commands, so `&` masks the trap. Ctrl-C in a real terminal is
unaffected.)

**Two expectations were CORRECTED before the final run. Both were mis-derivations of
mine, verified against the framework's own source, and neither weakened a case:**

1. **`G11` — the cursor contract is a BICONDITIONAL, not "both cursors are always
   present".** I had asserted `startCursor` on page 1 of a forward walk.
   `application/queries/view_reader.go:136` states the actual promise: *"EndCursor is
   set exactly when HasNextPage, StartCursor exactly when HasPreviousPage. So the
   first page of a forward walk carries no StartCursor."* The case now pins that
   biconditional on every page, which is a STRONGER assertion than the one it
   replaced — it would catch a cursor emitted when no such page exists, which the
   original could not.
2. **`D6` — the fixture, not the service, was answering.** `tenant_body` defaulted
   the status with `${4:-active}`, so a case meaning to send an EXPLICIT empty string
   silently sent `active` and got the 201 it then reported as a failure. Changed to
   `${4-active}`, which distinguishes "absent" from "present and empty". This was a
   defect in the suite, and worth recording because it is the exact shape of bug that
   makes a suite quietly untrue: the case ran, printed a verdict, and tested nothing.

**What the 10 remaining SKIPs are.** Nine are families the checklist names and this
service does not have, so a case would be asserting a promise nobody made: no
state-conflict notification and no revision precondition (`E3`), a flat aggregate with no
child table (`F7`) and no 1:N leg to push down (`H20`), `?search=` undeclared so the DTO
gate answers first (`G13`), every declared mode's route mounted so the 403 arm of the
three-way split is unreachable (`I12`), no gRPC transport (`J11`), no exports (`J12`), and
authz layers 2 and 3 structurally absent (`BuildRules` reads no principal field, both
`ToCriteria` return the criteria unchanged). The tenth, `R8b`, is the one that is not a
plain N/A: `TenantWorkspaceIsImmutableNotification` EXISTS in the model and no surface can
reach it, because `patchExcludes: [Workspace]` removes the field from the update DTO. That
is a fact about the model worth seeing rather than a gap in the suite.

**No finding about the service.** Every framework promise in §1, every business rule
in §1b and every refusal in §3 answered as the plan required.

**Residue, as §2 promised:** one throwaway database, `authcore_qa_db`, dropped and
recreated at the start of every run. `authcore_db` holds **0** rows written by this
suite (verified: `select count(*) from tenants where workspace like 'qa-%'`).
