# task_infra.md — infra

Model authority: [`spec.md`](spec.md) §1, §2, §9. Read
[`task_children.md`](task_children.md) and [`task_credential.md`](task_credential.md) first.

## What this layer must contain

**The table schema** for the root and for both collection tables — the Go-to-column
bijection, the managed columns, and the archive column, which must agree with the modes the
domain declares and with what the migration carries. Three places, one fact.

The schema carries **two shapes this service has not combined before**:

- the **composite** name, mapped across its two columns through the composite declaration —
  never through the plain field mapping, which is a boot panic naming its own fix;
- the **redacted** hash, which keeps the real value in the column and in the hydrated entity
  while masking it in each copy the framework makes of the row. **Both axes are mandatory**;
  a missing one is a construction panic, because the framework refuses to decide in your
  place whether silence meant leak or mask. Neither axis is the plain one here.

**The repository**, declaring the three traversals — one to the owning tenant from the root,
one per collection to its target — with the constraint bindings that turn a uniqueness
violation into the conflict status the model names. On this dialect a binding is keyed by the
constraint name.

**The relational read model**, declared over the repository's loader and contributed through
the relational feature seam. It takes its schema from the loader, so there is nothing to get
out of step, and it inherits the three traversals without declaring anything. It carries no
version, no registry row, no rebuild and no collection — none of those are even methods on it.

**The service implementation** behind the domain's port: eight cross-aggregate facts and the
hash comparison. The facts reach the tenants, groups and roles tables; the two escalation
walks resolve through the target repositories' own loaders, which already carry their
traversals, so no second query into the catalog and no id-to-handle step anywhere.

**The password hasher adapter** — Argon2id at the OWASP baseline, PHC-encoded, plus the
constant-cost path on a missing address and the absolute prohibition on logging its input.
See [`task_credential.md`](task_credential.md).

## What to read before writing — routed sections at the pin

| For | Read |
|---|---|
| the Go-to-column bijection, supported shapes, the composite mapping, the redaction family | `table-schema` |
| reaching another aggregate across a foreign key — declaration, inheritance, what a collection-level traversal cannot do | `read-joins` |
| what a relational read model serves and what it refuses | `relational-view` |
| indexes and the reserved-name rule for read models | `auto-query-handlers` |
| a service reaching another aggregate's repository | `service-to-service` |
| how a redacted value appears in the trail | `audit` |
| this dialect's identifier column type, its constraint-violation key, and active-only uniqueness | the dialect sheet for postgres |

Convention: `conventions/infra.md`.

## Traps specific to this layer

- **One schema per file.** Count the root constructor only — a schema legally contains other
  builder constructors.
- **Pair the field type with the column.** An identifier-typed field needs the dialect's
  native identifier column; a text-typed field is text always. A mismatch is a runtime
  failure on the first write that the build cannot catch.
- **The traversals are inner joins, and that is only safe because all three keys are
  non-null.** Over a nullable key an inner join silently drops rows — from the by-id load
  too, which turns a legitimate write into a not-found — and inside a collection it drops the
  entry, leaving a hole in the array rather than a missing aggregate.
- **A traversal's predicate is always the foreign key against the target's identifier.** A
  traversal onto any other column is not expressible.
- **A traversal field carries no domain type** — no value object and no identifier type. The
  value belongs to the owning aggregate and arrives read-only.
- **The traversals must not bring the target's removal stamp.** Both target aggregates
  deliberately keep it off their own reads; republishing it here is a back door to a decision
  already made the other way.
- **The read model's name may not end in the two reserved slot suffixes.**
- **Nothing on the read side is refused because a field is redacted** — filters and ordering
  keep working. Keeping the hash out of the query vocabulary is the web layer's job, not this
  one's, and it is a separate decision that has to be made by hand.

## Acceptance check

- The schema declares the archive column, and it agrees with the modes and with the
  migration.
- The composite reaches the schema through the composite declaration; the build boots.
- The hash is declared redacted with **both** axes, neither of them plain.
- Three traversals declared on the repository, none on the schema and none on the read model.
- The constraint bindings cover the global address index and both per-owner indexes.
- The read model is contributed through the relational seam and declares no schema of its own.
- Build and vet clean.
