# task — infra

Model authority: `spec.md` §1, §2, §7, §9. Layout/naming/granularity: `service-layout.html`.

## Read BEFORE generating (mandatory, at pin v0.54.0)

| Section | Why this layer needs it |
|---|---|
| `table-schema.html` | the schema DSL, the managed-column declarations, the boot checks, and the Go-to-column table for postgres — the authority on every column shape, never memory |
| `relational-view.html` | what the relational backing serves and what it refuses, at this exact pin |
| `views.html` | the view declaration surface and its version rule |
| `auto-query-handlers.html` | the view's indexes and options, and the filter operator vocabulary |
| `custom-command-handler.html` | the loader's hydration-free existence probe, which is what the uniqueness pre-check uses |
| `service-to-service.html` | the channels a domain Service implementation may use |
| `service-layout.html` | one schema per file, one view per file, repositories at the layer root |
| `shared/dialects/postgres.md` (plugin) | the identity column type, and the constraint key the repository binds |

**v0.54.0 note that changes this layer:** unarchiving through a repository that cannot load
an archived aggregate is now an error — the empty-sample fallback is gone. The framework's
base aggregate repository provides the needed capability; a hand-rolled repository would
have to implement it. Confirm the repository this entity gets is on the supported path
rather than assuming it.

## What to build

**The table schema** for `tenants`, binding the five fields of `spec.md` §2 to their
columns, declaring the identity column, the three managed timestamp columns, the revision
column, and the archive column. The archive column declaration and the aggregate's mode set
must agree — the boot checks that pair, and a disagreement is a panic rather than a warning.

**The repository**, with its constraint bindings: the two uniqueness violations of
`spec.md` §7 rules 5 and 6 must each map to the custom notification the spec names, so the
duplicate surfaces as the intended 409 rather than a raw 500. On postgres the binding key is
the constraint name, which means the migration must name those constraints deterministically
and this layer must bind the same names — the two files are one decision written twice, and
they are the easiest pair in the whole entity to let drift.

**The domain Service implementation**, answering the single question `spec.md` §7 leaves it:
has this workspace been used, on any row, active or archived. It uses the loader's
existence probe rather than hydrating an aggregate to discover a boolean.

**The view**, relational-backed per `spec.md` §9, reusing the aggregate's existing loader.
A second loader over the same table boots fine and is pure waste; a loader bound to a
different table fails the boot guard. Its archive regime is kept-but-hidden — not a choice
at this backing, since the drop-on-archive knob is a projection concept and a relational
view composes from the source at read time.

## Acceptance

- The schema declares one entity, in its own file, per `service-layout.html`.
- The identity column and the public tenant id column both use the dialect's native identity
  type, per the postgres sheet — a text column there would fail on the first insert, at
  runtime, in a way the build cannot catch.
- Every constraint the repository binds exists in the migration under exactly that name.
- The uniqueness probe hydrates nothing.
- The view carries no free-text search, because the backing cannot serve it.
- Layout and naming match `service-layout.html`.
- `go build -tags postgres` and `go vet -tags postgres` clean.
