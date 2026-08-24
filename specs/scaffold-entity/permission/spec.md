# Spec: Permission

- **Status:** APPROVED
- **Approved:** maintainer (Cláudio Schirmer Guedes), 2026-08-19 — the OPEN slots answered
  at the model gate (§B), then three refinements taken at the plan gate (§B Q5–Q7): the
  one-way archive, the collapsed value objects and the lean read payload. The `(proposed)`
  picks of §C stand
- **Pin:** omnicore **`v0.57.1`** · dialect postgres · Postgres SoR, no Mongo, no broker →
  relational-served views
  (modelled against `v0.57.0`; built against `v0.57.1`, a fix-only patch that changes
  nothing this entity declares — see `tasks.md` deviation 3)
- **Language:** English (all artifacts) · Portuguese (chat) — per `../../../CLAUDE.md`
  rule 3 and the maintainer's invocation
- **Generation:** omnicore-gen

The **global catalog of enforceable permissions**. One row per `resource:action` pair that
some route in the platform actually enforces — `tenant:read` is a string literal in the
tenant routes, and this entity is what makes that literal a row an operator can list,
describe and grant.

The single most important idea in this spec: **three columns go in, two go out.**

| | Stored | **Filterable** | Served on the wire |
|---|---|---|---|
| `resource` | ✅ own column | ✅ | ❌ — folded into `permission` |
| `action` | ✅ own column | ✅ | ❌ — folded into `permission` |
| `description` | ✅ own column | ✅ | ✅ |
| `permission` | ❌ no column | ❌ impossible by construction | ✅ **derived at read time** — `resource:action`, the exact string a JWT claim carries and `RequirePermission(...)` compares against |

The three stored columns are all **filterable**; only two things leave on the wire. Filtering
is declared by the Request DTO's own tags and does not consult the Response at all
(`auto-query-handlers`), which is what lets those two facts coexist.

`resource` and `action` are stored apart so each can be filtered on independently; they are
validated and rendered by **one type, in one place** — the composite value object
`vos.PermissionKey`. Nothing else in the service validates a permission part or
concatenates a resource and an action with a colon.

---

## 0. Inputs this spec was built from

| Input | What it contributed |
|---|---|
| Maintainer invocation | the three fields; reuse the existing `Description` VO; a **composite VO** taking resource + action, validating each separately, storing them separately, and rendering `resource:action` on the way out for the JWT |
| Maintainer decisions at the model gate | §B — five decisions: the wildcard vocabulary, the frozen key, active-only uniqueness, no seed, and the one-way archive |
| `../../../README.md` (§ Domain model) | `Permission` is a **global catalog**, defined by the platform, **not tenant-scoped** and read-only to tenants; the target `User → Group → Role → Permission` graph |
| `../tenant/spec.md` (APPROVED) | the local flavor: shared vs entity-specific VOs, the anti-junk predicates, archive-not-delete, the `<entity>:<verb>` permission taxonomy already granted on the tenant routes |
| `../../scaffold-service/spec.md` (APPROVED) | posture: Postgres SoR, **no Mongo**, no broker → relational-served views; REST + OpenAPI + GraphQL wired |
| omnicore `v0.57.0` `/docs` | `value-objects` (the composite kind), `table-schema` (`Composite`/`As`, the boot panics, the once rule), `auto-query-handlers` (computed read fields), `authz-seams` (the permission string format and the claim-side wildcard rules), `read-joins` (what it means for THIS catalog to be a traversal target), `relational-view` |
| `omnicore-gen explain coverage` | composite value objects ✓ and computed read fields ✓ are both emitted by the generator — the 1d gateway has two real options |

### Verified framework facts that shaped this spec (read, not assumed)

| Fact | Evidence at the pin (`v0.57.0`) |
|---|---|
| A composite VO declares `IsValid` and **must not** declare `Value()`; that method is the discriminator | `value-objects.html`, "Composite value objects" |
| Its canonical rendering is exposed "under any other name (`String()`, `Format()`)" | `value-objects.html`, same section |
| The schema is the only place that knows it spans columns — `Composite(core.NewCompositeValueObject[T]().Field(..).As(..))` | `table-schema.html`, `NewCompositeValueObject` |
| Downstream nothing learns a composite exists: filters, `orderBy`, `?fields=`, OpenAPI, GraphQL and exports all see the exposed part names | `table-schema.html`, same section |
| A computed Response field declares `computed:"Src1,Src2"` and is filled in the query's `FromQueryResult` — every surface then agrees | `auto-query-handlers.html`, "Computed fields" |
| A computed field **cannot be filtered** (boot panic) and **cannot be ordered by** (typed 400) | `auto-query-handlers.html`, same section |
| Permission strings are `resource:action`; the CLAIM side may carry `resource:*` / `*:*`, the ROUTE side may not (runtime panic) | `authz-seams.html`, Layer 1 |
| The claim matcher honors exactly three shapes: exact, `resource:*`, `*:*` | `authz-seams.html`, Layer 1 match table |
| The framework's own example gates a **hierarchical** resource: `RequirePermission("users:profile:read")` | `authz-seams.html`, Layer 1 |
| `Modes()` and the schema's archive-column declaration must agree, and are cross-checked at repository construction | `table-schema.html` · `../tenant/spec.md` §C |

---

## 1. Storage model                                    [high-risk — confirmed]

- **Kind: flat** (alternative considered: sharedbase-role — rejected).
  **No identity smell.** A permission is not a party playing a role: it carries no person,
  no document, no e-mail, no natural registry key of an asset. It is a catalog entry. There
  is no second role a `tenant:read` could also become, so a shared base would buy structure
  with no dedup to perform.

- **ER sketch**

```
permissions                                   -- the global catalog of enforceable permissions
  id           UUID     PK   (framework, UUIDv7 — internal, never issued)
  revision     INTEGER       (optimistic concurrency, framework-managed)
  resource_name VARCHAR(64)  NOT NULL   -- part 1 of vos.PermissionKey; EXPOSED as `resource`
  action_name   VARCHAR(64)  NOT NULL   -- part 2 of vos.PermissionKey; EXPOSED as `action`
  description  VARCHAR(500)  NOT NULL
  deleted_at   TIMESTAMPTZ   NULL       -- archive (one-way, §6)
  created_at   TIMESTAMPTZ   NOT NULL
  updated_at   TIMESTAMPTZ   NOT NULL

  UNIQUE (resource_name, action_name) WHERE deleted_at IS NULL   -- partial: ACTIVE rows only (§B Q3)
```

  **The `_name` suffix is deliberate and is not a wire name.** `resource` is a **reserved
  word on oracle**, and identifiers are not always emitted quoted, so the physical column
  carries the suffix; `action` follows it rather than leaving the pair asymmetric. The
  parts' EXPOSED names are untouched — every filter, `?orderBy` token, OpenAPI parameter,
  GraphQL argument and audit entry still says `resource` and `action`, which is the whole
  point of the composite's exposed-name layer.

  One table. No FK: the catalog is global, so there is **no `tenant_id` column** — see
  `../../../README.md`, "What is scoped to a tenant, and what is not". The future
  `role_permissions` join table (a later run) points at `permissions.id`.

  **Table description** (becomes the Postgres `COMMENT ON TABLE`): *The global catalog of
  enforceable permissions. One row per resource:action pair some route enforces; defined by
  the platform, never by a tenant. The resource:action string is rendered on read, never
  stored.*

### This table is a read-join TARGET, which makes two of its column names a cross-entity contract

`../role/spec.md` §2 declares a read join from `role_permissions.permission_id` into this
table, mapping `resource_name` → `Resource` and `action_name` → `Action` on each grant.
Three consequences that belong here rather than there, because they constrain **this**
entity:

- **`permissions.id` must stay the traversal's target.** A join's predicate is always
  `fk = target.id`, so the id column is what the other side lands on. Nothing in this spec
  proposes moving it (§1 rejects a derived public key outright), and this is one more reason
  not to.
- **Renaming `resource_name` or `action_name` is a two-file change.** The traversal names
  the target's physical columns, so a rename here silently stops `Role`'s join from
  resolving. They were already renamed once, away from `resource` / `action`, because
  `resource` is an oracle reserved word — that rename is now load-bearing beyond this
  aggregate and must not be undone casually.
- **`deleted_at` is reachable, and `Role` maps it.** A join may carry the target's managed
  columns — `created_at`, `updated_at`, `deleted_at`; `revision` is out — so `../role/spec.md`
  §2 renders each grant's `ArchivedAt` from this catalog's own archive stamp. Two boundaries
  come with it, and both belong here rather than only on the consuming side. First, the join
  is **not gated** on that state: an archived row keeps supplying its columns and an inner
  join keeps matching it, so the stamp *reports* the retirement, it never filters it out —
  which is what an access review needs, since a grant on a retired permission must stay
  readable. Second, a traversal answers only for rows already **stored**: an entry a write is
  adding carries no joined value at all, so a consumer's rule cannot ask this catalog anything
  through the join. That is exactly why `Role` and `Group` keep their domain-service probes
  (`../role/spec.md` §7) — the join renders the catalog, the probe judges it.

- **Public key: none** (alternative: a derived `permission_id` UUIDv5 mirroring
  `tenants.tenant_id`). Tenant needed one because its PK is a UUIDv7 whose embedded
  timestamp would have leaked creation order to every token holder. Nothing here is issued
  to a token: **the JWT carries the rendered string `tenant:read`, not an id**, and the only
  id consumer is a future internal join table. A derived column would add a column with no
  reader.

- If sharedbase-role: `N/A — flat`.

## 2. Fields

| Field | Go type | VO? | Nullable | Unique | Lives on | `example:` | Description |
|---|---|---|---|---|---|---|---|
| **Key** | `vos.PermissionKey` | **new-composite `vos.PermissionKey`** | no (mandatory as a whole) | **yes — the pair, among active rows** | root, **2 columns** | `tenant:read` (rendered) | The permission itself: what resource, and what may be done to it. Stored as two columns, rendered as one string. |
| ↳ part `Resource` | `string` | part of the composite — no type of its own | no | (half of the pair) | column `resource`, exposed as `Resource` | `tenant` | The thing being protected, as the enforcing route names it. `*` means every resource. |
| ↳ part `Action` | `string` | part of the composite — no type of its own | no | (half of the pair) | column `action`, exposed as `Action` | `read` | What may be done to the resource. `*` means every action on that resource. |
| **Description** | `vos.Description` | **reuse `vos.Description`** | no | no | root, column `description` | `Read tenants: list the tenant registry and fetch a tenant by id.` | What holding this permission actually lets a caller do, in the platform operators' own words. |

Notes that belong to the table above:

- **The composite is ONE concept, not two fields.** `Resource` and `Action` only mean
  something together — `read` alone is not a permission and `tenant` alone is not one
  either. That is exactly the case the composite kind exists for, and it is why they are
  written as one row with two parts rather than as two rows.
- **The parts are plain `string`, not value objects of their own** (§B Q6). A value object
  exists to give a rule ONE home; here both parts are built from **one shared segment rule**
  (§7a) and no other aggregate in this service carries a resource or an action on its own,
  so that home is the composite itself. Two extra named types would mean two copies of one
  helper, two more notification registrations and fourteen catalog entries buying nothing.
  Should a later entity ever need the rule standalone, extracting it is mechanical — a part
  gains a named type and the composite delegates to it — and touches no column, no wire name
  and no data.
- **Held by value, not by pointer** ⇒ mandatory as a whole, and both part columns are
  `NOT NULL`. (A pointer composite would make every part column nullable and let a
  permission exist with no resource and no action, which is not a thing.)
- **No `.As(...)` rename.** The default exposed names are the parts' own names,
  `Resource` and `Action`, which is what reads right for a value object this specific.
  The criteria, the filters, the audit timeline and the OpenAPI parameters therefore see
  plain `resource` and `action` and never learn a composite exists.
- **`vos.Description` is reused verbatim** — 15–500 runes, ≥ 2 words, ≥ 5 distinct runes,
  no run of 4 identical, at least one vowel. Its doc comment already anticipates this
  ("Shared with Group and Role"); nothing in it knows what a tenant is. **No second copy.**
- **`labelKey` for the parts lives inside the value object** (`PermissionResourceField`,
  `PermissionActionField`), per `value-objects.html`: a notification emitted on a part's
  name resolves its label from the tag inside the VO, so the entity declares nothing for
  them — and this works exactly the same with plain-string parts, since the tag is read off
  the struct, not off the part's type.
- The entity field name `Key` is a naming call, not a modeling one — say the word and it
  becomes `Grant`, `Scope` or `Target` with no other change.

### The value object owns BOTH halves — validating and rendering

This is the point of the composite, and it is deliberate: **how a permission is checked and
how a permission is written are one concept, so they live in one type.** Nothing outside
`vos.PermissionKey` knows that the separator is a colon.

`vos.PermissionKey` exposes **`String() string`** returning `resource + ":" + action`.

- **Not `Value()`** — declaring `Value()` on a composite is a boot panic, because that
  method is the discriminator the framework uses to tell a scalar VO from a composite
  (`table-schema.html`). `String()` is the name the docs name for a canonical rendering,
  and it satisfies `fmt.Stringer`, so `%s` and every logger render it for free.
- The invocation asked for `getPermission()`; the Go spelling of that is `String()`. If a
  more explicit name is wanted, `Token()` or `Permission()` are both free — one line.
- **Every consumer of the format calls this method**, and there are three:
  1. the read side's computed field — the query's `FromQueryResult` **reconstructs the value
     object from the two stored columns and calls `String()`** (§9). It does not join two
     strings; it rebuilds the domain type and asks it;
  2. the write side's responses — the command mapper's `FromEntity` calls it on the entity's
     own field;
  3. the future token issuer, which has not been written yet.
- **Acceptance, checked at the final gate:** a search for a colon-joining expression outside
  this value object must find nothing. One separator, one place, one test.

## 3. Children (1:N)

`N/A — no collections.` A permission owns nothing. The many-to-many `Role ↔ Permission`
edge from `../../../README.md` is a join table owned by `Role`'s run, not a child of this
aggregate: it is edited from the role's side and it is not restorable through this root.

## 4. Siblings (1:1)

`N/A — no optional field at all.` All three values are mandatory, so there is no sparse,
bulky or PII facet to split off. Recorded rather than skipped: the trade-off was considered
and there is nothing to trade.

## 5. Modes                                             [required]

**`display · insert · update · archive`** — **no `unarchive`, no `delete`** (§B Q5).

| Mode | Why it is here / absent |
|---|---|
| `display` | the catalog exists to be read |
| `insert` | the platform registers what its routes enforce |
| `update` | `description` is the one editable field (rule 9 freezes the rest) — a permission's wording is exactly what gets improved after operators read it |
| `archive` | retiring a permission without destroying the record of what a past grant meant |
| ~~`unarchive`~~ | **deliberately absent.** Un-archiving would re-enable, in one call, every grant still pointing at that row — old users would silently regain a permission nobody re-approved, with nothing in the audit trail reading like a grant. Coming back is therefore a **new row**, minted by an explicit `insert` that nobody holds yet (§6) |
| ~~`delete`~~ | absent — a purge destroys the only human-readable record of what an issued token's claim meant (§6) |

- `Modes()` lists `Archive`, so the schema declares its `deleted_at` column and the
  migration carries it — the three must agree or the boot aborts.
- **View archive regime**: the view is relational-backed (§9), so `DeleteOnArchive()` is not
  in play — an archived row is filtered out of default reads and returned when the caller
  asks for archived ones.

## 6. Delete semantics                                  [required]

**Soft, and ONE-WAY: `PATCH /permissions/:id/archive` only.** No `DELETE`, no
`/unarchive`.

- **Why no hard delete.** A permission that has ever been granted is referenced by role rows
  and named in issued tokens and audit lines. Purging the row destroys the only
  human-readable record of what a past grant meant.
- **Why no unarchive.** Archiving a permission is a revocation with a paper trail; undoing
  it is a *grant* to everyone who still references the row, performed by a verb that does
  not look like granting anything. The maintainer's call: that is a silent privilege
  restoration, and the catalog must not offer it.
- **How a retired permission comes back**, since it must sometimes: insert it again. The
  unique index is partial (`WHERE deleted_at IS NULL`), so the archived remnant does not
  block the pair — and the new row gets a **new `id`**, which is the whole point. Old
  `role_permissions` rows point at the archived id and stay dead; whoever wants the
  permission back must grant the new row explicitly, which is visible, attributable and
  auditable. **§B Q3 and §B Q5 are two halves of one design**: active-only uniqueness is
  what makes the one-way archive livable, and the one-way archive is what makes active-only
  uniqueness safe.
- Verb truth is respected: `DELETE` would be an irreversible purge, and nothing here is one.
- Per-child: `N/A — no children`.

## 7. Business rules                        [required]

Format, length and presence rules live in the value objects and are **never repeated** in
`BuildRules` — the framework validates a VO field on every write, so a duplicate rule would
report the same problem twice.

### 7a. The part rules — one segment helper, two thin uses

`vos.PermissionKey` validates both parts itself (§B Q6). One private helper defines what a
**segment** is; each part then says how many segments it accepts.

**The segment rule** — a 2–64 rune lowercase slug: `^[a-z0-9]+(-[a-z0-9]+)*$`, single
hyphens, never leading or trailing, and no run of 4+ identical runes (reusing the project's
existing `hasRunOfIdenticalRunes` helper). Written once.

| # | Part | Rule | Notification |
|---|---|---|---|
| 1 | both | not empty | `domain.RequiredFieldNotification` (framework) |
| 2 | both | the wildcard `*` **exactly** is accepted, and short-circuits the rest | — |
| 3 | `Resource` | **a colon-joined path of segments** — one segment (`tenant`) or several (`user:profile`), each passing the segment rule; 64 runes total | `InvalidResourceNameNotification` |
| 4 | `Action` | **exactly one segment**, no colon | `InvalidActionNameNotification` |

Each is emitted under the failing part's own name — `"Resource"` or `"Action"` — so the
caller reads which half is wrong, and the label comes from that part's tag inside the value
object. The two notifications stay distinct because "your resource is malformed" and "your
action is malformed" are different problems, and a caller reading the wrong one edits the
wrong field.

**Why the resource carries a hierarchy and the action does not.** The framework's own doc
gates `RequirePermission("users:profile:read")` — a resource may legitimately be a path.
Keeping the action to a single segment is what makes the rendering unambiguous: **the last
colon segment is always the action**, so `user:profile:read` parses back exactly one way.

**`*` is legal only as the ENTIRE part** — never as a segment inside a path (`user:*`),
never mixed into a slug (`ten*`). Reason, from `authz-seams.html`: the claim matcher
recognizes exactly three shapes — an exact string, `resource:*`, and `*:*`. A partially
wildcarded value fits none of them, so it would be a row that matches nothing while looking
like it grants something. Rule 2 makes it unrepresentable.

**No normalization**: `Tenant` and ` tenant ` are refused, never repaired — the same
doctrine `vos.TenantWorkspace` records, and here it matters more, because the value is
compared byte-for-byte against a token claim.

The vocabulary is **open** (§B Q1): `read`, `insert`, `update`, `archive` today, and
`impersonate`, `rotate-key` or `issue` tomorrow with no enum to widen and no migration.

### 7b. `vos.PermissionKey` — what only the composite can see

| # | Rule | Notification |
|---|---|---|
| 5 | both parts pass their 7a rule, each reported under its own name | (7a's) |
| 6 | **a `*` resource forces a `*` action** — `*:read` is refused | `UnmatchablePermissionKeyNotification` |
| ~~7~~ | ~~the **rendered** token is at most 129 runes (64 + `:` + 64)~~ — **withdrawn 2026-08-20, enforced structurally**; see below | ~~`InvalidPermissionKeyNotification`~~ — removed |

**Rule 6 is the reason this concept had to be a composite** and could not be two loose
fields: it is only expressible with both values in hand. `authz-seams.html` lists the three
claim shapes the matcher honors — exact, `resource:*`, `*:*`. `*:read` is not one of them,
so a caller granted it would match no route ever, while the catalog row reads like a
sweeping grant. The rule refuses the row instead of shipping the illusion.

**Rule 7 was withdrawn at the rebuild — the bound it stated is real, the check was not.**
It bounded what one entry costs in a `permissions` claim, and that bound still holds: each
part is independently capped at 64 runes by `isResource`/`isAction`, and the pair-level
rules run only after both parts pass, so the rendered form is at most 64 + `:` + 64 —
*exactly* the cap. The branch could therefore never be taken, and
`InvalidPermissionKeyNotification` had no raiser: dead code plus a notification type and
seven catalog entries nothing could ever emit.

Withdrawn on the maintainer's call, 2026-08-20, having been surfaced rather than removed
silently. What replaces it is a comment where the parts are sized, saying that the claim
budget is what the two 64-rune bounds are FOR — so that widening a part is visibly a
decision about the token as well. `maxRenderedRunes` is kept as a declared derived bound
for the same reason. The invariant is not lost; only the unreachable statement of it is.

### 7c. `Permission.BuildRules`

| # | Field(s) | Rule | Verb scope | Notification | HTTP |
|---|---|---|---|---|---|
| 8 | `Key` | the `(resource, action)` pair is not already taken **among active rows** — domain-Service pre-check with exclude-self, DB partial unique index as the race backstop | `IfInsertOrUpdate` | `PermissionAlreadyExistsNotification` | 409 |
| 9 | `Key.Resource`, `Key.Action` | **immutable after creation** — a changed pair silently changes what every role grant and every issued token means (§B Q2) | `IfUpdate` | `PermissionKeyIsImmutableNotification` | 422 |
| 10 | `Description` | must differ from the rendered `resource:action` under a normalized comparison — the description must explain the permission, not echo it | `IfInsertOrUpdate` | `PermissionDescriptionEchoesKeyNotification` | 422 |

- Rule 8 uses `enforce: service-precheck+constraint` (recommended style): the duplicate is
  reported **together** with any other validation error instead of arriving alone as a 409
  after everything else passes. The check is over the pair, not over either column — a
  second `tenant:<something else>` is perfectly legal. Its scope is **active rows only**
  (§B Q3): an archived `tenant:export` does not block a fresh one, which is the only route
  back now that `/unarchive` does not exist (§6). There is no `IfUnarchive` re-check to
  write — the mode is not declared.
- Rule 9 leaves `description` as the only editable field. That is not a degenerate `update`.
- Rule 10 mirrors Tenant's "description must differ from name/workspace" manual rule, for
  the same reason: it catches the lazy paste and nothing more.

**`RequiresService() bool { return true }`** — rule 8 needs an existence probe, so a
`PermissionService` is declared and must be wired end to end (feature constructs it, every
write handler receives it).

### 7d. What the wildcard grant costs — recorded, accepted

`*` was admitted deliberately (§B Q1), so the consequence is written down rather than
discovered later:

- A role granted `tenant:*` holds **every permission that will ever exist for `tenant`**,
  including ones added by a future release that nobody reviewed the grant for. That is the
  nature of a wildcard, not a defect of this model — but it is why the catalog should carry
  few of them, and why `permission:insert` is a platform-only grant (§10).
- `*:*` is the super-admin row. One row, total power.
- A wildcard row is **grantable, never enforceable**: no route may declare
  `RequirePermission("tenant:*")` — the framework panics at runtime on a caller-side
  wildcard reaching the route side. The catalog therefore holds two kinds of row: the ones
  that mirror a literal in the code, and the wildcard ones that only ever appear in grants.
  Nothing in the schema distinguishes them, and nothing needs to.

## 8. Update shape                                      [required]

**PATCH only** (alternative: `both`). No sibling exists, so nothing needs PUT's ability to
assign null, and every field is mandatory — there is nothing clearable.

**The pair is excluded from the partial body — `patchExcludes: [Key]`** (maintainer's call,
2026-08-24, taken during the build). Rule 9 already refused a changed pair with a 422, but
it was the ONLY layer: the PATCH request carried `resource` and `action`, the OpenAPI schema
documented both as editable, and `ApplyPartiallyTo` assigned them onto the entity before the
rule compared against the snapshot. Excluding them makes the refusal structural — there is
no field to send, nothing to assign, and nothing in the published contract claiming
otherwise. Mirrors what `../tenant/spec.md` does with `Workspace`.

**Rule 9 STAYS, and the two are not redundant.** This key closes the PATCH door; the rule
guards the value on every update path, whatever door it came through. The reason is the one
the maintainer gave: a permission whose `resource:action` moves hands the old permission,
free, to everyone who already held it — the grant rows and the issued tokens still say the
old string, and it now means something else.

## 9. Surfaces & reads                       [required]

- **REST: yes** (OpenAPI documented) · **GraphQL: yes** — both, mirroring Tenant, so the
  catalog is reachable from the same two surfaces every other aggregate uses.
- **gRPC: no** (available later through `/omnicore:implement`, no rework).
- **Exports (CSV/XLSX): no** — the catalog is small and operator-facing; say the word and
  it is one flag.
- **Integration events: no** — the posture has no broker, so publishing is unavailable
  (`../../scaffold-service/spec.md`). Noted here so it is not silently forgotten.
- **Reads**: by-id + by-params (the expected defaults).

### The wire shape — three columns filtered, two values returned (§B Q7)

**Every read and write response carries exactly:**

| Wire field | Source | Notes |
|---|---|---|
| `id` | the row id | not a data field — without it no caller can address the row for a patch or an archive |
| `description` | the column | |
| `permission` | **computed** | `computed:"Resource,Action"`, rendered by the value object |

`resource` and `action` are **read from the store and never leave the application layer**.
This is the documented shape, not a workaround: `auto-query-handlers` states that a
computed field's sources are *optional on the Response* and *mandatory on the Result*.

| Layer | Carries |
|---|---|
| the view / the store | `resource`, `action`, `description`, the managed columns |
| the query Result | `Resource`, `Action`, `Description`, **`Permission`** (derived) |
| the Response | `Description`, `Permission`, `ID` |

Two boot guards this shape must respect, both verified against the pin:

- **Result↔Response alignment**: every exported Response field needs a same-named Result
  field. `Permission` therefore exists on the Result — filled by `FromQueryResult` — and not
  only on the wire.
- The Result carries **no** `json` tags; wire naming belongs to the Response alone.

**What this shape costs: no ordering, and no filtering.** The ordering vocabulary *"lives
where the endpoint declares what it accepts on the wire — the Request DTO — and **never on
the Response**"* (`auto-query-handlers.html`, "The ordering pair"), and filters are declared
the same way. A leaf may therefore be orderable and filterable while carrying no value on
the wire at all. All three stored fields are, while only `id`, `description` and
`permission` leave on the wire — the two hidden composite parts included, verified with
`omnicore-gen check`.

What is genuinely unavailable, and why:

| Path | Orderable | Why |
|---|---|---|
| `resource` | **yes** | index-backed — the prefix of the partial unique index on the pair |
| `action` | **yes** | declared; a blocking sort on its own (not an index prefix) |
| `description` | **yes** | declared; a blocking sort — no index |
| `permission` | no | computed, and a computed path has no column to order by |
| `id` | no | not in the declared vocabulary. It is declarable — `sort: [ID]` is an ordinary entry — so this is the model's choice, not a limit |

The two blocking sorts are admitted deliberately. The project has no indexed-only rule to
break — `tenants` itself admits two unindexed sorts (`../../upgrade/migration-plan.md` §1)
— but the cost is worth naming per entity rather than inheriting. Here it is immaterial:
this catalog is bounded by the number of `resource:action` pairs the platform's own routes
enforce — dozens — and capped at 200 rows per page. `tenants`, which grows with the
customer base, is the one where it will eventually need an index.
**Filtering is untouched** and never consulted the Response in either version.

- **Reserved read controls served by the listing:**

| Control | Served | Why |
|---|---|---|
| pagination (`?first=`, cursor) | yes | default |
| `?orderBy=` | yes — over `resource`, `action` and `description` (§B Q8) | the id is declarable and simply not declared; the computed `permission` is the one path that cannot be, see the table above |
| `?fields=` | yes | `?fields=permission` returns the strings and nothing else — exactly what a token issuer wants. Selecting it pushes `Resource` + `Action` down to the store automatically |
| `?includeArchived` | yes | a retired permission must stay auditable — and with no unarchive verb, this listing is the only way to see one |
| `?onlyTotal` | yes | cheap |
| `?search=` | **no** | the view is relational-backed: free text is answered with a typed 400 `UnsupportedCapabilityNotification` (`SemanticSchema`). `?filter[description][contains]=` covers the real need |

- **Filters — all three stored fields, per the operator table:**

| Field | Operators |
|---|---|
| `resource` | `eq`, `ne`, `in`, `contains`, `startsWith` |
| `action` | `eq`, `ne`, `in`, `contains` |
| `description` | `contains` |
| `createdAt` / `updatedAt` | `gte`, `lte` |
| `permission` (computed) | **none — impossible by construction**; filter the two sources instead |

  A caller looking for `tenant:read` sends
  `?filter[resource][eq]=tenant&filter[action][eq]=read`; one looking for everything on
  tenants sends `?filter[resource][eq]=tenant`. Every scalar filter field is declared as a
  pointer, so none of them renders as required in the generated spec.

- **Field-level read authz:** none. Every field of a catalog entry is visible to any caller
  holding `permission:read`; there is no secret in a permission's name. (The two columns a
  caller never sees are hidden by the Response shape, not by an authorization rule.)
- **View backing: relational** — `query.RelationalView("permissions", repo.Loader)`,
  contributed through the feature's `RelationalViews()` opt-in. The project posture, and the
  only backing available (no Mongo). Reads are read-your-writes: a permission created is
  visible to the very next read, with no CDC round-trip. The view takes its schema from the
  loader, and carries no `Version`, no registry row, no rebuild and no Mongo collection.
- **This entity declares no read join of its own.** A permission holds no foreign key to
  anything: the catalog is global (`../../../README.md`, "What is scoped to a tenant"), so
  there is nothing to traverse into. It is a traversal TARGET, not a source — see §1.
- **`?fields=` naming a path this read model does not have is a 400**
  (`SchemaViolationNotification`, `SemanticSchema`) that names the offending Go path, never
  a silent `200 {}`. `?fields=permission` still pushes `Resource` + `Action` down.
- **Pagination is a camouflaged offset** on this backing — the same wire contract as a Mongo
  view, with an absolute row index inside each cursor instead of a sort-key tuple. Immaterial
  for a catalog bounded by the platform's own route literals, and recorded rather than
  assumed.

## 10. Authorization                          [required]

- **Permission gate (Layer 1)** — matching the taxonomy the tenant routes already grant
  (`<resource>:<verb>`, the action spelling the operation):

| Operation | Permission |
|---|---|
| `POST /permissions` · `createPermission` | `permission:insert` |
| `PATCH /permissions/:id` · `patchPermission` | `permission:update` |
| `PATCH /permissions/:id/archive` · `archivePermission` | `permission:archive` |
| `GET /permissions` · `GET /permissions/:id` · `permissions` · `permission` | `permission:read` |

  Singular resource, one verb per operation — identical in shape to Tenant, so no synonym
  enters the vocabulary. Tenant's `permission:archive` covers an unarchive twin; here there
  is none to cover (§6). **This service therefore gates itself with rows out of its own
  catalog**, and per §B Q4 those rows are inserted by an operator, not by a migration.

- **Data-access (Layer 2/3): none — anyone holding the permission sees and edits every
  row.** Stated explicitly, not skipped. The catalog is global by design
  (`../../../README.md`): it is not partitioned by tenant, so there is no `tenant_id` to
  filter on and no owner to check. Tenants never write it at all — they receive no
  `permission:insert`/`:update`/`:archive` grant, only `permission:read` if the product ever
  shows them the catalog.

---

## B. Decisions taken at the model gate (no longer open)

| # | Question | Answer |
|---|---|---|
| **Q1** | Is `action` an open validated slug, a closed enum, or an open slug that also accepts `*`? | **Open slug, and `*` is allowed** — as a whole value only, on either part. An enum was rejected because it would cap the catalog at CRUD verbs while real non-CRUD actions are coming; wildcards were admitted because a broad grant is then one row instead of a growing list. The cost is recorded in §7d, and the shapes the claim matcher cannot honor are refused by rule 6 rather than stored |
| **Q2** | Are `resource` and `action` frozen after insert? | **Frozen** — rule 9. The pair is the permission's identity everywhere except this table: the string in the JWT claim and the literal in `RequirePermission(...)`. Editing it would rewrite the meaning of every existing grant, retroactively and invisibly. `description` stays editable |
| **Q3** | Is `UNIQUE(resource, action)` over all rows or active only? | **Active only** — a partial index `WHERE deleted_at IS NULL`. Nothing is derived from the pair (unlike `tenant.workspace`, whose reuse would mint a colliding public key), so an archived remnant must not block a new active row. With Q5 this becomes load-bearing rather than a convenience: re-inserting is the *only* way a retired permission comes back |
| **Q4** | Should the migration seed the catalog with the permissions the code already enforces? | **No** — the migration creates the table and nothing else. The catalog is populated through the API by whoever operates the platform. Consequence recorded: the service ships gating eight literals (four `tenant:*`, four `permission:*`) that have no catalog row until someone inserts them, and `../../../README.md`'s line about the catalog being "seeded from what the code actually enforces" becomes a statement of intent for an operator, not of a migration. **Correcting that README line is a task of this run** |
| **Q5** | Does the aggregate accept `unarchive`? | **No — archive is one-way.** Un-archiving would re-enable, in a single call, every grant still pointing at that row: old users would silently regain a permission nobody re-approved, and the audit trail would show a restore rather than a grant. A retired permission comes back as a **new row** (new id) that must be granted explicitly — see §6. The mode is absent from `Modes()`, so no route, no mutation, no command, and no `IfUnarchive` rule is generated |
| **Q6** | Do `resource` and `action` each get their own value object inside the composite? | **No — plain `string` parts, validated by the composite itself.** A value object earns its keep by giving a rule one home and by being reusable; here nothing else in this microservice carries a resource or an action alone, and both parts are built from one shared segment rule. Two named types would be two copies of one helper for no reader. The composite therefore owns **both** halves of the concept: how a permission is validated and how it is rendered. Extracting a part later is mechanical and touches no data. **The hierarchy stayed**: the resource is still a colon-joined path (§7a rule 3) — collapsing the types never required collapsing the vocabulary |
| **Q7** | What leaves on the wire? | **`id` + `description` + `permission`.** `resource` and `action` are read into the Result to feed the derivation and stop there — the documented computed-field shape. All three stored fields remain **filterable**, since filters are declared on the Request DTO and never consult the Response. `id` is present because the by-id routes need it. The hidden pair stays orderable — the vocabulary is declared on the Request DTO, never on the Response — so the lean shape costs nothing on the read controls; see Q8 |
| **Q8** | `?orderBy=` is a two-half declaration — the `controls.orderBy` switch plus a `sort:` vocabulary on the Request DTO — and *"half a declaration is a boot failure"*. What does the listing accept? | **`sort: [Resource, Action, Description]`** — all three stored fields, both directions. The vocabulary lives on the Request DTO and never on the Response, so the lean wire shape (Q7) costs no ordering: the two hidden composite parts are orderable while still absent from every response body (verified with `omnicore-gen check` before the answer was taken). `resource` is index-backed by the partial unique index on the pair; `action` and `description` are blocking sorts, admitted deliberately because this catalog is bounded by the platform's own route literals and capped at 200 rows a page. Alternatives on the table and declined: `[Resource]` only (strict indexed-only, mirroring Tenant's option C — dropped `description`, which §9 wanted); `[Description]` only (§9 as literally written — the one unindexed choice, and it kept the group-by-resource gap); and dropping the control entirely. `id` is declarable — `sort: [ID]` is an ordinary declaration — and is deliberately not in this vocabulary |

## C. `(proposed)` picks carried into the approved model

flat storage · no derived public key · one composite `vos.PermissionKey` owning both the
validation and the rendering, with plain-string parts · a colon-joined resource path and a
single-segment action · `*` only as a whole part · reuse `vos.Description` ·
rendered-length cap · description-echoes-key rule · archive-only, one-way removal ·
PATCH-only updates · REST + GraphQL, no exports · the lean wire shape with the computed
`permission` · relational view backing · the `permission:*` gate taxonomy · no row-level
data access · the filter-operator table.

---

## What generation is expected to write by hand, and where it can refuse

Forward-looking, so a generation run can tell a declared escape from a real gap. Nothing
here is a decision being reopened; each line is a known boundary of the spec language with
the reason it exists.

**Written by hand, by design:**

| Written by hand | Why the generator cannot |
|---|---|
| `vos.PermissionKey` | `kind: manual`. The part shape is a regex, but rule 6 (`*` resource forces `*` action) is a pair-level invariant only the composite can see, and the substance checks reuse the project's shared anti-junk predicates, which are not statable as a pattern |
| The migration's partial unique index and the `Constraints` binding | The migration is a hook file. The index name is a contract written in two places — rename it there and the 409 binding silently stops matching |

**One thing to declare, and it is a declaration rather than a boundary:**

`createdAt` / `updatedAt` reach the read side through **`read.managed`**, which names the
framework-stamped columns by their fixed logical names. Listing one there returns it from
the by-id read and every listing row and makes it filterable like any other field, which is
what §9's operator table asks for — `{field: CreatedAt, ops: [gte, lte]}` is an ordinary
filter once the column is exposed. `deletedAt` stays off: archived state is reached through
`?includeArchived`, never through a timestamp filter.

**Two things that are unreachable by construction and must not be "fixed" into existence:**

- **A filter or an `?orderBy` over `permission`.** It is computed, and a computed path backs
  no column. §9's table already says so; the generator refuses it at `check` rather than at
  the framework's boot guard, which is the friendlier of the two.
- **The `?fields=` guard around the derivation.** `ComputePermission` is a pure string
  render and cannot fail, so a generic `if err != nil` around it is dead by construction.
  The generator emits the guard generically, which is right — a derivation that CAN fail
  needs it — and it is not a defect to chase in coverage.

**One constraint known to be unreachable by unit test:** the repository, the domain service
and the routes need a live relational engine and a running app. That is `/omnicore:qa`'s
territory, not a unit test's, and it is what `CLAUDE.md` rule 6's 95% floor collides with —
the maintainer's to accept or to fund with a test harness. Everything the framework's
division makes unit-testable is expected at 100%.
