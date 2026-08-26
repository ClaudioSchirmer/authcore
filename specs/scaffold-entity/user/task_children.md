# task_children.md — the two-collection delta

**Not a layer.** This is the delta the domain, application, web, infra and migrations layers
each read before they run. Read it with [`spec.md`](spec.md) §3, §7 (U8–U13) and §2 (the
three read joins).

`Group` carried one collection. This entity carries **two**, and everything below is either
that doubling or the one thing that genuinely gets deeper.

## Model decisions this delta carries

- Two owned collections: the groups a user belongs to, and the roles granted to them
  directly. Both are the inherited half and the direct half of the same question.
- Each entry stores **only the referenced id**. The counterpart's handle and label are read
  across the foreign key at load time, never persisted — an entry storing the handle would
  silently re-attach to a retired-and-recreated row, and archive is one-way on both targets.
- **A pair of operations per collection, not the usual trio.** Neither entry has an editable
  field: its single column *is* its identity, so "change this entry" would keep one row id
  while changing what it means, which an audit trail reads as one grant *becoming* another
  instead of as two events. Join and leave; grant and revoke.
- Soft removal only, on both. Never the purge verb — the row lingers with its removal stamp,
  and a purge verb that soft-removes is a lying contract.
- Both are commands **on the root**: load the root, let a domain method mutate the one entry,
  let the framework persist the diff.
- **Uniqueness per owner** on both, over active rows, with the index as the race backstop.
- **A cap per collection**, and the notification must carry its interpolated bound rather
  than an empty struct.

## What to read before writing — routed sections at the pin

| For | Read |
|---|---|
| how an aggregate persists its children, and what one write touches | `aggregate-persistence` · `lifecycle-map` |
| the per-entry operations and their field contract | `auto-handlers` |
| reaching another aggregate across a foreign key, and what a join on a COLLECTION can and cannot do | `read-joins` |
| what a relational read model serves for children, and what it refuses | `relational-view` |
| rules over the entries a write ADDED rather than over the stored set | `rules-dsl` · `old-state` |

Convention: `conventions/aggregate-children.md`, plus each layer's own file.

## The traps, and there are four

**1. The root-archive handler is instantiated at most ONCE per surface.** Wiring it to a
child route type-checks, boots, answers success — and archives the entire user. This entity
has **four** child routes to make that mistake on instead of two. A child operation mounts
its own command and follows the operation's own field contract.

**2. Business identity must be compared over the stored id explicitly.** Each entry carries
three fields, and two of them are read-only join values that are **blank on a freshly added
entry**. A comparison over all three answers "different" for an attachment that duplicates a
stored one, which is the duplicate guard failing open. Name the id field; do not let the
generic comparison pick the set.

**3. The rules judge what the write ADDS, never the stored set.** Re-judging stored entries
makes unrelated writes hostages of the past: a group archived after the user joined it would
make the user impossible to rename, and an operator who has since lost a permission could no
longer even remove the other memberships. An insert is unchanged — every entry is an added
one — and a removal asks nothing, because it adds nothing.

**4. The escalation walk runs at two depths, and the ORDER is load-bearing at both.**
Existence first, then the wildcard refusal, then the escalation question. The wildcard check
must answer "refuse" for an id it cannot resolve, so an unknown reference would otherwise be
reported as an escalation attempt — a 403 blaming the caller where the honest answer is a 422
saying the thing is not there. And the wildcard check must run before the escalation one
because the permission probe **panics** on any argument containing a wildcard.

The role walk is two hops. **The group walk is three** — a group carries roles, and each role
carries permissions — and it is the deepest reach in the service. It resolves in one read per
level because both target repositories already declare their own traversals; no second query
into the catalog and no id-to-handle resolution step anywhere.

## The by-id guard

An absent entry answers the canonical not-found notification, not the framework's
does-not-exist one — 404 rather than 422. That guard belongs in a domain method on the root,
not in a loop inside a command mapper.

## What the joins do NOT do

They render the counterpart; they do not make it addressable. A filter or an ordering token
over a field inside either collection is a typed 400, on this backing and on any other —
narrowing a root by a field of a one-to-many collection is a pushdown a single root query
cannot express. And they do **not** answer the availability rules: an added entry has no
joined value, so reading the handle off it yields an empty string, which the wildcard check
would read as "no wildcard". A security rule passing on a blank field is the worst possible
failure direction.

## Acceptance check

- Four child operations exist, two per collection, and none of them is an "update this
  entry".
- The removal operations use the archive verb, never the purge verb.
- The root-archive handler appears once per surface, and no child route mounts it.
- Two identical additions leave one entry, and the second answers the conflict status.
- An addition referencing another tenant's group or role is refused with the **same** message
  as one referencing something archived or absent — three questions, one answer, deliberately.
- A removal succeeds for a caller who could no longer perform the matching addition.
- Each cap refuses at its bound and the message carries the bound.
- A filter over a field inside either collection answers a typed 400, not a 500 and not an
  empty page.
