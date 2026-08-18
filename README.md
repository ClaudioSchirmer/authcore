# authcore-service

**Identity and authentication for a multi-tenant platform.** It owns who the tenants are,
who the users are, and what the tokens the rest of the platform trusts actually say.

The tenant registry is the foundation the rest is built on: a tenant's id is the value that
travels as the `tenant_id` claim of the issued JWT, so every other service can scope its
data by reading the token instead of asking this one.

Go module: `github.com/ClaudioSchirmer/authcore` · Go 1.26.5 · built on
[omnicore](https://github.com/ClaudioSchirmer/omnicore) **v0.51.0** (DDD + CQRS framework).

---

## Current state

Honest scope, so nobody reads intent as delivery:

| Capability | State |
|---|---|
| Tenant registry (CRUD, archive/unarchive, REST + GraphQL) | **built** |
| User entity | not started |
| User ↔ tenant association | not started |
| Group, Role, Permission entities | not started — target model agreed, see below |
| Effective-permission resolution (group path ∪ direct path) | not started |
| Reserved platform tenant | not started |
| Token issuance with the `tenant_id` claim | not started |
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
that reassigns `joao@acme.com` to a new hire can register them. Unlike the tenant slug,
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

An isolation partition. Flat aggregate, table `tenants`.

| Field | Type | Notes |
|---|---|---|
| `id` | UUID | the value issued as the `tenant_id` JWT claim |
| `name` | string(120) | unique among **active** tenants — archiving frees the name |
| `slug` | string(40) | unique across **all** rows, archived included; immutable after creation |
| `description` | string(500) | required, and validated for substance |

Two deliberate asymmetries worth knowing before you touch them:

- **The slug is reserved forever.** It reaches URLs, logs and bookmarks, so letting an
  archived tenant's slug be re-taken would silently point old references at a *different*
  tenant. The name has no such exposure, so archiving releases it.
- **A consequence of that pair:** if a tenant is archived and someone else takes its name,
  unarchiving the original answers 409 — two active tenants may not share a name.

Business rules enforced by the domain:

| Field | Rules |
|---|---|
| `name` | required · 2–120 characters · unique among active tenants (service pre-check + database constraint) |
| `slug` | required · `^[a-z][a-z0-9]*(-[a-z0-9]+)*$` · 3–40 characters · no input normalization — `" Acme "` is refused, never silently lowercased · no run of 4+ identical characters and at least 3 distinct · unique across all rows · immutable |
| `description` | required · 15–500 characters · at least two words · no run of 4+ identical characters and at least 5 distinct · must differ from the name and the slug under a normalized comparison |

Removal is **archive only** — there is no `DELETE`. Users and issued tokens will carry
`tenant_id`, and an irreversible purge would orphan them.

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
PATCH  /tenants/{id}             partial update (name, description — never the slug)
PATCH  /tenants/{id}/archive     reversible removal
PATCH  /tenants/{id}/unarchive   undo
```

Listing controls served: pagination (`?first`/`?after`/…), `?orderBy`, `?fields`,
`?onlyTotal`, `?includeArchived`. A control that is not declared is answered with a typed
400 — that is a contract, not an omission.

Answer semantics: `422` for a validation failure, `409` for a duplicate. Failures are
reported **together** rather than one at a time, which is why uniqueness is checked in the
domain before the database constraint backs it up.

Every message ships in seven languages (pt-BR, English, Spanish, French, German, Italian,
Dutch); the OpenAPI page carries a language selector.

## Layout

```
bootstrap/          composition root — Wire() assembles the features
internal/domain/    aggregates, rules, notifications, value objects
internal/application/  commands, queries, DTOs, translation catalogs
internal/web/       requests, responses, routes, authorization
internal/infra/     table schemas, repositories, views
migrations/postgres/   numbered up/down pairs, applied at boot in dev
devops/             the local bench (Postgres)
```

## Where the decisions are written down

This service is generated and evolved through the omnicore tooling, and the reasoning is
kept in the repository rather than in chat history:

- `scaffold-service/spec.md` — the approved service-level model: posture, surfaces, why
  there is no Mongo.
- `scaffold-entity/<entity>/spec.md` — the approved domain model per entity, with the
  alternatives that were rejected and why.
- `scaffold-entity/<entity>/tasks.md` — what each layer had to contain, plus the
  **deviations** between what was specified and what was actually generated.
- `omnicore-gen/<entity>.omnicore.yaml` — the generator's input; the code regenerates from
  it, the database never does.
- `omnicore-gen/<entity>.gen-report.md` — what was generated, what was refused, and what
  had to be written by hand.

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
