# task_docs.md — docs

Model authority: [`spec.md`](spec.md) §9, §10, §F.

## What this layer must contain

**A README section for the entity**, in the shape the four existing entities use: what it
is, its field table, its routes with their permissions, and the two or three behaviours that
are deliberate and worth knowing before somebody debugs them. For this entity those are:

- the secret is shown once and never again, and the row id is the client id;
- rotation overlaps rather than swaps, and a zero window is an immediate kill;
- an empty allow-list means any address, which is fail-open on purpose.

**The permission catalogue gains six entries**, one of them a verb no other resource has.

**The backlog gains what §C declined** — the hard expiry and the scope-down claims, each
with the reason it was declined rather than as a bare idea, so a future reader sees a
decision instead of an omission.

**The API-shape section records what does NOT exist yet** — the token route for this subject
kind, and the two prerequisites §F puts on it. The README already promises that route; this
run makes the entity it authenticates, and saying so is what keeps the promise honest.

## Acceptance

- Every route in §9 appears in the README with its permission.
- Nothing in the README claims the token route exists.
