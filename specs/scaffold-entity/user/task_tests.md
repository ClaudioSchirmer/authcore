# task_tests.md — tests

Model authority: [`spec.md`](spec.md) §7 and §D's closing note. Convention:
`conventions/tests.md`. The repository's own floor is **95%**, and §D already names in
advance the seam that cannot meet it.

## How coverage is measured, and why the flag is not decoration

Measure **per generated file**, from the cover profile, **across the whole internal tree** —
not per package. Without that, a file is credited only to tests in its own package, so a
mapper exercised from another package reads as zero and a whole test-less package reads as
untested when it is not. A run that reports zero for a file you know is exercised is
measuring wrong, not finding a gap. The per-file lines must appear in the verify report; a
bare package number means this level did not run.

## What must be covered

**Every rule branch of §7**, both directions — the refusal and the pass — including:

- both barriers, and specifically that they **end the pass**: the credential barrier's test
  is a request that fails the credential **and** carries an invalid new password, asserting
  **exactly one** notification comes back. A second one is the enumeration oracle, and it
  passes every happy-path test ever written;
- the conditional owner source, all of its branches: the scope-crossing caller supplying a
  tenant, the ordinary caller inheriting one, the ordinary caller supplying a **divergent**
  one, the caller with no tenant claim, and the caller with no identity at all;
- the status machine, every legal transition and every refused one, plus the no-ops;
- the archive mutation, asserted **on the entity**, not on the audit event;
- the escalation pairs at both depths, and specifically the **ordering**: an unknown
  reference must be reported as absent, not as an escalation attempt;
- the two caps, asserting the message carries its bound;
- the two immutability rules;
- the lockout counter and the lock, including that the counter **survives a rejected
  request** — a write that rolls back on refusal leaves it at zero and the lockout silently
  never engages, while every happy-path test stays green.

**The value objects**, each in its own tests: the composite's two parts and its rendering
method, including the short-half cases the shared anti-junk floor would have rejected; the
address rule, including the case rule; the password policy at every boundary of its length
and every missing class; and the enum's membership.

**The mappers** — the full and partial update paths, the entity construction, the result
construction — and the queries' criteria mapping, including the scope injection and its
skip for a scope-crossing caller.

**The hasher adapter to 100%.** It needs no engine and no app, and a hashing bug that a test
would have caught is not a bug anyone notices until it is a breach. Cover: the round trip,
a wrong password, a malformed stored value, the parameter-carrying encoding, and the
constant-cost path on a missing address.

## What CANNOT be covered here, and must be said rather than padded

The repository, the service facts that issue queries, and the route-mount functions need a
live relational engine and a running app. That is the contract suite's territory, it is
repo-wide rather than specific to this entity, and it is what the repository's 95% floor
collides with. Report the number honestly, split into "unit-testable" and "engine-bound",
and hand the deviation to the maintainer rather than inflating the denominator.

**Five of this entity's acceptance checks are also outside a unit test** and must be named as
belonging to the contract suite instead of marked green:

1. the masked value actually appearing in the trail, on both an insert snapshot and a change
   delta;
2. the constant-cost timing of a missing address — a wall-clock assertion;
3. the lock answering the **same** generic message as a wrong password, byte for byte;
4. the public route reaching its own handler rather than being swallowed by the by-id route;
5. the masked value being absent from all four read paths, checked separately rather than
   inferred from the flag.

## Acceptance check

- Every file this entity introduces is at or above the floor, or its shortfall is an
  explicit, stated deviation.
- The per-file coverage lines are in the report.
- No test was edited to pass. The test is the oracle.
