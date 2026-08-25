# Spec: Role

- **Status:** APPROVED
- **Approved:** maintainer (Cláudio Schirmer Guedes), 2026-08-20 — the five slots of §B
  answered at the model gate. Q1 → key + name + description · Q2 → **id only, plus the
  catalog's own `resource` / `action` reached by a read join** · Q3 → **the general
  no-escalation rule** ("só pode conceder o que você tem, a não ser que seja um `*:*`") ·
  Q4 → isolation on reads AND writes, with the service-wide `auth.authorization` switch
  left to `/omnicore:configure` · Q5 → absent identity ≠ absent claim. The `(proposed)`
  picks stand
- **Pin:** omnicore **`v0.59.0`** · dialect postgres · Postgres SoR, no Mongo, no broker →
  relational-served views. Built with `omnicore-gen` **`0.37.0`**; the framework facts
  verified below were read at `v0.57.0` and re-confirmed unchanged at `v0.57.1`
- **Language:** English (all artifacts) · Portuguese (chat) — per `../../../CLAUDE.md`
  rule 3 and the maintainer's invocation
- **Generation:** omnicore-gen — chosen by the maintainer at gate 1d
- **Amended:** 2026-08-24 — §2 and §9, the per-grant `permission` token (see §2); and
  §2/§10, how `TenantID` reaches the entity (see §2's "How the owner arrives"). The model is
  otherwise unchanged and every other `(proposed)` pick stands as approved

A **tenant's own cut of the global permission catalog**. `Permission` says what the
platform can enforce; `Role` says which of those a given customer has decided to bundle
together and hand out. It is the third node of the target graph the README draws —
`User → Group → Role → Permission` — and the first aggregate in this service that is
**owned by a tenant** rather than by the platform.

That last fact is what makes this entity different from the two that exist. `Tenant` and
`Permission` are platform-global: any authenticated caller holding `tenant:read` sees every
row. A role belongs to somebody, and the whole of §10 exists because of it.

## Sources read before writing this spec

| Source | What it settled |
|---|---|
| `../../../README.md` § Domain model | **`Role` is `tenant_id` NOT NULL, defined by the tenant** — not a nullable-scope entity. Platform power comes from a **reserved tenant**, not from `NULL`. Role→Permission is many-to-many |
| `../../../README.md` § Permission (rules 2 and 3) + § Wildcards | permission keys are frozen; archive is **one-way** so a retired permission "comes back as a new row, with a new id, which must be granted explicitly"; `*:*` is a grantable-but-unenforceable catalog row |
| `../permission/spec.md` · `../tenant/spec.md` (both APPROVED) | the local flavor: shared vs entity-specific VOs, substance-validated text, archive-not-delete, the `<entity>:<verb>` permission taxonomy, the service pre-check + DB backstop uniqueness style |
| `../../scaffold-service/spec.md` (APPROVED) | posture: Postgres SoR, **no Mongo**, no broker → relational-served views; REST + OpenAPI + GraphQL wired |
| `../tenant/spec.md` §2 · `../permission/spec.md` §2 (the VO inventory they declare) | what is available to reuse: `Description`, `DisplayName`, `PermissionKey` (composite), `TenantWorkspace`, `TenantStatus`, and the shared `text_predicates` helpers |
| omnicore `v0.57.0` `/docs` | `authz-seams` (the three layers), `read-joins` (reaching the catalog across the foreign key), `relational-view` (what a SoR-backed view serves and what it inherits from the loader), `aggregate-persistence` / `table-schema` (children) |

### Verified framework facts that shaped this spec (read, not assumed)

| Fact | Evidence at the pin (`v0.57.0`) |
|---|---|
| A **relational view DOES serve the aggregate's 1:N children** — the loader reaches them "by their own keyed reads" and the served document carries them | `relational-view.html`, "A subtlety worth calling out" |
| What a relational view refuses is **filter/sort on a child field** — typed 400 `UnsupportedCapabilityNotification`, never 500 | `relational-view.html`, feature-parity table + "Unsupported capabilities return 400" |
| A **read join** reaches another aggregate across a foreign key, is declared once on the repository with `WithJoins`, and every consumer of that loader inherits it — `FindByID`, `ScopedReader`, a service calling `repo.Loader.FindAll`, and a relational view declared over it | `read-joins.html`, "Why the repository, and not the schema or the view" |
| A join hanging off a **collection** (`LeftJoinInChild` / `InnerJoinInChild`) fills its fields on every loaded entry but is **not addressable in a criteria** — the same 1:N boundary every child field has | `read-joins.html`, "Joining from an aggregate child" |
| A join's predicate is **always `fk = target.id`** — the target's declared id column, nothing else. A traversal onto a non-id column of the target is deliberately not expressible | `read-joins.html`, "Mapping columns onto your own names" |
| A join field carries **no domain type** — no value object, no `domain.ID`. An identity column of the target arrives as canonical text in a `string` field | `read-joins.html`, "What a join field may be" |
| A join may map the target's **managed columns** — `created_at`, `updated_at`, `deleted_at`; `revision` is out — and a nullable one lands in a nullable Go type: `deleted_at` arrives as `*time.Time` | `read-joins.html`, "What a join field may be" · proven by generating this entity at this pin: the child entry comes out with `ArchivedAt *time.Time`, `go build` and `go vet` clean |
| A join is **not gated on the archived state of the target**: an archived counterpart keeps supplying its columns, and an inner join keeps matching it. Mapping `deleted_at` is therefore how a read *reports* that state — never how it filters on it | `read-joins.html`, "What a NULL means, and what archived means" |
| A relational view **declares no join of its own** — it carries the loader, and the loader carries whatever the repository declared | `relational-view.html`, "Read joins come from the loader, not from the view" |
| Layer 3 tenant isolation = middleware claim gate + `crit.Filter["tenant_id"]` in `ToCriteria` + a `BuildRules` match check | `authz-seams.html`, Layer 3 |
| `TenantMismatchNotification` (403) and `TenantMissingNotification` (403) are **framework-owned and already translated in all seven catalogs** — this entity declares neither | `application/translation/*.go` line 83 |
| The claim matcher honors exactly three shapes: exact, `resource:*`, `*:*`; `HasPermission` is nil-safe | `authz-seams.html`, Layer 1 |
| `auth.authorization` is **absent from both profiles today** → Layer 1 no-ops and Layer 3 is unwired service-wide | `microservice.dev.yaml` · `microservice.prd.yaml` |
| Child ops are commands **on the root**; there is no child upsert and **no per-child unarchive** | `aggregate-persistence.html` · `conventions/aggregate-children.md` |
| A child is an aggregate value object in `aggregatevos/`, embedding `domain.Managed`, declaring **no `ID` field of its own**, and a **mandatory `IsSameBusinessIdentity`** | `table-schema.html` · `conventions/aggregate-children.md` |

---

## §A — What large platforms actually do with a role

Requested explicitly at the invocation. Eleven things converge across AWS IAM, Azure RBAC /
Entra, Google Cloud IAM, Auth0, Okta and Keycloak. Each line ends with what this spec does
with it, so nothing here is decoration.

| # | What the big platforms do | Where it lands here |
|---|---|---|
| 1 | **A role carries a stable machine key AND a human display name.** GCP: immutable `roleId` in the resource name + editable `title`. Azure: immutable GUID + `roleName`. Keycloak/Auth0: a unique `name` used in the token claim + a free `description` | **DECIDED (Q1): both.** `key` + `name` + `description` — the exact pattern `Tenant` already ships one entity over (`workspace` + `name` + `description`) |
| 2 | **The key is unique per scope, not globally.** Two customers may both have `administrator` | §2 — unique over `(tenant_id, key)`, not over `key` |
| 3 | **The display name is never unique.** Nobody makes it so | §2 — `name` not unique, mirroring `Tenant.name` |
| 4 | **Predefined vs custom roles.** GCP predefined / Azure built-in are platform-owned and immutable; customers create their own beside them | **Already settled by the README, not re-asked.** No nullable scope and no `is_builtin` flag: platform roles are ordinary rows owned by the **reserved platform tenant**. The README rejects `OR tenant_id IS NULL` explicitly |
| 5 | **The permission set is validated against a catalog.** Azure validates actions against the resource provider's operation list; GCP against the published permission list. You cannot invent a permission inside a role | §7 R6 — a domain-service probe against `permissions`, exactly like `PermissionKeyTaken` |
| 6 | **A role definition caps its permission count.** GCP: 3 000 per custom role. Azure: 2 000 actions | §7 R8 — a cap, proposed at 200. Also a claim-size budget: the permission spec sizes one rendered entry at 129 runes |
| 7 | **Deleting a role in use is guarded.** Azure refuses to delete a role definition while assignments exist; GCP soft-deletes with a recovery window and then makes the id unusable for 37 days, precisely so a new role cannot inherit old bindings | §5/§6 — soft archive only, and the one-way question. The README already made this exact argument for `Permission` |
| 8 | **Role hierarchy / composite roles exist but are the minority.** Keycloak has composite roles and NIST RBAC1 has inheritance; AWS, GCP and Azure deliberately have **none** — a role is a flat set | **Rejected, recorded.** Flat set. Inheritance turns "what can this user do?" into a graph walk with cycle detection, and the union-of-two-paths model the README draws already gives the composition |
| 9 | **No deny rules.** GCP added deny policies only in 2022, as a separate object; Azure `NotActions` is a subtraction inside one definition | **Rejected, recorded.** The README states it: "There is no precedence and no deny rule: a permission is held or it is not" |
| 10 | **Privilege escalation is the named threat.** AWS permissions boundaries; GCP requires the granter to hold `setIamPolicy`; every platform stops a tenant admin minting themselves more power than they have | **DECIDED (Q3): the general no-escalation rule** — §7 R9a. A `*:*` holder is exempt by construction |
| 11 | **Assignment is a separate object from definition.** Azure `roleAssignments` ≠ `roleDefinitions`; GCP bindings ≠ roles | **Out of scope, by design.** `User→Role` and `Group→Role` are their own aggregates, later. This entity is the *definition* only |

Two further notes that are about this codebase rather than about the industry:

- **A grant must not silently survive a permission's retirement.** The README's Permission
  rule 3 says a retired permission "comes back as a **new row, with a new id**, which must
  be granted explicitly". A role that stored the *string* `tenant:export` would silently
  re-attach to the recreated row and break that promise; one that stores the **id** cannot.
  This is what decides §3's reference shape, and it is derived from the project's own stated
  invariant rather than from taste.
- **`ROLE` is a reserved word in Postgres.** Harmless for the TABLE name — every identifier
  in this project's DDL is quoted — but the migration must not be the first place that stops
  quoting.
- **`key` is a reserved word across the engine set, so the physical column is `role_key`.**
  The generator refuses a bare `key` in the union of the five engines it guards against.
  Only the storage name moves: the **exposed** name stays `key` everywhere — every filter,
  `?orderBy` token, OpenAPI parameter, GraphQL argument and JSON field. Exactly the move
  `../permission/spec.md` already made for `resource` → `resource_name`, and for the same
  reason: the DDL stays portable if a second dialect is ever added.

---

## 1. Storage model                                    [high-risk — confirm]

- **Kind: flat** (proposed; alternative: sharedbase-role — rejected).
  **No identity smell.** A role carries no person, no document, no e-mail, no natural
  registry key of an asset. It is a definition owned by a tenant, not a party playing a
  role, and there is no second role that a `billing-manager` could also become. A shared
  base would buy structure with nothing to share. This one is not close, so it is proposed
  rather than opened.

- **ER sketch**

```
tenants                          roles                              role_permissions
─────────                        ─────                              ────────────────
id            UUID PK      ┌──── id           UUID PK         ┌──── id            UUID PK
tenant_id     UUID UQ  ────┘     tenant_id    UUID FK ────────┘     role_id       UUID FK → roles.id
workspace     …                  role_key     VARCHAR(64)          permission_id UUID FK → permissions.id
…                                name         VARCHAR(120)         revision / created_at
                                 description  VARCHAR(500)         updated_at / deleted_at
                                 revision / created_at                    │
                                 updated_at / deleted_at                  │
                                                                          │
                                 permissions ─────────────────────────────┘
                                 id UUID PK · resource_name · action_name · description

UNIQUE (tenant_id, role_key) WHERE deleted_at IS NULL     -- roles
UNIQUE (role_id, permission_id) WHERE deleted_at IS NULL  -- role_permissions
```

| Table | Description (becomes the table COMMENT) |
|---|---|
| `roles` | A tenant's own bundle of catalog permissions — the unit a user or a group is granted. Owned by exactly one tenant; the platform's own roles live in the reserved platform tenant rather than in a null scope. |
| `role_permissions` | The permissions a role grants. One row per permission in the bundle, holding nothing but the catalog row's id — so a retired-and-recreated permission is never silently re-granted, and the pair is never stored twice. |

- **The two cross-aggregate foreign keys are written into the migration BY HAND.** The
  generator writes the PARENT key only — `role_permissions.role_id` → `roles.id` — because a
  collection's owner is part of the aggregate that declares it. A reference to *another*
  aggregate is outside the spec language, so `roles.tenant_id` → `tenants.id` and
  `role_permissions.permission_id` → `permissions.id` are appended by whoever writes the
  migration. That is the designed path and not an adoption: the migration is a hook file,
  written once and never regenerated. Both are load-bearing — §2's `inner` join is only safe
  because `permission_id` is `NOT NULL` and FK-backed.

- **The FK target is `tenants.id`** *(corrected 2026-08-24)*. Tenant has exactly one
  identifier: the derived UUIDv5 that used to sit beside the PK was removed, because a
  tenant's id is pinned inside every other microservice that stores it and was therefore a
  public key in practice regardless. The `tenant_id` JWT claim now carries this same value,
  so Layer 3 writes the claim straight onto the filter with no translation step — the
  property the earlier two-key model was reaching for, reached by having one key instead of
  two.

- If sharedbase-role: `N/A — flat`.

## 2. Fields                                 [one row per field]

| Field | Go type | VO? | Nullable | Unique | Lives on | example: | Description |
|---|---|---|---|---|---|---|---|
| `TenantID` | `domain.ID` | plain (an id) | no | no (part of the composite unique) | root | `a3f1c07e-2b58-5d94-8e61-4f2093ab77d5` | The tenant that owns this role. **Sent in the request body**, not taken from the claim (see below). Immutable after creation — a role never moves between tenants. Declared `required`: it is a plain id, so nothing validates its presence by type |
| `Key` | `vos.RoleKey` | **new-raw** `vos.RoleKey` | no | **yes, per tenant** | root | `billing-manager` | Stable machine handle of the role, unique within its tenant and immutable. What an API caller and an audit line reference; never the display name |
| `Name` | `vos.DisplayName` | **reuse** `vos.DisplayName` | no | no | root | `Billing Manager` | Human-readable name of the role as operators and end users see it. Not unique — two tenants, or two roles, may share a label |
| `Description` | `vos.Description` | **reuse** `vos.Description` | no | no | root | `Grants read access to the tenant registry and the permission catalog, without any write verb.` | What holding this role actually lets a user do, in the tenant's own words |
| `Permissions` | `[]aggregatevos.RolePermission` | aggregate VO → §3 | — | — | child | — | The catalog permissions this role grants |

Child `RolePermission` (`internal/domain/aggregatevos/`):

| Field | Go type | Source | Nullable | Unique | example: | Description |
|---|---|---|---|---|---|---|
| `PermissionID` | `domain.ID` | **stored column** `permission_id` | no | yes, within the role | `9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a3` | The catalog row this grant points at — the id and not the string, so a retired-and-recreated permission needs an explicit re-grant |
| `Resource` | `string` | **read join** → `permissions.resource_name` | no | — | `tenant` | What the granted permission protects. Read-only, filled on load, never written through this aggregate |
| `Action` | `string` | **read join** → `permissions.action_name` | no | — | `read` | What the granted permission allows on that resource. Same read-only contract |
*(`ArchivedAt` was declared here and REMOVED 2026-08-24. Nothing in this service read it,
and republishing it contradicted its owner: `Permission` deliberately keeps `deletedAt` off
its own reads, reaching archived state through `?includeArchived`. Surfacing the same column
through a join was a back door to a decision the owning aggregate had already made the other
way. The question it answered — "does this role still grant something retired?" — becomes
real when `User → Role` lands, and is a fresh decision then.)*

The ROOT also traverses into its OWNER *(added 2026-08-24, expressible only since the
derived tenant key was removed — see §1)*:

| Field | Go type | Source | Description |
|---|---|---|---|
| `TenantWorkspace` | `string` | **read join** → `tenants.workspace` | The owning tenant's handle. **Filterable and sortable** — a root join's fields are addressable in a criteria, unlike a child's |
| `TenantStatus` | `string` | **read join** → `tenants.status` | The owning tenant's commercial lifecycle. Plain `string`, never `vos.TenantStatus`: a join field carries no domain type |

### How the owner arrives — corrected after a live bench found it

*(Amended 2026-08-24, after a `POST /roles` on a dev bench answered 500.)*

`TenantID` is **carried in the request body**. An earlier build declared it
server-assigned from the `tenant_id` claim, and that was wrong in two ways that only a
running service showed:

1. **The super-admin case became inexpressible.** A server-assigned field is absent from
   every write DTO, so a platform operator had no way to say WHICH tenant — while §10 of
   this very spec promises "a superadmin may create anywhere".
2. **A dev bench could not create a role at all.** With `auth.mode: disabled` there is no
   identity, so nothing filled the field and the write died on an empty id.

**The row scope is not weakened by this.** The claim decides what a caller MAY write; it
never had to be the only thing that CAN write it. `refuseForeignTenant` already refuses a
value that is not the caller's own claim (403), and the super-admin bypass already crosses
it — both were written before this correction and needed no change. What the correction
restores is the ability to SAY the value.

**And it is `required`.** The framework's by-type validation reaches value-object-backed
fields only, and a plain `domain.ID` is not one — so before this rule nothing refused an
empty owner and it travelled all the way into the uniqueness pre-check, where it became a
500 rather than a 422. The rule is declared, and the manual `tenant-must-exist` rule's
early return on an empty owner is only correct because it is.

**The barrier that makes this safe.** The owner carries a `valueObject` rule with
`guard: true`, which emits three lines before anything else in the verb:

```go
e.TenantID.IsValid("TenantID", r.Context())   // domain.ID validates itself
r.IgnoreValueObject("TenantID")               // so the automatic pass does not repeat it
r.StopIfInvalid()                             // and nothing below runs on a bad owner
```

Without it the owner was still UNCHECKED at that point — the framework's value-object pass
runs AFTER the rules — so it travelled into the uniqueness pre-check, which is scoped BY the
owner and hands it straight to a criterion. A value a UUID column refuses made the probe's
query error and the probe panic: a 500 on a request whose problem is plain validation.

**It replaces a `required` rule rather than joining one.** `uuid.Parse` refuses the empty
string and a non-UUID alike, so one call covers both; a `required` rule beside it would tell
the caller the same thing twice. That ordering matters more than it looks: an earlier fix
used `required` alone and closed only the empty case — a non-empty non-UUID still reached the
probe whenever the row-scope guard had nothing to reject, which is every request on a dev
bench. Both shapes are pinned by a regression test.

Two consequences of the barrier, both deliberate:

- **It ends the whole pass**, including the automatic value-object validation and every
  collection of the aggregate. An insert with a bad owner AND a junk description reports the
  owner only — the point of a barrier is that nothing after it is meaningful.
- **It fires on anything already rejected**, not only on this rule. `refuseForeignTenant`
  runs earlier in the same verb, so a caller writing into somebody else's tenant also stops
  here and learns about the mismatch alone.

Needs `omnicore-gen` ≥ 0.38.0 (`kind: valueObject`, `guard`) and framework ≥ v0.59.0
(`Rules.StopIfInvalid`).

**One STORED field, and three read across the foreign key (Q2).** The `(resource, action)`
pair is still **not copied** into `role_permissions` — that denormalization is what §A
rejects, and it stays rejected. What the row holds is the id; what the *read* returns is
the id plus the pair plus the counterpart's archive stamp, fetched across the FK at load
time. Four consequences:

- **The read returns the permission itself, not just a UUID.** `GET /roles/:id` answers
  `"permissions": [{ "id": "…", "permissionID": "9f14b0a2-…", "resource": "tenant",
  "action": "read", "archivedAt": null }]`. A client renders `tenant:read` by joining the
  two halves it was handed, with no second call to `GET /permissions` — and a non-null
  `archivedAt` is how the same answer says *this grant points at a permission that has
  been retired*, which is the one thing an access review most needs to see and the one
  thing a stored id alone could never tell it.
- **Nothing about the write side moves.** A join field is not part of the `TableSchema`,
  so it never enters an INSERT or an UPDATE (`read-joins.html`, "What a read join is
  not"). The grant is still one column, and the invariant §A protects — a retired
  permission comes back as a new row with a new id, so a grant holding the *string* would
  silently re-attach and one holding the **id** cannot — is untouched, because the string
  is never stored.
- **`permission_id` is a real database foreign key** to `permissions.id` (the PK, so
  FK-able — unlike the partial unique index over the pair). It is also what makes the join
  an `inner` one legal: the column is `NOT NULL` and referential integrity guarantees the
  counterpart, so no entry is ever dropped. Referential integrity for the *existence* half
  of R6 comes free; the rule still earns its keep for the *active* half, and for turning a
  violation into a readable 422 instead of a raw constraint error.
- **The rendered `resource:action` string IS served, one per grant — and the two halves
  are NOT.** *(Amended 2026-08-24, maintainer's decision: "só o ID + `role:read`".)* Each
  entry carries `permissionID` and `permission`, and `resource` / `action` are declared
  `hidden` — they exist to feed the derivation and reach no response body, no listing row
  and no export. A caller reads one token here and one token on the catalog's own endpoint,
  instead of joining two halves in every client.

  **This reverses an earlier position in this spec, and the reason it reversed is worth
  keeping.** The original text said a per-grant token was "not expressible", because
  `read.computed` lands on the aggregate's ROOT result and one value per root derived from
  N entries means nothing. That was true of omnicore-gen `0.35.0`. It also uncovered three
  generator defects — `check` accepted a collection-path source, the emitter dropped it
  with a bare `continue`, and the hook name was not qualified by entity — all fixed at
  `0.36.0`, which added `children[].computed`: the per-entry seat, whose `from:` names the
  entry's own fields bare and reaches its join fields.

  What did NOT change is where the separator lives. The derivation rebuilds
  `vos.PermissionKey` and calls `String()`; nothing in this service concatenates a resource
  and an action, exactly as `../permission/spec.md` requires.

### The read join into the catalog — the declaration and its four properties

```
read.InnerJoinInChild(schemas.PermissionSchema()).To(...)   -- shape; the repository declares it
  RolePermission.permission_id  =  permissions.id
    → Resource    ← permissions.resource_name
    → Action      ← permissions.action_name
    → ArchivedAt  ← permissions.deleted_at        -- managed column; arrives *time.Time
```

Declared **once on `RoleRepository` with `WithJoins`**, beside `WithSchema`. Every consumer
of that loader inherits it in the same call: the relational view of §9, `repo.FindByID` (the
load the write-side auto handlers go through), `repo.ScopedReader(ctx)`, and any service
calling `repo.Loader.FindOne/FindAll`. That last one is not incidental — it is what
`../group/spec.md` §7 spends its escalation probe on.

Four properties that are the framework's, not this model's, and that every layer must hold:

1. **`inner`, and only because the key is `NOT NULL`.** Over a nullable foreign key an inner
   join silently drops rows — from `FindByID` too, which would turn a legitimate write into
   a 404. The framework refuses that combination at construction; here there is nothing to
   refuse, and the choice is intent rather than risk. **Inside a child, an inner join drops
   the ENTRY, not the aggregate** — one more reason the `NOT NULL` + FK pair is what makes
   it safe.
2. **Load-only: not filterable, not sortable.** A child join field is served on every loaded
   entry and is **not** addressable in a criteria — the same 1:N boundary every child field
   already has. `?filter[permissions.resource][eq]=tenant` is a typed 400. See §9 for what
   that costs.
3. **Not gated on the catalog's archived state — and it reports that state instead of
   hiding it.** An archived permission keeps supplying its `resource` and `action`, and the
   inner join keeps matching it. That is the wanted behaviour here: a grant pointing at a
   retired row must stay *readable* — an access review has to be able to see what a past
   grant meant, which is the whole reason `Permission` never hard-deletes. Mapping the
   catalog's `deleted_at` onto `ArchivedAt` is the other half of the same intent: the entry
   is served either way, and the reader is told which it is. `Permission` archives one-way,
   so this is not a transient — a grant can point at a retired row for the rest of the
   role's life, and the read says so on every load.

   The line the field does **not** cross: it renders, it never judges. §7 is where "is that
   key still live" is decided, and it is decided by a probe, not by this column.
4. **Filled on load, empty on an added entry.** An entry the request just added exists only
   in memory; nothing traversed a foreign key for it. So `Resource`, `Action` and
   `ArchivedAt` are populated for the grants **loaded from the row** and empty for the ones
   this write is adding — and note the direction `ArchivedAt` fails in: an added entry
   reads `nil`, which is indistinguishable from "the target is live". §7 depends on this and
   states it again where it bites.

**No traversal to `Tenant` is declared, and the reason is a trap worth naming.** A join's
predicate is always `fk = target.id`. `roles.tenant_id` does not point at `tenants.id` — it
pointed at `tenants.tenant_id`, a derived key that no longer exists (§1) — **a traversal
into Tenant IS expressible now**, and §7 R4 notes what it would and would not answer. The
paragraph below is kept as the record of why it was refused while that key existed. A
declaration reaching Tenant
would therefore be *accepted* (the column is an id) and would render
`roles.tenant_id = tenants.id`, which matches nothing: a `left` join would fill every field
with NULL and an `inner` one would drop every role from every read, `FindByID` included.
**Do not declare it.** The tenant's `workspace` and `name` are not reachable from this
aggregate, and the honest fix, if they are ever wanted on a role listing, is a second
foreign key onto `tenants.id` — never a join over the existing one.

Notes on the decisions embedded above:

- **`domain.ID` is the right type at this pin.** `table-schema.html`'s supported column
  shapes make an id-holding field `domain.ID` and the dialect's native id column follows
  from the Go type; `Tenant.TenantID` in this project already proves the pattern. Wire DTOs
  stay `string` and convert at the mappers.
- **`Resource` and `Action` are plain `string`, NOT `vos.PermissionKey` and not two halves
  of it.** A join field carries no domain type at all — the framework refuses a value
  object of any kind, and it refuses `domain.ID`. The reason is not mechanical: the value
  belongs to `Permission`, arrives read-only, and is never validated by *this* domain, so
  reconstructing `vos.PermissionKey` here would hand back an instance no rule of the owning
  aggregate ever approved. It is also structurally impossible — a composite spans two
  columns and a join field maps exactly one. The catalog validates its own pair; this
  aggregate reads the two columns it was given.
- **`vos.RoleKey` is a NEW raw VO, not a reuse.** `TenantWorkspace` is the closest shape
  (a DNS-ish slug) but it carries two rules that belong to nothing else — the platform's
  reserved-route list and `DeriveTenantID` — so reusing it would drag both onto a role.
  `RoleKey` gets its own file: 2–64 runes, the same lowercase-slug pattern already written
  twice in this codebase, the same anti-junk predicates (`distinctRunes`,
  `hasRunOfIdenticalRunes`) from `text_predicates.go`, and **no reserved list and no
  derivation**. If Q1 lands on "no key", this row and the VO disappear together.
- **Uniqueness enforcement style: domain pre-check + DB backstop** (the project's
  established style — `TenantService.WorkspaceTaken`, `PermissionService.PermissionKeyTaken`).
  A `RoleService.RoleKeyTaken(tenantID, key, selfID)` probe in `BuildRules` so the duplicate
  reports **together with** the other validation errors, plus
  `CREATE UNIQUE INDEX roles_tenant_id_key_key ON roles (tenant_id, key) WHERE deleted_at IS NULL`
  as the race backstop, bound in the repository's `Constraints` map to a 409.

## 3. Children (1:N)

| Child | Of whom | Edit strategy | Restorable alone? |
|---|---|---|---|
| `role_permissions` | the flat root (`roles`) | **B — targeted per-child ops** (proposed; alternatives: A replace-all, C own aggregate) | **no** — so B is legal; a grant that had to be revived on its own would force C |

- **Why B and not A.** A replace-all `PUT` means an omitted permission is revoked. That is
  in fact what Azure and GCP do (the role definition carries the whole `actions` array and
  you send it entire), so A is a defensible choice a reviewer might prefer — but it makes
  every partial client a silent mass-revoker, and a revocation is the one write here that
  should never happen by omission. **B** makes each grant and each revocation an explicit,
  auditable call.
- **The trio is a PAIR here, not a trio.** Strategy B's canonical ops are ADD / UPDATE /
  ARCHIVE. A `RolePermission` has **no editable field** — its single column *is* its
  identity — so "update this grant" has no meaning: changing which permission is granted IS
  revoking one and granting another. Only two child ops are generated:
  - **GRANT** `POST /roles/:id/permissions` — body carries `permissionID`; the server mints
    the child id.
  - **REVOKE** `PATCH /roles/:id/permissions/:childId/archive` — soft removal. **Never
    `DELETE`**: the row lingers with a `deleted_at` stamp, and a `DELETE` that soft-removes
    is a lying contract.

  Both are commands **on the root** (load root → a domain method mutates the one child →
  the framework persists the diff), and both dispatch `ModeUpdate`, so `IfInsertOrUpdate`
  in §7 covers them — verified: `rules-dsl.html` maps `GetPartialUpdatable` to `ModeUpdate`,
  and an AVO's own `BuildRules` fires under the root's mode with the root's `actionName`.
- **The by-id guard lives in a domain method on `Role`**, not in a loop inside the command
  mapper: absent child → the canonical `RecordNotFoundNotification` (404), never the
  framework's `EntityDoesNotExistNotification` (422).
- **`IsSameBusinessIdentity` is over `PermissionID`, and the explicit form is now
  load-bearing.** The entry struct carries three fields, and two of them are read-only join
  fields that are blank on a freshly added entry (§2, property 4). `domain.IsSameByBusinessFields`
  would compare all three and answer "different" for a grant that duplicates a stored one —
  same permission, blank `Resource`, populated `Resource` — which is the duplicate guard
  failing open. Naming `PermissionID` explicitly is what keeps R7 correct. It is what the
  framework's GRANT path reuses as its own duplicate guard.
- **No per-child unarchive**, by framework construction. Re-granting a revoked permission is
  a fresh GRANT with a fresh child id — which reads correctly in the audit trail and matches
  the README's stance on `Permission`.
- **⚠️ The root-archive auto handler is instantiated exactly ONCE per surface.** Wiring it to
  the child REVOKE route type-checks, boots, answers 200 — and archives the entire role.
  It is the canonical trap of this model and is called out again in the web task.

## 4. Siblings (1:1)

`N/A — no facet worth splitting.` The model has **no optional field at all**: `tenant_id`,
`key`, `name` and `description` are each required. With nothing nullable there is no sparse,
bulky or PII facet to move off the root, so the sibling question has no candidate group to
be asked about. (Recorded rather than omitted: this is the decision, not a skipped step.)

## 5. Modes                                             [required]

`Display, Insert, Update, Archive` — **one-way archive, no `Unarchive`** (proposed;
alternative: add `Unarchive`, following `Tenant` rather than `Permission`).

The two existing entities split on exactly this, so the precedent has to be chosen rather
than inherited:

- `Tenant` has `Unarchive` — a tenant is a customer relationship that can resume.
- `Permission` deliberately has **none**, and the README's rule 3 gives the reason:
  restoring a retired row "would re-enable, in a single call, every grant still pointing at
  it: users would silently regain a permission nobody re-approved, and the audit trail would
  read as a restore rather than a grant."

**That argument transfers to a role verbatim, one level up.** A role is granted to users and
groups; if those grants survive the archive (they point at the role's id, which does not
change), then a single `PATCH /roles/:id/unarchive` silently re-authorizes every user who
still holds it, with no re-approval and an audit line that reads "restored". A retired role
should come back as a **new row that must be granted again**.

Named honestly, because it is the cost: **GCP goes the other way** — `roles.undelete` exists,
with a recovery window, precisely because rebuilding a 200-permission role by hand after a
fat-finger is brutal. If that operator-error case matters more here than the
silent-reauthorization one, the answer is `Unarchive` plus a rule that refuses it when the
key has since been retaken. Say the word and it is one mode and one rule.

## 6. Delete semantics                                  [required]

**Soft only — archive, no hard delete.** `PATCH /roles/:id/archive`; there is no `DELETE`,
on the root or on the child. Consistent with both existing entities and with §A item 7:
purging a role destroys the only human-readable record of what a past grant meant, which is
exactly what an access review needs to read. Per-child revocation is
`PATCH /roles/:id/permissions/:childId/archive` — same verb-truth rule.

## 7. Business rules                        [required]

| # | Field(s) | Rule | Verb scope | Notification | HTTP |
|---|---|---|---|---|---|
| R1 | `Key` | Immutable after creation — it is what API callers and audit lines reference | `IfUpdate` | `RoleKeyIsImmutableNotification` | 422 |
| R2 | `TenantID` | Immutable after creation — a role never moves between tenants | `IfUpdate` | `RoleTenantIsImmutableNotification` | 422 |
| R3 | `Key`, `TenantID` | Unique **per tenant**, over active rows. Service pre-check with exclude-self + partial unique index as backstop | `IfInsertOrUpdate` | `RoleKeyAlreadyExistsNotification` | 409 |
| R4 | `TenantID` | The owner tenant must exist, not be archived, and not be **suspended**. A `trial` tenant is a live customer and passes — "unavailable" is not "not active" | `IfInsert` | `RoleTenantDoesNotExistNotification` | 422 |
| R5 | `TenantID` | **Tenant isolation.** The row's tenant must equal the caller's `tenant_id` claim, unless the caller is a `*:*` superadmin | `IfInsertOrUpdate` + `IfArchive` | `domain.TenantMismatchNotification` (framework-owned, already translated) | 403 |
| R6 | `Permissions[].PermissionID` | Every granted permission must exist in the catalog **and be active** | `IfInsertOrUpdate`, over the entries this write ADDS | `PermissionNotInCatalogNotification` | 422 |
| R7 | `Permissions[]` | No duplicate permission within one role — `IsSameBusinessIdentity` on the GRANT path, plus the explicit guard on the by-id path, plus the partial unique index as backstop | `IfInsertOrUpdate` | `RoleAlreadyGrantsPermissionNotification` | 409 |
| R8 | `Permissions[]` | At most **200** permissions in one role (proposed; GCP caps at 3 000, Azure at 2 000 — 200 is sized to this platform, not to theirs) | `IfInsertOrUpdate` | `TooManyPermissionsInRoleNotification` | 422 |
| **R9a** | `Permissions[]` | **No privilege escalation** — a caller may only grant a permission they themselves hold. A `*:*` superadmin passes for everything, by construction | `IfInsertOrUpdate`, over the entries this write ADDS | `CannotGrantUnheldPermissionNotification` | 403 |
| **R9b** | `Permissions[]` | **No wildcard grant through the API** — a permission with `*` in either part cannot be granted on any role | `IfInsertOrUpdate`, over the entries this write ADDS | `CannotGrantWildcardPermissionNotification` | 403 |
| — | `Key`, `Name`, `Description` | format, length, substance, anti-junk | — | **not declared here** — `vos.RoleKey`, `vos.DisplayName` and `vos.Description` validate by type on every write. Declaring `required` beside a VO makes the caller read the same complaint twice | 422 |

### R4 — why the commercial status counts, and why `trial` does not

*(Corrected 2026-08-24. An earlier build of this rule checked existence and archiving only,
on the reading that the commercial status is "orthogonal to archiving" — which the Tenant
model does say. That reading was wrong for THIS rule.)*

`Tenant` carries a commercial lifecycle (`trial` · `active` · `suspended`) beside its
archive stamp, and the two really are independent: archiving forces `suspended`, but a
tenant can be suspended while perfectly un-archived — a customer who stopped paying. What
the earlier reading missed is what a role IS. A role is the unit that grants access, so
minting one inside a suspended tenant hands out exactly what the commercial state says to
withhold. Existence and archiving are not enough.

**`trial` passes, and that is not an oversight.** "Unavailable" is not "not `active`": a
trial is a live customer being onboarded, and roles are the first thing they need. Only
`suspended` withholds. A rule written as `Status != active` would refuse every trial
signup — the plausible-looking mistake this paragraph exists to prevent.

The probe answers all three in ONE query: the active scope excludes an archived row by
default, and a status predicate excludes a suspended one.

### R5 — how the identity reaches the rule

Layer 2 in `authz-seams.html` terms: a rule about the relationship between the principal and
the resource, which Layer 1's static `RequirePermission` cannot express. The identity is
translated into the entity by the **command mapper** — the only layer allowed to read `ctx`
— onto runtime-only fields that are **not declared in the `TableSchema`**, so the persister
never persists or scans them:

- `RequestingTenantID` ← `ctx.Identity().TenantID()`
- `RequestingPrincipalIsSuperAdmin` ← **`ctx.Identity().IsSuperAdmin()`** — the framework's
  sanctioned way to ask the `*:*` question. **Never** `HasPermission("*:*")`, which
  panics by design, and never a hand-read of the claim: the claim NAME is configurable via
  `authorization.permissionsClaim`, so parsing `Claims["permissions"]` by hand starts
  answering `false` the day an operator renames it. `IsSuperAdmin` is nil-safe, reads the
  CONFIGURED claim name, tolerates all four claim shapes, and shares the parsed-claim cache
  with `HasPermission`. Like every `Identity` helper it reads the TOKEN, not the gate — so it
  is unaffected by the `auth.authorization.enabled` master switch that §10 leaves off. Note a
  resource wildcard is **not** a superadmin grant: `role:*` reports `false`

### Absent identity vs absent claim — two states, not one (Q5)

`authorization.tenant.required` is off service-wide (Q4), so the claim can be missing. But
there are **two** ways for the identity side to come up empty, and collapsing them breaks
one profile or the other:

| State | When | Verdict |
|---|---|---|
| **No `Identity` at all** (`ctx.Identity()` is nil) | `auth.mode: disabled` — the middleware is bypassed and never populates one | **no identity gate** — R5 and R9a stand down |
| **`Identity` present, claim empty or insufficient** | any authenticated request | **refuse** — 403, fail closed |

Collapsing them into a single fail-closed rule makes the entity **unusable in the dev
profile**: `HasPermission` is nil-safe and answers `false`, `TenantID()` answers `""`, so
every write and every grant would be refused on a service that has no tokens at all.

**The safety of the first row is not this entity's promise — it is the framework's boot
guard.** `AuthModeDisabled` is *"allowed only when `APP_PROFILE="dev"`; LoadConfig rejects
it under any other profile (prd or QA variants) so a non-dev boot cannot ship without auth
wired"* (`bootstrap/auth_config.go:14-16`, read). A nil identity therefore **cannot** occur
outside dev: a prd deployment that tried it aborts at boot rather than reaching these rules.

The rules must express this as the two-state test above, never as a bare
`if !hasPermission { refuse }` — and the dev-only branch is a named, tested case (§7 tests),
not an incidental nil check.

### R9a/R9b — the framework panic that shaped this pair

**Read in the source, not assumed** (`application/configuration/identity.go:52`):

```go
if strings.Contains(p, "*") {
    panic("...: wildcards are not allowed on the caller side; got " + strconv.Quote(p))
}
```

`HasPermission` **panics on any argument containing `*`** — it also panics on an empty
string or one without a colon. So the naive form of the no-escalation rule — "for each
granted permission, ask `HasPermission(key)`" — **crashes the request into a 500 the moment
somebody tries to grant the `*:*` catalog row**, which is exactly the case the rule exists
to stop. That is why the choice landed as a pair rather than as one rule:

- **R9b runs first and removes the panic input.** A wildcard permission is refused for
  everyone, on every role, so no wildcard string ever reaches `HasPermission`.
- **R9a then asks the question safely**, because every remaining key is concrete. And it
  gets the superadmin exemption for free: `HasPermission` returns `true` for *any* concrete
  permission when the claim set contains `*:*` (source, same function). So "só pode conceder
  o que você tem, a não ser que você seja um `*:*`" is one call, not two — no special case
  and no claim parsing of our own.

**What R9b costs, stated plainly.** The platform's own superadmin role — the one that
grants `*:*` — cannot be created through this API. That is consistent with where the README
already puts it: the reserved platform tenant is *"seeded by migration"*, and its status
table lists it as **not started**. The `*:*` role is seeded beside that tenant, in the same
migration, when that work happens. If instead the platform tenant should be able to grant
wildcards through the API, the relaxation is one clause — `unless the role's tenant is the
platform tenant` — and it needs no claim parsing either, because it tests the ROW's tenant,
not the caller's token. It is left out now only because the constant it would compare
against does not exist yet.

### What these three rules judge — added entries, not the stored ones

R6, R9a and R9b run on `IfInsertOrUpdate`, and on an update they judge **only the entries
that write ADDS**. All three ask about the act of granting, and a grant already in the row
was asked all three when it entered; re-asking them makes unrelated writes hostages of the
past:

- a permission the platform retires **after** a grant would make the role impossible to
  rename — 422 on a request whose only change is a label;
- a caller who has since lost a permission could no longer even **revoke** the others, since
  a revocation is an update and every remaining grant would be re-judged against a claim set
  that no longer holds them.

Neither refusal describes anything the caller is doing. An **insert is unchanged** — every
entry of a new role is an added one, so the whole collection is still judged there — and a
**revoke asks nothing**, because it adds nothing. Mechanically this is
`domain.GetAddedItemsOf` rather than `GetCurrentItemsOf` (`OpInsert`, which crosses the
original and current status, so a row loaded from the database is excluded and a re-granted
one is not).

Cost, named: a grant that was legitimate when made is never re-vetted. That is the intended
trade — revocation is the tool for a grant that stopped being acceptable, and it stays
reachable precisely because of this rule shape.

### Why the read join does NOT answer R6, R9a or R9b

Worth stating up front, because §2 just put `Resource`, `Action` and `ArchivedAt` on the
entry and the obvious next thought is that the probes are now redundant. **They are not**, and the reason
is one sentence: these three rules judge the entries a write ADDS, and an added entry has
has no joined value — nothing traversed a foreign key for a struct the mapper built from a
request body a millisecond ago. Reading `Resource` off it yields `""`, which R9b would read
as "no wildcard" and R6 as nothing at all. A security rule that passes on a blank field is
the worst possible failure direction.

**`ArchivedAt` is the sharpest case of that, and it is barred from the rules outright.** It
is the one joined field whose empty value is also a perfectly legitimate one: an added
entry reads `nil`, and so does a grant on a live permission. A rule that skipped archived
keys by testing `ArchivedAt == nil` would therefore wave through **every entry the write is
adding** — fail-open on exactly the path R6 exists to close. R6 keeps asking the probe
whether the id is in the catalog and still active. `ArchivedAt` is a rendering field, and
§9 is the whole of its job.

The join and the probes therefore answer two different questions, and both are needed:

| | The read join | The domain-service probe |
|---|---|---|
| Answers for | grants **already stored** on a loaded role | the grant this write is **adding** |
| Question | "which key does this grant point at, and is that catalog row retired" | "is that id in the catalog, still active, a wildcard, and held by the caller" |
| Costs | nothing — the columns ride the child's own SELECT | one lookup per added entry |
| Sees archived counterparts | yes, returns them, and stamps them with `ArchivedAt` | yes, and that is exactly what it reports on |
| Trusted by a rule | **never** | yes — it is the authority for all three |

What the join *does* change is who else can stop paying for the lookup: a **consumer** of
this repository's loader now gets the keys for free. `../group/spec.md` §7 is built on that
— its escalation check resolves a role to its permission keys in one read through
`RoleRepository.Loader` instead of a second query into `permissions`.

### The domain port — three per-entry facts, named for the problem

R6, R9a and R9b all ask about the catalog rows behind the granted ids, and the shape the
port may take is fixed by two properties of the spec language, both confirmed at this pin
(`omnicore-gen explain rules`):

- **A fact answers with a SCALAR** — `bool` / `int64` / `float64` / `string`. A map over the
  whole collection is not expressible, so "the active keys behind these ids" cannot be one
  lookup returning a map.
- **A fact about a collection is asked ONCE PER ENTRY** —
  `filters: [<collection>.<field>]`, the entry's field arriving as the argument.

- **And a fact is named for the PROBLEM, never for the healthy state.** The generated suite
  stubs the service so every probe answers "nothing found". A fact spelled `TenantIsActive`
  reads false under that stub and therefore means *the tenant is gone*, turning a correct
  spec red on the day it is written. `TenantIsUnavailable` reads false and the happy path
  passes. This is not a style preference — it is what makes the generated suite green.

So the port is:

```
RoleService (internal/domain/role_service.go) — plain values, no error, per the local pattern
  RoleKeyTaken(tenantID domain.ID, key string, selfID domain.ID) bool
  TenantIsUnavailable(tenantID domain.ID) bool
  PermissionIsNotInCatalog(permissionID domain.ID) bool    // per entry — unknown or archived (R6)
  PermissionIsWildcard(permissionID domain.ID) bool        // per entry (R9b)
  CallerDoesNotHoldPermission(permissionID domain.ID) bool // per entry, concrete keys only (R9a)
  CallerIsSuperAdmin() bool                                // ctx.Identity().IsSuperAdmin() (R5, R9a, §10)
```

**The cost this shape carries, stated rather than discovered.** The happy path asks three
questions per granted entry instead of one per collection, bounded by the 200-permission
cap; the refusing paths short-circuit. Two mitigations belong in the build and not in a
later optimization pass: the domain walks the collection **once** (a single
`refuseUngrantablePermissions` pass, not three), and the implementation funnels all three
facts through **one** cached catalog lookup per entry, memoised on the request-scoped store
so three questions about one id cost one query. A role at the cap then pays 200 round trips
inside the write transaction rather than 600.

`CallerDoesNotHoldPermission` is where the ctx-bound seam pays off: `ScopedService(ctx)`
binds the request to the service in this project, so the impl reads `s.ctx.Identity()`
without the domain ever seeing a context. It **guards the wildcard itself** and answers
"does not hold" rather than calling through — defence in depth behind R9b, because a panic
here is a 500 on a security rule.

**Cross-aggregate reach:** the three per-entry facts query `permissions` and
`TenantIsUnavailable` queries `tenants`, so the impl holds those repositories beside its
own — keyed by the owning repository, never in a package-level `sync.Once`, which would hand
the first engine's repositories to every later service. Confirm the shape against
`service-to-service.html` before writing it.

## 8. Update shape                                      [required]

**PATCH only.** No sibling in §4, so the PUT-invariant does not bind. Mirrors
`patch_tenant` and `patch_permission` — the whole service speaks PATCH.

Only `Name` and `Description` are actually patchable: `Key` and `TenantID` are frozen by R1
and R2, and `Permissions` moves through the §3 child ops rather than through the root body.

## 9. Surfaces & reads                       [required]

- **REST: yes** (OpenAPI documented) · **GraphQL: yes** — both, mirroring `Tenant` and
  `Permission`.
- **gRPC: no** — available later through `/omnicore:implement`, no rework.
- **Exports (CSV/XLSX): no** (proposed) — operator-facing and small, exactly the call the
  permission spec made. One flag if wanted; an access-review export is the plausible reason
  to want it.
- **Integration events: no** — the posture has no broker, so publishing is unavailable.
  Noted so it is not silently forgotten.
- **Reads:** by-id + by-params.
- **Reserved read controls served by the listing:** pagination (`first`/`last`/`after`/
  `before`) · `orderBy` · `?fields=` **yes** · `?onlyTotal` **yes** · `?includeArchived`
  **yes** · **`?search=` no** — the relational backing answers it with a typed 400
  (`UnsupportedCapabilityNotification`, `SemanticSchema`), so declaring it would advertise
  a capability the server refuses.
- **Computed read fields: one, on the GRANT and not on the root.** `permission` —
  `resource:action` rendered per entry, from the two join fields, through
  `vos.PermissionKey.String()`. Declared under `children[].computed` (the per-entry seat,
  `omnicore-gen` ≥ `0.36.0`); its body is in
  `internal/application/queries/role_computed_manual.go`, written once and never
  regenerated. Nothing on the ROOT is derived, and nothing else in this model is.
- **Field-level read authz (`ReadCriteria.Restrict`): none.** Every field a caller may see
  the row at all for, they may see entirely. Row-level isolation does the work here. (Noted
  because the backing matters: on a relational view a restricted column is still read from
  the SoR and dropped before the document is served — the value never reaches the wire, but
  it is not withheld from the SELECT. A column whose VALUE must not leave the database
  belongs behind a view that does not project it. Nothing here is in that class.)
- **View backing: relational** — `query.RelationalView("roles", repo.Loader)`, contributed
  through the feature's `RelationalViews()` opt-in (the sibling of `ReadableFeature`). The
  project posture, and the only option without Mongo. It takes its schema from the loader,
  so there is no `.Schema(...)` to get out of step, and it carries **no** `Version`, no
  registry row, no rebuild and no Mongo collection — changing what it serves, including
  adding a read join, needs no version bump.
- **The view inherits the read join and declares nothing.** `Resource`, `Action` and
  `ArchivedAt` reach the served document because `RoleRepository` declared the traversal,
  not because the view asked for it. One consequence worth stating: the view is not the source of truth for the
  reads — the loader is — so a service reading through `repo.Loader` sees exactly what the
  endpoint sees.
- **`?fields=` reaches the joined values under `permissions.resource`,
  `permissions.action` and `permissions.archivedAt`** — a child join's fields are addressed
  as `<segment>.<field>`, the same shape as any leaf inside a collection. A path this read model does not have is a
  **400** (`SchemaViolationNotification`, `SemanticSchema`) naming the offending Go path —
  never a silent `200 {}`. Note the cost profile: this backing composes the aggregate as
  declared and prunes afterwards, so a narrow `?fields=` shapes the answer without buying
  any I/O.
- **Pagination is a camouflaged offset.** The wire contract is identical to a Mongo view —
  `?first=`, `?after=`, `?before=`, `endCursor`/`startCursor` emitted only where the
  neighbour exists — but each cursor carries an absolute row index rather than a sort-key
  tuple. Paging a static result set visits every row exactly once; a row inserted or removed
  ahead of the window shifts every later page by one, so a walk under heavy concurrent
  writes can skip or repeat. The right trade for an operator-facing listing, and recorded so
  it is not discovered during an access review.
- **The one real cost, stated up front:** a caller **cannot filter or sort by a granted
  permission**. The read join renders it, it does not make it addressable —
  `?filter[permissions.resource][eq]=tenant` and `?orderBy=permissions.action` are both a
  typed 400 (`UnsupportedCapabilityNotification`). This is the 1:N boundary, not the
  backing: a filter on a child field is a pushdown a single root `SELECT` cannot express.
  **"Which roles grant `tenant:read`?" is still not answerable from this listing** — what
  changed is that once you have the roles, you can see what each one grants without a second
  call. The reverse question becomes answerable the day the service gains Mongo
  (`/omnicore:configure`), with no change to this model.

- **Filter/sort operators per field** (low-risk — decided):

| Field | `filter:` | `sort:` |
|---|---|---|
| `tenantID` | `eq,in` | `asc,desc` |
| `key` | `eq,ne,in,startswith,istartswith,contains,icontains` | `asc,desc` |
| `name` | `eq,in,startswith,istartswith,contains,icontains` | `asc,desc` |
| `description` | `contains,icontains` | — |

## 10. Authorization                          [required]

### Layer 1 — the permission gate

`role:insert` · `role:update` · `role:archive` · `role:read` (proposed) — the taxonomy the
service already grants on its other eight routes, extended with no new verb. The action
spells the operation; `:update` covers PATCH; `:archive` would cover unarchive too if §5
gains it.

The two child ops ride the parent's verb rather than inventing their own:

| Operation | Route | Permission |
|---|---|---|
| create | `POST /roles` | `role:insert` |
| patch | `PATCH /roles/:id` | `role:update` |
| archive | `PATCH /roles/:id/archive` | `role:archive` |
| list | `GET /roles` | `role:read` |
| by id | `GET /roles/:id` | `role:read` |
| grant a permission | `POST /roles/:id/permissions` | `role:update` |
| revoke a permission | `PATCH /roles/:id/permissions/:childId/archive` | `role:update` |

Alternative worth naming: a distinct `role:grant` for the two child ops, so "may edit the
role's label" and "may change what the role can do" are separately grantable. That is a real
distinction — the second is the privilege-escalation surface — but it adds a verb the rest of
the service does not have. **Proposed: `role:update` for both**; say the word for `role:grant`.

### Layer 2/3 — data access

**Every row is tenant-scoped. Q4 answered: isolation binds READS and WRITES both.**

- **Writes (Layer 2):** R5. The row's `tenant_id` must equal the caller's claim, else 403
  `TenantMismatchNotification`. A `*:*` superadmin bypasses it and may act on any tenant's
  roles. On insert the same rule applies — a caller may only create inside their own tenant;
  a superadmin may create anywhere.
- **Reads (Layer 3):** `ToCriteria(ctx)` injects
  `crit.Filter["tenant_id"] = ctx.Identity().TenantID()`, so a listing returns only the
  caller's own roles, and a by-id read of another tenant's role returns **404 rather than
  403** — it does not exist for this caller, which leaks nothing about who else exists. **A
  `*:*` holder skips the filter and sees every tenant's roles**, which is what lets a
  platform operator support a customer — detected with `ctx.Identity().IsSuperAdmin()`, the
  case the framework names for it ("cross-tenant bypass inside `Query.ToCriteria(ctx)`").
- The rejected alternative, recorded: writes-only isolation, with every authenticated caller
  able to *read* every tenant's roles. It is the narrower reading of *"alterar"*, and it
  leaks each customer's org structure to every other customer.

### The service-wide switch this entity depends on

**Neither profile configures `auth.authorization` today.** `microservice.dev.yaml` is
`auth.mode: disabled`; `microservice.prd.yaml` is `mode: jwt` with no `authorization:` block,
and that block defaults to **off** — so `RequirePermission` currently no-ops on all eight
existing routes and `Identity.TenantID()` is never required to be present.

Layer 1 is unaffected in shape (the strings are declared either way, and the OpenAPI
description suffix is suppressed until the runtime gate is live). **Layer 3 is not**: with
`tenant.required: false`, `ctx.Identity().TenantID()` can be empty, and the isolation filter
would then be a no-op that silently returns every tenant's roles. The rules must therefore
treat an empty claim as a **refusal**, not as a pass — and the honest fix is the prd block:

```yaml
auth:
  authorization:
    enabled: true
    tenant:
      enabled: true
      required: true
```

That is a **service-wide** change: it turns enforcement on for the eight existing routes at
the same time, and boot-time enforcement then panics on any non-public route missing a
`RequirePermission` declaration. Every existing route already declares one, so the scan
should pass — but this is a posture decision, not an entity decision. **Q4 answered: left to
`/omnicore:configure`.** This run generates the rules and the `ToCriteria` filter ready to
enforce, and they fail closed until the switch is flipped.

---

## §B — the open questions, as answered

All were answered at the model gate; nothing in this spec is defaulted or assumed.

| # | Question | Answer | What it changed |
|---|---|---|---|
| Q1 | Does a role carry a key and a display name, or only a description? | **key + name + description** | §2 gains `Key` (new VO `vos.RoleKey`) and `Name` (reuse `vos.DisplayName`); R1 and R3 exist because of it |
| Q2 | How does a grant reference the catalog, and what does a read of it return? | **`permission_id` is the only STORED field — no denormalized pair — and the read reaches `resource` / `action` across the foreign key** | §2's child stores one column and serves four — the id, the pair, and the counterpart's archive stamp; the copy §A rejects is still rejected, because the pair is read, not written; a real FK backs the traversal and makes an `inner` join safe; R6/R9a/R9b still probe, because they judge ADDED entries and those carry no joined value |
| Q3 | May a tenant-owned role grant a wildcard? | **the general no-escalation rule** — you may only grant what you hold, unless you are `*:*` | R9a + R9b, and the framework-panic finding that forced them to be a pair |
| Q4 | Does the tenant rule gate reads too, and do we flip the config now? | **reads and writes both; the config switch is left to `/omnicore:configure`** | §10 Layer 3 filter; the fail-closed treatment of an empty tenant claim |
| Q5 | How do R5 and R9a treat an ABSENT identity (dev, auth disabled)? — raised after the plan gate, when the maintainer asked where the caller's permissions are actually compared | **nil identity ⇒ no identity gate; identity present but insufficient ⇒ refuse** | §7's two-state table; the dev profile stays usable and prd stays fail-closed, guaranteed by the framework's own boot guard rather than by this entity |

### Two things the answers left as open work, recorded so they are not lost

1. **The reserved platform tenant does not exist** (`README.md` status table: *not started*).
   R9b's consequence is that the platform's own `*:*` role has to be seeded by migration
   alongside that tenant, when that work happens — not created through this API. See §7 R9b
   for the one-clause relaxation if that turns out to be the wrong call.
2. **`auth.authorization` is off service-wide**, so Layer 1 no-ops on every route this
   service mounts and the tenant claim is not required. The rules generated here enforce
   correctly the moment it is switched on, and fail closed until then. `/omnicore:configure`
   owns the flip.
