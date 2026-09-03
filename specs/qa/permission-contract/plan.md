# QA contract suite — `permission-contract`

- **Status:** APPROVED (maintainer, 2026-09-03) — §8 records the decisions.
- **Scope:** the `Permission` aggregate, surfaces REST + GraphQL, plus the permission-route
  rows of the security lane. It **extends** `specs/qa/tenant-contract/plan.md` (APPROVED,
  run GREEN 2026-09-03) and the same `qa/run.sh`.
- **Pin:** omnicore **v0.72.1** (`go list -m github.com/ClaudioSchirmer/omnicore`).
- **Profile under test:** `dev` config shape via the suite-owned `qa/microservice.qa.yaml`,
  engine tag `postgres`, **no** transport tag.
- **Plan lives here; the runnable suite lives at `qa/` in the project root.** Neither is
  ever added to `.gitignore`.

Everything below is DERIVED — from `specs/omnicore-gen/permission.omnicore.yaml`, from
`specs/scaffold-entity/permission/spec.md` (Status: APPROVED), from the code actually
mounted, from the tracked seed migration, and from the pin's own docs
(`status-mapping`, `auto-handlers`, `auto-query-handlers`, `auth-middleware`,
`authz-seams`, `graphql`, `relational-view`). **No expectation in this file was read off a
live response.** The service was never called to write it.

### What this round INHERITS and does not repeat

The tenant round already proved, service-wide and once: the whole 401 family (`S1`–`S10`),
the public-route split in both directions including the GraphQL introspection bypass edges
(`S11`–`S15`), and the middleware tenant-claim gate (`S21`/`S21b`). Those are properties of
the middleware, not of an aggregate, and re-running them per entity would double the
runtime while proving nothing new. §3 below therefore covers only what IS per-entity: the
permission gate on the five permission routes, on **both** surfaces.

---

## 0. Surface inventory — what the service DECLARES

**Storage** — flat table `permissions`; managed `revision / created_at / updated_at /
deleted_at`. `delete.root: soft` — no hard-delete verb exists anywhere.

**The shape that makes this entity different from Tenant: three columns go in, two values
come out.**

| layer | carries |
|---|---|
| the table / the view | `resource_name`, `action_name`, `description`, the managed columns |
| the query Result | `Resource`, `Action`, `Description`, **`Permission`** (derived) |
| the Response (every surface) | `id`, `description`, **`permission`**, `createdAt`, `updatedAt` |
| the request body (insert) | `resource`, `action`, `description` — the composite's EXPOSED PARTS, one wire key each |

`Permission` is a **composite value object** (`vos.PermissionKey`, `written: manual`)
decomposed into two columns. Its own field name appears on the wire in exactly one place —
as the **rejection field** of the pair-level 409 — and never as a request or response
*value*. `resource` and `action` are `hidden: true`: **stored, filterable, sortable,
writable, and in no response body on any surface.** The read side answers the rendered
`permission` instead, a `read.computed` field with `from: [Resource, Action]`
(`internal/web/requests/find_permissions_by_params.go:44`, `computed:"Resource,Action"`),
derived in `FromQueryResult`
(`internal/application/queries/find_permissions_by_params_query.go:22`).

That asymmetry is the source of this round's sharpest cases, and every one of them is a
promise the pin makes explicitly in `auto-query-handlers`:

- `?fields=permission` **pushes the SOURCES** down to the store and blanks them before the
  Response projection → the body carries `permission` alone.
- `?orderBy=permission` is **400** — a computed path backs no column, and the keyset cursor
  is built from stored values.
- `?fields=resource` is **400** — the part is not a Response field, so it is not selectable,
  while remaining filterable and orderable. Filterable-but-invisible is the whole point of
  `hidden`, and a case has to prove BOTH halves.

**Wire fields**

| wire | VO | rule |
|---|---|---|
| `resource` (request only) | `PermissionKey.Resource` | colon-joined path of 2–64-rune lowercase slugs `^[a-z0-9]+(-[a-z0-9]+)*$`, ≤64 runes total, no run of 4 identical runes; or exactly `*` |
| `action` (request only) | `PermissionKey.Action` | exactly ONE such slug, no colon; or exactly `*` |
| `permission` (response only) | computed | `resource + ":" + action`, rendered by `PermissionKey.String()` |
| `description` | `Description` (reused) | 15–500 runes, ≥2 words, ≥5 distinct, ≥1 vowel, no run of 4 |
| `createdAt` / `updatedAt` | framework-managed | `read.managed`, read-only |

`deletedAt` is **not** projected — archived state is reached only through
`?includeArchived`.

**Modes** — `[display, insert, update, archive]`. **No `unarchive`, no delete** (§B Q5:
un-archiving would re-enable every grant still pointing at the row in one call). Update
shape `patch`, `patchExcludes: [Permission]`.

**Routes mounted** (`internal/web/permission_routes.go`), each behind `RequirePermission`:

| verb | route | success | permission |
|---|---|---|---|
| insert | `POST /permissions` | 201 + body | `permission:insert` |
| patch | `PATCH /permissions/:id` | 200 + body | `permission:update` |
| archive | `PATCH /permissions/:id/archive` | 204 | `permission:archive` |
| list | `GET /permissions` | 200 + page envelope | `permission:read` |
| by-id | `GET /permissions/:id` | 200 + document | `permission:read` |

**Five, not six.** GraphQL mirrors exactly these five (`permissions`, `permission`,
`createPermission`, `patchPermission`, `archivePermission`) — same handlers, same
permissions. There is no `unarchivePermission` and no `PATCH /permissions/:id/unarchive`,
which is a reachable case here that Tenant could not offer.

**Read backing** — `RelationalView("permissions", loader)` (`internal/infra/views/permission_view.go:43`),
`read.backing: relational`. Therefore **read-your-writes**: every read-back in this suite is
IMMEDIATE, and a case that only passes after a retry is itself RED. No CDC, no poll, no drain.

**Declared read controls** — `FindPermissionsRequest` declares `first, last, after, before,
orderBy, fields, onlyTotal, includeArchived`. **`search` is NOT declared** (deliberately —
a relational-served view answers free text with a typed 400).
`FindPermissionByIDRequest` declares `includeArchived` and nothing else.

| field | filter operators | orderable |
|---|---|---|
| `resource` | eq, ne, in, contains, startswith | asc, desc |
| `action` | eq, ne, in, contains | asc, desc |
| `description` | contains | asc, desc |
| `createdAt` | gte, lte | — |
| `updatedAt` | gte, lte | — |
| `permission` | — (computed) | — (computed) |
| `id` | — | — (declarable, deliberately not declared — §B Q8) |

Note the asymmetry that earns its own case: `startswith` is declared on `resource` and
**not** on `action`; `description` filters on `contains` alone while being orderable both
directions; `createdAt`/`updatedAt` are the mirror — filterable, never orderable.

Page ceiling: no `query:` block in either yaml and no per-view override →
`bootstrap.FrameworkDefaultMaxLimit = 100` (framework `bootstrap/config.go:896`).
The spec's §9 and §B Q8 said "capped at 200 rows a page"; 100 is what the cascade resolves
to, and **both sentences were corrected in this round** (§8). `H13` asserts 100.

**Read joins** — N/A: the repository declares none.

**Infra posture** — Postgres 17 only. No Mongo, no broker, no CDC relay, no
`integration_events` table, no export surface, no gRPC transport.

**The catalog is SEEDED, and that is this round's biggest difference from `tenant`.**
`migrations/postgres/0012_bootstrap_seed_manual.up.sql:37` inserts **39** rows — 38
`resource:action` pairs that routes in `internal/web` enforce today, plus the `*:*`
wildcard — and `role_permissions` binds that wildcard row, and only that row, to the
`master` role the bootstrap admin holds. So on a fresh database `GET /permissions` is
**never empty**, and every count assertion in this round is either scoped to the lane's own
rows or stated as `39 + n`.

> **The spec is stale here too.** §B Q4 answers *"should the migration seed the catalog?"*
> with **No**. Migration 0012, added 2026-09-01, seeds it anyway and argues the case in its
> own header: every write endpoint sits behind `RequirePermission`, every permission is a
> catalog row, and `no-wildcard-grant` refuses `*:*` through the API always — so on a fresh
> database there is no caller who could create the first permission. That is a later
> decision superseding an earlier one, not a defect. It is written down here because a
> reader of the spec alone would derive the wrong baseline.

**A catalog row that is archived stops granting.** The sign-in traversal reads
`PermissionArchivedAt` through the join
(`internal/infra/authentication_reader.go:105`) and drops those rows from the effective set
(`:350`). Archiving a catalog entry therefore revokes it from every token minted
afterwards. §1b `P6` proves it.

**Security posture** — unchanged from the tenant round and re-verified: `auth.mode: jwt`;
`authorization.enabled: true`; `authorization.tenant.required: true`; `auth.issuer.enabled:
true`, so the service mints its own tokens and §3c needs nothing invented.
**Identity-derived rules on Permission: NONE.** `authz.dataAccess: anyone-with-permission`,
and both `ToCriteria` implementations return the criteria unchanged — no tenant filter, no
`Restrict` (`find_permissions_by_params_query.go:18`, `find_permission_by_id_query.go:19`).
The catalog is global by design (§10): it is not partitioned by tenant, there is no
`tenant_id` to filter on and no owner to check. Authz layers 2 and 3 are **structurally
absent on this aggregate**, and §3 says so rather than implying coverage.

**Existing `qa/`** — the tenant round's three lanes plus `lib.bash` and
`microservice.qa.yaml`. This round MIRRORS their conventions and EXTENDS them; it does not
start a parallel style and it does not add a second runner.

---

## 1. Coverage matrix — the framework's promises

Every family is a real case in `qa/permission.sh` unless marked `N/A`. Assertions are
always **status + notification KEY (+ field where the pin names one)**, never prose. Every
request pins `Accept-Language: en`.

### B — Happy path, one per served verb (REST)

`B1` insert → **201**, body exactly `{id, description, permission}` · `B2` by-id → **200**,
body exactly `{id, description, permission, createdAt, updatedAt}` · `B3` list scoped to the
lane's rows → **200**, exactly 1 row · `B4` patch `description` → **200**, new description,
**`permission` unchanged** · `B5` archive → **204**. Every read-back is IMMEDIATE.

`B6` **the absent verb, both surfaces**: `PATCH /permissions/<id>/unarchive` matches no
route → **404** `RouteNotFoundNotification`; `unarchivePermission` is not a field in the
GraphQL schema → a validation error, not a 404 envelope. The mode is absent from `Modes()`
AND no route is mounted, which is the 404 arm of the three-way split — the arm the tenant
round could not reach.

### C — Golden-record round-trip, and the hidden-part leak case

One record exercising **every declared field**, written then read back field-by-field on
REST by-id, on the REST listing row, and on the GraphQL node. `createdAt`/`updatedAt`
present and RFC3339-parseable; `deletedAt` **absent** from every body on every surface.

The composite is asserted as its **EXPOSED PARTS**: `resource` and `action` are sent as two
separate request keys, and `permission` comes back as their rendering, byte-identical to
`<resource>:<action>` as sent. The composite's own field name never appears as a value.

`C2` — **the case nobody writes.** `resource` and `action` appear in **NO** response body,
on **NO** surface: not in the insert 201, not in the patch 200, not in the by-id document,
not in a listing row, not in the GraphQL node, not under `?fields=permission`, and not under
`?includeArchived=true`. A `hidden` field that leaks is a silent regression that every other
family in this matrix passes over.

### D — Validation 422 (one per rule shape, asserting the KEY and the FIELD)

| case | request | expected key | field |
|---|---|---|---|
| `D1` | `resource: ""` | `RequiredFieldNotification` (framework) | `resource` |
| `D2` | `action: ""` | `RequiredFieldNotification` (framework) | `action` |
| `D3` | `resource: "Tenant"` | `InvalidResourceNameNotification` | `resource` |
| `D4` | `resource: " tenant "` | `InvalidResourceNameNotification` | `resource` |
| `D5` | `resource: "a"` (below the 2-rune segment floor) | `InvalidResourceNameNotification` | `resource` |
| `D6` | `resource: "tenant:"` (trailing colon → empty segment) | `InvalidResourceNameNotification` | `resource` |
| `D7` | `resource: "ten--ant"` (doubled hyphen) | `InvalidResourceNameNotification` | `resource` |
| `D8` | `resource: "aaaab"` (run of 4 identical) | `InvalidResourceNameNotification` | `resource` |
| `D9` | `resource:` 65 runes | `InvalidResourceNameNotification` | `resource` |
| `D10` | `action: "read:write"` (a colon in the action) | `InvalidActionNameNotification` | `action` |
| `D11` | `action: "Read"` | `InvalidActionNameNotification` | `action` |
| `D12` | `description: "short"` | `InvalidDescriptionNotification` | `description` |
| `D13` | `resource: "user:profile"` — a colon-joined PATH | **201** — positive control | — |
| `D14` | `action: "rotate-secret"` — a hyphenated slug | **201** — positive control | — |
| `D15` | accented / non-Latin description | **201** — the vowel and word tests are Unicode | — |

All rejections **422**, `semantic: "Validation"`. `D1`/`D2` assert the FRAMEWORK's
required-field notification rather than the VO's own, because the VO short-circuits on
empty (`permission_key.go:74`, `:96`) — a case asserting `InvalidResourceNameNotification`
there would be asserting a branch the code cannot take.

### E — 409, and what uniqueness over a TUPLE means

`E1` the same active pair twice → **409** `PermissionAlreadyExistsNotification`,
`semantic: "Conflict"`, **field `permission`**, and — because `unique.echoValue: true` — the
refused value echoed as `resource:action` through `PermissionKey.String()`.
`E2` a second `<same resource>:<different action>` → **201**. The constraint is over the
PAIR, not over either column, and without this control `E1` would also pass for an
over-broad index.
`E3` a patch that does not move the pair does not self-collide (`excludeSelf: true`) → **200**.
`E4` `resource: "*", action: "*"` → **409** — the seeded wildcard row already holds it. This
case proves the seed and the constraint in one call.

`SemanticStateConflict` — **N/A**, same derivation as the tenant round: no state-conflict
notification is declared, no verb carries a revision precondition, and archive misuse
resolves as 404 through `LoadForWrite`.

### F — Archive round-trip (kept-but-hidden; no `DeleteOnArchive`; **no unarchive**)

`F1` archive → by-id → **404** `RecordNotFoundNotification` ·
`F2` by-id `?includeArchived=true` → **200** ·
`F3` listing hides it, `?includeArchived=true` reveals it ·
`F4` archive an already-archived permission → **404** ·
`F5` **re-inserting the archived pair → 201 with a NEW id**, the old row still readable
under `?includeArchived=true`. This is `unique.scope: active-only` — the exact inverse of
Tenant's `scope: all`, and with no unarchive verb it is the only route back. Cross-referenced
as §1b `P2`'s positive case.

Child stamp-scoped unarchive — **N/A**: flat aggregate, no child table, and no unarchive verb.

### G — Read vocabulary, the computed field, and the pagination envelope

`G1` `?resource.eq=` · `.ne=` · `.in=` · `.contains=` · `.startswith=` ·
`G2` `?action.eq=` · `.ne=` · `.in=` · `.contains=` ·
`G3` `?description.contains=` ·
`G4` `?createdAt.gte=` + `.lte=`, `?updatedAt.gte=` + `.lte=` ·
`G5` `?orderBy=` over `resource` / `-resource` / `action` / `-action` / `description` /
`-description` — all six, because all three fields declare both directions, and two of them
are fields that carry **no value on the wire**: ordering by an invisible column is the
promise `hidden` makes.

`G6` **`?fields=permission` → the body carries `permission` and nothing else** — no
`description`, and still no `resource`/`action` even though the framework pushed them to the
store to feed the derivation. The pin's computed-field pushdown contract, end to end.
`G7` `?fields=description` → `description` present, `permission` absent.
`G8` `?onlyTotal=true` → `totalCount` only, no `data`, no cursors.
`G9` `?last=2` alone → the TAIL window.
`G10` **envelope truthfulness as a BICONDITIONAL** — the correction the tenant round earned
(`application/queries/view_reader.go:136`: *EndCursor is set exactly when HasNextPage,
StartCursor exactly when HasPreviousPage*). With a known seeded count and `?first=2`: assert
the biconditional on every page, echo `endCursor` into `?after=` → page 2 DISJOINT from page
1 with `hasPreviousPage == true`, walk back with `?before=` → page 1 again.
`G11` `?includeArchived=true` raises `totalCount` by exactly the number of archived rows.
`G12` **the baseline is a contract**: on a freshly migrated database the catalog holds
exactly **39** active rows, derived from the tracked seed migration — not from a live
answer.
`G13` **the seed↔routes biconditional**: every `RequiredPermission` literal that
`GET /openapi.json` declares has an ACTIVE catalog row. The seed's own header promises this
list and the `RequirePermission` calls move together; nothing enforces it at boot, so a
route added with a permission nobody seeded is invisible until someone is refused. This is
the case that sees it.

`?search=` — **N/A as a capability case**: the DTO does not declare it, so the opt-in gate
answers first (`H5`) before any engine sees it. `UnsupportedCapabilityNotification` is
therefore unreachable on this entity, exactly as on Tenant.

### H — Rejected reads: the whole typed-400 guard family

All **400**, key `SchemaViolationNotification` unless stated.

| case | request | field named |
|---|---|---|
| `H1` | `?bogus=1` | `bogus` |
| `H2` | `?resource.gte=x` (operator outside its allowlist) | `resource` |
| `H3` | `?action.startswith=x` — **declared on `resource`, NOT on `action`** | `action` |
| `H4` | `?description.eq=x` — `contains` is the only operator it declares | `description` |
| `H5` | `?search=x` (a reserved control the DTO never declared) | `search` |
| `H6` | `?permission.eq=x` — the computed field carries no `query:` tag at all | `permission` |
| `H7` | `?orderBy=permission` — **a computed path backs no column** | `orderBy[permission]` |
| `H8` | `?orderBy=createdAt` · `?orderBy=updatedAt` (filterable, never orderable) | `orderBy[createdAt]` |
| `H9` | `?orderBy=id` (declarable and deliberately not declared — §B Q8) | `orderBy[id]` |
| `H10` | `?orderBy=bogus` | `orderBy[bogus]` |
| `H11` | **`?fields=resource` · `?fields=action`** — filterable and orderable, never SELECTABLE | `fields[resource]` |
| `H12` | `?fields=bogus` | `fields[bogus]` |
| `H13` | `?first=101` | `LimitExceededNotification`, effective max `100` |
| `H14` | `?first=0` · `abc` · `-5` | `first` |
| `H15` | `?first=2&last=2` · `?first=2&before=X` · `?last=2&after=X` · `?after=X&before=Y` | the backward-side key |
| `H16` | `?onlyTotal=true` with `&first=` / `&orderBy=` / `&fields=` / `&after=` | `onlyTotal[<conflict>]` |
| `H17` | `?onlyTotal=true` + a filter, and + `&includeArchived=true` | **200** — counting a filtered subset is the point |
| `H18` | `?after=not-a-cursor` | `after` |
| `H19` | a cursor issued with no `orderBy`, replayed with `&orderBy=resource` | the structural check |
| `H20` | the same cursor replayed with `&includeArchived=true` | the context-hash check |
| `H21` | `?includeArchived=1` · `?onlyTotal=` (empty) | booleans take exactly `true`/`false` |
| `H22` | by-id gate: `?onlyTotal=false` · `?fields=permission` | presence gates an undeclared control |
| `H23` | by-id `?includeArchived=true` | **200** — positive control for the one it declares |
| `H24` | `?createdAt.gte=not-a-date` · `?createdAt=not-a-date` | **400** `InvalidFilterValueNotification` (pin ≥ v0.70.0) |

**No case pins the bracket form.** It was proposed at the gate and **declined**: the syntax
never existed in the framework, so a case asserting its refusal would pin the generic
unknown-key behaviour `H1` already covers, under a name that suggests the service once spoke
it. `H1` is the guard that matters.

### I — Routing, not-found, and the by-id ADDRESS contract (pin ≥ v0.70.0)

`I1` `GET /permissions/<unused uuid>` → **404** `RecordNotFoundNotification` ·
`I2` `DELETE /permissions/:id` → **405** `MethodNotAllowedNotification` — proves no hard
delete exists · `I3` `POST /permissions/:id` → **405** ·
`I4` `GET /permissions/:id/archive` → **405** (the path IS registered, under PATCH) ·
`I5` `PATCH /permissions/:id/unarchive` → **404** `RouteNotFoundNotification` (see `B6`).

**The by-id address family, split by VERB, on both surfaces:**

| case | request | expected |
|---|---|---|
| `I6` | `GET /permissions/not-a-uuid` | **404** `UnknownIDAddressNotification` |
| `I7` | `PATCH /permissions/not-a-uuid` | **400** `MalformedIDNotification` |
| `I8` | `PATCH /permissions/not-a-uuid/archive` | **400** `MalformedIDNotification` |
| `I9` | `{ permission(id: "not-a-uuid") }` | typed `UnknownIDAddressNotification` |
| `I10` | `mutation { archivePermission(id: "not-a-uuid") }` | typed `MalformedIDNotification` |

Mode-missing-with-route-mounted **403** — **N/A**: `unarchive` is absent from `Modes()` AND
unmounted, so it lands on the 404 arm (`I5`), not the 403 one. No route is mounted for any
undeclared mode.

### J — GraphQL (handler invariance)

`J1` `permissions(first: 2)` → `edges { node cursor } pageInfo totalCount`, node equal to
the REST listing row · `J2` `permission(id:)` equals the REST by-id document field for field
· `J3` `createPermission` → visible over **REST** immediately · `J4` `patchPermission` → same
· `J5` `archivePermission` → payload `{ success, id }`, effect confirmed over REST ·
`J6` `permission(id: <unused>)` → HTTP 200 with
`errors[0].extensions.notificationKey == "RecordNotFoundNotification"` ·
`J7` duplicate pair via `createPermission` → `PermissionAlreadyExistsNotification`,
`extensions.semantic == "Conflict"` · `J8` `permissions(search: "x")` → an **unknown
argument**, a gqlparser validation error, not the REST 400 envelope ·
`J9` `where:` / `orderBy:` over `resource` and `action` return the same set and order as
their REST twins — **the hidden parts are queryable on GraphQL too**, which no response body
would ever reveal · `J10` `__typename` beside every selection answers identically (pin ≥
v0.72.1) ·
`J11` **selecting `resource` or `action` on the node is an unknown-field validation error** —
the GraphQL half of `C2`; the schema must not carry a field the REST body hides ·
`J12` `unarchivePermission` is not a field in the schema (see `B6`).

gRPC — **N/A**: no transport wired. Exports — **N/A**: none declared.

### X — Suite meta

`X1` `GET /openapi.json` enumerates exactly the FIVE permission routes above, each carrying
its declared permission, and no sixth. A mismatch is a FINDING, not a silent reconciliation.

---

## 1b. Domain expectations — the business oracle

From `spec.md` §7/§B, the `rules` block, and the maintainer's answers at this gate. They
live in the existing `qa/domain.sh`, appended after the tenant rows. All four rules the
maintainer was asked to rank came back **CRITICAL**, so the ranking below is by setup cost,
not by importance.

| # | rule, as stated | source | POSITIVE case | NEGATIVE case | what a fail means |
|---|---|---|---|---|---|
| **P1** 🔴 | "The pair IS the permission's identity everywhere except this table — the string in the JWT claim and the literal in `RequirePermission(...)`. Editing it would rewrite the meaning of every existing grant, retroactively and invisibly." | `spec.md` §7c rule 9, §B Q2; `rules.list: key-immutable`; **maintainer: CRITICAL** | `PATCH {"description": …}` → **200**, description changed, `permission` byte-identical | `PATCH {"resource":"hijack","action":"read","description": …}` → **200**, and the read-back `permission` is **UNCHANGED** — the pair did not move through a door the DTO does not open | a permission silently changed meaning for everyone already holding it |
| **P2** 🔴 | "An archived remnant must not block a fresh pair — re-inserting is the ONLY way a retired permission comes back, as a new row with a new id." | `permission.omnicore.yaml` `unique.scope: active-only`; `spec.md` §B Q3+Q5; **maintainer: CRITICAL** | insert pair `X`, archive it, insert `X` again → **201** with an id DIFFERENT from the first, and the archived row still readable under `?includeArchived=true` | insert `X` twice while active → **409** `PermissionAlreadyExistsNotification`, field `permission`, value `resource:action` | either a retired pair is walled off forever, or an active one can be duplicated |
| **P3** 🔴 | "`*` is legal only as the ENTIRE part, and a `*` resource forces a `*` action — the claim matcher honours exactly `resource:action`, `resource:*` and `*:*`, so anything else would be a row that matches nothing while reading like a grant." | `spec.md` §7a rule 2, §7b rule 6; `vos/permission_key.go:64` | `<lane resource>` + `action: "*"` → **201** — the wildcard as an entire half | `*` + `read` → **422** `UnmatchablePermissionKeyNotification`, **field `action`** · `user:*` + `read` → **422** `InvalidResourceNameNotification` · `ten*` + `read` → **422** same · `tenant` + `re*d` → **422** `InvalidActionNameNotification` | the catalog stores a sweeping-looking grant that authorizes nothing |
| **P4** 🔴 | "The description must explain the permission, not repeat it — a normalized comparison, case-folded, with whitespace, colons and hyphens collapsed, which catches the lazy paste and nothing more." | `rules.manual: description-does-not-echo-key`; `spec.md` §7c rule 10; **maintainer: CRITICAL** | a genuinely distinct description → **201** | `resource: "user:profile"`, `action: "read"`, `description: "user profile read"` → **422** `PermissionDescriptionEchoesKeyNotification`, field `description`. The description clears the `Description` VO on its own (17 runes, 3 words, ≥5 distinct, a vowel, no 4-run) — so a pass here is the echo rule firing, never the length rule | the normalization is not doing the work it was written for |
| **P5** | "No normalization: a value that does not already comply is refused, never repaired — here it matters more than on Tenant, because the value is compared byte-for-byte against a token claim." | `spec.md` §7a; `permission_key.go` | after any accepted insert, `permission` reads back byte-identical to `<resource>:<action>` as sent | `Tenant` and ` tenant ` → **422**, never a quietly repaired `tenant` | a caller believes they registered a permission the server silently rewrote, and the rewritten one authorizes nothing |
| **P6** | **Archiving a catalog row revokes it from every token minted afterwards.** Verified: the sign-in traversal reads `PermissionArchivedAt` (`authentication_reader.go:105`) and drops those rows from the effective set (`:350`). | derived from the code + **maintainer, asked at this gate** | the admin's token, before the archive, carries `*:*` in `permissions` and `GET /permissions` answers **2xx** | archive the `*:*` catalog row → sign in again → the `permissions` claim no longer contains `*:*`, and `GET /permissions` answers **403** `MissingPermissionNotification` | a retired permission keeps granting, which is a revocation that silently did not happen |

**`P6` is irreversible within a run and therefore constrains the runner** — see §2 and §5.
Archiving `*:*` removes the only permission the bootstrap admin's `master` role carries, and
there is no unarchive verb and no way to re-insert it without `permission:insert`. It is the
**last case of the last lane**, by construction.

**Stated, not silently skipped:** `PermissionKeyIsImmutableNotification` is declared in the
model and is **unreachable through REST and GraphQL**, because `patchExcludes: [Permission]`
removes both halves from the update DTO entirely (`PatchPermissionRequest` carries
`Description` alone). `P1`'s negative case proves the STRUCTURAL immutability — the 200 plus
an unchanged pair — and no case asserts the notification. That is the same shape as the
tenant round's `R8`, and it is a fact about the model recorded here rather than discovered
later.

**Nothing is listed UNPROVEN in this round.**

---

## 2. Data hygiene — inherited, with one addition and one new hazard

**Unchanged from the approved tenant round:** the throwaway `authcore_qa_db`, selected
through `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml`, dropped and recreated at the start of
every run, `migrations.autoRun` rebuilding schema and seed. The project's own
`microservice.dev.yaml` is never touched and `authcore_db` is never written to.

**The addition — the baseline is not empty.** 39 seeded catalog rows exist before the first
case. Every listing this lane asserts a count on is scoped to its own rows with
`?resource.startswith=<lane prefix>`; the two cases that speak about the whole catalog
(`G12`, `G13`) say `39` and derive it from the tracked migration.

**A fixture trap worth naming before it bites.** The lane's generated resources must satisfy
the segment rule — 2–64 lowercase slug runes AND no run of 4 identical runes
(`isPermissionSegment`). `qa/lib.bash`'s `ws()` builds `qa-<lane>-<pid>-<seq>`, and a PID
like `11115` carries a 4-run: every insert would 422 and the lane would go RED for a fixture
reason, not a service one. The generator for this lane sanitizes the run tag against that
rule. This is the same class of bug as the tenant round's `D6` — a case that runs, prints a
verdict, and tests nothing — and it is cheaper to name now than to debug later.

**The new hazard — `P6` burns the admin's access.** After it runs, no further case in the
run can authenticate as an operator. It is therefore the final case of the final lane, and
§5 reorders `SUITES` so that `domain` runs last. The database is dropped and recreated on the
next run, so nothing leaks between runs.

**Residue:** one throwaway database. Nothing is ever written to `authcore_db`.

---

## 3. Security — the permission-route rows of `qa/security.sh`

**Where a valid token comes from: the service itself** (`auth.issuer.enabled: true`), through
`POST /auth/user/token` with the credentials the tracked seed migration puts in the database.
Nothing is invented. This is the same source the tenant round approved.

**3a — the 401 half: INHERITED, not repeated.** `S1`–`S10` prove the middleware's token
rules on `GET /tenants`, and the middleware does not know which route it is guarding. One
row is added, because a route that is not gated at all would still pass every one of them:
`SP1` `GET /permissions` with no `Authorization` header → **401**
`MissingAuthorizationNotification`. That is the "not declared public → 401 tokenless"
direction, asserted on this entity's own path.

**3b — the public-route half: INHERITED.** `S11`–`S15` cover it, including the exactness
neighbours and the GraphQL introspection bypass edges. No permission route is declared
public, and `SP1` is the assertion that it is not.

**3c — the 403 half, layer 1, per verb AND per surface.** This is the part that is genuinely
per-entity: a route gated on REST is not thereby gated on GraphQL, and "the other surface
forgot the gate" is a real regression only a per-surface row can catch.

| case | token | request | expected |
|---|---|---|---|
| `SP2` | the bootstrap admin's FIRST token (`mustChangePassword == true` → `restrictToPasswordChange`, so it carries `user:change-password` and nothing else) | `GET /permissions` | **403** `MissingPermissionNotification`, field `permission`, value `permission:read` |
| `SP3` | same | `GET /permissions/:id` | **403**, value `permission:read` |
| `SP4` | same | `POST /permissions` | **403**, value `permission:insert` |
| `SP5` | same | `PATCH /permissions/:id` | **403**, value `permission:update` |
| `SP6` | same | `PATCH /permissions/:id/archive` | **403**, value `permission:archive` |
| `SP7` | the rotated admin token (`*:*`) | each of the five | **2xx** — the complement. A gate that refuses everyone is also broken |
| `SP8` | the restricted token | `permissions`, `permission`, `createPermission`, `patchPermission`, `archivePermission` | the typed 403 in the **GraphQL** idiom, one per field |
| `SP9` | the rotated token | the same five GraphQL fields | **ok** — the complement, per surface |

**Layers 2 and 3 are structurally absent on this aggregate — stated, not skipped.**
`authz.dataAccess: anyone-with-permission`; both `ToCriteria` return the criteria unchanged,
with no tenant filter and no `Restrict`; no `BuildRules` clause reads a principal field.
There is no owner-check to prove, no cross-tenant row to fail to see, and no column to find
absent. Anyone holding `permission:read` sees every catalog row, by design — the catalog is
global and is not partitioned by tenant (`spec.md` §10).

**The middleware tenant-claim gate — INHERITED** (`S21`/`S21b`). It is a property of the
middleware, proven once.

**3d** — N/A: `auth.mode` is `jwt` and `authorization.enabled` is `true`, so neither
degenerate posture applies.

**3e** — what this round leaves out is exactly the inherited set, and the report prints the
inherited lane's own counts, so nothing reads as covered that was not executed.

---

## 4. Out of scope, named plainly

- **Load / performance / concurrency** — not covered. No revision precondition reaches the
  wire on this entity, so there is no race case to write.
- **UI** — `/docs` and `/graphql/ui` are asserted reachable by the inherited lane, never
  rendered.
- **Integration events — N/A, not skipped:** no `transport:` block, nothing published, no
  `integration_events` table.
- **Audit events** — the framework writes an `audit_events` row per write and it is provable
  in SQL. Deliberately out of this lane, as in the tenant round: cross-cutting, and it
  belongs in its own suite rather than duplicated per entity. ⚠️ Say so if you want it here.
- **The role↔permission grant edge** (`role_permissions`) — `P6` reads THROUGH it to prove
  revocation, but the edge's own contract (granting, withdrawing, `no-wildcard-grant`)
  belongs to a `role` round.
- **The other 5 entities** — `claim`, `client`, `group`, `role`, `user`. The runner is built
  to grow: a new lane means a new `qa/<entity>.sh` AND its name in `SUITES`, same change.

---

## 5. Runner contract — what this round CHANGES

**Still exactly one entry point: `qa/run.sh`.** `./qa/run.sh` runs everything;
`./qa/run.sh permission` runs a subset. No second runner, never one per entity. Everything
in the tenant round's §5 stands unchanged — root resolution from the script's own location,
fail-fast by default with `--all` for the exhaustive sweep, per-run namespacing of every
shared artifact, SIGTERM-only shutdown with a waited drain, `Accept-Language: en` on every
request, SKIPPED printed as SKIPPED.

Three changes, all in the same commit that creates the new lane:

1. **`SUITES=(tenant permission security domain)`** — `permission` is added, and the order
   changes. `domain` moves LAST because its final case (`P6`) archives the wildcard catalog
   row and burns the admin's access for the remainder of the run; `security` must therefore
   complete before it. The list stays explicit and in one place, and `ls qa/*.sh` minus
   `run.sh` must equal it exactly.
2. **`PLAN`** names both plans — the report cannot claim to execute one document when it
   executes two.
3. `qa/lib.bash` gains the permission fixtures (`permission_body`, `create_permission`, and
   the segment-safe resource generator §2 requires), beside the tenant ones. One library,
   extended — not a second one.

---

## 6. Report contract — `qa/qa-report.md`

Unchanged from the tenant round, which is already implemented and proven (live rewrite after
every suite, the `EXIT INT TERM` abort stamp, the SKIP column kept separate, the failures
section with real response bodies pointing at `qa/.logs/<run-id>/`). The matrix simply grows
a `permission` row, and every declared suite still appears — one that never ran prints `—`,
never vanishes.

`qa/qa-report.md` and `qa/.logs/` remain run artifacts, **offered** as `.gitignore` lines at
hand-off and never added by this skill.

---

## 7. Artifacts this plan produces

| path | what |
|---|---|
| `qa/permission.sh` | **new** — §1, the framework's promises on this entity |
| `qa/domain.sh` | **extended** — §1b `P1`–`P6` appended after the tenant rows |
| `qa/security.sh` | **extended** — §3 `SP1`–`SP9` appended |
| `qa/run.sh` | **edited** — `SUITES` gains `permission` and reorders; `PLAN` names both plans |
| `qa/lib.bash` | **extended** — permission fixtures + the segment-safe generator |
| `specs/qa/permission-contract/plan.md` | this document |

Neither `qa/` nor `specs/qa/` is added to `.gitignore`.

---

## 8. Gate decisions (2026-09-03)

- **Q1 — the filter-syntax prose → CORRECTED IN THIS ROUND, and no case pins it.** The
  maintainer's call: *"faz as correções das descrições na mesma rodada, é muito fácil para
  ficar reportando apenas."* `?filter[resource][eq]=tenant` was prose in
  `permission.omnicore.yaml` (three places, one of them the live `docs.operations.byParams`)
  which the generator copied into `permission_routes.go:135` and the service published on
  `/openapi.json`. The framework parses `?resource.eq=tenant`; the bracket form appears in no
  section of the v0.72.1 docs. **Fixed at the source** and regenerated: the diff is the prose,
  the file's checksum, and the lock/report bookkeeping — nothing else — and `go build -tags
  postgres ./...` is green. The proposed `H25` (asserting the bracket form 400s) was
  **declined**: *"não, isso nem existe"* — the syntax was never real, so a case named after it
  would suggest the service once spoke it while proving only what `H1` already proves.
- **Q2 — `*:read` names `action`, and the prose agrees.** My reading was wrong and the
  maintainer corrected it: the set-level validation (the pair is already taken) names
  `permission`, and the validations of a HALF via the code's rules name `resource`/`action`.
  `*:read` is the second kind. No finding, no divergence. `P3` asserts field `action`; `E1`
  asserts field `permission`.
- **Q3 — the revocation case is IN, as the last case of the domain lane.** Approved knowing
  it archives the `*:*` row and ends the run's ability to authenticate. §5 reorders `SUITES`
  so `security` completes first.
- **Q4 — all four rules are CRITICAL**: key immutability, active-only uniqueness with the
  re-insert path, `*` only as a whole half, and description-does-not-echo-key.

**Two stale sentences in `specs/scaffold-entity/permission/spec.md` — CORRECTED in this
round on the maintainer's instruction** (*"corrige (sincroniza com o que tu sabe agora)"*).
The document is `Status: APPROVED` and is the model authority, so both edits record the
supersession rather than quietly rewriting history:

1. **§9 and §B Q8 said the listing is "capped at 200 rows a page".** The resolved ceiling is
   **100** — `bootstrap.FrameworkDefaultMaxLimit`, with no `query.maxLimit` in either yaml and
   no per-view override. Both occurrences now say 100, which is what `H13` asserts.
2. **§B Q4 answered "should the migration seed the catalog?" with No**, and §10 leaned on that
   answer to say the gating rows are inserted by an operator. Migration 0012 (2026-09-01) seeds
   39 rows and argues why in its own header: every write endpoint sits behind
   `RequirePermission`, every permission is a row in a catalog that starts empty, and
   `no-wildcard-grant` refuses `*:*` through the API with no caller exempt — so on a fresh
   database no caller could create the first permission. Q4 now records the supersession and
   its price (the seed and the `RequirePermission` literals have to move together), which is
   exactly what `G13` proves. §10 was updated to match.

---

## 9. Run record — 2026-09-03

`./qa/run.sh --all` · **GREEN 465 · RED 0 · SKIPPED 18**, four lanes.

| lane | pass | fail | skip |
|---|---:|---:|---:|
| `tenant` | 155 | 0 | 7 |
| `permission` | 172 | 0 | 6 |
| `security` | 63 | 0 | 3 |
| `domain` | 75 | 0 | 2 |

**Reconcile.** Every case family named in §1, §1b and §3 exists in the generated suite and
RAN — checked family by family against the lane sources. `ls qa/*.sh` minus `run.sh` equals
`SUITES=(tenant permission security domain)` exactly. Every §1b rule carries both halves
except `P6`, whose "positive" is the pre-archive state the same case asserts before it acts.

**The suite can fail** — the mandatory meta-case. `E1`'s expectation was flipped by hand to
assert **200** where the service answers **409**; the run went RED, the runner exited
non-zero, fail-fast stopped after the `permission` lane, the report's matrix showed
`permission` RED with `security` and `domain` as `—` rather than absent, the failures
section named the case with expected-vs-received and the real 409 body, and the footer read
`❌ RED — 1 of 4 suites`. The expectation was then restored and the full sweep re-run green.

**No finding about the service.** Every framework promise in §1, every business rule in §1b
and every refusal in §3 answered as this plan required, on the first run that had a correct
suite behind it.

### Six suite defects were corrected before the final run — all mine, none a weakened case

Recorded because each is a shape that makes a suite quietly untrue: the case runs, prints a
verdict, and tests nothing.

1. **A command substitution runs in a subshell, so `RESP_CODE` never came back.**
   `ID="$(create_permission …)"` followed by `assert_status … 201` asserted against whatever
   the PARENT shell last sent — a listing (200) at `B1`, an archive (204) at `P2`. The POST
   itself answered 201 throughout; the id arrived on stdout and every later case that used it
   passed. Fixed by calling `req POST` directly wherever the status is the assertion, and the
   trap is now named in `create_permission`'s own comment.
2. **`permission_body` used `${1:-tenant}`.** An explicitly EMPTY resource — the whole point
   of `D1` — was silently replaced by `tenant`, so the case sent a valid pair and reported the
   409 that followed as if the empty value had been refused. Now `${1-tenant}`, the same
   distinction the tenant round's `D6` earned.
3. **`${PG}-g` is a prefix of `${PG}-golden`.** Eleven count and ordering cases in section G
   scoped the golden record in by accident. The paging fixture is now `${PG}-page`.
4. **`SchemaViolationNotification` names the WIRE TOKEN, not the bare field.** `H2`/`H3`/`H4`/
   `H6` expected `resource`, `action`, `description`, `permission`; the contract is
   `resource.gte`, `action.startswith`, `description.eq`, `permission.eq` —
   `auto-query-handlers` states it outright ("the canonical SchemaViolationNotification on the
   wire token"), and the tenant round's own `orderBy[status]` and `fields[bogus]` were already
   that same rendering. The corrected assertions are STRICTER than the ones they replaced.
5. **`H24b` asserted a promise this entity does not make.** `?createdAt=` is the bare `eq`
   shorthand, and Permission declares only `gte`/`lte` on its temporal leaves — so the schema
   gate answers before any value is parsed. Tenant declares `eq` there and reaches
   `InvalidFilterValueNotification` on the identical request. The case now asserts
   `SchemaViolationNotification`, which is what THIS DTO promises.
6. **`SP9` sent `permissions(first: 1) { totalCount }`.** A GraphQL selection of `totalCount`
   alone is how that surface spells `?onlyTotal=true`, which then conflicts with `first:` —
   a 400 saying nothing about the gate the case exists to prove. The selection now asks for
   edges too. Worth knowing rather than only fixing: the only-total conflict matrix is
   reachable through the SELECTION on GraphQL, not only through a query parameter.

**Residue, as §2 promised:** one throwaway database, `authcore_qa_db`, dropped and recreated
at the start of every run — it holds 60 permission rows at rest (39 seeded + 21 the lanes
wrote). `authcore_db` holds **0** rows written by this suite (verified:
`select count(*) from permissions where resource_name like 'qa-%'` → 0).
