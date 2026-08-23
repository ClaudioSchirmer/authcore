# Task 4 — infra

## Docs to READ (mandatory, at the pin)

- `table-schema` — the Go↔column mapping, the **supported column shapes** (the id contract at
  this pin), the child declaration, and the archive-column declaration.
- `views` — the view definition surface and its version.
- `auto-query-handlers` — view indexes and options, and which changes require a version bump.
- `relational-view` — what a source-of-record-backed view serves and what it refuses.
- `service-to-service` — how the domain service implementation reaches another aggregate's
  repository.
- `/Users/claudio/.claude/plugins/cache/omnicore/omnicore/0.30.0/shared/dialects/postgres.md` — the id column type, the
  constraint-violation key the constraint map binds, and active-only uniqueness. **The only
  dialect sheet this project needs.**

Convention: `conventions/infra.md`. Read `task_children.md` too.

## What this layer contains

**Schema** — one schema per file (a bundled schema file is a layout violation `go build`
will not flag). The root's columns as spec §2, the collection declared as the root's child
keyed by the parent column, and the archive column declared — **`Modes()` listing archive
⟺ this declaration ⟺ the migration column**. The two runtime-only identity fields are
**not** declared here.

The handle's physical column is **prefixed with the entity name**: the bare word is reserved
across the engine set. The exposed name is untouched, so every filter, ordering token,
OpenAPI parameter and GraphQL argument still says `key` — exactly how `Role` stores its own
handle.

**Repository** — with the constraint map binding each unique index to its 409:

- the per-tenant handle index → the handle-already-exists notification;
- the collection's per-owner role index → the already-grants-role notification.

**Domain service implementation** — the six facts of spec §7, request-scoped so it reads the
caller identity without the domain ever seeing a context (the project already binds this for
`Permission` and `Role`; mirror that). It holds the tenants and roles repositories beside its
own. The two escalation facts share **one** resolution (role → its active permission keys),
so a write attaching N roles pays N lookups, not 2N. The caller-holds fact **guards the
wildcard itself** and answers "refuse" rather than calling through — defence in depth behind
G10b, because a panic on a security rule is a 500.

**View** — relational-backed (the project posture, and the only option without Mongo). It
serves the root's fields and the collection; a filter or sort **inside** the collection is a
typed 400 and that is the accepted cost of spec §9.

## Acceptance check

- One schema per file; the archive column declared and agreeing with the modes.
- The handle column is entity-prefixed; the exposed name is unchanged.
- The constraint map binds by **column list**, matching what the postgres sheet says the
  driver reports — not a guessed index name.
- The service implementation exposes exactly the six facts, request-scoped, sharing one
  resolution across the two escalation facts.
- The view is relational-backed with the reserved controls of spec §9.
- `gofmt -l` prints nothing; `go vet` and `go build` clean.
