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
| Group, Role, Permission entities | not started — target model agreed, see below |
| Effective-permission resolution (group path ∪ direct path) | not started |
| Reserved platform tenant | not started |
| Token issuance with the `tenant_id` claim | not started — the value it must carry is the tenant's derived `tenant_id`, never the row id |
| Commercial status (`trial` / `active` / `suspended`) | **built** on Tenant; nothing consumes it yet |
| Contract QA suite (`/omnicore:qa`) | not generated |
| Permission enforcement in production | routes are gated (`tenant:read`, `tenant:insert`, `tenant:update`, `tenant:archive`); `auth.mode` is `disabled` in dev and `jwt` in prd |

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
code ever consults, so the catalog is seeded from what the code actually enforces and is
read-only to tenants.

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
  validator and emitted by nothing.
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
