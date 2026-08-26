# task_migrations.md — migrations

Model authority: [`spec.md`](spec.md) §1 — the ER sketch, the table descriptions, the
indexes and the foreign keys.

Dialect: **postgres only.** Numbering continues the service's own sequence; the framework's
own migrations live in a separate tracking table and never collide. Granularity, numbering
and naming per `service-layout` and `conventions/migrations.md` — this file states the
**tables**, not the files.

## The three tables, in foreign-key order

**1. The user table.** Columns: the identifier; the owning tenant reference; the two name
parts; the address; the verification stamp; the hash; the credential timestamp; the
must-change flag; the failure counter; the lock expiry; the account status; and the
framework-managed set — revision, created, updated, removed.

Sizes that are decisions rather than defaults: the hash column is sized for a
parameter-carrying encoded string **with room for raised parameters**, not for today's exact
output. Each name part is sized as a half-name, not as a full one. The address is sized to
the protocol maximum.

Defaults that matter: the failure counter defaults to zero and the must-change flag to a
literal, so a row is valid without a backfill. The lock expiry is nullable and means "not
locked" when null.

**2. The group-membership table.** Its own identifier, the owner reference, the referenced
group, and the managed set.

**3. The direct-role table.** Same shape, referencing a role.

Every table carries a COMMENT, and every column carries one — the text is the description
column of §2 and the table descriptions of §1, in the project's working language.

## Constraints and indexes

- **The address is unique GLOBALLY over active rows** — a partial unique index, not scoped by
  tenant. This is the only uniqueness in this service that is not tenant-scoped, it is the
  README's own decision, and it is what lets the public credential route resolve an address
  to exactly one user with no tenant claim to scope by.
- **Each collection is unique per owner over active rows** — a partial unique index per
  table.
- **The owner reference is indexed** on the user table: it is the read-side scope filter and
  it runs on every listing.
- **The owner reference is indexed on both collection tables** — the parent read — and so is
  the target reference, for the reverse walk when it eventually exists.
- **Three foreign keys**, all to primary keys and never to a partial unique index, which this
  engine will not point a foreign key at. All three take no referential action on delete,
  deliberately and for the reason already written into the role migration: a tenant is
  archived and never purged.

## What to read before writing — routed sections at the pin

| For | Read |
|---|---|
| numbering, the reverse pair, dialect handling | `migrations` |
| the identifier column type for this engine, and its constraint-violation key | the dialect sheet for postgres |
| which columns the framework manages and what soft removal means | `table-schema` |

Convention: `conventions/migrations.md`.

## Traps specific to this layer

- **Every forward migration needs its reverse twin**, even a no-op one, or the boot aborts.
- **A migration that has already run is never edited.** A later change is a new numbered pair.
- **The cross-aggregate foreign keys are outside what a generator writes** — it emits the
  parent key only, so the two references into the other aggregates land in the hand-written
  hook. That is the designed path, not an adoption.
- **The archive column must exist** — it is one third of the three-places-one-fact rule with
  the schema and the modes, and it is also what makes every partial index above meaningful.
- **The framework's own control-plane tables use a different write path.** Never mirror them
  for entity tables.

## Acceptance check

- Three tables, created in foreign-key order, each with its COMMENT and every column with
  its own.
- Both directions of every migration exist.
- The address index is global and active-only; both collection indexes are per-owner and
  active-only.
- No identifier or foreign-key column uses a text type on this dialect.
- The migrations apply from empty and reverse cleanly.
