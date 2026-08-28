# task_infra — Claim

Model authority: [`spec.md`](spec.md) §1, §2, §7, §9. Layout, naming and granularity:
`service-layout.html`.

## Read BEFORE writing this layer

| What | Where |
|---|---|
| the schema: Go type ↔ column, the supported column shapes, the id contract at this pin | `table-schema.html` |
| the read join into another aggregate — declared on the repository, inherited by every consumer of the loader | `shared/read-joins.md` (owner) · `read-joins.html` |
| what a relational read model serves and what it refuses | `shared/read-side.md` (owner) · `relational-view.html` |
| the read model's own declaration surface | `views.html` |
| postgres specifics: id column type, the constraint-violation key the binding map binds, active-only uniqueness | `shared/dialects/postgres.md` |
| the layer's process and traps | `conventions/infra.md` |

## What this layer must contain

**The table schema** — one schema per file, six fields plus the managed set
(`revision`, `created_at`, `updated_at`, `deleted_at`). The archive column declaration and
the aggregate's `Archive` mode must agree; they are one decision expressed twice, and a
disagreement is a boot failure rather than a subtle bug. The claim name's storage column is
renamed to avoid the reserved word; **only the storage name moves** — the exposed name stays
`name` in every filter, order-by token, OpenAPI parameter, GraphQL argument and JSON field.

**The repository**, carrying two things beyond the ordinary:

1. **The constraint binding** for the per-tenant active-only uniqueness on the claim name,
   mapping the violation to the 409 notification `spec.md` §7 R5 names. The binding key must
   match what the migration actually creates, exactly — a mismatch does not fail loudly, it
   degrades the intended 409 into a raw 500, and only against a live engine.
2. **The read join into the owner** — inner, on the owner foreign key, bringing the owner's
   handle and its commercial status, both visible on the wire. Inner is correct only because
   the column is NOT NULL and FK-backed. Declared once here and inherited by every consumer
   of the loader: the by-id load the write handlers go through, the scoped reader, and the
   read model. Neither joined field carries a domain type — the value belongs to the owner,
   arrives read-only, and reconstructing the owner's enum here would hand back an instance no
   rule of the owning aggregate ever approved.

**The domain service implementation** — the two facts `task_domain.md` declares. The
uniqueness probe filters on owner **and** name and excludes the row being updated; the
availability probe reads the owner by primary key and answers the PROBLEM (missing, archived
or suspended), with a trial owner reported as available.

**The read model** — relational, the project posture. It is its own declaration type: no
version, no collection, no registry row, no rebuild, no drift, no indexes, no delete-on-
archive; none of those are even methods on it. It is declared by name over this aggregate's
existing loader, which already carries the schema and the join, and contributed through the
relational feature seam. Its reads are read-your-writes and provable immediately, with no CDC
round trip.

## Traps

- **The id column type.** Every managed id and foreign key takes the engine's native id
  shape; a character column there is the boot-trap the final checklist greps for.
- **The uniqueness probe forgetting the owner.** Scoped by name alone it reports a collision
  across tenants, which is the opposite of the decision in `spec.md` §2.
- **Normalizing inside the probe.** The comparison is over the stored string, prefix and all.
- **Declaring free-text search on the read model.** It answers 400.
- **Bundling more than one schema into one file** — a layout violation the compiler will
  not flag.

## Acceptance

- The schema, repository, service implementation and read model compile; `go vet` clean.
- The archive column declaration and the aggregate's mode set agree.
- The join is declared on the repository and on nothing else — not on the schema, not on the
  read model.
- The constraint binding key is written down here and matched literally by
  `task_migrations.md`; the two are checked against each other before either is called done.
- No character-typed id or foreign-key column anywhere in this entity's DDL-facing
  declarations.
