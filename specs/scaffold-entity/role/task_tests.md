# task: tests — Role

Model: all of `spec.md`. Convention: `conventions/tests.md`.

**Floor: 95% per generated file** — `CLAUDE.md` rule 6, which is stricter than the skill's
80% and is what governs here. Measured per FILE from the cover profile, with the whole
internal tree as the coverage target — without that flag a mapper exercised from another
package's test reads as 0% and a whole package reads as untested when it is not.

**No production change to enable testability without the maintainer's approval** (rule 6).
Test files may cross layers only where production imports already allow it.

## What to cover

**Every rule branch of §7, both ways** — the violation and the pass. The ones that are easy
to leave half-covered, listed so they are not:

- the two immutability rules: the snapshot is nil on insert, so the guard's absent-old branch
  is a real case and needs its own test;
- the uniqueness pre-check on update, where the row must be excluded from colliding with
  itself;
- the tenant-isolation rule in all five of its states: matching tenant, mismatched tenant,
  superadmin bypass, **claim empty while an identity IS present (must refuse)**, and **no
  identity at all (must stand down — the dev profile)**. The last two are the pair §7's
  two-state table exists for, and testing only one of them proves nothing;
- the wildcard refusal and the no-escalation check as an ORDERED pair — including the test
  that proves a wildcard grant never reaches the identity helper, because that is the
  difference between a 403 and a panicked 500;
- the per-role cap, at the boundary and one past it;
- the duplicate-grant guard on both paths, the collection one and the by-id one;
- the child not-found path, asserting the 404-mapped notification and not the 422 one.

**The new value object**, exhaustively: length bounds at both ends, the shape pattern, the
anti-junk predicates, and the fact that it refuses rather than normalizes.

**The child aggregate value object**: its business-identity method, including that a
cosmetically different but identically-referenced child is the same child.

**Every command mapper** — the full-body one, the partial one, the result projection, and
the two child ones. The identity translation is mapper logic and is tested here: each of the
four identity states above enters through a mapper.

**Both queries' criteria hooks**, especially the tenant filter's presence, its absence for a
superadmin, its fail-closed behaviour on an empty claim, and its stand-down when there is no
identity at all.

**The seven catalogs**: every notification key introduced by §7 resolves in all seven, and no
catalog carries a key the others lack.

## What a unit test cannot reach

The repository, the service implementation and the routes need a live engine. Report their
coverage honestly rather than padding it, and say so in the verify table — the existing
entities' task files set that precedent. End-to-end proof of the seven endpoints belongs to
the QA skill and is not claimed here.

## Acceptance

- The per-file coverage lines for every generated file appear in the verify report. A bare
  package percentage means this level did not run.
- Anything under the floor is either fixed with more tests or surfaced as an explicit
  deviation for the maintainer to accept — never quietly averaged away.
- No test was edited to make production pass.
