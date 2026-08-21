# task: infra — Role

Model: `spec.md` §1, §2, §3, §7, §9. Convention: `conventions/infra.md` +
`conventions/aggregate-children.md`. Layout/naming: `service-layout.html`.

## READ before writing (mandatory)

- `table-schema.html` — the Go↔column mapping, the supported column shapes, the child
  declaration, the archive-column declaration, and the boot checks around all of them.
- `aggregate-persistence.html` — how the aggregate is written and how children cascade.
- `views.html` + `relational-view.html` — the view surface, the version field, and precisely
  what a source-of-record-backed view serves and refuses.
- `service-to-service.html` — the shape of a service implementation that reaches another
  aggregate's store.
- `shared/dialects/postgres.md` — the engine's id column type, and **the key a constraint
  violation is bound by on this engine** (this is what turns a duplicate into a clean 409
  instead of a raw 500).

## What to build

**Two schemas, one per file** — the root and, declared on it, the child. The child type set
declared on the aggregate and the child set declared on the schema must match, or schema
binding panics at boot. Depth is one: the child declares no child of its own.

**The archive column is declared on both** — the root's declaration must agree with the
modes, and the child needs its own so a revoke is a soft removal rather than an error.

**The two runtime-only identity fields are NOT declared** in the schema. That absence is
what keeps them out of every write and every scan; declaring them would try to persist a
claim.

**The repository, with two constraint bindings** — the role key per tenant, and the duplicate
grant. Each maps its engine-level violation to the custom notification named in §7 so the
race that slips past the domain pre-check still surfaces as a clean 409. Bind them by the key
shape this engine actually reports, read from the dialect sheet — not from SQL habit.

**The service implementation.** It answers the four facts of the domain port and is bound to
the request through the framework's scoping hook, exactly as the two existing services are.
Three of its facts are database probes; the fourth reads the request identity, and **must
refuse a wildcard argument itself** rather than calling through to the framework helper,
which panics on one. A failed probe panics rather than inventing an answer — the local
pattern, and the reason is that a plausible answer here skips the invariant.

**Cross-aggregate reach:** two of the probes query the catalog and the tenant tables, so this
implementation holds those repositories beside its own. Confirm the shape against the
service-to-service section before writing it.

**The view**, relational-backed, sharing the repository's own loader — never a second one
built here. Its version starts at one. Confirmed against the docs: this backing DOES serve
the child collection inside the document; what it refuses is a filter or sort over a child
field, and that refusal is a typed 400 the web layer must not try to pre-empt.

## Acceptance

- One schema per file; the aggregate's child type set and the schema's child set agree.
- Modes ⟺ archive declaration ⟺ (next layer) the migration column: all three agree.
- The view's loader is the repository's own, and the loader's bound table matches the
  schema's — the boot asserts it.
- Both constraint bindings use the engine's actual violation key shape.
- Builds and vets clean.
