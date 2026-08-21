# Spec: Group

- **Status:** APPROVED
- **Approved:** maintainer (Cláudio Schirmer Guedes), 2026-08-21 — the two OPEN slots of §B
  answered at the model gate. **Q1 → A**, `key` + `name` + `description` (the `Role` shape).
  **Q2 → A *and* B**, not one of them: the transitive no-escalation rule in the domain AND a
  distinct `group:grant` permission. That is §C-5, and it is the combination Entra ships in
  practice — A is the guarantee that cannot be misconfigured away, B is the delegation
  control that lets "may rename the group" and "may change what the group confers" be granted
  separately. Every `(proposed)` pick stands as written.
- **Language:** English (all artifacts) · Portuguese (chat) — per `../../../CLAUDE.md` rule 3
  and the maintainer's invocation
- **Generation:** `<pending>` — asked at gate 1d, after the plan gate
- **Pin:** omnicore `v0.56.1` (checked at Phase 0v: no newer release; the plugin is current
  at `0.30.0`). `v0.56.1` is a fix-only release — `BaseAggregateRepository` embeds the write
  base directly to kill a GoLand false positive. **No framework fact this spec relies on
  moved between `v0.56.0` and `v0.56.1`**, so the facts verified for `../role/spec.md` carry
  over; `Identity.IsSuperAdmin()` re-verified in the source at this pin.

The **second node** of the graph the README draws — `User → Group → Role → Permission`. A
group mirrors the customer's own org structure ("Engineering", "Finance / AP") and bundles
roles, so a person joins one team and inherits everything that team may do. It is the
inherited half of a user's effective permissions; the direct `User → Role` path is the
other, and there is no precedence between them.

Structurally it is **`Role` one level up**, and deliberately so: tenant-owned flat root,
one owned collection holding nothing but the referenced aggregate's id, per-child
grant/revoke, one-way archive, tenant isolation on reads and writes. Everything that is
NOT a copy of `Role` is called out explicitly below — there are four such places, and
three of them are the interesting part of this spec.

## Sources read before writing this spec

| Source | What it settled |
|---|---|
| `../../../README.md` § Domain model | `Group` is `tenant_id` **NOT NULL**, defined by the tenant, "it mirrors the customer's org structure". `Group → Role` is many-to-many. Effective permissions = group path ∪ direct path, no precedence, no deny |
| `../role/spec.md` (APPROVED, amended 2026-08-21) | the pattern this entity follows, and the four decisions it inherits: id-only references, per-child GRANT/REVOKE, one-way archive, the no-escalation rule and its wildcard interlock |
| `../permission/spec.md` · `../tenant/spec.md` (both APPROVED) | the local flavor: shared vs entity-specific VOs, substance-validated text, archive-not-delete, the `<entity>:<verb>` taxonomy, service pre-check + DB backstop uniqueness |
| `../../scaffold-service/spec.md` (APPROVED) | posture: Postgres SoR, **no Mongo**, no broker → relational-served views; REST + OpenAPI + GraphQL wired |
| `internal/domain/vos/` (read) | reuse inventory: `Description`, `DisplayName`, `RoleKey`, `PermissionKey`, `TenantWorkspace`, `TenantStatus`, `text_predicates` |
| `migrations/postgres/0003_role_manual.up.sql` (read) | the exact FK shape this entity mirrors: `roles.tenant_id → tenants.tenant_id` (the public derived key, NO ACTION), and `role_permissions.permission_id → permissions.id` (the PK, because the pair's own uniqueness is a partial index Postgres will not point an FK at) |
| omnicore `v0.56.1` `/docs` | `relational-view` (what a SoR-backed view serves), `changelog`, `application/configuration/identity.go` (the `IsSuperAdmin` / `HasPermission` contract) |

### Verified framework facts (read at this pin, not assumed)

| Fact | Evidence at `v0.56.1` |
|---|---|
| A relational view **does** serve the aggregate's 1:N children — "the children by their own keyed reads" | `relational-view.html` l.97-103, read |
| What it refuses is filter/sort **on a child field** — typed 400, never a 500 | `relational-view.html`, feature-parity table |
| `Identity.HasPermission` **panics** on any argument containing `*`; the panic message now ends "Compose explicit OR over concrete actions, or call IsSuperAdmin." | `application/configuration/identity.go:67-76`, read |
| `Identity.IsSuperAdmin()` is the sanctioned `*:*` question — nil-safe, reads the CONFIGURED claim name, shares the parsed-claim cache, unaffected by the `auth.authorization.enabled` switch. A resource wildcard (`group:*`) reports **false** | `identity.go:92-112` + `changelog.html` v0.56.0, read |
| `TenantMismatchNotification` / `TenantMissingNotification` are framework-owned and already translated in all seven catalogs — this entity declares neither | established by `../role/spec.md`, unchanged at this pin |
| Ordering vocabulary lives on the **Request** DTO per field (`sort:"asc,desc"`), paired with the `query:"orderBy"` switch; either half alone fails the boot | `changelog.html` v0.55.0 (breaking), read |

---

## §A — What large platforms actually do with a group

Requested explicitly at the invocation ("da uma investigada se esse padrão é usado por
grandes empresas"). **Short answer: yes — `Group → Role → Permission` is the mainstream
enterprise shape, not an invention.** AWS IAM, Microsoft Entra ID, Okta, Keycloak and
Google Cloud all ship it; they differ mostly on nesting and on how hard they guard the
group-grants-a-role edge. Each line ends with what this spec does about it, so nothing
here is decoration.

| # | What the big platforms do | Where it lands here |
|---|---|---|
| 1 | **The group-as-permission-bundle is universal.** AWS IAM user groups attach policies and users inherit them; Entra assigns roles to groups; Keycloak maps realm roles onto a group and "users that become members inherit the role mappings"; Okta assigns apps and roles to groups | The whole entity. §3 — `group_roles` is exactly this edge |
| 2 | **A group carries a human display name plus a description.** Okta's default group profile is literally `Name` (required, unique, case-sensitive) + `Description` (optional). Entra: `displayName` + `description`. SCIM 2.0 `Group` requires only `displayName` | §2 — `name` + `description`, both required here (this service validates description for substance everywhere; see `Tenant` and `Permission`) |
| 3 | **A separate stable machine handle is common but not universal.** Entra groups carry `mailNickname` beside `displayName`; AWS's `GroupName` *is* the handle and is unique per account; Keycloak groups are addressed by `path`. Okta has no separate key — it makes `Name` itself unique instead | **DECIDED (Q1 → A):** `key` + `name` + `description`. The Entra/AWS shape, not Okta's — §B for why |
| 4 | **Uniqueness is scoped, never global.** AWS: per account. Okta: per org. Entra: per tenant | §2 — unique over `(tenant_id, <handle>)`, never over the handle alone. Two customers may both have `engineering` |
| 5 | **Nesting is the minority position and the platforms that skip it say so out loud.** AWS IAM: "user groups can't be nested; they can contain only users, not other IAM groups". Okta: "Okta doesn't support nested groups" — it offers Group Rules instead. Keycloak DOES nest (subgroups inherit the parent's role mappings). Entra nests, **but forbids it on role-assignable groups** | **Rejected, recorded.** Flat. See §C-3 for the full argument — the short version is that the two platforms that make the group→role edge a security boundary are the two that refuse to nest across it |
| 6 | **The group→role edge is a named privilege-escalation surface, guarded harder than the group itself.** Entra role-assignable groups "are protected by default to prevent privilege escalation, with only high-tier administrative roles able to manage the group" — a Groups Administrator cannot add members to one; you need Privileged Role Administrator. AWS gates `iam:AttachGroupPolicy` separately from `iam:UpdateGroup` | **DECIDED (Q2 → A + B):** the transitive domain rule (G10a/G10b) *and* a distinct `group:grant`. Exactly the Entra split, guarantee plus delegation — §B |
| 7 | **Groups are frequently directory-synced, and a synced group must not be hand-edited.** Okta types groups by source (`OKTA_GROUP` / `APP_GROUP` / `BUILT_IN`) and reserves `externalId` on the group profile; SCIM 2.0 defines `externalId` precisely so a provisioning client can find the resource it created | **Not baked, offered** — §C-1 and §C-2. Two additive fields, cheap now and cheap later; neither changes anything in this model |
| 8 | **A "default"/"everyone" group exists on most platforms.** Keycloak default groups auto-apply to new users; Okta ships the Everyone group | **Deferred, recorded** — §C-4. It is a rule about *membership*, and membership is the `User ↔ Group` aggregate that does not exist yet |
| 9 | **Deleting a group in use is guarded.** No major platform hard-deletes a group that carries assignments without a warning; Entra soft-deletes with a 30-day restore window | §5/§6 — soft archive only. The one-way question is §5 |
| 10 | **Membership is a separate object from the group definition.** SCIM models `members` as a sub-resource with immutable sub-attributes; Entra `groupMembers` and AWS `AddUserToGroup` are their own operations | **Out of scope, by design.** `User ↔ Group` is its own aggregate, later — exactly as `../role/spec.md` §A-11 put `User ↔ Role`. This entity is the *definition* plus the role bundle |

**Where this design deviates from the big platforms, deliberately:** they let you attach a
role/policy to a group in a single flat operation with a single coarse permission, and
then bolt privilege-escalation protection on top as a special class of group (Entra's
role-assignable groups) or a separate IAM action (AWS). This service has no "special
class"; every group is role-assignable. That is why §B Q2 has to be answered at the model
gate instead of assumed — the guard has to live in the model, because there is no second
kind of group to hide it behind.

---

## §B — OPEN questions (must be answered; a blanket "ok" does not cover these)

### ✅ Q1 — Does a group carry a machine `key`, or is `name` the handle? → **A**

**ANSWERED at the model gate: Option A.** The reasoning below is kept as the record of what
was weighed, not as an open question.

The invocation named **name, description and the role collection**. `Role` carries a
fourth field, `key`, and "seguir mais ou menos o padrão de Role" could mean either. This
cannot be self-answered, because the two shapes are both coherent and both shipped by real
platforms — and whichever is picked, **something has to be unique per tenant**, or a tenant
ends up with five groups called "Engineering" and no way for a token issuer, an audit line
or an IdP mapping to say which one it means.

| | **Option A — `key` + `name` + `description`** (proposed) | **Option B — `name` + `description`, `name` unique per tenant** |
|---|---|---|
| Shape | `Role`'s, one level up | Okta's |
| Unique per tenant | `group_key`, active-only | `name`, active-only |
| Renaming the display label | free — the key is what references point at | **it is the identity.** A rename is a rename of the thing every IdP mapping and audit line names |
| IdP / SCIM group mapping ("map my Okta group `eng` to authcore group X") | maps onto a stable slug that never changes | maps onto a label a tenant admin may retitle on a Tuesday |
| Cost | one more required field on create; one more VO (`vos.GroupKey`) | none — strictly what was asked for |
| Fields | `tenant_id`, `group_key`, `name`, `description` | `tenant_id`, `name`, `description` |

**Recommendation: A.** Not for symmetry with `Role` — for §A-7: groups are the object
enterprise customers directory-sync, and a sync maps an external handle onto a **stable**
local one. Option B works and is honest (Okta ships it), but it makes every display-name
edit a breaking change for anything that referenced the group by name, and this service
has a token issuer and an audit trail ahead of it that will both want to name a group.

*Everything downstream of this choice — `vos.GroupKey`, rule G1, G3's uniqueness over
`group_key`, and the `key` filters — is now settled and written plainly below.*

### ✅ Q2 — What guards the group→role edge? → **A + B**

**ANSWERED at the model gate: BOTH A and B**, which the question already named as a legitimate
answer and §C-5 recommends. The domain rule is the guarantee; the separate permission is the
delegation control. C was declined. The reasoning below is kept as the record.

§A-6: this is the escalation surface. A caller who may attach roles to a group can hand
themselves — via any group they belong to — every permission any role in the tenant
grants. `Role` already answered the equivalent question one level down ("só pode conceder
o que você tem, a não ser que seja um `*:*`"), but the answer does **not** transfer
mechanically, because a role is an indirection: the caller is granting a *bundle*, and
checking it means resolving the role to its permission keys first.

| | **Option A — transitive no-escalation** (proposed) | **Option B — a distinct `group:grant` permission** | **Option C — nothing beyond `group:update`** |
|---|---|---|---|
| The check | for each role this write ATTACHES: the caller must hold **every** permission that role grants. `*:*` passes by construction | attaching/detaching a role requires `group:grant`, which "may rename the group" (`group:update`) does not imply | anyone who may edit the group may attach any role in the tenant |
| Industry analogue | the rule `Role` already ships, extended one level | Entra (Privileged Role Administrator ≠ Groups Administrator); AWS (`iam:AttachGroupPolicy` ≠ `iam:UpdateGroup`) | the naive shape every platform has moved away from |
| Escalation blocked? | **yes, by the domain** — cannot be misconfigured away | **only if the deployment grants `group:grant` narrowly.** It is a policy hook, not a guarantee | no |
| Cost | one probe per write that attaches: resolve role → its active permission keys, compare against the claim. Needs the same wildcard interlock `Role` needed (see below) | one new verb in a taxonomy that currently has four per resource | zero |
| Consistency with `Role` | exact | diverges — `Role` chose the domain rule over a `role:grant` verb | contradicts the decision `Role` already made |

**Recommendation: A** — it is the same policy the maintainer already approved for `Role`,
applied to the node that actually needs it more (a role bundles permissions; a group
bundles bundles). **A and B are not exclusive**: A is the guarantee, B is the delegation
control, and picking both is a legitimate answer.

**What A drags in, stated up front — the wildcard interlock.** `HasPermission` panics on
any argument containing `*` (verified in source, above). The platform's own `*:*` role is
seeded by migration (`../role/spec.md` §7, README). So a transitive check that resolves a
role to its keys and asks `HasPermission` on each **crashes into a 500 the moment somebody
attaches the platform superadmin role to a group** — the exact case the rule exists to
stop. Same fix as `Role`, same shape: **a role that grants any wildcard permission cannot
be attached to a group through the API** (G10b), refused *before* the escalation question
is asked. Consequence, accepted and consistent: the platform's own superadmin **group** is
seeded by migration beside the reserved platform tenant, not created through this API.

**Both halves are in.** G10a and G10b are live rules with their two `GroupService` facts, and
the child ops carry `group:grant` rather than riding `group:update` (§10). The two are
independent: revoking `group:grant` from a principal stops them reaching the rules at all,
and holding `group:grant` still does not let them attach a role they do not fully hold.

---

## 1. Storage model                                    [high-risk — confirm]

- **Kind: flat** (proposed; alternative: sharedbase-role — rejected).
  **No identity smell.** A group carries no person, no document, no e-mail, no natural
  registry key of an asset. It is an org unit owned by a tenant, not a party playing a
  role, and there is no second role an `engineering` group could also become. Same call
  `Role` made, for the same reason, and equally not close.

- **ER sketch**

```
tenants                        groups                             group_roles
─────────                      ──────                             ───────────
id          UUID PK      ┌──── id           UUID PK         ┌──── id          UUID PK
tenant_id   UUID UQ  ────┘     tenant_id    UUID FK ────────┘     group_id    UUID FK → groups.id
workspace   …                  group_key    VARCHAR(64)           role_id     UUID FK → roles.id
…                              name         VARCHAR(120)          created_at / updated_at
                               description  VARCHAR(500)          deleted_at
                               revision / created_at                     │
                               updated_at / deleted_at                   │
                                                                         │
                               roles ────────────────────────────────────┘
                               id UUID PK · tenant_id · role_key · name · description

UNIQUE (tenant_id, group_key) WHERE deleted_at IS NULL    -- groups
UNIQUE (group_id, role_id)    WHERE deleted_at IS NULL    -- group_roles
INDEX  (group_id)                                         -- group_roles, the parent read
```

| Table | Description (becomes the table COMMENT) |
|---|---|
| `groups` | A tenant's own org unit — the bundle of roles a member inherits by belonging to it. Owned by exactly one tenant; groups never nest, and membership lives in its own aggregate. |
| `group_roles` | The roles a group confers on its members. One row per role in the bundle, holding nothing but the role's id — so a retired-and-recreated role is never silently re-conferred, and the pair is never stored twice. |

- **The `tenant_id` FK targets `tenants.tenant_id`**, the public derived key the JWT claim
  carries — not the surrogate `tenants.id` — so the Layer 3 isolation filter compares the
  claim directly instead of paying a lookup on every read. `NO ACTION`, deliberately, for
  the reason written into `0003_role_manual.up.sql`: a tenant is archived and never purged.
  Verbatim mirror of the `roles` FK, verified by reading that migration.
- **`group_roles.role_id` FKs to `roles.id`, the PK.** Not to any of the partial unique
  indexes — Postgres will not point a foreign key at one. This buys the EXISTENCE half of
  G6 for free; the domain rule still earns its keep for the ACTIVE half and the SAME-TENANT
  half (see §7).
- **Reserved-word note.** `GROUP` is reserved in Postgres (`groups` plural is not, but the
  column `key` is reserved across the engine set). Every identifier in this project's DDL
  is quoted, and the migration must not be the first place that stops — and the physical
  column is **`group_key`**, exactly as `Role` stores `role_key`, so the exposed
  name stays `key` in every filter, `orderBy` token, OpenAPI parameter and GraphQL argument.
- If sharedbase-role: `N/A — flat`.

## 2. Fields

| Field | Go type | VO? | Nullable | Unique | Lives on | `example:` | Description |
|---|---|---|---|---|---|---|---|
| `TenantID` | `domain.ID` | plain (an id) | no | no (part of the composite unique) | root | `a3f1c07e-2b58-5d94-8e61-4f2093ab77d5` | The tenant that owns this group. Immutable after creation — a group never moves between tenants |
| `Key` | `vos.GroupKey` | **new-raw** `vos.GroupKey` | no | **yes, per tenant** | root | `engineering` | Stable machine handle of the group, unique within its tenant and immutable. What an API caller, an audit line and a directory mapping reference; never the display name |
| `Name` | `vos.DisplayName` | **reuse** `vos.DisplayName` | no | no — two groups in one tenant may share a label; the `key` is what disambiguates | root | `Engineering` | Human-readable name of the group as operators and end users see it |
| `Description` | `vos.Description` | **reuse** `vos.Description` | no | no | root | `Everyone in the product engineering org: read access to the tenant registry and the permission catalog, plus deploy rights.` | What belonging to this group actually lets a member do, in the tenant's own words |
| `Roles` | `[]aggregatevos.GroupRole` | aggregate VO → §3 | — | — | child | — | The roles this group confers on its members |

Child `GroupRole` (`internal/domain/aggregatevos/`):

| Field | Go type | VO? | Nullable | Unique | `example:` | Description |
|---|---|---|---|---|---|---|
| `RoleID` | `domain.ID` | plain (an id) | no | yes, within the group | `7c2e9b41-5a83-4f16-9d02-8b6f31c0ae57` | The role this entry confers — the id and not the key, so a retired-and-recreated role needs an explicit re-attach |

**One field, and that is the whole child.** Inherited verbatim from `../role/spec.md` §2,
and for the same reason one level up: `Role`'s archive is **one-way**, so a retired role
comes back as a new row with a new id. An entry storing the role's *key* would silently
re-attach to the recreated row and break that promise; one storing the **id** cannot. Three
consequences, all accepted:

- **The read returns UUIDs, not role names.** `GET /groups/:id` answers
  `"roles": [{ "id": "…", "roleID": "7c2e9b41-…" }]`. A client that wants to display *what*
  the group confers calls `GET /roles` and joins by id. The server cannot: with no Mongo
  there is no read-time join (`relational-view.html` — `Embed`/`Link`/`ComposedView` all
  unavailable). This stops being the client's problem the day the service gains Mongo, with
  no change to this model.
- **`role_id` is a real foreign key**, so referential integrity for the existence half of
  G6 is free.
- **The escalation check costs two hops, not one.** Role id → the role's active
  permission ids → their keys → the claim. One probe answers all of G6, G10a and G10b (see
  §7), so it is one query, not three.

Notes on the decisions above:

- **`domain.ID` is the right type at this pin** (`table-schema.html` supported column
  shapes; `Role.TenantID` and `RolePermission.PermissionID` already prove it in this
  project). Wire DTOs stay `string` and convert at the mappers.
- **`vos.GroupKey` is a NEW raw VO, not a reuse of `vos.RoleKey`.** The *rule* is
  identical — 2–64 runes, one lowercase slug, plus the shared `text_predicates` anti-junk
  pair — but a `Group.Key` field typed `vos.RoleKey` reads as a bug in every file it
  appears in, and a VO's notification (`InvalidRoleKeyNotification`) would tell a caller
  their **role** key is malformed while they were creating a group. `GroupKey` is ~10 lines
  over the same shared predicates. **Advisory, not baked:** `RoleKey` and `GroupKey` are
  then the same rule written twice, and a later `vos.Handle` with a caller-supplied field
  name would unify them — that is an `/omnicore:evolve-entity` change on `Role`, out of
  scope here. Recorded in §C-6.
- **Uniqueness enforcement style: domain pre-check + DB backstop** — the project's
  established style (`TenantService.WorkspaceTaken`, `PermissionService.PermissionKeyTaken`,
  `RoleService.RoleKeyTaken`), so the duplicate reports **together with** the other
  validation errors, with the partial unique index as the race backstop bound in the
  repository's `Constraints` map to a 409.

## 3. Children (1:N)

| Child | Of whom | Edit strategy | Restorable alone? |
|---|---|---|---|
| `group_roles` | the flat root (`groups`) | **B — targeted per-child ops** (proposed; alternatives: A replace-all, C own aggregate) | **no** — so B is legal |

- **Why B and not A.** A replace-all `PUT` means an omitted role is revoked from every
  member of the group at once. `Role` rejected A for making every partial client a silent
  mass-revoker; the same mistake here is strictly worse, because one omitted entry
  de-authorizes a whole team rather than one role.
- **A PAIR, not the usual trio.** Strategy B's canonical ops are ADD / UPDATE / ARCHIVE. A
  `GroupRole` has **no editable field** — its single column *is* its identity — so
  "update this entry" would turn entry A into entry B while keeping A's row id, which an
  audit trail reads as one grant *becoming* another instead of as two events. Two child
  ops only:
  - **ATTACH** `POST /groups/:id/roles` — body carries `roleID`; the server mints the child id.
  - **DETACH** `PATCH /groups/:id/roles/:childId/archive` — soft removal. **Never `DELETE`**:
    the row lingers with a `deleted_at` stamp, and a `DELETE` that soft-removes is a lying
    contract.

  Both are commands **on the root** (load root → a domain method mutates the one child →
  the framework persists the diff) and both dispatch `ModeUpdate`, so `IfInsertOrUpdate` in
  §7 covers them.
- **The by-id guard lives in a domain method on `Group`**, not in a loop inside the command
  mapper: absent child → the canonical `RecordNotFoundNotification` (404), never the
  framework's `EntityDoesNotExistNotification` (422).
- **`IsSameBusinessIdentity` is over `RoleID`** — written explicitly rather than leaning on
  `domain.IsSameByBusinessFields`, so it states which column carries identity and survives a
  second field being added later. It is what the framework's ATTACH path reuses as the
  duplicate guard (G8).
- **No per-child unarchive**, by framework construction. Re-attaching a detached role is a
  fresh ATTACH with a fresh child id — which reads correctly in the audit trail.
- **⚠️ The root-archive auto handler is instantiated exactly ONCE per surface.** Wiring it
  to the child DETACH route type-checks, boots, answers 200 — and archives the entire
  group. The canonical trap of this model; repeated in the web task.

## 4. Siblings (1:1)

`N/A — no facet worth splitting.` The model has **no optional field at all**: `tenant_id`,
`name` and `description` are each required, and `key` is required too. With nothing
nullable there is no sparse, bulky or PII facet to move off the root, so the sibling
question has no candidate group to be asked about. (Recorded rather than omitted: this is
the decision, not a skipped step. If §C-1/§C-2 are ever taken — `externalId` and `source`
— they are two scalars and stay nullable columns on the root, not a satellite.)

## 5. Modes                                             [required]

`Display, Insert, Update, Archive` — **one-way archive, no `Unarchive`** (proposed;
alternative: add `Unarchive`, following `Tenant` rather than `Permission`/`Role`).

The argument `Role` made transfers one level up, and gets *stronger*: a group is granted to
**users**, and those memberships point at the group's id, which does not change. A single
`PATCH /groups/:id/unarchive` would therefore silently re-authorize **every member of the
team at once**, with no re-approval and an audit line reading "restored". A retired group
comes back as a new row whose members must be re-added.

Named honestly, because it is the cost, and it is bigger here than it was on `Role`:
**Entra soft-deletes a group with a 30-day restore window** precisely because losing a
50-member group to a fat-finger is brutal. This model's answer to that fat-finger is
"create it again and re-add the members" — and since `User ↔ Group` membership does not
exist yet, nobody has been hurt by it yet either. If the operator-error case should win,
the answer is `Unarchive` plus a rule refusing it when the handle has since been retaken
— one mode and one rule, and it can be added later without rework.

## 6. Delete semantics                                  [required]

**Soft only — archive, no hard delete.** `PATCH /groups/:id/archive`; there is no `DELETE`,
on the root or on the child. Consistent with all three existing entities and with §A-9:
purging a group destroys the only human-readable record of what a past membership meant,
which is exactly what an access review reads. Per-child detach is
`PATCH /groups/:id/roles/:childId/archive` — same verb-truth rule.

## 7. Business rules                        [required]

| # | Field(s) | Rule | Verb scope | Notification | HTTP |
|---|---|---|---|---|---|
| G1 | `Key` | Immutable after creation — it is what API callers, audit lines and directory mappings reference | `IfUpdate` | `GroupKeyIsImmutableNotification` | 422 |
| G2 | `TenantID` | Immutable after creation — a group never moves between tenants | `IfUpdate` | `GroupTenantIsImmutableNotification` | 422 |
| G3 | `Key` + `TenantID` | Unique **per tenant**, over active rows. Service pre-check with exclude-self + partial unique index as backstop | `IfInsertOrUpdate` | `GroupKeyAlreadyExistsNotification` | 409 |
| G4 | `TenantID` | The owner tenant must exist and not be archived | `IfInsert` | `GroupTenantDoesNotExistNotification` | 422 |
| G5 | `TenantID` | **Tenant isolation.** The row's tenant must equal the caller's `tenant_id` claim, unless the caller is a `*:*` superadmin | `IfInsertOrUpdate` + `IfArchive` | `domain.TenantMismatchNotification` (framework-owned, already translated) | 403 |
| G6 | `Roles[].RoleID` | Every attached role must exist, be **active**, and belong to **this group's tenant** — one rule, one message, see below | `IfInsertOrUpdate`, over the entries this write ADDS | `RoleNotAvailableInTenantNotification` | 422 |
| G8 | `Roles[]` | No duplicate role within one group — `IsSameBusinessIdentity` on the ATTACH path, plus the explicit guard on the by-id path, plus the partial unique index as backstop | `IfInsertOrUpdate` | `GroupAlreadyGrantsRoleNotification` | 409 |
| G9 | `Roles[]` | At most **50** roles in one group (proposed) | `IfInsertOrUpdate` | `TooManyRolesInGroupNotification` | 422 |
| G10a | `Roles[]` | **No privilege escalation** — a caller may attach a role only if they hold **every** permission it grants. A `*:*` superadmin passes by construction | `IfInsertOrUpdate`, over the entries this write ADDS | `CannotGrantRoleWithUnheldPermissionsNotification` | 403 |
| G10b | `Roles[]` | **No wildcard-bearing role may be attached through the API** — a role granting a permission with `*` in either part is refused on every group | `IfInsertOrUpdate`, over the entries this write ADDS | `CannotGrantWildcardRoleNotification` | 403 |
| — | `Key`, `Name`, `Description` | format, length, substance, anti-junk | — | **not declared here** — `vos.GroupKey`, `vos.DisplayName` and `vos.Description` validate by type on every write. Declaring `required` beside a VO makes the caller read the same complaint twice | 422 |

*(There is no G7: the numbering is kept aligned with `../role/spec.md`'s R-series so the
two specs can be read side by side, and `Role`'s R7 duplicate rule is G8 here.)*

### G6 — three questions, one notification, on purpose

An attached role must be **(a)** in the catalog, **(b)** active, and **(c)** owned by the
group's tenant. (c) is the **first thing in this entity that `Role` did not need**:
`Permission` is a global catalog, so "does this permission belong to me?" was not a
question. `Role` is tenant-scoped, so a group in tenant A attaching tenant B's role would
confer another customer's permissions on A's members — a cross-tenant leak through a route
that looks like ordinary group editing.

All three collapse into **one** notification — *"This role is not available in your tenant,
or is no longer active."* — deliberately, because separating them leaks. A distinct "that
role belongs to another tenant" message confirms to a caller in tenant A that a specific
UUID is a live role in some other tenant, which is an existence oracle over a competitor's
org structure. Same reasoning that makes the by-id read answer **404 rather than 403**
across tenants (§10).

The database FK covers (a) only. (b) and (c) are the domain's, and are what makes this rule
earn its keep beyond turning a constraint error into a readable 422.

### G10a/G10b — the escalation pair, and why it is a pair

Identical in structure to `../role/spec.md` R9a/R9b, and for the identical reason, verified
in the source at this pin: `HasPermission` panics on any argument containing `*`. Order
matters and is not an implementation detail —

- **G10b runs first and removes the panic input.** Any role granting a wildcard permission
  is refused for everyone, on every group, so no wildcard string ever reaches
  `HasPermission`. It must answer **true for an unresolvable role id too**, rather than let
  an unknown id fall through to the escalation question.
- **G10a then asks the question safely**, because every remaining key is concrete. The
  superadmin exemption is free: `HasPermission` returns `true` for *any* concrete permission
  when the claim set contains `*:*`.

**What G10b costs, stated plainly.** The platform's own superadmin group — the one carrying
the `*:*` role — cannot be created through this API. Consistent with where the README
already puts both the reserved platform tenant and the `*:*` role: seeded by migration.
The superadmin group is seeded in that same migration when that work happens.

### What G6 / G10a / G10b judge — added entries, not the stored ones

Inherited from `../role/spec.md`'s 2026-08-21 amendment, which was decided *after* seeing
what the whole-collection reading costs. All three ask about the **act of attaching**, and
an entry already in the row was asked all three when it entered. Re-judging the stored ones
makes unrelated writes hostages of the past:

- a role the tenant archives **after** it was attached would make the group impossible to
  rename — 422 on a request whose only change is a label;
- a caller who has since lost a permission could no longer even **detach** the other roles,
  since a detach is an update and every remaining entry would be re-judged.

Mechanically `domain.GetAddedItemsOf`, not `GetCurrentItemsOf`. An **insert is unchanged**
— every entry of a new group is an added one — and a **detach asks nothing**, because it
adds nothing. Cost, named: an attachment that was legitimate when made is never re-vetted;
detaching is the tool for one that stopped being acceptable, and it stays reachable
*because* of this rule shape.

### G5 — how the identity reaches the rule

Layer 2 in `authz-seams.html` terms. The identity is translated into the entity by the
**command mapper** — the only layer allowed to read `ctx` — onto runtime-only fields **not
declared in the `TableSchema`**, so the persister never persists or scans them:

- `RequestingTenantID` ← `ctx.Identity().TenantID()`
- `RequestingPrincipalIsSuperAdmin` ← **`ctx.Identity().IsSuperAdmin()`** — never
  `HasPermission("*:*")` (panics), never a hand-read of `Claims["permissions"]` (the claim
  NAME is configurable via `authorization.permissionsClaim`, so a hardcoded read starts
  answering `false` the day an operator renames it).

### Absent identity vs absent claim — two states, not one

Inherited verbatim from `../role/spec.md`; restated because it is a security contract, not
a detail:

| State | When | Verdict |
|---|---|---|
| **No `Identity` at all** (`ctx.Identity()` is nil) | `auth.mode: disabled` — the middleware never populates one | **stand down** — G5 and G10a do not fire |
| **`Identity` present, claim empty or insufficient** | any authenticated request | **refuse** — 403, fail closed |

Collapsing them makes the entity unusable in the dev profile. The safety of the first row
is the framework's boot guard, not this entity's promise: `AuthModeDisabled` is accepted
only under `APP_PROFILE=dev` and `LoadConfig` rejects it elsewhere, so a nil identity
cannot occur in production — a prd deployment that tried it aborts at boot.

### The domain service port

```
GroupService (internal/domain/group_service.go) — plain values, no error, per the local pattern
  GroupKeyTaken(tenantID domain.ID, key string, selfID domain.ID) bool   // G3
  TenantIsUnavailable(tenantID domain.ID) bool                           // G4
  RoleIsUnavailableInTenant(tenantID, roleID domain.ID) bool             // G6
  RoleGrantsWildcard(roleID domain.ID) bool                              // G10b
  CallerLacksAnyPermissionOf(roleID domain.ID) bool                      // G10a
  CallerIsSuperAdmin() bool                                              // G5, G10a
```

Every fact is **named for the problem, never for the healthy state** — the generated suite
stubs the service so each probe answers "nothing found", which is what lets a valid fixture
through. A fact spelled `RoleIsAvailable` would read `false` under that stub and mean "the
role is gone", turning a correct spec red on the day it is written. (The convention `Role`
established; `RoleKeyTaken` and `TenantIsUnavailable` there are the precedent.)

**Cross-aggregate reach:** `TenantIsUnavailable` queries `tenants`; the three role facts
query `roles` and, transitively, `role_permissions` + `permissions`. The implementation
holds those repositories beside its own — confirm the shape against `service-to-service.html`
before writing it. `CallerLacksAnyPermissionOf` and `RoleGrantsWildcard` share one
resolution (role → its active permission keys), so a write that attaches N roles pays N
lookups, not 2N. `CallerLacksAnyPermissionOf` **guards the wildcard itself** and returns
`false` rather than calling through — defence in depth behind G10b, because a panic on a
security rule is a 500.

`CallerHolds`-style access to the token works because `ScopedService(ctx)` already binds
the request to the service in this project (`internal/infra/permission_service.go`,
`internal/infra/role_service.go`), so the impl reads `s.ctx.Identity()` without the domain
ever seeing a context.

## 8. Update shape                                      [required]

**PATCH only.** No sibling in §4, so the PUT-invariant does not bind. Mirrors
`patch_tenant`, `patch_permission` and `patch_role` — the whole service speaks PATCH.

Only `Name` and `Description` are actually patchable: `Key` and `TenantID` are
frozen by G1/G2, and `Roles` moves through the §3 child ops rather than through the root
body.

## 9. Surfaces & reads                       [required]

- **REST: yes** (OpenAPI documented) · **GraphQL: yes** — both, mirroring all three existing
  entities. GraphQL carries the **root verbs only** (`groups`, `group`, `createGroup`,
  `patchGroup`, `archiveGroup`); the two collection verbs are REST-only, exactly as `Role`
  ships them.
- **gRPC: no** — available later through `/omnicore:implement`, no rework.
- **Exports (CSV/XLSX): no** (proposed) — operator-facing and small, the call all three
  existing entities made. An access-review export is the plausible reason to want one; it is
  one flag.
- **Integration events: no** — the posture has no broker, so publishing is unavailable.
  Recorded so it is not silently forgotten.
- **Reads:** by-id + by-params.
- **Reserved read controls served:** pagination (`first`/`last`/`after`/`before`) ·
  `orderBy` · `?fields=` **yes** · `?onlyTotal` **yes** · `?includeArchived` **yes** ·
  **`?search=` no** — a relational-served view answers free text with a typed 400, so
  declaring it would advertise a capability the server refuses.
- **Computed read fields: none.** The obvious candidate — rendering each attached role's key
  or name on the entry — is impossible for the same reason it was on `Role`: the child
  stores only the id, and a computed field derives from *stored* fields of the same
  document. The rendering is the client's job (§2).
- **Field-level read authz (`ReadCriteria.Restrict`): none.** Every field a caller may see
  the row at all for, they may see entirely. Row-level isolation does the work.
- **View backing: relational** (`.RelationalSource(repo.Loader)`) — the project posture, and
  the only option without Mongo. Confirmed serviceable at this pin: a relational view **does**
  carry the 1:N children in its document (`relational-view.html`, read).
- **The one real cost, stated up front:** a caller **cannot filter or sort by an attached
  role**. `?filter[roles.roleID][eq]=…` is a typed 400. **"Which groups confer role X?" is
  not answerable from this listing on this posture** — and it is a question an access review
  asks more often than `Role`'s equivalent, because it is how you find out who a role
  actually reaches. It becomes answerable the day the service gains Mongo
  (`/omnicore:configure`), with no change to this model. Recorded again in §C-7.

- **Filter/sort operators per field** (low-risk — decided):

| Field | `filter:` | `sort:` |
|---|---|---|
| `tenantID` | `eq,in` | `asc,desc` |
| `key` | `eq,ne,in,startswith,istartswith,contains,icontains` | `asc,desc` |
| `name` | `eq,ne,in,startswith,istartswith,contains,icontains` | `asc,desc` |
| `description` | `contains,icontains` | — |

`description` is deliberately not sortable: ordering a listing by a 500-char free-text
column is a blocking sort nobody asks for on purpose. `name` carries `ne` anyway — it costs
nothing and "everything except the ops group" is a listing an operator does ask for.

## 10. Authorization                          [required]

### Layer 1 — the permission gate

`group:insert` · `group:update` · `group:archive` · `group:read` · **`group:grant`** — the
taxonomy the service already grants across its 34 existing gate points (REST + GraphQL,
three resources × four verbs), plus **one new verb** that Q2-B introduces deliberately. The
action spells the operation; `:update` covers PATCH; `:archive` would cover unarchive too
if §5 gains it.

| Operation | Route | Permission |
|---|---|---|
| create | `POST /groups` | `group:insert` |
| patch | `PATCH /groups/:id` | `group:update` |
| archive | `PATCH /groups/:id/archive` | `group:archive` |
| list | `GET /groups` | `group:read` |
| by id | `GET /groups/:id` | `group:read` |
| attach a role | `POST /groups/:id/roles` | **`group:grant`** |
| detach a role | `PATCH /groups/:id/roles/:childId/archive` | **`group:grant`** |

**`group:grant` is the one place this entity's taxonomy diverges from `Role`'s, and the
divergence is the decision, not an oversight.** `Role` weighed a `role:grant` verb and
declined it to keep four verbs per resource; `Group` takes it, because §A-6 is the reason —
Entra and AWS both split "manage the group" from "change what the group confers", and the
group→role edge reaches further than the role→permission one (a group hands a member every
permission of every role it carries). Consequences worth stating:

- **`group:grant` implies nothing and is implied by nothing.** A principal with
  `group:update` may rename and re-describe a group and gets 403 on both collection verbs.
  A principal with only `group:grant` may attach and detach roles on a group they cannot
  rename — deliberate: the two are different jobs.
- **It does not replace G10a.** Layer 1 asks "may this principal touch this edge at all";
  G10a asks "may this principal confer *this* role". Holding `group:grant` and attempting to
  attach a role carrying a permission the caller lacks is still a 403 from the domain. That
  is the whole point of taking A and B together rather than either alone.
- **A fifth verb has a deployment cost**, named honestly: `group:grant` is a catalog row
  somebody has to insert and a grant somebody has to make, and until they do, nobody can
  attach a role to a group once `auth.authorization` is switched on. That is the correct
  failure direction for an escalation surface, but it is a step that must not be forgotten
  when the reserved-platform-tenant seeding work happens.

### Layer 2/3 — data access

**Every row is tenant-scoped, on READS and on WRITES both** — the answer `Role` already
carries, applied unchanged.

- **Writes (Layer 2):** G5. The row's `tenant_id` must equal the caller's claim, else 403
  `TenantMismatchNotification`. A `*:*` superadmin bypasses it and may act on any tenant's
  groups. On insert the same rule applies — a caller may only create inside their own
  tenant.
- **Reads (Layer 3):** `ToCriteria(ctx)` injects
  `crit.Filter["tenant_id"] = ctx.Identity().TenantID()`, so a listing returns only the
  caller's own groups, and a by-id read of another tenant's group returns **404 rather than
  403** — it does not exist for this caller, which leaks nothing about who else exists. A
  `*:*` holder skips the filter, which is what lets a platform operator support a customer
  — detected with `ctx.Identity().IsSuperAdmin()`.
- **The cross-tenant role attach (G6) is the same policy at the child level**, and the
  single-notification decision above is what keeps it from becoming a read oracle.
- The rejected alternative, recorded: writes-only isolation, with every authenticated caller
  able to *read* every tenant's groups. It leaks each customer's org chart to every other
  customer — worse here than on `Role`, since a group listing **is** the org chart.

### The service-wide switch this entity depends on

**Neither profile configures `auth.authorization` today.** `microservice.dev.yaml` is
`auth.mode: disabled`; `microservice.prd.yaml` is `mode: jwt` with no `authorization:`
block, which defaults to **off** — so `RequirePermission` currently no-ops across the whole
service and `Identity.TenantID()` is never required to be present. Everything in this
section is generated and correct, and **inert until that switch is turned on**; the rules
fail closed the moment it is. Flipping it is a service-wide posture change owned by
`/omnicore:configure`, unchanged by this entity.

---

## §C — Improvement suggestions (offered, not baked)

Requested explicitly at the invocation ("faça sugestões se tiver alguma de melhora").
**None of these is in the model above** — each is a separate yes/no, and each is additive
(no rework if taken later). Ordered by how much they would change if adopted *now* versus
later.

1. **`externalId` — the SCIM/directory sync handle.** RFC 7643 defines `externalId` on
   `Group` precisely so a provisioning client can find the resource it created; Okta
   reserves the name on group profiles for the same reason. **Take it now if IdP-driven
   group provisioning is on the roadmap at all** — it is one nullable `VARCHAR(255)` column
   plus a unique-per-tenant index, and adding it later to a table with rows is a migration
   with a nullable default (cheap, but a migration). Cost of taking it: a nullable field,
   which is the model's first, and §4's "no facet worth splitting" line gets one caveat.
2. **`source` — `manual` | `directory`, an enum VO.** Okta types groups by source
   (`OKTA_GROUP` / `APP_GROUP` / `BUILT_IN`); Entra distinguishes cloud from synced. What it
   buys is a rule: **a directory-sourced group's membership and name are not editable
   through this API**, so a hand-edit cannot silently diverge from the IdP that will
   overwrite it on the next sync. Only worth it together with (1), and only once something
   actually syncs. **Recommendation: skip for now**, revisit with (1).
3. **Nested groups — recommended NO, and recorded as a decision rather than an omission.**
   Keycloak nests (subgroups inherit the parent's role mappings) and Entra nests, but AWS
   IAM refuses outright and Okta refuses outright — and Entra, the one platform that both
   nests *and* makes group→role a security boundary, **forbids nesting on role-assignable
   groups**. That is the tell: nesting turns "what can this user do?" into a graph walk
   with cycle detection, on the exact path the token issuer has to run on every login. The
   README's target graph is flat and the union-of-two-paths model already gives the
   composition. If a customer needs "Engineering ⊃ Platform", two groups and two
   memberships express it with no graph.
4. **A default / "everyone" group.** Keycloak default groups auto-apply to new users; Okta
   ships Everyone. **Deferred, not rejected** — it is a rule about *membership*, and
   `User ↔ Group` does not exist yet. It belongs in that aggregate's spec, as a flag on this
   one (`is_default`, at most one active per tenant) plus a hook on user creation. Noting it
   here so it is not rediscovered from scratch.
5. **~~Both Q2-A and Q2-B~~ — TAKEN at the model gate**, and folded into §7 and §10 above.
   Kept here for the reasoning. They are not exclusive and the combination is what Entra
   effectively ships: the domain guarantees no escalation (A), *and* the deployment can
   delegate "may change what a group confers" separately from "may rename a group" (B).
   If both are wanted, say so at the gate — it is one extra permission literal and no extra
   rule.
6. **Unify `vos.RoleKey` and `vos.GroupKey` into one `vos.Handle`.** After this entity they
   are the same rule written twice (2–64 runes, one lowercase slug, the shared anti-junk
   predicates), differing only in the notification they raise. A `vos.Handle` taking the
   field name would collapse both. **Out of scope here** — it changes `Role`, which is
   `/omnicore:evolve-entity`'s job and needs its own approval. Flagged so the duplication is
   a recorded choice, not an accident.
7. **The reverse question this posture cannot answer.** "Which groups confer role X?" and
   "which roles grant permission Y?" are both typed 400s on a relational-served view (§9).
   Together they are most of what an access review does. The fix is not per-entity — it is
   `/omnicore:configure` adding Mongo, after which a `ComposedView` answers both with no
   change to either model. Recorded here because this entity is where the gap starts to
   hurt: with `Role` alone a client could still join two small listings; with `Group` in
   between, the client is joining three.

## §D — Advisory findings (this repo, not this entity)

Neither is a blocker and neither is fixed by this run without approval:

- **`README.md` line 21 says the service is built on omnicore `v0.54.0`; `go.mod` pins
  `v0.56.1`.** The prose below it already describes v0.55.0 and v0.56.0 behavior correctly,
  so it is the badge line alone that is stale. The docs task of this run touches the README
  anyway (`Group` is listed as *not started* at line 36 and has to move to *done*) — say the
  word and that line is corrected in the same pass.
- **No misuse of the framework was found in the existing entities.** `Role`'s tree was read
  closely enough to mirror it, and its use of `domain.ID`, the runtime-only identity fields,
  the `Constraints` bindings and the child-op shape all match the pinned docs. Recorded
  because "found nothing" is a result, and the skill asks for the check either way.
