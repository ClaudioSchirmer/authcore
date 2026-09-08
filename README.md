# authcore

**The identity provider of a multi-tenant platform.** It owns the tenants, the people and the
machines that act inside them, the roles and permissions they hold, and the signed tokens the
rest of the mesh trusts. Any other service accepts an authcore token by pointing
`auth.jwt.jwksUrl` at this service — configuration only, no code.

What it answers, in one line each:

- **Who is this?** `POST /auth/user/token` for a person, `POST /auth/client/token` for a machine.
- **What may they do?** The access token carries their effective permissions, resolved from the
  roles granted to them directly *plus* the roles conferred by every group they belong to.
- **Who administers all of that?** 57 management endpoints over seven aggregates — Tenant, User,
  Client, Group, Role, Permission and Claim — on REST and GraphQL alike.

---

## Framework

> ### [omnicore](https://github.com/ClaudioSchirmer/omnicore) **v0.74.0**
>
> A DDD + CQRS framework for Go. It supplies the composition root, the request pipeline, the
> repositories and view readers, the OpenAPI and GraphQL surfaces, the authorization
> middleware, the migration runner, the audit trail, the translation catalogs and the RS256
> token issuer with its JWKS route. This repository holds the domain and the wiring; almost
> everything structural above comes from the pin.
>
> **Repository:** https://github.com/ClaudioSchirmer/omnicore
> **Pinned in:** [`go.mod`](go.mod) — `github.com/ClaudioSchirmer/omnicore v0.74.0`
> **Upgrading:** `/omnicore:upgrade` (never a hand-edited pin)

## Tooling

> ### [omnicore-plugin](https://github.com/ClaudioSchirmer/omnicore-plugin) **v0.67.0**
>
> The Claude Code plugin this repository is maintained with. It ships **omnicore-gen** — the
> spec-driven generator that writes most of `internal/` from `specs/omnicore-gen/` — and the
> `/omnicore:*` skills that scaffold, evolve, configure, run, QA and upgrade a service built on
> the framework. It is tooling and nothing else: no artifact it installs is compiled into this
> service, and no file here imports it.
>
> **Repository:** https://github.com/ClaudioSchirmer/omnicore-plugin
> **Installed as:** marketplace `omnicore` → plugin `omnicore` **v0.67.0** (`/plugin`)

The two versions move independently: the pin above is what this service **runs on**, the plugin
is what **wrote** it. Which framework release each entity was last generated against is recorded
per entity in [`specs/omnicore-gen/lock.json`](specs/omnicore-gen/lock.json) — all seven read
`v0.74.0` today, so no entity is drifting behind the pin.

Files named `*_manual.go` are hand-written and the generator never touches them again.

## Technologies

| Layer | Choice |
|---|---|
| Language | Go **1.26.5** — module `github.com/ClaudioSchirmer/authcore` |
| Framework | **omnicore v0.74.0** (DDD, CQRS, features, pipeline, audit, i18n) |
| Tooling | **omnicore-plugin v0.67.0** — the `/omnicore:*` skills and the `omnicore-gen` generator |
| HTTP | Fiber **v3.3.0** |
| Source of truth | PostgreSQL 17 (`pgx/v5`), build tag `postgres` |
| Migrations | `golang-migrate/v4`, numbered up/down pairs under `migrations/postgres/` |
| Read models | Relational views served straight from the tables — no Mongo, no projection lag |
| API surfaces | REST + OpenAPI 3 (`/docs`) and GraphQL (`/graphql`) — one handler behind both |
| Tokens | RS256 JWT + JWKS, opaque single-use refresh tokens with family revocation |
| Password hashing | Argon2id (`golang.org/x/crypto`), PHC-encoded, m=19456 t=2 p=1 |
| Observability | OpenTelemetry traces, structured `slog` output, framework audit events |
| i18n | Seven catalogs — ptbr · eng · esp · fra · deu · ita · nld |
| Local bench | Docker Compose: one Postgres container |

The framework also ships adapters for Mongo, Kafka, NATS, Redis and gRPC. **This service
activates none of them** — see the posture below.

## Architecture posture

PostgreSQL is the single source of truth and there is **no CDC pipeline**: neither profile
declares a `mongo:` nor a `transport:` block, and that absence is the declaration.

- **Reads are served straight from the tables**, so a write is visible to the very next read —
  no eventual consistency, no projection to rebuild, no view `Version` to bump.
- **Free-text search is not served**: a relational-backed view answers `?search=` with a typed
  400 instead of pretending. Filters and sorts over 1:1-reachable fields work normally.
- **Integration events cannot be published** — publishing rides a CDC relay that does not exist
  here.
- Multi-source read models (ComposedView, SharedBaseView, the Embed/Link family) need Mongo and
  are therefore unavailable.

The posture is reversible in one pass with `/omnicore:configure`, without losing application code.

## Domain model

```
                        ┌──> Group ──> Role ──> Permission     (inherited, via group)
User ───────────────────┤
 │                      └──────────> Role ──> Permission       (granted directly)
 │
 ├── claims ──> Claim (catalog)                                (the values its token carries)
 └── tenant_id ──> Tenant                                      (the partition it lives in)

Client ─────────────────────────────> Role ──> Permission      (machines: direct grants only)
 ├── claims ──> Claim (catalog)
 ├── allowedCIDRs                                              (enforced at the token route)
 └── tenant_id ──> Tenant
```

| Aggregate | What it is |
|---|---|
| **Tenant** | The isolation partition. `id` (UUID v7) is the `tenant_id` claim, the FK target of every scoped row and the by-id URL; `workspace` is the human handle. Commercial `status`: `trial` → `active` → `suspended`, never back to `trial`. |
| **User** | A person inside exactly one tenant. `email` is unique across the whole platform; the same person in two tenants is two users. Holds password state, group memberships, direct roles and claim values. Capped at **50 groups, 50 direct roles and 20 claim values**. |
| **Client** | A machine identity: a client id, a hashed secret with a rotation grace window, an optional CIDR allow-list, direct roles and claim values. Never gets a refresh token. Capped at **50 roles, 20 CIDR ranges and 20 claim values**. |
| **Group** | A bundle of roles. Attaching a user to a group confers every role the group carries. Capped at **50 roles** — a cap that multiplies against Role's own, since a member inherits every permission of every role in the bundle. |
| **Role** | A bundle of catalog permissions, scoped to one tenant. Capped at **250 permissions**. `*:*` can never be granted through the API. |
| **Permission** | The platform-wide catalog of enforceable `resource:action` pairs. Not tenant-scoped. |
| **Claim** | The catalog of tenant-defined claims a token may carry: a name (reserved prefix `x_`), a `valueType` (`string` · `number` · `bool`), an `appliesTo` (`user` · `client` · `both`) and an optional tenant-wide default. Capped at **20 active definitions per tenant per identity kind**. |

Effective permissions are the **union** of both paths, archive-gated at every hop: a revoked
grant, a retired role, a left group and an archived membership each confer nothing. There is no
precedence and no deny rule — a permission is held or it is not.

## Running it locally

Requires Docker and a Go toolchain.

```bash
./start.sh          # macOS / Linux / WSL
start.cmd           # Windows (or: pwsh -File .\start.ps1)
```

It brings up the Postgres bench (`devops/docker-compose.yml`), generates a dev RSA signing key
on first run (`devops/dev-signing-key.pem`, never committed), applies pending migrations and
serves on `:8080` under `APP_PROFILE=dev`.

| Surface | URL |
|---|---|
| OpenAPI UI | http://localhost:8080/docs (`GET /` redirects here) |
| GraphQL | http://localhost:8080/graphql · playground at `/graphql/ui` |
| JWKS | http://localhost:8080/.well-known/jwks.json |
| Probes | http://localhost:8080/livez · http://localhost:8080/readyz |

**The dev bench is closed on purpose.** `auth.mode` is `jwt` in *both* profiles, with the
permission gate on and a required `tenant_id` claim. Everything except the three token routes,
the probes, the JWKS document and the documentation surface answers **401** without a bearer.

### The bootstrap credential

Migration `0012_bootstrap_seed_manual` seeds what a fresh database needs before anyone can sign
in at all — the 38 permission rows plus the wildcard, the `master` tenant, a `master` role
carrying `*:*`, and one administrator holding it:

```
e-mail:    admin@authcore.local
password:  admin          (must_change_password = TRUE)
```

The hash is a literal in a tracked file: **treat this credential as public knowledge and rotate
it on first sign-in.** A must-change-password session is restricted to exactly one permission,
`user:change-password`, and carries no custom claims — it exists only to reach
`PATCH /users/{id}/password`. The wildcard grant is seeded by
migration because `no-wildcard-grant` in the domain refuses `*:*` on any role through the API,
with no caller exempt.

```bash
curl -s -X POST http://localhost:8080/auth/user/token \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@authcore.local","password":"admin"}'
```

One boot line is expected and not a fault: this service validates through a JWKS document it
has not begun serving yet, so the first fetch is refused and logged as a WARN. The first
request carrying a token triggers a refresh that succeeds.

---

# Endpoints

**60 REST routes and 57 GraphQL fields.** Every GraphQL field reuses the handler of its REST
twin with the same permission attached, so the two surfaces cannot drift.

Reading the tables:

- **Permission** is what `RequirePermission` demands at the mount. It is satisfied exactly, by a
  resource wildcard (`user:*`), or by `*:*`. Everything additionally requires a bearer token
  carrying a `tenant_id` claim — without it the answer is 403 before any handler runs.
- Paths use `{id}`; the OpenAPI document spells collection routes with a trailing slash
  (`/tenants/`), and both forms are routed.
- **Archive is reversible only where an unarchive route exists** — today that is Tenant alone.
  Archived rows stay in the table and are hidden from reads unless a read asks for them.
- Every response uses the framework's canonical envelope, translated into the caller's language;
  unknown filter keys, operators or controls are rejected with a typed 400 rather than ignored.
- Who reaches *which rows* — tenant scope, self-rules, what `*:*` crosses — is a separate
  document: [`ACCESS_MATRIX.md`](ACCESS_MATRIX.md).

## Authentication — public, no token required

These three are the routes that hand out the credential every other route demands. They are
REST only: no GraphQL twin.

| Endpoint | Permission | Description |
|---|---|---|
| `POST /auth/user/token` | *public* | Sign-in. Exchanges an e-mail and a password for a signed access token and an opaque refresh token. The access token carries the caller's identity, their effective permissions and their tenant claims. |
| `POST /auth/user/token/refresh` | *public* | Rotation. Redeems an unused refresh token for a fresh pair. Single-use: replaying a redeemed value revokes the whole session family. |
| `POST /auth/client/token` | *public* | Machine sign-in. Exchanges a client id and secret for an access token — **no refresh token** (RFC 6749 §4.4.3: the secret already is the long-lived credential). The client's CIDR allow-list is enforced here and nowhere else. |

**Brute-force lockout.** Five failed *user* sign-ins for one identity inside 15 minutes answer
**429** with the remaining window, auto-releasing as the window ages out. The counter is keyed by
the attempted identity, not by a user row, so an address naming no account locks exactly as a
real one does — the uniformity is what stops the endpoint being an existence oracle. The client
route **counts on the same table but never locks**: a client secret is 32 random bytes that no
one guesses, and a client id is public, so locking would hand anyone a five-request outage
against a production integration. Every sign-in outcome is published as a structured `event`
record on the log stream; the attempted password enters neither the table nor the stream.

## Tenants

| Endpoint | GraphQL | Permission | Description |
|---|---|---|---|
| `POST /tenants` | `createTenant` | `tenant:insert` | Creates a tenant and returns it as stored — the response reflects what the domain normalised or defaulted, not an echo of the request. |
| `PATCH /tenants/{id}` | `patchTenant` | `tenant:update` | Partial update: only the fields present in the body change. Cannot set a value back to null. |
| `PATCH /tenants/{id}/archive` | `archiveTenant` | `tenant:archive` | Archives the tenant and forces its commercial status to `suspended`. Reversible. |
| `PATCH /tenants/{id}/unarchive` | `unarchiveTenant` | `tenant:archive` | Restores a previously archived tenant. |
| `GET /tenants` | `tenants` | `tenant:read` | Paged listing, with filters and sorts over the view's fields. **Row-scoped:** a caller who is not a `*:*` operator sees their own tenant and no other. |
| `GET /tenants/{id}` | `tenant` | `tenant:read` | Reads one tenant by its identifier. Same scope: any id but the caller's own answers **404**, unless the caller holds `*:*`. |

## Users

| Endpoint | GraphQL | Permission | Description |
|---|---|---|---|
| `POST /users` | `createUser` | `user:insert` | Creates a person's account with its first credential. The tenant comes from the caller's `tenant_id` claim; naming another one in the body takes a `*:*` operator. |
| `PATCH /users/{id}` | `patchUser` | `user:update` | Partial update of the account's descriptive attributes. |
| `PATCH /users/{id}/archive` | `archiveUser` | `user:archive` | Retires the account so the person can no longer sign in. |
| `GET /users` | `users` | `user:read` | Paged listing of user accounts. |
| `GET /users/{id}` | `user` | `user:read` | Reads one account with its group memberships, its direct roles — each carrying key and label — and its claim values. |
| `PATCH /users/{id}/password` | `changeUserPassword` | `user:change-password` | **Self-service.** The id on the path must be the caller's own — pointing it at another user answers 403 whatever the caller holds — and the current password is verified before the new one is accepted. |
| `PATCH /users/{id}/password-reset` | `resetUserPassword` | `user:reset-password` | **The helpdesk operation.** Sets a new password without knowing the current one; the id must *not* be the caller's own. Can force a change on the next sign-in. |
| `POST /users/{id}/groups` | `addUserGroup` | `user:grant` | Places the user in one group, inheriting every role that group confers. |
| `PATCH /users/{id}/groups/{userGroupId}/archive` | `archiveUserGroup` | `user:grant` | Removes the user from that group. |
| `POST /users/{id}/roles` | `addUserRole` | `user:grant` | Grants one role directly to the user. Refuses an escalation the caller does not already hold. |
| `PATCH /users/{id}/roles/{userRoleId}/archive` | `archiveUserRole` | `user:grant` | Withdraws that direct grant. |
| `POST /users/{id}/claims` | `addUserClaim` | `user:set-claim` | Sets the value this user carries for one catalog definition — same tenant, an `appliesTo` admitting a user, and a value parsing as the declared type. |
| `PATCH /users/{id}/claims/{userClaimId}` | `patchUserClaim` | `user:set-claim` | Corrects the value. The body carries **only** `value`: the definition is read off the stored row, so an entry can never become the value of a different claim. |
| `PATCH /users/{id}/claims/{userClaimId}/archive` | `archiveUserClaim` | `user:set-claim` | Withdraws the value; the tenant-wide default takes over if the definition declares one. |

## Clients (machine identities)

| Endpoint | GraphQL | Permission | Description |
|---|---|---|---|
| `POST /clients` | `createClient` | `client:insert` | Registers a machine that authenticates with credentials of its own. |
| `PATCH /clients/{id}` | `patchClient` | `client:update` | Partial update of the client's descriptive attributes. |
| `PATCH /clients/{id}/archive` | `archiveClient` | `client:archive` | Retires the client so it can no longer obtain a token. |
| `GET /clients` | `clients` | `client:read` | Paged listing of machine clients. |
| `GET /clients/{id}` | `client` | `client:read` | Reads one client with its roles, allow-list and claim values. |
| `POST /clients/{id}/secret` | `rotateClientSecret` | `client:rotate-secret` | Mints a new secret and starts retiring the current one. **The plaintext is in this response and nowhere else** — nothing stores it, so a caller who loses it rotates again. A grace period keeps the old secret working while consumers redeploy; a zero window kills it with this call. |
| `POST /clients/{id}/roles` | `addClientRole` | `client:grant` | Grants one role to the machine. |
| `PATCH /clients/{id}/roles/{clientRoleId}/archive` | `archiveClientRole` | `client:grant` | Withdraws that grant. |
| `POST /clients/{id}/allowedCIDRs` | `addClientAllowedCIDR` | `client:manage-network` | Adds a network range the client may connect from. Enforced at `POST /auth/client/token` against the origin address as this process resolves it. |
| `PATCH /clients/{id}/allowedCIDRs/{clientAllowedCIDRId}/archive` | `archiveClientAllowedCIDR` | `client:manage-network` | Removes a range from the allow-list. |
| `POST /clients/{id}/claims` | `addClientClaim` | `client:set-claim` | Sets the value this client carries for one catalog definition. |
| `PATCH /clients/{id}/claims/{clientClaimId}` | `patchClientClaim` | `client:set-claim` | Corrects the value — body carries only `value`. |
| `PATCH /clients/{id}/claims/{clientClaimId}/archive` | `archiveClientClaim` | `client:set-claim` | Withdraws the value. |

## Groups

| Endpoint | GraphQL | Permission | Description |
|---|---|---|---|
| `POST /groups` | `createGroup` | `group:insert` | Creates a group that bundles roles for the users placed in it. |
| `PATCH /groups/{id}` | `patchGroup` | `group:update` | Partial update of the group's descriptive attributes. |
| `PATCH /groups/{id}/archive` | `archiveGroup` | `group:archive` | Retires the group; nobody can be placed in it any longer and it confers nothing. |
| `GET /groups` | `groups` | `group:read` | Paged listing of groups. |
| `GET /groups/{id}` | `group` | `group:read` | Reads one group with the roles it confers. |
| `POST /groups/{id}/roles` | `addGroupRole` | `group:grant` | Attaches a role to the group. Refuses an escalation the caller does not already hold. |
| `PATCH /groups/{id}/roles/{groupRoleId}/archive` | `archiveGroupRole` | `group:grant` | Withdraws a role from the group. |

## Roles

| Endpoint | GraphQL | Permission | Description |
|---|---|---|---|
| `POST /roles` | `createRole` | `role:insert` | Creates a role that bundles catalog permissions inside one tenant. |
| `PATCH /roles/{id}` | `patchRole` | `role:update` | Partial update of the role's descriptive attributes. |
| `PATCH /roles/{id}/archive` | `archiveRole` | `role:archive` | Retires the role so it can no longer be granted to anyone. |
| `GET /roles` | `roles` | `role:read` | Paged listing of roles. |
| `GET /roles/{id}` | `role` | `role:read` | Reads one role with the permissions it bundles. |
| `POST /roles/{id}/permissions` | `addRolePermission` | `role:grant` | Adds a catalog permission to the role. **`*:*` is refused here, always, with no caller exempt.** |
| `PATCH /roles/{id}/permissions/{rolePermissionId}/archive` | `archiveRolePermission` | `role:grant` | Removes a permission from the role. |

## Permissions (the catalog)

`permissions` has no `tenant_id` column: the catalog is platform-wide. Update carries the
description only — `resource` and `action` are reachable by no request after creation.

| Endpoint | GraphQL | Permission | Description |
|---|---|---|---|
| `POST /permissions` | `createPermission` | `permission:insert` | Adds an enforceable `resource:action` pair to the platform catalog. |
| `PATCH /permissions/{id}` | `patchPermission` | `permission:update` | Changes the wording that explains what the entry lets a caller do. |
| `PATCH /permissions/{id}/archive` | `archivePermission` | `permission:archive` | Retires the entry so no role can bundle it any longer. |
| `GET /permissions` | `permissions` | `permission:read` | Paged listing of catalog entries. |
| `GET /permissions/{id}` | `permission` | `permission:read` | Reads one catalog entry. |

## Claims (the catalog of token claims)

| Endpoint | GraphQL | Permission | Description |
|---|---|---|---|
| `POST /claims` | `createClaim` | `claim:insert` | Adds a definition of a claim a token may carry: name (`x_` prefix required), value type, which identity kinds it applies to, and an optional tenant-wide default. Capped at 20 active definitions per tenant per identity kind. |
| `PATCH /claims/{id}` | `patchClaim` | `claim:update` | Changes a definition that already exists. |
| `PATCH /claims/{id}/archive` | `archiveClaim` | `claim:archive` | Retires the definition. An archived definition **mints nothing**, including for a principal still holding a value for it. |
| `GET /claims` | `claims` | `claim:read` | Paged listing of definitions. |
| `GET /claims/{id}` | `claim` | `claim:read` | Reads one definition. |

There is deliberately no `claim:grant`: the catalog owns no collection, and the verb that sets a
*value* lives on the two principals that hold one (`user:set-claim`, `client:set-claim`).

## Platform surface (framework-mounted)

| Endpoint | Auth | Description |
|---|---|---|
| `GET /.well-known/jwks.json` | public | The public keys this service signs with. Mounted by the `auth.issuer.jwks` block; a document requiring a bearer could never bootstrap trust in itself. |
| `GET /livez` | public | Liveness probe. |
| `GET /readyz` | public | Readiness probe, including the database. |
| `GET /openapi.json` · `GET /docs` · `GET /` | public | The OpenAPI document and its UI; `/` redirects to `/docs`. |
| `POST /graphql` · `GET /graphql/ui` | token | The GraphQL endpoint and its playground. In `prd` the playground and introspection are off. |

## What a token carries

Beside the standard `sub` / `iss` / `aud` / `exp`, the access token mints nine platform claims:

| Claim | Meaning |
|---|---|
| `tenant_id` | The tenant's `id` — the same value every isolation filter compares, with no translation step. |
| `tenant_workspace` | The human-facing handle, so other services need not hold this one's tenant table. |
| `identity_kind` | `user` or `client` — person or machine, without cross-referencing any table. |
| `email` | The person's address (user tokens). |
| `name` | The display label of the principal. |
| `permissions` | The de-duplicated union of direct and group-inherited permissions, resolved in one statement. |
| `roles` · `groups` | What conferred them. |
| `must_change_password` | A restricted session: its `permissions` claim is reduced to `user:change-password` alone and it carries no custom claims. |

Tenant-defined claims are merged on top, **up to 20 per token**: the value set on the principal
wins, the definition's tenant-wide default fills in when none is set, and a claim with neither
is absent rather than empty. Each is minted in the JSON type its definition declares. The
reserved `x_` prefix means a tenant definition can never take over a platform claim name, and
the fixed set is assigned last so not even a row written by migration could.

---

## Proving it

The contract suite lives in `qa/`. It boots this service for real, against a throwaway database
it drops and recreates first, and asserts what the answers are supposed to be:

```bash
./qa/run.sh                  # every lane, fail-fast — the first RED stops the run
./qa/run.sh --all            # every lane, exhaustive sweep
./qa/run.sh user domain      # a subset, on that same runner — never a rival script
```

Seventeen lanes: a REST lane and a GraphQL lane for each of the seven aggregates, plus `domain`
(the rules this service's own specs promise), `security` (what it *refuses* — 401 per token rule,
the public-route split in both directions, 403 per authorization layer) and `audit` (what the
trail records, and what it redacts). The last full run — 2026-09-08, pin `v0.74.0` — was
**2224 cases green across 17/17 lanes**, with 33 families named as *not proven* in a column of
their own rather than folded into the pass count.

The verdict is rewritten in full after every lane into [`qa/qa-report.md`](qa/qa-report.md), so a
run killed halfway still leaves what it had proven. What each lane is meant to prove, and why, is
in `specs/qa/<entity>-contract/plan.md`.

---

## Repository layout

```
bootstrap/              composition root — Wire() assembles the ten features
internal/domain/        aggregates, rules, notifications
internal/domain/vos/    value objects — text predicates in runes and Unicode
internal/application/   commands, queries, DTOs, the seven translation catalogs
internal/web/           requests, responses, routes (REST + GraphQL registration)
internal/infra/         table schemas, repositories, relational views, readers
migrations/postgres/    numbered up/down pairs, applied at boot in dev
devops/                 the local bench (Postgres)
qa/                     executable contract suites (qa/run.sh)
specs/                  the approved models, generator inputs and capability plans
```

Everything in `internal/` is generated from `specs/omnicore-gen/<entity>.omnicore.yaml` **except**
the files named `*_manual.go` and two paths the spec language cannot express:

- **The credential path** — the generator gates operations by a closed set of lifecycle verbs and
  "change a credential" is not one of them: the Argon2id hasher, the rules entered by action
  name, the two commands and the two routes.
- **The token path** — the framework ships the Issuer as *methods*, never as HTTP endpoints, so
  all three token routes, both authentication readers, the refresh-token store and migration
  `0006` are this service's own. Both readers are anchored on Direct schemas rather than on the
  entity repositories: a sign-in protects no invariant and drives no lifecycle, so it reads rows.

## Configuration

Two profiles, selected by `APP_PROFILE`:

| File | Profile | Differences that matter |
|---|---|---|
| `microservice.dev.yaml` | `dev` | Migrations auto-run; GraphQL playground and introspection on; signing key from `start.sh`. |
| `microservice.prd.yaml` | `prd` | `migrations.autoRun: check` — a pending or dirty migration aborts the boot, because applying schema changes is a deploy step, not a side effect of a pod starting. Playground and introspection off. |

Both declare `auth.mode: jwt`, `auth.authorization.enabled: true` and a required tenant claim.
Every endpoint is written as `${VAR:default}`, so a deployment repoints it through the
environment without editing the file. `auth.publicRoutes` is an exact `METHOD /path` list
validated at boot: a typo, a wrong method or a trailing slash aborts the boot naming the
offender.

## Where the decisions are written down

The reasoning lives in the repository rather than in chat history:

- `specs/scaffold-service/spec.md` — the approved service-level model: posture, surfaces, why
  there is no Mongo.
- `specs/scaffold-entity/<entity>/spec.md` · `tasks.md` — the approved domain model per entity,
  the alternatives that were rejected, and the deviations between spec and generated code.
- `specs/evolve-entity/<slug>/spec.md` — an approved change to an entity that already exists.
- `specs/omnicore-gen/<entity>.omnicore.yaml` · `.gen-report.md` — the generator's input, and what
  it generated, refused, or left to be written by hand.
- `specs/implement/<slug>/plan.md` — for a capability rather than an entity: the integration
  semantics confirmed before any code, the impact map, and what was proven once it shipped.
- `ACCESS_MATRIX.md` — who reaches which rows, per endpoint.

## Changing things

Every command below is a skill of [omnicore-plugin](https://github.com/ClaudioSchirmer/omnicore-plugin)
**v0.67.0**, not something this repository carries.

| Goal | Tool |
|---|---|
| add an entity | `/omnicore:scaffold-entity` |
| change an existing one | `/omnicore:evolve-entity` — it edits the spec and regenerates |
| add a cross-entity read model | `/omnicore:scaffold-view` |
| wire a framework capability | `/omnicore:implement` |
| change the infrastructure posture | `/omnicore:configure` |
| upgrade the framework pin | `/omnicore:upgrade` |
| generate an end-to-end contract suite | `/omnicore:qa` |

Two rules that are easy to trip over:

- **Never hand-edit a generated file.** It is hashed; the next run refuses it and leaves the edit
  stranded. Change the spec and regenerate, or `omnicore-gen adopt` the file with a reason.
- **`*_manual.go` and the migration pairs are yours.** They are written once and never touched
  again — which is exactly why a schema change is a *new* numbered migration, never an edit to
  one that has already run.

Contribution constraints for this repository are in [`CLAUDE.md`](CLAUDE.md).
