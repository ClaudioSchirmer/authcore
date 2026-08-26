# authcore-service

**Identity and authentication for a multi-tenant platform.** It owns who the tenants are,
who the users are, and what the tokens the rest of the platform trusts actually say.

The tenant registry is the foundation the rest is built on. A tenant carries **three
identifiers with one job each**, and telling them apart is the first thing to understand
here:

| Identifier | Value | Who sees it |
|---|---|---|
| `id` | UUID v7, minted by the framework | **the key**: the `tenant_id` token claim, the foreign-key target of every tenant-scoped aggregate, and the management API's by-id URLs |
| `workspace` | `acme-comercio` | the human-facing handle — URLs, logs, support |

So `tenant_id` means exactly one value everywhere it appears — column, claim and foreign
key. Every other service scopes its data by reading that claim off the token instead of
asking this one.

An earlier model carried a **third** identifier beside these: a `UUIDv5` derived from the
workspace, kept so the row id would never leave the service. It was removed on 2026-08-24,
because it could not deliver what it existed for. A tenant's id is stored inside every OTHER
microservice that references it — in their tables, their logs and their own APIs — so it is
public in practice whatever this service calls it. The second key bought a translation step
on every isolation filter, a foreign key whose target nobody guessed right the first time,
and a standing "which of these two UUIDs is this?" question, in exchange for hiding a
creation timestamp. One key is the better trade.

Go module: `github.com/ClaudioSchirmer/authcore` · Go 1.26.5 · built on
[omnicore](https://github.com/ClaudioSchirmer/omnicore) **v0.60.0** (DDD + CQRS framework).

---

## Current state

Honest scope, so nobody reads intent as delivery:

| Capability | State |
|---|---|
| Tenant registry (create, read, patch, archive/unarchive, REST + GraphQL) | **built** — six REST endpoints and the matching GraphQL queries/mutations, generated from `specs/omnicore-gen/tenant.omnicore.yaml` against the model in `specs/scaffold-entity/tenant/spec.md`. Build, vet and the unit suite are green, and the service has been booted against Postgres with a tenant registered through the API. The contract suite (`/omnicore:qa`) is still to come |
| `User` entity | **built** — five REST endpoints (insert · patch · archive · by-id · listing), four collection ops (join/leave a group · grant/revoke a role), **one credential operation** (password reset) and the matching GraphQL queries/mutations, generated from `specs/omnicore-gen/user.omnicore.yaml` against the model in `specs/scaffold-entity/user/spec.md`. Build, vet and the unit suite are green; the contract suite (`/omnicore:qa`) and a boot against Postgres are still to come |
| User ↔ tenant association | **built** — `users.tenant_id` NOT NULL, FK to `tenants.id`, filled from the caller's `tenant_id` claim and nameable in the body only by a `*:*` operator crossing the scope |
| Self-service password change | **not started** — and the endpoint that briefly pretended to be it was removed. It needs a forgot-password flow (e-mail, expiring link) or an authenticated change carrying the current password; neither exists |
| User ↔ group / User → role membership | **built** — two owned collections with per-entry join/leave and grant/revoke, both gated on `user:grant` and both refusing an escalation the caller does not already hold |
| `Permission` entity | **built** — five REST endpoints (insert · patch · archive · by-id · listing) and the matching GraphQL queries/mutations, generated from `specs/omnicore-gen/permission.omnicore.yaml` against the model in `specs/scaffold-entity/permission/spec.md`. Build, vet and the unit suite are green; the contract suite (`/omnicore:qa`) and a boot against Postgres are still to come |
| `Role` entity | **built** — five REST endpoints (insert · patch · archive · by-id · listing) plus the two child ops (grant · revoke) and the matching GraphQL queries/mutations, generated from `specs/omnicore-gen/role.omnicore.yaml` against the model in `specs/scaffold-entity/role/spec.md`. Build, vet and the unit suite are green; the contract suite (`/omnicore:qa`) and a boot against Postgres are still to come |
| `Group` entity | **built** — five REST endpoints plus the two collection ops (attach · detach), gated on `group:grant`; model in `specs/scaffold-entity/group/spec.md`. *(This row read "specified, not built" until 2026-08-26 — it was stale from the moment the entity merged.)* |
| Effective-permission resolution (group path ∪ direct path) | not started — **and it is now the next thing worth building.** Both arrows exist in the data; nothing yet walks them into one set. It is what turns this model into a token |
| Reserved platform tenant | not started — and **two** entities now DEPEND on it. `Role`: no wildcard permission can be granted through the API, so the platform's own `*:*` role has to be seeded by migration beside that tenant. `Group`: no wildcard-bearing role can be attached to a group through the API either, so the platform's own super-admin **group** has to be seeded in that same migration |
| Token issuance with the `tenant_id` claim | not started — the value it must carry is `tenants.id`. Nothing in the code enforces that yet. **The subject it would mint for now exists**; the minting path does not |
| Commercial status (`trial` / `active` / `suspended`) | **built and enforced** on Tenant — the transition machine refuses any return to `trial`, and archiving forces `suspended`. Nothing downstream consumes it yet |
| Contract QA suite (`/omnicore:qa`) | not generated |
| Generated code | **All five aggregates.** `internal/` holds Tenant, Permission, Role, Group and User end to end — domain, application, web, infra, migrations `0001` to `0005`, wiring and the seven catalogs. What is NOT generated, and could not be, is the credential path: the password hasher, the two credential operations and the rules behind them are hand-written, and `specs/omnicore-gen/user.gen-report.md` lists every piece |
| Permission enforcement in production | `tenant:read` · `:insert` · `:update` · `:archive` and `permission:read` · `:insert` · `:update` · `:archive` now gate the built routes for real, on REST and GraphQL alike; `role:read` · `:insert` · `:update` · `:archive` now gate the built role routes too, the two child ops riding `role:update`; `group:*` (**five** verbs) and `user:*` (**seven** — `read` · `insert` · `update` · `archive` · `grant` · `reset-password` · `change-password`) gate their built routes too. `auth.mode` is `disabled` in dev and `jwt` in prd. The literals have **no catalog row until an operator inserts one** — see the seeding note below, and note that `group:grant`, `user:grant`, `user:reset-password` and `user:change-password` are the four nobody will guess from the pattern — and that `user:change-password` is the one that must reach EVERY user, since without it a caller cannot set their own password at all. **Every route in the service declares a permission** — the framework refuses to boot otherwise once `auth.authorization` is on |

## Architecture posture

Real PostgreSQL as the source of truth, **no CDC pipeline**. Both profiles declare it
identically, and the absence is the declaration: no `mongo:` block and no `transport:`
block.

What that buys and what it costs:

- **Reads are served straight from the tables** — `query.RelationalView("<name>",
  repo.Loader)`, contributed through the feature's `RelationalViews()` opt-in — so a write is
  visible to the very next read: no eventual consistency, no waiting on a projection, and no
  `Version`, registry row, rebuild or Mongo collection behind the view.
- **Free-text search is not served.** A relational-backed view answers `?search=` with a
  typed 400 rather than pretending; filters and sorts over 1:1-reachable fields work
  normally.
- **Integration events cannot be published** — publishing rides the CDC relay, which does
  not exist here. Consuming another service's events would only need a broker and the
  transport build tag.
- Multi-source read models (ComposedView, SharedBaseView, the Embed/Link family) need
  Mongo and are therefore unavailable.

All of it is reversible without losing application code — `/omnicore:configure` converts
the posture in one pass.

## Domain model

### The shape we are building towards

**Every node and every arrow below now exists in code.** What does not exist yet is the
walk itself — nothing resolves a user into one set of effective permissions, and nothing
mints a token from it. The picture is the destination, and it is now also the schema:

```
                        ┌──> Group ──> Role ──> Permission     (inherited, via group)
User ───────────────────┤
 │                      └──────────> Role ──> Permission       (granted directly)
 │
 └── tenant_id ──> Tenant                                      (the partition it lives in)
```

A user's **effective permissions** are the union of both paths — the roles reached through
their groups, plus the roles granted to them directly. **Both paths are stored and served;
neither is resolved.** `GET /users/{id}` answers with the groups and the direct roles, each
carrying its key and label, and walking those into a permission set is the piece that comes
next. There is no precedence and no deny
rule: a permission is held or it is not.

Every arrow between `User`, `Group`, `Role` and `Permission` is many-to-many. The one
arrow that is not is `tenant_id`: a user belongs to exactly **one** tenant, and the same
person operating in two tenants is two users. That is what keeps `tenant_id` a single
scalar claim on the JWT instead of a list the consuming services would have to
disambiguate.

#### One e-mail, one user

`email` is unique **across the whole platform**, not per tenant:

```sql
CREATE UNIQUE INDEX users_email_key ON users (email) WHERE deleted_at IS NULL;
```

The index is the point, not a detail of it. Scoping uniqueness to the tenant would let one
address exist as two users — and since the credential (password, e-mail verification, MFA)
hangs off the user row, that person would carry two passwords, enrol MFA twice, and a
password reset would repair one login while the other stayed broken. Worst of all, locking
the account would close one door and leave the other open.

A global index makes that unrepresentable. It also imposes a rule rather than hoping for
one: a person present in two tenants reaches them through two **corporate** addresses
(`maria@acme.com`, `maria@globex.com`), which is honest — those really are two employment
relationships, not one identity split in half.

The price, stated so it is not discovered later: someone with a single personal address
can belong to exactly one tenant, forever. A freelancer serving a second customer on this
platform needs a second address. That is acceptable while tenants are companies with their
own domain; if self-employed customers ever become normal here, the answer is not to
loosen this index but to split the credential from the membership — a global `Identity`
holding e-mail, password and MFA, with `User` demoted to the per-tenant link. Recorded as
the escape hatch, not as a plan.

**The address is immutable.** A user cannot change their e-mail through this API at all —
it is absent from the patch body, not merely refused by a rule — so the login handle, the
global unique key is (the lookup the removed public change-password route resolved by was the same one) — all one
value that never moves. The cost is stated where it lands: correcting a typo means
archiving the user and creating them again, which loses every group and role they had.

Uniqueness is over **active** rows: archiving a user releases the address, so a company
that reassigns `joao@acme.com` to a new hire can register them. Unlike the tenant workspace,
an e-mail is not a durable reference — tokens and audit rows point at the user's UUID.

#### What is scoped to a tenant, and what is not

| Entity | Scope | Who defines it |
|---|---|---|
| `Tenant` | — | the platform |
| `User` | `tenant_id` **NOT NULL** · `email` unique **globally** | the tenant |
| `Group` | `tenant_id` **NOT NULL** | the tenant — it mirrors the customer's org structure |
| `Role` | `tenant_id` **NOT NULL** | the tenant — its own cut of the permissions |
| `Permission` | **global catalog** | the platform |

`Permission` is deliberately *not* tenant-scoped. A permission exists only because some
route enforces it — `tenant:read` is a string literal in `internal/web/tenant_routes.go`,
not a row a customer invented. Letting a tenant create permissions would produce rows no
code ever consults, so the catalog is global and read-only to tenants.

**Nothing seeds it.** The migration creates the table empty and an operator populates it
through the API; there is no seed script and no fixture, because a migration that invents
grants is a migration that grants power nobody reviewed. The consequence is worth stating
plainly: the service ships gating eight literals — four `tenant:*`, four `permission:*` —
that have no catalog row until somebody inserts one. The catalog documents what the code
enforces; it does not control it.

#### Where the platform operators live

`tenant:insert` is a platform operation, so the user who holds it belongs to no customer.
Rather than make `tenant_id` nullable — which would force `OR tenant_id IS NULL` into
every scoped query forever — the platform gets a **reserved tenant** of its own, seeded by
migration. Platform power then follows from which roles that tenant carries, not from a
special-cased `NULL`, and the "exactly one tenant per user" invariant holds with no
exception anywhere.

### Tenant

An isolation partition. Flat aggregate, table `tenants`. Its approved model, with the
alternatives that were rejected and why, is in `specs/scaffold-entity/tenant/spec.md`.

| Field | Type | Notes |
|---|---|---|
| `id` | UUID v7 | the row id, and the tenant's only identifier: the `tenant_id` token claim carries it and every tenant-scoped foreign key targets it |
| `name` | string(120) | display name. **Not unique** — two customers may legitimately share a trade name |
| `workspace` | string(63) | the handle. Unique across **all** rows, archived included; immutable after creation |
| `description` | string(500) | required, and validated for substance |
| `status` | enum | `trial` · `active` · `suspended`. Mandatory on create, no default |

#### The three asymmetries worth knowing before you touch them

- **The workspace is reserved forever.** It reaches URLs, logs, bookmarks and external
  configuration, so re-issuing a handle would make a new tenant answer to a retired
  tenant's history — support threads, dashboards and integrations all pointing at the wrong
  customer, with nothing to detect. Its unique index is therefore total, never partial.
- **The name is not unique at all.** No large multi-tenant product makes the display name
  unique — only the handle. Operators tell two "Acme" rows apart by the workspace beside
  the name.
- **Suspension is not archiving.** Archiving is *removal* — the row leaves every default
  read. Suspension is *commercial state* — the row stays listed and keeps authenticating
  for billing while being blocked in the product. Using archive to suspend a delinquent
  customer would hide them from the very reports collections needs.

Business rules enforced by the domain (`specs/scaffold-entity/tenant/spec.md` §7 is the full
table):

| Field | Rules |
|---|---|
| `name` | 2–120 characters · at least one letter · at least min(3, length) distinct characters · no run of 4 or more identical characters · no leading/trailing whitespace and no double space |
| `workspace` | 3–63 characters · `^[a-z0-9]+(-[a-z0-9]+)*$` · at least 3 distinct characters · no run of 4 or more identical · not on the 51-entry reserved list · unique across all rows · immutable · **no input normalization** — `" Acme "` and `"ACME-CORP"` are refused, never silently repaired |
| `description` | 15–500 characters · at least two words · at least 5 distinct characters · no run of 4 or more identical · at least one vowel · must differ from the name and the workspace under a normalized comparison |
| `status` | a declared member · transitions `trial→active`, `trial→suspended`, `active→suspended`, `suspended→active` and no-ops only — `active→trial` and `suspended→trial` are refused |

Every bound counts **runes, not bytes**, and "letter", "word" and "vowel" are Unicode, not
ASCII: this service ships seven translation catalogs, and a byte-based bound is wrong by
two characters for a name like "Acme Comércio e Serviços Ltda" while an ASCII vowel test
would reject a description written in Japanese or Arabic as keyboard junk. The predicates
live in `internal/domain/vos/text_predicates.go` and are shared with every future entity.

**Known limitation**, found by testing those predicates against all seven languages and
left in deliberately: the two-word rule on `description` is unsatisfiable in languages
that do not separate words with spaces — Japanese, Chinese, Thai, Lao, Khmer. A complete
Japanese sentence counts as one word and is refused. Arabic and Cyrillic are unaffected.

Removal is **archive only** — there is no `DELETE`. Users and issued tokens will carry
its id, and an irreversible purge would orphan them; it would also erase the row that
*reserves* the workspace, handing that public key to the next registration. **Archiving
forces the status to `suspended`**, which makes archived-and-active an unrepresentable
state rather than a refused one, and unarchiving therefore returns the tenant suspended —
reactivation is always a separate, explicit, separately audited act.

### Permission

The global catalog of enforceable permissions. Flat aggregate, table `permissions`. Its
approved model, with the alternatives that were rejected and why, is in
`specs/scaffold-entity/permission/spec.md`.

| Field | Type | Notes |
|---|---|---|
| `id` | UUID v7 | the row id. Internal, but **returned** — a caller needs it to patch or archive the row |
| `resource` | string(64) | what is protected. A slug, or a colon-joined path (`user:profile`); `*` means every resource. Stored in `resource_name` |
| `action` | string(64) | what may be done to it. Exactly one slug, never a path; `*` means every action. Stored in `action_name` |
| `description` | string(500) | required, and validated for substance — it must explain the permission, not repeat it |
| `permission` | string | **not a column.** `resource:action`, rendered on read |
| `createdAt` / `updatedAt` | timestamp | framework-stamped, returned on every read and filterable by range. `deletedAt` is not exposed — archived rows are reached through `?includeArchived` |

Three things a reader will otherwise get wrong.

**1. The permission string is rendered, never stored.** Three columns go in, two values come
out: `resource` and `action` are stored apart so each can be filtered and ordered on its own,
and neither is ever returned. What the API returns is `permission` — the exact string a JWT
claim carries and `RequirePermission(...)` compares against — built by one method,
`vos.PermissionKey.String()`, which is the only place in this service that knows the
separator is a colon. Filtering is unaffected: a caller looking for `tenant:read` sends
`?filter[resource][eq]=tenant&filter[action][eq]=read`, because filters are declared on the
request and never consult the response.

**2. The resource and the action are frozen after creation.** Only `description` is
editable. The pair IS the permission's identity everywhere except this table — the string in
the token and the literal in the route — so editing it would rewrite the meaning of every
existing grant, retroactively and invisibly.

**3. Archive is one-way.** There is no unarchive verb, deliberately. Restoring a retired row
would re-enable, in a single call, every grant still pointing at it: users would silently
regain a permission nobody re-approved, and the audit trail would read as a restore rather
than a grant. A retired permission comes back as a **new row, with a new id**, which must be
granted explicitly. The unique index over `(resource, action)` is scoped to the ACTIVE rows
precisely so that this is possible — an archived `tenant:export` does not block a fresh one.
Removal is `PATCH /permissions/:id/archive`; there is no `DELETE`, because purging the row
would destroy the only human-readable record of what a past grant meant.

#### Wildcards

A catalog row may carry `*` on either part, and `*:*` is the super-admin row. Two rules
govern them, and both come from the framework's claim matcher, which honors exactly three
shapes — an exact string, `resource:*`, and `*:*`:

- **A wildcard resource requires a wildcard action.** `*:read` is refused. It fits none of
  the three shapes, so it would be a row that matches no route while reading like a sweeping
  grant.
- **`*` is legal only as an ENTIRE part** — never as a segment inside a path (`user:*`),
  never mixed into a slug (`ten*`).

A wildcard row is **grantable but never enforceable**: no route may declare
`RequirePermission("tenant:*")` — a caller-side wildcard reaching the route side is a runtime
panic. So the catalog holds two kinds of row: the ones that mirror a literal in the code, and
the wildcard ones that only ever appear in grants. Nothing in the schema distinguishes them,
and nothing needs to. The cost is the nature of a wildcard rather than a defect: a role
granted `tenant:*` holds every permission that will ever exist for `tenant`, including ones a
future release adds that nobody reviewed the grant for.

### Role

A tenant's own cut of the global permission catalog. `Permission` says what the platform can
enforce; `Role` says which of those a given customer has bundled together and hands out.
Flat aggregate, table `roles`, with one owned collection, `role_permissions`. Its approved
model, with the alternatives that were rejected and why, is in
`specs/scaffold-entity/role/spec.md`.

It is the **first aggregate in this service owned by a tenant** rather than by the platform,
which is what every authorization note below exists for.

| Field | Type | Notes |
|---|---|---|
| `id` | UUID v7 | the row id. Internal, but **returned** — a caller needs it to patch, archive or grant |
| `tenantID` | UUID | the owning tenant. Immutable. FK to `tenants.id` — the tenant's only identifier, and the value the `tenant_id` token claim carries |
| `key` | string(64) | the stable machine handle (`billing-manager`). Immutable, and unique **per tenant**, not globally. Stored in `role_key` |
| `name` | string(120) | the display name. Not unique — two tenants, or two roles, may share a label |
| `description` | string(500) | required, and validated for substance |
| `permissions[]` | collection | the grants. Each entry carries `id`, `permissionID` and the rendered `permission` token — the last one derived from two columns read across the foreign key |
| `tenantWorkspace` · `tenantStatus` | string | the owning tenant's handle and commercial state, **read across the foreign key**. Read-only, absent from every write body, and — unlike a grant's fields — filterable and sortable, because a ROOT join's fields are addressable in a criteria |

Five things a reader will otherwise get wrong.

**1. The read returns the permission, not just its id — without storing a copy of it.**
Both `GET /roles/{id}` and the listing `GET /roles` answer
`"permissions": [{ "id": "…", "permissionID": "9f14b0a2-…", "permission": "tenant:read" }]`.
The client reads ONE token, with no second call to `GET /permissions` and no two halves to
join — the same shape the catalog's own endpoint serves.

The row still holds **only the id**. `resource` and `action` are a **read join** — declared
once on the repository, traversed at load time, never written — and they are `hidden`: they
feed the rendered token and reach no response body, no listing row and no export. That is
the whole point: denormalizing the `resource:action` pair into each grant was rejected,
because a retired permission comes back as a **new row with a new id**, so a grant holding
the *string* would silently re-attach to the recreated row. A grant holding the **id**
cannot, and reading the string across the FK costs nothing that a copy would cost.

The traversal also reports nothing about the catalog row's own state. An earlier build
surfaced the permission's archive stamp here; it was removed on 2026-08-24 because nothing
consumed it and because `Permission` deliberately keeps `deletedAt` off its own reads —
republishing the same column through a join was a back door to a decision the owning
aggregate had already made the other way.

**1b. A role also carries its tenant's handle and status, by the same mechanism.**
`tenantWorkspace` and `tenantStatus` are a root join into `Tenant`, so a listing shows which
customer a role belongs to without a second call. Two consequences worth knowing:

- **They are filterable and sortable**, unlike anything inside `permissions[]`. A root
  join's fields are addressable in a criteria; a child join's are not, because narrowing a
  root by a field of a 1:N collection is a pushdown one root `SELECT` cannot express.
- **`tenantStatus` is a plain string, never the `TenantStatus` enum.** A join field carries
  no domain type: the value belongs to `Tenant`, arrives read-only and is never validated
  here, so reconstructing the enum would hand back an instance no rule of the owning
  aggregate ever approved.

This traversal became expressible only when the derived tenant key was removed. A join's
predicate is always `fk = target.id`, and while `roles.tenant_id` pointed at a second,
derived column, the declaration would have been accepted and matched nothing.

**2. You can only grant what you hold.** A caller may add a permission to a role only if
their own token carries it — the standard defence against a tenant admin minting themselves
more power than they have. A `*:*` super-admin is exempt **by construction rather than by a
special case**: the framework's `HasPermission` already answers true for any concrete
permission when the claim set contains `*:*`, so the rule needs no branch and no claim
parsing of its own.

**3. No wildcard can be granted through the API.** A permission carrying `*` in either part
is refused on every role, for everyone. That is partly a policy and partly a safety
interlock: `Identity.HasPermission` **panics** on any argument containing `*`, so the naive
form of rule 2 — ask `HasPermission` for each granted key — would crash into a 500 on exactly
the `*:*` row the rule exists to stop. Refusing wildcards first means no wildcard string ever
reaches that call. The consequence is deliberate and has a home: the platform's own `*:*`
role is **seeded by migration** beside the reserved platform tenant, not created through this
API.

**4. Archive is one-way here too**, and for the same reason it is one-way on the catalog. A
role is granted to users and groups, and those grants point at the role's id, which does not
change — so a single `PATCH /roles/{id}/unarchive` would silently re-authorize everyone still
holding it, with no re-approval and an audit line reading "restored". A retired role comes
back as a new row that must be granted again. Same rule one level down: revoking a grant is
`PATCH /roles/{id}/permissions/{entryId}/archive`, never `DELETE`, because the row lingers
with a `deleted_at` stamp and a `DELETE` that soft-removes is a lying contract. There is no
`DELETE` anywhere on this aggregate: purging a role would destroy the only human-readable
record of what a past grant meant, which is exactly what an access review needs to read.

**5. You cannot filter or sort by a granted permission.**
`?filter[permissions.resource][eq]=tenant` is a typed 400, not a result — and so is the same
path on `permissionID`. A read join **renders** a child's counterpart; it does not make it
addressable. "Which roles grant `tenant:read`?" is therefore not answerable from this
listing: a relational-served view carries the collection in the document it returns, but a
filter on a field inside it is a pushdown a single root `SELECT` cannot express. That is the
1:N boundary, not the backing. It becomes answerable the day the service gains Mongo
(`/omnicore:configure`), with no change to this model. What the join already gives you is the
other direction — once you have the roles, you can see what each grants without a second
call.

#### Tenant isolation

Every row is tenant-scoped, on reads **and** on writes:

- **Reads.** The listing injects the caller's `tenant_id` claim as a filter, so it returns
  only their own roles, and a by-id read of another tenant's role answers **404 rather than
  403** — it does not exist for that caller, which leaks nothing about who else exists.
- **Writes.** Creating, patching or archiving a row whose `tenantID` is not the caller's is
  refused with 403. A `*:*` super-admin crosses the scope in both directions, which is what
  lets a platform operator support a customer.
- **An absent identity is not an absent claim.** With no identity at all the scope stands
  down — that state is reachable only with `auth.mode: disabled`, which the framework refuses
  outside `APP_PROFILE=dev`, so the dev bench stays usable. A real signed token that simply
  carries no `tenant_id` claim is an ordinary production request and is still **refused**.

All of this is generated and correct today, and **inert until `auth.authorization` is turned
on** — neither profile configures it yet, so `RequirePermission` currently no-ops across the
whole service. The rules fail closed the moment it is switched on. Flipping it is a
service-wide posture change, owned by `/omnicore:configure`.

### Group

A tenant's own org unit, and the bundle of roles its members inherit by belonging to it. It
is the **second node** of `User → Group → Role → Permission` — the inherited half of a
user's effective permissions, where the direct `User → Role` path is the other. Flat
aggregate, table `groups`, with one owned collection, `group_roles`. Its approved model —
including a survey of what AWS IAM, Entra ID, Okta, Keycloak and Google Cloud actually do
with groups, and the alternatives that were rejected — is in
`specs/scaffold-entity/group/spec.md`.

Structurally it is **`Role` one level up**, and deliberately so. Four things are not a copy
of it, and three of them are the interesting part.

| Field | Type | Notes |
|---|---|---|
| `id` | UUID v7 | the row id. Internal, but **returned** — a caller needs it to patch, archive or attach |
| `tenantID` | UUID | the owning tenant. Immutable. FK to `tenants.id` — the tenant's only identifier, and the value the `tenant_id` token claim carries |
| `key` | string(64) | the stable machine handle (`engineering`). Immutable, and unique **per tenant**, not globally. Stored in `group_key` |
| `name` | string(120) | the display name. Not unique — two groups in one tenant may share a label; the `key` is what disambiguates |
| `description` | string(500) | required, and validated for substance |
| `roles[]` | collection | the bundle. Each entry carries `id`, `roleID`, and the role's own `roleKey`, `roleName` and `archivedAt`, read across the foreign key |

**1. The read returns the role, not just its id.** `GET /groups/{id}` answers
`"roles": [{ "id": "…", "roleID": "7c2e9b41-…", "roleKey": "billing-manager",
"roleName": "Billing Manager", "archivedAt": null }]` — a read join, exactly as on `Role`
one level down. The entry still stores only the id, because an entry holding the role's
*key* would silently re-attach to a retired-and-recreated role and one holding the **id**
cannot. A non-null `archivedAt` says the group still confers a role the tenant has retired.

> **Open, for whoever builds this.** `Role` carried the equivalent field on its own grants
> and it was **removed** on 2026-08-24 — nothing consumed it, and it republished a column
> its owning aggregate deliberately keeps off its own reads. `Group`'s model still declares
> it because that spec was approved before the removal and has not been re-opened. Decide it
> at the gate rather than inheriting it.

So the forward walk is two complete reads: `GET /groups/{id}` names the roles,
`GET /roles/{id}` names the permissions. Neither needs a third listing joined by hand.

**2. The collection verbs are a pair, not the usual trio.** ATTACH and DETACH, no "change
this entry": its single field IS its identity, so an edit would keep one row id while
changing what it means, which an audit trail reads as one grant *becoming* another instead of
as two events.

**3. `group:grant` is a fifth verb, and this is where the taxonomy diverges from `Role`'s.**
The two collection routes do **not** ride `group:update`. `Role` weighed a `role:grant` and
declined it to keep four verbs per resource; `Group` takes it, because the group→role edge
reaches further — a group hands a member every permission of every role it carries. Entra
guards a role-assignable group behind Privileged Role Administrator rather than Groups
Administrator, and AWS spells `iam:AttachGroupPolicy` apart from `iam:UpdateGroup`. So a
principal with `group:update` may rename and re-describe a group and gets **403** on both
collection verbs, and a principal with only `group:grant` may attach and detach on a group
they cannot rename. Named honestly: this is a catalog row somebody has to insert and a grant
somebody has to make, and until they do, nobody can attach a role to a group once
`auth.authorization` is on. That is the correct failure direction for an escalation surface,
and it must not be forgotten when the reserved-platform-tenant seeding work happens.

**4. No escalation, and the check is TRANSITIVE.** A caller may attach a role only if they
hold **every** permission that role grants — a set, not one key, because a role is an
indirection. And as on `Role`, no **wildcard-bearing** role can be attached through the API
at all: partly policy, partly the safety interlock, since `Identity.HasPermission` panics on
any argument containing `*`. The two are a pair and the order is load-bearing — wildcards are
refused first, so no wildcard string ever reaches that call. `group:grant` does not replace
this: Layer 1 asks "may this principal touch the edge at all", the rule asks "may they confer
*this* role", and holding the permission still does not let you attach a role you do not
fully hold.

**5. An attached role must be available IN YOUR TENANT — and the message deliberately will
not say which of three things went wrong.** Absent from the catalog, archived, or owned by
another tenant all answer with the same *"This role is not available in your tenant, or is no
longer active."* The tenant half is the question `Role` never had to ask (`Permission` is a
global catalog), and separating the three would confirm to a caller in tenant A that a
specific UUID is a live role in some other tenant — an existence oracle over a competitor's
org chart. It is the same reasoning that makes a cross-tenant by-id read answer 404 rather
than 403.

**6. Only what a write ATTACHES is judged**, never what is already stored. A role the tenant
archives *after* it was attached does not make the group impossible to rename, and a caller
who has since lost a permission can still **detach** the others — which is the tool for
fixing exactly that situation.

**7. Archive is one-way here too, and it hurts more than it does on `Role`.** A group is
granted to users, and those memberships point at the group's id — so a single
`PATCH /groups/{id}/unarchive` would silently re-authorize **an entire team at once**, with
no re-approval and an audit line reading "restored". A retired group comes back as a new row
whose members must be re-added. Entra soft-deletes a group with a 30-day restore window
precisely because losing a 50-member group to a fat-finger is brutal; this model's answer is
"create it again", and since `User ↔ Group` membership does not exist yet, nobody has been
hurt by it yet either. Same rule one level down: detaching is
`PATCH /groups/{id}/roles/{entryId}/archive`, never `DELETE`.

**8. You cannot filter or sort by an attached role.**
`?filter[roles.roleKey][eq]=…` is a typed 400 — the join renders the role, it does not make
it addressable. **"Which groups confer role X?" is not answerable from this listing** — and
it is a question an access review asks more often than `Role`'s equivalent, because it is how
you find out who a role actually reaches. Only the REVERSE direction is missing; the forward
walk arrives complete. It becomes answerable the day the service gains Mongo
(`/omnicore:configure`), with no change to this model.

**Nesting is refused, and that is a decision rather than an omission.** Keycloak nests and
Entra nests, but AWS IAM and Okta both refuse outright — and Entra, the one platform that
both nests *and* treats group→role as a security boundary, **forbids nesting on
role-assignable groups**. Nesting turns "what can this user do?" into a graph walk with cycle
detection, on the exact path the token issuer runs on every login. "Engineering ⊃ Platform"
is two groups and two memberships, with no graph.

**Membership is out of scope, by design.** `User ↔ Group` is its own aggregate, later. This
entity is the *definition* plus the role bundle.

#### Tenant isolation

Identical to `Role`'s, and applied unchanged — reads filtered by the claim with a by-id read
of another tenant's group answering **404 rather than 403**; writes refused with 403; a `*:*`
super-admin crossing in both directions; and an absent identity distinguished from an absent
claim. The rejected alternative is worth naming here specifically: reads-open isolation would
leak each customer's org chart to every other customer, and a group listing **is** the org
chart.

### User

A person's account inside exactly one tenant: the thing a token is minted for, the thing
`tenant_id` is read off, and the **only place in this service that holds a credential**. It
is the first node of `User → Group → Role → Permission` and the last one built. Flat
aggregate, table `users`, with two owned collections — `user_groups` and `user_roles`. Its
approved model, with the alternatives that were rejected and why, is in
`specs/scaffold-entity/user/spec.md`.

Structurally it is **`Group` with two collections instead of one, plus a secret**. The
tenant-owned flat root, the id-only child rows with the counterpart read across the foreign
key, the per-child pair of verbs, the one-way archive and the tenant isolation are all
inherited unchanged. Everything that is NOT a copy of `Group` is the credential.

| Field | Type | Notes |
|---|---|---|
| `id` | UUID v7 | the row id. Internal, but **returned** — a caller needs it to patch, archive, grant or reset |
| `tenantID` | UUID | the owning tenant. Immutable. **Filled from the caller's `tenant_id` claim**, and nameable in the create body only by a `*:*` operator crossing the row scope |
| `name` | composite | **one value across two columns** — `givenName` + `familyName`, each 1–75 runes. There is no `name` column; the rendered form is a computed read field |
| `email` | string(254) | the login handle. Unique across the **whole platform** over active rows, lowercase-only, and **immutable** |
| `emailVerifiedAt` | timestamp | ⚠️ **nothing in this service writes it.** The column exists so the verification flow, when it arrives, is code only |
| `password` | — | write-only, and it has **no column at all**. See below |
| `passwordChangedAt` · `mustChangePassword` | timestamp · bool | credential state. The second is what forces a rotation after an admin-set or reset password |
| `status` | enum | `active` · `suspended`. Suspension is **not** archiving — the same distinction `Tenant` draws |
| `groups[]` · `roles[]` | collections | the inherited half and the direct half. Each entry carries `id`, the target's id, and the target's `key` and `name`, read across the foreign key |
| `tenantWorkspace` · `tenantStatus` | string | the owning tenant's handle and commercial state, read across the foreign key. Filterable and sortable, unlike anything inside a collection |

Five things a reader will otherwise get wrong.

**1. The password has no column, and that is a language feature rather than a trick.** The
plaintext and its confirmation are declared `runtime: true, source: body`: they cross the
request, the command and the aggregate so the rules can judge them, and they reach no
`TableSchema`, no migration, no outbox payload, no audit event and no response. There is
nothing to redact because there is nothing stored. What IS stored is the Argon2id hash, and
it takes **four** separate declarations to keep it out of four different copies —
`assignedFrom: derived` (no write request can propose one), `hidden` (no response body
carries it), and `redact` on **both** axes (the audit event and the sync payload). A fifth
protection is not automatic at all: no filter and no `orderBy` names it, anywhere, because
a filter over a password hash is an oracle walked one rune at a time.

**2. The name is one value object across two columns**, exactly like `Permission`'s
`resource:action`. "Maria" alone is not a person's name and "Souza Lima" alone is not
either. Both halves are filterable and sortable in their own right — sorting a staff
listing by family name is the query an operator actually runs — and the joined rendering is
a **computed read field**, derived per row from the value object's own method. So
`?orderBy=fullName` is a typed 400 and `?orderBy=familyName` is not, which is the trade the
computed field makes everywhere in this service.

**3. The owner is filled from the token, and a super-admin may still name one.** This is the
one place `User` deliberately diverges from `Role` and `Group`. Both of those put
`tenantID` in the body with a rule refusing anything but the caller's own, because the
alternative available at the time removed the field from every write DTO — and with it the
operator's ability to create a row inside a customer's tenant at all. `User` uses
`bypassMaySet`: the field is filled from the claim for everybody, and rejoins the **create**
body as an optional value that only a scope-crossing caller can use. It stays out of the
patch entirely: a user does not change tenant by being edited.

**4. Adding somebody to a group is a THREE-HOP grant, and it is checked as one.** A group
confers roles and a role grants permissions, so joining a user to a group hands them the
union of every bundle it carries. The caller must hold **every** one of those permissions —
a set, not a key — and a group carrying any wildcard-bearing role cannot be joined through
this API at all. The order is load-bearing and identical to `Group`'s: availability, then
the wildcard refusal, then the escalation question, because `Identity.HasPermission` panics
on a wildcard and the wildcard check is what removes that input. Granting a role directly is
the same walk one hop shorter.

**5. Archiving is one-way here for a reason the other entities do not have.** Archiving a
user **releases their e-mail** — the unique index is over active rows — so between an
archive and any hypothetical unarchive, a new hire may have been registered with it.
Reversible deactivation has its own home: `status: suspended`, which keeps the row listed
and stops the product. A user on leave is suspended; a user who left the company is
archived, and archiving forces the status to `suspended` so archived-and-active is
unrepresentable rather than merely refused.

#### The password reset

Two operations, hand-written because the generator cannot express them — the spec language
gates operations by a closed set of verbs and neither "change a credential" nor "reset one"
is among them.

```
PATCH  /users/{id}/password             user:change-password  (the id must BE the caller)
PATCH  /users/{id}/password-reset       user:reset-password   (the id must NOT be the caller)
```

**They are one operation each, not one route with a mode.** The change is the caller
rotating their own credential: it requires `currentPassword`, verifies it, and CLEARS
`mustChangePassword`, because the password it leaves is the caller's own choice. The reset
is the helpdesk setting somebody else's: it carries **no current password** — not knowing it
is the point — and SETS the flag, so the next sign-in has to replace what a stranger chose.

**Each refuses the other's rows, and the second half of that is what matters.** Without the
reset refusing the caller's own id, a holder of `user:reset-password` points it at
themselves and replaces their credential without proving the previous one — defeating the
change endpoint's `currentPassword` by choosing the other URL. Same id is always the change;
a different id is always the reset. `user:update` deliberately reaches **neither**:
whoever can fix a typo in a name must not be able to take over an account.

The password a RESET leaves is somebody else's choice, so it sets `mustChangePassword`; the
CHANGE clears it, because the caller picked that password and proved the one before it. The
insert sets it too, for the same reason the reset does — an admin chose the initial
password.

**The permission answers WHO may attempt the verb and says nothing about WHOSE row.** The
aggregate's own row-scope guard is what refuses a holder of `user:reset-password` in one
tenant from resetting a password in another, and it only runs because the handler feeds it
the caller's identity the way every generated command mapper does. A `*:*` operator crosses
that scope, which is what lets the platform support a customer.

**The password policy is 8–128 runes with all four character classes** — lowercase,
uppercase, digit and symbol, judged by **Unicode** rather than ASCII — plus a context rule
refusing a password that contains the e-mail's local part or any word of the name. That is a
decision made knowingly against NIST SP 800-63B-4, which states a SHALL NOT on composition
requirements and sets 15 runes as the floor for a single-factor secret. Both sides of the
argument are in `specs/scaffold-entity/user/spec.md` §B Q3; it is one value object, and
revisiting it changes about fifteen lines.

⚠️ **One trap worth knowing before touching those rules.** The plaintext field declares
`modes: [insert]`, so the framework excludes its value object from the automatic validation
pass on every update — correctly, because a PATCH that renames somebody carries no password.
This route DOES carry one, and the policy runs only because the rule calls `IsValid`
directly. The obvious repair — `Rules.ValidateValueObject` — does not work and fails
silently: the forced list honours the same ignore set, which is how a five-character
password was accepted once.

#### Tenant isolation

Identical to `Role`'s and `Group`'s, applied unchanged — reads filtered by the claim with a
by-id read of another tenant's user answering **404 rather than 403**; writes refused with
403; a `*:*` super-admin crossing in both directions. The rejected alternative is worth
naming here specifically: a user listing is the customer's staff directory **including
e-mail addresses**, so reads-open isolation would leak more here than the org chart `Group`
already refused to leak.

**No request in this service legitimately carries no identity in production** *(amended
2026-08-26)*. This paragraph described the public change-password route, which was removed
that day; every stand-down that remains is confined to `auth.mode: disabled`, which the
framework permits only under `APP_PROFILE=dev`. The paragraph below is kept as the record of
why that route was exempt while it existed. Nobody should "fix" it by requiring a claim the caller structurally cannot
have.


## Running it locally

Requires Docker and a Go toolchain.

```bash
./start.sh          # macOS/Linux — brings up the bench, then runs the service
start.cmd           # Windows
```

It starts the Postgres bench (`devops/docker-compose.yml`), applies pending migrations and
serves on `:8080` under `APP_PROFILE=dev`.

| Surface | URL |
|---|---|
| OpenAPI UI | http://localhost:8080/docs (`GET /` redirects here) |
| GraphQL | http://localhost:8080/graphql · playground at `/graphql/ui` |
| Probes | `/livez` · `/readyz` |

Authentication is `disabled` in the dev profile — accepted **only** there; any other
profile aborts the boot without an `auth` block.

### API shape

```
GET    /tenants/                 list, filter, paginate
POST   /tenants/                 create
GET    /tenants/{id}             read one
PATCH  /tenants/{id}             partial update (name, description, status)
PATCH  /tenants/{id}/archive     reversible removal
PATCH  /tenants/{id}/unarchive   undo
```

```
GET    /permissions/                 list, filter, paginate
POST   /permissions/                 create
GET    /permissions/{id}             read one
PATCH  /permissions/{id}             partial update (description only)
PATCH  /permissions/{id}/archive     removal — ONE-WAY, there is no unarchive
```

```
GET    /roles/                                    list, filter, paginate
POST   /roles/                                    create
GET    /roles/{id}                                read one
PATCH  /roles/{id}                                partial update (name, description)
PATCH  /roles/{id}/archive                        removal — ONE-WAY, there is no unarchive
POST   /roles/{id}/permissions                    grant one permission
PATCH  /roles/{id}/permissions/{entryId}/archive  revoke one — never DELETE
```

```
GET    /users/                                     list, filter, paginate
POST   /users/                                     create — carries the password and its confirmation
GET    /users/{id}                                 read one
PATCH  /users/{id}                                 partial update (givenName, familyName, status)
PATCH  /users/{id}/archive                         removal — ONE-WAY, there is no unarchive
POST   /users/{id}/groups                          join a group        — user:grant
PATCH  /users/{id}/groups/{entryId}/archive        leave one           — user:grant
POST   /users/{id}/roles                           grant a role        — user:grant
PATCH  /users/{id}/roles/{entryId}/archive         revoke one          — user:grant

PATCH  /users/{id}/password                        change own — user:change-password
PATCH  /users/{id}/password-reset                  reset another — user:reset-password (or *:*)
```

`PATCH /users/{id}` carries **three** fields and no more: `givenName`, `familyName` and
`status`. `email` and `tenantID` are not merely refused there — they are absent from the
request shape, so there is no body in which a caller can propose one. Neither is any
credential field: there is no shape reaching the ordinary update in which a password exists
to be sent.

**Self-service password change: `PATCH /users/{id}/password`.** A PUBLIC change-password
route existed briefly and was removed on 2026-08-26 — it did what the reset does, without a
token, on the premise that somebody needing a new password might be unable to obtain one.
That premise does not hold here: there is no login route, so nothing blocks authentication
for a caller who knows their password. What replaced it later the same day is the
authenticated change above: token required, id must be the caller's own, current password
proved.

**What is still missing is a FORGOT-PASSWORD flow** — e-mail, an expiring link, a one-time
token — for the caller who has no password to prove. That is real work nobody has started,
and it is the only credential gap this service ships with.

```
GET    /groups/                              list, filter, paginate
POST   /groups/                              create
GET    /groups/{id}                          read one
PATCH  /groups/{id}                          partial update (name, description)
PATCH  /groups/{id}/archive                  removal — ONE-WAY, there is no unarchive
POST   /groups/{id}/roles                    attach one role      — group:grant
PATCH  /groups/{id}/roles/{entryId}/archive  detach one — never DELETE — group:grant
```

Permission serves the same five listing controls as Tenant, and `?orderBy` over
`resource`, `action` and `description`. Filters served, per field: `resource` (eq, ne, in,
contains, prefix) · `action` (eq, ne, in, contains) · `description` (contains). Neither
`resource` nor `action` is ever returned in a response body — they go in and stay in, and
what comes back is the rendered `permission`. `?orderBy=permission` and `?orderBy=id` are
both a typed 400: a computed field has no column to sort on, and the row id is not in the
declared vocabulary.

Role serves the same five listing controls as Tenant and Permission, and `?orderBy` over
`tenantId`, `key` and `name` — `description` is deliberately not sortable, since ordering a
listing by a 500-character free-text column is a blocking sort nobody asks for. Filters
served, per field: `tenantId` (eq, in) · `key` (eq, ne, in, prefix, contains, and the
case-insensitive twins) · `name` (eq, in, prefix, contains, and the twins) · `description`
(contains, icontains).

User serves the same five listing controls. Filters served, per field: `tenantId` (eq, in) ·
`givenName` and `familyName` (eq, in, prefix, contains and the case-insensitive twins;
`familyName` also `ne`) · `email` (the same set plus `ne`) · `status` (eq, in) ·
`mustChangePassword` (eq) · `passwordChangedAt` (gte, lte) · `emailVerifiedAt` (eq, gte,
lte) · `tenantWorkspace` and `tenantStatus` · `createdAt` and `updatedAt`. `?orderBy` admits
`familyName`, `givenName`, `email`, `status`, `passwordChangedAt`, `emailVerifiedAt`,
`tenantId`, `tenantWorkspace`, `tenantStatus`, `createdAt`, `updatedAt` and `id`.
**`passwordHash`, `failedLoginAttempts` and `lockedUntil` appear in neither list, on any
surface** — the silence is the policy, because a redacted field is still queryable by design
and nothing else keeps it out. `?orderBy=fullName` is a typed 400: a computed field has no
column to sort on.

Group serves the same five listing controls and the same `?orderBy` vocabulary as Role
(`tenantId`, `key`, `name`; `description` deliberately not sortable). Filters served, per
field: `tenantId` (eq, in) · `key` (eq, ne, in, prefix, contains, and the case-insensitive
twins) · `name` (eq, **ne**, in, prefix, contains, and the twins) · `description` (contains,
icontains). `name` carries `ne` where Role's does not — it costs nothing, and "everything
except the ops group" is a listing an operator does ask for. `?search=` is not served, for
the same reason as everywhere else here.

Group's two collection verbs are the one place in this service where a child route does NOT
ride the parent's update permission: both require `group:grant`. See the Group section above
for why.

The two collection verbs are a **pair, not the usual trio**: there is no "change this grant".
An entry's single field IS its identity, so changing which permission is granted is revoking
one and granting another — expressing it as an edit would keep the old row's id while
changing what it means, which an audit trail reads as one grant *becoming* another instead of
as two events. Re-granting a revoked permission is a fresh `POST` with a fresh entry id.

Both collection verbs ride the parent's permission, `role:update`, rather than inventing a
verb the rest of the service does not have. A distinct `role:grant` — so that "may rename the
role" and "may change what the role can do" are separately grantable — is a real distinction
and a one-line change if it is wanted.

The **GraphQL surface carries the root verbs only** (`roles`, `role`, `createRole`,
`patchRole`, `archiveRole`). The two collection verbs are REST-only.

Listing controls served: pagination (`?first`/`?after`/…), `?orderBy`, `?fields`,
`?onlyTotal`, `?includeArchived`. A control that is not declared is answered with a typed
400 — that is a contract, not an omission.

Filters served, per field: `tenantId` (eq, in) · `name` (eq, in, prefix, contains, and the
case-insensitive twins) · `workspace` (eq, in, prefix, iprefix) · `description` (contains,
icontains) · `status` (eq, in) · `createdAt` and `updatedAt` (eq, and the four range
operators). The two timestamps reach the read side through `read.managed`, which names the
framework-stamped columns; `deletedAt` stays off, because archived rows are reached with
`?includeArchived` rather than through a timestamp filter.

**`?orderBy` is a field allowlist**, and it admits three paths: `name`, `workspace` and
`createdAt`. Anything else — `description`, `status`, `tenantId`, `updatedAt` — is a typed
400, which is the point: nothing is orderable by accident, and an undeclared path never
becomes a blocking sort. `updatedAt` filters but does not order, deliberately.

`{id}` is the row id, not the public key. That is a deliberate trade: the management API
is operator-only and already behind a permission, and keeping the row id out of the URLs
too would mean abandoning the framework's automatic by-id handlers for hand-written ones.
What matters is that the row id never reaches a token or a consuming service, and it does
not.

Answer semantics:

| Status | When |
|---|---|
| `422` | a validation failure. Failures are reported **together** rather than one at a time, which is why uniqueness is checked in the domain before the database constraint backs it up |
| `409` | a duplicate — a workspace handle already used, by an active or an archived tenant |
| `409` | **also** a stale write: every root write pins the revision it was loaded with, so a write built on an out-of-date read is refused instead of silently reverting another writer's columns. Reload and reapply |
| `404` | archiving or reading a tenant that is not there. Archiving a missing row answers 404 rather than committing an event about nothing |
| `400` | a read control the listing does not declare — `?search=` is the one this entity does not serve, because a relational-backed view cannot answer free-text search and pretending otherwise would be worse |

Archiving is a real update at this pin: it stamps `updated_at`, and a rule that changes a
field while archiving reaches the row and not only the audit trail. That is what lets
archiving force a tenant to `suspended`.

**`tenantID` is not in any write body at all.** It is declared `assignedFrom: derived`,
which takes it out of the create and patch request schemas, out of the commands and out of
the OpenAPI request documentation — so there is no shape in which a caller can propose one,
and nothing has to ignore a value that was sent. It is fully present on the READ side:
every response carries it and it is filterable, which is how a consuming service resolves a
token claim back to a tenant.

Every message ships in seven languages (pt-BR, English, Spanish, French, German, Italian,
Dutch); the OpenAPI page carries a language selector. Enum VALUES are translated
alongside the field labels: each member's label is registered under the key the framework
derives from its value (`TenantStatus.trial`) and resolved at the boundary by
`translator.EnumDescription`, so a response renders `Avaliação` rather than `trial`.

## Layout

```
bootstrap/          composition root — Wire() assembles the features
internal/domain/    aggregates, rules, notifications
internal/domain/vos/   value objects — the shared text predicates live here, in runes and Unicode
internal/application/  commands, queries, DTOs, translation catalogs
internal/web/       requests, responses, routes, authorization
internal/infra/     table schemas, repositories, views
migrations/postgres/   numbered up/down pairs, applied at boot in dev
devops/             the local bench (Postgres)
```

Everything in `internal/` is generated from `specs/omnicore-gen/<entity>.omnicore.yaml`
EXCEPT the files named `*_manual.go`, which the generator writes once and never touches
again, and the credential path, which it cannot express at all:

```
internal/domain/password_hasher.go          the port — Hash · Matches · DummyMatches
internal/infra/password_hasher.go           Argon2id, OWASP baseline, PHC-encoded
internal/domain/user_credential_manual.go   the rules, entered by actionName
internal/application/commands/…_manual.go   the two commands and their handlers
internal/web/…_credential_routes_manual.go  the two routes
internal/infra/role_probe.go                what "one role, resolved" means — shared by Group and User
```

## Where the decisions are written down

This service is generated and evolved through the omnicore tooling, and the reasoning is
kept in the repository rather than in chat history:

- `specs/scaffold-service/spec.md` — the approved service-level model: posture, surfaces, why
  there is no Mongo.
- `specs/scaffold-entity/<entity>/spec.md` — the approved domain model per entity, with the
  alternatives that were rejected and why.
- `specs/scaffold-entity/<entity>/tasks.md` — what each layer had to contain, plus the
  **deviations** between what was specified and what was actually generated.
- `specs/omnicore-gen/<entity>.omnicore.yaml` — the generator's input, written when the
  entity is generated; the code regenerates from it, the database never does.
- `specs/omnicore-gen/<entity>.gen-report.md` — what was generated, what was refused, and what
  had to be written by hand.
- anything the spec language cannot express is named in each entity's `spec.md` **before**
  generation, under *What generation will have to write by hand, and why* — next to the model
  decision it affects rather than in a file that outlives it. A gap in the tooling itself goes
  upstream to the maintainer; it is never routed around in this repository.

Read those before changing an entity. They exist so a reviewer can see what was decided
without reverse-engineering it from the code.

## Changing things

| Goal | Tool |
|---|---|
| add an entity | `/omnicore:scaffold-entity` |
| change an existing one | `/omnicore:evolve-entity` — it edits the spec and regenerates |
| add a cross-entity read model | `/omnicore:scaffold-view` |
| change the infrastructure posture | `/omnicore:configure` |
| generate an end-to-end contract suite | `/omnicore:qa` |

Two rules the generator enforces and that are easy to trip over:

- **Never hand-edit a generated file.** It is hashed; the next run refuses it and leaves
  your edit stranded. Change the spec and regenerate.
- **`*_manual.go` and the migration pair are yours.** They are written once and never
  touched again — which is exactly why a schema change is a *new* numbered migration, never
  an edit to one that has already run.

Contribution constraints for this repository are in `CLAUDE.md`.
