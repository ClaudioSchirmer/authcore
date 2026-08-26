# task_credential.md — the credential delta

**Not a layer.** This is the delta the domain, application, web, infra and migrations layers
each read before they run, because the password crosses all five and no single layer owns it.
Read it together with [`spec.md`](spec.md) §2 (the password end to end), §7 (U5–U7i), §E (the
open endpoint) and §D (what the generator cannot write).

**Read this delta BEFORE any of the five layers.** It is the part of the entity where a
mistake is a credential leak rather than a defect, and three of its requirements look like
polish until you know why they are there.

## Model decisions this delta carries

- The plaintext exists **in memory, for the length of one request**, and is never written
  anywhere. The stored value is an irreversible hash, PHC-encoded so its parameters travel
  with it.
- **Argon2id at the OWASP baseline** — 19 MiB of memory, 2 iterations, 1 degree of
  parallelism, roughly 100 ms on a modern server core. **No pepper**, decided at the gate.
- The policy is **8 to 128 runes and all four character classes**, Unicode-aware, plus a
  context rule refusing a password that echoes the address or either half of the name.
  Decided knowingly against the current NIST guidance; §B Q3 holds both sides.
- The write side carries **three values that have no column**: the plaintext, its
  confirmation, and — on the change operation only — the current password.
- **Two operations, and they are not the same operation twice.** One is public, identified
  by e-mail, verifies the current password, and clears the must-change flag. The other
  requires a token, is identified by the row id, does not verify anything, and sets the
  must-change flag.
- **Lockout is in**: a consecutive-failure counter and a lock expiry, five failures buying
  fifteen minutes, both invisible on every surface.

## What to read before writing — routed sections at the pin

| For | Read |
|---|---|
| how a value object validates itself, and how the automatic pass is switched off per mode | `value-objects` — the automatic-pass section and its two adjustment methods |
| keeping a persisted value out of the framework's own copies of the row, and the two mandatory axes | `table-schema` → the redaction section · `audit` → the redacted-fields section |
| what the audit event actually contains, and where it is routed | `audit` — the body discriminator, the two destinations, the actor stamping |
| a server-filled field that no write request carries | `table-schema` · the generator's own key documentation, if that path is chosen |
| an operation the auto handlers do not cover | `custom-command-handler` — and `handler-invariance` for what a custom handler still owes |
| a route that bypasses authentication, and what an absent identity means downstream | `auth-middleware` — the public-route bypass and the anonymous-actor behaviour · `yaml-reference` for the exact declaration shape |
| where the framework's responsibility for credentials ends | `token-issuance` — the section naming what stays in the consuming service |
| a barrier rule that ends the validation pass | `rules-dsl` |

Convention: `conventions/domain.md`, `conventions/application.md`, `conventions/web.md`,
`conventions/infra.md` — each for its own half of this delta.

## What each layer owes this delta

**Domain.** The three columnless values are ordinary exported fields of the aggregate, so the
framework's automatic value-object pass reaches the plaintext and enforces the policy on
every write that carries one — and the pass must be **switched off, per mode**, on every
write that does not, or a plain rename answers "password is required". The policy itself
lives in a value object, not in the aggregate: one type, one rule, three entry points that
cannot drift apart. The credential barrier is a **guard**: it ends the pass, so a failed
credential produces exactly one answer and nothing else can be appended to it.

**Application.** The hash is produced here, never in the domain — a port turns a plaintext
into a hash, and the domain never learns which algorithm is in use. That is what makes the
algorithm a one-line change later. The two operations are custom commands; neither is a
shape the auto handlers cover.

**Infra.** The port's adapter owns Argon2id and the PHC encoding, and it owns one more thing
that looks like waste and is not: **on an unknown address it must still spend the same
verification cost**, against a fixed hash generated at boot. Without it the generic answer
leaks by timing — a miss returns in about a millisecond and a hit in about a hundred, which
enumerates users exactly as well as a distinct message would. The adapter must also never log
its input, on any path including the error path; it is the last place a plaintext could reach
a log line.

**Web.** The public route is declared in the boot configuration of **both** profiles, and the
production one is the one that gets forgotten. It cannot carry a path parameter — the bypass
matches an exact method and path with no globs — which is why the operation is identified by
e-mail. It also shares a shape with the by-id patch route, so registration order is
load-bearing and is an acceptance check rather than a comment.

**Migrations.** The hash column is sized for a PHC string with room for raised parameters,
not for today's exact output. The two lockout columns carry defaults that make an existing
row valid without a backfill.

## Acceptance check

**Five copies, five mechanisms — all five verified, none inferred from a flag:**

1. the write request and the OpenAPI request schema carry no hash field at all;
2. no response body carries it — the by-id read, every listing row, the write results and
   the export renderer, checked separately;
3. the audit event carries the key **masked and present**, never absent, on both an insert
   snapshot and a change delta;
4. the sync payload carries the same mask — inert today, and the axis is mandatory anyway;
5. no filter and no ordering token mentions it, on any surface.

**And the four that are not about the hash:**

6. a request with an unregistered address **and** an invalid new password returns **exactly
   one** notification. A second one is the enumeration oracle, and it passes every
   happy-path test ever written;
7. the unknown-address path costs the same wall-clock as the known one;
8. five failures produce a lock, the lock answers the **same** generic message, and the
   counter survives the rejected request — a write that rolls back on refusal would leave
   it at zero and the lockout would silently never engage;
9. the public route reaches its own handler. If it answers 401, the by-id route swallowed it
   and the auth middleware got there first — that looks like a permissions bug and is a
   routing bug.

Items 3, 6, 7, 8 and 9 need a live engine or a running app, so they belong to the contract
suite rather than to a unit test. Say so in the report rather than marking them green.
