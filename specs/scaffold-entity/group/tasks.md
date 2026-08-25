# tasks.md — Group

**Model authority: [`spec.md`](spec.md), `Status: APPROVED` (2026-08-21).** Nothing here
re-decides the model. Where this file and the spec disagree, the spec wins; where a task
file's mechanical detail contradicts a routed `/docs` section or a layer convention, the
**doc/convention wins** — apply it and record the deviation in the section at the bottom.

- **Pin:** omnicore `v0.59.0` · **Dialect:** postgres (only) · **Posture:** Postgres SoR,
  no Mongo, no broker → relational-served views · **Surfaces:** REST + OpenAPI + GraphQL
- **Branch:** `feature/group-aggregate` (the earlier `feature/group-entity` was fully
  merged into main and stale, so this run branched off main rather than re-stacking)
- **Generation:** `omnicore-gen` **0.38.0** (path chosen at gate 1d, 2026-08-21)

*(The three values above were carried over from the 2026-08-21 draft and were stale by the
time the entity was built; `spec.md`'s own header had already been re-pinned to v0.59.0 on
2026-08-24 and is what the build followed.)*

**BUILT 2026-08-24.** Every layer is done. `gofmt`, `go vet`, `go build` and the full test
suite are green on `-tags postgres`; the entity has not yet been booted against a live
engine — that is `/omnicore:qa`'s step.

## Layer order and status

| # | Layer | Task file | Status |
|---|---|---|---|
| 0 | children delta (read WITH the layers it touches: domain, application, web, infra, migrations) | [`task_children.md`](task_children.md) | **done** |
| 1 | domain | [`task_domain.md`](task_domain.md) | **done** |
| 2 | application | [`task_application.md`](task_application.md) | **done** |
| 3 | web | [`task_web.md`](task_web.md) | **done** |
| 4 | infra | [`task_infra.md`](task_infra.md) | **done** |
| 5 | migrations | [`task_migrations.md`](task_migrations.md) | **done** |
| 6 | bootstrap | [`task_bootstrap.md`](task_bootstrap.md) | **done** |
| 7 | tests | [`task_tests.md`](task_tests.md) | **done** (one accepted deviation, below) |
| 8 | docs | [`task_docs.md`](task_docs.md) | **pending** — not part of this run |

Layers 0–6 were emitted in one pass by `omnicore-gen` from
`specs/omnicore-gen/group.omnicore.yaml`, which is the mechanical restatement of `spec.md`.
What the generator does not write — the `GroupKey` value object, the four domain rules, the
four service facts, the two cross-aggregate foreign keys, and the tests for all of them —
was written by hand and is listed in `group.gen-report.md`.

Layer 0 is not a step of its own — it is the delta every child-bearing layer reads before
it runs. It is listed first because the model's single worst trap lives in it.

## The four things about this entity that are NOT `Role`

Carried here from the spec so no layer has to rediscover them:

1. **G6 asks a third question `Role` never had — same tenant.** `Role` is tenant-scoped
   (`Permission` was not), so an attached role from another tenant is a cross-tenant leak.
   All three questions answer with **one** notification, deliberately: a distinct
   "belongs to another tenant" message is an existence oracle over a competitor's org chart.
2. **`group:grant` is a fifth verb.** The two collection operations do NOT ride
   `group:update`. This is the one place the taxonomy diverges from `Role`'s, and it is the
   decision (spec §10), not an oversight.
3. **G10a is transitive.** The escalation check resolves a role to its permission keys and
   requires the caller to hold every one of them — not one key, a set.
4. **One-way archive hurts more here.** Unarchiving would re-authorize a whole team at once.

## Framework contract each layer must confirm before it writes

Never from memory — the routed section at the pin is the authority:

- ids are `domain.ID` on the domain and schema side, `string` on the wire, converted at the
  mappers (`table-schema`).
- a domain field carries `labelKey` and nothing else — no `json:`, no `db:`.
- `Modes()` listing Archive ⟺ the schema declares its archive column ⟺ the migration
  carries it. Three places, one fact.
- a `?fields=` opt-in forces every Response field AND every nested response field to
  `*T`/slice with `,omitempty`, and the query Result pointer/slice throughout.
- the ordering vocabulary lives on the Request per field, paired with the `orderBy` switch;
  either half alone fails the boot.
- `HasPermission` panics on any argument containing `*`. `IsSuperAdmin()` is the sanctioned
  `*:*` question.

## Deviations from `spec.md`, and why

*(filled during execution — one row per deviation, with the reason. An empty table at the
end of a build means the tree matches the model exactly.)*

| # | Spec says | Built as | Why |
|---|---|---|---|
| 1 | §2 leaves the `Tenant` traversal as "mirror it here **unless the gate decides otherwise**" | Mirrored in full — `TenantWorkspace` + `TenantStatus`, on the wire, filterable and sortable | Decided at the generation gate on 2026-08-24 and written back into `spec.md` §2 and §9. It is the one slot the approved spec deferred |
| 2 | §7's service port lists `CallerIsSuperAdmin()` | NOT declared | §7 itself says to check before declaring it, and §D resolves it. Verified against the built `Role`: the fact is implemented and tested there and **no rule calls it** — the generator answers the question itself from `authz.bypass` (`RequestingMayCrossScope = id.IsSuperAdmin()` in every command mapper, `IsSuperAdmin()` in both queries). Declaring it would put a second, hand-written answer beside the generated one |
| 3 | §10's table gates the reads on `group:read` | Same, but declared explicitly under `authz.permissions.read` | `Role` never declared it — the generator used to derive it. At gen 0.38.0 an ungated served read is a **blocker**, so the string is now written out. Same value, now explicit |
| 4 | CLAUDE.md rule 6 sets a 95% coverage floor | 99.0% of the unit-testable statements; 74.1% counting the engine-bound seam | The deviation `spec.md` §D already names and hands to the maintainer — see below |


## The coverage deviation, stated rather than padded

`spec.md` §D names this in advance: `internal/infra/` and the route-mount functions need a
live relational engine and a running app, so they are `/omnicore:qa`'s territory rather
than a unit test's. It is repo-wide, not specific to this entity.

Measured per file with `-coverpkg=./internal/...`, statement-weighted:

| Statements | Where | Covered |
|---|---|---|
| 205 | everything unit-testable | **203 — 99.0%** |
| 42 | `group_repository.go`, `group_service.go`, `group_routes.go` | 0 — they construct an engine, a store binding and a route tree |
| 27 | the query bodies inside `group_service_manual.go` | 0 — each one issues SQL |
| **274** | **the whole entity** | **203 — 74.1%** |

Every file that can be unit-tested is at 100% except `internal/domain/group.go`, at 95.8%.

**The 2 statements missing there are unreachable, not untested**, and the same 2 are
missing on `Role`: the generated `childDuplicate` loop body. `domain.AddAggregateChild`
already refuses a business-identical entry at add time — measured, the collection holds 1
entry after two identical adds and the refusal comes from the framework's own guard — so
no public path can put two matching entries in front of that loop. The duplicate rule is
therefore redundant with the framework, and the partial unique index remains the real race
backstop. Worth reporting upstream; harmless here.

The hand-written service facts are covered where they reach no store: the fail-closed
guards, the wildcard predicate, the transitive escalation walk, the two identity states and
the per-request memo scope are all tested (`internal/infra/group_service_manual_test.go`),
which is what takes that file to 53.4% against `Role`'s 38.5%.
