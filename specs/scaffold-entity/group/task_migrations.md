# Task 5 — migrations

## Docs to READ (mandatory, at the pin)

- `migrations` — numbering, the up/down pair, and dialect layout.
- `yaml-reference` — how the profiles point at the migration set.
- `table-schema` — the column shape per Go type, so the DDL and the schema agree.
- `shared/dialects/postgres.md` (the pinned plugin's copy) — id column type, decimal and
  boolean shapes, and active-only uniqueness.

Convention: `conventions/migrations.md`. Numbering, naming and granularity per
`service-layout.html` — this is a **new numbered pair** continuing the service's own
sequence, never an edit to a pair that has already run.

## Tables, in FK order

**1. `groups`** — the root.

- The row id (PK), the owner tenant reference, the entity-prefixed handle, the display name,
  the description, and the framework's managed columns: the revision stamp, created-at,
  updated-at and the archive stamp.
- Column widths per spec §2; every column carries the spec's description as its COMMENT, and
  the table carries the spec §1 table description as its own.
- **Unique over (tenant reference, handle), scoped to the active rows** — an archived group
  releases its handle. This is the index the repository's constraint map binds.

**2. `group_roles`** — the collection.

- The entry id (PK), the owner group reference, the role reference, and the managed columns
  the collection needs including its archive stamp.
- The parent FK **cascades on delete**; an index on the parent key, because every read of the
  aggregate loads the collection by it.
- **Unique over (owner group, role reference), scoped to the active rows** — a detached entry
  releases the pair so it can be re-attached. This is the backstop the business-identity
  check cannot be: that check sees one write, so two concurrent attaches of the same role
  both pass it and both rows land.

## The two cross-aggregate foreign keys — write them by hand, they are ours

Mirroring what `Role`'s own migration already does in this repository (read it before
writing these; it carries the reasoning in comments and this pair should read the same way):

- **`groups` → the tenant registry.** The target is the registry's **public derived key**,
  not its surrogate row id — that is what the tenant claim carries and what the isolation
  filter compares, so pointing at the surrogate would cost a lookup on every read. Its unique
  index is total (the handle is never reused), which is what makes it eligible as an FK
  target where a partial index would not be. **No action** on delete, deliberately: a tenant
  is archived and never purged.
- **`group_roles` → the roles table.** The target is the **primary key**. Not any of the
  partial unique indexes — Postgres will not point a foreign key at one. This buys the
  existence half of G6 for free; the domain rule still earns its keep for the **active** half
  and the **same-tenant** half, and for turning a violation into a readable 422 instead of a
  raw constraint error.

## Non-negotiables

- **Quote every identifier.** The entity's singular name is reserved in Postgres and the
  handle's bare column name is reserved across the engine set; this pair must not be the
  first place in the repository that stops quoting.
- **Every up has its down**, or boot aborts. The down drops in reverse FK order.
- Only the postgres directory exists in this service — no other dialect to mirror.

## Acceptance check

- A new numbered up/down pair, continuing the sequence; the down is the exact inverse.
- Both tables, both partial unique indexes, the parent index, and the two hand-written
  cross-aggregate FKs.
- Every table and column carries its COMMENT from the spec.
- No 36-char id columns anywhere — the postgres sheet's native id type, matching the Go type.
- Applying and rolling back against the local bench leaves no residue.
