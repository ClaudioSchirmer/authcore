# Task 7 — tests

## Docs to READ (mandatory, at the pin)

- `rules-dsl` and `old-state` — what a rule sees on insert versus update, so the branches are
  exercised for the right reason.
- `status-mapping` — the status each notification semantic produces, so the assertions are
  about the contract and not about today's behavior.

Convention: `conventions/tests.md`.

## Coverage target

**This repository's floor is 95%, not the skill's 80%** (`CLAUDE.md` rule 6, which binds this
run). Measure **per generated file** from the cover profile, with the coverage scope set
across the internal tree — without that, a mapper exercised from another package's test reads
as 0% and a whole test-less package reads as untested when it is not. A file under the floor
is RED: add tests, or surface it as an explicit deviation for the maintainer to accept. **The
per-file coverage lines for every generated file must appear in the verify report** — a bare
package percentage means this level did not run.

**No production change to enable testability without approval** (`CLAUDE.md` rule 6). Test
files may cross DDD layers only where production imports already allow it.

## What must be covered

**Domain** — every branch of every rule of spec §7:

- G1/G2 immutability: the change refused on update, and **the insert path not tripping over
  it** (the previous-state snapshot is nil on insert — that is a real branch and a real bug
  if it is missed).
- G3 uniqueness: taken, free, and **exclude-self on update** (patching a group without
  changing its handle must not report its own row as a duplicate).
- G4: tenant unknown, tenant archived, tenant fine.
- **G5 the two identity states** — no identity at all → stands down; identity present with an
  empty or wrong tenant claim → refused; super-admin → crosses. These are named cases, not an
  incidental nil check.
- **G6 all three questions separately**: role absent, role archived, role belonging to
  another tenant — and **all three asserted to produce the same single notification** (the
  no-oracle decision of spec §7 is a test, not a comment).
- G8 duplicate, on both the attach path and the by-id path.
- G9 the cap: at the cap, and one over.
- **G10b before G10a** — a wildcard-bearing role must be refused by G10b, and the test must
  prove the caller-holds question was never reached. This is the panic interlock; if the
  ordering ever regresses, this is the only thing that catches it before a 500 in production.
- G10a: caller holds every permission of the role → passes; holds all but one → refused;
  super-admin → passes.
- **The added-entries semantics**: a rename of a group whose stored entry points at a
  since-archived role must **succeed**, and a detach must **not** re-judge the remaining
  entries. These two are the whole reason spec §7 reads added entries rather than the
  collection, and they are the regression that amendment exists to prevent.
- **The join fields never reach a verdict.** An entry the write is ATTACHING carries
  `RoleKey == ""`, `RoleName == ""` and `ArchivedAt == nil`; pin that, and pin that G6 still
  refuses an attach onto an archived role even though `ArchivedAt` reads `nil` on it. Both
  failures are silent, and the second is fail-open.
- **The escalation facts judge every key the role grants, archived rows included** (spec §7,
  fail-closed): a role whose bundle carries a retired `tenant:export` is refused to a caller
  who does not hold `tenant:export`, even though the grant's own `ArchivedAt` is set.

**Value object** — the new handle: the boundary lengths, the slug shape (leading, trailing
and doubled hyphen each refused), and the anti-junk predicates. Rune-based, not byte-based.

**Application** — the command mappers both ways, the partial-update mapper, and the query
criteria including the tenant filter and the super-admin bypass.

**Never edit a test to make it pass.** The test is the oracle.

## Acceptance check

- Every generated file at or above 95%, with the per-file lines in the report.
- The full suite green, and the pre-existing suite still green (regression).
- The named cases above all exist and assert the contract, not the implementation.
