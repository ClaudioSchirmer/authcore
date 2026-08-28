# task_docs — Claim

No framework docs to read: this layer writes the project's own prose. Model authority:
[`spec.md`](spec.md) in full.

Written **last**, against the built tree rather than against the plan — every existing
document in this project describes the code as it stands, and a row written from a spec
ahead of the build is how one of them went stale before.

## `README.md`

- **The status table** gains a row for the catalog, describing what actually shipped: the
  registry, tenant-owned, no emission path yet.
- **A `### Claim` section** in the domain-model part, in the shape the existing entity
  sections take: what the entity is for, its fields, its rules, its verbs. Three things
  belong in it that a reader cannot infer from the field list:
  1. **the two-level chain** and that this run built only level 2 (the default), because the
     level-1 edges live on the two principal aggregates;
  2. **why the registry is called `Claim` and not `Attribute`** — the row exists to be
     minted, so a separate internal name would be 1:1 with the claim name and postiche.
     The backlog argues this at length; the README needs the conclusion, not the survey;
  3. **the reserved prefix, and that it is caller-owned** — the name is typed with `x_`,
     stored with `x_`, minted with `x_`, and nothing anywhere prepends or strips it.
- **The API-shape paragraph** gains this entity's five operations.
- **The reserved-platform-tenant row** is worth a sentence: this entity was deliberately
  built **not** to depend on it (`spec.md` §0), so the count of blocked entities does not
  grow. Say so, because the obvious assumption from the backlog is the opposite.

## `ACCESS_MATRIX.md`

**This is a first-class deliverable of this run, not a footnote.** The document's promise is
in its own subtitle — *"Who reaches what, per endpoint. The code as it stands"* — so an
entity that ships endpoints and is missing from it makes the whole document wrong rather
than merely incomplete. It is written **against the built routes**, read from the code, not
from `spec.md` §10: the spec says what was approved, the matrix says what is mounted, and
the point of the document is that those are checked against each other.

**Three edits, and all three are required.**

**1. The header status line.** It enumerates what is mapped and what is pending. The new
entity joins the mapped list. Leaving it stale is the specific failure this document already
suffered once — the README carries a note about a row that read "not started" on the day the
route landed.

**2. A `## Claim` section**, placed in the document's existing order (it runs by dependency,
and this entity depends on the owning tenant), in the exact table shape every other section
uses — the same eight columns, with the values read from the mounted routes:

| What the column asks | What this entity answers |
|---|---|
| **Endpoint** / **GraphQL** | the five operations of `spec.md` §9, both spellings per row |
| **Permission** | `claim:insert` · `claim:update` · `claim:archive` · `claim:read` (twice) |
| **Admission** | `JWT + claim` on all five — the service-wide gate, no exceptions |
| **Tenant scope** | `guard foreign-tenant` on the three writes; `filter TenantID` on the two reads. **Two different mechanisms, and the column exists to keep them apart** — a write loads the row through the repository, which the read filter never touches |
| **Self** | `—` on all five. Nothing here compares the caller's subject against the target row; this entity has no counterpart to the credential routes |
| **`*:*` crosses** | `yes` on all five |
| **Rows reached** | `own tenant` on all five |

**3. The prose under that table**, which is where every existing section carries what a table
cannot. Four things belong in it, and the first two are the ones a reader will otherwise get
wrong by analogy with the sections above it:

- **Four verbs, not five, and it is not an oversight.** `Role`, `Group` and `User` each carry
  a `:grant` because each owns a collection whose contents change what a principal can DO —
  "may rename it" and "may change what it confers" are separately grantable. This entity owns
  no collection and confers nothing, so a fifth verb would gate nothing. Say this explicitly:
  the document's own `Role` and `Group` sections argue the `:grant` split at length, and a
  reader arriving here finds it missing.
- **There is no "What a claim may be granted" sub-section**, for the same reason — no
  escalation rules, no wildcard refusal, no in-catalog check, because nothing is granted.
  Its absence is meaningful and worth one sentence.
- **PATCH carries only what it can change.** The document currently records, against both
  `Role` and `Group`, that their patch bodies advertise fields the domain then refuses — an
  asymmetry it notes was closed on `Permission` by excluding the field from the partial body.
  **This entity is the first tenant-owned registry built with that closed from the start**:
  the owner, the name and the value type are not members of the patch body at all, so the
  three immutability rules guard the value without the API ever offering it. That is a fact
  about the service's consistency and it belongs in the matrix, not only in `spec.md` §8.
- **No unarchive and no `DELETE`**, on the root or anywhere — so a retired definition comes
  back only as a new row with a new id, and the listing's include-archived control is the
  only way to see one. Plus: a by-id read of another tenant's definition answers **404**, not
  403 — it does not exist for this caller, which leaks nothing about who else exists.

## `backlog.md`

The entry *Custom claims via a `Claim` catalog* stops being an open question and becomes a
decision with a pointer. It is **not deleted** — the file's own preamble says entries stay
until promoted or dropped with a reason, and this one is promoted. It must record:

- **promoted**, with the date and a pointer to `spec.md`;
- **the two answers the gate gave**: the prefix is `x_`, and it is caller-owned rather than
  service-owned — with the reason the second one carries, since it is the backlog's own
  argument against a postiche internal name applied to itself;
- **what is still open, and it is most of the entry**: the two edge collections, the emission
  merge into token issuance, the third-level question, removal semantics on the edge, the
  audit allowlist, and which verb sets a value on a principal. Those did not become easier;
  they became reachable.
- The older *Custom claims on `Group`* entry gets one line: this shape does not answer it,
  because a user reaches several groups and the collision this design removes by
  construction returns there.

## Acceptance

- Every claim these documents make is true of the built tree, checked against it rather than
  against the spec.
- **`ACCESS_MATRIX.md`: all three edits present.** The status line names the entity; the
  section carries five rows over the document's eight columns; the prose carries the four
  points above. Each of the five rows is verified against the mounted route it describes —
  the permission demanded, the scope mechanism, and the surface spelling — rather than copied
  from `spec.md` §10.
- No document says the platform's nine are seeded — they are not.
- No document promises a token carries a tenant-defined claim — no token does yet.
