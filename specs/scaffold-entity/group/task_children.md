# Task 0 — the children delta (`group_roles`)

Read this **with** each of domain, application, web, infra and migrations — it is the part
of the model those five share, and it carries the trap that costs the most to find late.

## Docs to READ (mandatory, at the pin)

- `aggregate-persistence` — how a 1:N collection is loaded, diffed and persisted; what a
  child mutation on the root actually dispatches.
- `table-schema` — the child's declaration, the parent key, and the archive column.
- `auto-handlers` — which handler serves a child operation, and its field contract
  (strict full-body vs lenient).
- `rules-dsl` — under which mode and `actionName` an aggregate value object's own
  `BuildRules` fires.
- `relational-view` — confirm again that the served document carries the collection
  (spec §9 depends on it) and that a filter or sort **inside** the collection is a typed
  400, not a 500.

Convention: `conventions/aggregate-children.md`. Layout and naming: `service-layout.html`.

## Model decisions this delta carries

- The collection is **`group_roles`**, owned by the flat root `groups`, edit strategy **B**
  (targeted per-child operations). The entry is an aggregate value object with **one stored
  field**, the referenced role's id — no key, no name, no denormalized copy — plus the three
  fields the read join fills: `RoleKey`, `RoleName` and `ArchivedAt`. Those three are
  read-only, absent from the table schema, populated on every load and empty on an entry a
  write is attaching. Do not confuse them with a copy: a copy would break the re-attach
  invariant, a traversal cannot.
- **Two operations, not three.** ATTACH and DETACH. There is deliberately no "update this
  entry": its single column *is* its identity, so an edit would keep one row id while
  changing what it means, which an audit trail reads as one grant *becoming* another instead
  of as two events. Do not generate a third verb "for symmetry".
- **DETACH is a soft removal and must be spelled as an archive operation on the entry, never
  as a hard-delete verb.** The row survives with an archive stamp; a purge verb over a row
  that lingers is a lying contract.
- **No per-child unarchive**, by framework construction. Re-attaching is a fresh ATTACH with
  a fresh entry id.
- Business identity is over the role reference, written **explicitly** rather than delegated
  to the by-all-fields helper — it states which column carries identity and survives a
  second field being added later. The framework's ATTACH path reuses it as the duplicate
  guard (G8).
- Both operations are commands **on the root** (load root → a domain method mutates the one
  entry → the framework persists the diff) and both dispatch the update mode, so the
  insert-or-update rules of spec §7 cover them.
- The by-id guard for DETACH lives in a **domain method on the root**, not in a loop inside a
  command mapper: an absent entry must answer the canonical record-not-found (404), never the
  framework's entity-does-not-exist (422).

## ⚠️ The trap

**The root-archive auto handler is instantiated at most ONCE per surface.** Wiring it to the
DETACH operation type-checks, boots, and answers 200 — while archiving **the entire group**,
de-authorizing every member. It is the single worst failure this model can ship, it is
invisible to `go build`, and it is on the mechanical checklist of the final verify. The child
operations mount their **own** commands and follow the operation's own field contract.

## Acceptance check

- ATTACH and DETACH exist; no third collection verb exists; DETACH is an archive operation,
  not a purge.
- The root-archive auto handler appears at most once per surface, and not on a collection
  route.
- The explicit business-identity method exists over the role reference — and over that
  alone, never over a join field.
- The duplicate guard, the cap and the three attach-time rules all reach the collection
  under the update mode.
- `go build` and `go vet` clean.
