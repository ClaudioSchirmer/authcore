# QA plan — `tenant-contract`

- **Status:** APPROVED — maintainer (Cláudio Schirmer Guedes), 2026-09-06. §3c option **(a)** approved: the tenant gate is proven through a second, short boot on a suite-owned keypair.
- **Suite slug:** `tenant-contract` — what this round proves: the whole wire contract of the
  `Tenant` aggregate, on both surfaces it is mounted on, plus the business rules its own
  specs declare and the security boundary that guards it.
- **Written:** 2026-09-06
- **Pin:** omnicore **`v0.74.0`** · dialect postgres · read backing **relational**
  (read-your-writes — every read-back below is IMMEDIATE; a poll would itself be a failure)
- **Surfaces in scope:** REST + GraphQL (full parity — dev decision, 2026-09-06)
- **Plan destination:** `specs/qa/tenant-contract/plan.md` · **suite destination:** `qa/` at the
  project root · **verdict destination:** `qa/qa-report.md`
- **Prior rounds:** none. `specs/qa/` did not exist before this run; `qa/` did not exist
  before this run. This suite therefore CREATES `qa/run.sh` and every lane it names.

## 0. Where every expectation below comes from

Nothing here was read off a running service. The service was never called while this plan
was written, with one exception noted in §5 (the runner reads `GET /openapi.json` at boot to
pin the exact route strings — an enumeration of what EXISTS, never a value that becomes an
expectation).

| Source | What it settled |
|---|---|
| `specs/scaffold-entity/tenant/spec.md` (APPROVED, amended 2026-08-24, superseded note 2026-09-06) | the model, the two-identifier story, §A.2's mutability doctrine, §B Q1–Q10 |
| `specs/omnicore-gen/tenant.omnicore.yaml` | fields, VOs, notifications, modes, rules (declarative + manual), `service.facts`, the read vocabulary, `authz` |
| `internal/domain/tenant.go`, `internal/domain/tenant_rules_manual.go`, `internal/domain/vos/tenant_workspace.go` | the rules as implemented, the reserved-handle list, the normalization used by `description-differs` |
| `internal/web/tenant_routes.go` | the 6 REST endpoints + the 6 GraphQL fields and the permission on each |
| `internal/web/requests/find_tenants_by_params.go`, `find_tenant_by_id.go`, `insert_tenant.go`, `patch_tenant.go` | the DTO opt-in gate: which reserved controls each read endpoint serves, which operators each leaf admits, which fields a write body carries |
| `internal/application/queries/find_tenants_by_params_query.go` | `ToCriteria` returns the criteria unchanged — Tenant carries **no** identity-derived row filter (this is what makes §3's layer 2 `N/A`) |
| `migrations/postgres/0012_bootstrap_seed_manual.up.sql` | the token source: `admin@authcore.local` / `admin`, holding the wildcard `*:*` through the `master` role |
| `microservice.dev.yaml` (`auth:` block) | `mode: jwt`, `authorization.enabled: true`, `tenant.required: true`, the exact `publicRoutes`, and that **this service mints its own tokens** (`auth.issuer`) |
| pin docs — `status-mapping`, `auth-middleware`, `authz-seams`, `auto-handlers`, `auto-query-handlers`, `lifecycle-map`, `graphql`, `audit`, `migrations`, `bootstrap` | every status code, notification key and envelope shape asserted below |
| pin source — `application/persistence/scoped_reader.go`, `application/handlers/{archive,unarchive}.go`, `infra/db/command/read/base_aggregate_repository.go`, `infra/db/criteria/builder.go`, `infra/db/query/engine/relational/relational_view_reader.go`, `infra/audit/persister.go` | the load scopes behind archive/unarchive/patch (why they answer **404**, not 409), the page ceiling **100**, the `audit_events` column list |
| **the maintainer, asked 2026-09-06** (`AskUserQuestion`) | §1b rows 6, 10 and 11; the §2 hygiene mode; the GraphQL reach; the criticality ranking |

---

## 1. Coverage matrix — the FRAMEWORK's promises

Entity **Tenant** × surfaces **REST** and **GraphQL**. Read backing **relational**, so every
write→read-back below is immediate; the archive regime is *kept-but-hidden* (no
`DeleteOnArchive`), and the aggregate is **flat** (no children, no siblings, no shared base).

Envelope asserted on REST: `errors[].messages[].notificationKey` + the HTTP status.
Envelope asserted on GraphQL: HTTP is **always 200**, and the same key rides
`errors[].extensions.notificationKey` — never the REST envelope. Every request pins
`Accept-Language: en-US`.

### F1 — Happy path, one per served verb

| REST | GraphQL twin | Expected |
|---|---|---|
| `POST /tenants` | `createTenant(input:)` | **201** · body is the record AS STORED |
| `GET /tenants/{id}` | `tenant(id:)` | **200** · the full document |
| `GET /tenants` | `tenants(...)` | **200** · `data` + `pagination` / the Relay connection |
| `PATCH /tenants/{id}` | `patchTenant(id:, input:)` | **200** · the record after the change |
| `PATCH /tenants/{id}/archive` | `archiveTenant(id:)` | **204, NO BODY** / payload `{success, id}` |
| `PATCH /tenants/{id}/unarchive` | `unarchiveTenant(id:)` | **204, NO BODY** / payload `{success, id}` |

`Modes()` declares exactly `display, insert, update, archive, unarchive` — six verbs, six
routes, and the inventory is cross-checked against `GET /openapi.json` at boot. A
source-vs-openapi disagreement is a FINDING, not something the suite reconciles silently.

### F2 — Golden-record round-trip

One record exercising **every declared field**, written then read back field-by-field on all
three read shapes (REST by-id, REST listing row, GraphQL node):

`name` · `workspace` · `description` · `status` · `id` · `createdAt` · `updatedAt` ·
`archivedAt`.

`archivedAt` is asserted through its whole life: `null` while active → stamped after archive
(visible only via `?includeArchived=true`) → `null` again after unarchive. There is no
composite value object on this entity, so the exposed-parts rule has nothing to bite on.

### F3 — Validation, 422, notification KEY asserted (never prose)

| Input | Key |
|---|---|
| `name` empty | `RequiredFieldNotification` |
| `name` = `aaaa` (run of 4 identical runes) | `InvalidDisplayNameNotification` |
| `workspace` = `acme_corp` (illegal alphabet) | `InvalidTenantWorkspaceNotification` |
| `workspace` = `admin` (well-formed, reserved) | `ReservedTenantWorkspaceNotification` |
| `workspace` = `id` (2 runes AND reserved) | **both** keys in one 422 — the VO evaluates both branches on purpose |
| `description` = `abc` (under 15 runes) | `InvalidDescriptionNotification` |
| `status` = `paused` | `UnknownTenantStatusNotification` |

### F4 — The 409 family

| Case | Expected |
|---|---|
| Insert a workspace held by an **active** tenant | **409** `TenantWorkspaceAlreadyExistsNotification`, semantic `"Conflict"` |
| Insert a workspace held by an **archived** tenant | **409**, the same key — `service.facts.WorkspaceTaken` sets `activeOnly: false` on purpose. **Ranked critical by the maintainer.** |

**The wrong-state 409 is `N/A` on this entity, and the reason is worth writing down.**
`EntityIsNotActiveNotification` / `ConcurrentModificationNotification` (semantic
`"StateConflict"`) are not reachable through this aggregate's HTTP surface: there is no PUT
verb, no wire field carries a revision the caller could send stale, and every "wrong state"
attempt is intercepted one layer earlier by the LOAD scope — see F8, where they land as
**404**. Asserting a 409 there would encode a promise the pin does not make.

### F5 — Archive round-trip

`archive` → `204` · by-id → **404** `RecordNotFoundNotification` · by-id
`?includeArchived=true` → **200** with `archivedAt` stamped **and `status: "suspended"`**
(§1b row 4) · listing → row absent · listing `?includeArchived=true` → row present ·
`unarchive` → `204` · by-id → **200**, `archivedAt` back to `null`, **status still
`suspended`** (§1b row 5).

No `DeleteOnArchive`, so absence is never the expectation. No child table, so the
stamp-scoped child unarchive is `N/A — flat aggregate, no child declares an archive column`.

### F6 — Read vocabulary (everything the DTO declares, and only that)

- **Filters, one per declared operator family**: `name` `eq,in,startswith,contains,istartswith,icontains` ·
  `workspace` `eq,in,startswith,istartswith` · `description` `contains,icontains` ·
  `status` `eq,in` · `createdAt` `eq,gte,lte,gt,lt` · `updatedAt` `eq,gte,lte,gt,lt`.
  Wire form `?name.startswith=Acme`; GraphQL `where: { name: { startswith: "Acme" } }`.
- **`?orderBy=`**: `name`, `-name`, `workspace`, `createdAt` — the three the spec declares
  sortable, in both directions. GraphQL: `orderBy: [{field: NAME, direction: DESC}]`.
- **`?fields=id,workspace`** → exactly those two present, every other key absent (not null).
  GraphQL's equivalent is the selection set itself.
- **`?onlyTotal=true`** → `{success, status, description, pagination: {totalCount}}` — no
  `data`, no `hasNextPage`, no cursors.
- **`?last=N` alone** → the TAIL window, `hasNextPage: false`.
- **Pagination envelope as a contract**: `pagination.totalCount` truthful against a known
  seeded set · `hasNextPage`/`hasPreviousPage` at both ends · `startCursor`/`endCursor` are
  WINDOW EDGES — walked by echoing `endCursor` into `?after=` — · page-2 disjoint from
  page-1 · backward via `last` + `before` returns the previous window exactly.
  **And the EMISSION rule, which is easy to get wrong from the generic Relay convention:** an
  edge cursor is emitted only where its neighbouring page exists — `endCursor` exactly when
  `hasNextPage`, `startCursor` exactly when `hasPreviousPage`. The head of a forward walk
  therefore carries **no** `startCursor`, and an absent edge means "nothing to walk to on this
  side", never a contradiction with the flag beside it (`auto-query-handlers` at the pin).
- **`?includeArchived=true`** on both the listing and the by-id read.

### F7 — Rejected reads: the whole typed-400 guard family

Every row asserts the KEY **and** the `field` the envelope names.

| Request | Status · key · field |
|---|---|
| `?bogus=x` | 400 · `SchemaViolationNotification` · `bogus` |
| `?status.contains=act` (operator outside the leaf's allowlist) | 400 · `SchemaViolationNotification` |
| `?description=x` (bare `eq`, which `Description` does not declare) | 400 · `SchemaViolationNotification` |
| `?search=foo` | 400 · `SchemaViolationNotification` on `search` — **the DTO opt-in gate**, see the note below |
| `?fields=bogus` on the listing | 400 · `SchemaViolationNotification` · `fields[bogus]` |
| `?fields=id` on the **by-id** route (which declares only `includeArchived`) | 400 · `SchemaViolationNotification` · `fields` |
| `?createdAt=lixo` (value outside the leaf's kind) | 400 · `InvalidFilterValueNotification` — pin ≥ v0.70.0 |
| `?first=101` | 400 · `LimitExceededNotification`, `FieldValue` = **100** |
| `?first=5&last=5` · `?first=5&before=X` · `?after=X&before=Y` | 400 · `SchemaViolationNotification` on the backward-side key |
| `?onlyTotal=true&fields=id` / `&orderBy=name` / `&first=10` / `&after=X` | 400 · `SchemaViolationNotification` · `onlyTotal[fields]`, `onlyTotal[orderBy]`, `onlyTotal[first]`, `onlyTotal[after]` |
| `?onlyTotal=1` · `?onlyTotal=` · `?includeArchived=1` | 400 on the control's own key — the booleans take exactly `true` / `false` |
| `?onlyTotal=false&first=10` | **200** — present-but-inactive never trips the conflict matrix |
| `?onlyTotal=true&status.eq=active` · `&includeArchived=true` | **200** — counting a filtered subset is the canonical use |
| `?after=not-a-cursor` | 400 · `SchemaViolationNotification` |
| a cursor minted under the default order, replayed with `?orderBy=name` | 400 · `SchemaViolationNotification` (cursor↔orderBy) |
| a cursor minted without archived rows, replayed with `?includeArchived=true` | 400 · `SchemaViolationNotification` |
| `?orderBy=status` (filterable, deliberately not sortable) | 400 · `SchemaViolationNotification` · `orderBy[status]` |
| `?orderBy=-updatedAt` (the spec's own deliberate cut: filterable, not orderable) | 400 · `SchemaViolationNotification` · `orderBy[-updatedAt]` |

**Why `?search=` is a DTO-gate rejection here and not `UnsupportedCapabilityNotification`.**
`FindTenantsRequest` declares no `Search` field, so the allowlist refuses the key at the wire
wrapper — before any engine is reached. The relational capability refusal
(`UnsupportedCapabilityNotification`) would need a control the DTO DOES declare and the
backing cannot serve; on this entity that combination does not exist (no `search` declared,
no 1:N child to filter or sort across). **`UnsupportedCapabilityNotification` is therefore
`N/A` for Tenant, by construction, and asserting it would be asserting a lie.**

### F8 — Addressing and absent verbs

| Request | Expected |
|---|---|
| `DELETE /tenants/{id}` · `PUT /tenants/{id}` · `POST /tenants/{id}` · `GET /tenants/{id}/archive` | **405** `MethodNotAllowedNotification` — the path is registered, the method is not |
| `GET /does-not-exist` | **404** `RouteNotFoundNotification` |
| `GET /tenants/{unknown-uuid}` | **404** `RecordNotFoundNotification` |
| `GET /tenants/not-a-uuid` (a **read** address) | **404** `UnknownIDAddressNotification` |
| `PATCH /tenants/not-a-uuid` · `/archive` · `/unarchive` (a **write** intention) | **400** `MalformedIDNotification` |
| `unarchive` on an **active** tenant | **404** `RecordNotFoundNotification` — `FindArchivedByID` runs `OnlyArchived` |
| `archive` on an **already-archived** tenant | **404** — `LoadForWrite` runs the default `ScopeActive` |
| `PATCH` on an **archived** tenant | **404** — same scope |

The by-id split by VERB (404 for the read, 400 for the write) is the family an existing suite
is most likely to miss; both halves get a case, on both surfaces.

**The 403 mode-not-allowed shape is `N/A` here**: Tenant declares every mode it mounts, and
the one verb it does not serve (`DELETE`) has no route at all, so it lands on the 405 branch
above. There is no `…NotAllowedNotification` reachable on this entity.

### F9 — Workspace immutability, proven by EFFECT

`PatchTenantRequest` carries only `name`, `description`, `status` — the handle is
structurally absent from the update body (`patchExcludes: [Workspace]`). PATCH is the
**lenient** handler (no `FullBody` marker), so an unknown key is ignored rather than refused.

- **Case**: `PATCH` with `{"name": "...", "workspace": "hijacked"}` → **200**, and a read-back
  proves the handle is unchanged.
- **Consequence recorded honestly**: `TenantWorkspaceIsImmutableNotification` is
  **unreachable through any mounted surface**. The declarative `immutable` rule is a
  belt-and-braces layer behind a structural cut, exactly as the yaml says. The suite asserts
  the effect and does NOT assert a notification no request can provoke.

### F10 — GraphQL, where the idiom differs BY DESIGN

- Every rejection family of F3/F4/F7/F8 repeated over `POST /graphql`, asserting
  `errors[].extensions.notificationKey` at HTTP **200**.
- An **undeclared argument** (`search:`) is cut out of the schema entirely — `gqlparser`
  answers an unknown-argument validation error before any resolver. The suite asserts THAT,
  never the REST envelope.
- `fields` and `onlyTotal` have no argument on GraphQL — selection-natural. Asserted as
  absent from the schema, not as a 400. **Consequence worth stating, because it changes what a
  case may select:** asking for `totalCount` ALONE *is* the only-total mode on this surface, so
  a page-shaping argument beside it (`first:`) trips the only-total conflict matrix rather than
  the ceiling. A case about the ceiling must therefore select `edges` too.
- `orderBy: [{field: STATUS}]` → a schema-validation error (the value is not in
  `TenantOrderField`), not a runtime 400.
- **`__typename` beside a normal selection must answer identically** — the v0.72.1 regression
  guard, kept as a standing case even though Tenant declares no `Restrict`.
- **Handler invariance**: the same operation on both surfaces produces the same effect on a
  read-back and the same notification key.

---

## 1b. Domain expectations — what the BUSINESS requires

These are the only rows in this plan the framework never had an opinion about. Source is
named per row; `asked` means the maintainer answered on 2026-09-06 and the answer is recorded
verbatim. Ranked by the cost the maintainer named — rows 1–4 were all called critical.

| # | The rule | Source | POSITIVE case | NEGATIVE case | Rank |
|---|---|---|---|---|---|
| 1 | "An archived remnant MUST keep blocking the handle, because it is what URLs, logs and support conversations carry." | `spec.md` §A.2 rule 5 + `service.facts.WorkspaceTaken activeOnly: false` | insert with a fresh handle → **201** | insert with an **archived** tenant's handle → **409** `TenantWorkspaceAlreadyExistsNotification` | **critical** |
| 2 | "The handle reaches URLs, logs and external configuration; changing it breaks all three." | `spec.md` §A.2 rule 7 + `rules.list.workspace-immutable` + `patchExcludes` | PATCH `name`/`description`/`status` → **200**, handle unchanged | PATCH carrying `workspace` → **200** and the handle **still unchanged** (F9: the notification is unreachable by design) | **critical** |
| 3 | "A trial is a beginning — no tenant returns to it." | `rules.list.status-transition` | `trial→active`, `active→suspended`, `suspended→active` → **200** | `active→trial` and `suspended→trial` → **422** `InvalidTenantStatusTransitionNotification` | **critical** |
| 4 | "Archiving forces Status to suspended… archived+active becomes an unrepresentable state." | `spec.md` §B Q10 + `rules.manual.archive-forces-suspended` | archive an **active** tenant → `204`, then `?includeArchived=true` reads `status: "suspended"` | over the whole archived set, **no** row ever reads back `active` or `trial` | **critical** |
| 5 | "Unarchiving brings the tenant back suspended by consequence." | same rule's own wording | unarchive → by-id **200**, `archivedAt: null`, `status: "suspended"` | the pre-archive status is **not** restored (assert `!= "active"`) | high |
| 6 | "The description must differ from both Name and Workspace under a normalized comparison — case-folded, with whitespace and hyphens collapsed." | `rules.manual.description-differs-from-name-and-workspace` + **asked**: *"Passar está certo"* — a description that merely CONTAINS the name is legal | name `Acme Comercio`, description `Acme Comercio is the retail arm of the group in Brazil.` → **201** | description `acme-comercio` against name `Acme Comercio` → **422** `TenantDescriptionMustDifferNotification`; and the same against the workspace | high |
| 7 | "Not a member of the reserved list (platform routes and phishing-prone words)." | `vos/tenant_workspace.go` | a non-reserved handle → **201** | `admin`, `docs`, `graphql`, `readyz`, `tenants` → **422** `ReservedTenantWorkspaceNotification` | high |
| 8 | "NOTHING IS NORMALIZED. A value that does not already comply is refused, never quietly repaired." | `vos/tenant_workspace.go` | `3m-brasil` (leading digit, RFC 1123 relaxed) → **201** | `ACME-CORP`, `-acme`, `acme-`, `ac`, `aaaa` → **422** `InvalidTenantWorkspaceNotification`, **and no normalized variant is ever stored** | high |
| 9 | Anti-junk on the two shared text types: name 2–120 runes with a letter and ≥ min(3,len) distinct runes; description 15–500 runes, ≥2 words, ≥5 distinct runes, ≥1 vowel, no run of 4. | `tenant.omnicore.yaml valueObjects` + the VO sources | a real company name and a real description → **201** | the boundary values on each side → **422** with the matching key | medium |
| 10 | Insert accepts **all three** statuses; the transition table guards the UPDATE only. | **asked** — *"Sim — os três são legais no insert"* | insert with `trial`, with `active` and with `suspended` → **201** each | — (by the maintainer's decision there is no refusal here; the negative lives in row 3) | medium |
| 11 | The seeded `master` tenant is archivable — *"Aceitável — é operação de plataforma"* (**asked**). | **asked** | — | — | recorded |

**Row 11 carries no case on purpose, and that is stated rather than assumed.** Archiving
`master` inside the QA database would suspend the tenant that owns the wildcard role and the
bootstrap administrator, invalidating the token every later case depends on. The suite
therefore never calls it, and the decision is recorded here so a future round does not read
the absence as an oversight.

**Nothing in §1b is UNPROVEN this round** — every row the maintainer left open was answered
on 2026-09-06 before this plan was written.

---

## 2. Data hygiene ⚠️ high-risk — the maintainer decided: **a dedicated throwaway database**

*"Banco descartável próprio da suíte"* (2026-09-06). The reason this mattered more than usual
on this entity: **a workspace handle is never reused, archived rows included** (§1b row 1), so
every run that inserts tenants into a database burns handles in it permanently.

| Decision | Value |
|---|---|
| Database | `authcore_qa`, on the same bench container (`authcore-dev-postgres`, already healthy) |
| Selection | a **suite-owned** config `qa/microservice.qa.yaml`, reached via `OMNICORE_CONFIG_PATH` + `APP_PROFILE=qa` — the project's own `microservice.dev.yaml` is **never touched** (this skill writes only under `specs/qa/` and `qa/`) |
| Migrations | `migrations.autoRun: true` **declared explicitly** — unset resolves to `true` only under `APP_PROFILE=dev`, and to the strict `check` mode under every other profile, which would abort the boot on a fresh database |
| Seed | migration `0012` runs itself → the permission catalog, the `master` tenant, the `master` role with `*:*`, and `admin@authcore.local` / `admin` |
| Port | `:8099`, with `AUTH_SELF_URL=http://localhost:8099` so `auth.issuer.selfUrl`, `auth.jwt.issuer` and `jwksUrl` stay equal to each other (the framework refuses the boot otherwise) — and so a dev instance on `:8080` is never disturbed. The runner probes the port first and frees it with SIGTERM before booting. |
| Signing key | reuses `devops/dev-signing-key.pem` through the same `JWT_SIGNING_KEY` / `JWT_SIGNING_KID` interpolation `start.sh` uses; generated the same way if absent |
| **Reset between runs** | `DROP DATABASE IF EXISTS authcore_qa` + `CREATE DATABASE`, executed through `docker exec` on the bench container **before** each boot. There is **no Mongo and no CDC** in this posture, so there is no projection to clear and no drain to wait out — the relational drop IS the whole reset, which is what makes the exact-count assertions of F6 legitimate. |
| Residue | none in any database the maintainer cares about. `authcore_qa` is left in place after the run (dropped and recreated by the next one) so a failure can be inspected. `authcore_db` is never opened. |

---

## 3. Security — never `N/A`

### 3a — The 401 half (no valid token needed; built in full)

Keys from `auth-middleware` at the pin. Config read from the `auth:` block: `algorithms:
[RS256]`, `issuer`/`audience` from `AUTH_SELF_URL`/`AUTH_AUDIENCE`, keys via `jwksUrl`, and
**no `leewaySeconds` declared** (framework default).

| Case | Expected |
|---|---|
| no `Authorization` header | 401 · `MissingAuthorizationNotification` |
| `Authorization: Basic abc` (wrong scheme) · `Authorization: Bearer` (empty value) | 401 · `MissingAuthorizationNotification` |
| `Authorization: bearer <valid>` (lowercase scheme) | **2xx — a PASS case**: the scheme match is case-insensitive |
| `Authorization: Bearer not-a-jwt` | 401 · `InvalidTokenNotification` |
| a well-formed RS256 JWT signed with a **foreign** key | 401 · `InvalidTokenNotification` |
| a token with the wrong `iss` | 401 · `InvalidTokenNotification` |
| a token whose `aud` omits the configured audience | 401 · `InvalidTokenNotification` |
| an **HS256**-signed token where the allowlist is `[RS256]` (algorithm confusion) | 401 · `InvalidTokenNotification` |
| a token with `exp` in the past | 401 · **`ExpiredTokenNotification`** — asserted as a distinct key, because that split is what lets a client branch refresh-vs-reauthenticate |

The forged tokens are produced by the suite from its own throwaway keypair. Signing a
deliberately INVALID token needs no secret from anybody and asserts a refusal — it is not the
"invent a credential" this skill forbids.

### 3b — The public-route half, asserted in BOTH directions

Declared in `auth.publicRoutes` (exact `METHOD /path`, no prefix matching):
`GET /livez` · `GET /readyz` · `POST /auth/user/token` · `POST /auth/user/token/refresh` ·
`POST /auth/client/token`.

- **Direction 1 — every declared entry answers tokenless.** Including the probes, which are
  public *here only because this project opted them in* — they are not framework-public.
- **Direction 2 — a route that is NOT declared answers 401 tokenless.** `GET /tenants` with no
  token. This is the case that catches a `publicRoutes` entry widened past its intent.
- **Exactness made visible**: `GET /auth/user/token` (same path, other method) → 401 or 405,
  never a tokenless 200; `POST /auth/user/token/refresh/extra` (sibling sharing a prefix) →
  not public.
- **The framework's own appended surfaces**, reachable tokenless without an entry:
  `GET /openapi.json` · the `/docs` UI · `GET /` (because `openapi.rootRedirect: true`) ·
  `GET /.well-known/jwks.json` (appended by the `auth.issuer.jwks` block) · the GraphQL
  playground at `/graphql/ui`.
- **The introspection bypass and its edges** (maintainer asked for it explicitly;
  `graphql.introspection: true`): an introspection-only document passes tokenless, while a
  data field beside `__schema`, a decoy introspection operation next to a real one, a root
  fragment spread, and a mutation each stay behind the bearer.

### 3c — The 403 half. **Where a valid token comes from: this service issues its own.**

`auth.issuer.enabled: true` and `POST /auth/user/token` is public, so the suite obtains every
valid token through the service's own documented flow. **Nothing is invented and no external
IdP is needed.**

- **Principal A (privileged)** — `admin@authcore.local` / `admin`, seeded by migration `0012`,
  holding `*:*` through the `master` role.
- **Principal B (unprivileged)** — created BY the suite, through the API, using principal A's
  token: a role carrying only `tenant:read`, and a user holding it. It exists so the layer-1
  negative is a real principal and not a hypothetical.

**Both need their password rotated before they can do anything, and that is a property of this
service rather than an obstacle** (verified against the running seed while the suite was being
generated; the finding is about how a token is OBTAINED, not about what any case expects).
Migration `0012` sets `must_change_password = TRUE` on the seeded admin — the literal password
`admin` is the one value in that file the API itself would refuse — and a user created through
`POST /users` is born the same way, because an admin-set password has to be replaced by its
owner. **A token minted for an account in that state carries exactly ONE permission,
`user:change-password`; every other verb answers 403 until the credential is rotated.** The
runner therefore rotates both, inside its own throwaway database, through
`PATCH /users/{id}/password` — the route that narrowed token does reach — and signs in again
for the real token. Without that step §3c's complement case ("a gate that refuses everyone is
also broken") would pass for entirely the wrong reason.

| Layer | Case | Expected |
|---|---|---|
| **1 — `RequirePermission`** | principal B (`tenant:read` only) → `POST /tenants` | **403** `MissingPermissionNotification` |
| | the same for `PATCH /tenants/{id}`, `/archive`, `/unarchive` | 403, same key |
| | **the complement** — principal B → `GET /tenants` and `GET /tenants/{id}` | **200**: a gate that refuses everyone is also broken |
| | principal A → every one of the six verbs | 2xx |
| | the same eight rows on **GraphQL**, where the key rides `errors[].extensions` with `field: "permission"` — a route gated on REST is not thereby gated on GraphQL | |
| **2 — identity-derived `BuildRules` / `ToCriteria`** | — | **`N/A`, and the reason is a decision, not a gap**: `authz.dataAccess: anyone-with-permission` (§B Q4 — *"anyone holding the permission sees and edits every row"*), `FindTenantsByParamsQuery.ToCriteria` returns the criteria unchanged, `BuildRules` reads no principal field, and no `Restrict` is declared. There is no per-row or per-field boundary on Tenant to assert. Stated here so a future round does not read the silence as an oversight. |
| **3 — tenant scoping** | `authorization.tenant.required: true` — a token with an **empty** tenant claim must be refused with **403** `TenantMissingNotification` before any handler runs | see the proposal below |
| | a cross-tenant READ leaking another tenant's row | **`N/A` by the same §B Q4 decision** — Tenant is a platform registry with no row scope; there is no isolation on this entity to leak |

**⚠️ The one open item in this section: how to reach the tenant-gate 403.** Every token this
service mints carries `tenant_id` (`users.tenant_id` is `NOT NULL`), and validation resolves
keys through the service's own JWKS — so a token with no tenant claim cannot be obtained from
the service, and a suite-signed one would not validate against that JWKS. Two ways forward,
and **the maintainer picks one at this gate**:

- **(a) Prove it — a second, short boot on a suite-owned keypair.** `qa/microservice.qa-key.yaml`
  is identical to `qa/microservice.qa.yaml` except that `auth.jwt` validates through
  `publicKeyPem` (the public half of a keypair the suite generates) instead of `jwksUrl`. The
  suite then signs two tokens itself — one with a `tenant_id` claim and one without — and
  asserts **403 `TenantMissingNotification`** for the second and **2xx** for the first. This
  invents no credential of the maintainer's: the suite is the issuer and says so. It costs one
  extra boot + shutdown inside `qa/security.sh` (~5s), and nothing the project owns is touched.
- **(b) Do not prove it.** The plan and the report both state that the tenant gate is
  **UNPROVEN**, printed in the SKIP column, with this exact reason.

**This plan proposes (a).** The gate is one of the two the maintainer ranked critical
("gate de permissão abrir"), and it is the one gate no other case in the suite touches.

### 3d — `auth.mode` posture

Not applicable as an excuse: `mode: jwt` and `authorization.enabled: true` in both profiles
(`disabled` was removed from the dev bench on 2026-08-26 precisely so these tests would run).
The suite runs against a QA config that keeps that posture unchanged.

### 3e — Coverage reported, not asserted in prose

Whatever 3a–3c leaves out — layer 2 and cross-tenant isolation, both `N/A` by decision, and
the tenant gate if option (b) is chosen — is printed by the run in the SKIP column of §6's
report, with the reason. A security family that never executed is exactly where "no failures"
reads most like "we are safe".

---

## 4. Out of scope, named plainly

- **Load and performance**, and any UI.
- **gRPC** — no `transport:` block, no transport build tag, no procedures mounted. Nothing to
  assert.
- **Tabular exports** — `surfaces` declares no CSV/XLSX for Tenant.
- **Integration events** — this service declares no `transport:` block and no relay, so it
  publishes nothing to a broker. The framework's `outbox` table exists (embedded control
  plane) but nothing consumes it. **Not `⚠️ OPEN`: there is no delivery half to defer.**
- **The other six aggregates** (user, client, role, group, permission, claim) — touched only
  as the suite's token source (§3c principal B), never asserted. A later `specs/qa/<suite>/`
  extends the same `qa/run.sh`.
- **The token/credential aggregates' own contract** — password rules, refresh rotation, client
  CIDR enforcement, attempt counters. Out of this round entirely.

### In scope by the maintainer's explicit ask (2026-09-06): the audit trail

Every write in this service emits an in-TX `audit_events` row (the `database` destination is
on by framework default, and the yaml declares `auditClaims`). It is always provable on a
relational posture, so it earns its own lane:

- one row per `insert` / `update` / `archive` / `unarchive`, with `entity_type = 'Tenant'` and
  `aggregate_id` = the tenant's id;
- `actor` = the acting principal's `sub`, and `NULL` for an anonymous write;
- `tenant_id` = the actor's tenant claim;
- `payload` carrying the declared `auditClaims` — `tenant_workspace`, `email`, `name`,
  `identity_kind` — and the `changes` block on a transition that persisted a change;
- the archive row records the transition, and the unarchive row records the explicit null.

Asserted by SQL through `docker exec psql` against `authcore_qa`, never through an endpoint.

---

## 5. Runner contract

**Exactly one entry point: `qa/run.sh`.** One command for the dev, one line for CI.

```
qa/
├── run.sh                    ← THE runner. Its lane list is the whole inventory.
├── lib/common.sh             ← sourced helpers (assertions, curl wrappers, JSON probing,
│                               token minting). NOT a lane — it lives under lib/ so the
│                               `qa/*.sh` reconcile of the final verify stays exact.
├── tenant.sh                 ← F1–F9, REST
├── tenant_graphql.sh         ← F10 + F1–F8 repeated in the GraphQL idiom
├── domain.sh                 ← §1b, both surfaces
├── security.sh               ← §3a + §3b + §3c (incl. the second boot, if (a) is approved)
├── audit.sh                  ← the audit-trail family of §4
├── microservice.qa.yaml      ← suite-owned config (§2)
└── microservice.qa-key.yaml  ← only if §3c option (a) is approved
```

- **Lane list**: `tenant tenant_graphql domain security audit`, in that order, declared as an
  explicit array inside `run.sh` so what runs is readable in one place. `./qa/run.sh person`-style
  subsetting: `./qa/run.sh tenant domain` runs those two on the same runner — a convenience,
  never a rival script.
- **Root resolution**: `cd "$(dirname "$0")/.."` as the first act of `run.sh` **and of every
  lane**, so `devops/docker-compose.yml`, `migrations/` and the build resolve no matter where
  the dev invoked it from.
- **Fail-fast by default** — the first RED stops the run; `--all` sweeps every lane.
- **Every lane exits non-zero** when any of its cases failed (without that, fail-fast can
  never trip).
- **Per-run namespacing** — the run id (`$$`-timestamp) namespaces every temp file, the log
  file, the compiled binary **and the port**, so two runs never corrupt each other.
- **Boot sequence**, per `shared/boot-contract.md` and this project's own `start.sh`:
  1. `docker compose -f devops/docker-compose.yml ps` → `up -d --wait` anything missing;
  2. drop + create `authcore_qa`;
  3. ensure `devops/dev-signing-key.pem`, export `JWT_SIGNING_KEY` (literal `\n`) + `JWT_SIGNING_KID`;
  4. `go build -tags 'postgres' -o ./qa/.bin/authcore-<runid> ./bootstrap` — the engine tag from
     `relational.dialect: postgres`, and **no transport tag** because the yaml declares no
     `transport:` block;
  5. probe `:8099`; free it with SIGTERM if something answers;
  6. boot with `APP_PROFILE=qa OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml`, log to
     `qa/.logs/<run-id>/server.log`, poll `/readyz` **reading the 503 reason**;
  7. read `GET /openapi.json` once to pin the exact route strings and cross-check the verb
     inventory (an enumeration of what exists — never a value that becomes an expectation).
- **Shutdown**: SIGTERM only, **never** `kill -9`, and the runner WAITS on the server pid
  until the drain completes (`shutdown.drainTimeoutSeconds: 30` — budget 35s) before anything
  rebinds the port. Cleanup runs on `EXIT`.
- **Style**: POSIX-friendly bash + `curl`. Every request pins `Accept-Language: en-US`. Each
  case prints its name, its expectation and its verdict; a failed assertion prints the **real
  response body**, not a summary. `grpcurl` is not needed (no gRPC).

---

## 6. Report contract — the run leaves `qa/qa-report.md` behind

Rendered **live**, rewritten in full after every lane, so a run killed halfway still leaves
what it had proven.

- **Header**: timestamp · `APP_PROFILE=qa` + the tags actually built (`postgres`, no transport)
  · omnicore pin `v0.74.0` · hygiene mode (`throwaway database authcore_qa`) · lane count ·
  the plan it executes (`specs/qa/tenant-contract/plan.md`).
- **Matrix**, one row per lane: `| Suite | Pass | Fail | Skip | Verdict | Time |`. **Every
  declared lane appears** — one that never ran prints `—`, never vanishes.
- **Failures section, only when RED**: per failed case, its name, expected vs received, and the
  real response body (first few per lane), each pointing at `qa/.logs/<run-id>/`.
- **Footer**, printed to stdout beside the report's path:
  `✅ ALL GREEN — <n>/<n> suites · <cases> cases · <secs>s` or
  `❌ RED — <x> of <n> suites — logs: qa/.logs/<run-id>/`.
- **A trap on `EXIT INT TERM`** stamps `❌ RUN ABORTED — <reason>`, disarmed only once the final
  verdict is on disk. A stale green report is worse than no report.
- **SKIP keeps its own column** and is never folded into the pass count. §3e's unproven
  families land here.
- `qa/qa-report.md` and `qa/.logs/` are RUN ARTIFACTS — re-running the command reproduces them.
  **`qa/.logs` is already the last line of this project's `.gitignore`**; `qa/qa-report.md` is
  not, and will be **offered** at hand-off, never added: no skill edits `.gitignore`, and the
  plan and the suite themselves are project files that stay tracked.

---

## Approval

Two things need a word before anything is generated:

1. **The plan as a whole** — `Status: DRAFT` until you say otherwise.
2. **§3c** — option **(a)** (second short boot on a suite-owned keypair, proving the tenant
   gate) or option **(b)** (leave it unproven and printed as SKIP). The plan proposes **(a)**.

Nothing is written under `qa/` and nothing is executed until both are answered.
