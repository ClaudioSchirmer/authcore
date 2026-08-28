# task_migrations — Claim

Model authority: [`spec.md`](spec.md) §1 (the ER sketch and the table description), §2.
Numbering, granularity and naming: `conventions/migrations.md` + `service-layout.html` —
normative. Nothing about file count or numbering is decided here.

## Read BEFORE writing this layer

| What | Where |
|---|---|
| numbering, the down twin, dialect layout | `migrations.html` · `conventions/migrations.md` |
| the column type per Go type, and the id shape | `table-schema.html` · `shared/dialects/postgres.md` |

## What this layer must contain

**One table, `claims`**, in the project's single dialect. Columns exactly as the `spec.md`
§1 sketch draws them, in that order:

- the primary key, in the engine's native id shape;
- the owner foreign key — NOT NULL, in the same native shape, **with a real FK constraint to
  the owning table**. A reference to another aggregate is outside the generator's language,
  so if this run takes the codegen path the constraint is appended by hand — the same move
  the existing tenant-owned entities record;
- the claim name — the renamed storage column, bounded to the length `spec.md` §2 gives;
- the value type and the applies-to — bounded text holding the enum member values;
- the default value — **nullable**, bounded to the length §2 gives, which is the claim-size
  budget made concrete;
- the description — NOT NULL, bounded to the shared prose length;
- the managed set: revision, created-at, updated-at, and the nullable archive stamp.

**One partial unique index**: the owner together with the claim name, **restricted to rows
whose archive stamp is null**. Two things depend on getting this exactly right and neither
fails loudly:

1. its **name must match the repository's constraint binding literally** — a mismatch turns
   the intended 409 into a raw 500, and only against a live engine;
2. the **partial predicate is what makes archived rows stop blocking a re-insert**, which is
   the only way back with no unarchive verb. A full unique index instead would make a
   retired name permanently unusable in that tenant.

**Comments.** The table comment and every column comment carry the descriptions already
written in `spec.md` §1 and §2 — the DDL is where an operator reads them, so they are not
optional decoration.

**A down twin**, without which boot aborts.

**No seed.** The platform's nine are not written here; `spec.md` §0 says why, and the
reserved platform tenant they would belong to does not exist yet.

## Traps

- **A character-typed id or foreign-key column.** The native id shape, on both the primary
  key and the owner reference.
- **A full unique index where the spec says partial.** Silent, and it only bites the first
  time somebody archives a definition and tries to recreate it.
- **A missing down twin.**
- **Starting the sequence anywhere but after the service's existing highest number.**

## Acceptance

- The up and the down both apply cleanly against a live engine, and the down leaves nothing
  behind.
- The unique index name matches the repository binding, verified by reading both.
- Archiving a definition and re-inserting the same name in the same tenant **succeeds**;
  inserting a duplicate while the first is active is refused with the 409 notification, not
  a 500.
- Every column carries a comment.
