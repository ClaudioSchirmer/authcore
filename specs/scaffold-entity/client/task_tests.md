# task_tests.md — tests

Model authority: [`spec.md`](spec.md) §7, §E, and `../../../CLAUDE.md` rule 6.

**The bar is 95 %, not the skill's 80 %** — the repository's own rule governs, and it is
measured **per generated file** from the coverage profile, collected across the whole
internal tree rather than per package. Without that, a mapper exercised from another
package's test reads as zero and a whole test-less package reads as untested when it is not.
A bare package percentage is not this level running.

## What this layer must cover

**Every rule branch of §7**, each asserted on its own notification and not merely on the
write failing — a rule that fires the wrong notification passes a test that only checks for
failure.

**Every command mapper** — build, apply fully, apply partially, read back — and the query
criteria builder, including the tenant scope injection.

**The two value objects**: the status enum's membership, and the network range's parse,
normalisation and both universal refusals.

**The hashing adapter to 100 %.** It needs no database and no fixture, and it is the one
piece here where a defect is a credential defect.

**The three checks §E names**, which are not ordinary unit tests:

1. the minted secret appears in exactly one response and in no other copy of the row;
2. no direct equality comparison exists over either hash;
3. a zero grace window clears both retiring columns rather than stamping a past deadline.

**Never edit a test to pass.** The test is the oracle.

## What to read before writing

Convention: `conventions/tests.md`.

## Acceptance

- Per-file coverage lines for every file this run produced appear in the verify report, and
  each is at or above 95 % — or is surfaced as an explicit deviation for the maintainer to
  accept.
- The existing suite still passes.
