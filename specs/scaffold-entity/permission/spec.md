# Spec: Permission

- **Status:** APPROVED
- **Approved:** maintainer (Cláudio Schirmer Guedes), 2026-08-19 — the four ⚠️ OPEN slots
  answered at the model gate (§B), then three refinements taken at the plan gate (§B
  Q5–Q7): the one-way archive, the collapsed value objects and the lean read payload. The
  `(proposed)` picks of §C stand
- **Language:** English (all artifacts) · Portuguese (chat) — per `../../../CLAUDE.md`
  rule 3 and the maintainer's invocation
- **Generation:** omnicore-gen

The **global catalog of enforceable permissions**. One row per `resource:action` pair that
some route in the platform actually enforces — `tenant:read` is a string literal in
`internal/web/tenant_routes.go` today, and this entity is what makes that literal a row an
operator can list, describe and grant.

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
| omnicore `v0.54.0` `/docs` | `value-objects` (the composite kind), `table-schema` (`Composite`/`As`, the boot panics, the once rule), `auto-query-handlers` (computed read fields), `authz-seams` (the permission string format and the claim-side wildcard rules) |
| `omnicore-gen explain coverage` (plugin 0.23.0) | composite value objects ✓ and computed read fields ✓ are both emitted by the generator — the 1d gateway has two real options |

### Verified framework facts that shaped this spec (read, not assumed)

| Fact | Evidence at the pin (`v0.54.0`) |
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
  resource     VARCHAR(64)   NOT NULL   -- part 1 of vos.PermissionKey
  action       VARCHAR(64)   NOT NULL   -- part 2 of vos.PermissionKey
  description  VARCHAR(500)  NOT NULL
  deleted_at   TIMESTAMPTZ   NULL       -- archive (one-way, §6)
  created_at   TIMESTAMPTZ   NOT NULL
  updated_at   TIMESTAMPTZ   NOT NULL

  UNIQUE (resource, action) WHERE deleted_at IS NULL    -- partial: ACTIVE rows only (§B Q3)
```

  One table. No FK: the catalog is global, so there is **no `tenant_id` column** — see
  `../../../README.md`, "What is scoped to a tenant, and what is not". The future
  `role_permissions` join table (a later run) points at `permissions.id`.

  **Table description** (becomes the Postgres `COMMENT ON TABLE`): *The global catalog of
  enforceable permissions. One row per resource:action pair some route enforces; defined by
  the platform, never by a tenant. The resource:action string is rendered on read, never
  stored.*

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
| 7 | the **rendered** token is at most 129 runes (64 + `:` + 64) | `InvalidPermissionKeyNotification` |

**Rule 6 is the reason this concept had to be a composite** and could not be two loose
fields: it is only expressible with both values in hand. `authz-seams.html` lists the three
claim shapes the matcher honors — exact, `resource:*`, `*:*`. `*:read` is not one of them,
so a caller granted it would match no route ever, while the catalog row reads like a
sweeping grant. The rule refuses the row instead of shipping the illusion.

Rule 7 bounds what one entry costs in a `permissions` claim.

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

**What this shape costs, accepted at the gate:** `?orderBy=` is validated against the
Response's declared wire paths, so **ordering by `resource` or `action` is a typed 400** —
they are not on the Response. `permission` was never orderable (a computed path cannot be,
by construction). What remains sortable is `description` and the id. The catalog is small
and capped at 200 rows per page, so a client that wants it grouped by resource sorts
locally. Filtering, which is what was asked for, is untouched: it comes from the Request
DTO's own tags and never consults the Response.

- **Reserved read controls served by the listing:**

| Control | Served | Why |
|---|---|---|
| pagination (`?first=`, cursor) | yes | default |
| `?orderBy=` | yes — over `description` and the id only | see the cost above |
| `?fields=` | yes | `?fields=permission` returns the strings and nothing else — exactly what a token issuer wants. Selecting it pushes `Resource` + `Action` down to the store automatically |
| `?includeArchived` | yes | a retired permission must stay auditable — and with no unarchive verb, this listing is the only way to see one |
| `?onlyTotal` | yes | cheap |
| `?search=` | **no** | the view is relational-backed: free text is answered with a typed 400 `RelationalCapabilityNotification`. `?filter[description][contains]=` covers the real need |

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
- **View backing: relational** (`.RelationalSource(repo.Loader)`) — the project posture, and
  the only backing available (no Mongo). Reads are read-your-writes: a permission created is
  visible to the very next read, with no CDC round-trip.

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
| **Q7** | What leaves on the wire? | **`id` + `description` + `permission`.** `resource` and `action` are read into the Result to feed the derivation and stop there — the documented computed-field shape. All three stored fields remain **filterable**, since filters are declared on the Request DTO and never consult the Response. Accepted cost: `?orderBy=` is Response-scoped, so ordering by resource or action is a typed 400. `id` is present because the by-id routes need it |

## C. `(proposed)` picks carried into the approved model

flat storage · no derived public key · one composite `vos.PermissionKey` owning both the
validation and the rendering, with plain-string parts · a colon-joined resource path and a
single-segment action · `*` only as a whole part · reuse `vos.Description` ·
rendered-length cap · description-echoes-key rule · archive-only, one-way removal ·
PATCH-only updates · REST + GraphQL, no exports · the lean wire shape with the computed
`permission` · relational view backing · the `permission:*` gate taxonomy · no row-level
data access · the filter-operator table.

---

## Deviations recorded at generation time

Generated on 2026-08-19 via `omnicore-gen` (the 1d gateway choice, recorded in the header),
from `../../omnicore-gen/permission.omnicore.yaml`. Everything below is a place where the
shipped code and this document do not match, or where a low-risk detail was settled during
generation. Nothing here was decided silently.

### A. The physical column names carry a `_name` suffix

`§1`'s ER sketch names the columns `resource` and `action`. The generator refuses
`resource` outright — it is a **reserved word on oracle**, and identifiers are emitted
unquoted in places it does not control. The pair was renamed together rather than left
asymmetric, so the DDL reads `resource_name` / `action_name`.

**Nothing outside the DDL moved.** The parts' EXPOSED names are untouched, so every filter,
OpenAPI parameter, GraphQL argument and audit entry still says `resource` and `action` —
which is the whole point of the composite's exposed-name layer. Read `§1` with the two
column names substituted; every other statement in it stands.

### B. Uniqueness over the PAIR is not generated — it ships in two hand-written halves

`§7c` rule 8 asked for `enforce: service-precheck+constraint` over the `(Resource, Action)`
pair. `fields[].unique` is **single-column by construction** and this build refuses it over
a composite value object: *"uniqueness over a composite value object is not generated — it
would need a multi-column constraint, and none is emitted"*.

The requirement was split across the two halves it was always made of, and **both shipped**:

| Half | Where it lives | Generated? |
|---|---|---|
| the pre-check (reports the duplicate *together with* other validation errors) | service fact `PermissionKeyTaken` + rule `permission-key-unique-among-active` in `permission_rules_manual.go` | fact yes, rule by hand |
| the race backstop (the arbiter under concurrency) | the partial unique index `permissions_resource_name_action_name_key … WHERE deleted_at IS NULL` | by hand, in the migration — which was always hand-owned |

**CLOSED, by adoption — the maintainer's call, 2026-08-19.** The repository's `Constraints`
map is an *owned* generated file that bound only `permissions_pkey`; nothing in the spec
language can add a second binding, and the framework accepts one only there. Left alone, a
genuine race — two simultaneous inserts of the same pair, both passing the pre-check —
would see the index refuse the second row and the caller read a **raw 500**.
`internal/infra/permission_repository.go` was therefore adopted
(`omnicore-gen adopt … -why …`) and binds
`permissions_resource_name_action_name_key` → `PermissionAlreadyExistsNotification`, so the
race answers with the same 409 the pre-check gives. **The price is permanent**: that file no
longer tracks the spec, and future emitter improvements will not reach it. The index NAME is
now a contract written in two places — rename it in the migration and the binding silently
stops matching.

### C. Key immutability moved from the declarative list to `rules.manual`

`§7c` rule 9 is a `kind: immutable`. The generator refuses that kind over a composite —
*"the entity's rules are checked against the entity's own fields"* — and its fix line points
at `rules.manual`, "where the composite is in hand as a whole". It shipped there, guarded on
the pre-write snapshot being non-nil, and its behaviour is exactly what `§7c` describes. The
rule list is consequently **empty**: all three of this entity's invariants are hand-written,
each for its own reason.

### D. `createdAt` / `updatedAt` are not filterable

`§9` asked for `gte` / `lte` on both. This build serves filters only over declared entity
fields, and the framework-managed timestamps are not among them — **the same deviation the
Tenant run recorded**, unchanged and reported upstream there.

### E. The archive endpoint's generated OpenAPI text claims an undo that does not exist

`internal/web/permission_routes.go` documents the archive route as *"Archive a permission
(reversible) … Reversible through unarchive."* — a fixed string the archive emitter writes
regardless of whether `unarchive` is among the entity's `modes`. It is **not** here (`§B`
Q5), and no unarchive route, mutation or command was generated: the code was right and only
the documentation lied.

**CLOSED, by adoption — the maintainer's call, 2026-08-19.** No spec key controls
per-operation summaries, so `internal/web/permission_routes.go` was adopted and the archive
route now reads *"Archive a permission (one-way)"*, with a description that says why there is
no undo and how a retired permission comes back. **The price is permanent, and higher here
than on B**: the routes file is the one that moves most when the spec moves, and it no longer
tracks it.

### G. The WRITE responses carry `id` + `description`, but not `permission`

`§9` promised the same three values on *every* read **and write** response. The reads
deliver all three; `POST /permissions` and `PATCH /permissions/{id}` answer with `id` and
`description` only.

Both halves of the cause are decisions this spec made, and they meet here:

- `read.computed` is, by name, the READ side's mechanism — it fills a query Result in
  `FromQueryResult`. There is no write-side counterpart, so a command Result has no way to
  declare a derived field;
- `hidden: true` on the key (the `§B` Q7 lean shape) removes `Resource` and `Action` from
  the write responses too, exactly as documented — *"the by-id read, each row of the
  listing, the write responses, and the CSV/XLSX exports"*.

So the caller gets back neither the parts nor the rendering. **The practical cost is small**
— a caller who just POSTed the pair knows what it is, and one `GET /permissions/{id}`
returns the rendered string — but it is a real gap against `§9` as written, not a
re-reading of it. Dropping `hidden` would fix the write response and break the lean read
shape `§B` Q7 chose deliberately; there is no third option in the spec language today.
Recorded rather than quietly re-interpreted.

### F. Coverage is 75.3% for the entity, against `CLAUDE.md` rule 6's 95% floor

Every file the framework's division makes unit-testable is at **100%** — including the three
hand-written ones, which the generator does not test and which carry this entity's whole
substance:

| File | Coverage |
|---|---|
| `internal/domain/vos/permission_key.go` | 100.0% (47/47) |
| `internal/domain/permission_rules_manual.go` | 100.0% (15/15) |
| `internal/application/queries/permission_computed_manual.go` | 100.0% (7/7) |
| every generated command, query, request, schema, view and the aggregate | 100.0% |
| `internal/infra/permission_repository.go` · `permission_service.go` · `internal/web/permission_routes.go` | **0.0%** (37 statements) |

The 0% trio needs a live engine and a running app, which is `/omnicore:qa`'s territory and
not a unit test's. This is the **same shape** the Tenant run recorded as its deviation E,
and it remains OPEN.
