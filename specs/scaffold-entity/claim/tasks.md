# tasks.md — Claim

**Model authority: [`spec.md`](spec.md), `Status: APPROVED` (2026-08-28).** Nothing here
re-decides the model. Where this file and the spec disagree, the spec wins; where a task
file's mechanical detail contradicts a routed `/docs` section or a layer convention, the
**doc/convention wins** — apply it and record the deviation in the table at the bottom.

- **Pin:** omnicore `v0.62.0` (the latest published — the 0v check found no update) ·
  plugin `omnicore` 0.48.0 · **Dialect:** postgres (only) · **Posture:** Postgres SoR, no
  Mongo, no broker → relational-served views · **Surfaces:** REST + OpenAPI + GraphQL
- **Branch:** currently `docs/attribute-catalog-backlog-entry`. This run writes code, so it
  needs a coherent branch of its own before any edit lands — renamed in flight or cut from
  `main`, per `../../../CLAUDE.md` rule 5. `Claim` needs `tenants` to exist, and it does.
- **Generation:** **`omnicore-gen`** — chosen at gate 1d, 2026-08-28. The eight task files
  below are the REVIEW CHECKLIST for the emitted tree: they say what each layer must contain,
  which is exactly what the generated code is read against.

## Layer order and status

Execution is inside → out. Each task names the `/docs` sections that must be READ before its
layer is written; that read is mandatory at execution time, not optional, and is what keeps
the output correct against this pin rather than against memory.

| # | Layer | Task file | Status | Notes |
|---|---|---|---|---|
| 1 | domain | [`task_domain.md`](task_domain.md) | **done** | generated + `vos.ClaimName` and the 2 manual rules written by hand |
| 2 | application | [`task_application.md`](task_application.md) | **done** | generated; the identity translation is the generator's (`RequestingIdentityPresent` / `RequestingTenant` / `RequestingMayCrossScope`) |
| 3 | web | [`task_web.md`](task_web.md) | **done** | generated; 5 REST routes + 5 GraphQL operations, no child routes |
| 4 | infra | [`task_infra.md`](task_infra.md) | **done** | generated + the 1 manual fact written by hand |
| 5 | migrations | [`task_migrations.md`](task_migrations.md) | **done** | generated, then the tenant FK added by hand (D1) |
| 6 | bootstrap | [`task_bootstrap.md`](task_bootstrap.md) | **done** | generated (`bootstrap/claims_feature.go` + `wire.go`) |
| 7 | tests | [`task_tests.md`](task_tests.md) | **done** | generated suite + 2 hand-written files; see D2 for the measured deviation |
| 8 | docs | [`task_docs.md`](task_docs.md) | **done** | README (status row, `### Claim`, API shape, the pin), `ACCESS_MATRIX.md` (status line, `## Claim`, the prose), `backlog.md` (entry promoted) |

No children delta and no siblings delta: §3 and §4 of the spec are both `N/A`. This is the
first aggregate in the service with **no collection at all** — every trap the existing
entities carry about child routes and the root-archive handler is simply absent here, and a
layer that reaches for one has copied from `Role` instead of reading the spec.

## The five things this entity carries into more than one layer

Named once, and repeated only in the tasks that can actually trip them.

1. **The claim name is caller-owned, prefix included.** `x_` is typed by the caller, stored
   verbatim, and minted verbatim. **Nothing prepends it and nothing strips it**, at any
   layer — not the value object, not a mapper, not a handler. A layer that normalizes has
   reversed the decision the gate took (`spec.md` §2, OPEN-2 → option B).
2. **The double prefix is a refusal, not a repair.** A remainder that itself begins with
   `x_` is rejected by the value object.
3. **`DefaultValue` is validated against `ValueType`, so it is a cross-field rule** — a
   value object cannot see a sibling field. It is a named manual rule (R7), and a null
   default is always valid: absent and empty are not the same thing to a consumer.
4. **Row-level tenant isolation, on reads AND writes.** The read filter and the write guard
   are two different mechanisms and neither substitutes for the other; the guard is what
   stands between a caller and another tenant's definition, because writes load the row
   through the repository, which the read filter never touches.
5. **An empty tenant claim must fail closed.** The service-wide `auth.authorization` switch
   is absent from both profiles today, so the claim can be missing at runtime; a rule that
   reads absent as "unconstrained" would serve every tenant's rows. Absent IDENTITY
   (`auth.mode: disabled`, dev only) and absent CLAIM are two different states — collapsing
   them makes the entity either unusable in dev or unsafe in prd.

## What this entity deliberately does NOT have

Recorded so no layer adds it back from habit after reading `Role`:

- **no `:grant` verb** — there is no collection, so a fifth permission would gate nothing;
- **no escalation rules and no caller-identity service fact** — a claim definition confers
  nothing, so there is no privilege to escalate;
- **no `unarchive`, no `DELETE`** — soft archive only, root only;
- **no `?search=`** — a relational-served read model answers it with a typed 400;
- **no exports, no gRPC, no integration events**;
- **no seed of the platform's nine** — it has nowhere to land until the reserved platform
  tenant exists (`spec.md` §0), and this run does not write it.

## Cross-cutting acceptance (the final gate, `SKILL.md` "Final verify")

1. The mechanical boot-trap checklist, run to a clean pass **before** anything boots.
2. `gofmt -l` silent · `go vet -tags postgres ./...` clean · `go build -tags postgres ./...`.
3. Unit tests **≥ 95% per generated file** — `../../../CLAUDE.md` rule 6, which is stricter
   than the skill's 80% and wins. From the cover profile, measured with
   `-coverpkg=./internal/...`; the per-file `go tool cover -func` lines appear in the report.
4. The existing QA suite as a regression check.
5. Level 0 reconcile: walk `spec.md`'s promises with real command evidence.

## Deviations from `spec.md`, and why

*(filled during execution — one row per place the built tree differs from the approved
model, with the reason. An empty table at the end of a build means the tree matches the
model exactly; an unfilled one means nobody looked.)*

| # | Spec says | Built as | Why |
|---|---|---|---|
| D1 | §1's ER sketch shows the owner as `tenant_id UUID FK → tenants.id` | Present, **hand-written** in the migration | A reference to ANOTHER aggregate is outside the generator's spec language, so the `ALTER TABLE … ADD CONSTRAINT` block is appended by hand — the same append `roles` and `groups` both carry. Not a gap: the generator says so by name, and the report lists the join it feeds |
| D2 | `task_tests.md` floor: 95% per file | **Met at 100%** on every hand-written file (`vos/claim_name.go`, `claim_rules_manual.go`) and on `domain/claim.go`, `schemas`, `views`, every command, query and request mapper. **Under the floor, measured and accepted:** `internal/infra/claim_repository.go`, `internal/infra/claim_service.go`, `internal/infra/claim_service_manual.go` and `internal/web/claim_routes.go` — all 0% | They need a **live engine** or a mounted server; there is no unit-level seam. `task_tests.md` records this as accepted in advance, and it is the same deviation `Role`'s build recorded for the same four kinds of file. What is NOT waved through: the manual fact's one pure branch (an unusable owner id) is reasoned about in D3 rather than left silent |
| D3 | §7 R4: `ValueType` immutability is a rule of its own | Present and now **exercised through the only caller who can reach it** — a `*:*` operator | Found while closing coverage, and worth recording because the generated suite's own `TestClaim_TenantID_IsImmutable` passes for the WRONG reason: moving the row to another tenant makes it foreign, so `refuseForeignTenant` answers 403 and the barrier ends the pass before the update gate runs. The immutability rule is never reached on that path. It IS reached for a caller who crosses the scope, which is exactly what it is there for — `TestClaimTenantIsImmutableEvenForACallerWhoCrossesTheScope` pins it. **No production defect**: both paths refuse the write, and the generated test's assertion (the refusal names `TenantID`) is true either way |
| D4 | §1 called the physical column `claim_name`, on the premise that `name` is reserved across the engine set | Column `name` | The premise was wrong and the project disproves it — `roles.name` and `groups.name` are ordinary quoted columns. Corrected in `spec.md` §1 **before** generation, so the model and the tree agree |
