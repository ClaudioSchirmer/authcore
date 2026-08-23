# authcore-service

**Identity and authentication for a multi-tenant platform.** It owns who the tenants are,
who the users are, and what the tokens the rest of the platform trusts actually say.

The tenant registry is the foundation the rest is built on. A tenant carries **three
identifiers with one job each**, and telling them apart is the first thing to understand
here:

| Identifier | Value | Who sees it |
|---|---|---|
| `id` | UUID v7, minted by the framework | internal. Referenced by no foreign key; appears only in the management API's by-id URLs |
| `workspace` | `acme-comercio` | the human-facing handle — URLs, logs, support |
| `tenant_id` | `UUIDv5(namespace, workspace)` | **the public key**: the token claim, and the foreign-key target of every future aggregate |

So `tenant_id` means exactly one value everywhere it appears — column, claim and foreign
key — and the row id is never issued to anyone. Every other service scopes its data by
reading that claim off the token instead of asking this one.

Go module: `github.com/ClaudioSchirmer/authcore` · Go 1.26.5 · built on
[omnicore](https://github.com/ClaudioSchirmer/omnicore) **v0.54.0** (DDD + CQRS framework).

---

## Current state

Honest scope, so nobody reads intent as delivery:

| Capability | State |
|---|---|
| Tenant registry (create, read, patch, archive/unarchive, REST + GraphQL) | **built** — generated from `specs/omnicore-gen/tenant.omnicore.yaml`; `gofmt`/`vet`/`build`/tests green. **Not yet booted against a real Postgres in this working tree** — `/omnicore:run` boots it, `/omnicore:qa` proves the endpoints |
| User entity | not started |
| User ↔ tenant association | not started |
| `Permission` entity | **done** — the global catalog; see below |
| `Role` entity | **done** — a tenant's own bundle of catalog permissions; generated from `specs/omnicore-gen/role.omnicore.yaml`; `gofmt`/`vet`/`build`/tests green. **Not yet booted against a real Postgres in this working tree** |
| `Group` entity | **done** — a tenant's org unit and the bundle of roles its members inherit; generated from `specs/omnicore-gen/group.omnicore.yaml`; `gofmt`/`vet`/`build`/tests green. **Not yet booted against a real Postgres in this working tree** |
| Effective-permission resolution (group path ∪ direct path) | not started |
| Reserved platform tenant | not started — and **two** entities now DEPEND on it. `Role`: no wildcard permission can be granted through the API, so the platform's own `*:*` role has to be seeded by migration beside that tenant. `Group`: no wildcard-bearing role can be attached to a group through the API either, so the platform's own super-admin **group** has to be seeded in that same migration |
| Token issuance with the `tenant_id` claim | not started — the value it must carry is the tenant's derived `tenant_id`, never the row id |
| Commercial status (`trial` / `active` / `suspended`) | **built** on Tenant; nothing consumes it yet |
| Contract QA suite (`/omnicore:qa`) | not generated |
| Permission enforcement in production | routes are gated (`tenant:*`, `permission:*` and `role:*`, four verbs each, plus `group:`'s **five**); `auth.mode` is `disabled` in dev and `jwt` in prd. The literals have **no catalog row until an operator inserts one** — see the seeding note below, and note that `group:grant` is the one nobody will guess from the pattern |

## Architecture posture

Real PostgreSQL as the source of truth, **no CDC pipeline**. Both profiles declare it
identically, and the absence is the declaration: no `mongo:` block and no `transport:`
block.

What that buys and what it costs:

- **Reads are served straight from the tables** (`.RelationalSource`), so a write is
  visible to the very next read — no eventual consistency, no waiting on a projection.
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

Only **Tenant** exists in code today. Everything else below is the agreed target, recorded
here so a reader can see the destination without reverse-engineering it from half a
service:

```
                        ┌──> Group ──> Role ──> Permission     (inherited, via group)
User ───────────────────┤
 │                      └──────────> Role ──> Permission       (granted directly)
 │
 └── tenant_id ──> Tenant                                      (the partition it lives in)
```

A user's **effective permissions** are the union of both paths — the roles reached through
their groups, plus the roles granted to them directly. There is no precedence and no deny
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
| `id` | UUID v7 | the row id. Internal — never issued, never a foreign-key target |
| `tenant_id` | UUID v5 | **derived** from `workspace`; the token claim and the FK target. Read-only: computed on insert, immutable afterwards |
| `name` | string(120) | display name. **Not unique** — two customers may legitimately share a trade name |
| `workspace` | string(63) | the handle. Unique across **all** rows, archived included; immutable after creation |
| `description` | string(500) | required, and validated for substance |
| `status` | enum | `trial` · `active` · `suspended`. Mandatory on create, no default |

#### Why the public key is derived instead of being the row id

The row id is a **UUID v7**, which embeds a millisecond timestamp by construction. Issuing
it as a claim would hand every client and every consuming service the creation instant and
the creation ORDER of every tenant — how many customers signed in March, and who was first.
A `UUIDv5(namespace, workspace)` carries no time at all, and any service that knows the
handle can recompute it offline with no call back here.

**It is not a secret and must never be treated as one.** UUID v5 is a hash of the namespace
and the name, so given the namespace it is brute-forceable back to the handle over a small
dictionary of company handles. That costs nothing here: the handle is public by design. What
the derivation hides is the timestamp, and that it hides completely.

The namespace is a project constant in `internal/domain/vos/tenant_workspace.go`, and it
**must never change**: changing it re-derives every `tenant_id` in existence, invalidating
every issued token and orphaning every foreign key pointing at one.

#### The three asymmetries worth knowing before you touch them

- **The workspace is reserved forever.** It reaches URLs, logs and bookmarks — and it
  derives the public key, so re-issuing a handle would re-issue a byte-identical
  `tenant_id`. Every token ever minted for the archived tenant would then validate, and be
  *authorized*, against the new one's data. Valid signature, correct claim, wrong tenant,
  nothing to detect. Its unique index is therefore total, never partial.
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
| `tenant_id` | must equal `workspace.DeriveTenantID()` · immutable |
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
`tenant_id`, and an irreversible purge would orphan them; it would also erase the row that
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
| `tenantID` | UUID | the owning tenant. Immutable. FK to `tenants.tenant_id` — the PUBLIC derived key, not the surrogate row id |
| `key` | string(64) | the stable machine handle (`billing-manager`). Immutable, and unique **per tenant**, not globally. Stored in `role_key` |
| `name` | string(120) | the display name. Not unique — two tenants, or two roles, may share a label |
| `description` | string(500) | required, and validated for substance |
| `permissions[]` | collection | the grants. Each entry carries `id` and `permissionID`, and nothing else |

Five things a reader will otherwise get wrong.

**1. The read returns catalog ids, not permission strings.** `GET /roles/{id}` answers
`"permissions": [{ "id": "…", "permissionID": "9f14b0a2-…" }]`. A client that wants to show
*what* the role grants calls `GET /permissions` and joins by id itself. The server cannot do
it: this service has no Mongo, so its views are served straight from the tables and there is
no read-time join to compose one with. The alternative was denormalizing the
`resource:action` pair into each grant, and that was rejected for a specific reason — a
retired permission comes back as a **new row with a new id**, so a grant holding the *string*
would silently re-attach to the recreated row and quietly break that promise. A grant holding
the **id** cannot. This stops being the client's problem the day the service gains Mongo, with
no change to the model.

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
`?filter[permissions.permissionID][eq]=…` is a typed 400, not a result. "Which roles grant
`tenant:read`?" is not answerable from this listing on this backing — a relational-served
view carries the collection in the document it returns, but cannot filter or order by a field
inside it. It becomes answerable the day the service gains Mongo (`/omnicore:configure`),
with no change to this model.

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
| `tenantID` | UUID | the owning tenant. Immutable. FK to `tenants.tenant_id` — the PUBLIC derived key, not the surrogate row id |
| `key` | string(64) | the stable machine handle (`engineering`). Immutable, and unique **per tenant**, not globally. Stored in `group_key` |
| `name` | string(120) | the display name. Not unique — two groups in one tenant may share a label; the `key` is what disambiguates |
| `description` | string(500) | required, and validated for substance |
| `roles[]` | collection | the bundle. Each entry carries `id` and `roleID`, and nothing else |

**1. The read returns role ids, not role names.** `GET /groups/{id}` answers
`"roles": [{ "id": "…", "roleID": "7c2e9b41-…" }]`. A client that wants to show *what* the
group confers calls `GET /roles` and joins by id itself. Same reason as on `Role`, one level
up: with no Mongo there is no read-time join, and an entry holding the role's *key* would
silently re-attach to a retired-and-recreated role. An entry holding the **id** cannot.

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
`?filter[roles.roleID][eq]=…` is a typed 400. **"Which groups confer role X?" is not
answerable from this listing on this backing** — and it is a question an access review asks
more often than `Role`'s equivalent, because it is how you find out who a role actually
reaches. With `Group` in the graph a client is now joining three listings rather than two.
It becomes answerable the day the service gains Mongo (`/omnicore:configure`), with no change
to this model.

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
icontains) · `status` (eq, in).

Two narrower-than-intended edges, both recorded in `specs/scaffold-entity/tenant/spec.md`:
**`createdAt` and `updatedAt` are not filterable** (the generator declares filters only over
declared entity fields, and the framework-managed timestamps are not among them), and
**`?orderBy` is not restricted to a field allowlist** — the model named four sortable
fields, and what the generator can express is orderBy on or off for the whole view.

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

Two of those — the stale-write `409` and the archive `404` — arrived with omnicore
`v0.54.0`, which also made archiving a real update: it stamps `updated_at`, and a rule
that changes a field while archiving now reaches the row instead of only the audit trail.
That last one is what lets archiving force a tenant to `suspended`.

**`tenantID` is not in any write body at all.** It is declared `assignedFrom: derived`,
which takes it out of the create and patch request schemas, out of the commands and out of
the OpenAPI request documentation — so there is no shape in which a caller can propose one,
and nothing has to ignore a value that was sent. It is fully present on the READ side:
every response carries it and it is filterable, which is how a consuming service resolves a
token claim back to a tenant.

(An earlier run of this service could not say that: the generator had no way to declare a
server-computed field, so `tenantID` sat in the write schema being silently overwritten.
That gap was reported upstream and is closed — the key exists as of omnicore plugin
`0.22.0`, and this entity uses it.)

Every message ships in seven languages (pt-BR, English, Spanish, French, German, Italian,
Dutch); the OpenAPI page carries a language selector. **Field labels are translated;
`status` VALUES are not** — a response carries the raw token (`trial`, `active`,
`suspended`), because the generator accepted the seven member translations and emitted no
catalog entry for them. The texts are already written in the spec, so they will land the
moment that emitter exists.

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

## Where the decisions are written down

This service is generated and evolved through the omnicore tooling, and the reasoning is
kept in the repository rather than in chat history:

- `specs/scaffold-service/spec.md` — the approved service-level model: posture, surfaces, why
  there is no Mongo.
- `specs/scaffold-entity/<entity>/spec.md` — the approved domain model per entity, with the
  alternatives that were rejected and why.
- `specs/scaffold-entity/<entity>/tasks.md` — what each layer had to contain, plus the
  **deviations** between what was specified and what was actually generated.
- `specs/omnicore-gen/<entity>.omnicore.yaml` — the generator's input; the code regenerates from
  it, the database never does.
- `specs/omnicore-gen/<entity>.gen-report.md` — what was generated, what was refused, and what
  had to be written by hand.
- tooling gaps found while generating are recorded in each entity's `spec.md`, under
  *Deviations recorded at generation time* — that keeps them next to the model decision
  they affect instead of in a file that outlives them. Of the two found on the first pass
  at Tenant, both are now closed upstream (server-derived fields, and field labels being
  seeded from the field's description); one new one is open, and it is visible in this
  service: the status enum's per-locale member labels are accepted by the generator's
  validator and emitted by nothing. Permission's own first pass found six; the rebuild on
  generator 0.25.0 closed four of them outright — uniqueness and immutability over a
  composite value object are now generated, the archive endpoint's documentation no longer
  advertises an undo that does not exist, and the write responses carry the derived field.
  Two of those four had cost a permanent file adoption on the first pass; this entity pays
  none, so every file of it still tracks its spec.
- `specs/upgrade/rollback/` — the `go.mod`/`go.sum` pair from before the framework upgrade this
  entity needed, kept as an exact restore point.

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
