# task: migrations — Permission

## Read first (mandatory, at execution time)

- `migrations` — numbering, the up/down pairing requirement, and dialect handling.
- `table-schema`, "Supported column shapes" — the exact column type per Go type on this
  pin, and the partial-index form for active-only uniqueness. Never typed from memory.
- `../../../shared/dialects/postgres.md` (plugin) — the identity column type, the
  constraint-name convention the repository binds, and the fact that table and column
  descriptions belong in the catalogue as comment statements rather than as SQL line
  comments.
- Convention: `conventions/migrations.md`. Granularity and naming: `service-layout`.

## What to create

One table, in the project's single migration directory (postgres is the only dialect).

**Table `permissions`** — the global catalog of enforceable permissions.

| Column | Type | Null | Notes |
|---|---|---|---|
| id | the engine's native identity type | not null | primary key |
| revision | integer | not null | framework-managed |
| resource | varchar(64) | not null | first part of the permission key |
| action | varchar(64) | not null | second part |
| description | varchar(500) | not null | |
| deleted_at | timestamptz | null | archive marker; one-way |
| created_at | timestamptz | not null | |
| updated_at | timestamptz | not null | |

**One partial unique index over the pair**, restricted to rows whose archive marker is
null. Its name must be exactly what the repository binds — decide the name once, in this
task, and use the same string in both places. This partial-ness is load-bearing, not a
nicety: `spec.md` §6 makes re-insertion the only way a retired permission returns, and a
total unique index would close that door permanently.

**No seed data.** `spec.md` §B Q4 settled this: the table ships empty and an operator
populates it through the API. Do not insert the permissions this service itself enforces.

**Descriptions**: one line per table and per column, written into the catalogue as comment
statements after the create, in English. The table description is quoted verbatim in
`spec.md` §1; the column descriptions are the `Description` column of `spec.md` §2.

**A down twin** that reverses it, or the boot aborts.

## Acceptance

- The service migration sequence starts where the project's own sequence continues — this
  is the second entity, so it follows the existing tenant migration, and the framework's own
  sequence is tracked separately and never collides.
- Every up statement has its down twin.
- No identity column is declared as a 36-character text type.
- The partial index name matches the repository's binding exactly.
- The table applies cleanly against the local bench and the framework's check mode accepts
  it.
