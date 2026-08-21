# Spec: Role

- **Status:** APPROVED
- **Amended:** maintainer, 2026-08-20 — §B Q5 (absent identity vs absent claim) answered after
  the plan gate; §7 gained the two-state table. No other slot moved
- **Approved:** maintainer (Cláudio Schirmer Guedes), 2026-08-20 — the four OPEN slots
  of §B answered at the model gate. Q1 → key + name + description · Q2 → **id only** ·
  Q3 → **the general no-escalation rule** ("só pode conceder o que você tem, a não ser que
  seja um `*:*`") · Q4 → isolation on reads AND writes, with the service-wide
  `auth.authorization` switch left to `/omnicore:configure`. The `(proposed)` picks stand
- **Language:** English (all artifacts) · Portuguese (chat) — per `../../../CLAUDE.md`
  rule 3 and the maintainer's invocation
- **Generation:** omnicore-gen — chosen by the maintainer at gate 1d, 2026-08-20

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
| `internal/domain/vos/` | inventory for reuse: `Description`, `DisplayName`, `PermissionKey` (composite), `TenantWorkspace`, `TenantStatus`, `text_predicates` |
| omnicore `v0.55.0` `/docs` | `authz-seams` (the three layers), `relational-view` (what a SoR-backed view serves), `aggregate-persistence` / `table-schema` (children) |

### Verified framework facts that shaped this spec (read, not assumed)

| Fact | Evidence at the pin (`v0.55.0`) |
|---|---|
| A **relational view DOES serve the aggregate's 1:N children** — the loader reaches them "by their own keyed reads" and the served document carries them | `relational-view.html`, "A subtlety worth calling out" |
| What a relational view refuses is **filter/sort on a child field** — typed 400 `RelationalCapabilityNotification`, never 500 | `relational-view.html`, feature-parity table + "Unsupported capabilities return 400" |
| No read-time join exists on this posture: `Embed`/`Link`/`ComposedView` are boot-fail or not expressible without Mongo | `relational-view.html`, feature-parity table |
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
- **`ROLE` is a reserved word in Postgres.** Harmless here — every identifier in this
  project's DDL is quoted — but the migration must not be the first place that stops
  quoting.

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
workspace     …                  key          VARCHAR(64)          permission_id UUID FK → permissions.id
…                                name         VARCHAR(120)         revision / created_at
                                 description  VARCHAR(500)         updated_at / deleted_at
                                 revision / created_at                    │
                                 updated_at / deleted_at                  │
                                                                          │
                                 permissions ─────────────────────────────┘
                                 id UUID PK · resource_name · action_name · description

UNIQUE (tenant_id, key)      WHERE deleted_at IS NULL     -- roles
UNIQUE (role_id, permission_id) WHERE deleted_at IS NULL  -- role_permissions
```

| Table | Description (becomes the table COMMENT) |
|---|---|
| `roles` | A tenant's own bundle of catalog permissions — the unit a user or a group is granted. Owned by exactly one tenant; the platform's own roles live in the reserved platform tenant rather than in a null scope. |
| `role_permissions` | The permissions a role grants. One row per permission in the bundle, holding nothing but the catalog row's id — so a retired-and-recreated permission is never silently re-granted, and the pair is never stored twice. |

- **The FK target is `tenants.tenant_id`, not `tenants.id`.** The public derived key is what
  the `tenant_id` JWT claim carries, and Layer 3 writes that claim straight onto
  `crit.Filter["tenant_id"]`. Pointing the FK at the surrogate `id` would make the isolation
  filter need a lookup on every read. `tenants_tenant_id_key` is a unique index, so Postgres
  accepts it as an FK target.

- If sharedbase-role: `N/A — flat`.

## 2. Fields                                 [one row per field]

| Field | Go type | VO? | Nullable | Unique | Lives on | example: | Description |
|---|---|---|---|---|---|---|---|
| `TenantID` | `domain.ID` | plain (an id) | no | no (part of the composite unique) | root | `a3f1c07e-2b58-5d94-8e61-4f2093ab77d5` | The tenant that owns this role. Immutable after creation — a role never moves between tenants |
| `Key` | `vos.RoleKey` | **new-raw** `vos.RoleKey` | no | **yes, per tenant** | root | `billing-manager` | Stable machine handle of the role, unique within its tenant and immutable. What an API caller and an audit line reference; never the display name |
| `Name` | `vos.DisplayName` | **reuse** `vos.DisplayName` | no | no | root | `Billing Manager` | Human-readable name of the role as operators and end users see it. Not unique — two tenants, or two roles, may share a label |
| `Description` | `vos.Description` | **reuse** `vos.Description` | no | no | root | `Grants read access to the tenant registry and the permission catalog, without any write verb.` | What holding this role actually lets a user do, in the tenant's own words |
| `Permissions` | `[]aggregatevos.RolePermission` | aggregate VO → §3 | — | — | child | — | The catalog permissions this role grants |

Child `RolePermission` (`internal/domain/aggregatevos/`):

| Field | Go type | VO? | Nullable | Unique | example: | Description |
|---|---|---|---|---|---|---|
| `PermissionID` | `domain.ID` | plain (an id) | no | yes, within the role | `9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a3` | The catalog row this grant points at — the id and not the string, so a retired-and-recreated permission needs an explicit re-grant |

**One field, and that is the whole child (Q2 = id only).** The rendered `resource:action`
pair is NOT copied into the row. Three consequences, all accepted deliberately:

- **The read returns UUIDs, not permission strings.** `GET /roles/:id` answers
  `"permissions": [{ "id": "…", "permissionID": "9f14b0a2-…" }]`. A client that wants to
  display *what* the role grants calls `GET /permissions` and joins by id itself. The
  server cannot do it for them: with no Mongo there is no read-time join
  (`relational-view.html` — `Embed` / `Link` / `ComposedView` are all unavailable), so the
  only way to render the string server-side would have been the denormalized copy that was
  rejected. This stops being the client's problem the day the service gains Mongo, with no
  change to this model.
- **`permission_id` is a real database foreign key** to `permissions.id` (the PK, so
  FK-able — unlike the partial unique index over the pair). Referential integrity for the
  *existence* half of R6 comes free; the rule still earns its keep for the *active* half,
  and for turning a violation into a readable 422 instead of a raw constraint error.
- **The no-escalation rule (R9a) now costs a lookup.** It has to compare permission
  *strings* against the caller's claim, and the row holds only an id — so the domain
  service resolves ids → keys on every write that touches the collection. One probe
  answers both R6 and R9a, so it is one query and not two.

Notes on the decisions embedded above:

- **`domain.ID` is the right type at this pin.** `table-schema.html`'s supported column
  shapes make an id-holding field `domain.ID` and the dialect's native id column follows
  from the Go type; `Tenant.TenantID` in this project already proves the pattern. Wire DTOs
  stay `string` and convert at the mappers.
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
- **`IsSameBusinessIdentity` is over `PermissionID`.** Today that happens to be every field,
  so `domain.IsSameByBusinessFields` would behave identically — the explicit form is written
  anyway, because it states which column carries identity and survives a second field being
  added later. It is what the framework's GRANT path reuses as the duplicate guard (R7).
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
| R4 | `TenantID` | The owner tenant must exist and not be archived | `IfInsert` | `RoleTenantDoesNotExistNotification` | 422 |
| R5 | `TenantID` | **Tenant isolation.** The row's tenant must equal the caller's `tenant_id` claim, unless the caller is a `*:*` superadmin | `IfInsertOrUpdate` + `IfArchive` | `domain.TenantMismatchNotification` (framework-owned, already translated) | 403 |
| R6 | `Permissions[].PermissionID` | Every granted permission must exist in the catalog **and be active** | `IfInsertOrUpdate` | `PermissionNotInCatalogNotification` | 422 |
| R7 | `Permissions[]` | No duplicate permission within one role — `IsSameBusinessIdentity` on the GRANT path, plus the explicit guard on the by-id path, plus the partial unique index as backstop | `IfInsertOrUpdate` | `RoleAlreadyGrantsPermissionNotification` | 409 |
| R8 | `Permissions[]` | At most **200** permissions in one role (proposed; GCP caps at 3 000, Azure at 2 000 — 200 is sized to this platform, not to theirs) | `IfInsertOrUpdate` | `TooManyPermissionsInRoleNotification` | 422 |
| **R9a** | `Permissions[]` | **No privilege escalation** — a caller may only grant a permission they themselves hold. A `*:*` superadmin passes for everything, by construction | `IfInsertOrUpdate` | `CannotGrantUnheldPermissionNotification` | 403 |
| **R9b** | `Permissions[]` | **No wildcard grant through the API** — a permission with `*` in either part cannot be granted on any role | `IfInsertOrUpdate` | `CannotGrantWildcardPermissionNotification` | 403 |
| — | `Key`, `Name`, `Description` | format, length, substance, anti-junk | — | **not declared here** — `vos.RoleKey`, `vos.DisplayName` and `vos.Description` validate by type on every write. Declaring `required` beside a VO makes the caller read the same complaint twice | 422 |

### R5 — how the identity reaches the rule

Layer 2 in `authz-seams.html` terms: a rule about the relationship between the principal and
the resource, which Layer 1's static `RequirePermission` cannot express. The identity is
translated into the entity by the **command mapper** — the only layer allowed to read `ctx`
— onto runtime-only fields that are **not declared in the `TableSchema`**, so the persister
never persists or scans them:

- `RequestingTenantID` ← `ctx.Identity().TenantID()`
- `RequestingPrincipalIsSuperAdmin` ← see below; **not** `HasPermission("*:*")`

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

### R6 + R9a share one probe

Both need the same fact: the active catalog keys behind the granted ids. The domain port
therefore exposes one lookup, not two, and the rules read it once:

```
RoleService (internal/domain/role_service.go) — plain values, no error, per the local pattern
  RoleKeyTaken(tenantID domain.ID, key string, selfID domain.ID) bool
  TenantIsActive(tenantID domain.ID) bool
  ActivePermissionKeys(ids []domain.ID) map[domain.ID]vos.PermissionKey   // absent id ⇒ unknown or archived (R6)
  CallerHolds(key vos.PermissionKey) bool                                 // concrete keys only (R9a)
```

`CallerHolds` is where the ctx-bound seam pays off: `ScopedService(ctx)` already binds the
request to the service in this project (`internal/infra/permission_service.go`), so the impl
reads `s.ctx.Identity()` without the domain ever seeing a context. Its implementation
**guards the wildcard itself** and returns `false` rather than calling through — defence in
depth behind R9b, because a panic here is a 500 on a security rule.

**Cross-aggregate reach:** `ActivePermissionKeys` queries `permissions` and `TenantIsActive`
queries `tenants`, so the impl holds those repositories beside its own. Confirm the shape
against `service-to-service.html` before writing it.

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
  **yes** · **`?search=` no** — the relational backing answers it with a typed 400, so
  declaring it would advertise a capability the server refuses.
- **Computed read fields: none.** The obvious candidate — rendering `resource:action` on
  each grant — died with Q2: the child stores only the id, and a computed field derives from
  *stored* fields of the same document, so there is nothing to derive from. The rendering is
  the client's job (§2). Nothing else in this model is derived.
- **Field-level read authz (`ReadCriteria.Restrict`): none.** Every field a caller may see
  the row at all for, they may see entirely. Row-level isolation does the work here.
- **View backing: relational** (`.RelationalSource(repo.Loader)`) — the project posture, and
  the only option without Mongo. Confirmed serviceable: a relational view **does** carry the
  1:N children in its document.
- **The one real cost, stated up front:** a caller **cannot filter or sort by a granted
  permission** (`?filter[permissions.resource][eq]=tenant` → typed 400
  `RelationalCapabilityNotification`). "Which roles grant `tenant:read`?" is not answerable
  by this listing on this posture. It becomes answerable the day the service gains Mongo
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
  platform operator support a customer.
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

## §B — the four open questions, as answered

All four were answered at the model gate; nothing in this spec is defaulted or assumed.

| # | Question | Answer | What it changed |
|---|---|---|---|
| Q1 | Does a role carry a key and a display name, or only a description? | **key + name + description** | §2 gains `Key` (new VO `vos.RoleKey`) and `Name` (reuse `vos.DisplayName`); R1 and R3 exist because of it |
| Q2 | How does a grant reference the catalog? | **`permission_id` only** — no denormalized pair | §2's child is a single field; the read returns UUIDs and the client joins; a real FK replaces the copy; R9a gains a lookup |
| Q3 | May a tenant-owned role grant a wildcard? | **the general no-escalation rule** — you may only grant what you hold, unless you are `*:*` | R9a + R9b, and the framework-panic finding that forced them to be a pair |
| Q4 | Does the tenant rule gate reads too, and do we flip the config now? | **reads and writes both; the config switch is left to `/omnicore:configure`** | §10 Layer 3 filter; the fail-closed treatment of an empty tenant claim |
| Q5 | How do R5 and R9a treat an ABSENT identity (dev, auth disabled)? — raised after the plan gate, when the maintainer asked where the caller's permissions are actually compared | **nil identity ⇒ no identity gate; identity present but insufficient ⇒ refuse** | §7's two-state table; the dev profile stays usable and prd stays fail-closed, guaranteed by the framework's own boot guard rather than by this entity |

### Two things the answers left as open work, recorded so they are not lost

1. **The reserved platform tenant does not exist** (`README.md` status table: *not started*).
   R9b's consequence is that the platform's own `*:*` role has to be seeded by migration
   alongside that tenant, when that work happens — not created through this API. See §7 R9b
   for the one-clause relaxation if that turns out to be the wrong call.
2. **`auth.authorization` is off service-wide**, so Layer 1 no-ops on all eight existing
   routes and the tenant claim is not required. The rules generated here enforce correctly
   the moment it is switched on, and fail closed until then. `/omnicore:configure` owns the
   flip.
