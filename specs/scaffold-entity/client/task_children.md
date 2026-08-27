# task_children.md — the two collections (delta)

Model authority: [`spec.md`](spec.md) §3, §7 (C8–C12, C15–C18), §9.
Convention: `conventions/aggregate-children.md`. Read this BEFORE domain, application, web,
infra and migrations — it crosses all five.

## What this delta must contain

**Two aggregate value objects on the flat root**, and they are not symmetric:

- **The role grants.** Each entry holds the referenced role id and nothing else, plus the
  key and the display name read across the foreign key and filled on every load. Business
  identity is the referenced id. This collection carries the escalation rules.
- **The allowed network ranges.** Each entry holds a normalised range and a required label.
  Business identity is the range. This collection references no other aggregate, so it has
  no read join.

**Edit strategy A for both**, which the spec fixes and this layer does not re-decide: a
complete insert of the whole aggregate, a root-only partial update, and per-entry add and
archive by entry id. **No replace-all update on either collection** — an omitted entry must
never silently revoke a grant, and on the allow-list it must never silently widen access.

**Verb truth.** Both removals are soft, so both are an archive intent and neither is a purge.

**Caps** are per the spec's numbers, counted over the whole collection.

## The trap this delta exists to name

**The root-archive auto handler is instantiated at most once per surface** — its own archive
route on REST, its own mutation on GraphQL. A child operation that mounts it compiles, and
answers 200 while archiving the entire client. With two collections there are four child
operations and therefore four places to make that mistake instead of two. Each child
operation mounts its OWN command and follows that operation's field contract.

**A child-mutation method that emits a notification before delegating must initialise the
root first**, or the notification is silently dropped. A method that only delegates must not
— the framework's add/change/remove helpers already do it, and repeating it there is noise.

## What to read before writing — routed sections at the pin

`aggregate-persistence` · `auto-handlers` (the field contract per operation, and the
root-archive handler's name) · `read-joins` (the shared owner is
`shared/read-joins.md`) · `service-layout` for naming and granularity.

## Acceptance

- Both collections load with every read of the aggregate; the grant collection arrives with
  its joined key and name filled.
- Adding a duplicate entry to either collection is refused by business identity, and the
  partial unique index refuses the concurrent one.
- The root-archive handler appears once per surface and in no child route.
- Archiving one entry leaves the root and the other collection untouched.
