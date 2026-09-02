# QA contract suite — Tenant

Status: APPROVED (2026-09-01)
Scope: entity `Tenant`, surfaces REST + GraphQL
Location: `specs/qa/<entity>/plan.md` — one plan per lane, matching this repo's
`specs/<skill>/<subject>/` convention, so a later entity's QA run adds a sibling folder
instead of overwriting this file. §5 (the runner contract) is SUITE-WIDE: a later lane
plan points here rather than restating it differently.
Pin: omnicore **v0.69.0** (`go list -m github.com/ClaudioSchirmer/omnicore`)
Profile under test: `dev` (`APP_PROFILE=dev`), engine tag `postgres`, no transport tag

Everything below is DERIVED — from `specs/omnicore-gen/tenant.omnicore.yaml`, from the
generated code actually mounted, and from the pin's own docs (`status-mapping`,
`auto-handlers`, `auto-query-handlers`, `query-side`, `graphql`, `lifecycle-map`,
`relational-view`). Where a promise could not be derived it is named as `N/A — <why>`
or flagged `⚠️ OPEN`, never guessed.

---

## 0. What the service declares (the test surface)

**Storage** — flat table `tenants`, managed `revision / created_at / updated_at /
deleted_at`. `delete.root: soft`, no hard-delete verb anywhere.

**Fields on the wire** — 5 written + 2 managed, no composite VO, no children, no
siblings, no SharedBase role:

| wire | VO | notes |
|---|---|---|
| `name` | `DisplayName` (manual) | 2–120 runes, anti-junk |
| `workspace` | `TenantWorkspace` (manual) | DNS label, reserved list, **immutable**, unique `scope: all` |
| `description` | `Description` (manual) | 15–500 runes, ≥2 words, ≥1 vowel |
| `status` | `TenantStatus` (enum) | `trial \| active \| suspended` |
| `createdAt` / `updatedAt` | framework-managed | read-only, `read.managed` |

`deletedAt` is deliberately NOT projected — archived state is reached only through
`?includeArchived`.

**Modes** — `[display, insert, update, archive, unarchive]`. Update shape `patch`,
`patchExcludes: [Workspace]` (the handle is not a member of the partial body at all).

**Routes actually mounted** (`internal/web/tenant_routes.go`), each behind
`RequirePermission`:

| verb | route | success | permission |
|---|---|---|---|
| insert | `POST /tenants` | 201 + body | `tenant:insert` |
| patch | `PATCH /tenants/:id` | 200 + body | `tenant:update` |
| archive | `PATCH /tenants/:id/archive` | 204 | `tenant:archive` |
| unarchive | `PATCH /tenants/:id/unarchive` | 204 | `tenant:archive` |
| list | `GET /tenants` | 200 + page envelope | `tenant:read` |
| by-id | `GET /tenants/:id` | 200 + document | `tenant:read` |

GraphQL mirrors all six (`tenants`, `tenant`, `createTenant`, `patchTenant`,
`archiveTenant`, `unarchiveTenant`), same handlers, same permissions.
This inventory is **cross-checked against `GET /openapi.json` at run time** (case
`X1`); a source-vs-openapi disagreement is reported as a finding, not reconciled
silently.

**Read backing** — `RelationalView("tenants", loader)`, no materialisation.
Therefore **read-your-writes**: every write→read-back assertion in this suite is
IMMEDIATE, and a case that only passes after a retry is itself a RED. No CDC, no
poll, no drain anywhere in this suite.

**Declared read controls** — listing DTO `FindTenantsRequest` declares
`first, last, after, before, orderBy, fields, onlyTotal, includeArchived`
(**`search` is NOT declared**); by-id DTO `FindTenantByIDRequest` declares
`includeArchived` and nothing else.

Filter vocabulary (`filter:` tags) and sort vocabulary (`sort:` tags):

| field | operators | orderable |
|---|---|---|
| `name` | eq, in, startswith, contains, istartswith, icontains | asc, desc |
| `workspace` | eq, in, startswith, istartswith | asc, desc |
| `description` | contains, icontains | — |
| `status` | eq, in | — |
| `createdAt` | eq, gte, lte, gt, lt | asc, desc |
| `updatedAt` | eq, gte, lte, gt, lt | — |

Page ceiling: no `query:` block in `microservice.dev.yaml` and no per-view override →
`bootstrap.FrameworkDefaultMaxLimit = 100`.

**Read joins** — N/A: the repository declares none, so there is neither a served
joined field to assert nor a rules-only field to prove absent from the wire.

**Infra posture** — Postgres only (`devops/docker-compose.yml`: one container). No
Mongo, no broker, no CDC relay, no `integration_events` table, no export surface, no
gRPC.

**Auth** — `auth.mode: jwt`, `authorization.enabled: true`, tenant claim
`required: true`. The bench is CLOSED; every tenant route needs a bearer. See §3.

---

## 1. Coverage matrix

Every family below is a real case in `qa/tenant.sh` unless marked `N/A`.
Assertions are always **status + notification KEY (+ field where the pin names one)**,
never prose — every request pins `Accept-Language: en` so the envelope is
deterministic (`parseLanguage` → `LangENG`).

### A — Sign-in and the permission gate (REST)

The seeded bootstrap admin has `must_change_password = TRUE`, and by the service's own
contract that first token carries **`user:change-password` and nothing else**. That
makes the gate free to prove:

- `A1` `POST /auth/user/token` (`admin@authcore.local` / `admin`) → **200**,
  `user.mustChangePassword == true`, `permissions == ["user:change-password"]`.
- `A2` `GET /tenants` with that restricted token → **403**,
  `MissingPermissionNotification`, `field: "permission"`, value `tenant:read`.
- `A3` `PATCH /users/<bootstrap-id>/password` with it → success; re-sign-in → token
  whose `permissions` contains `*:*` and `mustChangePassword == false`. This token is
  what every later case uses.
- `A4` `GET /tenants` with no `Authorization` header → **401**,
  `MissingAuthorizationNotification`.
- `A5` `GET /tenants` with a syntactically bogus bearer → **401**,
  `InvalidTokenNotification`.

### B — Happy path, one per served verb (REST)

`B1` insert → **201** + `data` mirroring the STORED entity ·
`B2` by-id → **200**, same values ·
`B3` list filtered to this record → **200**, exactly 1 row ·
`B4` patch `name`+`description` → **200**, new values, `workspace` untouched ·
`B5` archive → **204** · `B6` unarchive → **204**.
Each write is read back IMMEDIATELY (relational posture).

### C — Golden-record round-trip

One record exercising **every declared wire field**, written then read back
field-by-field on: REST by-id, REST listing row, GraphQL node. Catches a field
silently dropped from a DTO or a projection — the family every other one passes over.
`createdAt`/`updatedAt` asserted present and RFC3339-parseable; `deletedAt` asserted
**absent** from every body (it is not projected).

### D — Validation 422 (one per rule shape, asserting the KEY)

| case | request | expected key |
|---|---|---|
| `D1` | `name: "aaaa"` | `InvalidDisplayNameNotification` |
| `D2` | `workspace: "Acme Corp"` | `InvalidTenantWorkspaceNotification` |
| `D3` | `workspace: "admin"` | `ReservedTenantWorkspaceNotification` |
| `D4` | `description: "short"` | `InvalidDescriptionNotification` |
| `D5` | `status: "frozen"` | `UnknownTenantStatusNotification` |
| `D6` | `description` == `name` | `TenantDescriptionMustDifferNotification` |
| `D7` | patch `status: "trial"` on an `active` tenant | `InvalidTenantStatusTransitionNotification` |

All **422** (`SemanticValidation`).

`D8` — **structural immutability of `workspace`**: `PatchTenantRequest` has no
`Workspace` member, so the handle cannot be sent at all. The wire promise is therefore
"a `workspace` key in a PATCH body changes nothing": → **200**, stored `workspace`
unchanged. `TenantWorkspaceIsImmutableNotification` is the belt-and-braces layer behind
a door REST cannot open, so no case asserts it — stated rather than silently skipped.

### E — 409, both flavors

- `E1` duplicate: insert an existing `workspace` → **409**,
  `TenantWorkspaceAlreadyExistsNotification`, `semantic: "Conflict"`.
- `E2` `scope: all` proof: archive a tenant, then insert its workspace again → still
  **409**. The handle is never released.
- `E3` wrong-state (`SemanticStateConflict`) — **N/A**: the entity declares no
  state-conflict notification, and the framework's two
  (`EntityIsNotActiveNotification`, `ConcurrentModificationNotification`) are not
  reachable through this REST surface — no verb here carries a revision precondition,
  and the archive/unarchive misuse cases resolve as 404 (see `F6`/`F7`), which is what
  `LoadForWrite` / `LoadArchivedForWrite` actually produce.

### F — Archive round-trip (regime: kept-but-hidden, `DeleteOnArchive` not declared)

`F1` archive → by-id → **404** `RecordNotFoundNotification` ·
`F2` by-id `?includeArchived=true` → **200** ·
`F3` listing hides it; `?includeArchived=true` reveals it ·
`F4` the manual rule `archive-forces-suspended` reaches the ROW: read back archived →
`status == "suspended"` even though it was `active` ·
`F5` unarchive → visible again, `status` STAYS `suspended` (the rule's stated
consequence) ·
`F6` archive an already-archived tenant → **404** (`LoadForWrite` filters archived) ·
`F7` unarchive an active tenant → **404** (`LoadArchivedForWrite` is `OnlyArchived`).

Child stamp-scoped unarchive — **N/A**: flat aggregate, no child table.

### G — Read vocabulary (REST listing)

One case per declared operator family, plus the pagination envelope as a contract:

`G1` eq (`?workspace=…`) · `G2` `?status.in=active,trial` · `G3` `?workspace.startswith=`
and `?workspace.istartswith=` · `G4` `?name.contains=` / `?name.icontains=` /
`?description.icontains=` · `G5` `?createdAt.gte=` + `?createdAt.lte=` ·
`G6` `?updatedAt.gte=` (filterable though not orderable) ·
`G7` `?orderBy=name`, `?orderBy=-name`, `?orderBy=workspace`, `?orderBy=createdAt` ·
`G8` `?fields=name,workspace` → those keys present, `description` absent ·
`G9` `?onlyTotal=true` → `pagination.totalCount` only, **no** `data`, no
`hasNextPage`/cursors · `G10` `?last=2` alone → the TAIL window, `hasNextPage == false` ·
`G11` **pagination envelope truthfulness**: with 5 seeded records and `?first=2` —
`totalCount == 5`, `hasNextPage == true`, `hasPreviousPage == false`, `endCursor`
present, `startCursor` present; echo `endCursor` into `?after=` → page 2 is DISJOINT
from page 1, `hasPreviousPage == true`; walk back with `?before=<startCursor of page 2>`
→ page 1 again · `G12` `?includeArchived=true` raises `totalCount` by exactly the number
of archived records.

`?search=` — **N/A as a capability case**: the DTO does not declare it, so it is
refused by the opt-in gate (`H3`) BEFORE any engine sees it. See the note under §H.

### H — Rejected reads: the whole typed-400 guard family

All **400**. Key is `SchemaViolationNotification` unless stated.

| case | request | field named |
|---|---|---|
| `H1` | `?bogus=1` | `bogus` |
| `H2` | `?workspace.contains=x` (operator outside its allowlist) | `workspace` |
| `H3` | `?search=acme` (reserved control the DTO never declared) | `search` |
| `H4` | `?fields=bogus` | `fields[bogus]` |
| `H5` | `?orderBy=status` (filterable, never declared orderable) | `orderBy[status]` |
| `H6` | `?orderBy=updatedAt` (same) | `orderBy[updatedAt]` |
| `H7` | `?orderBy=bogus` | `orderBy[bogus]` |
| `H8` | `?orderBy=-workspace` | **200** — positive control, `desc` IS declared |
| `H9` | `?first=101` | `LimitExceededNotification` on `first`, effective max `100` |
| `H10` | `?first=0` | `first` |
| `H11` | `?first=2&last=2` · `?first=2&before=X` · `?last=2&after=X` · `?after=X&before=Y` | the backward-side key — `last` when `last` is present, `before` otherwise (`web/queryschema/gate.go`, `ValidateControls` step 2). CORRECTED after the first run: `last+after` names `last`, not `after`. |
| `H12` | `?onlyTotal=true` with `&first=10` / `&orderBy=name` / `&fields=name` / `&after=X` / `&before=X` | `onlyTotal[<conflict>]` |
| `H13` | `?onlyTotal=true&workspace.startswith=…` · `&includeArchived=true` | **200** — filters and archive are NOT conflicts, counting a subset is the point |
| `H14` | `?after=not-a-cursor` | `after` |
| `H15` | cursor issued with no `orderBy`, replayed with `&orderBy=name` | `after` — the STRUCTURAL check, run by the REST wrapper before dispatch |
| `H16` | same cursor replayed with `&includeArchived=true` | `cursor` — the CONTEXT-HASH check, run inside the reader (`core.InvalidCursorError`). CORRECTED after the first run, and a framework finding: see §8. |
| `H17` | `?includeArchived=1` · `?onlyTotal=` (empty) | the control's own key — booleans take exactly `true`/`false` |
| `H18` | **by-id DTO gate**: `GET /tenants/:id?onlyTotal=false` · `?fields=name` | `onlyTotal` / `fields` — presence gates, declared-ness is the whole test |
| `H19` | `GET /tenants/:id?includeArchived=true` | **200** — positive control for the one control it does declare |

`UnsupportedCapabilityNotification` — **N/A for this entity**: it is raised when a read
engine is asked for something the store cannot serve — `?search=`, or a filter/sort on a
field a single-root read cannot reach. Tenant is FLAT (no 1:N leg to push down) and
`search` is not declared, so the DTO gate answers first with a
`SchemaViolationNotification`. Asserting the other key here would be asserting a
promise this entity does not make.

### I — Routing and not-found

`I1` `GET /tenants/<unused uuid>` → **404** `RecordNotFoundNotification` ·
`I2` `DELETE /tenants/:id` → **405** `MethodNotAllowedNotification`, field
`DELETE /tenants/<id>` — proves no hard delete exists ·
`I3` `POST /tenants/:id` → **405** (path registered, method not) ·
`I4a` `GET /tenants/:id/archive` → **405**: the path IS registered, under PATCH, so Fiber
matched it and refused the method. CORRECTED after the first run — this was written as a
404 case on a wrong example ·
`I4b` `GET /tenants/:id/purge` (matches no registered route) → **404**
`RouteNotFoundNotification`.

Mode-missing-with-route-mounted **403** (`…NotAllowedNotification`) — **N/A**: every
mode `Tenant` declares has its route mounted and no route is mounted for an undeclared
mode, so the 403 arm of the three-way split is unreachable here. The 403 that IS
reachable is the permission gate, covered by `A2`.

`I5` `GET /tenants/not-a-uuid` → **404**. DECIDED (Q2): the pin's docs promise nothing
here and `domain.NewID` does not validate — the string is bound against a `uuid` column.
404 is the only defensible contract (a syntactically impossible id names no record); a
500 is a genuine FINDING about the service, reported verbatim and routed to
`/omnicore:doctor`, never patched away in the case.

### J — GraphQL (handler invariance)

`J1` `tenants(first: 2)` → `edges { node cursor } pageInfo { … } totalCount`, and the
node equals the REST listing row for the same record ·
`J2` `tenant(id:)` → equals the REST by-id document, field for field ·
`J3` `createTenant(input:)` → the record is visible over **REST** immediately (one
write, two surfaces) · `J4` `patchTenant(id, input:)` → same ·
`J5` `archiveTenant` / `unarchiveTenant` → payload `{ success, id }`, effect confirmed
over REST · `J6` `tenant(id: <unused>)` → **HTTP 200** with `errors[0].extensions
.notificationKey == "RecordNotFoundNotification"` (the GraphQL idiom — the REST
envelope is NOT asserted cross-surface) · `J7` duplicate workspace via `createTenant`
→ HTTP 200, `extensions.notificationKey == "TenantWorkspaceAlreadyExistsNotification"`,
`extensions.semantic == "Conflict"` · `J8` GraphQL with no bearer → **401** (the route
is not public) · `J9` the DTO opt-in gate in GraphQL idiom: `tenants(search: "x")` is
an **unknown argument** — the schema never advertises it — so the answer is a
gqlparser validation error, not the REST 400 envelope · `J10` `where: { workspace:
{ startswith: … } }` and `orderBy: [{field: NAME, direction: DESC}]` return the same
set and order as their REST twins.

gRPC — **N/A**: no transport is wired. Exports — **N/A**: none declared.

### X — Suite meta

`X1` `GET /openapi.json` enumerates exactly the six tenant routes above (the cheapest
oracle for "which verbs does this entity really serve"); a mismatch is a FINDING. The two
collection verbs are advertised as `/tenants/` with a trailing slash — they are mounted on
the Fiber group under path `/`. Fiber serves both spellings; the case asserts the
document's own.

---

## 2. Data hygiene — DECIDED: Option A

The suite needs a **clean, exactly-countable baseline** (`G9`/`G11`/`G12` assert exact
totals) and it must sign in, which on a fresh database means rotating the bootstrap
password — a one-way change. Two honest options, decided at the gate:

**Option A — CHOSEN. A dedicated throwaway database, suite-owned config.**
`qa/microservice.qa.yaml` (generated by this skill, under `qa/`, never touching the
project's own yaml) is a copy of the dev profile with the DSN pointed at
`authcore_qa_db`, selected through `OMNICORE_CONFIG_PATH`. Reset per run:
`docker compose exec -T postgres dropdb --if-exists authcore_qa_db && createdb`, then
the service's own `migrations.autoRun` rebuilds the schema AND the bootstrap seed. No
Mongo, no CDC, so reset is that one step and nothing races it.
Residue: one throwaway database, dropped and recreated on the next run.
Cost: the suite must be able to `docker compose exec` the bench container.

**Option B — rejected.** The dev bench database, run-scoped records. Every record the suite
writes carries a run-unique workspace prefix (`qa-<epoch>-…`), every listing assertion
is scoped by `?workspace.startswith=qa-<epoch>-` so counts stay exact, and the suite
archives its own records at the end.
Residue: archived `qa-*` tenants accumulate in `authcore_db`, permanently holding their
handles (`scope: all` — that is the model, not a leak).
Blocker to be honest about: the bootstrap admin's password in that database is whatever
the dev already set it to. The suite would need it handed in via an env var
(`QA_ADMIN_PASSWORD`), and `A1`/`A3` (the must-change-password gate) become
unassertable.

## 3. Auth — not open, but stated

`auth.mode: jwt` with `authorization.enabled: true`; the dev bench is deliberately
CLOSED. **No token is invented**: the suite signs in against this service's own
`POST /auth/user/token` with the credentials the tracked bootstrap seed migration
(`0012_bootstrap_seed_manual.up.sql`) puts in the database — `admin@authcore.local` /
`admin` — and rotates that password through `PATCH /users/:id/password` to a
suite-owned value, which is required anyway because the pre-rotation token carries only
`user:change-password`.
401 and 403 are therefore first-class cases (`A2`, `A4`, `A5`), not an untested layer.
The tenant claim (`tenant.required: true`) is satisfied by that token's own
`tenant_id` — the master tenant.

## 4. Out of scope, named plainly

- **Load / performance / concurrency** — not covered. No throughput, no revision-race
  case (the wire carries no revision precondition on this entity).
- **UI** — `/docs` and `/graphql/ui` are reachability-only, not asserted.
- **Integration events — N/A, not skipped**: the service declares no `transport:`
  block, publishes nothing, and has no `integration_events` table. There is no in-TX
  outbox row to assert and no relay to prove delivery through. If a broker is ever
  wired, this section becomes a real family.
- **Audit events** — the framework writes an `audit_events` row per write. Asserting it
  is possible (SQL) and is deliberately **out of scope for the tenant lane**: it is a
  cross-cutting concern that belongs in its own suite rather than duplicated per entity.
  ⚠️ Say so if you want it in this lane instead.
- **The other 8 entities** — this run is scoped to `tenant` (the argument given). The
  runner is built to grow: adding `qa/user.sh` means adding `user` to its lane list.

## 5. Runner contract

**Exactly one entry point: `qa/run.sh`.** It is the only thing a dev or a CI job
invokes; `./qa/run.sh` runs everything, `./qa/run.sh tenant` runs a subset. There is no
second runner and never one per entity.

- First act of `run.sh` and of every lane: `cd "$(dirname "$0")/.."` — the project root
  is resolved from the script's own location, so `devops/docker-compose.yml`,
  `migrations/` and the build resolve from any working directory.
- The lane list is an explicit array inside `run.sh` (`SUITES=(tenant)`), deterministic
  order, visible in one place. `ls qa/*.sh` minus `run.sh` must equal it exactly.
- **Fail-fast by default**; `--all` runs the exhaustive sweep. Each lane exits
  non-zero when any of its cases failed, which is what makes fail-fast able to trip.
- Per-run namespacing of every shared artifact: temp files, the compiled binary
  (`bin/authcore-qa-$$`), the server log, and the HTTP port. Two lanes never share one.
- Boot: probe the effective port FIRST and free it with SIGTERM if something is
  listening (never assume the listener is yours), build
  `go build -tags 'postgres'` (dialect `postgres`, no `transport:` block → no transport
  tag), boot in background with `APP_PROFILE=dev` (+ `OMNICORE_CONFIG_PATH` under
  Option A) and the `JWT_SIGNING_KEY`/`JWT_SIGNING_KID` pair `start.sh` already
  generates, then poll `/readyz` READING the 503 reason.
- Shutdown: **SIGTERM only, never `kill -9`**, and `wait` on the server PID until the
  drain completes (budget 30s, matching `shutdown.drainTimeoutSeconds`) before the next
  lane binds the port. Cleanup runs on `EXIT`.
- Every request pins `Accept-Language: en`. Every case prints name, expectation and
  verdict; a failed assertion dumps the REAL response body. `SKIPPED` is printed as
  SKIPPED and never folded into GREEN.
- `jq` is required (assertions read JSON); its absence is a loud precondition failure,
  not a silent skip.

## 6. Gate decisions (2026-09-01)

- **Q1 — data hygiene → Option A.** Dedicated `authcore_qa_db`, selected by
  `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml`. Reset per run: `dropdb --if-exists` +
  `createdb` inside the bench container, then `migrations.autoRun` rebuilds schema and
  bootstrap seed. Residue: that one throwaway database, recreated next run. Nothing is
  ever written to `authcore_db`.
- **Q2 — `GET /tenants/not-a-uuid` → include, asserting 404.** See `I5`.
- **Q3 — GraphQL depth → the full §J family** (`J1`–`J10`).

## 7. Artifacts this plan produces

| path | what |
|---|---|
| `qa/run.sh` | the ONE runner; lane list `SUITES=(tenant)`; `--all` disables fail-fast |
| `qa/tenant.sh` | the tenant lane, self-contained: provisions the DB, boots, asserts, drains |
| `qa/microservice.qa.yaml` | suite-owned config — the dev profile with the DSN on `authcore_qa_db` and the port on `${QA_HTTP_ADDR::8099}`. The project's own yaml is never touched. |

Neither `qa/` nor `specs/qa/` is added to `.gitignore` — both are part of the project.
A new lane means a new `qa/<entity>.sh` AND its name in `SUITES` in the same change;
`ls qa/*.sh` minus `run.sh` must equal that list exactly.


---

## 8. Run record — 2026-09-01

`./qa/run.sh` · **GREEN 184 · RED 1 · SKIPPED 0** · every family in §1 exists in
`qa/tenant.sh` and RAN; `ls qa/*.sh` minus `run.sh` equals `SUITES=(tenant)`.

**The suite can fail** (mandatory meta-case): `H19`'s expectation was flipped from 200 to
409 by hand, the run went RED on it with the real 200 body printed, and the expectation
was restored. A suite that cannot fail proves nothing.

Three expectations were CORRECTED between the first and final run — all three were
mis-derivations of mine, verified against the framework's own source before changing, and
none of them weakened a case: `H11c` (backward-side key), `H16` (which of the two cursor
checks fires), `I4` (a path that turned out to be registered). One was a broken `jq`
filter (`X1b`). None was a service regression.

### FINDING — `GET /tenants/{non-uuid}` answers 500, not 404 · `I5` stays RED

```
GET /tenants/not-a-uuid  →  500 InternalServerErrorNotification
server log: ERROR: invalid input syntax for type uuid: "not-a-uuid" (SQLSTATE 22P02)
            route=/tenants/:id
```

The path segment is never validated: `domain.NewID(s)` (omnicore
`domain/id.go:35`) wraps any string, and the relational reader binds it straight into
`WHERE id = $1` against a `uuid` column, so Postgres raises `22P02` and the error escapes
as the generic 500. It is FRAMEWORK-level, not authcore code — every by-id route of every
entity in this service (and in any other omnicore service on a uuid PK) behaves the same
way. An id that cannot possibly name a row is a request the reader should answer 404 to,
or 400 at the wire boundary; leaking a driver error as a 500 also tells a caller that
something broke when nothing did.

The case is left RED on purpose. Per the plan's gate decision (Q2) it is not weakened to
pass — the contract is decided upstream, and this case turns green on its own once it is.

Ownership was checked as three separate questions, not assumed: `omnicore-gen doctor`
reports no drift (nothing hand-edited), `omnicore-gen explain` has no key that concerns id
validation (nothing the generator withheld), and the binding happens inside the
framework's own by-id wrappers. It is omnicore's.

Full write-up, forwardable to the framework team:
[`finding-unvalidated-path-id.md`](finding-unvalidated-path-id.md).
