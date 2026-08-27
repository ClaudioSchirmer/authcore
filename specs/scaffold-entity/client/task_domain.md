# task_domain.md — domain

Model authority: [`spec.md`](spec.md) §2, §5, §6, §7. Read
[`task_children.md`](task_children.md) and [`task_credential.md`](task_credential.md)
first — both cross this layer.

## What this layer must contain

**The aggregate root**, carrying: the owning tenant reference, the label, the description,
the current hash, the credential timestamp, the retiring hash and its deadline, the account
status, the two collections, and the runtime-only values — the minted secret, the requested
grace window, and the five identity inputs (presence, tenant, scope-crossing, subject and
subject kind). Every field carries a label key and nothing else: no wire tags, no storage
tags. A wire tag on this struct also corrupts the snapshot the framework compares old state
against.

**Two aggregate value objects**, one per collection — see the children delta for how they
differ.

**Two value objects.** An enum for the account status, a verbatim structural copy of the
user one and NOT a reuse of it: a column on this table typed with the other entity's status
reads wrong and couples two lifecycles that may diverge. And a raw one for a network range,
which parses, normalises host bits away, and refuses the two universal prefixes — a
composition of parse, mask and two refusals rather than a pattern, so it cannot be stated
declaratively.

**No value object for the secret.** Its rule would be a rule about caller input, and the
value is never caller input. Spec §A is the argument; a layer that adds one has added a rule
that can never fire.

**The rule set of §7**, in the spec's own numbering, including the barrier, the owner check,
the conditional owner source, the tenant immutability rule, the status transition machine,
the archive-forces-suspended mutation, the escalation pair on the grant collection, the
per-collection caps, and the row rule that restricts a client-subject caller to its own row.

**The row rule is inert on the day it is written** and must stay written. It reads a claim
nothing mints yet, so it evaluates to "not a client" and changes nothing — deliberate, per
spec §B-Q8e. Do not remove it and do not add a fallback that guesses the subject kind from
some other claim's absence; that inference was weighed and rejected at the gate.

**The subject comparison reads the identity's canonical subject accessor, never the raw
claim map.** The framework builds identities that set a subject and carry no such claim, so
a comparison against the map answers "not the owner" to everyone. The existing user
credential file records this trap in full.

**A service port** naming one fact per cross-aggregate question — the label-taken probe, the
tenant probes and the role probes — plus whatever the hashing decisions need. Every fact is
**named for the problem, never for the healthy state**: the generated suite stubs the
service so each probe answers "nothing found", and a fact named for the healthy state reads
false under that stub and turns a correct spec red the day it is written.

**Notifications** for everything §7 names that the framework does not already own, each with
its seven translations. Several already exist in this service and are **reused, not
duplicated** — the §7 table says which by name.

## What to read before writing — routed sections at the pin

`rules-dsl` · `old-state` (the previous-state snapshot is nil on insert — guard it) ·
`status-mapping` · `value-objects` (the automatic pass walks the struct, not the schema, and
the per-mode escape hatch) · `service-layout` for naming and granularity.
Convention: `conventions/domain.md`.

## Acceptance

- Every rule in §7 has a clause and a distinct notification; every notification has all
  seven translations.
- The declared mode set matches §5 and agrees with what the storage declaration says about
  archiving — a disagreement aborts the boot rather than failing at a write.
- No wire or storage tag appears anywhere in this layer.
- The escalation pair refuses a grant carrying a permission the caller does not hold, and
  refuses a wildcard-bearing role outright.
