# Spec: Claim

- **Status:** APPROVED
- **Approved:** maintainer (Cláudio Schirmer Guedes), 2026-08-28 — both ⚠️ OPEN slots answered
  at the model gate: **the reserved prefix is `x_`** (OPEN-1, option (a)) and **the prefix is
  CALLER-owned** (OPEN-2, option (B): one string on the wire, in the column and in the token;
  a name without the prefix is refused, nothing is prepended and nothing is stripped). Every
  `(proposed)` pick stands as approved
- **Pin:** omnicore **`v0.62.0`** (already the latest published — the 0v check found no update) ·
  dialect **postgres**, single · Postgres SoR, **no Mongo, no broker** → relational-served views ·
  plugin `omnicore` 0.48.0 (current)
- **Language:** English (all artifacts) · Portuguese (chat) — per `../../../CLAUDE.md` rule 3
- **Generation:** **omnicore-gen** — chosen by the maintainer at gate 1d, 2026-08-28
- **Amended:** 2026-08-28, after the build, on the maintainer's explicit approval — **§10:
  `TenantID` moved from an ordinary wire field to `assignedFrom: identity-claim` +
  `bypassMaySet: true`**, and §8's `patchExcludes` dropped it as a consequence. The model
  gate had approved "on the wire, as on `Role`"; that reading was inherited from `Role`'s
  own comment, which rejected `assignedFrom` for two reasons — the operator losing
  cross-tenant creation, and a token-less dev bench with nothing to fill the field. The
  first is exactly what `bypassMaySet` answers, and the second expired on 2026-08-26 when
  `auth.mode` became `jwt` in both profiles. **The same amendment was applied to `Role` and
  `Group` in the same pass**; `User` and `Client` already had it. Nothing else in the model
  changed, and no migration was needed — the column does not move

The **tenant-owned catalog of claim definitions**: the vocabulary of extra facts a token may
carry that are neither permissions nor platform identity (`cost_center`, `region`,
`plan_tier`, `erp_id`). One row is one claim NAME, and it holds the type its values must
have, which identity kinds may hold one, and the DEFAULT value — level 2 of the chain the
backlog draws.

## Sources read before writing this spec

| Source | What it settled |
|---|---|
| `../../../backlog.md` § *Custom claims via a `Claim` catalog* | The whole draft shape: six fields, two levels, per-tenant uniqueness, why the registry is called `Claim` and not `Attribute`, and the six questions that had to be answered before it could become a spec |
| `../../../ACCESS_MATRIX.md` § Role · § Group | The standard tenant-scoped registration: `guard foreign-tenant` on writes, `filter TenantID` on reads, `*:*` crosses, no unarchive mounted, `<entity>:<verb>` taxonomy |
| `../../../README.md` line 48 | **Reserved platform tenant: not started** — and two entities already depend on it. §0 below states how this entity avoids becoming the third |
| `../role/spec.md` (APPROVED) · `specs/omnicore-gen/role.omnicore.yaml` | The local flavor for a tenant-owned catalog: shared VOs, service pre-check + DB backstop uniqueness, the `Tenant` read join, relational `read.backing`, `noIdentity: stand-down` |
| `specs/omnicore-gen/permission.omnicore.yaml` line 256-266 | `patchExcludes` beside an immutability rule — the two are not redundant, and leaving only the rule documents a field as editable that the domain then refuses. §8 applies it |
| `internal/domain/vos/` (inventory) | What exists to reuse: `Description`, `DisplayName`, `PermissionKey`, `RoleKey`, `GroupKey`, `TenantWorkspace`, `TenantStatus`, `ClientStatus`, `UserStatus`, `Email`, `PersonName`, `Password`, `CidrBlock`, and the shared `text_predicates` helpers |
| `internal/application/commands/authentication_commands_manual.go:65-92, 572` | The nine names the platform mints, and `buildClaims` — the merge point this catalog eventually feeds |
| omnicore `v0.62.0` `/docs/relational-view.html` | A relational read model answers `?search=` with a typed **400** `UnsupportedCapabilityNotification`, never a 500 — so §9 does not declare it |
| `omnicore-gen explain rules` (0.48.0) | The rule kinds available: `required · immutable · length · range · comparison · transition · requiredIf · groupCap · factRange · childDuplicate · ownerCheck · valueObject`; anything else is a NAMED `rules.manual` entry |

---

## §0 — Scope, and the two dependencies it deliberately does not inherit

**Scope: the `claims` catalog ONLY** *(proposed; alternative: also build the two edge
collections in the same run)*. The invocation named exactly the six catalog fields, and the
two owned collections the backlog draws — `user_claims` on `User`, `client_claims` on
`Client` — are **children of aggregates that already exist**. Adding a collection to a living
aggregate is `/omnicore:evolve-entity`'s job, not this skill's, and it is two separate runs
(one per parent) rather than one. They are the natural next step; they are not this step.

**Dependency 1 — the reserved platform tenant (`README` line 48, not started).** The backlog
says this entry "inherits `Role`'s and `Group`'s dependency" on it. **This spec breaks that
inheritance**, and the mechanism is the one already used for the platform's own `*:*` role:
whatever refuses a reserved name applies to **every definition created through the API**,
full stop, with no exception carved for a tenant. The platform's own nine then enter by
**migration**, the same door the `*:*` role and the super-admin group enter by, and never
meet the rule at all. Nothing in this entity needs to know which tenant is reserved.

Consequence, stated rather than hidden: **the seed of the platform's nine is NOT part of this
run** *(proposed)*. It has nowhere to land until the reserved tenant exists, and seeding it
into a customer's tenant would be worse than not seeding it. It stays a backlog line that
this entity makes possible rather than a migration this run writes.

**Dependency 2 — emission.** Wiring the resolved map into `buildClaims`
(`authentication_commands_manual.go:572`) needs the edges to exist, so it is downstream of
both. **Nothing in this run touches token issuance.** The catalog can be filled, read and
audited with no observable change to any token — which is the right first step, not a
half-feature.

**Answered elsewhere, and not re-asked here.** Three of the backlog's six open questions
belong to the edges, not to the catalog, and this run neither answers nor blocks on them:
removal semantics on the edge, `auth.auditClaims`, and whether setting a VALUE rides
`user:grant` or a new `*:set-claim`. The claim-size budget is noted in §2 (it sizes
`DefaultValue`), and the third-level question is recorded in §11.

---

## 1. Storage model                                    [high-risk — confirm]

- **Kind: flat** *(proposed; alternative: sharedbase-role — rejected)*.
  **No identity smell.** A claim definition carries no person, no document, no e-mail, and no
  natural registry key of an asset. It is a vocabulary entry owned by a tenant — the same
  shape `Role` and `Permission` already have, and neither is a party playing a role. There is
  no second role a `cost_center` definition could also become.

- **ER sketch**

```
tenants                                claims
─────────                              ──────
id            UUID PK   ┌───────────── id             UUID PK
workspace     …         │              tenant_id      UUID FK → tenants.id   NOT NULL
status        …         └───────────── name           VARCHAR(64)            NOT NULL
                                       value_type     VARCHAR(16)            NOT NULL
                                       applies_to     VARCHAR(16)            NOT NULL
                                       default_value  VARCHAR(256)           NULL
                                       description    VARCHAR(500)           NOT NULL
                                       revision       …
                                       created_at / updated_at / deleted_at
                                       UNIQUE (tenant_id, name) WHERE deleted_at IS NULL
```

  - **`claims`** — *The vocabulary of tenant-defined claims: one row per claim name a token
    of this tenant may carry, holding the type its values must have, which identity kinds may
    hold one, and the default used when no principal-level value is set.* (→ table COMMENT)

- **The physical column is `name`** — *corrected 2026-08-28, before generation.* This spec
  originally called for a renamed `claim_name` on the assumption that `name` was a reserved
  word across the engine set. **It is not, and the project disproves it**: `roles.name` and
  `groups.name` are ordinary quoted columns in the existing migrations. The rename that IS
  real is `Role`'s `key` → `role_key` and `Permission`'s `resource` → `resource_name`, and
  neither generalises to this one. Using `name` keeps this entity's DDL consistent with every
  other display-name column in the service; there is no exposed-vs-stored split to carry.

- If sharedbase-role: **N/A — flat.**

## 2. Fields

| Field | Go type | VO? | Nullable | Unique | Lives on | `example:` | Description |
|---|---|---|---|---|---|---|---|
| `TenantID` | `domain.ID` | reuse (`domain.ID` self-validates) | no | no | root | `0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410` | The tenant that owns this definition. Immutable — a claim never moves between tenants |
| `Name` | `vos.ClaimName` | **new-raw** | no | **yes** — per tenant, active-only | root | `x_cost_center` | The exact name minted into the token. Immutable, and what every consuming service branches on |
| `ValueType` | `vos.ClaimValueType` | **new-enum** (`string` · `number` · `bool`) | no | no | root | `string` | What a value for this claim must parse as — the type the default and every principal-level value are validated against |
| `AppliesTo` | `vos.ClaimAppliesTo` | **new-enum** (`user` · `client` · `both`) | no | no | root | `both` | Which identity kinds may hold a value for this claim |
| `DefaultValue` | `*string` | **plain** (see below) | **yes** | no | root | `1000` | Level 2 of the chain: the value every principal of this tenant gets when none is set on the principal itself. Null means the claim is simply absent from the token |
| `Description` | `vos.Description` | reuse | no | no | root | `Internal cost center this account is billed against, as the ERP knows it.` | What the value means, for the operator filling it in |

Managed columns as everywhere else: `revision`, `created_at`, `updated_at`, `deleted_at`.

**`DefaultValue` is `plain`, and that is a decision rather than an omission.** Its rule is
*"parse as whatever `ValueType` says"* — a rule that reads ANOTHER field, which a value object
cannot see. A composite `TypedValue{Type, Value}` was weighed and rejected: `ValueType` also
governs the values on the two edge collections, in other aggregates, so it is a type
declaration the whole chain reads and not half of a private pair — and the two halves have
different nullability, which a composite handles badly. It stays two fields plus the named
manual rule R7. Its length (256) is the **claim-size budget** made concrete: whatever this
adds rides in a header on every request, on top of a budget the shipped token already argued
down to keys rather than display names.

**`Name` uniqueness is per tenant and ACTIVE-only** *(proposed; alternative: over all rows)*.
Per tenant because a token carries exactly one tenant, so two customers both naming a claim
`x_region` is harmless. Active-only because §5 mounts no unarchive: re-inserting is the only
way a retired definition comes back — the same shape `Role.Key` carries.
**Enforcement: service pre-check + DB constraint backstop** *(recommended; alternative:
constraint-only)* — the duplicate then reports together with the other validation errors
instead of arriving alone as a 409 after everything else passes.

**`vos.ClaimName` — the new raw VO, and where the reserved prefix lives.** Shape: lowercase
`[a-z][a-z0-9_]*`, 2..64 runes, snake_case (the convention every claim in `buildClaims`
already follows), plus this project's shared anti-junk predicates, plus **the reserved
prefix**. It goes in the VO rather than in a rule for the reason §0 gives: the rule is about
the string alone and applies to every write through the domain, while the platform's own rows
arrive by migration and never reach it. Precedent: `TenantWorkspace` carries its reserved
list the same way. Nothing is normalized — `X_Cost_Center` is refused, never quietly
repaired, because the value is immutable and a caller who believes they registered one string
has no second chance.

**The reserved prefix is `x_` — DECIDED at the model gate (2026-08-28).** Every claim name
created through the API carries it, with no exception carved for any tenant; the platform's
own nine arrive by migration and never meet the rule (§0). Two runes of a budget that matters,
and it reads as "extension" to anyone who has met an HTTP header.

Three alternatives were weighed and are recorded rather than dropped: **`ext_`** — more
explicit, two runes dearer; **a URI namespace**
(`https://authcore.example/claims/cost_center`), which is what Auth0 requires for
collision-resistance across issuers — genuinely the safest and by far the most expensive per
token, and this issuer's audience is its own mesh rather than the open internet; and **no
prefix plus an explicit reserved LIST of the nine platform names**, which works today with
zero dependency and mirrors `TenantWorkspace.IsReserved()`, but protects only against the
names that exist NOW — the tenth platform claim would collide with a definition a customer
already created, and there is no migration out of that. The prefix closes it by construction,
which is why it wins.

**The prefix is CALLER-OWNED — DECIDED at the model gate (2026-08-28, option (B)).** The
caller sends the full name, `x_` included; a name without the prefix is refused. **Nothing is
prepended and nothing is stripped** — one string on the wire, in the column, in the token and
in whatever a consumer greps for. This is the stance every value object in this project
already takes (`RoleKey`, `TenantWorkspace`: refused, never quietly repaired), and the reason
is the same: the value is immutable, so a caller who believes they registered one string has
no second chance to correct it. The cost is an operator typing two extra runes once per
definition.

The alternative — **service-owned**, where the wire carries the bare `cost_center` and the
domain prepends — was weighed and turned down on the backlog's own argument. It would make the
wire name and the token name differ, so a consumer that reads `x_cost_center` out of a JWT and
searches the catalog for it finds nothing. That is the AD FS two-name shape the backlog
rejected by name — "a `Key` and a `ClaimName` that are always 1:1 … which is what a postiche
internal name looks like" — reintroduced implicitly through a prefix instead of explicitly
through a second column.

**The double prefix is refused.** `x_x_cost_center` is well-formed snake_case that begins with
`x_`, so shape alone would let it through; `ClaimName.IsValid` rejects a remainder that itself
begins with the prefix. It is the paste error this decision makes possible, so it is closed
here rather than discovered in a token.

Three consequences that belong to the layers, named here so no layer rediscovers them:
the OpenAPI/GraphQL `example:` for `name` is a **prefixed** value (`x_cost_center`); the
uniqueness index and the `ClaimNameTaken` probe compare the **stored** string, prefix
included, with no normalization step anywhere; and the platform's nine — which carry no
prefix — remain un-writable through this API by construction, exactly as §0 intends.

## 3. Children (1:N)

**N/A — no collections on this aggregate.** The two the backlog draws (`user_claims`,
`client_claims`) are children of `User` and of `Client`, not of `Claim`; see §0.

## 4. Siblings (1:1)

**N/A — no facet worth splitting.** The model has exactly one optional field, `DefaultValue`,
and the trade-off is shown rather than buried: a single nullable `VARCHAR(256)` is neither
bulky, nor PII, nor rarely read — it is read on every load, because it IS level 2 of the
chain. A satellite would buy one join per read for nothing. It stays a nullable column on the
root *(proposed; alternative: a `claim_defaults` 1:1 satellite — rejected for the above)*.

## 5. Modes

`display, insert, update, archive` *(proposed; alternative: add `unarchive`)*.

- **No `delete`** — a claim name that ever reached a token has to stay auditable; the same
  call `Permission`, `Role`, `Group` and `User` all made.
- **No `unarchive`**, matching every entity in this service except `Tenant`. The deliberate
  consequence, stated out loud because it mirrors the README's Permission rule 3: a retired
  definition comes back as a **new row with a new id**, so an edge holding the old id does not
  silently re-attach to the recreated definition and must be re-set explicitly. That is the
  same property that made `role_permissions` store the id and not the string.

## 6. Delete semantics

**Soft only, root.** `PATCH /claims/{id}/archive`; no `DELETE` verb is mounted, on the root or
anywhere. Verb truth holds: nothing soft rides behind `DELETE`. No per-child ops — §3 is N/A.

## 7. Business rules

| # | Field(s) | Rule | Verb scope | Notification | HTTP |
|---|---|---|---|---|---|
| R1 | `TenantID` | `valueObject` + **`guard: true`** — pull the owner's own validation forward and make it the barrier. Everything after it depends on the owner being a usable id, most sharply R5, which is scoped BY it and hands it straight to a criterion. Replaces a `required` rule rather than joining one | insertOrUpdate | *(the value object's own)* | 422 |
| R2 | `Name` | `immutable` — the name IS what every token carries and every consuming service branches on. Editing it rewrites the meaning of every issued token retroactively and invisibly | update | `ClaimNameIsImmutableNotification` | 422 |
| R3 | `TenantID` | `immutable` — a claim never moves between tenants | update | `ClaimTenantIsImmutableNotification` | 422 |
| R4 | `ValueType` | `immutable` *(proposed; alternative: mutable behind a "no values exist yet" service probe)* — flipping `string`→`number` retro-invalidates every value already stored on the edges, with no cascade and no migration path. Immutable is the only answer that does not need an aggregate that does not exist yet | update | `ClaimValueTypeIsImmutableNotification` | 422 |
| R5 | `Name`, `TenantID` | Unique per tenant over ACTIVE rows — the service pre-check half of §2's enforcement, exclude-self on update | insertOrUpdate | `ClaimNameAlreadyExistsNotification` | 409 |
| R6 | `TenantID` | **manual** — the owning tenant must exist, must not be archived and must not be commercially `suspended`. A TRIAL tenant is a live customer and passes. Ask `TenantIsUnavailable`; a read join cannot answer it, because on an insert a joined field is blank | insert | `ClaimTenantDoesNotExistNotification` | 422 |
| R7 | `DefaultValue`, `ValueType` | **manual** — the default must parse as the declared type: `number` → a valid decimal number; `bool` → exactly `true` or `false`; `string` → any non-empty value. A null default is always valid and skips the check — "no default" is a legitimate state, and level 2 of the chain simply does not fire | insertOrUpdate | `DefaultValueDoesNotMatchValueTypeNotification` | 422 |
| R8 | `DefaultValue` | `length` max 256 — the claim-size budget of §2, nil-safe | insertOrUpdate | `DefaultValueTooLongNotification` | 422 |

**Not declared, deliberately.** No `required` rule on `Name`, `ValueType`, `AppliesTo` or
`Description`: all four are value-object-backed, so the framework validates them on every
write — a raw VO answers an empty value with `RequiredFieldNotification` and an enum with its
own unknown-member notification. Declaring `required` beside one makes the caller read the
same complaint twice, and `omnicore-gen check` warns about it by name.

**`AppliesTo` stays MUTABLE** *(proposed; alternative: immutable, like R4)*. Widening
(`user` → `both`) is always safe, and it is the ordinary operational move. Narrowing strands
values on the kind being dropped — but there are no edges to strand yet, and when they arrive
the guard belongs beside them, in the run that builds them, where the probe can actually be
asked. Recorded so that run does not have to rediscover it.

**Notifications this entity declares** (all seven catalogs, per `CLAUDE.md` rule 3):
`InvalidClaimNameNotification` · `UnknownClaimValueTypeNotification` ·
`UnknownClaimAppliesToNotification` · `ClaimNameAlreadyExistsNotification` ·
`ClaimNameIsImmutableNotification` · `ClaimTenantIsImmutableNotification` ·
`ClaimValueTypeIsImmutableNotification` · `ClaimTenantDoesNotExistNotification` ·
`DefaultValueDoesNotMatchValueTypeNotification` · `DefaultValueTooLongNotification`.
`TenantMismatchNotification` and `TenantMissingNotification` are framework-owned and already
translated — this entity declares neither.

**Domain service — required: true.** Facts, each named for the PROBLEM:
`ClaimNameTaken` (`exists`, filters `TenantID` + `Name`, excludeSelf, activeOnly) ·
`TenantIsUnavailable` (`manual`, filters `TenantID` — missing, archived or suspended; trial
passes). No caller-identity fact: there is no escalation surface here, because a claim
definition confers nothing.

## 8. Update shape

**PATCH** *(proposed; alternative: PUT, or both)*. No sibling, so the §4 PUT invariant does not
apply. `patchExcludes: [TenantID, Name, ValueType]` — the three immutable fields are removed
from the partial body, so the OpenAPI request schema never advertises them as editable. The
immutability rules R2–R4 **stay** and are not redundant with it: the exclusion closes the
PATCH door, the rules guard the value on every update path whatever door it came through. That
is `Permission`'s reading (`permission.omnicore.yaml:260-266`), and it closes the asymmetry
`ACCESS_MATRIX.md` records against `Role` and `Group`, which still advertise fields the domain
then refuses.

PATCH therefore carries: `appliesTo`, `defaultValue`, `description`.

## 9. Surfaces & reads

- **REST: yes** · **GraphQL: yes** (both reads and every write verb — the shape `Role`,
  `Group`, `User` and `Client` all ship) · **gRPC: no** (via `/omnicore:implement` later, if
  ever) · **Exports (CSV/XLSX): no** — operator-facing and small, the call `Permission` and
  `Role` already made · **Integration events: no** (no broker in this posture).
- **Reads:** by-id + by-params.
- **View backing: relational** — the project posture (`../../scaffold-service/spec.md`:
  Postgres SoR, no Mongo, no broker). Read-your-writes, no version, no rebuild.
- **Reserved read controls:** `pagination` ✔ · `orderBy` ✔ · `?fields=` ✔ · `?onlyTotal` ✔ ·
  `?includeArchived` ✔ (with no unarchive verb, the listing is the only way to see a retired
  definition) · `?search=` ✘ — **not declared**: a relational-served read model answers it with
  a typed 400 `UnsupportedCapabilityNotification`, so declaring it would promise what this
  posture cannot serve (`relational-view.html`, v0.62.0).
- **Computed read fields:** none. Every value the reads return is a stored column or a joined
  one; nothing here is derived.
- **Field-level read authz:** none. A definition is vocabulary, not a secret — `DefaultValue`
  is the only field that carries a business value at all, and any holder of `claim:read` in the
  tenant is already entitled to it.
- **Read joins** *(proposed; alternative: none)*: **`inner` → `Tenant` on `tenant_id`**, bringing
  `TenantWorkspace` (`workspace`) and `TenantStatus` (`status`), both visible on the wire.
  `inner` is correct because `tenant_id` is NOT NULL and FK-backed. A ROOT join's fields are
  addressable in a criteria, so both are filterable and sortable like any local column — which
  is what makes *"claims of acme-comercio"* answerable without a second call. Neither carries a
  domain type: the value belongs to `Tenant`, arrives read-only, and reconstructing
  `vos.TenantStatus` would hand back an instance no rule of the owning aggregate approved.
  This mirrors `Role` exactly.
- **Filters / sort:**

  | Field | Operators |
  |---|---|
  | `TenantID` | `eq, in` |
  | `Name` | `eq, ne, in, startswith, istartswith, contains, icontains` |
  | `ValueType` | `eq, in` |
  | `AppliesTo` | `eq, in` |
  | `DefaultValue` | `eq, in, contains, icontains` |
  | `Description` | `contains, icontains` |
  | `TenantWorkspace` (join) | `eq, in, startswith, istartswith` |
  | `TenantStatus` (join) | `eq, in` |
  | `CreatedAt`, `UpdatedAt` | `gte, lte` |

  **Sort:** `Name, ValueType, AppliesTo, TenantID, TenantWorkspace, CreatedAt, UpdatedAt`.

## 10. Authorization

- **Permission gate (Layer 1)** *(proposed)* — the taxonomy the service already grants, one
  action per operation, no synonyms:

  | Operation | Permission |
  |---|---|
  | `POST /claims` · `createClaim` | `claim:insert` |
  | `PATCH /claims/{id}` · `patchClaim` | `claim:update` |
  | `PATCH /claims/{id}/archive` · `archiveClaim` | `claim:archive` |
  | `GET /claims` · `GET /claims/{id}` · `claims` · `claim` | `claim:read` |

  No fifth verb. `Role`, `Group` and `User` each carry a `:grant` **because they have a
  collection whose contents change what a principal can DO** — "may rename it" and "may change
  what it confers" are separately grantable. This aggregate has no collection and confers
  nothing, so a `claim:grant` here would gate nothing. The verb that eventually sets a VALUE on
  a principal is a real question — and it belongs to the edges, on `User` and `Client`, not
  here (§0).

- **Data-access (Layer 2/3):** **tenant-scoped, on reads AND writes** — the standard this
  service applies to every owned registry, and what `ACCESS_MATRIX.md` records for `Role` and
  `Group`:
  - reads: `ToCriteria` forces `Filter["TenantID"] = identity.TenantID()`;
  - writes: the domain refuses a row whose `tenant_id` is not the caller's
    (`TenantMismatchNotification`, 403) — the guard, not the read filter, is what stands between
    a caller and another tenant's definition;
  - `tenantID` is **server-assigned from the caller's claim, and stateable only by the caller
    who crosses the scope** — `assignedFrom: identity-claim` + `bypassMaySet: true`
    *(amended 2026-08-28, after the maintainer approved the change; see the amendment note
    below)*. It appears in the INSERT body as an OPTIONAL value — absent means "mine" — and
    in no update body at all, so §8 no longer needs to exclude it. Nothing is checked in the
    mapper: a stated value is applied whoever sent it, and the guard above is what answers,
    with the same 403 a foreign write meets;
  - **bypass: `*:*`** — a platform operator supporting a customer crosses the row scope; a
    resource wildcard (`claim:*`) does not;
  - **`noIdentity: stand-down`** — `ctx.Identity()` nil happens only under `auth.mode: disabled`,
    which the framework's own boot guard allows in dev only. An identity present with an
    insufficient claim still refuses. Two states, not one.
  - a by-id read of another tenant's claim answers **404**, not 403: it does not exist for this
    caller, which leaks nothing about who else exists.

## 11. Recorded, not decided here

- **Does a tenant need to override the default of a PLATFORM-defined claim?** The one question
  that reopens the third level (`tenant_claims`). Nothing in this spec forecloses it: adding a
  middle level later is additive and touches the resolution chain, not this table. Until it is
  asked, two tenants both needing `x_region` write two definitions — the vocabulary can diverge
  between them, which is the trade the two-level shape makes.
- **The bundle** (`claim_sets`) stays out, for the reason the backlog gives: two packages
  granted to the same principal can carry the same definition with different values, which is
  the Keycloak case — undefined precedence reaching the token. The two levels deliver
  "default plus specialised" without it.
- **`Custom claims on Group`** (the older backlog entry) is not answered by this shape: a user
  reaches several groups, so the collision this design removes by construction returns there.
