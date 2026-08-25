# Task 4 — infra

## Docs to READ (mandatory, at the pin)

- `table-schema` — the Go↔column mapping, the **supported column shapes** (the id contract at
  this pin), the child declaration, and the archive-column declaration.
- `views` — the view definition surface. Note which family the version field belongs to: a
  relational view has none.
- `auto-query-handlers` — view indexes and options.
- `relational-view` — what a source-of-record-backed view serves and what it refuses, and
  that it inherits the loader's joins rather than declaring any.
- `read-joins` — the whole of it. This layer declares the traversal, and it declares it on
  the repository, where every consumer of the loader inherits it.
- `service-to-service` — how the domain service implementation reaches another aggregate's
  repository.
- `shared/dialects/postgres.md` (the pinned plugin's copy) — the id column type, the
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

**…and the read join, declared with `WithJoins` beside `WithSchema`** — spec §2:

```
read.InnerJoinInChild(<child schema>).To(<role schema>)
  GroupRole.role_id  =  roles.id
    → RoleKey     ← roles.role_key
    → RoleName    ← roles.name
    → ArchivedAt  ← roles.deleted_at              -- managed column; *time.Time
```

`inner` is correct only because `role_id` is `NOT NULL` and FK-backed — inside a child an
inner join drops the ENTRY, and over a nullable key it would drop entries from `FindByID`
too. The three fields are read-only, carry no domain type (`RoleKey` is a plain `string`,
never `vos.RoleKey`), and are absent from the `TableSchema`, so none of them reaches an
INSERT or an UPDATE. `ArchivedAt` is `*time.Time` because the target's `deleted_at` is
nullable. **No traversal to `Tenant`** — spec §2 names the trap: `groups.tenant_id` points at
`tenants.id` since 2026-08-24 (the derived key was removed), so the traversal IS expressible now; what follows is the record of why it was refused while that key existed — it would have been accepted and would have matched
nothing.

**Domain service implementation** — the six facts of spec §7, request-scoped so it reads the
caller identity without the domain ever seeing a context (mirror how `Permission` and `Role`
bind it). It holds the **tenants and roles** repositories beside its own — and deliberately
**not** a `PermissionRepository`, which is what the read joins buy this layer:

- `RoleRepository` declares its own traversal into `permissions` (see `../role/task_infra.md`),
  so one read through `RoleRepository.Loader` returns each role **with every grant's
  `resource` and `action` already filled**. Role id → the role's grants, keys included → the
  claim. There is no id→key resolution step and no second query into the catalog.
- The two escalation facts share that **one** resolution, so a write attaching N roles pays N
  reads, not 2N and not 3N.
- **The keys they judge are every key the role grants, archived catalog rows included** —
  spec §7's fail-closed decision. The grant entries do carry an `ArchivedAt`, so the narrower
  reading is technically available in the same read; it is rejected on merit. Do not
  "optimize" it back in.
- The caller-holds fact **guards the wildcard itself** and answers "refuse" rather than
  calling through — defence in depth behind G10b, because a panic on a security rule is a
  500.

The same warning `../role/task_infra.md` carries applies here one level up: these facts judge
the entries a write is ATTACHING, and an attached entry has no joined value. `RoleKey` reads
`""` and `ArchivedAt` reads `nil`, indistinguishable from a live role. No fact reads the
join.

**View** — relational-backed (the project posture, and the only option without Mongo):
`query.RelationalView("groups", repo.Loader)`, contributed through the feature's
`RelationalViews()` opt-in, sharing the repository's own loader and never a second one built
here. It takes its schema from the loader, carries **no `Version`**, no registry row, no
rebuild and no collection, and declares **no join of its own** — `RoleKey`, `RoleName` and
`ArchivedAt` reach the served document because the repository declared the traversal. It
serves the root's fields and the collection; a filter or sort **inside** the collection is a
typed 400 and that is the accepted cost of spec §9.

## Acceptance check

- One schema per file; the archive column declared and agreeing with the modes.
- The handle column is entity-prefixed; the exposed name is unchanged.
- The constraint map binds by **column list**, matching what the postgres sheet says the
  driver reports — not a guessed index name.
- The read join is declared **on the repository** with `WithJoins`, maps the three fields of
  spec §2, is `inner` in the child, and reaches `roles` only — never `tenants`.
- No join field appears in the `TableSchema`, in a command, or in any request DTO.
- The service implementation exposes exactly the six facts, request-scoped, sharing one
  resolution across the two escalation facts, and holds **no** `PermissionRepository` — the
  keys arrive through `RoleRepository.Loader`.
- No rule and no service fact reads `ArchivedAt`.
- The view is relational-backed with the reserved controls of spec §9, carries no `Version`,
  and is contributed through `RelationalViews()`.
- `gofmt -l` prints nothing; `go vet` and `go build` clean.
