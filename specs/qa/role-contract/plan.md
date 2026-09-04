# QA contract suite — `role-contract`

- **Status:** APPROVED (maintainer, 2026-09-03) — §8 records the decisions.
- **Scope:** the `Role` aggregate and its `role_permissions` collection, surfaces REST +
  GraphQL, plus the role rows of the security lane. It **extends**
  `specs/qa/tenant-contract/plan.md` and `specs/qa/permission-contract/plan.md` (both
  APPROVED, both GREEN 2026-09-03) and the same `qa/run.sh`.
- **Pin:** omnicore **v0.72.1** (`go list -m github.com/ClaudioSchirmer/omnicore`).
- **Profile under test:** `dev` config shape via the suite-owned `qa/microservice.qa.yaml`,
  engine tag `postgres`, **no** transport tag.
- **Plan lives here; the runnable suite lives at `qa/` in the project root.** Neither is
  ever added to `.gitignore`.

Everything below is DERIVED — from `specs/omnicore-gen/role.omnicore.yaml`, from
`specs/scaffold-entity/role/spec.md` (Status: APPROVED), from the code actually mounted,
from the tracked seed migration, and from the pin's own docs (`status-mapping`,
`auto-handlers`, `auto-query-handlers`, `read-joins`, `relational-view`, `auth-middleware`,
`authz-seams`, `graphql`, `rules-dsl`). **No expectation in this file was read off a live
response.** The service was never called to write it.

### What makes this round different from the two before it

Tenant and Permission could both write `N/A — structurally absent` against authorization
layers 2 and 3. **Role cannot.** It is the first aggregate in this service that is owned by
a tenant, so the owner-check in `BuildRules` and the tenant filter in `ToCriteria` both
exist, and §3 turns two long-standing skips into real cases.

It is also the first aggregate carrying **both kinds of read join at once**, which is the
shape half this matrix is built around:

| | the ROOT join → `Tenant` | the CHILD join → `Permission` |
|---|---|---|
| declares | `TenantWorkspace`, `TenantStatus` | `Resource`, `Action` |
| served on the wire | **yes**, as two fields | **no** — `hidden: true` |
| what the wire shows instead | the values themselves | `permission`, their rendering through `vos.PermissionKey.String()` |
| addressable in a criteria | **yes** — filterable and sortable | **no** — load-only, the 1:N boundary |
| the case that proves it | `G5`/`G6` filter and order by them | `H7` filtering by one is a typed 400, `H12` selecting one is a typed 400, `C2` neither ever reaches a body |

One service, both halves, asserted side by side. Nothing earlier in this suite could do that.

### What this round INHERITS and does not repeat

The tenant round proved, once and service-wide: the whole 401 family (`S1`–`S10`), the
public-route split in both directions including the GraphQL introspection bypass edges
(`S11`–`S15`), and the middleware tenant-claim gate (`S21`/`S21b`). Those are properties of
the middleware, not of an aggregate. §3 below covers only what is genuinely per-entity.

---

## 0. Surface inventory — what the service DECLARES

**Storage** — flat table `roles`, managed `revision / created_at / updated_at / deleted_at`,
plus ONE collection, `role_permissions`, with the same four managed columns.
`delete.root: soft`; no hard-delete verb anywhere, on the root or on the child.

**The collection is edit-strategy B — and it is a PAIR, not the usual trio.** A
`RolePermission` has no editable field: its single column *is* its identity, so "update this
grant" has no meaning. Two child ops are generated, GRANT and REVOKE, and there is **no
per-child unarchive** — re-granting a revoked permission is a fresh GRANT that mints a fresh
child id.

**Wire fields, and the asymmetry between the write shape and the read shape**

| layer | carries |
|---|---|
| insert body | `tenantID` (OPTIONAL), `key`, `name`, `description`, `permissions: [{permissionID}]` |
| patch body | `key`, `name`, `description` — and **nothing else** |
| write response (insert / patch) | `id`, `tenantID`, `key`, `name`, `description`, `permissions: [{id, permissionID}]` |
| read document (by-id / listing row) | `id`, `tenantID`, `key`, `name`, `description`, `createdAt`, `updatedAt`, `tenantWorkspace`, `tenantStatus`, `permissions: [{id, permissionID, permission}]` |
| GRANT response | `{roleId, rolePermission: {id, permissionID}}`, 201 |
| REVOKE response | none, 204 (GraphQL answers `{success}` — a field must answer something) |

**`permission` is on the READ entry and NOT on the WRITE entry.** The derivation
(`ComputeRoleRolePermissionPermission`, `internal/application/queries/utils/role_computed_manual.go`)
runs below the web boundary on the read side, once per entry; the command result carries the
stored column alone. Both halves are asserted — a `permission` appearing in a 201 body would
mean the derivation moved somewhere it must not be.

**`tenantID` is optional on insert and absent from patch, and both are deliberate.** The
yaml declares `assignedFrom: identity-claim` + `claim: tenant_id` + `bypassMaySet: true`:
absent means *mine*, and a `*:*` operator may name another tenant. It is out of the patch
body because a role does not change tenant by being edited — which is what makes
`RoleTenantIsImmutableNotification` unreachable through the wire (§1b `RL7`).

**Modes** — `[display, insert, update, archive]`. **No `unarchive`, no delete**, and the
spec argues why at §5: a role is granted to users and groups, and those grants point at an
id that does not change, so one `unarchive` call would silently re-authorize everyone
holding it with an audit line reading "restored".

**Routes mounted** (`internal/web/role_routes.go`), each behind `RequirePermission`:

| verb | route | success | permission |
|---|---|---|---|
| insert | `POST /roles` | 201 + body | `role:insert` |
| patch | `PATCH /roles/:id` | 200 + body | `role:update` |
| archive | `PATCH /roles/:id/archive` | 204 | `role:archive` |
| list | `GET /roles` | 200 + page envelope | `role:read` |
| by id | `GET /roles/:id` | 200 + document | `role:read` |
| **grant** | `POST /roles/:id/permissions` | **201** + entry | **`role:grant`** |
| **revoke** | `PATCH /roles/:id/permissions/:rolePermissionId/archive` | **204** | **`role:grant`** |

**Seven, and GraphQL mirrors exactly these seven** — `roles`, `role`, `createRole`,
`patchRole`, `archiveRole`, `addRolePermission`, `archiveRolePermission` — same handlers,
same permissions. There is no `unarchiveRole` and no `PATCH /roles/:id/unarchive`.

`role:grant` is its own verb, decided 2026-08-28: a principal holding `role:update` alone
may relabel a role and gets **403** on both child routes. That split is the
privilege-escalation surface, and §3 asserts it.

**Read backing** — `RelationalView("roles", repo.Loader)` (`internal/infra/views/role_view.go`),
`read.backing: relational`. Therefore **read-your-writes**: every read-back in this suite is
IMMEDIATE, and a case that only passes after a retry is itself RED. No Mongo, no CDC, no
poll, no drain. The view declares nothing about the joins — it inherits both from the
repository's `WithJoins`, so a service reading through `repo.Loader` sees exactly what the
endpoint sees.

**Declared read controls** — `FindRolesRequest` declares `first, last, after, before,
orderBy, fields, onlyTotal, includeArchived`. **`search` is NOT declared** (deliberately: a
relational-served view answers free text with a typed 400, so declaring it would advertise a
capability the server refuses). `FindRoleByIDRequest` declares `includeArchived` and nothing
else.

| field | filter operators | orderable |
|---|---|---|
| `tenantID` | eq, in | asc, desc |
| `key` | eq, ne, in, startswith, istartswith, contains, icontains | asc, desc |
| `name` | eq, in, startswith, istartswith, contains, icontains | asc, desc |
| `description` | contains, icontains | **—** |
| `tenantWorkspace` *(root join)* | eq, in, startswith, istartswith, contains, icontains | asc, desc |
| `tenantStatus` *(root join)* | eq, in | asc, desc |
| `createdAt` / `updatedAt` | gte, lte | asc, desc |
| `permissions.*` *(the collection)* | **—** | **—** |

Two asymmetries earn their own cases: `ne` is declared on `key` and **not** on `name`;
`description` filters on `contains`/`icontains` while being orderable in **neither**
direction. Both are `H`-family rows.

> **The two join rows were added in THIS round** — see §8 `Q1`. `spec.md` §2 has said since
> 2026-08-24 that the owner's join fields are "filterable and sortable", which is the
> property a ROOT join has; `read.byParams` never declared them, so the promise was prose
> and `?tenantWorkspace.eq=acme` was a typed 400. The maintainer's call at this gate was to
> CLOSE the gap rather than record it. The yaml was amended, the entity regenerated (one
> file changed, `find_roles_by_params.go`), `go build -tags postgres ./...` is green, and
> `spec.md` §9 now carries the two rows with the supersession dated.

Page ceiling: no `query:` block in the yaml and no per-view override →
`bootstrap.FrameworkDefaultMaxLimit = 100`, the same cascade the permission round resolved
and asserted. `H14` asserts 100.

**Read joins — both kinds, and both halves of each are assertable**

```
RoleRepository.WithJoins(
  read.InnerJoin(schemas.TenantSchema()).On("tenant_id")          -- ROOT
    → TenantWorkspace ← tenants.workspace       served · filterable · sortable · selectable
    → TenantStatus    ← tenants.status          served · filterable · sortable · selectable
  read.InnerJoinInChild(schemas.PermissionSchema()).To(...)       -- CHILD, on permission_id
    → Resource        ← permissions.resource_name   hidden · load-only · feeds the derivation
    → Action          ← permissions.action_name     hidden · load-only · feeds the derivation
)
```

`inner` is safe on both because each foreign key is `NOT NULL` and FK-backed. Inside a
child an inner join drops the ENTRY rather than the aggregate, which is one more reason the
`NOT NULL` + FK pair is what makes it legal.

**The rules-only half is asserted, and it is the case nobody writes.** `Resource` and
`Action` exist to feed `permission` and for nothing else: they appear in **no** response
body, on **no** surface, under **no** `?fields=` selection. `C2` is that case.

**Infra posture** — Postgres 17 only. No Mongo, no broker, no CDC relay, no
`integration_events` table, no export surface, no gRPC transport.

**The baseline is SEEDED, and it is one row.** `migrations/postgres/0012_bootstrap_seed_manual.up.sql`
inserts the `master` tenant (`01990000-0001-…-000000000001`), the `master` role
(`01990000-0002-…-000000000001`) inside it, and exactly one grant binding that role to the
`*:*` catalog row (`01990000-0000-…-000000000000`). So on a fresh database `roles` holds
**1** active row and `role_permissions` holds **1**, and `G14` states both as a contract
derived from the tracked migration rather than from an answer.

That seeded role is also the reason the wildcard exists at all: `no-wildcard-grant` refuses
`*:*` on any role through the API with **no caller exempt**, so a migration is the only way
a super-admin can come into existence. §1b `RL1` proves the refusal against the strongest
possible caller.

**Security posture** — `auth.mode: jwt`; `authorization.enabled: true`;
`authorization.tenant.required: true`; `auth.issuer.enabled: true`, so the service mints its
own tokens and §3c needs nothing invented.

**Identity-derived rules on Role: THREE, and this is the first aggregate that has any.**

| layer | seam | code |
|---|---|---|
| 1 | `RequirePermission` on all seven routes, both surfaces | `role_routes.go` |
| 2 | `refuseForeignTenant` under `IfInsertOrUpdate` **and** `IfArchive` | `internal/domain/role.go:127-128, :223` |
| 2 | `CallerDoesNotHoldPermission` per added grant (R9a) | `role_rules_manual.go` |
| 3 | `ToCriteria` injects `Filter["TenantID"] = id.TenantID()` unless `IsSuperAdmin()` | `find_roles_by_params_query.go:63`, `find_role_by_id_query.go:68` |

`Restrict` — **none**. `spec.md` §9: *"Every field a caller may see the row at all for, they
may see entirely. Row-level isolation does the work here."* So the column-absence family is
`N/A` and §3 says so rather than implying coverage.

**Existing `qa/`** — four lanes plus `lib.bash` and `microservice.qa.yaml`. This round
MIRRORS their conventions and EXTENDS them; it starts no parallel style and adds no second
runner.

---

## 1. Coverage matrix — the framework's promises

Every family is a real case in `qa/role.sh` unless marked `N/A`. Assertions are always
**status + notification KEY (+ field where the pin names one)**, never prose. Every request
pins `Accept-Language: en`. The lane runs as the bootstrap admin (`*:*`), so nothing here is
blocked by row scope; §3 and §1b are where a scoped principal does the work.

### B — Happy path, one per served verb (REST)

`B1` insert with two grants → **201**; body carries `id, tenantID, key, name, description,
permissions[{id, permissionID}]` and **`permission` is ABSENT** from every write entry ·
`B2` by-id → **200**, each entry now carrying `permission` rendered as `<resource>:<action>` ·
`B3` list filtered to the lane's rows → **200**, exactly 1 row ·
`B4` patch `name` + `description` → **200**, both changed, `key` and `tenantID` unchanged ·
`B5` GRANT a third permission → **201**, body `{roleId, rolePermission:{id, permissionID}}`,
visible on the next read-back ·
`B6` **REVOKE one grant → 204, and the ROOT IS STILL ACTIVE** with its remaining grants
intact and the revoked one gone from the read. This is the model's canonical trap, called
out twice in `spec.md` §3: *"the root-archive auto handler is instantiated exactly ONCE per
surface — wiring it to the child REVOKE route type-checks, boots, answers 200, and archives
the entire role."* Nothing else in this matrix would notice ·
`B7` archive the root → **204** ·
`B8` **the absent verb, both surfaces**: `PATCH /roles/<id>/unarchive` matches no route →
**404** `RouteNotFoundNotification`; `unarchiveRole` is not a field in the GraphQL schema →
a validation error, not a 404 envelope. `unarchive` is absent from `Modes()` **and**
unmounted, which is the 404 arm of the three-way split.

Every read-back is IMMEDIATE. A case needing a retry is a failure, not a lag.

### C — Golden-record round-trip, and the hidden-part leak case

`C1` one role exercising **every declared field**, written then read back field-by-field on
REST by-id, on the REST listing row, and on the GraphQL node. `tenantWorkspace` and
`tenantStatus` carry the owning tenant's actual handle and status — the ROOT join reaching
the wire. `createdAt`/`updatedAt` present and RFC3339-parseable; `deletedAt` **absent** from
every body on every surface. Each entry carries `id`, `permissionID` and `permission`, and
`permission` is byte-identical to the `resource:action` of the catalog row `permissionID`
addresses.

`C2` — **the case nobody writes.** `resource` and `action` appear in **NO** response body on
**NO** surface: not in the insert 201, not in the GRANT 201, not in the patch 200, not in the
by-id document, not in a listing row, not in the GraphQL node, not under
`?fields=permissions.permission`, and not under `?includeArchived=true`. A rules-only join
field that leaks is a silent regression every other family here passes over.

### D — Validation 422 (asserting the KEY and the FIELD)

| case | request | expected key | field |
|---|---|---|---|
| `D1` | `key: ""` | `RequiredFieldNotification` | `Key` |
| `D2` | `key: "Billing"` (uppercase) | `InvalidRoleKeyNotification` | `Key` |
| `D3` | `key: "a"` (below the 2-rune floor) | `InvalidRoleKeyNotification` | `Key` |
| `D4` | `key: "bil--ling"` (doubled hyphen) | `InvalidRoleKeyNotification` | `Key` |
| `D5` | `key: "-billing"` · `"billing-"` (leading/trailing hyphen) | `InvalidRoleKeyNotification` | `Key` |
| `D6` | `key: "aaaab"` (run of 4 identical) | `InvalidRoleKeyNotification` | `Key` |
| `D7` | `key:` 65 runes | `InvalidRoleKeyNotification` | `Key` |
| `D8` | `name: ""` | `RequiredFieldNotification` | `Name` |
| `D9` | `description: "short"` | `InvalidDescriptionNotification` | `Description` |
| `D10` | `permissions: [{permissionID: "tatu"}]` | `InvalidIDUUIDNotification` | the entry's id |
| `D11` | `key: "billing-manager"`, hyphenated, accented description | **201** — positive control | — |

`D1` and `D8` assert the FRAMEWORK's required-field notification rather than each VO's own,
because both VOs short-circuit on empty (`role_key.go:56`, `display_name.go:49`) — a case
asserting `InvalidRoleKeyNotification` there would be asserting a branch the code cannot take.

**`D12`/`D13` — the guard barrier, which is a family of its own.** `TenantID` carries a
`valueObject` rule with `guard: true`, so `domain.ID.IsValid` runs FIRST and
`r.StopIfInvalid()` ends the whole pass:

- `D12` `tenantID: ""` → **422** `InvalidIDUUIDNotification` (`uuid.Parse` refuses the empty
  string and a non-UUID through the same call, so both inputs carry the SAME key) ·
- `D13` `tenantID: "tatu"` **together with** `description: "short"` → **422** reporting the
  **owner ALONE**. The barrier ends the pass including the automatic value-object validation
  and every collection, so a second violation in the same body must NOT appear. This is the
  case that proves the barrier is a barrier and not merely one more rule — and the bug it
  guards is documented: without it the bad owner reached the uniqueness pre-check, bound to
  a UUID column, and the probe panicked into a **500** on a request whose problem is plain
  validation.

### E — 409, and what "unique within the tenant" means

`E1` the same `key` twice in one tenant → **409** `RoleKeyAlreadyExistsNotification`,
`semantic: "Conflict"`, field `Key` ·
`E2` **the same `key` in a DIFFERENT tenant → 201.** The constraint is
`(tenant_id, role_key)`, not `role_key` — without this control `E1` would pass just as well
for an over-broad global index, which is exactly the regression that would make two
customers collide ·
`E3` a patch that does not move the key does not self-collide (`excludeSelf`) → **200** ·
`E4` archive a role, then insert the same key again → **201** with a NEW id, the archived
row still readable under `?includeArchived=true`. `unique.scope: active-only`, and with no
unarchive verb this is the only route back ·
`E5` the same `permissionID` twice inside ONE insert body → **409**
`RoleAlreadyGrantsPermissionNotification` ·
`E6` GRANT a permission the role already holds → **409**, same key. This is the path where
`IsSameBusinessIdentity` over `PermissionID` alone is load-bearing: the entry carries three
fields and two are join fields that are blank on a freshly added entry, so a
compare-all-fields identity would answer "different" and the duplicate guard would fail open ·
`E7` REVOKE a grant, then GRANT the same permission again → **201** with a **NEW child id**.
The child's unique index is `active-only` and there is no per-child unarchive, so a fresh
add is the only way back and it reads correctly in the audit trail.

`SemanticStateConflict` — **N/A**, the same derivation as both earlier rounds: no
state-conflict notification is declared, no verb carries a revision precondition, and
archive misuse resolves as 404 through `LoadForWrite`.

### F — Archive round-trip (kept-but-hidden; no `DeleteOnArchive`; **no unarchive**)

`F1` archive → by-id → **404** `RecordNotFoundNotification` ·
`F2` by-id `?includeArchived=true` → **200**, with its grants still readable — a retired role
must stay auditable, and with no unarchive verb this listing is the only way to see one ·
`F3` the listing hides it; `?includeArchived=true` reveals it ·
`F4` archive an already-archived role → **404** ·
`F5` GRANT onto an archived role → **404** — the write loads through `LoadForWrite`, which
does not see it ·
`F6` REVOKE on an archived role → **404**, same reason.

**Child stamp-scoped unarchive — `N/A`, and stated rather than skipped.** The family exists
to prove that a child removed on its own BEFORE the root's archive stays archived after the
root comes back. This root **never comes back**: no `unarchive` mode, no route. There is no
restore for a stamp to be scoped to.

### G — Read vocabulary, the two join fields, and the pagination envelope

`G1` `?key.` — `eq` · `ne` · `in` · `startswith` · `istartswith` · `contains` · `icontains` ·
`G2` `?name.` — `eq` · `in` · `startswith` · `istartswith` · `contains` · `icontains` ·
`G3` `?description.contains=` · `.icontains=` ·
`G4` `?tenantID.eq=` · `.in=` ·
`G5` **`?tenantWorkspace.` — `eq` · `in` · `startswith` · `istartswith` · `contains` ·
`icontains`.** The ROOT join reaching a criteria: *"the roles of `acme-comercio`"* answered
without a second call, over a column that lives in another table. This is the round's
headline capability and it did not exist before this gate ·
`G6` **`?tenantStatus.eq=` · `.in=`** — same seam, over an enum ·
`G7` `?createdAt.gte=` + `.lte=`, `?updatedAt.gte=` + `.lte=` ·
`G8` `?orderBy=` over `key` / `-key` / `name` / `-name` / `tenantID` / `-tenantID` /
**`tenantWorkspace` / `-tenantWorkspace` / `tenantStatus` / `-tenantStatus`** / `createdAt` /
`-createdAt` / `updatedAt` / `-updatedAt` — fourteen, because every one of those seven
declares both directions, and two of them are columns of another table ·
`G9` `?fields=key` → `key` alone · `?fields=tenantWorkspace` → the join value alone ·
`?fields=permissions.permission` → the collection carrying that entry field and nothing else ·
`G10` `?onlyTotal=true` → `totalCount` only, no `data`, no cursors ·
`G11` `?last=2` alone → the TAIL window ·
`G12` **envelope truthfulness as a BICONDITIONAL**, the correction the tenant round earned
(`application/queries/view_reader.go:136` — *EndCursor is set exactly when HasNextPage,
StartCursor exactly when HasPreviousPage*). With a known lane count and `?first=2`: assert
the biconditional on every page, echo `endCursor` into `?after=` → page 2 DISJOINT from page
1 with `hasPreviousPage == true`, walk back with `?before=` → page 1 again ·
`G13` `?includeArchived=true` raises `totalCount` by exactly the number of archived rows ·
`G14` **the seeded baseline is a contract**: on a freshly migrated database, read as the
`*:*` admin, `GET /roles?onlyTotal=true` answers **1**, and `GET /roles/<master role id>`
carries exactly **one** grant whose `permission` renders `*:*`. Both derived from the tracked
seed migration, never from a live answer.

`?search=` — **N/A as a capability case**: the DTO does not declare it, so the opt-in gate
answers first (`H6`) and no engine is ever consulted.
`UnsupportedCapabilityNotification` is therefore unreachable on this entity, exactly as on
Tenant and Permission.

### H — Rejected reads: the whole typed-400 guard family

All **400**, key `SchemaViolationNotification` unless stated. The field named is the **wire
token**, not the bare field — the lesson the permission round paid for (`auto-query-handlers`:
*"the canonical SchemaViolationNotification on the wire token"*).

| case | request | field named |
|---|---|---|
| `H1` | `?bogus=1` | `bogus` |
| `H2` | `?key.gte=x` (operator outside its allowlist) | `key.gte` |
| `H3` | `?name.ne=x` — **declared on `key`, NOT on `name`** | `name.ne` |
| `H4` | `?description.eq=x` — `contains`/`icontains` only | `description.eq` |
| `H5` | `?tenantStatus.contains=x` — the enum declares `eq`/`in` alone | `tenantStatus.contains` |
| `H6` | `?search=x` (a reserved control the DTO never declared) | `search` |
| `H7` | **`?permissions.resource.eq=x` · `?permissions.permission.eq=x` · `?permissions.permissionID.eq=x`** — the CHILD join's fields, the computed entry field, and the entry's own stored column: none is addressable in a criteria | the wire token |
| `H8` | `?orderBy=description` — filterable, orderable in neither direction | `orderBy[description]` |
| `H9` | `?orderBy=permissions.permission` — the 1:N boundary again, on the sort side | `orderBy[permissions.permission]` |
| `H10` | `?orderBy=id` (declarable, deliberately not declared) | `orderBy[id]` |
| `H11` | `?orderBy=bogus` | `orderBy[bogus]` |
| `H12` | **`?fields=permissions.resource` · `?fields=permissions.action`** — `hidden`, feeding the derivation, never SELECTABLE | `fields[…]` |
| `H13` | `?fields=bogus` · `?fields=deletedAt` | `fields[…]` |
| `H14` | `?first=101` | `LimitExceededNotification`, effective max **100** |
| `H15` | `?first=0` · `abc` · `-5` | `first` |
| `H16` | `?first=2&last=2` · `?first=2&before=X` · `?last=2&after=X` · `?after=X&before=Y` | the backward-side key |
| `H17` | `?onlyTotal=true` with `&first=` / `&orderBy=` / `&fields=` / `&after=` | `onlyTotal[<conflict>]` |
| `H18` | `?onlyTotal=true` + a filter, and + `&includeArchived=true` | **200** — counting a filtered subset is the point |
| `H19` | `?after=not-a-cursor` | `after` |
| `H20` | a cursor issued with no `orderBy`, replayed with `&orderBy=key` | the structural check |
| `H21` | the same cursor replayed with `&includeArchived=true` | the context-hash check |
| `H22` | `?includeArchived=1` · `?onlyTotal=` (empty) | booleans take exactly `true`/`false` |
| `H23` | by-id gate: `?onlyTotal=false` · `?fields=key` | presence gates an undeclared control |
| `H24` | by-id `?includeArchived=true` | **200** — positive control for the one it declares |
| `H25` | `?createdAt.gte=not-a-date` | **400** `InvalidFilterValueNotification` (pin ≥ v0.70.0) |
| `H26` | `?tenantID.eq=lixo` — an identity column that declares `eq` | **400** `InvalidFilterValueNotification` |

`H26` is the case an older suite has no equivalent of: below v0.70.0 the same request was a
**500** on a relational backing and an empty `200` page on Mongo.

### I — Routing, not-found, and the by-id ADDRESS contract (pin ≥ v0.70.0)

`I1` `GET /roles/<unused uuid>` → **404** `RecordNotFoundNotification` ·
`I2` `DELETE /roles/:id` → **405** `MethodNotAllowedNotification` — proves no hard delete
exists · `I3` `POST /roles/:id` → **405** · `I4` `GET /roles/:id/archive` → **405** (the path
IS registered, under PATCH) · `I5` `PATCH /roles/:id/unarchive` → **404**
`RouteNotFoundNotification` (see `B8`) · `I6` `DELETE /roles/:id/permissions/:childId` →
**404/405**, whichever the router answers — what it must never be is 204 ·
`I7` GRANT onto a role id that addresses nothing → **404**.

**The by-id address family, split by VERB, on both surfaces:**

| case | request | expected |
|---|---|---|
| `I8` | `GET /roles/not-a-uuid` | **404** `UnknownIDAddressNotification` |
| `I9` | `PATCH /roles/not-a-uuid` | **400** `MalformedIDNotification` |
| `I10` | `PATCH /roles/not-a-uuid/archive` | **400** `MalformedIDNotification` |
| `I11` | `POST /roles/not-a-uuid/permissions` | **400** `MalformedIDNotification` |
| `I12` | `{ role(id: "not-a-uuid") }` | typed `UnknownIDAddressNotification` |
| `I13` | `mutation { archiveRole(id: "not-a-uuid") }` | typed `MalformedIDNotification` |

**`I14` — the CHILD address, which is NOT that contract and is derived separately.**
`PATCH /roles/<good uuid>/permissions/not-a-uuid/archive` → **404**
`RecordNotFoundNotification`. `ArchiveRolePermissionRequest.RolePermissionID` is a plain
`string` path field, not a `domain.ID`, so the framework's wire wrapper never inspects it;
the value reaches `Role.RemoveRolePermissionByID`, which answers the canonical not-found for
any id the collection does not carry. The distinction is worth a case precisely because the
two ids sit in one URL and answer differently.

Mode-missing-with-route-mounted **403** — **N/A**: `unarchive` is absent from `Modes()` AND
unmounted, so it lands on the 404 arm (`I5`). No route is mounted for any undeclared mode.

### J — GraphQL (handler invariance)

`J1` `roles(first: 2)` → `edges { node cursor } pageInfo totalCount`, node equal to the REST
listing row · `J2` `role(id:)` equals the REST by-id document field for field, **including
`tenantWorkspace`, `tenantStatus` and every `permissions[].permission`** · `J3` `createRole`
→ visible over **REST** immediately · `J4` `patchRole` → same · `J5` `archiveRole` → payload
`{ success }`, effect confirmed over REST · `J6` **`addRolePermission`** → the entry, visible
over REST · `J7` **`archiveRolePermission`** → `{ success }`, **and the root still active
over REST** — the GraphQL half of `B6` · `J8` `role(id: <unused>)` → HTTP 200 with
`errors[0].extensions.notificationKey == "RecordNotFoundNotification"` · `J9` duplicate key
via `createRole` → `RoleKeyAlreadyExistsNotification`, `extensions.semantic == "Conflict"` ·
`J10` `roles(search: "x")` → an **unknown argument**, a gqlparser validation error, not the
REST 400 envelope · `J11` `where:` / `orderBy:` over **`tenantWorkspace`** return the same set
and order as their REST twins — the root join queryable on this surface too · `J12`
`__typename` beside every selection answers identically (pin ≥ v0.72.1) · `J13` **selecting
`resource` or `action` on a `permissions` entry is an unknown-field validation error** — the
GraphQL half of `C2`; the schema must not carry a field the REST body hides · `J14`
`unarchiveRole` is not a field in the schema (see `B8`).

gRPC — **N/A**: no transport wired. Exports — **N/A**: none declared (`spec.md` §9).

### X — Suite meta

`X1` `GET /openapi.json` enumerates exactly the **SEVEN** role routes above, each carrying
its declared permission, and no eighth. A mismatch is a FINDING, not a silent reconciliation.

---

## 1b. Domain expectations — the business oracle

From `spec.md` §7/§10/§B, the `rules` block, `role_rules_manual.go`'s stated INTENT, and the
maintainer's answers at this gate. They live in the existing `qa/domain.sh`, appended after
the permission rows **and before `P6`**, which must stay the last case of the file.

**The maintainer named four families CRITICAL** (🔴): R9a/R9b, R5 with the Layer-3 read
scope, R6, and R4.

**Every negative case below needs a caller who is NOT a super-admin**, which the suite
provisions through the service's own documented flow and nothing else (§3c).

| # | rule, as stated | source | POSITIVE case | NEGATIVE case | what a fail means |
|---|---|---|---|---|---|
| **RL1** 🔴 | "No wildcard grant through the API — a permission with `*` in either part cannot be granted on any role, **and no caller is exempt**." | `spec.md` §7 R9b; `rules.manual: no-wildcard-grant`; **maintainer: CRITICAL** | the admin grants a concrete catalog permission → **201** | **the `*:*` super-admin** grants the seeded `*:*` row → **403** `CannotGrantWildcardPermissionNotification`. The strongest possible negative: if anyone were exempt it would be them, and the seed migration's own header says this refusal is why a migration is the only way a super-admin exists | the platform's own unrestricted grant becomes mintable through the API, and the audit trail reads as an ordinary grant |
| **RL1b** 🔴 | the wildcard rule runs BEFORE the escalation rule, and that order removes a panic input | `spec.md` §7 "R9a/R9b — the framework panic"; `identity.go:52` | — | the same request as `RL1` answers **403**, never **500**. `HasPermission` panics on any argument containing `*`, so a service that reordered these two rules would crash the request into a 500 on exactly the case the pair exists to stop | a security rule became a server error, which fails open in every log that only counts 4xx |
| **RL2** 🔴 | "Só pode conceder o que você tem, a não ser que você seja um `*:*`." | `spec.md` §7 R9a, §B Q3; **maintainer: CRITICAL** | the scoped principal grants `tenant:read`, which its own role carries → **201** | the same principal grants `permission:archive`, which it does not hold → **403** `CannotGrantUnheldPermissionNotification` | any principal who may touch a role can grant themselves the whole catalog |
| **RL2b** 🔴 | the super-admin exemption is free, not special-cased — `HasPermission` answers true for any CONCRETE permission when the claim set holds `*:*` | `spec.md` §7 R9a | the admin grants `permission:archive`, which no role of theirs names individually → **201** | — | without this the rule could pass for a service that refuses every grant, which is the mirror failure of the gate that refuses everyone |
| **RL3** 🔴 | "Tenant isolation binds WRITES: the row's tenant must equal the caller's claim, unless the caller is a `*:*` super-admin." | `spec.md` §7 R5, §10 Layer 2, §B Q4; **maintainer: CRITICAL** | the scoped principal creates a role in its OWN tenant, and omits `tenantID` entirely → **201** in that tenant (absent means *mine*) | the same principal creates one naming the **master** tenant → **403** `TenantMismatchNotification`, field `TenantID`. And archiving another tenant's role → **403**, because `refuseForeignTenant` runs under `IfArchive` too and the write side is not filtered | one customer writes into another's partition |
| **RL3b** 🔴 | "A by-id read of another tenant's role returns **404 rather than 403** — it does not exist for this caller, which leaks nothing about who else exists." | `spec.md` §10 Layer 3 | the principal reads its own role by id → **200**; the admin's listing DOES contain the master role | the principal reads the master role by id → **404**, and its listing NEVER contains it. **404, not 403** — a 403 here would confirm the row exists | either an isolation leak (a 200) or an existence oracle (a 403) |
| **RL3c** | the `*:*` holder crosses the row scope, which is what lets a platform operator support a customer | `spec.md` §10 Layer 3; `find_roles_by_params_query.go:63` | the admin's listing contains roles from tenants that are not the master one | — | a service that filtered everyone would pass `RL3b` while making support impossible |
| **RL4** 🔴 | "Every granted permission must exist in the catalog **and be active**. A retired permission comes back as a NEW row with a NEW id, so re-granting the old id is refused rather than silently honoured." | `spec.md` §7 R6; **maintainer: CRITICAL** | grant a live catalog id → **201** | grant a random UUID → **422** `PermissionNotInCatalogNotification`; **and grant the id of a permission the lane archived a moment earlier** → **422**, same key. The second half is the one that matters — it is the whole reason the grant stores the id and not the string | a role keeps granting something the platform retired, or a recreated permission silently re-attaches to an old grant |
| **RL5** 🔴 | "The owner tenant must exist, not be archived, and not be **suspended**. A `trial` tenant is a live customer and passes — 'unavailable' is not 'not active'." | `spec.md` §7 R4 (corrected 2026-08-24); **maintainer: CRITICAL** | create a role inside a **`trial`** tenant → **201**. This is the *plausible-mistake control*: a rule written as `Status != active` would refuse every trial signup, and only this case sees it | create inside a **`suspended`** tenant → **422** `RoleTenantDoesNotExistNotification`; inside an **archived** tenant → **422**, same key; naming a tenant id no row carries → **422**, same key | a suspended customer keeps minting the units that grant access, or every trial signup is refused |
| **RL6** | "The key is immutable after creation — it is what API callers and audit lines reference." | `rules: key-immutable`, `spec.md` §7 R1 | patch `name` + `description` → **200**, `key` byte-identical | patch `{"key": "<other>"}` → **422** `RoleKeyIsImmutableNotification`, field `Key` | a role silently changes the handle every audit line and every client already references |
| **RL7** | "A role never moves between tenants" — enforced STRUCTURALLY, not by a refusal | `spec.md` §7 R2; **maintainer's answer at this gate** | — | patch `{"tenantID": "<other>"}` → **200**, and the read-back `tenantID` is **UNCHANGED**. The door does not exist: `PatchRoleRequest` carries `key`, `name` and `description` alone. `RoleTenantIsImmutableNotification` is therefore **unreachable through REST and GraphQL** and **no case asserts it** — recorded here rather than discovered later, the same shape as the permission round's `P1` | a role changes owner through a field the DTO was believed not to carry |
| **RL8** | "At most **200** permissions in one role." | `rules: permissions-cap`, `spec.md` §7 R8; **maintainer: the cheap form** | a role carrying the whole live catalog minus the wildcard (**38** grants) → **201** | an insert carrying **201** distinct UUIDs → **422** with `TooManyPermissionsInRoleNotification` **present in the envelope**. Stated plainly: 201 `PermissionNotInCatalogNotification` keys ride with it, because the invented ids are in no catalog — the assertion reads the whole envelope (`has_key`), never only the first message. The clean alternative was seeding 201 real catalog rows and was **declined** as not worth ~202 requests | a role can bundle an unbounded set, and the per-entry probes it forces become an unbounded cost inside one write transaction |
| **RL9** 🔴 | "R6, R9a and R9b judge the entries a write **ADDS**, never the ones already stored." | `spec.md` §7 "What these three rules judge"; `GetAddedItemsOf` in `role_rules_manual.go` | archive a catalog permission that an existing role ALREADY grants, then `PATCH` that role's **name** → **200**. A rule re-judging stored entries would answer 422 on a request whose only change is a label | — the refusals are `RL2`/`RL4`, which fire on ADDED entries in the same run | a permission the platform retires makes every role holding it unwritable, and a caller who lost a permission can no longer even REVOKE the others |
| **RL10** | "A REVOKE asks nothing, because it adds nothing." | same source | REVOKE a grant from a role whose OTHER grant points at an archived permission → **204** | — | revocation, the tool for a grant that stopped being acceptable, stops being reachable exactly when it is needed |

**Nothing is listed UNPROVEN in this round.**

**Two notifications are declared and UNREACHABLE through the wire, both stated rather than
asserted:** `RoleTenantIsImmutableNotification` (`RL7`, no `tenantID` in the patch body) and
`TenantMismatchNotification` **on the read path** (Layer 3 answers 404, never 403 — which is
`RL3b`'s whole point).

---

## 2. Data hygiene — inherited, with one new fixture class

**Unchanged from both approved rounds:** the throwaway `authcore_qa_db`, selected through
`OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml`, dropped and recreated at the start of every
run, `migrations.autoRun` rebuilding schema and seed. `microservice.dev.yaml` is never
touched and `authcore_db` is never written to. State reset is that one step and nothing races
it: no Mongo, no CDC, no projection to clear and no drain to wait on.

**The baseline is not empty and it is not the catalog's 39.** For `roles` it is **1** (the
seeded `master` role) and for `role_permissions` **1** (its wildcard grant). Every count this
lane asserts is scoped to its own rows with `?key.startswith=<lane prefix>`; `G14` is the one
case that speaks about the whole table and it says **1**, derived from migration 0012.

**The new fixture class: a SCOPED PRINCIPAL, provisioned through the service's own flow.**
`§1b` and `§3` need a caller who is authenticated, is NOT a super-admin, is bound to a tenant
of the suite's own making, and holds a CHOSEN bundle of permissions. Four calls, no invented
credential, no forged token:

1. as the admin — `POST /tenants` → the lane's own tenant `T`;
2. as the admin — `POST /roles` in `T`, granting `role:insert`, `role:update`,
   `role:archive`, `role:read`, `role:grant`, `tenant:read` and deliberately **not**
   `permission:archive` (which is what `RL2`'s negative needs);
3. as the admin — `POST /users` in `T` holding that role;
4. sign in, rotate the password (`PATCH /users/:id/password`), sign in again. **The rotation
   is not optional**: `user_rules_manual.go:125` sets `MustChangePassword = true` on every
   API-created user, and the first token is therefore restricted to `user:change-password`
   alone. A suite that skipped this step would test the restricted session and read its 403s
   as the role gate.

**A fixture trap worth naming before it bites.** `vos.RoleKey` refuses a run of 4 identical
runes and anything outside `^[a-z0-9]+(-[a-z0-9]+)*$`, so a run tag like `11115` would 422
every insert and take the lane RED for a fixture reason rather than a service one.
`qa/lib.bash` already carries `_slug`, which collapses runs to two; the role fixtures reuse
it rather than writing a second sanitizer.

**The order hazard, restated.** `qa/domain.sh`'s `P6` archives the `*:*` catalog row and
ends the run's ability to authenticate as an operator. Every role row of `§1b` is appended
**before** it, and `role` runs before `security` in `SUITES` (§5).

**Residue:** one throwaway database, now also holding the lane's tenants, roles and users.
Nothing is ever written to `authcore_db`.

---

## 3. Security — the role rows of `qa/security.sh`

**Where a valid token comes from: the service itself** (`auth.issuer.enabled: true`), through
`POST /auth/user/token`. The super-admin is the tracked seed's bootstrap account; the scoped
principal is provisioned as §2 describes. Nothing is invented, and no token is forged except
the ones that exist to be refused (all of which the tenant round already owns).

**3a — the 401 half: INHERITED.** `S1`–`S10` prove the middleware's token rules, and the
middleware does not know which route it is guarding. One row is added, because a route that
was never gated at all would still pass every one of them: `SR1` `GET /roles` with no
`Authorization` header → **401** `MissingAuthorizationNotification`.

**3b — the public-route half: INHERITED** (`S11`–`S15`, exactness neighbours and the GraphQL
introspection bypass edges included). No role route is declared public, and `SR1` is the
assertion that none is.

**3c — the 403 half. All three layers, for the first time in this suite.**

**Layer 1 — the gate, per verb AND per surface.** A route gated on REST is not thereby gated
on GraphQL, and a route can lose its gate alone.

| case | token | request | expected |
|---|---|---|---|
| `SR2` | a principal holding no role grant | `GET /roles` | **403** `MissingPermissionNotification`, field `permission`, value **`role:read`** |
| `SR3` | same | `GET /roles/:id` | **403**, value `role:read` |
| `SR4` | same | `POST /roles` | **403**, value `role:insert` |
| `SR5` | same | `PATCH /roles/:id` | **403**, value `role:update` |
| `SR6` | same | `PATCH /roles/:id/archive` | **403**, value `role:archive` |
| `SR7` | same | `POST /roles/:id/permissions` | **403**, value **`role:grant`** |
| `SR8` | same | `PATCH /roles/:id/permissions/:childId/archive` | **403**, value **`role:grant`** |
| `SR9` | **the scoped principal, holding `role:update` and NOT `role:grant`** | the two child routes | **403**, value `role:grant` — the decision of 2026-08-28 made visible: "may relabel the role" and "may change what the role can do" are separately grantable, and the second is the escalation surface |
| `SR10` | the admin | each of the seven | **2xx** (reads 200; writes aimed at an id that addresses nothing must reach the HANDLER — a **404**, never a 403 — which is what proves the gate opened) |
| `SR11` | the ungranted principal | `roles`, `role`, `createRole`, `patchRole`, `archiveRole`, `addRolePermission`, `archiveRolePermission` | the typed 403 in the **GraphQL** idiom, one per field |
| `SR12` | the admin | the same seven fields | **ok** — the complement, per surface |

`SR9` needs a SECOND scoped principal, holding the same bundle minus `role:grant`. It is one
more role and one more user through the same four-call fixture, and it is the only way that
verb split can be seen.

**Layer 2 — an identity-derived rule in `BuildRules`.** Proven by two calls that differ only
in **who is asking**:

| case | request | expected |
|---|---|---|
| `SR13` | the scoped principal creates a role naming the **master** tenant | **403** `TenantMismatchNotification`, field `TenantID` |
| `SR14` | the **same body** sent by the admin (`*:*`) | **201** — the bypass, which is what lets a platform operator support a customer |
| `SR15` | the scoped principal archives a role belonging to another tenant | **403** `TenantMismatchNotification` — `refuseForeignTenant` runs under `IfArchive`, and the write side is not filtered by `ToCriteria`, so the row loads and the RULE is what refuses |
| `SR16` | the scoped principal grants a permission it does not hold | **403** `CannotGrantUnheldPermissionNotification` — the second Layer-2 rule, cross-referenced to `RL2` |

**Layer 3 — tenant row scoping.** An isolation leak answers **200**, which is exactly why it
needs its own cases rather than riding on a refusal:

| case | request | expected |
|---|---|---|
| `SR17` | the scoped principal lists roles | **200**, and the master role is **absent** from every page — asserted by id, not by count |
| `SR18` | the scoped principal reads the master role by id | **404**, **not 403**. A 403 would confirm the row exists to a caller who may not see it |
| `SR19` | the admin lists roles | **200**, and the master role **is** present — without this, a service that filtered everyone would pass `SR17` for the wrong reason |
| `SR20` | the middleware tenant-claim gate | **INHERITED** (`S21`/`S21b`) |

**`Restrict` — `N/A`, stated rather than skipped.** `spec.md` §9 declares no field-level read
authz: *"Every field a caller may see the row at all for, they may see entirely. Row-level
isolation does the work here."* There is no column to find absent for one caller and present
for another, no tabular export whose header could be pruned, and no `__typename` edge to
assert — that edge exists only where a restricted field is in the selection. Naming it here
is the honest form; asserting one would be inventing a rule.

**3d** — `N/A`: `auth.mode` is `jwt` and `authorization.enabled` is `true`, so neither
degenerate posture applies.

**3e — what this round does not claim.** Exactly the inherited set (`S1`–`S15`, `S21`), which
the same lane executes and reports its own counts for, so nothing reads as covered that was
not run. Nothing else in §3 is deferred.

---

## 4. Out of scope, named plainly

- **Load / performance / concurrency** — not covered. The uniqueness backstops (`roles`
  partial index, `role_permissions` partial index) exist precisely for the race the domain
  pre-check cannot see, and proving a race needs concurrent writers this suite does not have.
  ⚠️ Say so if you want it; it is a real gap and it is named rather than implied.
- **The per-entry probe cost.** `spec.md` §7 accepts up to 200 catalog round trips inside one
  write transaction, mitigated by a single walk and a memoised lookup. That is a performance
  property, not a contract, and `RL8` bounds it rather than measuring it.
- **UI** — `/docs` and `/graphql/ui` are asserted reachable by the inherited lane, never rendered.
- **Integration events — `N/A`, not skipped:** no `transport:` block, nothing published, no
  `integration_events` table.
- **Audit events** — the framework writes an `audit_events` row per write and it is provable
  in SQL. Deliberately out of this lane, as in both earlier rounds: cross-cutting, and it
  belongs in its own suite rather than duplicated per entity. ⚠️ Say so if you want it here.
- **`User → Role` and `Group → Role`** — this round provisions a user holding a role in order
  to get a scoped token, and asserts nothing about `user_roles` or `group_roles` as
  contracts. Those edges belong to their own rounds.
- **The remaining 4 entities** — `claim`, `client`, `group`, `user`.

---

## 5. Runner contract — what this round CHANGES

**Still exactly one entry point: `qa/run.sh`.** `./qa/run.sh` runs everything;
`./qa/run.sh role` runs a subset. No second runner, never one per entity. Everything in the
tenant round's §5 stands unchanged — root resolution from the script's own location,
fail-fast by default with `--all` for the exhaustive sweep, per-run namespacing of every
shared artifact, SIGTERM-only shutdown with a waited drain, `Accept-Language: en` on every
request, SKIPPED printed as SKIPPED, each lane exiting non-zero on any failure.

Three changes, all in the same commit that creates the new lane:

1. **`SUITES=(tenant permission role security domain)`** — `role` is added before `security`.
   `domain` stays LAST because `P6` burns the admin's access for the remainder of the run.
   The list stays explicit and in one place, and `ls qa/*.sh` minus `run.sh` must equal it
   exactly.
2. **`PLAN`** names all three plans — the report cannot claim to execute two documents when
   it executes three.
3. `qa/lib.bash` gains the role fixtures beside the tenant and permission ones — `role_body`,
   `create_role`, `permission_id_of <resource> <action>` (which resolves a seeded catalog id
   by its pair, so no id literal is hardcoded twice), and `provision_scoped_principal`, the
   four-call fixture of §2 including the mandatory password rotation. One library, extended —
   not a second one.

---

## 6. Report contract — `qa/qa-report.md`

Unchanged from the two approved rounds, which are implemented and proven: live rewrite after
every suite, the `EXIT INT TERM` abort stamp, the SKIP column kept separate, the failures
section carrying real response bodies and pointing at `qa/.logs/<run-id>/`. The matrix simply
grows a `role` row, and every declared suite still appears — one that never ran prints `—`,
never vanishes.

`qa/qa-report.md` and `qa/.logs/` remain run artifacts, **offered** as `.gitignore` lines at
hand-off and never added by this skill.

---

## 7. Artifacts this plan produces

| path | what |
|---|---|
| `qa/role.sh` | **new** — §1, the framework's promises on this entity |
| `qa/domain.sh` | **extended** — §1b `RL1`–`RL10`, inserted before `P6` |
| `qa/security.sh` | **extended** — §3 `SR1`–`SR19` appended |
| `qa/run.sh` | **edited** — `SUITES` gains `role`; `PLAN` names three plans |
| `qa/lib.bash` | **extended** — role fixtures + the scoped-principal provisioner |
| `specs/qa/role-contract/plan.md` | this document |

**Already applied at this gate, outside `qa/` — see §8 `Q1`:**

| path | what |
|---|---|
| `specs/omnicore-gen/role.omnicore.yaml` | `read.byParams` gains the two root-join filters and the two sort keys |
| `internal/web/requests/find_roles_by_params.go` | regenerated — the only Go file the change touched |
| `specs/omnicore-gen/lock.json`, `role.gen-report.md` | generator bookkeeping |
| `specs/scaffold-entity/role/spec.md` | four stale passages synchronised, each with a dated supersession |

Neither `qa/` nor `specs/qa/` is added to `.gitignore`.

---

## 8. Gate decisions (2026-09-03)

- **Q1 — the two root-join fields: the gap was CLOSED, not recorded.** `spec.md` §2 promised
  `TenantWorkspace` and `TenantStatus` were "filterable and sortable" and `read.byParams`
  declared neither. The maintainer's call: *"tu faz a correção para adicionar os dois filtros
  e aí faz a correção no spec e por fim faz os asserts."* Verified against the pin first —
  `read-joins.html` states a root join's fields are *"an ordinary field of the loaded entity,
  filterable, sortable, projectable and exportable"*, so the framework always offered it and
  only the spec was silent. `omnicore-gen check` accepted the amended yaml, `generate`
  updated **one** file, and `go build -tags postgres ./...` is green. `G5`, `G6`, `G8` and
  `J11` are the asserts. The complement stays true and is asserted beside it: the CHILD's
  join fields remain load-only (`H7`, `H9`, `H12`).
- **Q2 — the 200-permission cap takes the cheap form.** 201 invented UUIDs in one insert; the
  assertion reads the whole envelope for `TooManyPermissionsInRoleNotification` and the plan
  states out loud that 201 catalog-miss keys ride with it. Seeding 201 real catalog rows was
  declined as not worth ~202 requests.
- **Q3 — `RL7` asserts STRUCTURAL immutability.** `PATCH {"tenantID": …}` → 200 with the
  value unchanged, and `RoleTenantIsImmutableNotification` is recorded as unreachable through
  the wire with no case asserting it. Same shape as the permission round's `P1`.
- **Q4 — four families are CRITICAL:** R9a/R9b (escalation and wildcard), R5 with the Layer-3
  read scope, R6 (the granted permission exists and is active), and R4 (a suspended tenant
  gets no role, a `trial` one does).
- **A wording correction the maintainer made, kept because it is the rule itself.** The
  question described `*:*` as *"irrecusável"*; it is the opposite — **`*:*` is always
  refused, for every caller, with no exemption**. `RL1` is written against the super-admin
  precisely because that is where an exemption would hide.

**Four stale passages in `specs/scaffold-entity/role/spec.md` — CORRECTED in this round**
under the maintainer's standing instruction to fix prose at the source rather than only
report it. The document is `Status: APPROVED` and is the model authority, so each edit
records the supersession rather than quietly rewriting history:

1. **§2's per-grant read example** showed `resource`, `action` and `archivedAt` as wire
   fields. Both later amendments in the same list overrule it: `resource`/`action` became
   `hidden` on 2026-08-24 and `ArchivedAt` was removed from the child the same day. The
   example now reads `{id, permissionID, permission}`, verified against
   `internal/web/requests/dtos/role_permission.go`.
2. **§2's join-properties item and §9's "one real cost" paragraph** both wrote the filter as
   `?filter[permissions.resource][eq]=tenant`. The framework parses `?<field>.<op>=<value>`;
   the bracket form appears in no section of the v0.72.1 docs and never existed — the same
   correction `../permission/spec.md` took at its own QA gate.
3. **§9's "one real cost" paragraph also named the wrong notification.** It said
   `UnsupportedCapabilityNotification`, which is what a DECLARED capability raises when the
   engine cannot serve it. The listing DTO declares nothing under `permissions.*`, so the
   schema gate answers first and no engine is consulted: it is
   `SchemaViolationNotification`. A suite asserting the wrong key would have gone red against
   a correct service, which is the whole reason this was worth chasing.
4. **§2's heading said "No traversal to `Tenant` is declared"** while the same section's field
   table, added 2026-08-24, declares two fields reached through exactly that traversal. The
   paragraph's reasoning — a join predicate is always `fk = target.id`, and while the derived
   key existed `roles.tenant_id` matched nothing — is kept, reframed as the record of why it
   was refused until the derived key was removed.
5. **§9's operator table** gained `tenantWorkspace`, `tenantStatus` and the temporal pair
   (the last of which was always in the yaml and simply missing from the table).

---

## 9. Run record — 2026-09-03

`./qa/run.sh --all` · **GREEN 794 · RED 2 · SKIPPED 23**, five lanes.

| lane | pass | fail | skip |
|---|---:|---:|---:|
| `tenant` | 155 | 0 | 7 |
| `permission` | 172 | 0 | 6 |
| `role` | 243 | **2** | 4 |
| `security` | 106 | 0 | 4 |
| `domain` | 118 | 0 | 2 |

**Reconcile.** All 113 case families named in §1 exist in `qa/role.sh` and RAN — checked
family by family against the file. Every §1b row carries BOTH halves except `RL2b`,
`RL3c`, `RL9` and `RL10`, whose complements are the negatives of the row they control and
are stated as such in the matrix. Every §3 row `SR1`–`SR19` exists in `qa/security.sh`.
`ls qa/*.sh` minus `run.sh` equals `SUITES=(tenant permission role security domain)`
exactly.

**The suite can fail** — the mandatory meta-case. `tenant`'s `B1` was flipped by hand to
assert **200** where the service answers **201**; the run went RED, the runner exited
non-zero, fail-fast stopped after the first lane, the report's matrix showed `tenant` RED
with `permission`, `role`, `security` and `domain` as `—` rather than absent, the failures
section named the case with expected-vs-received and the real 201 body, and the footer read
`❌ RED — 1 of 5 suites`. The expectation was then restored.

### The RED is a FINDING about the framework, not a defect in the suite

`D12a` and `D12b` are left RED **deliberately**, and the cases were not weakened.

**The same field answers under two different names in one endpoint's 422 envelope.**

| raised by | code | wire `field` |
|---|---|---|
| `RoleTenantDoesNotExistNotification` (a domain rule) | `r.AddNotification("TenantID", …)` | **`tenantID`** — proven green by `RL5` |
| `InvalidIDUUIDNotification` (the framework's own id validation) | `domain.ID.IsValid` → `ctx.AddNotificationMessage` | **`TenantID`** — `D12a`/`D12b` |

Both start from the identical Go identifier `"TenantID"` on the identical field of the
identical entity. The divergence is in the framework's two emission paths, read at the pin:

- `NotificationContext.AddNotification` (`domain/notification.go:198`) writes a
  single-segment **`Path`**, and its own doc says *"in a root entity context… `"Name"`
  renders as `"name"`"* — the wire-name fold.
- `domain.ID.IsValid` (`domain/id.go:70`) calls `AddNotificationMessage` with
  **`FieldName`** instead — what `notification.go:163` itself calls *"the legacy form"* —
  and nothing folds it.

Every other field on this endpoint (`key`, `name`, `description`) comes back camelCased, so
a client branching on `field` must special-case the one notification the framework raises
for it. **Routed to the framework, not patched here.** The expectation stays `tenantID`
because that is the convention the rest of the same envelope keeps; if the maintainer rules
that `TenantID` is the intended contract on this path, the two assertions change and the
plan records the decision — but the suite does not move first.

### Five suite defects were corrected before this record — all mine, none a weakened case

Recorded because each is a shape that makes a suite quietly untrue.

1. **`jq`'s `fromdateiso8601` is narrower than RFC3339.** `C1`'s two timestamp cases piped
   the value through it; the builtin accepts only a `Z`-suffixed instant with no fractional
   part, while this service stamps from Postgres (`relational.clock: db`) and answers
   `2026-09-03T23:31:06.322534-04:00` — valid RFC3339 that the builtin refuses. The cases
   now assert the grammar itself. Asserting through the builtin would have pinned a
   narrower format than the contract, and would have gone red against a correct service.
2. **GraphQL sorting is a typed input, not the REST string.** `J1` and `J11` sent
   `orderBy: "key"`; the surface takes `orderBy: [{field: KEY, direction: ASC}]` over the
   reflected `RoleOrderField` enum (`graphql.html`, "Sorting"). Introspected rather than
   guessed. The fix earned an extra case for free — `J11c` now asserts the reflected enum
   is exactly the seven-field REST sort vocabulary, so the capability cannot come to exist
   on one surface alone.
3. **`RL8`'s positive fixture swept in a lane-created wildcard.** It excluded the seeded
   `*:*` id and nothing else, but `no-wildcard-grant` refuses a `*` in EITHER half and the
   permission lane's own `P3` positive creates a `<resource>:*` row. The case answered 403
   for a fixture reason that had nothing to do with the cap it exists to prove. It now
   excludes every row whose rendered token contains a `*`.
4. **Two assertions piped a possibly non-JSON body through `jq`.** `I6` (a routing refusal)
   and `RL1b` (a 500 check) used `assert_jq_true` for what is a plain shell comparison; a
   non-JSON body would have failed them for the parser rather than for the contract. Both
   are now `if`/`pass`/`fail`.
5. **A no-op assertion in `X`.** `assert_jq_true … "true"` could not fail. It is now a real
   count: all seven role operations must carry a `**Required permission:**` suffix, so a
   route mounted open is visible.

**Residue, as §2 promised:** one throwaway database, `authcore_qa_db`, dropped and
recreated at the start of every run — at rest it holds 28 roles, 83 role_permissions, 33
tenants and 5 users. `authcore_db` holds **0** rows written by this suite (verified:
`roles where role_key like 'qa-%'` → 0, same for `tenants.workspace` and `users.email`).
