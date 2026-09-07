# Capability plan — tenant-row-scope

- **Status:** APPLIED (approved 2026-09-07; executed on omnicore-gen 0.66.0)
- **Framework pin:** `github.com/ClaudioSchirmer/omnicore v0.74.0`. Generator: `omnicore-gen` from plugin **0.66.0**.

## §1 The request (restated)

> o cadastro de permissoes e tenant não tem filtros hoje, permissões ta tudo bem, mas um tenant com tenant:read, não pode ver os outros tenants. Faltou implementar o filtro no byId, e byParams o filtro de tenantId se não for superAdmin. Olhar a matrix de acesso.md. OBS: Foi feita uma alteração no gerador, plugin, preciso que vc leia o changelog do plugin e faça os ajutes e regere tudo, já com esse filtro novo.

A caller holding `tenant:read` and a `tenant_id` claim now reads exactly one row — its own — on `GET /tenants`, `GET /tenants/:id` and the `tenants` / `tenant` GraphQL fields. A `*:*` super-admin still crosses. Another tenant's id answers **404**, not 403. Alongside it, the whole spec set moved off the two `authz` keys plugin 0.65.0 retired.

**Maintainer's scope decision at the gate (2026-09-07):** `applies: [read]`. `PATCH`, `archive` and `unarchive` stay unscoped, contained by the distribution of `tenant:update` / `tenant:archive` and by `no-escalation`.

**Execution note.** This plan was first executed on plugin 0.65.0 and fully reverted after two generator defects surfaced (a documented-equivalent spec migration that renamed exported identifiers and broke hand-written code; `applies: [read]` emitting the whole write-side plumbing). Both were fixed in **0.66.0** and this run is the re-execution on that build. Neither defect reproduced.

## §2 Routing evidence — the owning docs

| Capability piece | Owning section(s) at this pin | Existence check |
|---|---|---|
| The read filter narrowing rows from the identity | `authz-seams.html` → *Layer 3 — tenant scoping*, reads half | offered at pin |
| The super-admin bypass inside that filter | `authz-seams.html` → *Asking the super-admin question* — `IsSuperAdmin()`, sanctioned for "cross-tenant bypass inside `Query.ToCriteria(ctx)`" | offered at pin |
| `"ID"` as the framework's logical name for the aggregate id | framework source `application/queries/result_fill.go` (+ `result_fill_test.go:253`) | offered at pin |
| `authz.dataAccess: scoped` + `authz.scopes[]` + `field: ID` + `applies` | plugin `CHANGELOG.md` [0.65.0]; naming and read-only gate corrected in [0.66.0] | offered at generator |

Routing outcome: **offered at pin**. No upgrade, no `/omnicore:configure`, no infra gate.

## §3 Integration semantics

- **Seam:** the generated `ToCriteria(ctx)` of the two Tenant read queries. Declaring it in the spec is correct — those files are `class: owned`, so a hand edit would be overwritten.
- **Sync/async:** synchronous, inside the query flow. One entry added to criteria the query already builds; no extra round trip.
- **Failure policy:** N/A — no external dependency. Absent identity is a policy: **`noIdentity: stand-down`**, the generator default and what the five scoped specs already declare. Reachable only under `auth.mode: disabled`, refused outside `APP_PROFILE=dev`.
- **A token with no `tenant_id` claim never reaches here** — `auth.authorization.tenant.required: true` rejects it at the middleware with 403. Verified present in all four boot profiles.
- **Idempotency / cache slots:** N/A.
- **Wire/API impact — a behavior change on a live contract, and it is the point.** No route, field, status code or payload shape changes; WHICH ROWS an existing endpoint answers with does:

  | Caller | `GET /tenants` before | after | `GET /tenants/:id` (another tenant) before | after |
  |---|---|---|---|---|
  | `*:*` super-admin | every row | every row (unchanged) | 200 | 200 (unchanged) |
  | `tenant:read` + `tenant_id` | every row | **its own row only** | 200 | **404** |

## §4 External contract

N/A — no external system.

## §5 Impact map — what was actually touched

| Artifact | Change |
|---|---|
| `microservice.{dev,prd}.yaml`, `qa/microservice.qa{,-key}.yaml` | **no change**; all four verified to carry `authorization.enabled: true` and `authorization.tenant.required: true` |
| `specs/omnicore-gen/tenant.omnicore.yaml` | `anyone-with-permission` → `scoped` + `scopes: [{field: ID, from: tenant, applies: [read]}]` + `bypass: "*:*"` + `noIdentity: stand-down`; comment block rewritten (it argued the opposite) |
| `specs/omnicore-gen/{claim,client,group,role,user}.omnicore.yaml` | `dataAccess: tenant` + `tenantField: TenantID` → `dataAccess: scoped` + `scopes: [{field: TenantID, from: tenant}]` |
| `specs/omnicore-gen/permission.omnicore.yaml` | **no change** — `anyone-with-permission` is not retired; the catalog is global by design |
| `find_tenant_by_id_query.go`, `find_tenants_by_params_query.go` | **the deliverable** — `Filter["ID"] = id.TenantID()` under an `IsSuperAdmin()` bypass |
| `find_tenants_by_params_query_test.go` | the generated scope case |
| 5 migrated entities' generated files | **no production-code change** — comment rewordings, one field reorder, and generated-test message/fixture updates |
| 15 `*_manual.go` (`class: hook`) | untouched |
| `specs/omnicore-gen/lock.json` + 7 `*.gen-report.md` | rewritten |
| notifications + 7 translation catalogs | `N/A` — a filtered row is an ordinary 404, not a new 403; `applies: [read]` emits no write guard |
| `migrations/` | `N/A` — no schema change |
| `qa/lib/common.sh` | `jwt_claim` helper — reads the caller's own tenant off the token it already holds, so the assertion is not certified by the endpoint under test |
| `qa/security.sh` | S3c.6 now reads the caller's OWN tenant; S3c.11 / S3c.12 promoted from `skip_` (which asserted the opposite) to executed cases; +3 complements: `*:*` bypass on by-id and on the listing, and the same scope on GraphQL |
| `ACCESS_MATRIX.md` | Tenant read rows updated; the "Still open on 2026-09-07" paragraph superseded and dated; the legend gained the `filter ID` spelling; the leak paragraph moved to past tense |

## §6 Config & secrets

No key added, removed or renamed. No secret, no env placeholder. All four boot profiles read and confirmed.

## §7 Verify — executed

1. `omnicore-gen check` — all seven accepted (Permission's `unique.echoValue` warning is pre-existing and unrelated).
2. `go build ./...` — clean, **with no hand fix required**; `RequestingTenant` / `refuseForeignTenant` kept their names on 0.66.0.
3. `go vet ./...` — clean.
4. `go test ./...` — 14 packages ok, 0 failures.
5. Diff inspected: production-code changes confined to the two Tenant `ToCriteria`. Tenant emitted **no** runtime carrier fields and **no** write-mapper feeds — `applies: [read]` honored.
6. `omnicore-gen doctor` — no drift on any of the seven.
7. **Not yet run:** `./qa/run.sh` needs the docker bench. It is the only proof that the filter answers correctly end to end, on both surfaces.
8. **Deliberately not proven:** `PATCH` / `archive` / `unarchive` still reach any row for a holder of `tenant:update` / `tenant:archive` — the maintainer's decision, recorded in `ACCESS_MATRIX.md` beside those rows.
