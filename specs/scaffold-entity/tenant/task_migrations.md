# task — migrations

Model authority: `spec.md` §1 (the ER sketch), §2, §7. Granularity and numbering: the
migrations convention plus `service-layout.html` — this file specifies WHAT the schema must
contain, never how many files carry it or what they are called.

## Read BEFORE generating (mandatory, at pin v0.54.0)

| Section | Why this layer needs it |
|---|---|
| `migrations.html` | numbering, the up/down pairing, and how the service sequence relates to the framework's own |
| `table-schema.html` | the Go-to-column table for postgres — the authority on every column type here |
| `yaml-reference.html` | how the migration directory and the auto-run mode are resolved per profile |
| `shared/dialects/postgres.md` (plugin) | the native identity type, the partial-index form, and how table and column descriptions reach the catalogue |

## What to build

One table, `tenants`, exactly as `spec.md` §1 sketches it. Target dialect: postgres only —
it is the single dialect in both profiles.

**The service sequence starts at one.** The framework's own migrations live in a separate
tracking table, so there is no collision to avoid and no offset to leave.

**Columns**, with the types the postgres sheet gives for each Go type in `spec.md` §2 — the
identity and the public tenant id both take the dialect's native identity type, not text.

**Constraints:**

- the primary key on the identity column;
- a plain unique constraint over the workspace column, spanning all rows — **not** a partial
  index. `spec.md` §7 rule 5 and §A.2 explain why: an archived remnant must block a new
  tenant taking that handle, because the handle derives the public key;
- a unique constraint over the public tenant id column;
- **no** constraint on the name column, and no partial index over it — `spec.md` §B Q8
  removed it, and re-adding it here would silently reinstate a rule the maintainer reversed;
- **no** check constraint over the status column — `spec.md` §2 records that decision and
  its reason.

**Every constraint is named deterministically**, and those exact names are what the
repository binds in the infra layer. The two must agree or the intended 409 becomes a raw
500. Naming them is a decision made once and written in two places.

**Descriptions.** The table comment of `spec.md` §1 and the per-column descriptions of
`spec.md` §2 are emitted as catalogue comments, not as SQL line comments — a `--` line is
invisible to every client and to the catalogue, which is the entire audience it was written
for. Apostrophes inside the text need doubling.

**The down direction** exists for every up. Boot aborts on a missing counterpart, so this is
structural rather than tidy.

## Acceptance

- The service sequence begins at one.
- Every up has its down counterpart.
- No managed identity column is text — a grep for the fixed-width character types over the
  migrations directory must find nothing for this table.
- Every constraint name in the DDL matches a binding in the repository, and no binding names
  a constraint the DDL does not create.
- The workspace uniqueness is total, not partial. The name carries no uniqueness at all.
- Table and column descriptions land in the catalogue.
