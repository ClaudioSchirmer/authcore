# task_docs.md — docs

Model authority: [`spec.md`](spec.md). This layer changes the repository's own prose, not the
service.

## What must change in the README

**The state table.** Four rows move and two are new: the user entity itself, the
user-to-tenant association, and the two rows that describe what is enforced in production —
the permission literals this entity adds, including the two nobody guesses from the pattern
(the grant verb and the reset verb).

**The domain-model section** gains a `User` entry beside `Tenant`, `Permission`, `Role` and
`Group`, following their shape: the field table, then the handful of things a reader will
otherwise get wrong. For this entity those are:

1. the password never comes back — and the four separate mechanisms that make that true,
   because a reader who knows only one of them will remove another;
2. the name is one value across two columns, with the rendered form derived on read;
3. the owner is conditional, and **this is where the entity diverges from `Role` and
   `Group`** — the README should say so, because the divergence is deliberate and the other
   two are candidates to be brought into line later;
4. the archive is one-way and releases the address, and reversible deactivation is the
   status field instead;
5. membership is edited here and **not** on the group, which is the opposite of what the big
   platforms do — with the reason, and with the cost (the reverse listing is not answerable
   on this posture).

**The API-shape section** gains this entity's eleven REST operations, and must show the two
credential ones apart from the rest, because one of them is public and that is the single
most surprising fact in the whole surface.

**A new short section on the public endpoint** — what it is, why it is public, why it is
identified by address rather than by id, and why every credential failure answers the same
message. A reader who finds it without that context will "fix" it.

**The "one e-mail, one user" section** gains one sentence: the address is now **immutable**,
which the section's own reasoning did not previously state either way.

## What must change in the backlog

The open entry about custom claims on groups depends on token issuance, which depends on this
entity. Note that its dependency is now half-satisfied — the subject exists; the minting path
still does not.

## What must NOT change

`spec.md` and the task files are the record of what was decided and what it was built from.
They stay in place, they are committed, and they are not summarized away into the README.

## Acceptance check

- No row of the state table claims more than what was actually built and verified.
- The permission literals are listed with the note that **nothing seeds them** — the catalog
  documents what the code enforces, it does not control it.
- Every deviation recorded in [`tasks.md`](tasks.md) is reflected, not quietly dropped.
