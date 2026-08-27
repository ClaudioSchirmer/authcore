# task_credential.md — the secret (delta)

Model authority: [`spec.md`](spec.md) §2 (*"The secret, end to end"*), §B-Q2, §B-Q3, §B-Q4,
§7 (C7, C13), §9, §E. Read this BEFORE domain, application, web, infra and migrations.

**This is the part of the entity that has no counterpart anywhere else in the service.**
`User` holds a credential too, and almost everything about how this one is handled differs
from it on purpose.

## What this delta must contain

**A new domain port for hashing a machine secret**, with exactly two operations — produce a
hash, and report whether a candidate produced one — and no third. It is a NEW port and NOT
the existing password one: the algorithm is SHA-256, the stored form is fixed-length
lowercase hex, and there is no salt and no parameter set travelling with the value. Spec
§B-Q4 is the argument; a layer that reuses the password adapter has reversed a gate
decision.

**Its adapter**, which must compare in constant time. A direct equality over the hash string
is the one way to reintroduce a timing oracle here, and it is an acceptance check rather
than a comment.

**A minting function** producing the fixed prefix followed by 32 bytes from the
cryptographic random source, rendered base64url without padding. The prefix is the point of
the format (spec §B-Q2) and is a constant, not a parameter.

**The insert path**: the secret is minted and hashed **only after every other check has
passed**, and the timestamp is stamped in the same step. Deriving earlier and letting the
framework discard the entity would work today and would be one refactor away from writing a
hash for a secret the rules refused.

**The rotate operation**, dispatching the update mode and told apart from the ordinary
partial update by its action name — the same discriminator `User`'s two credential
operations use, and for the same reason: all three share the mode. It moves the current hash
to the retiring slot with a deadline, mints a new secret, and stamps the timestamp.

**The zero window is a distinct branch, not an edge case.** A grace period of zero clears
both retiring columns rather than stamping a deadline in the past — an immediate kill, which
is what a caller rotating a *leaked* secret is asking for. Omitted and zero must be
distinguishable at the request boundary.

## The five places the secret must not appear, and the mechanism for each

1. **No caller supplies it** — there is no request field, at insert or ever, and the
   published API document names none.
2. **No column holds the plaintext** — it is a runtime-only field on the entity: no storage
   declaration, no migration, no outbox payload, no audit event.
3. **Both hash columns are kept out of every response body** by the response-hiding
   declaration — current and retiring, not just the current one.
4. **Both hash columns are redacted on BOTH axes** — the sync copy and the audit copy. Both
   axes are mandatory; a missing one is a construction panic, and the panic is the friendly
   outcome.
5. **Neither hash nor the retiring deadline appears in any filter, sort or projection
   vocabulary.** This one is NOT automatic — redaction refuses nothing on the read side, so
   it is a decision this layer makes by not declaring them.

**The one place it does appear** is the response of the operation that minted it — the
insert and the rotate, and nowhere else. Spec §D establishes, by running the generator, that
no spec key puts a runtime value into a response, so this is hand-written on either
generation path.

## What to read before writing — routed sections at the pin

`table-schema` (the redaction family and the two mandatory axes) · `audit` (redaction runs
after the delta is computed) · `custom-command-handler` and `auto-handlers` (the action-name
discriminator) · `rules-dsl` · `value-objects` (why the secret is deliberately not one).

## Acceptance

- Create a client, capture the secret, then read it back by id, list it, project every field
  by name, and read the audit row: the value is in none of them. Same for the rotate.
- A grep finds no direct equality comparison over either hash.
- Rotating with a zero window leaves both retiring columns empty.
- Rotating with a window above the ceiling is refused with a translated notification, not
  clamped silently.
