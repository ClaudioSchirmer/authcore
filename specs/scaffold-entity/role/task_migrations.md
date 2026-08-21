# task: migrations — Role

Model: `spec.md` §1 (the ER sketch and the two table descriptions), §2, §7.
Convention: `conventions/migrations.md`. Layout/granularity/naming: `service-layout.html`.

## READ before writing (mandatory)

- `migrations.html` — numbering, the up/down pairing requirement, dialect placement.
- `table-schema.html` — the column type each Go field maps to, so the DDL and the schema
  cannot disagree.
- `shared/dialects/postgres.md` — the id column type on this engine, and the uniqueness
  shapes available.

## What to build

**Two tables, created in foreign-key order** — the root before its child.

**`roles`** — the four modelled columns of §2 plus the framework's managed set (row id,
revision, created/updated timestamps, archive stamp). Its owning-tenant column is a foreign
key to the tenant table's **public derived key column, not its surrogate row id** (§1); that
column carries a unique index, which is what makes it a legal target on this engine.

**`role_permissions`** — the child: its own row id, the parent reference, the catalog
reference, and the same managed set including its own archive stamp (a revoke is a soft
removal). The catalog reference is a real foreign key to the permission table's primary key.

**Two partial unique indexes**, both scoped to the active rows so an archived row releases
its value:

- the role key, unique **within its tenant** — over the pair, not over the key alone;
- the grant, unique **within its role** — over the parent/catalog pair.

Both are what the repository's constraint bindings map to a clean 409, so their names and
the bindings are written together or the 409 silently becomes a 500.

**Comments on every table and every column**, taken from §1's table descriptions and §2's
field descriptions — the same discipline the two existing tables already follow.

**Every up has its down.** A missing twin aborts the boot.

**Quote every identifier.** One of these table names is a reserved word on this engine; the
project's existing DDL quotes throughout, and this must not be the first place that stops.

## Acceptance

- The numbering continues the service's own sequence, without a gap and without colliding
  with what is already applied.
- Both pairs present; each down actually reverses its up, dropping in reverse dependency
  order.
- The archive column exists on both tables, agreeing with the schema and the modes.
- No fixed-width character type is used for any managed id or foreign key column.
- The tables apply cleanly against a fresh database and against the current one.
