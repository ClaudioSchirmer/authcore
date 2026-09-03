# QA round — the permission catalog

Status: APPROVED (2026-09-02)
Suite slug: `permission-catalog` — what this round PROVES: the contract of the global
permission catalog, the entity whose rows the service's own authorization layer reads.
Pin: omnicore **v0.71.0** (previous rounds were approved at v0.69.0 and v0.70.0)
Scope: entity `Permission`, surfaces REST + GraphQL
Lane added: `qa/permission.sh` · runner extended to `SUITES=(tenant permission)`

Everything below is DERIVED — from `specs/omnicore-gen/permission.omnicore.yaml`, from the
generated code actually mounted, from the migrations, and from the pin's own docs. Where a
promise could not be derived it is `N/A — <why>` or `⚠️ OPEN`, never guessed. Items marked
**PROBE** are derivations I will confirm against the running service BEFORE writing them as
cases — the first round's lesson was that a mis-derived expectation costs a whole run.

---

## What this round INHERITS, unchanged

[`specs/qa/tenant/plan.md`](../tenant/plan.md) and
[`specs/qa/id-address-and-filter-values/plan.md`](../id-address-and-filter-values/plan.md),
both APPROVED. Inherited and NOT reopened:

- **§3 auth** — the service's own `POST /auth/user/token` with the tracked bootstrap seed
  credentials, rotated through `PATCH /users/:id/password`. No token is invented.
- **§5 runner contract** — one `qa/run.sh`, fail-fast by default, SIGTERM + drain, per-lane
  namespacing, `Accept-Language: en` on every request, `jq` required.
- **§6 report contract** — `qa/qa-report.md` rewritten after every suite, abort trap,
  SKIP in its own column.
- **out of scope** — load/performance, UI, audit rows, integration events (still N/A: no
  `transport:` block, no `integration_events` table).

The tenant lane is NOT rewritten. It re-runs as-is, and this round records what the new pin
does to its two deliberately-RED cases (§4).

---

## 0. What the service declares (the test surface)

**Storage** — flat table `permissions`, managed `revision / created_at / updated_at /
deleted_at`. `delete.root: soft`. No hard delete anywhere.

**THREE COLUMNS GO IN, TWO GO OUT.** This is the entity's defining shape and the reason it
is worth a lane of its own — no other entity in this service has it:

| column | stored | filterable | sortable | on the WRITE wire | on the READ wire |
|---|---|---|---|---|---|
| `resource_name` | ✅ | ✅ | ✅ | ✅ `resource` | ❌ **never** |
| `action_name` | ✅ | ✅ | ✅ | ✅ `action` | ❌ **never** |
| `description` | ✅ | ✅ (contains) | ✅ | ✅ | ✅ |
| — | no column | ❌ | ❌ | ❌ | ✅ `permission`, computed `resource:action` |

`Key` — the composite value object's OWN name — appears on **no** wire, in **no** direction.
`revision` and `deletedAt` are not projected either.

**Value object** — `PermissionKey`, a COMPOSITE (`written: manual`), exposed as its two
parts and never under its own name. It declares no `Value()`, which is what makes the
framework decompose it into two columns; `String()` is the single home of the `:` separator.
Its `IsValid` checks BOTH parts before the pair, and neither short-circuits the other — a
call carrying two malformed halves is told about both in one answer.

**Modes** — `[display, insert, update, archive]`. **No unarchive. No delete.** Archiving is
one-way by design: un-archiving would re-enable, in one call, every grant still pointing at
that row. The way back is a NEW row with a NEW id.

**Routes actually mounted** (`internal/web/permission_routes.go`) — FIVE, each behind
`RequirePermission`:

| verb | route | success | permission |
|---|---|---|---|
| insert | `POST /permissions` | 201 + body | `permission:insert` |
| patch | `PATCH /permissions/:id` | 200 + body | `permission:update` |
| archive | `PATCH /permissions/:id/archive` | 204 | `permission:archive` |
| list | `GET /permissions` | 200 + page envelope | `permission:read` |
| by-id | `GET /permissions/:id` | 200 + document | `permission:read` |

GraphQL mirrors exactly those five (`permissions`, `permission`, `createPermission`,
`patchPermission`, `archivePermission`) — and mounts **no** `unarchivePermission`.
Cross-checked against `GET /openapi.json` at run time (case `X1`).

**Read backing** — `RelationalView("permissions", loader)`, no materialisation → **read-your-
writes**. Every read-back is IMMEDIATE; a case that only passes after a retry is itself a RED.
No CDC, no poll, no drain.

**Declared read controls** — `FindPermissionsRequest` declares
`first, last, after, before, orderBy, fields, onlyTotal, includeArchived`
(**`search` is NOT declared** — deliberately, a relational view cannot serve free text).
`FindPermissionByIDRequest` declares `includeArchived` and nothing else.

Filter and sort vocabulary, read off the DTO tags — note how it differs from Tenant's:

| field | operators | orderable |
|---|---|---|
| `resource` | eq, **ne**, in, contains, startswith | asc, desc |
| `action` | eq, **ne**, in, contains | asc, desc |
| `description` | contains | asc, desc |
| `createdAt` | gte, lte | — |
| `updatedAt` | gte, lte | — |

**No `icontains` anywhere** (Tenant has it), `startswith` on `resource` but **not** on
`action`, and `createdAt`/`updatedAt` are filterable but **not** orderable. Every one of
those asymmetries is a case.

**Uniqueness** — over the TUPLE `(resource_name, action_name)`, `scope: active-only`,
enforced twice: the service pre-check `PermissionKeyTaken` (`excludeSelf`, `activeOnly`) and
the partial index `permissions_resource_name_action_name_key ... WHERE deleted_at IS NULL`,
bound to `PermissionAlreadyExistsNotification`. **This is the INVERSE of Tenant's
`scope: all`** and the two must not be asserted the same way.

**Read joins** — N/A: `PermissionRepository` declares none.

**Infra posture** — Postgres only. No Mongo, no broker, no CDC, no gRPC, no exports.

**Auth** — `auth.mode: jwt`, `authorization.enabled: true`. Permissions
`permission:insert|update|archive|read`.

**The table is NOT empty at boot.** `migrations/postgres/0012_bootstrap_seed_manual.up.sql`
inserts **39 catalog rows** (38 real pairs + `*:*`). This is the single biggest difference
from the tenant lane, where the table started empty, and it drives §2.

---

## 1. Coverage matrix

Every family below is a real case in `qa/permission.sh` unless marked `N/A`. Assertions are
**status + notification KEY (+ field where the pin names one)**, never prose.

### A — Sign-in and the permission gate (REST)

Same mechanism the tenant lane proves, re-asserted for THIS resource's keys because the lane
is self-contained (its own database, its own server):

`A1` bootstrap sign-in → 200, `mustChangePassword == true`, `permissions == ["user:change-password"]` ·
`A2` `GET /permissions` with that restricted token → **403** `MissingPermissionNotification`,
field `permission`, value **`permission:read`** ·
`A3` rotate password → re-sign-in → `*:*` token, used by every later case ·
`A4` tokenless → **401** `MissingAuthorizationNotification` ·
`A5` bogus bearer → **401** `InvalidTokenNotification`.

### B — Happy path, one per served verb (REST)

`B1` insert → **201** + body · `B2` by-id → **200** · `B3` list scoped to the record →
**200**, exactly 1 row · `B4` patch `description` → **200**, new value, the pair untouched ·
`B5` archive → **204**. Read back IMMEDIATELY after each.

There is no `B6`: **unarchive does not exist**, which is `I6` rather than a missing happy path.

### C — Golden record and THE HIDDEN PARTS

The family that carries this lane. One record exercising every declared field, written then
read back on every surface — and, just as importantly, the fields that must NOT come back.

| case | assertion |
|---|---|
| `C1` | insert response carries `id`, `description`, `permission` — and `permission` equals the `resource:action` that went in |
| `C2` | insert response carries **no** `resource`, **no** `action`, **no** `key` |
| `C3` | patch response: same three keys, same three absences |
| `C4` | by-id: `id`, `description`, `createdAt`, `updatedAt`, `permission`; `createdAt`/`updatedAt` RFC3339-parseable |
| `C5` | by-id carries **no** `resource`, `action`, `key`, `revision`, `deletedAt` |
| `C6` | listing row: identical key set to `C4`, identical absences |
| `C7` | GraphQL node: same values as the REST by-id document, field for field |
| `C8` | the GraphQL **schema** does not advertise `resource` or `action` as node fields — asking for one is an unknown-field validation error, not a null |
| `C9` | the pair is filterable while invisible: `?resource=<x>&action=<y>` returns exactly the record whose `permission` is `<x>:<y>` — proving the two halves are queryable and unprojected at once |

`C9` is the case that proves the model rather than the plumbing: filters are declared on the
Request DTO and never consult the Response, which is what lets a stored part stay queryable
while leaving no value on the wire.

### D — Validation 422 (one per rule shape, asserting the KEY)

| case | request | expected key | on field |
|---|---|---|---|
| `D1` | `resource: "Tenant"` (uppercase) | `InvalidResourceNameNotification` | Resource |
| `D2` | `resource: "user::profile"` (empty segment) | `InvalidResourceNameNotification` | Resource |
| `D3` | `action: "read:all"` (a colon in the action) | `InvalidActionNameNotification` | Action |
| `D4` | `action: "ten*"` (wildcard mixed into a slug) | `InvalidActionNameNotification` | Action |
| `D5` | `resource: "user:*"` (wildcard INSIDE a path) | `InvalidResourceNameNotification` | Resource |
| `D6` | `resource: "*", action: "read"` | `UnmatchablePermissionKeyNotification` | Action |
| `D7` | `resource: ""` | `RequiredFieldNotification` | Resource |
| `D8` | `action: ""` | `RequiredFieldNotification` | Action |
| `D9` | `description: "short"` | `InvalidDescriptionNotification` | Description |
| `D10` | `description` == the rendered `resource:action` | `PermissionDescriptionEchoesKeyNotification` | Description |
| `D11` | **both** parts malformed in one call | BOTH `InvalidResourceNameNotification` and `InvalidActionNameNotification` in ONE answer |

All **422** (`SemanticValidation`). `D11` asserts the value object's stated no-short-circuit
promise — the one an implementation change could quietly break.

`D12` — **structural immutability of the pair**: `PatchPermissionRequest` declares only
`description`, so `resource`/`action` cannot be sent at all. The wire promise is therefore
"a `resource` or `action` key in a PATCH body changes nothing" → **200**, stored `permission`
unchanged. `PermissionKeyIsImmutableNotification` is the belt-and-braces layer behind a door
REST cannot open, so no case asserts it — stated rather than silently skipped, exactly as the
tenant lane states `D8`.

### E — 409, and the active-only scope

- `E1` insert a pair the catalog already holds (`tenant:read`, seeded) → **409**
  `PermissionAlreadyExistsNotification`, `semantic: "Conflict"`.
- `E2` **the tuple is the unit**: same resource, fresh action (`tenant:<qa-action>`) → **201**.
  A second `tenant:*something*` is perfectly legal; only the PAIR is taken.
- `E3` **`*:*` is a VALID shape that is already taken**: inserting `resource: "*", action: "*"`
  answers **409**, not a 422. That distinguishes "the wildcard pair is malformed" (it is not)
  from "the wildcard pair exists" (it does, seeded) — and it is the only way to exercise the
  wildcard accept-path, since a valid wildcard insert always collides with the seed.
- `E4` **active-only, the INVERSE of Tenant**: create a pair, archive it, insert the SAME pair
  again → **201**. The archived row does not hold the handle; with no unarchive verb this is
  the only route back for a retired permission.
- `E5` wrong-state (`SemanticStateConflict`) — **N/A**, same reasoning as the tenant lane: no
  verb here carries a revision precondition, and archive misuse resolves as 404 (`F4`).
- `E6` **PROBE — the `field` a 409 echoes.** The service pre-check raises
  `AddNotification("Key", ...)` and the constraint binding declares `Field: "key"`, but `Key`
  is the composite's own name, which this model says reaches no wire. I will read the real
  envelope before writing the assertion. Two honest outcomes: it echoes a wire name (assert
  it), or it echoes `Key`/`key` — a name that appears nowhere on the wire — which is a
  FINDING to report, not a case to weaken.

### F — Archive: one-way, and what stays visible

Regime: kept-but-hidden (`DeleteOnArchive` not declared).

`F1` archive → by-id → **404** `RecordNotFoundNotification` ·
`F2` by-id `?includeArchived=true` → **200**, and the archived row still renders its
`permission` string (a retired permission must stay auditable — with no unarchive verb this
listing is the ONLY way to see one) ·
`F3` listing hides it; `?includeArchived=true` reveals it, raising the scoped `totalCount` by
exactly 1 ·
`F4` archive an already-archived permission → **404** (`LoadForWrite` filters archived) ·
`F5` patch an archived permission → **404** — an archived catalog entry is not editable.

Child stamp-scoped unarchive — **N/A**: flat aggregate, no child table.

### G — Read vocabulary (REST listing)

Every count is scoped by `?resource.startswith=qa-<run>-` (see §2), so the 39 seeded rows
never make an assertion ambiguous.

`G1` `?resource=<exact>` · `G2` `?resource.ne=<x>` (the operator Tenant has no case for) ·
`G3` `?resource.in=a,b` · `G4` `?resource.contains=` · `G5` `?resource.startswith=` ·
`G6` `?action=` / `?action.ne=` / `?action.in=` / `?action.contains=` ·
`G7` `?description.contains=` · `G8` `?createdAt.gte=` + `?createdAt.lte=` ·
`G9` `?updatedAt.gte=` (filterable though not orderable) ·
`G10` `?orderBy=resource` / `-resource` / `action` / `-action` / `description` ·
`G11` `?fields=description` → that key present, `permission` absent ·
`G12` **`?fields=permission`** → the computed field alone, which per the spec pushes
`Resource + Action` down to the store automatically — the case that proves a computed
projection is selectable even though it backs no column ·
`G13` `?onlyTotal=true` → `pagination.totalCount` only, no `data`, no cursors ·
`G14` `?last=2` alone → the TAIL window, `hasNextPage == false` ·
`G15` **pagination envelope truthfulness**: 5 scoped fixtures, `?first=2&orderBy=resource` —
`totalCount == 5`, `hasNextPage == true`, `hasPreviousPage == false`, both cursors present;
echo `endCursor` into `?after=` → page 2 DISJOINT from page 1, `hasPreviousPage == true`;
walk back with `?before=<page 2 startCursor>` → page 1 again ·
`G16` `?includeArchived=true` raises the scoped `totalCount` by exactly the number archived ·
`G17` **the seeded catalog is readable**: `?resource=permission&action=read` → exactly 1 row,
`permission == "permission:read"`. The rows the service's own `RequirePermission` calls depend
on are rendered correctly — read-only, never written to.

`?search=` — **N/A as a capability case**: undeclared on the DTO, so the opt-in gate refuses
it first (`H4`).

### H — Rejected reads: the whole typed-400 guard family

All **400**. Key is `SchemaViolationNotification` unless stated.

| case | request | why it must be refused |
|---|---|---|
| `H1` | `?bogus=1` | unknown field |
| `H2` | `?key=x` | the composite's own name reaches no wire, in either direction |
| `H3` | `?permission=tenant:read` | the computed field backs no column — filter the two sources instead |
| `H4` | `?search=tenant` | reserved control the DTO never declared |
| `H5` | `?resource.icontains=x` | operator outside the allowlist (Tenant HAS `icontains`; this entity does not) |
| `H6` | `?action.startswith=x` | declared on `resource`, deliberately not on `action` |
| `H7` | `?description.eq=x` | `description` declares `contains` only |
| `H8` | `?createdAt.contains=x` | operator outside a temporal leaf's allowlist |
| `H9` | `?fields=bogus` | unresolvable projection path |
| `H10` | `?fields=resource` | **PROBE** — a hidden STORED part is not a Response field. Expected 400; if it resolves instead, the hidden part leaks through `?fields=` and that is a FINDING |
| `H11` | `?orderBy=permission` | the computed path cannot be an order token |
| `H12` | `?orderBy=createdAt` · `?orderBy=updatedAt` | filterable, never declared orderable |
| `H13` | `?orderBy=bogus` | unknown |
| `H14` | `?orderBy=-description` | **200** — positive control, `desc` IS declared |
| `H15` | `?first=101` | `LimitExceededNotification`, effective max `100` (no `query:` block in the yaml, no per-view override → framework default) |
| `H16` | `?first=0` | below the floor |
| `H17` | `?first=2&last=2` · `?first=2&before=X` · `?last=2&after=X` · `?after=X&before=Y` | the mixed-direction matrix; the backward-side key is named (`last` when present, else `before`) — the correction the first round paid for |
| `H18` | `?onlyTotal=true` with `&first=10` / `&orderBy=resource` / `&fields=description` / `&after=X` | `onlyTotal[<conflict>]` |
| `H19` | `?onlyTotal=true&resource.startswith=…` · `&includeArchived=true` | **200** — filters and archive are not conflicts; counting a subset is the point |
| `H20` | `?after=not-a-cursor` | malformed cursor |
| `H21` | cursor issued with no `orderBy`, replayed with `&orderBy=resource` | the STRUCTURAL check, before dispatch |
| `H22` | same cursor replayed with `&includeArchived=true` | the CONTEXT-HASH check, inside the reader |
| `H23` | `?includeArchived=1` · `?onlyTotal=` (empty) | booleans take exactly `true`/`false` |
| `H24` | **by-id DTO gate**: `GET /permissions/:id?fields=description` · `?onlyTotal=false` | presence gates — declared-ness is the whole test |
| `H25` | `GET /permissions/:id?includeArchived=true` | **200** — positive control for the one control it declares |

`UnsupportedCapabilityNotification` — **N/A for this entity**, same reasoning as the tenant
lane: flat aggregate (no 1:N leg to push down) and `search` undeclared, so the DTO gate
answers first with a `SchemaViolationNotification`.

### I — Routing, not-found, and the verb that does not exist

`I1` `GET /permissions/<unused uuid>` → **404** `RecordNotFoundNotification` ·
`I2` `DELETE /permissions/:id` → **405** `MethodNotAllowedNotification` — proves no hard
delete exists ·
`I3` `POST /permissions/:id` → **405** (path registered under PATCH, method not) ·
`I4` `GET /permissions/:id/archive` → **405** (registered under PATCH) ·
`I5` `GET /permissions/:id/purge` → **404** `RouteNotFoundNotification` ·
`I6` **`PATCH /permissions/:id/unarchive` → 404 `RouteNotFoundNotification`.** The design
decision made observable: no route is mounted for a mode the entity does not declare, so the
path matches nothing. This is the arm the tenant lane had to mark N/A ·
`I7` GraphQL `unarchivePermission(id:)` → an unknown-field validation error; the schema never
advertises it.

Mode-missing-**with-route-mounted** → 403 `…NotAllowedNotification` — **N/A**: every mode
`Permission` declares has its route mounted and no route is mounted for an undeclared mode,
so that arm of the three-way split stays unreachable. The 403 that IS reachable is the
permission gate (`A2`).

### K — The by-id address, split by VERB (pin ≥ v0.70.0)

Envelope: context `Request`, `field: "id"`, `value` echoing the rejected segment.

| case | request | expected |
|---|---|---|
| `K1` | `GET /permissions/not-a-uuid` | **404** `UnknownIDAddressNotification` |
| `K2` | `PATCH /permissions/not-a-uuid` (body) | **400** `MalformedIDNotification` |
| `K3` | `PATCH /permissions/not-a-uuid/archive` | **400** — the bodyless by-id command |
| `K4` | `value` echo on `K2` == `"not-a-uuid"`, `context == "Request"` | an address problem is not labelled a payload problem |
| `K5` | GraphQL `permission(id: "not-a-uuid")` | 200, `extensions.notificationKey == UnknownIDAddressNotification`, `semantic == NotFound` |
| `K6` | GraphQL `archivePermission(id: "not-a-uuid")` | 200, `extensions.notificationKey == MalformedIDNotification`, `semantic == Schema` |

### L — A filter VALUE the leaf cannot take (pin ≥ v0.70.0, **fixed at v0.71.0**)

| case | request | expected |
|---|---|---|
| `L1` | `?createdAt.gte=not-a-date` | **400** `InvalidFilterValueNotification` |
| `L2` | `?createdAt=not-a-date` | **400** `InvalidFilterValueNotification` |
| `L3` | `?updatedAt.lte=13/45/2026` | **400** `InvalidFilterValueNotification` |

These are the cases the previous round wrote against Tenant and left **deliberately RED**:
v0.70.0 promised the guard and the `*time.Time` leaf still fell through to the driver as an
external 500. v0.71.0's changelog says the axis was wrong — coercion switched on
`reflect.Kind` alone, where `time.Time` collapses to `Kind=struct` — and that the declared
TYPE is now consulted first, parsing `time.Time` as RFC3339. **This round expects them
GREEN**, here and in the inherited tenant lane. If they are not, the finding stands and both
lanes stay RED, unweakened.

Permission declares no `int64`, `bool`, `domain.ID` or `time.Duration` filter leaf, so the
other kinds v0.71.0 names cannot be proven from this lane — the tenant round already probed
them by hand and recorded the result. Not smuggled in here as cases against routes this lane
does not own.

### J — GraphQL (handler invariance)

`J1` `permissions(first: 2)` → `edges { node cursor } pageInfo totalCount`, node equal to the
REST listing row for the same record · `J2` `permission(id:)` == the REST by-id document ·
`J3` `createPermission(input:)` → visible over **REST** immediately (one write, two surfaces) ·
`J4` `patchPermission(id, input:)` → same · `J5` `archivePermission` → payload
`{ success, id }`, effect confirmed over REST · `J6` `permission(id: <unused uuid>)` → HTTP
200, `errors[0].extensions.notificationKey == "RecordNotFoundNotification"` (the GraphQL
idiom; the REST envelope is NOT asserted cross-surface) · `J7` duplicate pair via
`createPermission` → `extensions.notificationKey == "PermissionAlreadyExistsNotification"`,
`semantic == "Conflict"` · `J8` GraphQL with no bearer → **401** · `J9` `permissions(search:)`
→ unknown argument (the DTO opt-in gate in GraphQL idiom, not the REST 400 envelope) ·
`J10` typed `where` + `orderBy` return the same set and order as their REST twins.

**PROBE** — the exact spelling of the GraphQL `where` / `orderBy` arguments (the tenant lane
uses `where: {workspace: {startswith: …}}` and `orderBy: [{field: WORKSPACE, direction: DESC}]`)
will be confirmed against the introspected schema before these are written, not assumed from
the tenant pattern.

gRPC — **N/A**: no transport wired. Exports — **N/A**: none declared.

### X — Suite meta

`X1` `GET /openapi.json` enumerates exactly the **five** permission routes — and, positively,
**no** `/permissions/{id}/unarchive` entry. The cheapest oracle for "which verbs does this
entity really serve"; a mismatch is a FINDING, not a guess to reconcile silently.

---

## 2. Data hygiene

The mode is INHERITED (throwaway database, `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml`,
dropped and recreated per run, `migrations.autoRun` rebuilding schema + bootstrap seed). Two
things are genuinely new and are the gate questions below.

**The table is not empty.** 39 catalog rows exist before the first case runs, and they are the
rows this service's own authorization reads. Consequences, decided:

- **The suite never writes to a seeded row.** No archive, no patch, no re-description of any
  of the 39. `G17` reads one; `E1`/`E3` collide with two; nothing mutates them.
- **Every count assertion is scoped** by `?resource.startswith=qa-<run>-`, where `<run>` is
  the lane's epoch stamp. Fixture resources are single slugs (`qa-1756800000-p1`), valid under
  the segment rule — lowercase alphanumerics in hyphen-separated groups, 2–64 runes.

**Q1 — DECIDED: both lanes share `authcore_qa_db`.** Each lane drops and recreates it as its
own first act, so every lane still starts from a freshly migrated database carrying nothing
but the bootstrap seed. Residue: that one throwaway database, recreated on the next run;
`authcore_db` is never written to.

> **The lanes are therefore SEQUENTIAL-ONLY, and this is the one shared artifact that makes
> them so.** `qa/run.sh` runs them one after another and nothing in it runs lanes in
> parallel, so the shared database is safe as invoked. Two lanes started by hand in two
> terminals WOULD destroy each other's data — the second one's `dropdb` lands while the first
> is asserting. Both lane scripts say so at the top. Everything else stays namespaced per
> lane (port, binary, server log, temp dir), so the day a parallel runner is wanted, the
> database is the single thing to split — by exporting `QA_DATABASE_URL`, which the qa yaml
> already reads as `${QA_DATABASE_URL:…}`, with no file to change.

**Q2 — DECIDED: the unfiltered catalog total is NOT asserted.** Every count assertion is
scoped by `?resource.startswith=qa-<run>-`. Asserting `GET /permissions?onlyTotal=true` == 39
would couple this contract suite to the contents of the seed migration, turning the lane RED
for maintenance every time a route and its catalog row are added together. That the seed and
the `RequirePermission` calls must move together is a real invariant — it just is not a WIRE
promise, and this suite asserts wire promises. `G17` still reads a seeded row and proves the
catalog renders correctly.

---

## 3. Auth — inherited, restated for this resource

Unchanged from the tenant round: `auth.mode: jwt`, `authorization.enabled: true`, the bench
is CLOSED, no token is invented. The lane signs in with the tracked bootstrap credentials and
rotates the password (required anyway — the pre-rotation token carries only
`user:change-password`). 401 and 403 are first-class cases (`A2`, `A4`, `A5`), and `A2` names
**`permission:read`**, this resource's own key.

## 4. What the new pin does to the INHERITED lane

`qa/tenant.sh` is re-run unchanged as part of `./qa/run.sh`. Two of its cases were left
deliberately RED at v0.70.0 (`L1`, `L2` — the temporal leaf reaching the driver as a 500).
v0.71.0 claims that defect fixed. This round REPORTS the outcome rather than assuming it:

- if they pass, the finding
  [`finding-filter-value-temporal-leaf.md`](../id-address-and-filter-values/finding-filter-value-temporal-leaf.md)
  gains a **RESOLVED at v0.71.0** section recording the verification. The approved plan it
  belongs to is NOT rewritten — an approved plan is a record of what was approved.
- if they still fail, the finding stands, both lanes stay RED, and nothing is weakened.

The v0.71.0 changelog also mentions the field-name discrepancy the same finding raised
(`field` echoing the Go name rather than the wire key) only indirectly. That half will be
re-checked at run time and the finding updated with what is actually observed.

**Boot risk checked and cleared:** v0.71.0's breaking change renames the tracing instrument
token `pgx` → `relational`, and an unknown token aborts the boot. Neither
`microservice.dev.yaml` nor `qa/microservice.qa.yaml` declares an `instrument:` block, so
nothing in this service is affected.

## 5. Out of scope, named plainly

Unchanged from the inherited rounds: load/performance/concurrency, UI, audit rows,
integration events (N/A — no `transport:` block, no `integration_events` table). The
`role_permission` child that bundles catalog entries into a Role belongs to the **role**
aggregate and to a future `role` lane, not here: this lane never asserts a grant.
The seven remaining entities are untouched.

## 6. Runner contract — inherited, extended

`SUITES=(tenant permission)`, deterministic order, in `qa/run.sh`. `ls qa/*.sh` minus
`run.sh` must equal it exactly. `./qa/run.sh permission` runs this lane alone.

Per-lane namespacing, this lane's values: port **8098** (tenant holds 8099), overridable
through `QA_PERMISSION_PORT`; its own `mktemp -d` for the binary, the server log and the
response buffer; `AUTH_SELF_URL` travels with the port, since the service issues and
validates its own tokens and refuses to boot if they disagree. Build `-tags 'postgres'`
(dialect from the yaml, no `transport:` block → no transport tag). Boot with
`APP_PROFILE=dev` + `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml`, poll `/readyz` reading
the 503 reason, free the port with SIGTERM first if something is listening. Shutdown:
SIGTERM only, `wait` for the drain (35s budget) before returning.

## 7. Artifacts this plan produces

| path | what |
|---|---|
| `qa/permission.sh` | the new lane, self-contained |
| `qa/run.sh` | edited: `SUITES=(tenant permission)`, plan pointer updated |
| `specs/qa/permission-catalog/plan.md` | this document |
| `specs/qa/id-address-and-filter-values/finding-filter-value-temporal-leaf.md` | appended with the v0.71.0 verification outcome (§4) — the approved plan beside it is not touched |

`qa/qa-report.md` and `qa/.logs/` are run artifacts. Neither `qa/` nor `specs/qa/` is added
to `.gitignore` — both are part of the project; the artifact lines are OFFERED at hand-off,
never added by this skill.

## 8. Gate decisions

- **Q1 — the lane's database → share `authcore_qa_db`**, each lane dropping and recreating it.
  Lanes are sequential-only as a documented consequence; see §2.
- **Q2 — the unfiltered catalog total (39) → NOT asserted.** Scoped counts only; the suite
  stays independent of the seed migration's contents. See §2.

---

## 9. Run record — 2026-09-02

`./qa/run.sh --all` · **GREEN 445 · RED 0 · SKIPPED 0** across both lanes
(`tenant` 201 · `permission` 244), 11s. Every family in §1 exists in `qa/permission.sh`
and RAN. `ls qa/*.sh` minus `run.sh` equals `SUITES=(tenant permission)` exactly.

**The suite can fail** (mandatory meta-case): `H25`'s expectation was flipped from 200 to
409 by hand; the run went RED on that one case, `qa/qa-report.md` showed the `permission`
row RED with the case named, expected-vs-received and the real 200 body, and the footer read
`❌ RED — 1 of 2 suites`. The expectation was then restored and the run is green again. A
report that stayed green through a failing run would hide every future failure.

### The three PROBE items, resolved against the running service

- **`E6` — the `field` a 409 echoes → it names `key`.** The envelope is
  `context: "Permission"`, `field: "key"`, `semantic: "Conflict"`, on both REST and GraphQL
  (which adds `fieldLabel: "Permission"`). Asserted as `E1c`.
  **This qualifies §0's claim** and the qualification is worth stating plainly: `Key` reaches
  no wire in the SUCCESS bodies (`C2`, `C3`, `C5`, `C6`), it is not a filter (`H2a`) and not
  a projection path (`H2b`) — but the CONFLICT envelope names it. A consumer highlighting the
  offending input receives a field name it never sent and can never read back.
  Not filed as a framework finding, because the name is the service's own: it comes from this
  spec's `unique.notification` on field `Key` and from the constraint binding
  (`Field: "key"`) in `internal/infra/permission_repository.go`. It is a design question for
  the maintainer — a composite with hidden parts has no wire field to point at, and `key`,
  `resource` and `action`, or no field at all are three defensible answers. Raised, not
  decided, and the case asserts today's behavior either way.
- **`H10` — can a hidden part be projected? → no.** `?fields=resource` and `?fields=action`
  are both 400 `SchemaViolationNotification` on `fields[resource]` / `fields[action]`. No
  leak. Asserted as `H10a`/`H10b`.
- **`J` — the GraphQL argument spellings → confirmed by introspection, not assumed.**
  `where: {resource: {startswith: …}}`, `orderBy: [{field: RESOURCE, direction: ASC}]`. The
  `PermissionOrderField` enum holds exactly `ACTION`, `DESCRIPTION`, `RESOURCE`, and the
  `Permission` node type holds exactly `id`, `description`, `createdAt`, `updatedAt`,
  `permission` — the hidden parts are filterable in `where` and absent from the node, which
  is `C9`'s GraphQL twin (`J10c`/`J10d`).

### Expectations CORRECTED before the suite was final

All four were mis-derivations of mine, verified against the service before changing. None
weakened a case.

1. **The planned `L2` (`?createdAt=not-a-date`) is not an `InvalidFilterValue` case on this
   entity.** Permission's temporal leaves declare `gte,lte` and **not `eq`**, so the bare
   form violates the query GRAMMAR and the gate answers 400 `SchemaViolationNotification`
   before any value is coerced. It lives as **`H8b`**, and the delivered `L` family is
   `L1` (`createdAt.gte`) + `L2` (`updatedAt.lte`). The `eq` half of the temporal contract
   is owned by the tenant lane, whose `createdAt` does declare `eq` — and it runs in the
   same suite.
2. **`G15` — `startCursor` is null on page 1**, where `hasPreviousPage` is false; only the
   forward edge is issued there. The plan predicted both cursors present. The walk itself
   (page 1 → `after` → page 2 → `before` → page 1) is asserted in full.
3. **`G16` is a DELTA, not an absolute.** It reads the scoped total twice and asserts the
   archived rows are exactly the difference, plus that the retired row is genuinely among
   them (`G16c`/`G16d`). An absolute count there would depend on how many live fixtures
   earlier sections happened to leave behind.
4. **Exact counts moved to their own fixtures.** The first full run went RED on 15 count
   assertions — all of them mine, all traceable to two facts I had not carried into the
   arithmetic: `B5` archives a fixture, and `D12` rewrites a description. Rather than
   patching fifteen numbers, section G now counts over `${PREFIX}p` — the five paging
   fixtures no other section archives, patches or adds to — and `D12` got a fixture of its
   own (`${PREFIX}imm`). The class of breakage is gone, not just its instances.

Two assertion bugs of mine were also fixed: `X1c` searched every OpenAPI path for the word
"unarchive" (`/tenants/{id}/unarchive` legitimately exists, so the case could never pass)
and now scopes to `/permissions`; `C4c`/`C4d` used jq's `fromdate`, which accepts only a
UTC `Z` instant with no fractional part and so rejected a perfectly conformant RFC3339
timestamp carrying microseconds and a numeric offset — they now match the RFC3339 grammar.

### §4 discharged — the inherited lane at the new pin

`qa/tenant.sh` was re-run **unedited** and is fully green, including `L1` and `L2`, which
stood deliberately RED at v0.70.0. The v0.71.0 fix for the temporal leaf is verified, and
the field-name half of the same finding is fixed for the wire-visible identity leaves
(`tenantID` was `TenantID`); it survives only on the reader-guarded `?id=` path, which
belongs to a future `user` lane. Written up in
[`finding-filter-value-temporal-leaf.md`](../id-address-and-filter-values/finding-filter-value-temporal-leaf.md),
now marked **RESOLVED / CLOSED**.

**No finding was opened against the framework by this round.** Every RED it produced was an
expectation of mine, corrected against observed behavior; the service did not regress once.

### E6 DECIDED — the 409 now names `permissionKey`, and the docs define it

Raised above as a design question, decided by the maintainer on 2026-09-02: the conflict
envelope must name a field the caller can read, and `permissionKey` is that name —
`permission` was considered and set aside because the read-side field already owns it.

Both emitters changed together, because otherwise the race between the pre-check and the
commit would decide the field name:

| where | before | after |
|---|---|---|
| `internal/domain/permission.go` — the service pre-check | `AddNotification("Key", …)` | `AddNotificationMessage` with `Override: "permissionKey"` |
| `internal/infra/permission_repository.go` — the unique-index backstop | `Field: "key"` | `Field: "permissionKey"` |

`Override` is the framework's own seat for this (`domain/notification.go`: precedence
`Override > rendered Path > FieldName`), so the structured `Path` survives for diagnostics
and `LabelKey` keeps the translated `fieldLabel` the envelope already carried. The emission
now also passes the rejected pair, so the answer says WHICH permission collided.

Documented where a consumer meets it: the four `permission_routes.go` OpenAPI descriptions
now state that the pair goes in as `resource` + `action`, comes back as `permission`, is
immutable on PATCH, and that a duplicate answers 409 on `permissionKey` = `resource:action`.

Suite: `E1c` now asserts `permissionKey`, and a new `E1d` asserts the echoed value
(`tenant:read`). **446 cases, 2 lanes, 0 RED.**

Two GAPS to raise upstream, written up in
[`finding-unique-wire-field-name.md`](finding-unique-wire-field-name.md): `unique:` has no
wire-name target in the spec language (hence the hand edit + `adopt`), and the GraphQL
schema carries no field descriptions at all, so the same explanation has no seat there.

### Residue, as §2 promised

The throwaway database `authcore_qa_db`, dropped and recreated by whichever lane runs next.
`authcore_db` was never written to. Inside the throwaway database the lane leaves its own
fixtures (live and archived) and the rotated bootstrap password. **The 39 seeded catalog
rows were read (`G17`) and collided with (`E1`, `E3`, `J7`) — never written to.**
