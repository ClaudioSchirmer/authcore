# task: infra — Role

Model: `spec.md` §1, §2, §3, §7, §9. Convention: `conventions/infra.md` +
`conventions/aggregate-children.md`. Layout/naming: `service-layout.html`.

## READ before writing (mandatory)

- `table-schema.html` — the Go↔column mapping, the supported column shapes, the child
  declaration, the archive-column declaration, and the boot checks around all of them.
- `aggregate-persistence.html` — how the aggregate is written and how children cascade.
- `views.html` + `relational-view.html` — the view surface and precisely what a
  source-of-record-backed view serves and refuses. Note which of the two families the version
  field belongs to: a relational view has none.
- `read-joins.html` — the whole of it. This is the layer that declares the traversal, and
  every consumer of the loader inherits whatever is declared here.
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

**The repository, with two constraint bindings AND the read join** — the role key per tenant,
and the duplicate grant. Each maps its engine-level violation to the custom notification named in §7 so the
race that slips past the domain pre-check still surfaces as a clean 409. Bind them by the key
shape this engine actually reports, read from the dialect sheet — not from SQL habit.

**The read join into the catalog, declared with `WithJoins` beside `WithSchema`** — spec §2:

```
read.InnerJoinInChild(<child schema>).To(<permission schema>)
  RolePermission.permission_id  =  permissions.id
    → Resource    ← permissions.resource_name
    → Action      ← permissions.action_name
    → ArchivedAt  ← permissions.deleted_at        -- managed column; *time.Time
```

Four things about this declaration that this layer is responsible for, because nothing
downstream can repair them:

- **It is declared once, HERE, and inherited by everything.** `repo.FindByID` (the load the
  write-side auto handlers go through), `repo.ScopedReader(ctx)`, the relational view below,
  and **any other aggregate's service reading through `repo.Loader`** all get it in the same
  call. That last one is not incidental: `../group/spec.md` §7 resolves a role to the
  permission keys it grants in ONE read through this loader, and it can only do that because
  the traversal lives on the repository rather than on the view. Do not move it, do not
  duplicate it on the view, and do not let a second loader be built anywhere.
- **`inner` is correct only because `permission_id` is `NOT NULL` and FK-backed.** Inside a
  child an inner join drops the ENTRY; over a nullable key it would drop entries from
  `FindByID` too, turning a legitimate write into a 404.
- **The join fields are read-only and carry no domain type.** `Resource` and `Action` are
  plain `string`, never `vos.PermissionKey`; `ArchivedAt` is `*time.Time`, nullable because
  the target's `deleted_at` is. None of the three is in the `TableSchema`, so none reaches an
  INSERT or an UPDATE.
- **`ArchivedAt` renders, it never gates.** The join is not archive-gated: an archived
  permission keeps supplying its columns and the inner join keeps matching it. The rule that
  decides whether a permission may be granted stays in the domain service below — see the
  next block for why reading `ArchivedAt` there would be fail-open.

**The service implementation.** It answers the **six** facts of the domain port and is bound to
the request through the framework's scoping hook, exactly as the two existing services are.
**The read join does not replace any of them, and one of them must not read it.** These facts
judge the grants a write is ADDING, and an added entry has no joined value: `Resource` reads
`""` and `ArchivedAt` reads `nil` — the same `nil` a live permission carries. A probe that
tested `ArchivedAt == nil` would therefore pass every entry being added, fail-open on exactly
the check it exists to make. The facts query the catalog; they never read the join.
**Four** of its facts are database probes — the key-taken probe, the tenant probe, and the
two per-entry catalog probes (in-catalog and wildcard). **Two** read the request identity: the
no-escalation probe, which **must refuse a wildcard argument itself** rather than calling
through to the framework helper, which panics on one; and the superadmin question, which is
`ctx.Identity().IsSuperAdmin()` and nothing else — never a hand-read of the claim, whose NAME
is configurable. A failed probe panics rather than inventing an answer — the local
pattern, and the reason is that a plausible answer here skips the invariant.

**Cross-aggregate reach:** three of the probes query the catalog and one queries the tenant
table, so this
implementation holds those repositories beside its own. Confirm the shape against the
service-to-service section before writing it.

**The view**, relational-backed, sharing the repository's own loader — never a second one
built here — and contributed through the feature's `RelationalViews()` opt-in.
`query.RelationalView("roles", repo.Loader)` takes its schema from the loader, so there is no
`.Schema(...)` to get out of step, and it carries **no `Version`**, no registry row, no
rebuild and no Mongo collection: do not write a version field for it. It declares **no join
of its own** — `Resource`, `Action` and `ArchivedAt` reach the served document because the
repository declared the traversal. Confirmed against the docs: this backing DOES serve the
child collection inside the document; what it refuses is a filter or sort over a child field,
and that refusal is a typed 400 the web layer must not try to pre-empt.

## Acceptance

- One schema per file; the aggregate's child type set and the schema's child set agree.
- The read join is declared **on the repository** with `WithJoins`, maps the three fields of
  spec §2, and is `inner` in the child. The view declares none of its own, and there is
  exactly one loader.
- No join field appears in the `TableSchema`, in a command, or in any request DTO.
- No rule and no service fact reads `ArchivedAt`.
- Modes ⟺ archive declaration ⟺ (next layer) the migration column: all three agree.
- The view's loader is the repository's own, and the loader's bound table matches the
  schema's — the boot asserts it. The view carries no `Version` field and is contributed
  through `RelationalViews()`.
- Both constraint bindings use the engine's actual violation key shape.
- Builds and vets clean.
