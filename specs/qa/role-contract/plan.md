# QA plan — `role-contract`

- **Status:** APPROVED — maintainer (Cláudio Schirmer Guedes), 2026-09-07. *"bora, ta aprovado, toca ficha"* — the plan as a whole, including the four EDITED files of §5 and the two scoped principals of §2.
- **Suite slug:** `role-contract` — what this round proves: the whole wire contract of the
  `Role` aggregate and its `role_permissions` collection on both surfaces, the business rules
  its own specs and the maintainer state, and the two things that make this entity unlike
  every other one already proven — **it is the first aggregate this service OWNS BY TENANT**,
  so authorization layers 2 and 3 stop being `N/A`; and it is the first carrying **both kinds
  of read join at once**, so half the matrix lives on that contrast.
- **Written:** 2026-09-07
- **Pin:** omnicore **`v0.74.0`** · dialect postgres · read backing **relational**
  (read-your-writes — every read-back below is IMMEDIATE; a poll would itself be a failure)
- **Surfaces in scope:** REST + GraphQL (`surfaces.graphql.enabled: true`)
- **Plan destination:** `specs/qa/role-contract/plan.md` · **suite destination:** `qa/` at the
  project root · **verdict destination:** `qa/qa-report.md`
- **Prior rounds:** `specs/qa/tenant-contract/plan.md` (APPROVED 2026-09-06) and
  `specs/qa/permission-contract/plan.md` (APPROVED 2026-09-07). This round **EXTENDS the same
  `qa/run.sh`** — no second entry point, and neither approved plan is reopened.

## 0. Where every expectation below comes from

Nothing here was read off a running service. The service was never called while this plan was
written. The one exception is an ENUMERATION and never a value that becomes an expectation:
the suite reads `GET /openapi.json` at boot to cross-check the verb inventory.

| Source | What it settled |
|---|---|
| `specs/scaffold-entity/role/spec.md` (APPROVED 2026-08-20, superseded 2026-09-06) | the model; §A the eleven industry findings; §2 the fields and the two joins; §3 the collection as a PAIR of ops; §5 the one-way archive; §7 R1–R9b; §9 the read vocabulary; §10 the three authorization layers; §B Q1–Q5 |
| `specs/omnicore-gen/role.omnicore.yaml` | fields, the collection, both joins, notifications, modes, rules (declarative + manual), `service.facts`, `read.byParams`, `authz` |
| `internal/domain/role.go` · `role_rules_manual.go` · `vos/role_key.go` | the rules as implemented — the guard barrier on `TenantID`, `refuseForeignTenant` under `IfInsertOrUpdate` **and** `IfArchive`, and the single ADDED-entries walk behind R6/R9a/R9b |
| `internal/web/role_routes.go` | the **7** REST endpoints and the **7** GraphQL fields, and the permission literal on each |
| `internal/web/requests/find_roles_by_params.go` · `find_role_by_id.go` · `insert_role.go` · `patch_role.go` · `add_role_permission.go` · `archive_role_permission.go` · `dtos/role_permission.go` | the DTO opt-in gate: which controls each read serves, which operators each leaf admits, and **which fields each Response carries** — including the three archive stamps added 2026-09-06 |
| `internal/application/queries/find_roles_by_params_query.go:52-64` · `find_role_by_id_query.go:57-69` | `ToCriteria` forces `Filter["TenantID"] = id.TenantID()` unless `IsSuperAdmin()` — Layer 3, and the reason §3 has a row-scope block at all |
| `migrations/postgres/0012_bootstrap_seed_manual.up.sql` | the token source, the 39-row catalog, and the baseline this lane counts against: **1** role, **1** grant |
| `qa/microservice.qa.yaml` · `qa/run.sh` · `qa/lib/common.sh` | the posture this suite runs under and the helper API every new lane speaks |
| pin docs — `status-mapping`, `auto-handlers`, `auto-query-handlers`, `read-joins`, `relational-view`, `auth-middleware`, `authz-seams`, `graphql`, `rules-dsl`, `audit` | every status code, notification key and envelope shape asserted below |
| **the maintainer, asked 2026-09-07** (`AskUserQuestion`) | the archive-stamp family (§1 `H7`), §1b `RL11`, the audit extension (§4), and the form of the cap case (`RL8`) |

### 0a. The deleted round, and why this is a NEW plan rather than a restored one

A `role-contract` round existed: commit `84f1a86` (2026-09-03, pin **v0.72.1**, plan APPROVED,
`qa/role.sh` 760 lines, run recorded GREEN 794 / RED 2). Commit `30432cf` **deleted the whole
`qa/` tree and both plans**, and the suite was rebuilt from scratch as `tenant-contract` and
`permission-contract`. Nothing of that round is on disk.

It is read here as **evidence of what was decided**, never as an expectation to restore,
because **three model changes landed on 2026-09-06, after it** — every one of them adding a
field to the wire:

| change | what it adds | where it lands |
|---|---|---|
| `read.managed` gained `ArchivedAt` | `archivedAt` on Role's own by-id read and every listing row | `H2`, `H7`, `H9` |
| the ROOT join gained `archived_at` | `tenantArchivedAt` — the owning tenant's stamp | `H2`, `H7`, `H9` |
| the CHILD join gained `archived_at`, **not hidden** | `permissionArchivedAt` per grant — the entry's only account of a grant that has outlived what it points at | `H3`, `H7` |

The deleted plan's golden record asserted `deletedAt` **absent from every body**. At this pin
that assertion is false three times over, which is exactly why the round is re-derived rather
than replayed. A fourth change is a framework fix and is asserted as a regression guard in
`H4` — see the note there.

### 0b. What this round INHERITS and does not repeat

Proven once, service-wide, by the two approved rounds, against this exact posture:

- the whole **401 family** (`S3a.1`–`S3a.10`) — token shape, signature, `iss`, `aud`, `alg`,
  `exp`, and the expired-vs-invalid key split;
- the **framework's appended public surfaces** (`/docs`, `/openapi.json`, JWKS, the playground,
  the root redirect) and the **GraphQL introspection bypass with its four edges**
  (`S3b.7`–`S3b.16`);
- **direction 1** of the public-route split — every declared `publicRoutes` entry answers
  tokenless, and the exactness neighbours (`S3b.1`–`S3b.6`);
- the middleware **tenant-claim gate** (`authorization.tenant.required: true`);
- the **boot, hygiene and report contracts** (§2, §5, §6 of `tenant-contract`), reused verbatim.

What this round adds to the security lane is only what is Role-specific: §3 below.

---

## 1. Coverage matrix — the FRAMEWORK's promises

Entity **Role** × surfaces **REST** and **GraphQL**. Read backing **relational**, so every
write→read-back is immediate. Archive regime *kept-but-hidden* (no `DeleteOnArchive`), and
**one-way** — no unarchive mode, no unarchive route, no unarchive mutation.

Envelope asserted on REST: `errors[].messages[].notificationKey` + the HTTP status, and the
`field` where the pin promises one. On GraphQL: HTTP is **always 200** and the same key rides
`errors[].extensions.notificationKey`. Every request pins `Accept-Language: en-US`.

**The lane runs as the bootstrap admin (`*:*`)**, so nothing in §1 is blocked by row scope;
§1b and §3 are where a scoped principal does the work.

**Case prefixes.** `H` = `qa/role.sh` (REST), `J` = `qa/role_graphql.sh`. They are free: tenant
owns `F`, permission owns `E` (REST) and `G` (GraphQL).

### The shape that governs this whole matrix

`role_permissions` stores **one column** — `permission_id` — and the read returns **four
values** per entry: that id, the rendered `permission`, `permissionArchivedAt`, and the entry's
own id. `resource` and `action` are read across the foreign key, feed the derivation, and reach
**no response body anywhere**. Alongside it, the ROOT join publishes `tenantWorkspace`,
`tenantStatus` and `tenantArchivedAt`, which ARE served, filterable, sortable and selectable.
One service, both kinds of join, opposite contracts:

| | the ROOT join → `Tenant` | the CHILD join → `Permission` |
|---|---|---|
| fields | `TenantWorkspace`, `TenantStatus`, `TenantArchivedAt` | `Resource`, `Action` *(hidden)*, `PermissionArchivedAt` |
| served on the wire | **all three** | `permissionArchivedAt` only; the other two never |
| what the wire shows instead | the values themselves | `permission`, their rendering through `vos.PermissionKey.String()` |
| addressable in a criteria | **yes** — `tenantWorkspace`/`tenantStatus` filterable and sortable | **no** — load-only, the 1:N boundary |
| the case that proves it | `H8` filters and orders by them | `H9` filtering or selecting one is a typed 400; `H3` neither ever reaches a body |

A case that forgets which side of that table it is on proves nothing, so each family below says
which side it is on.

### H1 — Happy path, one per served verb

`Modes()` declares exactly `display, insert, update, archive` — **four modes, SEVEN routes**,
because the collection carries two verbs of its own. The inventory is cross-checked against
`GET /openapi.json` at boot (`H11`); a source-vs-openapi disagreement is a FINDING, not
something the suite reconciles.

| REST | GraphQL twin | Expected |
|---|---|---|
| `POST /roles` | `createRole(input:)` | **201** · `{id, tenantID, key, name, description, permissions[{id, permissionID}]}` |
| `GET /roles/{id}` | `role(id:)` | **200** · the full document |
| `GET /roles` | `roles(...)` | **200** · `data` + `pagination` / the Relay connection |
| `PATCH /roles/{id}` | `patchRole(id:, input:)` | **200** · the root as stored |
| `PATCH /roles/{id}/archive` | `archiveRole(id:)` | **204, NO BODY** / payload `{success, id}` |
| **`POST /roles/{id}/permissions`** | `addRolePermission(id:, input:)` | **201** · `{roleId, rolePermission:{id, permissionID}}` |
| **`PATCH /roles/{id}/permissions/{rolePermissionId}/archive`** | `archiveRolePermission(id:, input:)` | **204, NO BODY** / payload `{success}` |

**`H1.7` is the model's canonical trap and it is asserted as such.** `spec.md` §3 warns twice:
*"the root-archive auto handler is instantiated exactly ONCE per surface — wiring it to the
child REVOKE route type-checks, boots, answers 204, and archives the entire role."* So the
REVOKE case does not stop at the 204: it re-reads the root, asserts it is **still active**,
asserts the remaining grants are **still there**, and asserts the revoked one is gone. Nothing
else in this matrix would notice.

**`permission` is absent from every WRITE body and present in every READ body.** The derivation
runs in `FromQueryResult`, below the web boundary and on the read side only; the command result
carries the stored column alone. Both halves are asserted — a `permission` appearing in a 201
would mean the derivation moved somewhere it must not be.

### H2 — Golden-record round-trip

One role exercising **every declared field**, written then read back field-by-field on all four
read shapes (REST by-id, REST listing row, GraphQL node, GraphQL connection edge):

`id` · `tenantID` · `key` · `name` · `description` · `createdAt` · `updatedAt` · **`archivedAt`**
· `tenantWorkspace` · `tenantStatus` · **`tenantArchivedAt`** · `permissions[]` with
`{id, permissionID, permission, permissionArchivedAt}`.

- `tenantWorkspace` and `tenantStatus` carry the owning tenant's **actual** handle and status —
  the ROOT join reaching the wire over a column that lives in another table.
- `permission` is byte-identical to the `resource:action` of the catalog row `permissionID`
  addresses, for **every** entry.
- `createdAt`/`updatedAt` present and RFC3339-parseable **by grammar, not by `jq`'s
  `fromdateiso8601`** — that builtin accepts only a `Z`-suffixed instant with no fractional
  part, while this service stamps from Postgres and answers
  `2026-09-07T23:31:06.322534-04:00`. Asserting through the builtin would pin a narrower
  format than the contract and go red against a correct service. (The deleted round paid for
  this; it is recorded so it is not paid twice.)
- The three stamps are **`null` while their target is live**. Their stamped behaviour is `H7`.

### H3 — "One column in, four out": the child-join doctrine, both halves

**The positive half.** Every served entry carries the id, the render, and the counterpart's
stamp; `?fields=permissions.permission` and `?fields=permissions.permissionArchivedAt` both
resolve.

**The negative half is the case nobody writes, and on this entity it is the point.** `resource`
and `action` are `hidden: true` — they exist to feed the derivation and for nothing else. They
appear in **NO** response body on **NO** surface: not in the insert 201, not in the GRANT 201,
not in the patch 200, not in the by-id document, not in a listing row, not in a GraphQL node,
not under any `?fields=` selection, and not under `?includeArchived=true`. Asserted as
**key-absent**, not as null. A rules-only join field that leaks is a silent regression every
other family here passes over.

**And the write side never speaks the derivation.** `RolePermissionRequest` carries
`permissionID` alone. Sending `{"permission": "tenant:read"}` as a grant supplies no
`permissionID` → **422** on the entry, never a silent match by string. That asymmetry is what
keeps the README's Permission rule 3 true: a retired-and-recreated permission comes back with a
new id, and a grant holding the string would silently re-attach.

### H4 — Validation 422, notification KEY and FIELD asserted (never prose)

| Input | Key | field |
|---|---|---|
| `key: ""` | `RequiredFieldNotification` | `key` |
| `key: "Billing"` (uppercase — refused, never repaired) | `InvalidRoleKeyNotification` | `key` |
| `key: "a"` (below the 2-rune floor) | `InvalidRoleKeyNotification` | `key` |
| `key: "bil--ling"` (doubled hyphen) | `InvalidRoleKeyNotification` | `key` |
| `key: "-billing"` · `"billing-"` (leading / trailing hyphen) | `InvalidRoleKeyNotification` | `key` |
| `key: "aaaab"` (run of 4 identical runes) | `InvalidRoleKeyNotification` | `key` |
| `key:` 65 runes | `InvalidRoleKeyNotification` | `key` |
| `name: ""` | `RequiredFieldNotification` | `name` |
| `name: "a"` (below the DisplayName floor) | `InvalidDisplayNameNotification` | `name` |
| `description: "short"` | `InvalidDescriptionNotification` | `description` |
| a grant entry with `permissionID: "tatu"` | `InvalidIDUUIDNotification` | the entry's id path |

The two empty cases assert the **framework's** required-field notification rather than each
VO's own, because both VOs short-circuit on empty (`role_key.go`, `display_name.go`) — a case
asserting `InvalidRoleKeyNotification` there would be asserting a branch the code cannot take.

Positive controls, each a rule's own boundary: `billing-manager` · `3d-assets-team` (leading
digit) · a 2-rune key · an accented, multi-word description.

**The guard barrier is a family of its own, and it is the reason a 500 once shipped.**
`TenantID` carries a `valueObject` rule with `guard: true`, so `domain.ID.IsValid` runs FIRST
and `r.StopIfInvalid()` ends the whole pass:

| case | request | expected |
|---|---|---|
| `H4.g1` | `tenantID: ""` | **422** `InvalidIDUUIDNotification` — `uuid.Parse` refuses the empty string and junk through the same call, so both inputs carry the SAME key |
| `H4.g2` | `tenantID: "tatu"` | **422** `InvalidIDUUIDNotification` |
| `H4.g3` | `tenantID: "tatu"` **together with** `description: "short"` | **422** reporting the **owner ALONE** — the barrier ends the pass including the automatic value-object validation and every collection, so the second violation must NOT appear. This is what proves the barrier is a barrier and not one more rule |
| `H4.g4` | **the field name the barrier reports** | field **`tenantID`**, camelCased like every other field in the same envelope |

**`H4.g4` is a REGRESSION GUARD for a framework defect the deleted round found and left RED.**
At v0.72.1 `domain.ID.IsValid` emitted through `AddNotificationMessage`'s `FieldName` — the
legacy slot, which nothing folds — so the same field came back as **`TenantID`** while
`key`, `name` and `description` came back camelCased, and a client branching on `field` had to
special-case one notification. Verified at **this** pin before the case was written:
`domain/id.go:70` now writes `Path: []PathSegment{{Name: fieldName}}`, and `renderPath`
(`domain/notification.go:112-125`) folds a Path through lowerCamel with acronym awareness. So
the expectation is `tenantID`, it is derived from the pin's own code and not from an answer,
and if it ever regresses this case is what says so.

### H5 — The 409 family, and what "unique within the tenant" actually means

| Case | Expected |
|---|---|
| the same `key` twice in one tenant | **409** `RoleKeyAlreadyExistsNotification`, semantic `"Conflict"`, field `key` |
| **the same `key` in a DIFFERENT tenant** | **201** — the constraint is `(tenant_id, role_key)`, not `role_key`. Without this control the row above would pass just as well for an over-broad global index, which is exactly the regression that makes two customers collide |
| a patch that does not move the key | **200** — `excludeSelf`, so a row never collides with itself |
| archive a role, then insert the same key again | **201** with a **NEW id**; the archived row still readable under `?includeArchived=true`. `unique.scope: active-only`, and with no unarchive verb this is the only route back |
| the same `permissionID` twice inside ONE insert body | **409** `RoleAlreadyGrantsPermissionNotification`, field `permissions` |
| GRANT a permission the role already holds | **409**, same key. This is where `IsSameBusinessIdentity` over `PermissionID` **alone** is load-bearing: the entry carries four fields and three are join fields blank on a freshly added entry, so a compare-all-fields identity would answer "different" and the duplicate guard would fail **open** |
| REVOKE a grant, then GRANT the same permission again | **201** with a **NEW child id** — the child index is `active-only` and there is no per-entry unarchive, so a fresh add is the only way back, and it reads correctly in the audit trail |

**The wrong-state 409 is `N/A`, by the same derivation as both earlier rounds.**
`EntityIsNotActiveNotification` / `ConcurrentModificationNotification` are not reachable through
this aggregate's HTTP surface: no PUT verb, no wire field carries a revision, and every
wrong-state attempt is intercepted a layer earlier by the LOAD scope, where it lands as **404**
(`H10`). Asserting a 409 there would encode a promise the pin does not make.

### H6 — Archive, which on this entity is a ONE-WAY DOOR

`archive` → **204, no body** · by-id → **404** `RecordNotFoundNotification` · by-id
`?includeArchived=true` → **200** with `archivedAt` stamped **and its grants still readable** —
a retired role must stay auditable, and with no unarchive verb this is the only way to see one ·
listing → row absent · listing `?includeArchived=true` → row present.

And then it stops: no unarchive mode, no route, no mutation. The way back is a fresh insert of
the same key, which answers **201 with a NEW id** — asserted as a different id, because "it came
back with the same id" would mean the one-way door has a hinge.

The collection verbs on an archived root, both loading through `LoadForWrite`, which runs the
default `ScopeActive`:

- GRANT onto an archived role → **404** · REVOKE on an archived role → **404** ·
  archive an already-archived role → **404** · PATCH an archived role → **404**.

**Child stamp-scoped unarchive — `N/A`, stated rather than skipped.** The family exists to prove
that a child removed on its own BEFORE the root's archive stays archived after the root comes
back. This root never comes back. There is no restore for a stamp to be scoped to.

### H7 — The three archive stamps, and what each one REPORTS

**Approved at this gate (maintainer, 2026-09-07): the full family, with both effects.** These
three fields entered the model on 2026-09-06 and their entire job is to report a state the
caller could not otherwise see. Asserting only that they arrive `null` would leave that job
unproven.

| # | The chain | Expected |
|---|---|---|
| `H7.1` | grant a live catalog permission, read the role back | `permissionArchivedAt` **absent/null** on the entry, `permission` rendered |
| `H7.2` | **archive that catalog permission**, read the role back again | the entry is **STILL SERVED** — an inner join is not gated on the archived state of its target — `permission` **still renders** the same token, and `permissionArchivedAt` is now **STAMPED**. This is the whole reason the field was re-added: `?includeArchived` governs which ROOTS a read returns, never the rows a traversal reaches across into, so an archived permission was already arriving here silently and indistinguishable from a live one |
| `H7.3` | the same over GraphQL | identical, field for field |
| `H7.4` | and the rules did **not** move | the role remains PATCHable by name — cross-referenced to `RL9`, because a rule reading `PermissionArchivedAt` instead of the probe would fail open here |
| `H7.5` | archive the **owning tenant**, read the role back with `?includeArchived=true` | `tenantArchivedAt` **STAMPED**, and `tenantWorkspace` / `tenantStatus` **still served** — the ROOT join keeps matching its archived counterpart too |
| `H7.6` | Role's own `archivedAt` | `null` while active, **stamped** after archive and visible only via `?includeArchived=true`. Terminal — there is no unarchive to return it to `null`, and that is asserted as such |

`H7.5` is run **last in its block and on a tenant the lane created**, never on `master`: see
§2's suite invariant.

### H8 — Read vocabulary (everything the DTO declares, and only that)

- **Filters, one per declared operator family:**
  `tenantID` `eq,in` · `key` `eq,ne,in,startswith,istartswith,contains,icontains` ·
  `name` `eq,in,startswith,istartswith,contains,icontains` ·
  `description` `contains,icontains` ·
  **`tenantWorkspace` `eq,in,startswith,istartswith,contains,icontains`** ·
  **`tenantStatus` `eq,in`** · `createdAt` `gte,lte` · `updatedAt` `gte,lte`.
  Wire form `?key.eq=…`; GraphQL `where: { key: { eq: "…" } }`.
  **The two `tenant*` rows are the round's headline capability**: a ROOT join's fields ARE
  addressable in a criteria, so *"the roles of `acme-comercio`"* is answered without a second
  call, over columns that live in another table.
  Two asymmetries earn their own cases because they are the kind a regression flattens: `ne` is
  declared on `key` and **not** on `name`; `description` filters and is orderable in **neither**
  direction.
- **`?orderBy=`**: `key`, `name`, `tenantID`, `tenantWorkspace`, `tenantStatus`, `createdAt`,
  `updatedAt` — **seven fields, both directions, fourteen cases**, two of them over another
  table's columns. GraphQL: `orderBy: [{field: KEY, direction: DESC}]` over the reflected enum,
  **introspected and not guessed** (the deleted round shipped `orderBy: "key"` and was wrong).
- **`?fields=`**: `key` alone · `tenantWorkspace` alone (the join value) ·
  `permissions.permission` (the collection carrying that entry field and nothing else) ·
  `permissions.permissionArchivedAt` · `archivedAt` · `tenantArchivedAt`.
- **`?onlyTotal=true`** → `{success, status, description, pagination:{totalCount}}` — no `data`,
  no cursors.
- **`?last=N` alone** → the TAIL window, `hasNextPage: false`.
- **Pagination envelope as a contract**, and as a **BICONDITIONAL**: `endCursor` is emitted
  exactly when `hasNextPage`, `startCursor` exactly when `hasPreviousPage`. With a known lane
  count and `?first=2`: assert the biconditional on every page, echo `endCursor` into `?after=`
  → page 2 disjoint from page 1 with `hasPreviousPage == true`, walk back with `last`+`before`
  → page 1 again. `totalCount` truthful against a known seeded+inserted set.
- **`?includeArchived=true`** on both reads; it raises `totalCount` by exactly the number of
  archived rows.
- **The seeded baseline is a contract, derived from the tracked migration and never from an
  answer:** on a freshly migrated database, read as the `*:*` admin,
  `GET /roles?onlyTotal=true` answers **1**, and the master role carries exactly **one** grant
  whose `permission` renders `*:*`.

### H9 — Rejected reads: the whole typed-400 guard family

All **400**, key `SchemaViolationNotification` unless stated. The `field` named is the **wire
token**, not the bare field — the lesson the permission round paid for.

| Request | field named |
|---|---|
| `?bogus=1` | `bogus` |
| `?key.gte=x` (operator outside its allowlist) | `key.gte` |
| `?name.ne=x` — **declared on `key`, NOT on `name`** | `name.ne` |
| `?description.eq=x` — `contains`/`icontains` only | `description.eq` |
| `?tenantStatus.contains=x` — the enum declares `eq`/`in` alone | `tenantStatus.contains` |
| `?search=x` (a RESERVED control the DTO never declared) | `search` |
| **`?archivedAt.gte=…` · `?tenantArchivedAt.gte=…`** | the wire token — **served but deliberately NOT in `byParams.filters`**: the read filter vocabulary has no `isnull`/`notnull`, so all a declaration would buy is "archived between these dates", while the question a caller actually asks is what `tenantStatus` answers. Absent **by choice**, and this is the case that pins the choice |
| **`?permissions.resource.eq=x` · `?permissions.permission.eq=x` · `?permissions.permissionID.eq=x`** | the CHILD join's field, the computed entry field, and the entry's own stored column: **none** is addressable in a criteria. Filtering a root by a field of a 1:N child is a pushdown one root SELECT cannot express |
| `?orderBy=description` — filterable, orderable in neither direction | `orderBy[description]` |
| `?orderBy=archivedAt` · `?orderBy=permissions.permission` | `orderBy[…]` — the 1:N boundary again, on the sort side |
| `?orderBy=id` (declarable, deliberately not declared) · `?orderBy=bogus` | `orderBy[…]` |
| **`?fields=permissions.resource` · `?fields=permissions.action`** | `fields[…]` — `hidden`, feeding the derivation, never SELECTABLE. The complement of `H3` |
| `?fields=bogus` · `?fields=deletedAt` (the pre-v0.74.0 token) | `fields[…]` |
| `?first=101` | `LimitExceededNotification`, `value` = **100** — no `query:` block in the yaml and no per-view override, so `bootstrap.FrameworkDefaultMaxLimit` (`config.go:896`) is the ceiling |
| `?first=0` · `abc` · `-5` | `first` |
| `?first=2&last=2` · `?first=2&before=X` · `?last=2&after=X` · `?after=X&before=Y` | the backward-side key |
| `?onlyTotal=true` beside `first` / `orderBy` / `fields` / `after` | `onlyTotal[<conflict>]` |
| `?onlyTotal=true` **+ a filter**, and **+ `includeArchived=true`** | **200** — counting a filtered subset is the canonical use |
| `?onlyTotal=false&first=10` | **200** — present-but-inactive never trips the conflict matrix |
| `?after=not-a-cursor` | `after` |
| a cursor minted under the default order, replayed with `&orderBy=key` | the structural check |
| the same cursor replayed with `&includeArchived=true` | the context-hash check |
| `?includeArchived=1` · `?onlyTotal=` (empty) | the control's own key — the booleans take exactly `true`/`false` |
| **by-id gate**: `?onlyTotal=false` · `?fields=key` on `GET /roles/{id}` | the control's own key — that endpoint declares `includeArchived` and nothing else, and PRESENCE is what trips the gate |
| by-id `?includeArchived=true` | **200** — the positive control for the one it does declare |
| `?createdAt.gte=not-a-date` | **400** `InvalidFilterValueNotification` (pin ≥ v0.70.0) |
| **`?tenantID.eq=lixo`** — an identity column that declares `eq` | **400** `InvalidFilterValueNotification`. The case an older suite has no equivalent of: below v0.70.0 the same request was a **500** on a relational backing and an empty `200` page on Mongo |

**`UnsupportedCapabilityNotification` is `N/A` on this entity, by construction** — the same
conclusion both earlier rounds reached, for the same reason. It needs a control the DTO DOES
declare and the backing cannot serve. `FindRolesRequest` declares no `Search` field, so
`?search=` is refused at the wire wrapper before any engine is consulted; and `permissions.*` is
declared nowhere, so the schema gate answers first there too. Asserting that key here would be
asserting a lie — and a suite asserting the wrong key would go RED against a correct service.

### H10 — Routing, absent verbs, and the by-id ADDRESS contract

| Request | Expected |
|---|---|
| **`PATCH /roles/{id}/unarchive`** | **404 `RouteNotFoundNotification`** — no route is registered at that path at all |
| `DELETE /roles/{id}` · `PUT /roles/{id}` · `POST /roles/{id}` | **405 `MethodNotAllowedNotification`** — the path is registered, the method is not. `DELETE` proves no hard delete exists |
| `GET /roles/{id}/archive` | **405** — same path, wrong method |
| `DELETE /roles/{id}/permissions/{childId}` | **404 or 405**, whichever the router answers — what it must never be is **204** |
| `GET /does-not-exist` | **404 `RouteNotFoundNotification`** |
| `GET /roles/{unused uuid}` | **404 `RecordNotFoundNotification`** |
| GRANT onto a role id that addresses nothing | **404** |
| `GET /roles/not-a-uuid` (a **read** address) | **404 `UnknownIDAddressNotification`** |
| `PATCH /roles/not-a-uuid` · `/archive` · `POST /roles/not-a-uuid/permissions` (a **write** intention) | **400 `MalformedIDNotification`** |
| `{ role(id: "not-a-uuid") }` | typed `UnknownIDAddressNotification` |
| `mutation { archiveRole(id: "not-a-uuid") }` | typed `MalformedIDNotification` |

**The CHILD address is NOT that contract, and it is derived separately.**
`PATCH /roles/{good uuid}/permissions/not-a-uuid/archive` → **404
`RecordNotFoundNotification`**. `ArchiveRolePermissionRequest.RolePermissionID` is a plain
`string` path field, not a `domain.ID`, so the framework's wire wrapper never inspects it; the
value reaches `Role.RemoveRolePermissionByID`, which answers the canonical not-found for any id
the collection does not carry. The distinction is worth a case precisely because the two ids sit
in one URL and answer differently.

**The 403 mode-not-allowed shape is `N/A`, and the reason is precise.** That shape needs a mode
absent from `Modes()` **while its route is still mounted**. Role's absent mode (`unarchive`) has
no route at all, so it lands on the 404 arm; its absent verb (`DELETE`) has a registered path,
so it lands on 405. No `…NotAllowedNotification` is reachable on this entity.

### H11 — Suite meta

`GET /openapi.json` enumerates exactly the **SEVEN** role routes above and no eighth, and each
carries a `**Required permission:**` suffix naming its declared literal — so a route mounted
open is visible rather than merely absent from a count.

### J — GraphQL, where the idiom differs BY DESIGN

- Every rejection family of `H4`/`H5`/`H9`/`H10` repeated over `POST /graphql`, asserting
  `errors[].extensions.notificationKey` at HTTP **200**.
- **Handler invariance**, per verb: `createRole` / `patchRole` / `archiveRole` /
  `addRolePermission` / `archiveRolePermission` each produce an effect visible over **REST**
  immediately, and the same notification key on refusal.
- **`J.revoke` is the GraphQL half of `H1.7`**: `archiveRolePermission` answers `{success}` and
  **the root is still active over REST**.
- `role(id:)` equals the REST by-id document field for field, **including `tenantWorkspace`,
  `tenantStatus`, `tenantArchivedAt`, `archivedAt` and every entry's `permission` and
  `permissionArchivedAt`**.
- **`where:` and `orderBy:` over `tenantWorkspace` return the same set and the same order as
  their REST twins** — the root join queryable on this surface too — and the reflected
  `RoleOrderField` enum is asserted to be exactly the seven-field REST sort vocabulary, so the
  capability cannot come to exist on one surface alone.
- **Selecting `resource` or `action` on a `permissions` entry is an unknown-field validation
  error** — the GraphQL half of `H3`. The schema must not carry a field the REST body hides.
- An **undeclared argument** (`search:`) is cut out of the schema entirely — asserted as a
  gqlparser validation error, never as the REST envelope. `fields` and `onlyTotal` have no
  argument here: selection is the projection, and selecting `totalCount` alone *is* the
  only-total mode.
- **No `unarchiveRole` field exists** → "Cannot query field" on the mutation type. The GraphQL
  twin of `H10`'s 404 arm.
- **`__typename` beside a normal selection must answer identically** — the v0.72.1 regression
  guard, kept as a standing case.

**gRPC — `N/A`**: no `transport:` block, no transport tag, no procedures mounted.
**Tabular exports — `N/A`**: `surfaces` declares no CSV/XLSX for Role.

---

## 1b. Domain expectations — what the BUSINESS requires

The only rows in this plan the framework never had an opinion about. Source named per row;
`asked` means the maintainer answered on **2026-09-07** and the answer is recorded verbatim.
They live in the existing `qa/domain.sh` as `RL1`–`RL11`, appended after the permission rows
(`P1`–`P10`).

**Every negative case below needs a caller who is NOT a super-admin**, which the suite
provisions through the service's own documented flow and nothing else (§2).

| # | The rule, as stated | Source | POSITIVE case | NEGATIVE case | Rank |
|---|---|---|---|---|---|
| **RL1** | "A wildcard permission cannot be granted on any role through the API, **and no caller is exempt**." | `spec.md` §7 R9b · `rules.manual.no-wildcard-grant` | the admin grants a concrete catalog permission → **201** | **the `*:*` super-admin** grants the seeded `*:*` row → **403** `CannotGrantWildcardPermissionNotification`, field `permissions`. The strongest possible negative: if anyone were exempt it would be them, and the seed migration's own header says this refusal is why a migration is the only way a super-admin exists | **critical** |
| **RL1b** | the wildcard rule runs BEFORE the escalation rule, and that order removes a panic input | `spec.md` §7 "the framework panic" · `identity.go:52` | — | the same request as `RL1` answers **403**, **never 500**. `HasPermission` panics on any argument containing `*`, so a service that reordered these two would crash the request into a 500 on exactly the case the pair exists to stop. Asserted as a plain status comparison, never through `jq` — a 500 body is not JSON | **critical** |
| **RL2** | "Só pode conceder o que você tem, a não ser que você seja um `*:*`." | `spec.md` §7 R9a, §B Q3 | the scoped principal grants `tenant:read`, which its own role carries → **201** | the same principal grants `permission:archive`, which it does not hold → **403** `CannotGrantUnheldPermissionNotification`, field `permissions`, value the permission id | **critical** |
| **RL2b** | the super-admin exemption is FREE, not special-cased — `HasPermission` answers true for any CONCRETE permission when the claim set holds `*:*` | `spec.md` §7 R9a | the admin grants `permission:archive`, which no role of theirs names individually → **201** | — | without this the rule could pass for a service that refuses **every** grant, the mirror failure of a gate that refuses everyone | high |
| **RL3** | "Tenant isolation binds WRITES: the row's tenant must equal the caller's claim, unless the caller is `*:*`." | `spec.md` §7 R5, §10 Layer 2, §B Q4 | the scoped principal creates a role **omitting `tenantID` entirely** → **201** in its own tenant (absent means *mine*) | the same principal names the **master** tenant → **403** `TenantMismatchNotification`, field `tenantID`. And archives another tenant's role → **403**, because `refuseForeignTenant` runs under `IfArchive` too and the write side is not filtered | **critical** |
| **RL3b** | "A by-id read of another tenant's role returns **404 rather than 403** — it does not exist for this caller, which leaks nothing about who else exists." | `spec.md` §10 Layer 3 | the principal reads its own role → **200** | the principal reads the master role by id → **404**, and its listing NEVER contains it. **404, not 403** — a 403 would confirm the row exists | **critical** |
| **RL3c** | the `*:*` holder crosses the row scope, which is what lets a platform operator support a customer | `find_roles_by_params_query.go:60` | the admin's listing contains roles from tenants that are not `master` | — | a service that filtered everyone would pass `RL3b` while making support impossible | high |
| **RL4** | "Every granted permission must exist in the catalog **and still be active**. A retired permission comes back as a NEW row with a NEW id, so re-granting the old id is refused rather than silently honoured." | `spec.md` §7 R6 | grant a live catalog id → **201** | grant a random UUID → **422** `PermissionNotInCatalogNotification`; **and grant the id of a permission the lane archived a moment earlier** → **422**, same key. The second half is the one that matters — it is the whole reason the grant stores the id and not the string | **critical** |
| **RL5** | "The owner tenant must exist, not be archived, and not be **suspended**. A `trial` tenant is a live customer and passes — 'unavailable' is not 'not active'." | `spec.md` §7 R4 (corrected 2026-08-24) | create a role inside a **`trial`** tenant → **201**. The *plausible-mistake control*: a rule written as `Status != active` would refuse every trial signup, and only this case sees it | create inside a **`suspended`** tenant → **422** `RoleTenantDoesNotExistNotification`; inside an **archived** tenant → **422**, same key; naming a tenant id no row carries → **422**, same key | **critical** |
| **RL6** | "The key is immutable after creation — it is what API callers and audit lines reference." | `rules.list.key-immutable` · §7 R1 | patch `name` + `description` → **200**, `key` byte-identical | patch `{"key": "<other>"}` → **422** `RoleKeyIsImmutableNotification`, field `key` | medium |
| **RL7** | "A role never moves between tenants" — enforced **STRUCTURALLY**, not by a refusal | `spec.md` §7 R2 | — | patch `{"tenantID": "<other>"}` → **200**, and the read-back `tenantID` is **UNCHANGED**. The door does not exist: `PatchRoleRequest` carries `key`, `name`, `description` alone, so `RoleTenantIsImmutableNotification` is **unreachable through REST and GraphQL** and **no case asserts it**. Recorded here rather than discovered later | medium |
| **RL8** | "At most **250** permissions in one role." **RAISED from 200 on 2026-09-08** (`specs/implement/role-permission-cap-250/plan.md`) | `rules.list.permission-cap` · §7 R8 · **asked — the cheap form, reconfirmed** | a role carrying the whole live catalog minus every wildcard row → **201**. The fixture excludes **every** row whose rendered token contains a `*`, not only the seeded `*:*`: `no-wildcard-grant` refuses a `*` in either half, and the permission lane creates a `<resource>:*` row of its own | an insert carrying **251** distinct invented UUIDs → **422** with `TooManyPermissionsInRoleNotification` **present in the envelope**. Stated plainly: 251 `PermissionNotInCatalogNotification` keys ride with it, because invented ids are in no catalog — the assertion reads the WHOLE envelope, never only the first message. The clean alternative (seeding 251 real catalog rows) was declined again at this gate as not worth ~252 requests. **Two assertions on the envelope, and they are different questions:** the echoed `value` is the COUNT SENT (`251`, because the rule exposes `len(items)`), while the **interpolated message** is the only place the CAP's own number reaches a caller (`250`) — which is why the second was added when the cap moved, the first having been unable to detect the change | medium |
| **RL9** | "R6, R9a and R9b judge the entries a write **ADDS**, never the ones already stored." | `spec.md` §7 "What these three rules judge" · `GetAddedItemsOf` | archive a catalog permission an existing role ALREADY grants, then `PATCH` that role's **name** → **200**. A rule re-judging stored entries would answer 422 on a request whose only change is a label | — the refusals are `RL2`/`RL4`, which fire on ADDED entries in the same run | **critical** |
| **RL10** | "A REVOKE asks nothing, because it adds nothing." | same source | REVOKE a grant from a role whose OTHER grant points at an archived permission → **204** | — | revocation, the tool for a grant that stopped being acceptable, stops being reachable exactly when it is needed | high |
| **RL11** | **"Arquivar um papel retira o poder: a linha `user_roles` sobrevive apontando para o papel arquivado, e isso é HISTÓRIA — mas um token reemitido não carrega mais as permissões dele."** | **asked 2026-09-07** — *"Some, e a suíte prova com fixture própria"* | before the archive, the fixture principal reaches the route its role's permission gates → **2xx**, and its token carries the pair | after archiving the ROLE, a **freshly reissued** token for the same principal no longer carries the pair and the same call answers **403** — while the `user_roles` row **still exists** (asserted by SQL, since no endpoint exposes it) | **asked** |

**`RL11`'s fixture is the round's one irreversible chain, and it is self-contained.** Insert a
catalog pair of the suite's own → create a tenant → create a role in it granting only that pair
→ create a user holding only that role → rotate its password → sign in → prove the reach →
**archive the ROLE** → prove the `user_roles` row survives → sign in AGAIN → prove the reach is
gone. **The seeded `master` role, the `*:*` catalog row and the bootstrap admin are never
touched**, which is why this chain needs no special position in the lane.

**Nothing in §1b is UNPROVEN this round** — every question this plan raised was answered on
2026-09-07 before it was written.

**Two notifications are declared and UNREACHABLE through the wire, both stated rather than
asserted:** `RoleTenantIsImmutableNotification` (`RL7`) and `TenantMismatchNotification` **on
the read path** (Layer 3 answers 404, never 403 — which is `RL3b`'s whole point).

---

## 2. Data hygiene — inherited, plus two new fixture classes

**Unchanged from both approved rounds, and no new decision is asked here:** the throwaway
database `authcore_qa`, selected through `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` with
`APP_PROFILE=qa`, dropped and recreated before every run, `migrations.autoRun` rebuilding schema
and seed, port `:8099`, the same signing key. `microservice.dev.yaml` is never touched and
`authcore_db` is never written to. The reset is that one step and nothing races it: no Mongo, no
CDC, no projection to clear, no relay to drain.

**The baseline is not empty and it is not the catalog's 39.** For `roles` it is **1** (the seeded
`master` role) and for `role_permissions` **1** (its wildcard grant). Every count this lane
asserts is scoped to its own rows with `?key.startswith=<lane prefix>`; `H8`'s baseline case is
the one that speaks about the whole table, and it says **1**, derived from migration 0012.

**New fixture class 1 — the SCOPED PRINCIPALS.** §1b and §3 need callers who are authenticated,
are NOT super-admins, are bound to a tenant of the suite's own making, and hold a CHOSEN bundle.
Two of them, both provisioned through the service's own documented flow — no invented
credential, no forged token:

| | tenant | holds | exists because |
|---|---|---|---|
| **E** | its own, created by the suite | `role:insert`, `role:update`, `role:archive`, `role:read`, `role:grant`, `tenant:read` — and deliberately **not** `permission:archive` | every §1b negative needs a real non-superadmin caller, and `RL2`'s negative needs one specific permission it does **not** hold |
| **F** | the **same** tenant as E | `role:read`, `role:update` — and **not** `role:grant` | it is the only caller for which the 2026-08-28 verb split is visible: it may relabel a role and must get **403** on both collection routes |

Each is four calls: `POST /tenants` (E only) → `POST /roles` → `POST /users` → sign in, `PATCH
/users/{id}/password`, sign in again. **The rotation is not optional**: every API-created user is
born `must_change_password = TRUE`, and its first token carries `user:change-password` alone. A
suite that skipped it would test the restricted session and read its 403s as the role gate.

**New fixture class 2 — a suite-created TENANT per lifecycle state.** `RL5` needs a `trial`, a
`suspended` and an archived tenant; `H7.5` needs one it can archive without touching anything
else. All are created by the admin and are the lane's own.

**A fixture trap worth naming before it bites.** `vos.RoleKey` refuses a run of 4 identical runes
and anything outside `^[a-z0-9]+(-[a-z0-9]+)*$`, so a run tag like `11115` would 422 every insert
and take the lane RED for a fixture reason rather than a service one. `qa/lib/common.sh` already
carries `qa_slug_runid`, which collapses runs of 3+ to two; the role fixtures **reuse it** rather
than writing a second sanitizer.

**A suite invariant, stated so it cannot be violated by accident: no lane may archive the seeded
`master` role, the `master` tenant, or the `*:*` catalog row.** Archiving any of them ends the
run's ability to authenticate as an operator. Every archive case in this round operates on a
row the suite itself created, and `RL11`'s revocation chain uses its own principal for exactly
that reason.

**Residue, as promised:** one throwaway database, now also holding the lane's tenants, roles and
users. Nothing is ever written to `authcore_db`.

---

## 3. Security — never `N/A`, and for the first time never `N/A` on layers 2 and 3

**Where a valid token comes from: the service itself** (`auth.issuer.enabled: true`), through
`POST /auth/user/token`. Nothing is invented, and no token is forged except the ones that exist
to be refused — all of which the tenant round already owns.

Everything Role-specific lands in the existing `qa/security.sh` under a new **`S5.x`** block
(tenant owns `S3.x`, permission owns `S4.x`).

### 3a — The 401 half: INHERITED. See `specs/qa/tenant-contract/plan.md` §3a.

The middleware does not know which route it is guarding, so `S3a.1`–`S3a.10` are not repeated.
One row is added, because a route that was never gated at all would still pass every one of
them: **`S5.0` `GET /roles` with no `Authorization` header → 401 `MissingAuthorizationNotification`.**

### 3b — The public-route half: direction 2 for the seven routes this round adds

`auth.publicRoutes` names no `/roles` path. Direction 1 is inherited. What this round adds:

- each of the **seven** REST routes, **tokenless → 401**;
- each of the **seven** GraphQL fields, tokenless → **401** in the REST envelope (the bearer is
  checked before the document is parsed).

### 3c — The 403 half. All three layers, for the first time in this suite.

**Layer 1 — the gate, per verb AND per surface.** A route gated on REST is not thereby gated on
GraphQL, and a route can lose its gate alone.

| # | token | request | expected |
|---|---|---|---|
| `S5.1a-g` | principal **B** (`tenant:read` only) | each of the seven REST routes | **403** `MissingPermissionNotification`, field `permission`, value **`role:read` / `role:insert` / `role:update` / `role:archive` / `role:grant`** as the route declares |
| `S5.2a-g` | **the admin** | the same seven | **2xx** — and for a write aimed at an id that addresses nothing, a **404**, never a 403: reaching the HANDLER is what proves the gate opened. A gate that refuses everyone is also broken |
| `S5.3` | **principal F** (`role:read` + `role:update`, **no** `role:grant`) | `GET /roles`, `GET /roles/{id}`, `PATCH /roles/{id}` | **200** |
| `S5.3c-d` | the same principal F | **both collection routes** | **403**, value **`role:grant`** — the decision of 2026-08-28 made visible: "may relabel the role" and "may change what the role can do" are separately grantable, and the second is the privilege-escalation surface. **No other principal can see this**, which is why F exists |
| `S5.4a-g` | principal B | the seven **GraphQL** fields | the typed 403 in this surface's idiom, `errors[].extensions.notificationKey` |
| `S5.5` | the admin | the same seven fields | ok — the complement, per surface |

**Layer 2 — identity-derived rules in `BuildRules`.** Proven by pairs of calls that differ only
in **who is asking**:

| # | request | expected |
|---|---|---|
| `S5.6` | principal **E** creates a role naming the **master** tenant | **403** `TenantMismatchNotification`, field `tenantID` |
| `S5.7` | **the same body** sent by the admin (`*:*`) | **201** — the bypass, which is what lets a platform operator support a customer |
| `S5.8` | principal E archives a role belonging to another tenant | **403** `TenantMismatchNotification` — `refuseForeignTenant` runs under `IfArchive`, and the write side is **not** filtered by `ToCriteria`, so the row loads and the RULE is what refuses. This is the seam that would be invisible if only reads were tested |
| `S5.9` | principal E grants a permission it does not hold | **403** `CannotGrantUnheldPermissionNotification` — the second Layer-2 rule, cross-referenced to `RL2` |

**Layer 3 — tenant row scoping. An isolation leak answers 200**, which is exactly why it needs
its own cases rather than riding on a refusal:

| # | request | expected |
|---|---|---|
| `S5.10` | principal E lists roles | **200**, and the master role is **absent** from every page — asserted by **id**, not by count |
| `S5.11` | principal E reads the master role by id | **404**, **not 403** |
| `S5.12` | the admin lists roles | **200**, and the master role **is** present — without this, a service that filtered everyone would pass `S5.10` for the wrong reason |
| `S5.13` | the same scope on **GraphQL** | principal E's connection carries only its own tenant's roles — `ToCriteria` is shared, but a surface that skipped it would look exactly like a passing REST case |

**`Restrict` — `N/A`, stated rather than skipped, and printed in the SKIP column.** `spec.md` §9
declares no field-level read authz: *"Every field a caller may see the row at all for, they may
see entirely. Row-level isolation does the work here."* There is no column to find absent for one
caller and present for another, no tabular export whose header could be pruned, and no
`__typename` edge to assert — that edge exists only where a restricted field is in the selection.
Naming it here is the honest form; asserting one would be inventing a rule.

### 3d — `auth.mode` posture

`mode: jwt`, `authorization.enabled: true`, `authorization.tenant.required: true`. Neither
degenerate posture applies, and neither is available as an excuse.

### 3e — Coverage reported, not asserted in prose

The `Restrict` `N/A` and the inherited set (§0b) are printed in the report's SKIP column with
their reasons, so nothing reads as covered that was not run.

---

## 4. Out of scope, named plainly

- **Load, performance and concurrency** — ⚠️ **named, not implied.** The uniqueness backstops
  (`roles` and `role_permissions` partial indexes) exist precisely for the race the domain
  pre-check cannot see, and proving a race needs concurrent writers this suite does not have.
  Say the word and it becomes a round of its own.
- **The per-entry probe cost.** `spec.md` §7 accepts up to 200 catalog round trips inside one
  write transaction, mitigated by a single walk and a memoised lookup. That is a performance
  property, not a contract; `RL8` bounds it rather than measuring it.
- **UI** — `/docs` and the GraphQL playground are asserted reachable by the inherited lane, never
  rendered.
- **Integration events — `N/A`, not deferred:** no `transport:` block, nothing published, no
  `integration_events` table. There is no delivery half to mark ⚠️ OPEN.
- **gRPC** and **tabular exports** — neither is wired or declared.
- **`User → Role` and `Group → Role` as contracts.** This round provisions users holding roles in
  order to get scoped tokens, and `RL11` asserts one `user_roles` row **by SQL** as a
  consequence. Neither edge is asserted as a contract; both belong to their own rounds.
- **The remaining four aggregates** — `claim`, `client`, `group`, `user`.

### In scope, extending the existing lane: the audit trail

**Approved at this gate (maintainer, 2026-09-07): root verbs AND the collection verbs.**
`qa/audit.sh` already proves the framework's in-TX `audit_events` promise for Tenant (`A1`–`A14`)
and Permission (`A15`–`A23`). Role brings a shape neither has, so this round extends that lane
rather than starting a new one (cases **`A24+`**):

- one row per `insert` / `update` / `archive` on the root, `entity_type = 'Role'`,
  `aggregate_id` = the row's id, `kind` `snapshot` on the insert and `transition` on the archive;
- **the two collection verbs, which is the new part**: a GRANT and a REVOKE each write **one
  `update` row against the ROOT's `aggregate_id`**, not the child's — a child op is a command on
  the root — each carrying a `changes` block naming the collection;
- **no `unarchive` row exists to assert**, and its absence from the timeline is itself asserted;
- `actor` = the acting principal's `sub`; `tenant_id` = the actor's tenant claim;
- the declared `auditClaims` (`email`, `tenant_workspace`, `identity_kind`) ride the payload, and
  an undeclared claim never does;
- a **refused** write leaves **no** row — the event is written in the write's own transaction and
  rolls back with it.

---

## 5. Runner contract — the SAME `qa/run.sh`, extended

**Still exactly one entry point.** This round adds two lanes and edits four existing files; the
lane list is where the addition becomes real.

```
qa/
├── run.sh                     ← EDITED: LANES gains role + role_graphql; the principal section gains E and F
├── lib/common.sh              ← EDITED: role_body / new_role / grant_permission / permission_id_of / provision_principal
├── tenant.sh                  ← untouched
├── tenant_graphql.sh          ← untouched
├── permission.sh              ← untouched
├── permission_graphql.sh      ← untouched
├── role.sh                    ← NEW: H1–H11, REST
├── role_graphql.sh            ← NEW: the J family
├── domain.sh                  ← EDITED: §1b appended as RL1–RL11 (tenant's R rows and permission's P rows stay)
├── security.sh                ← EDITED: §3 appended as S5.x
├── audit.sh                   ← EDITED: the Role family appended as A24+
├── microservice.qa.yaml       ← untouched
└── microservice.qa-key.yaml   ← untouched
```

- **Lane list becomes**:
  `tenant tenant_graphql permission permission_graphql role role_graphql domain security audit`
  — the two new lanes sit beside their twins, and the three shared lanes stay last so a Role
  domain rule and a Tenant one are read together.
  `./qa/run.sh role role_graphql` runs just this round's new surface work, on the same runner.
- **`PLANS` names all three plans** — the report cannot claim to execute two documents when it
  executes three.
- **`permission_id_of <resource> <action>`** resolves a seeded catalog id by its pair rather than
  hardcoding a UUID literal in a second place; `run.sh`'s existing two literals stay as they are.
- **Everything else is inherited verbatim**: root resolution (`cd "$(dirname "$0")/.."` first in
  every lane), fail-fast by default with `--all` for the sweep, non-zero exit per lane, per-run
  namespacing of every temp file / log / binary / port, the boot sequence, SIGTERM-only shutdown
  with a waited drain, `Accept-Language: en-US` on every request, SKIPPED printed as SKIPPED, and
  a failed assertion printing the real response body.
- **`CLAUDE.md` rule 1 applies to the four EDITED files** as much as to the two new ones: they
  are named here, before anything is touched, and this gate is their approval.

---

## 6. Report contract — unchanged

`qa/qa-report.md`, rendered live and rewritten in full after every lane, same header / matrix /
failures / footer / abort-trap / SKIP-column rules as `tenant-contract` §6. The only change is
that the matrix grows from seven rows to nine. A lane that never ran still prints `—`.

`qa/qa-report.md` and `qa/.logs/` remain run artifacts, **offered** as `.gitignore` lines at
hand-off and never added by this skill. Neither `qa/` nor `specs/qa/` is ever gitignored.

---

## 7. Gate — closed 2026-09-07

| Item | Answer |
|---|---|
| The plan as a whole, including the four EDITED files of §5 (`CLAUDE.md` rule 1) | **Approved** 2026-09-07 |
| §1 `H7` — the archive-stamp family, full form with both effects | ✅ asked 2026-09-07 |
| §1b `RL11` — archiving a role removes the power; own fixture | ✅ asked 2026-09-07 |
| §4 — the audit lane extended with root **and** collection verbs | ✅ asked 2026-09-07 |
| §1b `RL8` — the cap in its cheap form | ✅ asked 2026-09-07 (reconfirmed) |
| §2 — two scoped principals (E and F) added to `qa/run.sh` | **Approved** in the same breath |
| §1b ranking | `RL1`, `RL1b`, `RL2`, `RL3`, `RL3b`, `RL4`, `RL5`, `RL9` **critical** |

---

## 8. Run record — 2026-09-07

`./qa/run.sh --all` · **✅ ALL GREEN — 9/9 suites · 855 cases · 20s** (the footer of
`qa/qa-report.md`, quoted rather than paraphrased).

| lane | pass | fail | skip |
|---|---:|---:|---:|
| `tenant` | 111 | 0 | 0 |
| `tenant_graphql` | 36 | 0 | 0 |
| `permission` | 131 | 0 | 0 |
| `permission_graphql` | 37 | 0 | 0 |
| **`role`** | **207** | **0** | **4** |
| **`role_graphql`** | **44** | **0** | **0** |
| `domain` | 143 | 0 | 3 |
| `security` | 108 | 0 | 2 |
| `audit` | 38 | 0 | 0 |

**Reconcile.** `ls qa/*.sh` minus `run.sh` equals the runner's `LANES` array exactly, nine for
nine. Every family of §1 exists in the generated suite and RAN: `H1` 13 · `H2` 12 · `H3` 8 ·
`H4` 17 · `H5` 10+1 skip · `H6` 9+1 · `H7` 12 · `H8` 62 · `H9` 46+1 · `H10` 15+1 · `H11` 4;
`J1` 16 · `J2` 7 · `J3` 4 · `J4` 10 · `J5` 5 · `J6` 2. Every §1b row carries BOTH halves except
the four whose complement is another row and which the matrix already says so about — `RL2b`,
`RL3c`, `RL9` and `RL10` — and `RL7`, whose positive half is `—` by construction because the
door does not exist. `S5.1`–`S5.7` all executed (45 cases), and `A24`–`A38` all executed (15).

**The suite can fail** — the mandatory meta-case, run and then undone. `H1.1` was flipped by
hand to assert **200** where the service answers **201**. The lane went RED, the runner exited
**non-zero**, `qa/qa-report.md` showed `role` RED with the other eight lanes printing `—`
rather than vanishing, the failures section named the case with expected-vs-received and the
real 201 body, and the footer read `❌ RED — 1 of 1 suites`. The expectation was then restored
and the full sweep re-run green.

### Eleven suite defects were corrected before this record — none by weakening a case

Every failure of the first execution was the suite's, not the service's. Recorded because each
is a shape that makes a suite quietly untrue:

1. **`[.data | length, .pagination.hasNextPage]` does not mean what it reads as.** The pipe
   binds the whole comma list, so the second term is evaluated against `.data` and the filter
   yields nothing — four pagination cases were comparing an empty string against an expectation
   and would have gone red against a correct service. Fixed with `[(.data | length), …]`.
2. **`paths` is a jq builtin**, not the OpenAPI object's member. The three `H11` inventory cases
   were streaming every path in the document instead of reading `.paths`.
3. **A raw space is not a URL.** `?name.eq=QA Fixture Role` made curl refuse the request
   outright (HTTP 000) — a fixture failure wearing a contract failure's clothes.
4. **`H8.38` tripped the conflict matrix its own lane asserts.** The baseline count carried the
   walk's `orderBy`, and `?onlyTotal=true` beside a page-shaping control is exactly what `H9.30`
   refuses. The count now runs on the filter alone.
5. **`S5.5h` did the GraphQL version of the same thing**: selecting `totalCount` ALONE is this
   surface's only-total mode, so `first: 1` beside it is the same conflict. `edges` is now
   selected alongside, which is the rule `J5.5` already stated one file over.

Two expectations were written to match the wire's `omitempty` reality rather than the field
inventory, and both are recorded rather than smoothed: a live row's `archivedAt`,
`tenantArchivedAt` and `permissionArchivedAt` are `null`, and the LISTING DTO elides a null
pointer — so `H2.9` asserts the three keys an entry actually carries while live, and `H7` is
where the fourth appears, stamped. The by-id Response does not elide, which is why `H2.1` can
assert all twelve keys.

**Residue, as §2 promised.** One throwaway database, `authcore_qa`, dropped and recreated at the
start of every run; at rest it holds 39 roles, 108 role_permissions, 42 tenants, 8 users, 76
permissions and 210 audit_events. `authcore_db` holds **0** rows written by this suite —
verified: `roles.role_key LIKE 'qa-%'` → 0, and the same for `tenants.workspace`,
`users.email` and `permissions.resource_name`.

**No finding about the service came out of this round.** The one framework defect the deleted
2026-09-03 round left RED — `InvalidIDUUIDNotification` naming the field `TenantID` while every
other message in the same envelope was folded to camelCase — is **fixed at this pin**, and
`H4.g4` now stands as its regression guard.
