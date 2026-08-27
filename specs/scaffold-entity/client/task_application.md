# task_application.md — application

Model authority: [`spec.md`](spec.md) §2, §7, §8, §9. Read the two deltas first.

## What this layer must contain

**Commands for every write in §9** — the complete insert, the root-only partial update, the
archive, the rotate, and the four child operations. Each with its mapper: build the entity,
apply fully, apply partially, and read back.

**The identity inputs are filled by every write command's mapper, the bodyless ones
included.** An archive is a write to the row like any other and loads through the
repository, which the read side's filter never touches — so a mapper that skips them leaves
the row rules with nothing to judge.

**The two credential-bearing results carry one extra value** — the minted secret — and they
are the only results in the service that do. Everything else about them is ordinary.

**The rotate command carries the requested grace window as an optional value**, so that
omitted and zero stay distinguishable all the way from the request to the rule.

**Queries for the two reads of §9** — by id and by parameters — with the criteria builder
that injects the caller's tenant scope. The two computed read fields are filled once per
row in the query result mapping, so every surface agrees on them.

**An action-name map** covering the rotate, so the audit trail can tell it from the ordinary
partial update: both dispatch the same mode.

## What to read before writing — routed sections at the pin

`command-handler` · `auto-handlers` · `auto-query-handlers` (the result anatomy, and computed
read fields: their sources are pushed down, ordering by them is refused, filtering over them
is impossible) · `custom-command-handler` for the rotate · `lifecycle-map`.
Convention: `conventions/application.md`.

## Acceptance

- Every write command fills the identity inputs, archive included.
- The two computed read fields are filled in one place and read identically on both
  surfaces.
- Ordering by a computed field is a typed refusal and not a five-hundred.
- The rotate dispatches its own action name.
