# task_domain.md — domain

Model authority: [`spec.md`](spec.md) §2, §5, §6, §7. Read
[`task_children.md`](task_children.md) and [`task_credential.md`](task_credential.md) first —
both cross this layer.

## What this layer must contain

**The aggregate root**, carrying: the owning tenant reference, the name as a **composite
value object**, the address, the hash, the verification stamp, the credential timestamps and
flag, the account status, the two collections, the two lockout fields, and the runtime-only
values (the three credential inputs and the two identity inputs). Every field carries a label
key and nothing else — no wire tags, no storage tags.

**Two aggregate value objects**, one per collection, each holding the referenced id plus the
two read-only values filled across the foreign key.

**Four value objects.** A composite for the person's name, spanning two parts, declaring its
validity rule **and the method that renders the full name** — and declaring **no
single-value accessor**, which is what makes it a composite rather than a scalar. A raw one
for the address. A raw one for the password policy. An enum for the account status. The name
and the password compose the project's shared anti-junk predicates, which is why neither is
statable as a pattern.

**The rule set of §7**, in the spec's own numbering, including the two barriers (the owner
check and the credential check), the conditional owner source, the two immutability rules,
the status transition machine, the archive-forces-suspended mutation, and the escalation
pairs at both depths.

**A service port** naming one fact per cross-aggregate question, plus the hash comparison.
Every fact is **named for the problem, never for the healthy state** — the generated suite
stubs the service so each probe answers "nothing found", which is what lets a valid fixture
through; a fact named for the healthy state reads false under that stub and turns a correct
spec red on the day it is written.

**Notifications** for everything §7 names that the framework does not already own, each with
its seven translations. Several already exist in this service and are **reused, not
duplicated** — the spec's §7 table says which.

## What to read before writing — routed sections at the pin

| For | Read |
|---|---|
| the rule DSL, verb scoping, barriers and what does NOT short-circuit | `rules-dsl` |
| the previous-state snapshot and its behaviour on insert | `old-state` |
| how a notification maps to a status, and where a field's label comes from | `status-mapping` |
| the three kinds of value object, the automatic pass, and its two adjustments | `value-objects` |
| a composite's own family of boot panics | `table-schema` — the composite section · `value-objects` |
| rules that need a fact from another aggregate | the shared query-primitives note · `custom-command-handler` · `service-to-service` |
| the modes an aggregate declares | `auto-handlers` |

Convention: `conventions/domain.md`.

## Traps specific to this layer

- **The composite must not declare a single-value accessor**, must not be passed to the plain
  field mapping, must appear exactly once on the entity (resolution is by type), and must
  carry no custom JSON behaviour and no excluded part — the previous-state snapshot is a JSON
  round-trip and both poison it.
- **A wire tag on a domain field is the number-one reflex slip.** A domain aggregate is not a
  wire DTO, and an excluded field also corrupts the snapshot.
- **Rules do not short-circuit.** Raising a notification does not stop the clauses below it;
  only a guard ends the pass. The two barriers are positional — each must be declared first
  in its verb's list.
- **The previous-state snapshot is absent on insert.** Every rule reading it guards.
- **A child method that emits a notification before delegating** must ensure the root is
  initialized first, or the notification is silently dropped. A method that only delegates
  does not need it — the framework's own add and change helpers already do it.
- **The automatic value-object pass must be switched off, per mode, for the plaintext** on
  every write that does not carry one.

## Acceptance check

- Every rule in §7 exists, in its verb scope, raising the notification the spec names.
- The two barriers are declared first in their verb lists and actually end the pass.
- The archive mutation reaches the row, not merely the audit event.
- The status machine refuses what §7 says it refuses and no-ops what it says it no-ops.
- No wire or storage tag anywhere under this layer; every field carries its label key.
- The service port's facts are all named for the problem.
- Build and vet clean.
