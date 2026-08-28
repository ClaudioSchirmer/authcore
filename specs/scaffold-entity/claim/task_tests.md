# task_tests — Claim

Model authority: [`spec.md`](spec.md) §7 above all — the rules are what the suite exists to
pin. Process: `conventions/tests.md`.

## The floor

**≥ 95% per generated file** — `../../../CLAUDE.md` rule 6, which is stricter than the
skill's 80% and wins. Measured **per file** from the cover profile with
`-coverpkg=./internal/...`, never as a bare package percentage: without the flag a mapper
exercised from another package's test reads as 0% and a whole test-less package reads as
untested when it is not. The per-file coverage lines appear in the verify report; a bare
number means this level did not run.

**No production change to enable testability without maintainer approval** (rule 6 again).
A file that cannot reach the floor without one is a deviation to surface, not a refactor to
perform.

## What must be covered

**Every branch of the aggregate's rules** — all eight of `spec.md` §7, each in both
directions, and the verb scoping of each (an update-only rule must be proven not to fire on
insert).

**The claim-name value object, exhaustively.** This is where the gate's two decisions live,
so this is where they are pinned:

- accepted: an ordinary prefixed name, returned **byte for byte** — the test that would catch
  a normalizing "convenience" prepend or trim;
- refused: an unprefixed name · a double-prefixed name · uppercase · leading or trailing
  space · one rune · 65 runes · a name whose remainder is empty · keyboard junk under the
  shared predicates · a name with a character outside the allowed set;
- the empty value answers the framework's required-field notification, not the shape one.

**The two enum value objects** — each member accepted, an out-of-set value landing on the
unknown sentinel and answering the unknown-member notification, and the sentinel absent from
the member list.

**The cross-field default rule (R7)**, per declared type: a default that parses, one that
does not, and **a null default, which is valid for every type**. That last case is the one a
suite written from the happy path forgets, and it is the difference between "no default" and
"an empty default" — which are not the same thing to a consumer.

**The command mappers** — to-entity, apply, apply-partially, from-entity — and the query's
criteria construction, including that a by-params read with no explicit owner filter still
produces a criteria carrying one.

**The tenant seam, both halves**: the read filter, and the write guard refusing a foreign
row. Plus the two-states rule — absent identity stands down, absent-or-insufficient claim
refuses. Collapsing them is the bug, so both are asserted.

## What is accepted as uncovered, and why

The repository, the service implementation and the routes need a **live engine**; their
pure predicates and identity paths are covered rather than waved through, and the remainder
is recorded as a measured deviation in `tasks.md` with its number rather than hidden behind
a package average. This is the same accounting the existing tenant-owned entity's build
recorded.

## Never

**Never edit a test to make it pass.** The test is the oracle. If the code and the test
disagree, one of them is wrong and the answer is found, not chosen.

## Acceptance

- Per-file coverage at or above the floor for every file this run generated, with the
  `go tool cover -func` lines in the report; anything below it named, measured and accepted
  explicitly.
- The existing suite still green — this entity touches no shared code, so a regression here
  means something was changed that should not have been.
