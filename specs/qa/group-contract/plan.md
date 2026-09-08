# QA plan — `group-contract`

- **Status:** APPROVED — maintainer (Cláudio Schirmer Guedes), 2026-09-07. *"bora, aprovado — toca ficha"* — the plan as a whole, including the five EDITED files of §5 and the two prose files of §0c.
- **Suite slug:** `group-contract` — what this round proves: the whole wire contract of the
  `Group` aggregate and its `group_roles` collection on both surfaces, the business rules its
  own specs and the maintainer state, and the three things that make this entity unlike every
  other one already proven — **its power is TRANSITIVE** (a member inherits every permission of
  every role the group confers, and `authentication_reader.go` carries three kill switches for
  it that no other aggregate has); **its child join is SERVED** where `Role`'s was hidden, and
  is *still* not addressable in a criteria; and **`G6` asks a third question `Role` never
  had — same tenant** — which is a cross-tenant leak wearing the clothes of ordinary group
  editing.
- **Written:** 2026-09-07
- **Pin:** omnicore **`v0.74.0`** · dialect postgres · read backing **relational**
  (read-your-writes — every read-back below is IMMEDIATE; a needed poll would itself be a
  failure)
- **Surfaces in scope:** REST + GraphQL (`surfaces.graphql.enabled: true` — and unlike what
  `spec.md` §9 still says, the two COLLECTION verbs are mounted on GraphQL too; see §0c)
- **Plan destination:** `specs/qa/group-contract/plan.md` · **suite destination:** `qa/` at the
  project root · **verdict destination:** `qa/qa-report.md`
- **Prior rounds:** `specs/qa/tenant-contract/plan.md` (APPROVED 2026-09-06),
  `specs/qa/permission-contract/plan.md` (APPROVED 2026-09-07) and
  `specs/qa/role-contract/plan.md` (APPROVED 2026-09-07). This round **EXTENDS the same
  `qa/run.sh`** — no second entry point — and none of the three approved plans is reopened.

## 0. Where every expectation below comes from

Nothing here was read off a running service. The service was never called while this plan was
written. The one exception is an ENUMERATION and never a value that becomes an expectation: the
suite reads `GET /openapi.json` at boot to cross-check the verb inventory (`K11`).

| Source | What it settled |
|---|---|
| `specs/scaffold-entity/group/spec.md` (APPROVED 2026-08-21, reviewed 2026-08-24, superseded 2026-09-06) | the model; §A the ten industry findings; §B Q1/Q2 (`key`+`name`+`description`; the transitive rule **and** `group:grant`); §2 the fields and both joins; §3 the collection as a PAIR of ops; §5 the one-way archive and what it costs; §7 G0–G10b; §9 the read vocabulary and the 1:N boundary; §10 the five permissions and the three authorization layers |
| `specs/omnicore-gen/group.omnicore.yaml` | fields, the collection, both joins, notifications, modes, `patchExcludes`, rules (declarative + manual) and their ORDER, `service.facts`, `read.byParams`, `authz` |
| `internal/domain/group.go:113-238` · `group_rules_manual.go` · `vos/group_key.go` | the rules as implemented — the guard barrier on `TenantID`, `refuseForeignTenant` under `IfInsertOrUpdate` **and** `IfArchive`, the cap and duplicate checks over `GetCurrentItemsOf`, and the single ADDED-entries walk behind G6/G10b/G10a in that order |
| `internal/infra/group_service_manual.go` | the four probes, the one-read-per-entry memo, and that a failed probe PANICS rather than inventing an answer |
| `internal/web/group_routes.go` | the **7** REST endpoints and the **7** GraphQL fields, and the permission literal on each |
| `internal/web/requests/find_groups_by_params.go` · `find_group_by_id.go` · `insert_group.go` · `patch_group.go` · `add_group_role.go` · `archive_group_role.go` · `dtos/group_role.go` | the DTO opt-in gate: which controls each read serves, which operators each leaf admits, and **which fields each Response carries** — including that the write bodies carry `{id, roleID}` alone while the read bodies carry five fields per entry |
| `internal/application/queries/find_groups_by_params_query.go` · `find_group_by_id_query.go` | `ToCriteria` forces `Filter["TenantID"] = id.TenantID()` unless `IsSuperAdmin()` — Layer 3 |
| **`internal/infra/authentication_reader.go:108-131, 346-383`** | **the transitive walk `user_groups → groups → group_roles → roles → role_permissions → permissions`, the `groups` claim that falls out of it, and the three kill switches `GroupArchivedAt` (`:354`), `GroupGrantArchivedAt` (`:360`) and `RoleArchivedAt` (`:361`)** — the source that makes §1b `GR12` a provable family rather than a hope |
| `migrations/postgres/0004_group_manual.up.sql` | the two partial unique indexes (`(tenant_id, group_key)` and `(group_id, role_id)`, both `WHERE archived_at IS NULL`) and the two hand-appended cross-aggregate FKs |
| `migrations/postgres/0012_bootstrap_seed_manual.up.sql` | the token source, the five `group:*` catalog rows (`…000d`–`…0011`), and the baseline this lane counts against: **0** groups, **0** group_roles |
| `qa/microservice.qa.yaml` · `qa/run.sh` · `qa/lib/common.sh` | the posture this suite runs under and the helper API every new lane speaks |
| pin docs — `status-mapping`, `auto-handlers`, `auto-query-handlers`, `read-joins`, `relational-view`, `auth-middleware`, `authz-seams`, `graphql`, `rules-dsl`, `audit` | every status code, notification key and envelope shape asserted below |
| **the maintainer, asked 2026-09-07** (`AskUserQuestion`) | §1b `GR11` (the cap in its HYBRID form), §1b `GR12` (the full three-switch revocation chain), §2's principals **G** and **H**, and the disposition of §0c's stale prose |

### 0a. What this round INHERITS and does not repeat

Proven service-wide by the three approved rounds, against this exact posture:

- the whole **401 family** (`S3a.1`–`S3a.10`) — token shape, signature, `iss`, `aud`, `alg`,
  `exp`, and the expired-vs-invalid key split;
- the **framework's appended public surfaces** (`/docs`, `/openapi.json`, JWKS, the playground,
  the root redirect) and the **GraphQL introspection bypass with its four edges**;
- **direction 1** of the public-route split — every declared `publicRoutes` entry answers
  tokenless — and the exactness neighbours;
- the middleware **tenant-claim gate** (`authorization.tenant.required: true`);
- the **boot, hygiene and report contracts** (§2, §5, §6 of `tenant-contract`), reused verbatim.

What this round adds to the security lane is only what is Group-specific: §3 below.

### 0b. The one framework promise this entity leans on that no earlier round tested

`Role` proved a child join whose fields are **hidden** — `resource` and `action` fed a
derivation and reached no body. `Group`'s child join is the **opposite declaration**: `RoleKey`,
`RoleName` and `RoleArchivedAt` are **served**, on every read, on both surfaces. The pin's
promise that makes the two coexist is one sentence in `read-joins`: *a join on a collection
fills its fields on every loaded entry and is **not addressable in a criteria***. Served and
unfilterable at the same time is the shape `K3` proves and `K9` proves the complement of, and it
is the shape `spec.md` §9 names as *"the one real cost"*: **"which groups confer role X?" is not
answerable from this listing.**

### 0c. Stale prose found while deriving this plan — a FINDING, and its correction

Four statements in the project's own decision record contradict the code they describe. None
changes a single expectation below — the code is right in all four — but each is a sentence a
future round would derive a wrong case from, so they are recorded here and, **by the
maintainer's decision of 2026-09-07, corrected at the source with a dated supersession** (the
`CLAUDE.md` rule-1 approval for touching two files outside this skill's normal write scope):

| # | Where | What it says | What the code does |
|---|---|---|---|
| 1 | `group.omnicore.yaml`, the block comment above the child join | *"The role's `archived_at` is deliberately NOT traversed"* | the very next lines declare `RoleArchivedAt`, and its own comment explains why it IS there. `GroupRoleRow.roleArchivedAt` is on the wire — `K7.2` is the case that pins it |
| 2 | `spec.md` §2, review item 5 | *"The grants no longer carry the conferred role's archive stamp, matching `Role`"* | they do, since 2026-09-06 — the same pass that added `ArchivedAt` to `read.managed` and `archived_at` to both joins |
| 3 | `spec.md` §9 | `?fields=` reaches *"`roles.roleKey`, `roles.roleName` and `roles.archivedAt`"* | the third token is `roles.roleArchivedAt` — `GroupRoleRow` declares `RoleArchivedAt`, not `ArchivedAt`. `K8` uses the real token |
| 4 | `group.omnicore.yaml`, the `no-wildcard-role-attach` comment and `spec.md` §7 | the platform's superadmin group *"is seeded by migration beside the reserved platform tenant"* | migration 0012 seeds **no group and no `user_groups` row**. The admin holds `*:*` through `user_roles → master role`. The baseline `K8` asserts is **0 groups**, and `GR4`'s negative is the admin attaching the **master ROLE**, which is where the wildcard actually lives |

A fifth is not a defect but a supersession already recorded in the yaml, repeated here so no
case is derived from the older half: `spec.md` §9 says GraphQL carries **root verbs only**, and
the yaml's `surfaces.graphql` comment records that the two collection verbs joined *"when the
language grew a seat for them"*. `group_routes.go` registers **seven** GraphQL fields. **Seven
is what `K11` and the `L` family assert.**

---

## 1. Coverage matrix — the FRAMEWORK's promises

Entity **Group** × surfaces **REST** and **GraphQL**. Read backing **relational**, so every
write→read-back is immediate. Archive regime *kept-but-hidden* (no `DeleteOnArchive`), and
**one-way** — no unarchive mode, no unarchive route, no unarchive mutation.

Envelope asserted on REST: `errors[].messages[].notificationKey` + the HTTP status, and the
`field` where the pin promises one. On GraphQL: HTTP is **always 200** and the same key rides
`errors[].extensions.notificationKey`. Every request pins `Accept-Language: en-US`.

**The lane runs as the bootstrap admin (`*:*`)**, so nothing in §1 is blocked by row scope;
§1b and §3 are where the scoped principals do the work.

**Case prefixes.** `K` = `qa/group.sh` (REST), `L` = `qa/group_graphql.sh`. Both are free:
tenant owns `F`, permission owns `E`/`G`, role owns `H`/`J`.

### The shape that governs this whole matrix

`group_roles` stores **one column** — `role_id` — and a read returns **five values** per entry:
that id, the entry's own id, and the three the traversal fills. Alongside it the ROOT join
publishes `tenantWorkspace`, `tenantStatus` and `tenantArchivedAt`. One repository, both kinds
of join, and this time both are SERVED — what separates them is where they can be *asked
about*:

| | the ROOT join → `Tenant` | the CHILD join → `Role` |
|---|---|---|
| fields | `TenantWorkspace`, `TenantStatus`, `TenantArchivedAt` | `RoleKey`, `RoleName`, `RoleArchivedAt` |
| served on the wire | **all three** | **all three** — the difference from `Role`'s hidden pair |
| present in a WRITE body | never | **never** — `GroupRoleResponse` is `{id, roleID}` |
| addressable in a criteria | **yes** — `tenantWorkspace`/`tenantStatus` filterable and sortable | **no** — load-only, the 1:N boundary |
| selectable with `?fields=` | yes | **yes**, as `roles.<field>` |
| the case that proves it | `K8` filters and orders by them | `K3` reads them, `K8` selects them, `K9` proves filtering or ordering by one is a typed 400 |

A case that forgets which side of that table it is on proves nothing, so each family below says
which side it is on. **The child-join row is where a suite copied from `role.sh` goes wrong in
both directions**: `roles.roleKey` IS selectable (Role's `permissions.resource` was not), and it
is still NOT filterable (same as Role).

### K1 — Happy path, one per served verb

`Modes()` declares exactly `display, insert, update, archive` — **four modes, SEVEN routes**,
because the collection carries a fifth verb of its own. The inventory is cross-checked against
`GET /openapi.json` at boot (`K11`); a source-vs-openapi disagreement is a FINDING, not
something the suite reconciles.

| REST | GraphQL twin | Expected |
|---|---|---|
| `POST /groups` | `createGroup(input:)` | **201** · `{id, tenantID, key, name, description, roles[{id, roleID}]}` |
| `GET /groups/{id}` | `group(id:)` | **200** · the full document |
| `GET /groups` | `groups(...)` | **200** · `data` + `pagination` / the Relay connection |
| `PATCH /groups/{id}` | `patchGroup(id:, input:)` | **200** · the root as stored |
| `PATCH /groups/{id}/archive` | `archiveGroup(id:)` | **204, NO BODY** / payload `{success, id}` |
| **`POST /groups/{id}/roles`** | `addGroupRole(id:, input:)` | **201** · `{groupId, groupRole:{id, roleID}}` |
| **`PATCH /groups/{id}/roles/{groupRoleId}/archive`** | `archiveGroupRole(id:, input:{groupRoleId})` | **204, NO BODY** / payload `{success}` |

**`K1.7` is the model's canonical trap and it is asserted as such**, exactly as `H1.7` was one
level down: the root-archive auto handler is instantiated once per surface, and wiring it to the
child DETACH route type-checks, boots, answers 204 and archives the entire group — which here
would silently de-authorize a whole team. So the DETACH case does not stop at the 204: it
re-reads the root, asserts it is **still active**, asserts the remaining entries are **still
there**, and asserts the detached one is gone. Nothing else in this matrix would notice.

**The join fields are absent from every WRITE body and present in every READ body.** The
traversal runs on the load; a command result carries the stored column alone. Both halves are
asserted — a `roleKey` appearing in a 201 would mean a read-side value reached the write side.

### K2 — Golden-record round-trip

One group exercising **every declared field**, written then read back field-by-field on all four
read shapes (REST by-id, REST listing row, GraphQL node, GraphQL connection edge):

`id` · `tenantID` · `key` · `name` · `description` · `createdAt` · `updatedAt` · **`archivedAt`**
· `tenantWorkspace` · `tenantStatus` · **`tenantArchivedAt`** · `roles[]` with
`{id, roleID, roleKey, roleName, roleArchivedAt}`.

- `tenantWorkspace` and `tenantStatus` carry the owning tenant's **actual** handle and status —
  the ROOT join reaching the wire over a column that lives in another table.
- `roleKey` and `roleName` are byte-identical to the catalog row `roleID` addresses, for
  **every** entry — the child join doing the same across a second table.
- `createdAt`/`updatedAt` present and RFC3339-parseable **by grammar, not by `jq`'s
  `fromdateiso8601`** — that builtin accepts only a `Z`-suffixed instant with no fractional
  part, while this service stamps from Postgres. Inherited lesson; it is not paid twice.
- The three stamps are **`null` while their target is live**, and the LISTING DTO elides a null
  pointer (`omitempty`) while the by-id Response does not — so the listing assertion names the
  keys an entry actually carries while live, and the by-id assertion names all eleven. Their
  stamped behaviour is `K7`.

### K3 — "One column in, five out": the child join, both halves

**The positive half.** Every served entry carries `id`, `roleID`, `roleKey`, `roleName` and
`roleArchivedAt`; `?fields=roles.roleKey`, `?fields=roles.roleName` and
`?fields=roles.roleArchivedAt` all resolve.

**The negative half — and here it is the WRITE side, not a hidden field.** `roleKey`, `roleName`
and `roleArchivedAt` appear in **NO** write body on **NO** surface: not in the insert 201, not
in the ATTACH 201, not in the patch 200, not in the GraphQL mutation payloads. Asserted as
**key-absent**, not as null. `GroupRoleResponse` carries `{id, roleID}` and that is the whole
contract of a write.

**And the write side never speaks the render.** `GroupRoleRequest` carries `roleID` alone.
Sending `{"roleKey": "billing-manager"}` as an entry supplies no `roleID` → **422** on the
entry, never a silent match by string. That asymmetry is what keeps the model's own reasoning
true: a retired-and-recreated role comes back with a NEW id, and an entry holding the key would
silently re-confer it.

### K4 — Validation 422, notification KEY and FIELD asserted (never prose)

| Input | Key | field |
|---|---|---|
| `key: ""` | `RequiredFieldNotification` | `key` |
| `key: "Engineering"` (uppercase — refused, never repaired) | `InvalidGroupKeyNotification` | `key` |
| `key: "e"` (below the 2-rune floor) | `InvalidGroupKeyNotification` | `key` |
| `key: "eng--team"` (doubled hyphen) | `InvalidGroupKeyNotification` | `key` |
| `key: "-eng"` · `"eng-"` (leading / trailing hyphen) | `InvalidGroupKeyNotification` | `key` |
| `key: "aaaab"` (run of 4 identical runes — `groupKeyMaxIdenticalRun = 4`) | `InvalidGroupKeyNotification` | `key` |
| `key: "aa"` (2 runes, 1 distinct — below `groupKeyMinDistinct = 2`) | `InvalidGroupKeyNotification` | `key` |
| `key:` 65 runes | `InvalidGroupKeyNotification` | `key` |
| `name: ""` | `RequiredFieldNotification` | `name` |
| `name: "e"` (below the `DisplayName` floor of 2) | `InvalidDisplayNameNotification` | `name` |
| `description: "short"` (below the `Description` floor of 15) | `InvalidDescriptionNotification` | `description` |
| an entry with `roleID: "tatu"` | `InvalidIDUUIDNotification` | the entry's id path |

The two empty cases assert the **framework's** required-field notification rather than each VO's
own, because both VOs short-circuit on empty (`group_key.go:64`, `display_name.go`) — a case
asserting `InvalidGroupKeyNotification` there would be asserting a branch the code cannot take.

Positive controls, each a rule's own boundary: `engineering` · `3m-approvers` (leading digit) ·
`hr` (the 2-rune, 2-distinct floor the VO exists to accept) · an accented, multi-word
description.

**The guard barrier is a family of its own.** `TenantID` carries a `valueObject` rule with
`guard: true`, so `domain.ID.IsValid` runs FIRST and `r.StopIfInvalid()` ends the whole pass —
including the automatic value-object pass and every collection:

| case | request | expected |
|---|---|---|
| `K4.g1` | `tenantID: ""` | **422** `InvalidIDUUIDNotification` — `uuid.Parse` refuses the empty string and junk through the same call, so both inputs carry the SAME key |
| `K4.g2` | `tenantID: "tatu"` | **422** `InvalidIDUUIDNotification` |
| `K4.g3` | `tenantID: "tatu"` **together with** `description: "short"` **and** an entry with `roleID: "tatu"` | **422** reporting the **owner ALONE** — the second and third violations must NOT appear. This is what proves the barrier is a barrier and not one more rule, and it is the 500 the `Role` entity actually shipped once |
| `K4.g4` | the field name the barrier reports | field **`tenantID`**, camelCased like every other field in the same envelope — the standing regression guard for the v0.72.1 defect fixed at this pin (`domain/id.go:70` writes a `Path`, `renderPath` folds it) |

**A child id that is not a UUID is NOT covered by that barrier, and that is deliberate.**
`refuseUnattachableRoles` filters unusable ids out of its walk and raises nothing, because the
framework's own child validation reports them — so `K4`'s last row asserts
`InvalidIDUUIDNotification` on the entry and **not** a probe-driven 500.

### K5 — The 409 family, and what "unique within the tenant" actually means

| Case | Expected |
|---|---|
| the same `key` twice in one tenant | **409** `GroupKeyAlreadyExistsNotification`, semantic `"Conflict"`, field `key` |
| **the same `key` in a DIFFERENT tenant** | **201** — the constraint is `(tenant_id, group_key)`, not `group_key`. Without this control the row above would pass just as well for an over-broad global index, which is exactly the regression that makes two customers collide |
| a patch that changes only `name`/`description` | **200** — `excludeSelf`, so a row never collides with itself (and `Key` is not in the patch body at all — `GR8`) |
| archive a group, then insert the same key again | **201** with a **NEW id**; the archived row still readable under `?includeArchived=true`. `unique.scope: active-only`, and with no unarchive verb this is the only route back |
| the same `roleID` twice inside ONE insert body | **409** `GroupAlreadyGrantsRoleNotification`, field `roles` |
| ATTACH a role the group already confers | **409**, same key. This is where `IsSameBusinessIdentity` over `RoleID` **alone** is load-bearing: the entry carries five fields and three are join fields blank on a freshly added entry, so a compare-all-fields identity would answer "different" and the duplicate guard would fail **open** |
| DETACH an entry, then ATTACH the same role again | **201** with a **NEW child id** — the child index is `active-only` and there is no per-entry unarchive, so a fresh add is the only way back |

**The wrong-state 409 is `N/A`, by the same derivation as all three earlier rounds.**
`EntityIsNotActiveNotification` / `ConcurrentModificationNotification` are not reachable through
this aggregate's HTTP surface: no PUT verb, no wire field carries a revision, and every
wrong-state attempt is intercepted a layer earlier by the LOAD scope, where it lands as **404**
(`K10`).

### K6 — Archive, which on this entity is a ONE-WAY DOOR that de-authorizes a team

`archive` → **204, no body** · by-id → **404** `RecordNotFoundNotification` · by-id
`?includeArchived=true` → **200** with `archivedAt` stamped **and its entries still readable** ·
listing → row absent · listing `?includeArchived=true` → row present.

And then it stops: no unarchive mode, no route, no mutation. The way back is a fresh insert of
the same key, which answers **201 with a NEW id** — asserted as a *different* id, because "it
came back with the same id" would mean the one-way door has a hinge. **What that new id costs is
`GR13`**, and it is the reason §5 of the spec refused unarchive in the first place.

The collection verbs on an archived root, both loading through `LoadForWrite`, which runs the
default `ScopeActive`:

- ATTACH onto an archived group → **404** · DETACH on an archived group → **404** ·
  archive an already-archived group → **404** · PATCH an archived group → **404**.

**Child stamp-scoped unarchive — `N/A`, stated rather than skipped.** The family exists to prove
that a child removed on its own BEFORE the root's archive stays archived after the root comes
back. This root never comes back.

### K7 — The three archive stamps, and what each one REPORTS

| # | The chain | Expected |
|---|---|---|
| `K7.1` | attach a live role, read the group back | `roleArchivedAt` **null**, `roleKey`/`roleName` rendered |
| `K7.2` | **archive that role**, read the group back again | the entry is **STILL SERVED** — an inner join is not gated on the archived state of its target — `roleKey`/`roleName` **still render**, and `roleArchivedAt` is now **STAMPED**. This is the case that pins §0c finding 1: the field exists precisely because an archived role was already arriving here indistinguishable from a live one |
| `K7.3` | the same over GraphQL | identical, field for field |
| `K7.4` | and the rules did **not** move | the group remains PATCHable by name — cross-referenced to `GR9`, because a rule reading `RoleArchivedAt` instead of the probe would fail open here |
| `K7.5` | archive the **owning tenant**, read the group back with `?includeArchived=true` | `tenantArchivedAt` **STAMPED**, and `tenantWorkspace` / `tenantStatus` **still served** |
| `K7.6` | Group's own `archivedAt` | `null` while active, **stamped** after archive and visible only via `?includeArchived=true`. Terminal — there is no unarchive to return it to `null` |

`K7.5` runs **last in its block and on a tenant the lane created**, never on `master`: see §2's
suite invariant. `K7.2`'s archived role is likewise the lane's own.

### K8 — Read vocabulary (everything the DTO declares, and only that)

- **Filters, one per declared operator family:**
  `tenantID` `eq,in` · `key` `eq,ne,in,startswith,istartswith,contains,icontains` ·
  **`name` `eq,ne,in,startswith,istartswith,contains,icontains`** ·
  `description` `contains,icontains` ·
  `tenantWorkspace` `eq,in,startswith,istartswith,contains,icontains` ·
  `tenantStatus` `eq,in` · `createdAt` `gte,lte` · `updatedAt` `gte,lte`.
  Wire form `?key.eq=…`; GraphQL `where: { key: { eq: "…" } }`.
  **`name.ne` is DECLARED here and was NOT on `Role`** — a suite copied from `role.sh` would
  assert a 400 where this entity answers 200, so it gets an explicit positive case.
  `description` filters and is orderable in **neither** direction — the other asymmetry, and the
  one that survives from `Role`.
- **`?orderBy=`**: `key`, `name`, `tenantID`, `tenantWorkspace`, `tenantStatus`, `createdAt`,
  `updatedAt` — **seven fields, both directions, fourteen cases**, two of them over another
  table's columns. GraphQL: `orderBy: [{field: KEY, direction: DESC}]` over the reflected enum,
  **introspected and not guessed**.
- **`?fields=`**: `key` alone · `tenantWorkspace` (the root-join value) · **`roles.roleKey`** ·
  **`roles.roleName`** · **`roles.roleArchivedAt`** (the child-join values — the half `Role`
  could not have) · `roles.roleID` · `archivedAt` · `tenantArchivedAt`.
- **`?onlyTotal=true`** → `{success, status, description, pagination:{totalCount}}` — no `data`,
  no cursors.
- **`?last=N` alone** → the TAIL window, `hasNextPage: false`.
- **Pagination envelope as a contract, and as a BICONDITIONAL**: `endCursor` is emitted exactly
  when `hasNextPage`, `startCursor` exactly when `hasPreviousPage`. With a known lane count and
  `?first=2`: assert the biconditional on every page, echo `endCursor` into `?after=` → page 2
  disjoint from page 1 with `hasPreviousPage == true`, walk back with `last`+`before` → page 1
  again. `totalCount` truthful against a known inserted set.
- **`?includeArchived=true`** on both reads; it raises `totalCount` by exactly the number of
  archived rows.
- **The seeded baseline is a contract, derived from the tracked migration and never from an
  answer:** on a freshly migrated database, read as the `*:*` admin,
  `GET /groups?onlyTotal=true` answers **0**, and `group_roles` holds **0** rows. Migration 0012
  seeds no group — which is also §0c finding 4.

### K9 — Rejected reads: the whole typed-400 guard family

All **400**, key `SchemaViolationNotification` unless stated. The `field` named is the **wire
token**, not the bare field — the lesson the permission round paid for.

| Request | field named |
|---|---|
| `?bogus=1` | `bogus` |
| `?key.gte=x` (operator outside its allowlist) | `key.gte` |
| `?description.eq=x` — `contains`/`icontains` only | `description.eq` |
| `?tenantStatus.contains=x` — the enum declares `eq`/`in` alone | `tenantStatus.contains` |
| `?search=x` (a RESERVED control the DTO never declared) | `search` |
| `?archivedAt.gte=…` · `?tenantArchivedAt.gte=…` | the wire token — **served but deliberately NOT in `byParams.filters`**: the read vocabulary has no `isnull`/`notnull`, so all a declaration would buy is "archived between these dates", while the question a caller asks is what `tenantStatus` answers. Absent **by choice**, and this pins the choice |
| **`?roles.roleKey.eq=x` · `?roles.roleName.eq=x` · `?roles.roleID.eq=x` · `?roles.roleArchivedAt.gte=…`** | **the round's headline negative.** All four are SERVED values, and **none** is addressable in a criteria: filtering a root by a field of a 1:N child is a pushdown one root SELECT cannot express. This is `spec.md` §9's *"one real cost"* — *"which groups confer role X?"* stays unanswerable until the service gains Mongo |
| `?orderBy=description` — filterable, orderable in neither direction | `orderBy[description]` |
| `?orderBy=archivedAt` · `?orderBy=roles.roleKey` | `orderBy[…]` — the 1:N boundary again, on the sort side |
| `?orderBy=id` (declarable, deliberately not declared) · `?orderBy=bogus` | `orderBy[…]` |
| `?fields=bogus` · `?fields=deletedAt` (the pre-v0.74.0 token) · `?fields=roles.archivedAt` (§0c finding 3's wrong token) | `fields[…]` |
| `?first=101` | `LimitExceededNotification`, `value` = **100** — no `query:` block in `qa/microservice.qa.yaml` and no per-view override, so `bootstrap.FrameworkDefaultMaxLimit` is the ceiling |
| `?first=0` · `abc` · `-5` | `first` |
| `?first=2&last=2` · `?first=2&before=X` · `?last=2&after=X` · `?after=X&before=Y` | the backward-side key |
| `?onlyTotal=true` beside `first` / `orderBy` / `fields` / `after` | `onlyTotal[<conflict>]` |
| `?onlyTotal=true` **+ a filter**, and **+ `includeArchived=true`** | **200** — counting a filtered subset is the canonical use |
| `?onlyTotal=false&first=10` | **200** — present-but-inactive never trips the conflict matrix |
| `?after=not-a-cursor` | `after` |
| a cursor minted under the default order, replayed with `&orderBy=key` | the structural check |
| the same cursor replayed with `&includeArchived=true` | the context-hash check |
| `?includeArchived=1` · `?onlyTotal=` (empty) | the control's own key — the booleans take exactly `true`/`false` |
| **by-id gate**: `?onlyTotal=false` · `?fields=key` on `GET /groups/{id}` | the control's own key — that endpoint declares `includeArchived` and nothing else, and PRESENCE is what trips the gate |
| by-id `?includeArchived=true` | **200** — the positive control for the one it does declare |
| `?createdAt.gte=not-a-date` | **400** `InvalidFilterValueNotification` (pin ≥ v0.70.0) |
| `?tenantID.eq=lixo` — an identity column that declares `eq` | **400** `InvalidFilterValueNotification` |

**`UnsupportedCapabilityNotification` is `N/A` on this entity, by construction** — the same
conclusion all three earlier rounds reached. It needs a control the DTO DOES declare and the
backing cannot serve. `FindGroupsRequest` declares no `Search` field, so `?search=` is refused
at the wire wrapper before any engine is consulted; and `roles.*` is declared nowhere, so the
schema gate answers first there too. Asserting that key here would be asserting a lie.

### K10 — Routing, absent verbs, and the by-id ADDRESS contract

| Request | Expected |
|---|---|
| **`PATCH /groups/{id}/unarchive`** | **404 `RouteNotFoundNotification`** — no route is registered at that path at all |
| `DELETE /groups/{id}` · `PUT /groups/{id}` · `POST /groups/{id}` | **405 `MethodNotAllowedNotification`** — the path is registered, the method is not. `DELETE` proves no hard delete exists |
| `GET /groups/{id}/archive` | **405** — same path, wrong method |
| `DELETE /groups/{id}/roles/{childId}` | **404 or 405**, whichever the router answers — what it must never be is **204** |
| `GET /does-not-exist` | **404 `RouteNotFoundNotification`** |
| `GET /groups/{unused uuid}` | **404 `RecordNotFoundNotification`** |
| ATTACH onto a group id that addresses nothing | **404** |
| `GET /groups/not-a-uuid` (a **read** address) | **404 `UnknownIDAddressNotification`** |
| `PATCH /groups/not-a-uuid` · `/archive` · `POST /groups/not-a-uuid/roles` (a **write** intention) | **400 `MalformedIDNotification`** |
| `{ group(id: "not-a-uuid") }` | typed `UnknownIDAddressNotification` |
| `mutation { archiveGroup(id: "not-a-uuid") }` | typed `MalformedIDNotification` |

**The CHILD address is NOT that contract, and it is derived separately.**
`PATCH /groups/{good uuid}/roles/not-a-uuid/archive` → **404 `RecordNotFoundNotification`**.
`ArchiveGroupRoleRequest.GroupRoleID` is a plain `string` path field, not a `domain.ID`, so the
framework's wire wrapper never inspects it; the value reaches `Group.RemoveGroupRoleByID`, which
answers the canonical not-found for any id the collection does not carry. The distinction earns
a case precisely because the two ids sit in one URL and answer differently.

**The 403 mode-not-allowed shape is `N/A`, and the reason is precise.** That shape needs a mode
absent from `Modes()` **while its route is still mounted**. Group's absent mode (`unarchive`)
has no route at all, so it lands on the 404 arm; its absent verb (`DELETE`) has a registered
path, so it lands on 405. No `…NotAllowedNotification` is reachable on this entity.

### K11 — Suite meta

`GET /openapi.json` enumerates exactly the **SEVEN** group routes above and no eighth, and each
carries a `**Required permission:**` suffix naming its declared literal — `group:insert`,
`group:update`, `group:archive`, `group:read` ×2, **`group:grant` ×2** — so a route mounted open
is visible rather than merely absent from a count.

### L — GraphQL, where the idiom differs BY DESIGN

- Every rejection family of `K4`/`K5`/`K9`/`K10` repeated over `POST /graphql`, asserting
  `errors[].extensions.notificationKey` at HTTP **200**.
- **Handler invariance**, per verb: `createGroup` / `patchGroup` / `archiveGroup` /
  `addGroupRole` / `archiveGroupRole` each produce an effect visible over **REST** immediately,
  and the same notification key on refusal.
- **`L.detach` is the GraphQL half of `K1.7`**: `archiveGroupRole` answers `{success}` and **the
  root is still active over REST** with its other entries intact.
- `group(id:)` equals the REST by-id document field for field, **including `tenantWorkspace`,
  `tenantStatus`, `tenantArchivedAt`, `archivedAt` and every entry's `roleKey`, `roleName` and
  `roleArchivedAt`**.
- **Selecting `roleKey` / `roleName` / `roleArchivedAt` on a `roles` entry is VALID here** — the
  exact inverse of `Role`'s `J` family, where selecting `resource` was an unknown-field error.
  The schema must carry what the REST body carries, in both directions.
- **`where:` and `orderBy:` over `tenantWorkspace` return the same set and the same order as
  their REST twins**, and the reflected `GroupOrderField` enum is asserted to be exactly the
  seven-field REST sort vocabulary, so the capability cannot come to exist on one surface alone.
- An **undeclared argument** (`search:`) is cut out of the schema entirely — asserted as a
  gqlparser validation error, never as the REST envelope. `fields` and `onlyTotal` have no
  argument here: selection is the projection, and selecting `totalCount` alone *is* the
  only-total mode (so nothing else may be selected beside it).
- **No `unarchiveGroup` field exists** → "Cannot query field" on the mutation type.
- **`__typename` beside a normal selection must answer identically** — the v0.72.1 regression
  guard, kept as a standing case.

**gRPC — `N/A`**: no `transport:` block, no transport tag, no procedures mounted.
**Tabular exports — `N/A`**: `surfaces` declares no CSV/XLSX for Group.

---

## 1b. Domain expectations — what the BUSINESS requires

The only rows in this plan the framework never had an opinion about. Source named per row;
`asked` means the maintainer answered on **2026-09-07** and the answer is recorded verbatim.
They live in the existing `qa/domain.sh` as `GR1`–`GR13`, appended after the role rows
(`RL1`–`RL11`).

**Every negative case below needs a caller who is NOT a super-admin**, which the suite
provisions through the service's own documented flow and nothing else (§2).

| # | The rule, as stated | Source | POSITIVE case | NEGATIVE case | Rank |
|---|---|---|---|---|---|
| **GR1** | "Every attached role must **exist**." | `spec.md` §7 G6 (a) · `rules.manual.attached-roles-are-available-in-this-tenant` | attach a live role of the group's own tenant → **201** | attach a random UUID → **422** `RoleNotAvailableInTenantNotification`, field `roles`, value the role id | **critical** |
| **GR2** | "…and still be **ACTIVE**. A retired role comes back as a NEW row with a NEW id, so re-attaching the old id is refused rather than silently honoured." | same · §2 "storing the id and not the key" | attach a live role → **201** | attach the id of a role the lane archived a moment earlier → **422**, **same key**. This is the whole reason the entry stores the id | **critical** |
| **GR3** | "…and belong to **THIS GROUP'S TENANT**. A group in tenant A attaching tenant B's role would confer another customer's permissions on A's members." | `spec.md` §7 G6 (c) — *"the first thing in this entity that `Role` did not need"* | attach a role of the group's own tenant → **201** | a group in tenant A attaches tenant B's **live, active** role → **422** `RoleNotAvailableInTenantNotification` — **asserted as the SAME key as `GR1` and `GR2`, on purpose**: a distinct "belongs to another tenant" reply is an existence oracle over a competitor's org chart, and a service that "helpfully" split the message would be a regression this case is the only one to see | **critical** |
| **GR4** | "A role granting a permission with a wildcard in either part cannot be attached to any group, **and no caller is exempt**." | `spec.md` §7 G10b · `rules.manual.no-wildcard-role-attach` | the admin attaches a concrete role → **201** | **the `*:*` super-admin** attaches the seeded `master` role (which grants `*:*`) → **403** `CannotGrantWildcardRoleNotification`, field `roles`. The strongest possible negative: if anyone were exempt it would be them | **critical** |
| **GR4b** | the wildcard rule runs BEFORE the escalation rule, and that order removes a panic input | `spec.md` §7 G10a/G10b · `identity.go` | — | the same request as `GR4` answers **403**, **never 500**. `HasPermission` panics on any argument containing `*`. Asserted as a plain status comparison, never through `jq` — a 500 body is not JSON | **critical** |
| **GR5** | "**TRANSITIVE** no-escalation: a caller may attach a role only if they hold **EVERY** permission that role grants — a set, not one key." | `spec.md` §7 G10a · `rules.manual.no-privilege-escalation` | principal **G** attaches a role granting only permissions G holds → **201** | principal **G** attaches a role granting `permission:archive`, which G does not hold → **403** `CannotGrantRoleWithUnheldPermissionsNotification`, field `roles`, value the role id. **The set is the point**: the target role grants three permissions, two of which G holds — a rule checking "any" instead of "every" would let it through | **critical** |
| **GR5b** | the super-admin exemption is FREE, not special-cased | `spec.md` §7 G10a | the admin attaches that same role → **201** | — | without this the rule could pass for a service that refuses **every** attach, the mirror failure of a gate that refuses everyone | high |
| **GR5c** | the interlock ORDER, both links | `group.omnicore.yaml` `rules.manual` block comment · `group_rules_manual.go` | — | an **unknown** role id answers **422** `RoleNotAvailableInTenantNotification` and **never 403** — `RoleGrantsWildcard` deliberately answers TRUE for an id it cannot resolve, so a service that ran the wildcard rule first would blame the caller for an escalation attempt where the honest answer is "the role is not there". One case, and it is the only one that sees the order | **critical** |
| **GR6** | "Tenant isolation binds WRITES: the row's tenant must equal the caller's claim, unless the caller is `*:*`." | `spec.md` §7 G5, §10 Layer 2 · `group.go:233` | principal **G** creates a group **omitting `tenantID` entirely** → **201** in its own tenant (absent means *mine*, per `assignedFrom: identity-claim`) | the same principal names the **master** tenant → **403** `TenantMismatchNotification`, field `tenantID`. And archives another tenant's group → **403**, because `refuseForeignTenant` runs under `IfArchive` too and the write side is not filtered | **critical** |
| **GR6b** | "A by-id read of another tenant's group returns **404 rather than 403** — it does not exist for this caller." | `spec.md` §10 Layer 3 | G reads its own group → **200** | G reads another tenant's group by id → **404**, and its listing NEVER contains it, asserted **by id**. **404, not 403** — a 403 would confirm the row exists, and a group listing *is* the customer's org chart | **critical** |
| **GR6c** | the `*:*` holder crosses the row scope | `find_groups_by_params_query.go` | the admin's listing contains groups from tenants that are not its own | — | a service that filtered everyone would pass `GR6b` while making support impossible | high |
| **GR7** | "The owner tenant must exist, not be archived, and not be **suspended**. A `trial` tenant is a live customer and passes." | `spec.md` §7 G4 (corrected 2026-08-24) · `rules.manual.tenant-must-be-available` | create a group inside a **`trial`** tenant → **201**. The *plausible-mistake control*: a rule written as `Status != active` would refuse every trial signup, and only this case sees it | create inside a **`suspended`** tenant → **422** `GroupTenantDoesNotExistNotification`; inside an **archived** tenant → **422**, same key; naming a tenant id no row carries → **422**, same key. Scope is `IfInsert` only, so a PATCH on a group whose tenant was suspended afterwards still answers **200** — asserted, because it is the difference between a gate and a trap | **critical** |
| **GR8** | "The key is immutable after creation" — enforced **STRUCTURALLY**, not by a refusal | `rules.list.key-immutable` · `update.patchExcludes: [Key]` (decided 2026-09-06) | patch `name` + `description` → **200**, `key` byte-identical | patch `{"key": "<other>"}` → **200**, and the read-back `key` is **UNCHANGED**. The door does not exist: `PatchGroupRequest` carries `name` and `description` alone, on REST and GraphQL both, so **`GroupKeyIsImmutableNotification` is unreachable through the wire and NO case asserts it**. Recorded here rather than discovered later | medium |
| **GR8b** | "A group never moves between tenants" — same structural shape | `rules.list.tenant-immutable` · the patch DTO | — | patch `{"tenantID": "<other>"}` → **200**, read-back `tenantID` **UNCHANGED**; `GroupTenantIsImmutableNotification` likewise unreachable and unasserted | medium |
| **GR9** | "G6, G10a and G10b judge the entries a write **ADDS**, never the ones already stored." | `spec.md` §7 "What these three rules judge" · `GetAddedItemsOf` | archive a role a group ALREADY confers, then `PATCH` that group's **name** → **200**. A rule re-judging stored entries would answer 422 on a request whose only change is a label — and would make a group impossible to rename because of somebody else's archive | **critical** |
| **GR10** | "A DETACH asks nothing, because it adds nothing." | same source | DETACH an entry from a group whose OTHER entry points at an archived role → **204** | — | detaching, the tool for an attachment that stopped being acceptable, stops being reachable exactly when it is needed | high |
| **GR11** | "At most **50** roles in one group." | `rules.list.role-cap` · §7 G9 · **asked — the HYBRID form** | a group carrying **exactly 50** real, live, same-tenant roles → **201**. The edge, not an approximation of it | the **51st** role attached through `POST /groups/{id}/roles` → **422** `TooManyRolesInGroupNotification`, field `roles`, value `51`, and **CLEAN** — no other notification key in the envelope, which is what a fixture of invented ids could never show. The rule counts `GetCurrentItemsOf`, so the collection route trips it exactly as an oversized insert body does — both are asserted | medium |
| **GR12** | **"Arquivar o grupo tira o poder — e o poder do Group é transitivo. A linha `user_groups` sobrevive apontando para o grupo arquivado, e isso é HISTÓRIA; mas um token reemitido não carrega mais nenhuma permissão que vinha por ali."** | **asked 2026-09-07** — *"Os três interruptores"* · `authentication_reader.go:354-361` | the member reaches the route its inherited permission gates → **2xx**; its token carries the pair **and** a `groups` claim naming the group | the three switches, each on its own fixture — see below | **critical** |
| **GR13** | "A retired group comes back as a NEW row whose members must be re-added — which is why §5 refused `Unarchive`: one call would re-authorize a whole team with an audit line reading 'restored'." | `spec.md` §5 | — | after `GR12a`, insert the same key again → **201 with a NEW id**; reissue the former member's token → the inherited permission is **still gone**, because `user_groups` still points at the OLD id. The cost the model accepted, made visible | high |

### `GR12` — the three switches, and why each needs its own fixture

Each switch is irreversible, so a single chain cannot prove more than one of them. Three
self-contained fixtures, each built by the admin: a catalog pair of the suite's own → a role in
a lane tenant granting only that pair → a group conferring that role → a user who is a member of
that group **and holds no direct role at all** → rotate the password → sign in. That last
condition is what makes the assertion transitive rather than incidental: everything the token
carries came through the group.

| # | the switch | after reissuing the token | what must SURVIVE |
|---|---|---|---|
| `GR12a` | **archive the GROUP** (`GroupArchivedAt`, `:354`) | the pair is gone, the `groups` claim no longer names the group, and the gated call answers **403** | the `user_groups` row still exists — asserted by **SQL**, since no endpoint exposes it |
| `GR12b` | **detach the group_role** (`GroupGrantArchivedAt`, `:360`) | the pair is gone and the gated call answers **403** — while the `groups` claim **STILL names the group**: the person is still a member, the bundle just no longer confers that role. That distinction is the case's whole value | the `group_roles` row still exists, stamped |
| `GR12c` | **archive the ROLE the group confers** (`RoleArchivedAt`, `:361`) | the pair is gone and the gated call answers **403**; the `groups` claim still names the group | the `group_roles` row is **untouched and still active**, and the group still lists the entry with `roleArchivedAt` stamped — cross-referenced to `K7.2`, which is the read-side view of the same fact |

**The seeded `master` tenant, the `master` role, the `*:*` catalog row and the bootstrap admin
are never touched by any of the three**, which is why these chains need no special position in
the lane.

**Nothing in §1b is UNPROVEN this round** — every question this plan raised was answered on
2026-09-07 before it was written.

**Two notifications are declared and UNREACHABLE through the wire, both stated rather than
asserted:** `GroupKeyIsImmutableNotification` (`GR8`) and `GroupTenantIsImmutableNotification`
(`GR8b`). A third, `TenantMismatchNotification`, is unreachable **on the read path** — Layer 3
answers 404, never 403, which is `GR6b`'s whole point.

---

## 2. Data hygiene — inherited, plus two principals and one fixture class

**Unchanged from all three approved rounds, and no new decision is asked here:** the throwaway
database `authcore_qa`, selected through `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` with
`APP_PROFILE=qa`, dropped and recreated before every run, `migrations.autoRun` rebuilding schema
and seed, port `:8099`, the same signing key. `microservice.dev.yaml` is never touched and
`authcore_db` is never written to. The reset is that one step and nothing races it: no Mongo, no
CDC, no projection to clear, no relay to drain.

**The baseline is EMPTY, and that is itself the contract.** Migration 0012 seeds no group and no
`user_groups` row: `groups` → **0**, `group_roles` → **0**. `K8`'s baseline case says so, and
§0c finding 4 is the stale sentence that claims otherwise. Every other count this lane asserts
is scoped to its own rows with `?key.startswith=<lane prefix>`.

**New principals — G and H**, both in the **same** tenant `run.sh` already creates for E and F
(`QA_TENANT_SCOPED`), both provisioned through the service's own documented flow — no invented
credential, no forged token:

| | tenant | holds | exists because |
|---|---|---|---|
| **G** | `QA_TENANT_SCOPED` | `group:insert`, `group:update`, `group:archive`, `group:read`, `group:grant`, `role:read`, `tenant:read` — and deliberately **not** `permission:archive` | every §1b negative needs a real non-superadmin caller, and `GR5`'s negative needs one specific permission it does **not** hold |
| **H** | the **same** tenant | `group:read`, `group:update` — and **not** `group:grant` | it is the only caller for which the fifth verb is visible: it may relabel a group and must get **403** on both collection routes. B and G answer the same on all seven either way |

**The target role of `GR5` is created by the ADMIN, not by G — and that is not a convenience.**
`Role`'s own escalation rule (`RL2`) would refuse G the creation of a role granting
`permission:archive`, since G does not hold it. The admin crosses that scope; G is then asked to
*attach* it, which is the question `GR5` exists to ask.

**Fixture classes, all lane-owned:**

1. **Tenants per lifecycle state** — `GR7` needs a `trial`, a `suspended`, an archived and a
   nonexistent one; `K5` needs a second live tenant for the cross-tenant key; `GR3` needs one
   holding a live role that a group in another tenant will try to attach; `K7.5` needs one it
   can archive without touching anything else.
2. **Fifty roles**, for `GR11`'s edge. Created by the admin in one tenant, keys
   `qa-cap-<run>-01…50` through the existing `new_role` helper.
3. **Three revocation chains**, for `GR12a/b/c` — each a permission + role + group + member user,
   fully disjoint from every other fixture in the suite.

**A fixture trap worth naming before it bites.** `vos.GroupKey` refuses a run of 4 identical
runes, anything outside `^[a-z0-9]+(-[a-z0-9]+)*$`, and fewer than 2 distinct runes — so a run
tag like `11115` would 422 every insert and take the lane RED for a fixture reason rather than a
service one. `qa/lib/common.sh` already carries `qa_slug_runid`, which collapses runs of 3+ to
two; the group fixtures **reuse it** rather than writing a second sanitizer.

**The suite invariant stands, extended:** no lane may archive the seeded `master` tenant, the
`master` role, the `*:*` catalog row — **or the bootstrap admin's own group membership**, of
which there is none, which is precisely why `GR12`'s chains can archive freely. Every archive
case in this round operates on a row the suite itself created.

**Residue, as promised:** one throwaway database, now also holding the lane's groups,
group_roles, roles, tenants and users. Nothing is ever written to `authcore_db`.

---

## 3. Security — never `N/A`

**Where a valid token comes from: the service itself** (`auth.issuer.enabled: true`), through
`POST /auth/user/token`. Nothing is invented, and no token is forged except the ones that exist
to be refused — all of which the tenant round already owns.

Everything Group-specific lands in the existing `qa/security.sh` under a new **`S6.x`** block
(tenant owns `S3.x`, permission `S4.x`, role `S5.x`).

### 3a — The 401 half: INHERITED. See `specs/qa/tenant-contract/plan.md` §3a.

The middleware does not know which route it is guarding, so `S3a.1`–`S3a.10` are not repeated.
One row is added, because a route that was never gated at all would still pass every one of
them: **`S6.0` `GET /groups` with no `Authorization` header → 401
`MissingAuthorizationNotification`.**

### 3b — The public-route half: direction 2 for the seven routes this round adds

`auth.publicRoutes` names no `/groups` path. Direction 1 is inherited. What this round adds:

- each of the **seven** REST routes, **tokenless → 401**;
- each of the **seven** GraphQL fields, tokenless → **401** in the REST envelope (the bearer is
  checked before the document is parsed).

### 3c — The 403 half. All three layers.

**Layer 1 — the gate, per verb AND per surface.** A route gated on REST is not thereby gated on
GraphQL, and a route can lose its gate alone.

| # | token | request | expected |
|---|---|---|---|
| `S6.1a-g` | principal **B** (`tenant:read` only) | each of the seven REST routes | **403** `MissingPermissionNotification`, field `permission`, value **`group:read` / `group:insert` / `group:update` / `group:archive` / `group:grant`** as the route declares |
| `S6.2a-g` | **the admin** | the same seven | **2xx** — and for a write aimed at an id that addresses nothing, a **404**, never a 403: reaching the HANDLER is what proves the gate opened. A gate that refuses everyone is also broken |
| `S6.3a-c` | **principal H** (`group:read` + `group:update`, **no** `group:grant`) | `GET /groups`, `GET /groups/{id}`, `PATCH /groups/{id}` | **200** |
| `S6.3d-e` | the same principal H | **both collection routes** | **403**, value **`group:grant`** — Q2-B of the model made visible: "may rename the group" and "may change what the group confers" are separately grantable, and the second is the escalation surface. **No other principal can see this**, which is why H exists |
| `S6.4a-g` | principal B | the seven **GraphQL** fields | the typed 403 in this surface's idiom, `errors[].extensions.notificationKey` |
| `S6.5a-g` | the admin | the same seven fields | ok — the complement, per surface |

**Layer 2 — identity-derived rules in `BuildRules`.** Proven by pairs of calls that differ only
in **who is asking**:

| # | request | expected |
|---|---|---|
| `S6.6` | principal **G** creates a group naming the **master** tenant | **403** `TenantMismatchNotification`, field `tenantID` |
| `S6.7` | **the same body** sent by the admin (`*:*`) | **201** — the bypass, which is what lets a platform operator support a customer |
| `S6.8` | principal G archives a group belonging to another tenant | **403** `TenantMismatchNotification` — `refuseForeignTenant` runs under `IfArchive`, and the write side is **not** filtered by `ToCriteria`, so the row loads and the RULE is what refuses. This is the seam that would be invisible if only reads were tested |
| `S6.9` | principal G attaches a role granting a permission it does not hold | **403** `CannotGrantRoleWithUnheldPermissionsNotification` — cross-referenced to `GR5` |
| `S6.9b` | principal G attaches a wildcard-bearing role | **403** `CannotGrantWildcardRoleNotification` — cross-referenced to `GR4`, and never a 500 |

**Layer 3 — tenant row scoping. An isolation leak answers 200**, which is exactly why it needs
its own cases rather than riding on a refusal:

| # | request | expected |
|---|---|---|
| `S6.10` | principal G lists groups | **200**, and another tenant's group is **absent** from every page — asserted by **id**, not by count |
| `S6.11` | principal G reads another tenant's group by id | **404**, **not 403** |
| `S6.12` | the admin lists groups | **200**, and that same group **is** present — without this, a service that filtered everyone would pass `S6.10` for the wrong reason |
| `S6.13` | the same scope on **GraphQL** | principal G's connection carries only its own tenant's groups — `ToCriteria` is shared, but a surface that skipped it would look exactly like a passing REST case |

**`Restrict` — `N/A`, stated rather than skipped, and printed in the SKIP column.** `spec.md` §9
declares no field-level read authz: *"Every field a caller may see the row at all for, they may
see entirely. Row-level isolation does the work."* There is no column to find absent for one
caller and present for another, no tabular export whose header could be pruned, and no
`__typename` edge to assert — that edge exists only where a restricted field is in the
selection. Naming it here is the honest form; asserting one would be inventing a rule.

### 3d — `auth.mode` posture

`mode: jwt`, `authorization.enabled: true`, `authorization.tenant.required: true`. Neither
degenerate posture applies, and neither is available as an excuse.

### 3e — Coverage reported, not asserted in prose

The `Restrict` `N/A` and the inherited set (§0a) are printed in the report's SKIP column with
their reasons, so nothing reads as covered that was not run.

---

## 4. Out of scope, named plainly

- **Load, performance and concurrency** — ⚠️ **named, not implied.** The two partial unique
  indexes exist precisely for the race the domain pre-check cannot see, and proving a race needs
  concurrent writers this suite does not have.
- **The per-entry probe cost.** `group_service_manual.go` accepts up to 50 role round trips
  inside one write transaction, mitigated by a single walk and a request-scoped memo. That is a
  performance property, not a contract; `GR11` bounds it rather than measuring it.
- **UI** — `/docs` and the GraphQL playground are asserted reachable by the inherited lane,
  never rendered.
- **Integration events — `N/A`, not deferred:** no `transport:` block, nothing published, no
  `integration_events` table. There is no delivery half to mark ⚠️ OPEN.
- **gRPC** and **tabular exports** — neither is wired or declared.
- **`User → Group` as a CONTRACT.** `GR12` and `GR13` provision memberships and assert
  `user_groups` rows **by SQL** as a consequence of a group operation. The membership aggregate's
  own verbs, validations and permissions belong to the `user` round.
- **The remaining three aggregates** — `claim`, `client`, `user`.

### In scope, extending the existing lane: the audit trail

`qa/audit.sh` already proves the framework's in-TX `audit_events` promise for Tenant
(`A1`–`A14`), Permission (`A15`–`A23`) and Role (`A24`–`A38`). Group's shape is Role's, one
level up, so this round extends that lane rather than starting a new one (cases **`A39+`**),
applying the same decision the role gate approved on 2026-09-07:

- one row per `insert` / `update` / `archive` on the root, `entity_type = 'Group'`,
  `aggregate_id` = the row's id, `kind` `snapshot` on the insert and `transition` on the archive;
- **the two collection verbs**: an ATTACH and a DETACH each write **one `update` row against the
  ROOT's `aggregate_id`**, not the child's — a child op is a command on the root — each carrying
  a `changes` block naming the collection;
- **no `unarchive` row exists to assert**, and its absence from the timeline is itself asserted;
- `actor` = the acting principal's `sub`; `tenant_id` = the actor's tenant claim;
- the declared `auditClaims` (`email`, `tenant_workspace`, `identity_kind`) ride the payload, and
  an undeclared claim never does;
- a **refused** write leaves **no** row — including a `GR4` wildcard refusal and a `GR5`
  escalation refusal, both of which reach the domain before the transaction commits.

---

## 5. Runner contract — the SAME `qa/run.sh`, extended

**Still exactly one entry point.** This round adds two lanes and edits five existing files; the
lane list is where the addition becomes real.

```
qa/
├── run.sh                     ← EDITED: LANES gains group + group_graphql; the principal section gains G and H; PLANS names four plans
├── lib/common.sh              ← EDITED: group_body / new_group / attach_role / detach_role / role_id_of
├── tenant.sh                  ← untouched
├── tenant_graphql.sh          ← untouched
├── permission.sh              ← untouched
├── permission_graphql.sh      ← untouched
├── role.sh                    ← untouched
├── role_graphql.sh            ← untouched
├── group.sh                   ← NEW: K1–K11, REST
├── group_graphql.sh           ← NEW: the L family
├── domain.sh                  ← EDITED: §1b appended as GR1–GR13 (the R, P and RL rows stay)
├── security.sh                ← EDITED: §3 appended as S6.x
├── audit.sh                   ← EDITED: the Group family appended as A39+
├── microservice.qa.yaml       ← untouched
└── microservice.qa-key.yaml   ← untouched
```

- **Lane list becomes**:
  `tenant tenant_graphql permission permission_graphql role role_graphql group group_graphql domain security audit`
  — the two new lanes sit beside their twins, and the three shared lanes stay last so a Group
  domain rule and a Tenant one are read together.
  `./qa/run.sh group group_graphql` runs just this round's new surface work, on the same runner.
- **`PLANS` names all four plans** — the report cannot claim to execute three documents when it
  executes four.
- **`role_id_of <key>`** resolves a lane-created role id by its handle, so no UUID literal is
  written twice.
- **Everything else is inherited verbatim**: root resolution (`cd "$(dirname "$0")/.."` first in
  every lane), fail-fast by default with `--all` for the sweep, non-zero exit per lane, per-run
  namespacing of every temp file / log / binary / port, the boot sequence, SIGTERM-only shutdown
  with a waited drain, `Accept-Language: en-US` on every request, SKIPPED printed as SKIPPED, and
  a failed assertion printing the real response body.
- **`CLAUDE.md` rule 1 applies to the five EDITED files** as much as to the two new ones: they
  are named here, before anything is touched, and this gate is their approval.
- **Outside `qa/` and `specs/qa/`, and therefore called out separately:** the §0c corrections
  touch `specs/omnicore-gen/group.omnicore.yaml` (two comments) and
  `specs/scaffold-entity/group/spec.md` (§2 item 5, §9's `?fields=` token, §7's seeded-group
  claim). Comments and prose only — **no key, no field, no rule, no generated code changes**,
  and each carries a dated supersession note. Approved by the maintainer on 2026-09-07 at the
  Phase 0b-2 gate; listed here so the approval is on the record.

---

## 6. Report contract — unchanged

`qa/qa-report.md`, rendered live and rewritten in full after every lane, same header / matrix /
failures / footer / abort-trap / SKIP-column rules as `tenant-contract` §6. The only change is
that the matrix grows from nine rows to eleven. A lane that never ran still prints `—`.

`qa/qa-report.md` and `qa/.logs/` remain run artifacts, **offered** as `.gitignore` lines at
hand-off and never added by this skill. Neither `qa/` nor `specs/qa/` is ever gitignored.

---

## 7. Gate — closed 2026-09-07

| Item | Answer |
|---|---|
| §1b `GR12` — the three switches of the transitive revocation | ✅ asked 2026-09-07 — *"Os três interruptores"* |
| §1b `GR11` — the cap in its HYBRID form (50 real → 201, the 51st → clean 422) | ✅ asked 2026-09-07 |
| §2 — principals **G** and **H** | ✅ asked 2026-09-07 |
| §0c — the four stale statements: report **and** correct with a dated supersession | ✅ asked 2026-09-07 |
| §1b ranking | `GR1`, `GR2`, `GR3`, `GR4`, `GR4b`, `GR5`, `GR5c`, `GR6`, `GR6b`, `GR7`, `GR9`, `GR12` **critical** |
| **The plan as a whole, including the five EDITED files of §5 and the two prose files of §0c** | **Approved** 2026-09-07 |

---

## 8. Run record — 2026-09-07

`./qa/run.sh --all` · **✅ ALL GREEN — 11/11 suites · 1254 cases · 32s** (the footer of
`qa/qa-report.md`, quoted rather than paraphrased).

| lane | pass | fail | skip |
|---|---:|---:|---:|
| `tenant` | 111 | 0 | 0 |
| `tenant_graphql` | 36 | 0 | 0 |
| `permission` | 131 | 0 | 0 |
| `permission_graphql` | 37 | 0 | 0 |
| `role` | 208 | 0 | 4 |
| `role_graphql` | 44 | 0 | 0 |
| **`group`** | **214** | **0** | **4** |
| **`group_graphql`** | **51** | **0** | **0** |
| `domain` | 202 | 0 | 5 |
| `security` | 167 | 0 | 3 |
| `audit` | 53 | 0 | 0 |

**Reconcile.** `ls qa/*.sh` minus `run.sh` equals the runner's `LANES` array exactly, eleven
for eleven. Every family of §1 exists in the generated suite and RAN: `K1` 12 · `K2` 15 ·
`K3` 8 · `K4` 21 · `K5` 10 (9+1 skip) · `K6` 15 (14+1) · `K7` 9 · `K8` 61 · `K9` 46 (45+1) ·
`K10` 15 (14+1) · `K11` 6 — 218 in all; `L1` 20 · `L2` 8 · `L3` 6 · `L4` 10 · `L5` 5 · `L6` 2.
§1b ran as **70** cases in `qa/domain.sh`, §3 as **52** in `qa/security.sh`, §4's audit
extension as **15** (`A39`–`A53`).

Every §1b row carries BOTH halves except the ones the matrix already says otherwise about:
`GR2` and `GR3` share `GR1`'s positive by construction (one message, three questions — that
sharing IS the assertion), `GR4b`/`GR4c`/`GR5b`/`GR5c`/`GR6b`/`GR6c` are complements of their
parent row, `GR9`/`GR10` are positive-only because their refusals are `GR2`/`GR5` firing on
ADDED entries in the same run, and `GR8b` has no positive half because the door does not exist.

**Nothing skipped for a fixture reason.** All 16 skips are the declared-`N/A` families named
in this plan and its three predecessors — principals **G** and **H** were built, and all three
`GR12` revocation chains ran end to end.

**The suite can fail** — the mandatory meta-case, run and then undone. `K1.1` was flipped by
hand to assert **200** where the service answers **201**. The lane went RED, the runner exited
**non-zero** and fail-fast stopped the run, `qa/qa-report.md` showed `group` RED with the other
ten lanes printing `—` rather than vanishing, the failures section named the case with
expected-vs-received and the real 201 body, and the footer read `❌ RED — 1 of 1 suites`. The
expectation was then restored byte-for-byte and the full sweep re-run green.

### Four suite defects were corrected before this record — none by weakening a case

1. **A raw space is not a URL.** `?name.eq=QA Filter Group 2` made curl refuse the request
   outright (HTTP 000), taking five `K8` cases with it — a fixture failure wearing a contract
   failure's clothes. The role round paid for this once; it is now paid twice, and the fix is
   percent-encoding rather than a softer assertion.
2. **`K11.2` was mis-transcribed from its own source.** `group_routes.go` mounts the
   collection at path `"/"` under `app.Group("/groups")`, so the document publishes
   **`/groups/`**. The case had been written as `/groups`. The SOURCE said the trailing slash;
   the case did not. `K11.2b` was added to assert that the slash-less form still serves, so
   the correction records both halves instead of just relaxing one.
3. **`K11.3` read the wrong buffer.** The case added in fix 2 overwrote `HTTP_BODY` between
   the fetch of `/openapi.json` and the assertions that read it. Moved to the end of the lane.
4. **`L1.4c` tested something the surface cannot answer.** An unknown field inside a
   **variable's** input object is DROPPED by coercion, not refused, so the original case could
   never see the structure it was about. Replaced by three cases that can: `L1.4c`
   introspects `PatchGroupInput` and asserts it declares `description` and `name` **alone**;
   `L1.4d` puts `key` in the **document**, where gqlparser validates it; and `L1.4e` records
   the variable path's real contract — 200 with the key UNCHANGED — rather than smoothing it
   over. This is stated rather than hidden: a case asserting a validation error on the
   variable path would pass for the wrong reason against any schema.

### One expectation was RE-DERIVED rather than adjusted to the answer

`GR12a4` was written as *"the `group_roles` row survives, still ACTIVE"*, by analogy with the
role round's `RL11f`. That half was never derived from anything. The pin's own contract is the
opposite and is explicit — **archive cascades onto the active children**, an already-archived
child is skipped by the cascade, and under an archived scope children load unfiltered so the
read still serves them. So the row survives **stamped**, which is the history `GR12a` is
actually about. The case now asserts that, `GR12a4b` asserts nothing was DELETED, and the
cascade earned two cases of its own in `K6.11b`/`K6.11c` — a framework promise this entity
exercises that no earlier round in this suite had asserted.

**Residue, as §2 promised.** One throwaway database, `authcore_qa`, dropped and recreated at
the start of every run; at rest it holds 43 groups, 79 group_roles, 3 user_groups, 113 roles,
55 tenants, 13 users and 396 audit_events. **`authcore_db` holds 0 rows written by this
suite** — verified: `groups.group_key LIKE 'qa-%'` → 0, and the same for `tenants.workspace`
and `users.email`.

**No finding about the SERVICE came out of this round.** The four §0c items were findings
about the project's own PROSE, and all four were corrected at the source with dated
supersessions in the same pass (`specs/omnicore-gen/group.omnicore.yaml`,
`specs/scaffold-entity/group/spec.md`).
