# task_infra.md — infra

Model authority: [`spec.md`](spec.md) §1, §2, §9. Read the two deltas first.

## What this layer must contain

**One storage declaration per file** — the layout standard forbids bundling, and the boot
does not catch it.

**The root's storage declaration**, binding every persisted field to its column, declaring
the managed stamps and the archive column, and declaring **both hash columns as hidden and
as redacted on both axes** — the sync copy and the audit copy. Both axes are mandatory; a
missing one is a construction panic.

**Two collection declarations**, matched against what the aggregate declares — a
disagreement is refused at binding rather than at a write.

**The repository**, with the read joins §9 names: the root's traversal into the owning
tenant for its handle and its lifecycle, and the grant collection's traversal into the role
catalogue for the key and the display name. The network-range collection has none.

**The constraint bindings** for the three unique indexes, so each violation answers a clean
conflict rather than a five-hundred. Postgres binds by constraint name.

**The domain service adapter**, answering each fact the port names, and the new hashing
adapter (see the credential delta — its comparison is constant-time, and that is checked).

**The read model, declared as a relational one** — the project's posture. It has no version,
no collection, no rebuild and no indexes of its own; it is declared over the aggregate's
existing loader, which already carries the shape.

**Neither hash nor the retiring deadline is declared as filterable, sortable or
projectable.** Redaction refuses nothing on the read side, so this is the decision that
keeps them out of the query vocabulary.

## What to read before writing — routed sections at the pin

`table-schema` (the supported column shapes, the identity contract, the redaction family) ·
`relational-view` and the shared read-side owner · `read-joins` and its shared owner ·
`aggregate-persistence` · `service-layout`. Dialect sheet: postgres only.
Convention: `conventions/infra.md`.

## Acceptance

- No file declares more than one storage shape.
- Both hash columns are hidden and redacted on both axes.
- Every unique index has a binding and answers a conflict.
- A filter or sort naming a hash is a typed refusal.
- An identifier-typed field is paired with the dialect's native identifier column, and a
  text-typed one with text — the pairing is what the first insert proves.
