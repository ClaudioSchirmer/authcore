# task_migrations.md — migrations

Model authority: [`spec.md`](spec.md) §1 (the sketch), §2 (the descriptions).
Dialect: **postgres only**. Read the children delta first.

## What this layer must contain

**Three tables, in foreign-key order**: the root, then the grant collection, then the
network-range collection. Columns, types, nullability and constraints exactly as §1's sketch
states them, with the per-table and per-column descriptions of §2 carried into the DDL as
comments — they are the same sentences, not a paraphrase.

**The current-hash column is sized for the hash the entity actually produces**, not for the
format the password column uses. Sizing it for a format it does not use would invite
somebody to put one there.

**Three partial unique indexes**, each scoped to the active rows so an archived row releases
its value: the label per tenant on the root, and the referenced value per owner on each
collection.

**The owner index on the root**, because the isolation filter runs on every listing and must
not lean on the primary key.

**Foreign keys to the other aggregates are hand-written below a marked line** — a reference
to another aggregate is outside the generator's spec language and is the designed hook, not
an adoption. The owning tenant and the granted role both take no action on delete: neither
is ever purged, so there is no delete for a cascade to follow, and letting the database
remove clients because a tenant row vanished would be a rule nobody declared.

**The reverse index on the grant collection**, so "which clients hold this role" is
answerable — and because the foreign key check asks it on every role delete.

**Numbering and granularity per `service-layout` and the migrations convention** — this
layer does not restate them. **Every up statement has its down twin**, which may be a no-op;
its absence aborts the boot.

## What to read before writing — routed sections at the pin

`migrations` · `table-schema` (the identifier column per engine) · `yaml-reference` ·
`service-layout`. Convention: `conventions/migrations.md`. Dialect sheet: postgres.

## Acceptance

- Every up statement has its down twin.
- Every table and column carries the description §2 gives it.
- The three partial unique indexes are scoped to active rows.
- The foreign keys sit below the marked hand-written line with their reasoning.
