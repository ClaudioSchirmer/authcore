# task: domain — Role

Model: `spec.md` §1, §2, §3, §5, §7. Convention: `conventions/domain.md` +
`conventions/aggregate-children.md`. Layout/naming: `service-layout.html`.

## READ before writing (mandatory)

- `rules-dsl.html` — the seven clauses, the mode dispatch table, and the fact that an
  aggregate value object's own rules fire under the ROOT's mode with the root's action name.
- `old-state.html` — the previous-state snapshot the immutability rules compare against, and
  why it is nil on insert.
- `value-objects.html` — raw vs enum vs composite; auto-validation by type; why a rule that
  a value object already carries must not be restated on the entity.
- `aggregate-persistence.html` — the child primitives, the mandatory business-identity
  method, the child-id write-back.
- `table-schema.html` §"Supported column shapes" — confirm the pin's identity contract
  before typing any id-bearing field.
- `status-mapping.html` — which notification maps to 403 / 404 / 409 / 422, and the
  child-not-found distinction (the canonical not-found is 404; the framework's
  does-not-exist is 422).
- `custom-command-handler.html` + `auto-handlers.html` — the ctx-bound service probe.

## What to build

**A new raw value object for the role key.** A lowercase slug, 2–64 runes, groups separated
by single hyphens so a hyphen can never lead, trail or double; the project's existing
anti-junk predicates (distinct-rune floor, no long run of identical runes) reused rather
than re-written. **No reserved list and no derivation** — those two rules belong to the
tenant handle and must not be dragged onto a role. It normalizes nothing: a value the caller
did not send is never stored.

**The reuse decisions are already made in §2 and are not re-opened here**: the display name
and the description reuse the existing shared value objects.

**The aggregate root.** Four persisted fields per §2, each carrying a label key and nothing
else — no wire tags of any kind on a domain type. Plus the two runtime-only fields carrying
the translated identity (the requesting tenant and the superadmin flag); these are
deliberately absent from the table schema, which is what keeps them out of persistence and
out of the state snapshot.

**The child aggregate value object**, in the aggregate-value-object package with its own
scoped notifications: one field, the catalog reference. It embeds the framework's managed
carrier and declares **no id field of its own** — a hand-declared exported id compiles, is
never persisted, and the real id never round-trips. Its business-identity method is written
explicitly over the catalog reference.

**Modes** per §5 — display, insert, update, archive; **no unarchive**. The set must agree
with the schema's archive-column declaration or the repository refuses to construct.

**The rules**, R1 through R9b of §7, each in the clause that IS its verb — never an action
name string used to tell archive from update. Nothing that a value object already validates
is restated. The order inside the insert-or-update clause matters for exactly one pair: the
wildcard refusal runs before the no-escalation check, because it is what removes the input
that would panic.

**A domain method per child mutation**, each earning its existence: the grant path carries
the duplicate guard (reusing the business-identity method rather than writing a second
check), the revoke path carries the by-id guard and its 404. A method that would only
delegate to a primitive is not written.

**The service port** — four facts, plain values, no error, matching the local pattern
already set by the two existing services. Two of them reach other aggregates; one of them
reads the caller's identity and must refuse a wildcard argument itself rather than calling
through.

**Notifications** — the new ones listed in §7, added to the shared domain notifications
file. The two tenant-isolation ones are **framework-owned and already translated**; they are
consumed, never redeclared.

## Acceptance

- No wire tag of any kind on any domain type (the shared-snapshot corruption trap).
- Every regex and every format rule lives in the value-object package, not in the aggregate's
  or the child's rules.
- Every rule in §7 has exactly one clause and one notification, and every new notification
  reaches all seven catalogs (the application layer does that; this layer must not leave one
  without a key).
- The modes and the archive declaration agree.
- Builds and vets clean.
