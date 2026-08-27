# tasks.md — Client

**Model authority: [`spec.md`](spec.md), `Status: APPROVED` (2026-08-26).** Nothing here
re-decides the model. Where this file and the spec disagree, the spec wins; where a task
file's mechanical detail contradicts a routed `/docs` section or a layer convention, the
**doc/convention wins** — apply it and record the deviation in the table at the bottom.

- **Pin:** omnicore `v0.61.0` · `omnicore-gen` from plugin 0.44.0 · **Dialect:** postgres
  (only) · **Posture:** Postgres SoR, no Mongo, no broker → relational-served views ·
  **Surfaces:** REST + OpenAPI + GraphQL
- **Branch:** `feature/client-credentials`, cut from `main` after the authentication-token
  merge landed. Not stacked — `Client` needs `tenants` and `roles` to exist, and they do.
- **Generation:** `omnicore-gen` — chosen at gate 1d, 2026-08-26. The ten task files below
  are the REVIEW CHECKLIST for the emitted tree: they say what each layer must contain,
  which is exactly what the generated code is read against.

## Layer order and status

| # | Layer | Task file | Status |
|---|---|---|---|
| 0a | children delta — read WITH domain, application, web, infra, migrations | [`task_children.md`](task_children.md) | **done** |
| 0b | **credential delta** — read WITH the same five. The part of this entity with no counterpart anywhere else | [`task_credential.md`](task_credential.md) | **done** |
| 1 | domain | [`task_domain.md`](task_domain.md) | **done** |
| 2 | application | [`task_application.md`](task_application.md) | **done** |
| 3 | web | [`task_web.md`](task_web.md) | **done** |
| 4 | infra | [`task_infra.md`](task_infra.md) | **done** |
| 5 | migrations | [`task_migrations.md`](task_migrations.md) | **done** |
| 6 | bootstrap | [`task_bootstrap.md`](task_bootstrap.md) | **done** |
| 7 | tests | [`task_tests.md`](task_tests.md) | **done** |
| 8 | docs | [`task_docs.md`](task_docs.md) | **done** — README `### Client`, the API-shape paragraph, and two backlog entries |

Layers 0a and 0b are not steps of their own — they are the deltas every layer they touch
reads before it runs. They are listed first because the model's worst traps live in them.

## The six things about this entity that are NOT `User`

Carried here from the spec so no layer has to rediscover them.

1. **The credential is hashed with SHA-256, not Argon2id, behind a NEW port.** This is the
   one place the entity deliberately refuses to inherit from `User`, and the reasoning is
   spec §B-Q4. A layer that reaches for the existing password adapter has taken a decision
   the gate already took the other way.
2. **The secret is minted by the server and revealed exactly ONCE**, in the response of the
   operation that minted it. Every other copy of the row — response, listing, audit event,
   outbox payload, query vocabulary — must not contain it. Five mechanisms, five separate
   places to get it wrong, and getting one wrong is a credential leak rather than a bug.
   `task_credential.md`.
3. **The row id IS the client id.** There is no second identifier. Nothing anywhere mints,
   stores or validates one.
4. **Two collections, and they are not alike.** One holds grants and carries the escalation
   rules; the other holds network ranges and carries a normalising value object. Four child
   operations, therefore four places to mis-wire the root-archive handler.
5. **Rotation is an overlap, not a swap.** Two nullable columns hold the retiring credential
   and its deadline, and a grace window of zero is a distinct, legitimate case that must
   clear both rather than stamp a past timestamp. `task_credential.md`.
6. **One rule is inert on the day it is written** — the client-writes-only-its-own-row rule
   reads a claim nothing mints yet. That is deliberate (spec §B-Q8e) and must not be
   "fixed" by removing it or by inventing a fallback that guesses the subject kind.

## Acceptance, service-wide

`../../../CLAUDE.md` rule 6 governs: **95 % coverage minimum**, not the skill's 80 %.
Spec §E adds three entity-specific checks on top of the standard verify gate.

## Deviations

| # | Layer | What the plan said | What was done, and why |
|---|---|---|---|
| 1 | infra / domain | spec §D: a `domain.SecretHasher` port with its adapter | **No port at all.** The adapter is concrete in `internal/infra`, and the domain reaches it through `ClientService.HashSecret`, which already IS the port. The interface would have existed only to give the file a name reachable from another layer — the maintainer's rule, restated at the gate on 2026-08-26: `internal/domain` is not an import-convenience address |
| 2 | domain | *(not in the plan)* | **`internal/domain/password_hasher.go` was REMOVED** in the same pass, on the maintainer's explicit instruction: nothing in the domain ever called it either. `Argon2idHasher` keeps its reasoning and stays concrete in `infra`; the two call sites and one test now name the concrete type. This is a change to `User`'s tree, authorised in chat |
| 3 | domain | spec §2: the CIDR value object NORMALISES host bits away | **It REFUSES them instead, and names the canonical spelling in the payload.** The generated mapper converts the caller's string straight to the type, so there is no seat between the wire and the value in which a rewrite could happen. Refusing with `CIDRHasHostBitsSetNotification` — a notification the spec did not enumerate — makes the stored form canonical AND tells the caller what would have worked |
| 4 | application / web | spec §9: two computed read fields, `ipRestricted` and `secretRotationPending` | **Neither exists.** `ipRestricted` is REFUSED by the generator: `read.computed.from` may not name a collection's field, because the derivation runs once per document and a collection is a slice. The state is still visible — an empty `allowedCIDRs: []` is served on the by-id read and on every listing row. `secretRotationPending` was DROPPED as redundant once `previousSecretExpiresAt` was served directly, which is strictly more informative than a boolean derived from it |
| 5 | web | spec §9: `previousSecretExpiresAt` appears in no `?fields=` vocabulary | **It does appear there.** Keeping it out needed `hidden: true`, which would also have removed it from every response — and an operator asking "until when does the old secret work?" should not have to derive that. It stays out of every filter and every sort, as promised, and leaks nothing: it is a timestamp |
| 6 | domain | spec §7: three notifications the table did not enumerate | `InvalidGracePeriodNotification` (§7 C13 named it; the YAML had to declare it), `ClientMustBeActiveToRotateNotification` (state-conflict — a switched-off client must not be handed a fresh credential) and `CIDRHasHostBitsSetNotification` (deviation 3). All three in all seven catalogs |
| 7 | tests | `../../../CLAUDE.md` rule 6: 95% minimum | **`internal/domain/client_rules_manual.go` lands at 91.2%** — the two unreachable branches are named in the test file's own header: the `service == nil` guards (the GENERATED `BuildRules` asserts the service type unguarded and panics first, so nothing nil reaches them) and the random-source panic in `newClientSecret` (faking `crypto/rand` would test the fake). `user_rules_manual.go` sits at 91.5% for the same two shapes. **An explicit deviation, for the maintainer to accept or reject** |
| 8 | web | spec §9: `POST /clients` is a response that carries the secret | **It does not, and this is the one thing that is not finished.** See the open item at the bottom of this file |

## ⚠️ Open — the create does not hand back a secret

`POST /clients` mints a credential and stores its hash; **nothing renders the plaintext**,
so the client exists and nobody can sign in as it until `POST /clients/{id}/secret` is
called. Two calls where the spec promised one.

The cause was established by RUNNING the generator, not inferred: `omnicore-gen explain keys`
has no key in either direction that puts a runtime value INTO a response — every
response-shaping key subtracts. The emitted `InsertClientResult` confirms it: it carries
`SecretHash` (dropped from the response by `hidden`) and no `Secret` at all.

Three ways out, and the choice is the maintainer's:

1. **Leave it as two steps** — create, then issue. It is AWS's and Entra ID's shape, it
   touches no generated file, and it has a real security argument: provisioning automation
   can create clients without ever handling a credential.
2. **Report the gap upstream** and regenerate when the generator can express it. The same
   route `User`'s two gaps took, which were fixed at `omnicore-gen` 0.40.0 before the entity
   shipped.
3. **`omnicore-gen adopt`** the insert command and the insert request, and add the field by
   hand. It works today; the price is that those two files stop tracking the spec forever.
