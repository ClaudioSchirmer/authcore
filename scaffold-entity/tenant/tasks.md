# tasks — Tenant

Control file for the generation of the `Tenant` aggregate.

**Model authority: `spec.md` (Status: APPROVED).** Nothing here restates the model — when a
task and the spec disagree, the spec wins. When a task's mechanical detail contradicts a
`/docs` section or a layer convention, **the doc/convention wins** and the deviation is
recorded at the bottom of this file (a plan detail is a guess made before the layer's rules
were read).

Framework pin: **omnicore `v0.54.0`** — upgraded mid-run from `v0.53.0` (see `spec.md` §C
and `upgrade/rollback/`). Every doc read below is against `v0.54.0`; the `v0.53.0` docs are
stale for this entity, notably on archive semantics.

Dialect: `postgres` (single). Build tag: `postgres` (no transport block, so no transport tag).
Read-side posture: relational-served.

## Order

Inside → out. Each layer is executed from its own `task_<layer>.md`, which names the exact
`/docs` sections to read BEFORE generating anything in it.

| # | Layer | Task file | Status |
|---|---|---|---|
| 1 | domain | `task_domain.md` | pending |
| 2 | application | `task_application.md` | pending |
| 3 | web | `task_web.md` | pending |
| 4 | infra | `task_infra.md` | pending |
| 5 | migrations | `task_migrations.md` | pending |
| 6 | bootstrap | `task_bootstrap.md` | pending |
| 7 | tests | `task_tests.md` | pending |
| 8 | docs refresh | `task_docs.md` | pending |

Task 8 is not part of the entity; it is the `README.md` reconciliation the maintainer asked
for, deliberately scheduled last so the README describes what exists rather than what was
intended.

## Final verify (after task 7, before task 8)

Four distinct levels, none merged into another:

1. **Mechanical boot-trap checklist** — the pre-boot greps. Every item is a boot panic a
   grep catches for free, so none of it waits for a boot.
2. **`gofmt -l` · `go vet -tags postgres` · `go build -tags postgres`** — format, vet,
   compile. Necessary, not sufficient. Use an output path that is not the repo root: a bare
   `go build ./...` collides with the `bootstrap/` directory and reports an error that is
   about file naming, not about the code.
3. **Unit tests ≥ 80% per generated file**, measured with `-coverpkg=./internal/...` and
   read per file from the cover profile. A bare package percentage does not satisfy this.
4. **Existing QA suite** — regression only. There is none yet, so this level is a no-op and
   is reported as such rather than silently skipped.

Functional e2e of the new endpoints is `/omnicore:qa`'s job, offered after the verify is
green — it is not part of this plan and must not be reported as covered by it.

## Deviations (plan vs what the docs/conventions actually required)

_None recorded yet — appended per layer as they are found._
