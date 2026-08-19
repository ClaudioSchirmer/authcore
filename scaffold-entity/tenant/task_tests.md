# task — tests

Model authority: `spec.md` §7 above all — the rules table is the test plan. Target: **≥ 80%
per generated file**, measured per file, never as a package average.

## Read BEFORE generating (mandatory)

The tests convention, plus whichever `/docs` section owns the behavior under test when a
test needs to assert a framework contract rather than our own logic.

## What to cover

**The shared text predicates first, and hardest.** They are the most reused thing this run
produces and the easiest to get subtly wrong. `spec.md` §7 pins their semantics and requires
each to carry at least one accented-Latin case and one non-Latin case. Concretely, the tests
must fail if someone reimplements them over bytes: a name whose rune count and byte count
differ must be bounded by the rune count, a run of four identical accented characters must be
caught, and a description in a non-Latin script must pass the vowel predicate.

**Every value object**, both directions: the values the rule admits and the values it
refuses, one case per clause of the rule. The workspace type additionally needs its
derivation tested — same input, same output, every time — and its reserved list.

**Every branch of `BuildRules`**, one case per rule of `spec.md` §7 that has a mode-gate
scope. Rules 12 and 13 deserve more than one each: the transition rule has an allowed set
and a refused set and both matter, and rule 13's three recorded consequences are each a test.

**The command mappers** — the entity build, the full apply, the partial apply and the
result build. The insert mapper's derivation is the highest-value assertion in this layer:
it is the one place the public key is computed.

**The query criteria builder**, for each filter the spec declares.

## How to measure

With coverage across the internal tree, then read per file from the profile. Without that
flag, a file exercised only from another package's test reads as zero and a whole test-less
package reads as untested when it is not — a run that reports zero for a file known to be
exercised is measuring wrong, not finding a gap.

## Acceptance

- Every generated file at or above 80%, with the per-file lines shown in the verify report.
  A bare package number does not satisfy this, and a file below the target is either brought
  up or surfaced as an explicit deviation for the maintainer to accept.
- No test is edited to make production code pass. The test is the oracle.
