# tasks.md — Client

**Model authority: [`spec.md`](spec.md), `Status: APPROVED` (2026-08-26).** Nothing here
re-decides the model. Where this file and the spec disagree, the spec wins; where a task
file's mechanical detail contradicts a routed `/docs` section or a layer convention, the
**doc/convention wins** — apply it and record the deviation in the table at the bottom.

- **Pin:** omnicore `v0.61.0` · `omnicore-gen` from plugin 0.44.0 · **Dialect:** postgres
  (only) · **Posture:** Postgres SoR, no Mongo, no broker → relational-served views ·
  **Surfaces:** REST + OpenAPI + GraphQL
- **Branch:** `feature/client-credentials`, cut from `main` after the authentication-token
  merge landed. Not stacked — `Client` needs `tenants` and `roles` to exist, and they do.
- **Generation:** `<pending>` — gate 1d.

## Layer order and status

| # | Layer | Task file | Status |
|---|---|---|---|
| 0a | children delta — read WITH domain, application, web, infra, migrations | [`task_children.md`](task_children.md) | pending |
| 0b | **credential delta** — read WITH the same five. The part of this entity with no counterpart anywhere else | [`task_credential.md`](task_credential.md) | pending |
| 1 | domain | [`task_domain.md`](task_domain.md) | pending |
| 2 | application | [`task_application.md`](task_application.md) | pending |
| 3 | web | [`task_web.md`](task_web.md) | pending |
| 4 | infra | [`task_infra.md`](task_infra.md) | pending |
| 5 | migrations | [`task_migrations.md`](task_migrations.md) | pending |
| 6 | bootstrap | [`task_bootstrap.md`](task_bootstrap.md) | pending |
| 7 | tests | [`task_tests.md`](task_tests.md) | pending |
| 8 | docs | [`task_docs.md`](task_docs.md) | pending |

Layers 0a and 0b are not steps of their own — they are the deltas every layer they touch
reads before it runs. They are listed first because the model's worst traps live in them.

## The six things about this entity that are NOT `User`

Carried here from the spec so no layer has to rediscover them.

1. **The credential is hashed with SHA-256, not Argon2id, behind a NEW port.** This is the
   one place the entity deliberately refuses to inherit from `User`, and the reasoning is
   spec §B-Q4. A layer that reaches for the existing password adapter has taken a decision
   the gate already took the other way.
2. **The secret is minted by the server and revealed exactly ONCE**, in the response of the
   operation that minted it. Every other copy of the row — response, listing, audit event,
   outbox payload, query vocabulary — must not contain it. Five mechanisms, five separate
   places to get it wrong, and getting one wrong is a credential leak rather than a bug.
   `task_credential.md`.
3. **The row id IS the client id.** There is no second identifier. Nothing anywhere mints,
   stores or validates one.
4. **Two collections, and they are not alike.** One holds grants and carries the escalation
   rules; the other holds network ranges and carries a normalising value object. Four child
   operations, therefore four places to mis-wire the root-archive handler.
5. **Rotation is an overlap, not a swap.** Two nullable columns hold the retiring credential
   and its deadline, and a grace window of zero is a distinct, legitimate case that must
   clear both rather than stamp a past timestamp. `task_credential.md`.
6. **One rule is inert on the day it is written** — the client-writes-only-its-own-row rule
   reads a claim nothing mints yet. That is deliberate (spec §B-Q8e) and must not be
   "fixed" by removing it or by inventing a fallback that guesses the subject kind.

## Acceptance, service-wide

`../../../CLAUDE.md` rule 6 governs: **95 % coverage minimum**, not the skill's 80 %.
Spec §E adds three entity-specific checks on top of the standard verify gate.

## Deviations

| # | Layer | What the plan said | What was done, and which doc/convention required it |
|---|---|---|---|
| — | — | *(none yet)* | |
