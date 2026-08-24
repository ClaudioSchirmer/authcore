# Task 1 — domain

## Docs to READ (mandatory, at the pin)

- `rules-dsl` — the rule closures, their verb scopes, and how a notification is attached to
  a field. **Confirm the added-items primitive by name here** (spec §7 depends on judging
  only the entries a write ADDS, never the stored ones).
- `old-state` — the previous-state snapshot the immutability rules compare against, and the
  fact that it is nil on insert.
- `status-mapping` — which notification semantic produces which HTTP status, so 422 / 409 /
  403 land where spec §7 says.
- `value-objects` — raw VO shape, and the automatic validation of a VO-typed field on every
  write.
- `custom-command-handler` + `service-to-service` — the ctx-bound domain service seam, and
  how an implementation reaches another aggregate's repository.
- `shared/query-primitives.md` (the pinned plugin's copy) — which primitive answers each
  fact of the service port.

Convention: `conventions/domain.md`. Read `task_children.md` too.

## What this layer contains

**Types**

- The aggregate root `Group`: the owner tenant reference, the handle, the display name, the
  description, and the roles collection. Ids are the framework's id type; a domain field
  carries a label key and nothing else — **no wire tags, no db tags** (a stray one also
  corrupts the previous-state snapshot).
- Two **runtime-only** fields carrying the caller identity into the rules — the requesting
  tenant and the super-admin flag. They are **not declared in the table schema**, so nothing
  persists or scans them. Spec §7 names where they are populated (the command mapper, the
  only layer allowed to read the request context).
- The aggregate value object `GroupRole` in the aggregate-VO package: one stored field, the
  role reference, plus the three join-filled fields of spec §2 (`RoleKey`, `RoleName`,
  `ArchivedAt`), the framework's managed embed, **no id field of its own**, and the mandatory
  business-identity method written over the role reference alone. The join fields carry plain
  Go types — `string`, `string`, `*time.Time` — never `vos.RoleKey`. **The framework lets a
  rule read them; this model forbids it** (spec §7): the entries a rule judges are the ones a
  write is ATTACHING, and those carry `""` and `nil`, indistinguishable from a live role.
- A new raw value object for the group handle: 2–64 runes, one lowercase slug of letters,
  digits and single hyphens — never leading, trailing or doubled — plus the project's shared
  anti-junk predicates (distinct-rune count and the run-of-identical-runes guard) already in
  the value-object package. **No reserved list and no derivation** — it is a handle within
  one tenant, not a public key, so it must not reuse the tenant-workspace type.
- Its notification lives in the value-object package, beside the other VO notifications.
- The display name and the description **reuse** the existing shared value objects. Do not
  write a second copy of either.
- A domain method for each collection mutation, and the by-id guard for DETACH.

**Rules** — exactly spec §7, G1 through G10b. Two things that are easy to get wrong:

- **Do not declare presence, format or length beside a VO-typed field.** All three text
  fields are VO-backed and validated by type on every write; declaring `required` beside one
  makes the caller read the same complaint twice for one empty value.
- **G6, G10a and G10b judge only the entries the write ADDS.** Reading the whole collection
  makes unrelated writes hostages of the past — a role archived after it was attached would
  make the group impossible to rename, and a caller who has since lost a permission could no
  longer even detach the others. An insert is unchanged; a detach asks nothing.
- **G10b must run before G10a**, and must answer "refuse" for an unresolvable role reference
  too. It exists to keep every wildcard string away from the framework's caller-side
  permission check, which **panics** on one — a panic on a security rule is a 500.
- **The identity has two absent states, not one** (spec §7). No identity at all → G5 and
  G10a stand down. An identity present with an empty or insufficient claim → refuse, fail
  closed. Collapsing them makes the entity unusable in the dev profile; the two-state test is
  a named, tested case, not an incidental nil check.

**The service port** — the six facts of spec §7, every one **named for the problem, never
for the healthy state**: the generated suite stubs the service so each probe answers
"nothing found", and a fact spelled for the healthy state would read as "the thing is gone"
under that stub and turn a correct spec red on the day it is written.

**Notifications** — the nine this entity declares (spec §7), each with its semantic and its
seven catalog texts. `TenantMismatchNotification` and `TenantMissingNotification` are
**framework-owned and already translated** — declare neither.

**Modes** — display, insert, update, archive. No unarchive, no delete.

## Acceptance check

- Every rule of spec §7 exists with the scope and notification the table names; nothing extra.
- No presence/format rule sits beside a VO-typed field.
- No wire or db tag anywhere in the domain.
- The three attach-time rules read added entries, and G10b precedes G10a.
- The two runtime-only identity fields exist and are absent from the schema declaration.
- `gofmt -l` prints nothing; `go vet` and `go build` clean.
