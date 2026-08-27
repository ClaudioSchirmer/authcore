# Spec: Client

- **Status:** APPROVED
- **Approved:** maintainer (Cláudio Schirmer Guedes), 2026-08-26 — **every ⚠️ OPEN slot and
  every high-risk `(proposed)` pick was answered at the model gate, over six rounds.** One
  answer arrived unprompted and replaced a decision this spec had deferred: the row rule in
  §B-Q8, which is better than what it replaced.
- **Language:** English (all artifacts) · Portuguese (chat) — per `../../../CLAUDE.md`
  rule 3 and the maintainer's invocation
- **Generation:** `omnicore-gen` — chosen by the maintainer at gate 1d, 2026-08-26. §D
  records what stays hand-written on this path: the reveal-once secret (no spec key puts a
  runtime value into a response — established by running `explain keys`), the SHA-256 port
  and its adapter, the CIDR value object, the rotate route and its action name, the
  cross-aggregate foreign keys in the migration, and the seven catalogs for each new
  notification
- **Pin:** omnicore **`v0.61.0`** (already the latest published — the 0v check found no
  update) · `omnicore-gen` from plugin 0.44.0 · dialect **postgres** · Postgres SoR, no
  Mongo, no broker → **relational-served views**. The same posture every existing entity
  was built under
- **Branch:** `feature/client-credentials`

- **AMENDED 2026-08-26, AFTER the build** — eight things came out different from what this
  spec promised, and every one is in `tasks.md`'s deviation table with its reason. Three of
  them change what is written below, so they are marked inline: the CIDR value object
  **refuses** a non-canonical range instead of normalising it (§2 — the generated mapper
  converts straight to the type, so there is no seat to rewrite in); **neither computed read
  field exists** (§9 — one is refused by the generator, one became redundant); and there is
  **no `SecretHasher` port** (§D — nothing in the domain calls it, so it would have been a
  name placed by import convenience). The maintainer then had `PasswordHasher` removed from
  the domain for the same reason, which is a change to `User`'s tree authorised in chat.
  **One promise is not met and is OPEN**: `POST /clients` does not hand back a secret — see
  `tasks.md`'s open item and its three ways out.

## ✅ Decisions taken at the model gate (2026-08-26)

| Slot | Answer | What it changed |
|---|---|---|
| **Q1** `clientId` | **it IS the row id** | no second UUID, no extra unique index. §B-Q1 |
| **Q2** Secret format | **`acs_` + 32 random bytes, base64url unpadded** (43 chars, 256 bits) | the prefix is the argument, not the entropy. §B-Q2 |
| **Q3** Rotation | **flat overlap — two nullable columns** | `previous_secret_hash` + `previous_secret_expires_at`. §B-Q3 |
| **Q3b** Grace window | **caller-supplied, server-enforced ceiling** | `gracePeriodSeconds`, 0…604800, default 86400 when omitted. Zero is legitimate |
| **Q4** Hash | **SHA-256 behind a new `domain.SecretHasher` port** — NOT Argon2id | the one place this entity deliberately does not copy `User`. §B-Q4 |
| **Q5** Grants | **`client_roles` only, no groups** | one hop on the token path. §B-Q5 |
| **Q6** Tenant | **`tenant_id` NOT NULL, immutable** | `User`'s answer verbatim. §B-Q6 |
| **Q7a** `lastAuthenticatedAt` | **out** | answered by `authentication_attempts` instead of by a write per token |
| **Q7b** `secretExpiresAt` | **not now** | §C-2 keeps the offer and the reason |
| **Q7c** `description` | **required**, `vos.Description` as-is | symmetry with `Role` and `Group` |
| **Q8** May a client manage clients? | ~~deferred to the token run~~ → **replaced by the row rule below**, at the maintainer's instruction | C14 is back in §7, and it says more than the rule it replaced. §B-Q8 |
| **Q8b** Row scope, all three cases | **tenant token → its own tenant · `*:*` → any row · client token → only its own row (`sub == id`)** | §10, Layer 2/3. A row decision, so it lives in `BuildRules` and not in a new route |
| **Q8c** Scope of the client rule | **only on the `Client` entity**, not service-wide | *"provavelmente será só por segurança, dificilmente um client terá permission client:update"* — defense in depth, expected never to fire. §B-Q8 |
| **Q8d** Telling the two token kinds apart | **an `identity_kind` claim** (`user` \| `client`) | reuses the vocabulary `authentication_attempts.identity_kind` already holds. No new word |
| **Q8e** Claim absent | **assume `user`** | the restriction narrows only when the token positively says `client`; the framework's own internal identities carry no claims |
| **Audit** | **`auditClaims` gains `name` and `identity_kind`** | and the client token mints `name` = the client's label, so one shared vocabulary serves both subject kinds. §F |
| **§C-3** IP allow-list | **IN — a second child collection** | `client_allowed_cidrs`, a new `vos.CIDRBlock`, its own routes and rules |
| **§C-3a** Empty list | **empty = any IP** | fail-open, and the listing must SAY so — see the `ipRestricted` computed field in §9 |
| **§C-3b** Granularity | **CIDR, IPv4 and IPv6** | `VARCHAR(43)`; containment evaluated in Go (`net/netip`), never in SQL |
| **§C-3c** Source IP behind a proxy | **a hard prerequisite recorded for the token run** | §F. This run builds the write side; the reader does not exist yet |

**No ⚠️ OPEN slot remains.**

---

## §A — The request, and the one premise that did not survive contact

The maintainer's words: *"hoje temos usuário, queria algo parecido, mas agora para
máquinas, vai ser usuário de sistema, tipo integração… pensei em chamar de client e o
clientId e clientSecret seriam as maneiras de logar, mas ambos gerados pelo sistema, ambos
uuid, e o secret obviamente usando o mesmo padrão de senhas do usuário."*

Four claims. Three are straightforwardly right and are taken as decided:

1. **The name is `Client`.** Not proposed here — already committed by the platform.
   `POST /auth/client/token` is named in `README.md` § *API shape*, in
   `specs/implement/authentication-token/plan.md` § *Seam and shape*, and the
   `authentication_attempts.identity_kind` column already documents `'client'` as its
   second value. The word is overloaded in Go (omnicore ships an `httpclient` package),
   which is a naming cost worth knowing about and not worth reopening.
2. **It is a machine account, shaped like `User`.** A tenant-owned flat root holding a
   credential and a set of grants. Everything structural below that is not about the
   secret is inherited from `../user/spec.md` and is not re-argued here.
3. **Both identifiers are minted by the server.** No caller ever proposes either one.

**The fourth — *"o secret obviamente usando o mesmo padrão de senhas do usuário"* — could
not be taken literally, and the reason is mechanical rather than stylistic.** Three
independent reasons, in the order that matters:

1. **A UUID would be refused by that very policy.** `vos.Password.IsValid` requires an
   UPPERCASE letter (`internal/domain/vos/password.go`, `passwordClasses`). The canonical
   UUID string is lowercase hex plus hyphens: lowercase ✓, digit ✓, symbol ✓ (the hyphen
   is `unicode.IsPunct`), **uppercase ✗**. A system-generated UUIDv4 secret fails
   `WeakPasswordNotification` every time. The two halves of the request contradicted each
   other.
2. **A password policy exists to compensate for LOW entropy.** Its whole job is to steer a
   human away from `senha`. There is no human here: the server picks the value, uniformly
   at random, and there is nobody to steer.
3. **Nothing validates a value the server generated.** `vos.Password` is a rule about
   *caller input*. The secret is never caller input — there is no request field for it,
   at insert or ever.

**The reading this spec is built on, and it was confirmed at the gate:** the secret follows
the password's **HANDLING** — never stored in clear, never returned after the one moment it
is minted, never in a response body, an audit event, an outbox payload or a log — and
**not** its **POLICY**. Q4 then took the handling half one step further and dropped
Argon2id too, for a reason that is specific to high-entropy input.

---

## §B — The reasoning behind each answer (kept; every one is RESOLVED)

### ✅ Q1 — `clientId` **is** the row id

The framework mints a **UUID v7** (`infra/db/command/write/write_sql.go:19`, `uuid.NewV7()`
— read at this pin, not assumed): 48 bits of millisecond timestamp + **74 random bits**
(RFC 9562).

- One identifier. `GET /clients/{id}` and `POST /auth/client/token` speak the same value;
  the token's `sub` is that value, exactly as a user token's `sub` is the user's row id.
- **Not guessable**, and that matters more than it looks: the lockout keys on the
  *attempted identity*, so a guessable client id would let anyone lock an integration out
  for 15 minutes on a loop. 74 bits is not walked.
- **The cost, on the record:** the id cannot be rotated without creating a new row. That is
  arguably correct — rotating a client's *identity* means re-provisioning every consumer
  anyway — but it is a door this answer closes. The creation timestamp leaking in the
  identifier is a second, small disclosure, already true of every other row in this service.

*Rejected: a separate `client_id` v4 column.* It buys identity rotation and pays with two
identifiers for one thing, forever — the kind of duplication that is free on day one and
expensive in year two.

### ✅ Q2 — `acs_` + 32 random bytes, base64url unpadded

**The prefix is the argument, not the entropy.** 122 bits (UUIDv4) and 256 bits are both
uncrackable; that half is a tie. What a prefix buys that a UUID cannot:

- **Secret scanning.** A leaked UUID in a git repository, a CI log or a Slack message is
  indistinguishable from the several other UUIDs in the same file. `acs_` followed by 43
  base64url characters is a *pattern* — GitHub secret scanning, `gitleaks`, `trufflehog`
  and every in-house scanner match on it. This is how GitHub (`ghp_`), Stripe (`sk_live_`)
  and Slack (`xoxb-`) all ship API credentials, and it is the cheapest thing in this spec.
- **authcore can refuse to log its own secrets.** A pattern is something a log scrubber can
  match. A UUID is not.
- **It tells the two values apart.** A support thread where somebody pasted "the UUID" is
  unresolvable when both the id and the secret are UUIDs.

### ✅ Q3 — Flat overlap: two nullable columns, one grace window

The row carries `secret_hash` + `secret_changed_at` and `previous_secret_hash` +
`previous_secret_expires_at` (both NULL when no rotation is in flight). Rotating moves the
current hash into `previous_*`, stamps an expiry on it, and mints a new one. The token path
tries `secret_hash`, then `previous_secret_hash` **only while unexpired**. The old secret
retires itself; nothing sweeps it.

*Rejected: hard cutover.* Every consumer breaks between the rotation and its own redeploy,
so rotation becomes a scheduled outage, so it is deferred, so secrets live for years — the
model produces the behaviour it exists to prevent.

*Rejected: a `client_secrets` child collection* (the AWS / Entra ID shape). Fully general,
and considerable machinery — child table, aggregate value object, per-child routes, a token
path that walks a collection — for a set that is never bigger than two.

**Q3b — the window is the caller's, under a ceiling.** `gracePeriodSeconds` in the rotate
body: **0 … 604800** (7 days), **86400** (24 h) when omitted. A fixed constant cannot serve
both real cases — a *leaked* secret wants the old one dead **now** (zero is legitimate and
must stay reachable), a large migration wants a week. **Omitted and zero must be
distinguishable**, so the request field is a pointer.

### ✅ Q4 — SHA-256 behind `domain.SecretHasher`, **not** Argon2id

The project hashes passwords with Argon2id at the OWASP baseline (m=19 MiB, t=2, p=1),
PHC-encoded — `internal/infra/password_hasher.go`, behind the `domain.PasswordHasher` port.
This entity deliberately does not inherit it.

**What memory-hardness buys.** Argon2id exists to make *offline* guessing expensive after a
database leak. That is worth ~19 MiB and ~100 ms per verify **because a human password has
perhaps 30 bits of entropy and is therefore guessable**. A secret drawn from 256 random bits
is not guessable at any cost per guess. Multiplying an impossible search by 10⁵ leaves it
impossible. **Nothing is bought.**

**What it costs, in the two places it is spent.**
- **The token path.** `POST /auth/client/token` is unauthenticated by construction. Every
  request — including every wrong one, and including the fixed-dummy verify the timing
  equalisation performs on a miss (`plan.md` § *Timing equalisation*) — would allocate
  19 MiB and burn ~100 ms. That is a memory-amplification factor an attacker gets for the
  price of a TCP connection.
- **Rotation doubles it.** Under the overlap the miss path verifies twice: current, then
  previous. ~200 ms and 38 MiB per failed attempt.

**No salt, and the absence is not laziness.** Salting defends against precomputation across
a *population of low-entropy inputs*; a rainbow table over 2²⁵⁶ random values does not exist
and cannot. This is how GitHub stores personal access tokens.

**`crypto/subtle` for the comparison, and that is an acceptance check, not a comment** — a
`==` over the hash string is the one way to reintroduce a timing oracle here.

The port is the point: two methods and no third (`Hash`, `Matches`), exactly
`PasswordHasher`'s shape, so the algorithm stays a configuration decision and no rule ever
learns which one answers.

*Recorded as rejected:* **HMAC-SHA-256 with a server-side pepper** is strictly stronger — a
stolen database alone becomes useless — and was declined only because it adds key
management this service has nowhere else, whose loss breaks every client at once.

### ✅ Q5 — `client_roles` only

- `backlog.md` defines a group as where *"the tenant expresses 'these people are alike'"*.
  A machine is not alike anybody.
- The token path resolves one hop instead of three — and that is the hottest security query
  in the platform.
- **The cost:** a client cannot inherit a bundle an admin already curates as a group. If
  that turns out to be wanted, `client_groups` is **purely additive** — a new child table, a
  child value object, an attach/detach route pair, all mirroring `user_groups` verbatim. No
  data migration, no reshaping of anything that exists.

### ✅ Q6 — One tenant, NOT NULL, immutable

Platform-level integrations live in the reserved platform tenant — the same answer
`README.md` § *Where the platform operators live* already gives for platform operators.
*Rejected: a nullable `tenant_id` meaning "platform-wide"*, which would introduce a second
row-scoping regime into every `ToCriteria` and every write rule in the service.

### ✅ Q7 — The field set

**In:** `name` (`vos.DisplayName`, reused — the project's VO **for things, not people**, as
its own file header states), `description` (`vos.Description`, reused, NOT NULL, min 15
runes — a client nobody can explain is a client nobody dares revoke), `status`
(**new** `vos.ClientStatus`), `secretChangedAt`.

**Out, and each is a decision:**

- **`key`** (a stable machine handle, the way `Role` and `Group` have one). The client id
  *is* the handle; a second one would need its own uniqueness rule and would compete with
  `name` for the same job. Decided here, not asked.
- **`lastAuthenticatedAt`** — it would mean **a write to the `clients` row on every token
  request**: a revision bump, an audit event and an outbox row, on the hottest path in the
  service, plus a new collision surface against a concurrent PATCH on the revision guard.
  The question it answers is already answered by `authentication_attempts`, which records
  every success with a timestamp and is append-only precisely so this does not cost a write.
- **`secretExpiresAt`** — §C-2 keeps the offer and the reason.

**`vos.ClientStatus` is new and does not reuse `vos.UserStatus`** (decided, low-risk): a
column on `clients` typed `UserStatus` reads wrong and couples two lifecycles that may
diverge. It is a verbatim structural copy — `active` · `suspended`, with
`ClientStatusUnknown` as the zero sentinel.

### ✅ Q8 — Row scope: a client token writes only its own row

**This started as a narrower question and the maintainer replaced it with a better rule**,
unprompted, mid-gate: *"jwt de tenant só altera o mesmo tenant, jwt superAdmin (`*:*`)
altera qualquer um, e se por um acaso for um token de client, só altera ele mesmo,
`sub == id`."*

The first two clauses are C1, already inherited from `User`. The third is new, and it
**subsumes** the question this slot originally asked (*"may a client mint more clients?"*):

- **Insert is refused, and it has its OWN rule** *(amended 2026-08-26, after the build)*.
  It first shipped as a consequence: a row being created has no id, so `sub == id` is false
  and the write was refused with no rule naming it. The maintainer then asked the question
  this slot had never asked out loud — *"um client pode criar outro client do mesmo tenant,
  tipo um user que pode criar outro user?"* — and the answer, **no**, is now C14a rather
  than arithmetic. Two things were wrong with the consequence: a refusal that falls out of
  another rule reads as an accident to whoever maintains it, and the caller was answered
  *"you may only modify your own record"* about a CREATION.

  **The asymmetry with `User` is deliberate, and the honest version of it is on the record.**
  A user holding `user:insert` creates an account whose password they chose, so the human
  path carries the SAME persistence mechanism and is knowingly left open — "machines must not
  breed" is therefore not a principle this service applies uniformly, it is a rule applied to
  one subject kind. What distinguishes them is attendance: a compromised machine credential
  mints replacements in a loop at three in the morning, while a person can be refused,
  suspended and asked what they were doing. Closing the `User` side too was offered at the
  gate and declined as its own piece of work — it needs an invite flow or a
  creator-does-not-see-the-password path, not a rule.

  **What already bounds the damage, so this is not the only lock:** C9 and C10 mean a client
  can never be granted more than whoever granted it, and `client:insert` is held by nobody.
  **Provenance was offered and declined**: a `createdBy` column would turn "show me
  everything this compromised credential created" into a filter, and `audit_events.Actor`
  already answers it by walking the trail — accepted, with the note that an aggressive
  retention policy is exactly what erases that trail first.
- **What remains is a client editing itself**, which is exactly what it should be able to
  do: rotate its own secret is a legitimate self-service operation for an integration.
- **Granting itself a role is bounded by C9**, which already refuses any role carrying a
  permission the caller does not hold. A client granting itself what it already has gains
  nothing.

**It applies to the `Client` entity only, not service-wide** — and the maintainer named it
correctly as belt-and-braces: *"provavelmente será só por segurança, dificilmente um client
terá permission `client:update`"*. The service-wide reading was weighed and rejected at the
gate for a concrete cost: it would leave a machine account unable to provision a user,
create a role or import anything — a credential that exists to do work and can do none.

**Q8d — how the two kinds are told apart: an `identity_kind` claim**, values `user` and
`client`. Not invented here: `authentication_attempts.identity_kind` already holds exactly
those two values and its column comment already documents `'client'` as the second one. Same
word in the table and in the token, snake_case like `tenant_id` and `must_change_password`.

*Rejected: inferring from the absence of `email`* — a security decision resting on a field
that exists for another reason, which changes behaviour the day the claim set shrinks for
header budget. *Rejected: a `client:` prefix on `sub`* — it breaks "sub is the row id" and
forces every consuming service to parse before comparing. *Rejected: a new `subject_type`
name* — two words for one distinction across the repository.

**Q8e — an absent claim means `user`.** The restriction is a narrowing applied only when the
token positively says `client`. That preserves the identities the framework builds itself
(the grpc posture, the integration registry) which set `Subject` and may carry no claims at
all. **The residual risk, on the record:** a client token minted before the claim existed
would get user semantics. No such token exists — both endpoints mint the claim from their
first line, and nothing consumes this service yet.

**The rule is written in THIS run and is inert until the token run.** It reads a claim
nothing mints yet, so it evaluates to "user" and changes nothing — which is precisely what
Q8e makes safe. Writing it now rather than later means it is already in place the moment
`POST /auth/client/token` starts minting the claim, instead of being a thing somebody has to
remember.

### ✅ §C-3 — The IP allow-list is IN

A second child collection, `client_allowed_cidrs`. Machine callers usually have stable
egress, which turns a leaked secret from a compromise into a failed attempt — genuinely
valuable for this population in a way it would not be for `User`.

- **Empty = any IP** (fail-open). The only usable default: fail-closed would break every
  newly created client on its first token, turning creation into two operations where the
  first hands out a secret that does not work — a step every tutorial, provisioning script
  and e2e test would have to remember, whose symptom is a generic 401. **The cost is that
  "unrestricted" and "not configured yet" are the same state**, so §9 makes it visible with
  the `ipRestricted` computed field rather than leaving a silent empty array.
- **CIDR, IPv4 and IPv6.** One form covers both — `203.0.113.5/32` is an exact host. Stored
  as `VARCHAR(43)`: the framework's persistable type set is **closed**
  (`string | int | int64 | float64 | bool | time | id`), so postgres's native `cidr` type is
  not reachable and containment is evaluated **in Go** (`net/netip`) on the token path,
  never in SQL. A `/32`-only model was rejected: a caller behind a NAT gateway pool, or with
  egress in two zones, would file a ticket on every infrastructure change of their own.
- **The source-IP problem belongs to the token run** (§F), and this is not a half-built
  state: write side and read side are already separate runs here.

---

## 1. Storage model                                    [high-risk — confirm]

- **Kind: flat** (proposed; alternative: sharedbase-role — rejected below).
- **Identity-smell test, run explicitly rather than skipped:** the smell is a *party*
  identity (a person, or an asset with a natural registry key) that could play a second
  role. A `Client` has no such identity — it is not a person, it has no document, no e-mail
  and no registry number, and there is no second role for "this integration" to also be.
  `name` is a label, not an identity: two clients may legitimately be called the same thing
  in two tenants, and no dedup key exists to derive a shared PK from. **Flat, with no
  question to ask.**
- **ER sketch** (postgres, as decided):

```
clients                                  -- A machine account: an integration that
  id                UUID PK              --   authenticates as itself, holding a
                                         --   server-minted credential and a set of role
                                         --   grants inside exactly one tenant. The row id
                                         --   IS the client id the caller signs in with.
  tenant_id         UUID NOT NULL  FK → tenants(id)   NO ACTION
  name              VARCHAR(120) NOT NULL
  description       VARCHAR(500) NOT NULL
  secret_hash       VARCHAR(64)  NOT NULL   -- SHA-256, lowercase hex
  secret_changed_at TIMESTAMPTZ  NOT NULL
  previous_secret_hash        VARCHAR(64)  NULL
  previous_secret_expires_at  TIMESTAMPTZ  NULL
  status            VARCHAR(16)  NOT NULL
  revision          BIGINT NOT NULL DEFAULT 0
  created_at / updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
  deleted_at        TIMESTAMPTZ NULL

  UNIQUE (tenant_id, name) WHERE deleted_at IS NULL     -- see §2, Unique
  INDEX  (tenant_id)                                    -- the isolation filter runs on every listing

client_roles                             -- The roles granted to this client. One row per
  id          UUID PK                    --   grant, holding nothing but the role's id — so
  client_id   UUID NOT NULL FK → clients(id) ON DELETE CASCADE
  role_id     UUID NOT NULL FK → roles(id)  NO ACTION   --   a retired-and-recreated role is
  deleted_at  TIMESTAMPTZ NULL                          --   never silently re-granted.
  created_at / updated_at

  UNIQUE (client_id, role_id) WHERE deleted_at IS NULL
  INDEX  (client_id)          -- every read of the aggregate loads the collection by it
  INDEX  (role_id)            -- the reverse walk; and the FK check on every role delete

client_allowed_cidrs                     -- The network ranges this client may authenticate
  id          UUID PK                    --   from. An EMPTY collection means any address:
  client_id   UUID NOT NULL FK → clients(id) ON DELETE CASCADE
  cidr        VARCHAR(43)  NOT NULL      --   the restriction is opt-in, and the listing says
  label       VARCHAR(120) NOT NULL      --   which of the two a client is in.
  deleted_at  TIMESTAMPTZ NULL
  created_at / updated_at

  UNIQUE (client_id, cidr) WHERE deleted_at IS NULL
  INDEX  (client_id)
```

  **Child table names are owner-prefixed** (`client_roles`, `client_allowed_cidrs` — never
  bare `roles`/`allowed_cidrs`) — `migrations.md`.

  **`secret_hash` is `VARCHAR(64)` and not `VARCHAR(255)`** — SHA-256 is 64 hex characters,
  fixed. The password column is 255 because a PHC string carries its own parameters; this
  one carries nothing, and sizing it for a format it does not use would invite somebody to
  put one there.

- If sharedbase-role: **`N/A — flat`** (no shared identity; see the smell test above).

## 2. Fields

**Root — persisted**

| Field | Go type | VO? | Nullable | Unique | Lives on | example: | Description |
|---|---|---|---|---|---|---|---|
| `TenantID` | `domain.ID` | plain | no | no | root | `018f2c7e-…` | The tenant this client belongs to. Immutable — a client never moves between tenants |
| `Name` | `vos.DisplayName` | **reuse** | no | **yes, per tenant** | root | `Billing integration` | The human label this integration is found by |
| `Description` | `vos.Description` | **reuse** | no | no | root | `Posts invoices from the billing system into the ledger.` | What this integration is for, in the tenant's own words |
| `SecretHash` | `string` | plain | no | no | root | *(never rendered)* | The SHA-256 of the current secret, lowercase hex. Never sent by a caller, never returned, never copied |
| `SecretChangedAt` | `time.Time` | plain | no | no | root | `2026-08-26T14:03:11Z` | When the credential was last minted |
| `PreviousSecretHash` | `*string` | plain | **yes** | no | root | *(never rendered)* | The hash of the secret being retired, accepted until the grace window closes. NULL when no rotation is in flight |
| `PreviousSecretExpiresAt` | `*time.Time` | plain | **yes** | no | root | `2026-08-27T14:03:11Z` | When the retiring secret stops being accepted |
| `Status` | `vos.ClientStatus` | **new-enum** | no | no | root | `active` | The account's state — active or suspended. Orthogonal to archiving |

**Both nullable fields live on the ROOT and that is a decision, not a default** — the
sibling alternative was weighed in §4 and loses.

**Root — never persisted, and returned exactly once**

| Field | Go type | VO? | Description |
|---|---|---|---|
| `Secret` | `string` | plain | The plaintext secret, minted by the rules and rendered in the response of the operation that minted it — and nowhere else, ever. No column, no outbox payload, no audit event, no listing, no by-id read |
| `GracePeriodSeconds` | `*int` | plain | How long the retiring secret stays valid, 0…604800. Carried by the rotate body only; NULL means the 24 h default. Read by the rules, stored by nobody |

**`Secret` is deliberately NOT a value object.** A VO's `IsValid` is a rule about caller
input, and this value is never caller input (§A). Giving it one would be a rule that can
never fire, over a value the server chose.

**Root — read across the foreign key into `tenants`** (mirrors `User` verbatim)

| Field | Go type | Filled by | Description |
|---|---|---|---|
| `TenantWorkspace` | `string` | read join on `tenant_id` | The owning tenant's immutable handle. Read-only, filled on every load |
| `TenantStatus` | `string` | read join on `tenant_id` | The owning tenant's commercial lifecycle — what C2/C3 refuse on |

**Root — fed from the caller's identity, persisted by nothing** (mirrors `User` verbatim)

`RequestingIdentityPresent` · `RequestingTenant` · `RequestingMayCrossScope` ·
`RequestingSubject` (`source: subject`) · `RequestingIdentityKind`
(`source: claim`, `claim: identity_kind`).

**`RequestingSubject` is read off `Identity.Subject` and NEVER off `Claims["sub"]`** — the
distinction is load-bearing rather than stylistic, and `user_credential_manual.go` already
records why: the framework builds identities that set `Subject` and carry no `sub` claim at
all, so a comparison against the raw map reads `""` for those callers and answers "not the
owner" to everyone.

**Child `ClientRole`** (`internal/domain/aggregatevos/client_role.go`)

| Field | Go type | VO? | Description |
|---|---|---|---|
| `RoleID` | `domain.ID` | plain | The role granted to this client — the id and not the key, so a retired-and-recreated role needs an explicit re-grant |
| `RoleKey` / `RoleName` | `string` | plain | Read across the FK into `roles`; filled on every load, not persisted here |

**Child `ClientAllowedCIDR`** (`internal/domain/aggregatevos/client_allowed_cidr.go`)

| Field | Go type | VO? | Description |
|---|---|---|---|
| `CIDR` | `vos.CIDRBlock` | **new-raw** | A network range this client may authenticate from. IPv4 or IPv6, stored normalised |
| `Label` | `vos.DisplayName` | **reuse** | What this range is, in the tenant's words — `NAT gateway, sa-east-1`. Required, because an unlabelled range is one nobody dares remove |

### Unique — one field, and the enforcement style

**`(tenant_id, name)` over ACTIVE rows only** (proposed; alternative: not unique at all).
Two integrations called "Billing" in one tenant is the operational failure this prevents:
somebody revokes the wrong one. Scoped per tenant and not globally — two tenants naming
their integration the same way are not in conflict.

**Style: domain pre-check + DB backstop** — the project's own established answer for every
unique field (`Tenant.workspace`, `Role.key`, `Group.key`, `User.email`). A
`ClientService.NameTaken(name, tenantID, selfID)` probe in `BuildRules` so the duplicate
reports **together with** the other validation errors, plus the partial unique index and the
repository `Constraints` binding as the race backstop → 409.
Notification: `ClientNameAlreadyExistsNotification`, all seven catalogs.

The two child collections carry their own `(parent, value)` partial unique indexes, bound
the same way — `client_roles (client_id, role_id)` and
`client_allowed_cidrs (client_id, cidr)`.

### The secret, end to end — the only thing here that is not `User`

Six properties, and each names the mechanism that delivers it:

1. **No caller ever supplies it.** There is no request field, at insert or at rotate. A
   body key named `secret` is simply not in the schema; the OpenAPI document does not
   mention one.
2. **It is minted in the rules**, from `crypto/rand`, and hashed before the write. Nothing
   derives a hash for a secret the rules refused — `applyNewCredential`'s ordering
   discipline in `user_credential_manual.go`, applied here.
3. **No column holds the plaintext.** It is `runtime: true` on the entity, so there is no
   `TableSchema` entry, no migration, no outbox payload and no audit event to redact.
4. **The HASH is `hidden` + `redact`ed on both axes.** `hidden` keeps it out of every
   response; `RedactedField` with `core.InSync(r)` **and** `core.InAudit(r)` — both
   mandatory, a missing one is a construction panic (`table-schema.html`) — keeps it out of
   the sync payload and the audit event. Same as `users.password_hash`, and for the same
   reason. **Both hash columns**, current and previous.
5. **It is in no filter and no `?fields=` vocabulary.** Not automatic: `table-schema.html`
   states that redaction refuses nothing on the read side, so this is a separate decision
   that has to be made by hand. §9 makes it.
6. **It appears in exactly one place: the response of the operation that minted it** — the
   insert, and the rotate. That is one hand-written response field (§D), and the acceptance
   check for it is that `GET /clients/{id}` immediately afterwards does not contain it.

### The new value objects

- **`vos.ClientStatus`** (enum): `active` · `suspended`, with `ClientStatusUnknown` as the
  zero sentinel. A verbatim structural copy of `vos.UserStatus`.
- **`vos.CIDRBlock`** (raw): parses with `netip.ParsePrefix`, refuses what does not parse,
  and ~~normalises host bits away~~ → **REFUSES a range whose host bits are set, naming the
  canonical spelling in the notification payload** *(amended after the build)*. The reason is
  mechanical: the generated mapper converts the caller's string straight to this type, so
  there is no seat between the wire and the value in which a rewrite could happen. The
  property the normalisation was for is preserved — the stored form is canonical, so the
  unique index tells the truth about duplicates — and a caller told "write 203.0.113.0/24"
  learns something a silent rewrite would have hidden. It raises
  `CIDRHasHostBitsSetNotification`, a third answer this section did not originally list. It refuses the
  universal prefixes `0.0.0.0/0` and `::/0` outright — they are exactly equivalent to an
  empty collection, and having two spellings for "no restriction" is how a reviewer comes to
  believe a client is restricted when it is not.
  *Considered and NOT taken: a minimum prefix length* (refuse anything shorter than /16).
  It would catch a `/2` typo, and it would also refuse a legitimately large cloud egress
  range — a floor that is wrong blocks real configuration, and the universal case is the one
  that actually matters. Recorded so the absence reads as a decision.

**No new raw VO for the secret** (see above). `name`, `description` and the CIDR label reuse
what exists.

## 3. Children (1:N)

| Child | Of whom | Edit strategy | Restorable alone? |
|---|---|---|---|
| `ClientRole` (`client_roles`) | flat root | **A** — complete insert of the whole aggregate + root-only PATCH + per-child grant/revoke by id | no |
| `ClientAllowedCIDR` (`client_allowed_cidrs`) | flat root | **A** — same shape | no |

Verbatim `user_roles`: `POST /clients/{id}/roles` grants,
`PATCH /clients/{id}/roles/{childId}/archive` revokes; and the same pair under
`/clients/{id}/allowed-cidrs`. **No replace-all PUT on either** — an omitted entry must
never silently revoke a grant, and on the allow-list it must never silently widen access.

**Verb truth:** both removals are soft, so both are `PATCH …/archive` and never `DELETE`
(`aggregate-children.md`).

**Caps** (low-risk, decided): 50 role grants, 20 CIDR entries.

## 4. Siblings (1:1)

**`N/A — no facet split off, and this IS the decision, not a skipped step.`**

The model has two optional fields (`previousSecretHash`, `previousSecretExpiresAt`), which
is what makes this question live. A `client_secret_rotations` satellite would hold them.

**It loses, on one hard constraint:** a sibling loads by its **own statement**, and the token
path has to read both hashes in the **one** query that already loads the client by id.
Splitting them off puts a second round-trip on the hottest security path in the service to
save two nullable columns on a table that will never have many rows. They stay on the root.

## 5. Modes                                             [required]

`Display` · `Insert` · `Update` · `Archive` (proposed).

**No `Unarchive`**, mirroring `User` and not `Tenant`. Archiving a client is the platform's
"this integration is gone" — and a credential that can be brought back from the dead is a
credential whose revocation nobody can trust. Reversible deactivation has a home and it is
`status: suspended`.

**No `Delete`** — see §6.

**The cost, named:** archiving the wrong integration is not undoable through this API. It
comes back as a new client with a new id and a new secret, and every consumer is
reconfigured. That is the same trade `User` made.

## 6. Delete semantics                                  [required]

**Soft only — archive, no hard delete.** `PATCH /clients/{id}/archive`. There is no `DELETE`
route on the root and none on either child.

An archived client's row is the evidence of what a credential was allowed to do while it
lived; purging it would delete the answer to the question an incident asks first.

## 7. Business rules                        [required]

Numbered `C…`. Rules marked *(inherited)* are `User`'s, verbatim, and are not re-argued.

| # | Field(s) | Rule | Verb scope | Notification | HTTP |
|---|---|---|---|---|---|
| **C0** | — | The barrier: value objects the later rules depend on are validated first, then `r.StopIfInvalid()` *(inherited — U0)* | InsertOrUpdate, Archive | — | — |
| **C1** | `TenantID` | A caller may not write into a tenant that is not theirs; `*:*` crosses the scope; stands down when no identity is present at all *(inherited — `refuseForeignTenant`)* | InsertOrUpdate, Archive | `TenantMismatchNotification` (framework) | 403 |
| **C1b** | `TenantID` | Filled from the caller's `tenant_id` claim; only the `*:*` caller may state it explicitly *(inherited — U1b, `assignedFrom: identity-claim` + `bypassMaySet`)* | Insert | `TenantMissingNotification` (framework) | 400 |
| **C2** | `TenantID` | The named tenant must exist and must not be archived | Insert | `ClientTenantDoesNotExistNotification` | 422 |
| **C3** | `TenantID` | Immutable after creation | Update | `ClientTenantIsImmutableNotification` | 422 |
| **C4** | `Name` | Unique per tenant over active rows — service pre-check, DB backstop | InsertOrUpdate | `ClientNameAlreadyExistsNotification` | 409 |
| **C5** | `Status` | Transitions: `active ⇄ suspended` and nothing else. Staying put is always allowed *(inherited)* | Update | `InvalidClientStatusTransitionNotification` | 422 |
| **C6** | `Status` | **Archiving forces `suspended`** *(inherited — U15, which mirrors `Tenant` rule 13)* | Archive | — | — |
| **C7** | `Secret` | Minted from `crypto/rand` and hashed **only after every other check has passed**; `SecretChangedAt` stamped in the same step | Insert | — | — |
| **C8** | `RoleID` | The granted role must exist, must not be archived, and must belong to **this client's** tenant *(inherited — U8/U13b)* | InsertOrUpdate | `RoleNotAvailableInTenantNotification` (exists, reused) | 422 |
| **C9** | `RoleID` | **No escalation:** a caller may not grant a role carrying a permission the caller does not hold *(inherited — U13a)* | InsertOrUpdate | `CannotGrantRoleWithUnheldPermissionsNotification` (exists, reused) | 403 |
| **C10** | `RoleID` | **No wildcard:** a role carrying `*:*` cannot be granted at all *(inherited)* | InsertOrUpdate | `CannotGrantWildcardRoleNotification` (exists, reused) | 403 |
| **C11** | `RoleID` | At most **50** role grants on one client *(inherited cap)* | InsertOrUpdate | `TooManyRolesForClientNotification` | 422 |
| **C12** | `RoleID` | A duplicate grant is refused by business identity | (child add) | `ClientAlreadyGrantsRoleNotification` | 409 |
| **C13** | `Secret`, `GracePeriodSeconds` | **Rotate:** the row must not be archived and `status` must be `active`; the window must be 0…604800 (24 h when omitted); the current hash moves to `PreviousSecretHash` with `PreviousSecretExpiresAt = now + window`, and a new secret is minted. A window of **0** clears both `previous_*` columns instead of stamping them — an immediate kill, not a zero-length overlap | Update (`ActionRotateSecret`) | `InvalidGracePeriodNotification` | 422 |
| **C14a** | — | **A client-subject caller may not CREATE a client** — declared, not derived (§B-Q8). Stands down when the claim is absent (⇒ `user`) or no identity is present at all | Insert | `ClientsMayNotCreateClientsNotification` | 403 |
| **C14b** | — | **A client-subject caller writes only its own row** — `RequestingIdentityKind == "client"` ⇒ `RequestingClientID` must equal this row's id. Same stand-downs | Update, Archive | `ClientMayOnlyModifyItselfNotification` | 403 |
| **C15** | `CIDR` | The value must parse as an IPv4 or IPv6 prefix, and is stored masked *(the VO answers this)* | InsertOrUpdate | `vos` — `InvalidCIDRBlockNotification` | 422 |
| **C16** | `CIDR` | The universal prefixes `0.0.0.0/0` and `::/0` are refused — an empty collection is how "no restriction" is spelled, and there must be only one spelling | InsertOrUpdate | `UniversalCIDRNotAllowedNotification` | 422 |
| **C17** | `CIDR` | At most **20** entries on one client | InsertOrUpdate | `TooManyAllowedCIDRsForClientNotification` | 422 |
| **C18** | `CIDR` | A duplicate range is refused by business identity | (child add) | `ClientAlreadyAllowsCIDRNotification` | 409 |

**C9 and C10 are the rules that matter most here and they are the easiest to skip.** Without
them, anyone holding `client:grant` mints a machine credential carrying permissions they do
not hold themselves — a non-expiring, non-interactive privilege escalation. They already
exist and are already translated for `User` and `Group`; this entity reuses them rather than
restating them.

**C14a and C14b are inert until the token run mints `identity_kind`**, and that is by design (§B-Q8e):
an absent claim reads as `user`, so the rule evaluates to "no restriction" and changes
nothing until the claim exists. It is written now so it is already in place the day the
claim arrives. The maintainer's own reading applies — *"provavelmente será só por
segurança"*: nothing grants `client:update` to a client today, and nothing has to.

**Action names.** `ActionRotateSecret` dispatches `ModeUpdate`, exactly as
`ActionChangePassword` / `ActionResetPassword` do on `User`, and is told apart from the
ordinary PATCH by `actionName` — the discriminator, since all three share the mode.

## 8. Update shape                                      [required]

**PATCH only.** No sibling in §4, so the PUT-invariant does not bind, and it is the shape
every existing entity in this service uses.

**Patchable:** `name`, `description`, `status`.
**Not patchable, and each is a rule and not an omission:** `tenantId` (C3), `secretHash`,
`previousSecretHash`, `previousSecretExpiresAt` and `secretChangedAt` (no request shape
reaches them — the rotate operation is the only writer), `id`.

## 9. Surfaces & reads                       [required]

- **REST: yes** · **GraphQL: yes** (every existing entity is on both) · **gRPC:** via
  `/omnicore:implement`, not this run · **Exports (CSV/XLSX): no** — nothing else in this
  service has them, and a spreadsheet of credential metadata is not an artifact worth
  producing by default · **Integration events:** none — the CDC relay is not deployed
  (`README.md` § *Current state*).

| Operation | Route | Permission |
|---|---|---|
| list | `GET /clients` | `client:read` |
| by id | `GET /clients/{id}` | `client:read` |
| create | `POST /clients` — **the only response that ever carries a secret, besides the rotate** | `client:insert` |
| patch | `PATCH /clients/{id}` | `client:update` |
| archive | `PATCH /clients/{id}/archive` | `client:archive` |
| **rotate secret** | `POST /clients/{id}/secret` | **`client:rotate-secret`** |
| grant role | `POST /clients/{id}/roles` | `client:grant` |
| revoke role | `PATCH /clients/{id}/roles/{childId}/archive` | `client:grant` |
| allow a range | `POST /clients/{id}/allowed-cidrs` | `client:update` (proposed) |
| remove a range | `PATCH /clients/{id}/allowed-cidrs/{childId}/archive` | `client:update` (proposed) |

- **Reserved read controls:** pagination + `orderBy` (defaults) · `?fields=` **yes** ·
  `?search=` **no** (no text index will serve it; `name` filtering covers the need) ·
  `?onlyTotal` **yes** · `?includeArchived` **yes**, gated on `client:read` like every other
  entity.
- **Computed read fields** — ⚠️ **NEITHER WAS BUILT** *(amended after the build; `tasks.md`
  deviation 4)*. `ipRestricted` is REFUSED by the generator, and the refusal is right:
  `read.computed.from` may not name a collection's field, because the derivation runs once
  per document and what the root holds for a collection is a slice. The state stays visible
  — an empty `allowedCIDRs: []` is served on the by-id read and on every listing row.
  `secretRotationPending` was DROPPED as redundant once `previousSecretExpiresAt` is served
  directly, which is strictly more informative than a boolean derived from it. The original
  proposals are kept below for the reasoning:
  - **`ipRestricted`** (bool) — true when the allow-list is non-empty. This is what pays for
    the fail-open decision: "unrestricted" and "not configured yet" are the same state, so
    the listing has to name it rather than render a silent empty array.
  - **`secretRotationPending`** (bool) — derived from `previousSecretExpiresAt > now`.
    Answers "is a rotation still in flight" without exposing either hash.
  Both: `?orderBy=` on them is a typed 400 and a filter over them is impossible — stated
  because it is the kind of thing a caller assumes works.
- **Field-level read authz:** none. The hashes are `hidden` outright, which is stronger than
  `Restrict`; nothing else on this entity is need-to-know.
- **View backing: relational** — the project posture, and read-your-writes matters here: a
  create returns a secret and the caller's very next call is a read of the row it belongs to.
- **Read joins** (mirroring `User`): root → `tenants` for `workspace` + `status` (the caller
  **receives** `tenantWorkspace`; `tenantStatus` exists for C2/C3 alone); `client_roles` →
  `roles` for `key` + `name` (the caller receives both — a list of role ids is unreadable).
  `client_allowed_cidrs` needs none: it references no other aggregate.
- **Filters/sorts** (low-risk, decided): `name` — `eq`, `contains`, `startsWith`, sortable ·
  `status` — `eq`, `in` · `tenantId` — `eq` (injected by the isolation filter, never
  caller-supplied except for `*:*`) · `createdAt` / `secretChangedAt` — `gte`, `lte`,
  sortable. **`secretHash`, `previousSecretHash` and `previousSecretExpiresAt` appear in no
  filter, no sort and no `?fields=` vocabulary** — the hand-made decision property 5 above
  names.

## 10. Authorization                          [required]

**Layer 1 — the permission gate.** The `<resource>:<verb>` taxonomy this service already
grants, extended by one verb: `client:read` · `client:insert` · `client:update` ·
`client:archive` · `client:grant` · **`client:rotate-secret`**.

**`client:rotate-secret` is its own verb and not `client:update`**, for the same reason
`user:reset-password` is its own: replacing a credential is not editing a label, and an
operator who may fix a typo in an integration's description is not automatically an operator
who may hand out a new production credential.

**The allow-list pair is proposed under `client:update`** (alternative: its own
`client:network` verb). It is configuration rather than privilege — it cannot grant a client
anything it does not already hold; it only narrows or widens *where from*. The counter-case
is that widening it is a security-relevant act and `client:update` is otherwise a rather
tame permission.

**Layer 2/3 — data access.** Three cases, stated by the maintainer at the gate and written
out here because they are easy to leave in prose:

1. **A tenant token writes only inside its own tenant** — C1, inherited from `User`. On the
   read side the listing's `ToCriteria` injects the caller's `tenant_id` claim, and the
   by-id read of a foreign row answers **404 and not 403**, deliberately (a 403 confirms the
   id exists).
2. **A `*:*` token crosses the row scope** — the platform operator repairing a customer's
   row.
3. **A client token writes only its own row** — C14, `sub == id`, on this entity only.
   Defense in depth: nothing grants `client:update` to a client today.

All three are **row decisions**, so all three live in `BuildRules` and none of them is a
route. The permission gate above decides admission; this decides which row.

---

## §C — Improvement suggestions (offered, not baked)

- **§C-1 `lastAuthenticatedAt`** — ✅ **declined at the gate.** Kept here with its reason:
  the write-per-token cost, and `authentication_attempts` already answering the question.
- **§C-2 `secretExpiresAt`** — ✅ **declined for now.** Real hygiene; also a scheduled
  production outage the first time it fires. To be worth taking it needs a warning path
  (something has to tell somebody 30 days out) and this service has no outbound channel —
  no CDC relay, no mail. Revisit when one exists.
- **§C-3 IP allow-list** — ✅ **TAKEN.** Now part of the model; see §B and §3.
- **§C-4 scope-down claims** — letting a grant say "this client may act only on tenant X's
  billing resources". This is the `backlog.md` custom-claims question wearing a different hat
  and inherits its unanswered precedence problem. Not now.

## §D — What will have to be written by hand, and why

**Established by running the generator, not by reading about it** — `omnicore-gen explain
keys` at the plugin's bundled build:

- `fields[].runtime` exists, with `source: claim | body | manual | subject | tenant |
  permission | super-admin | present`. So a non-persisted field on the entity **is**
  expressible.
- `fields[].hidden` removes a **persisted** field from every response.
- **There is no key in either direction that puts a runtime value INTO a response.** Every
  response-shaping key subtracts; none adds.

**So the reveal-once secret is hand-written**, and it is a small, well-bounded piece: the
insert result and the rotate result carry one extra string field, filled from the entity
after the write. Everything around it — the entity, the rules, the schema, the migrations,
the routes, the translations — is generator territory.

Also hand-written, by the same boundary the `User` credential operations sit on:

- the `POST /clients/{id}/secret` route, its command, its handler and its
  `ActionRotateSecret` discriminator (`authz.permissions` takes a closed verb set and
  `rotate-secret` is not one);
- ~~`domain.SecretHasher` +~~ **the SHA-256 adapter alone**, with no port *(amended after
  the build)*: nothing in the domain calls it — the aggregate asks `ClientService.HashSecret`,
  which IS the port — so an interface would have existed only to give the adapter a name
  reachable from another layer. `internal/domain` is the one package every layer may import
  without a cycle, and that is never a reason to put a name there. It includes the
  `crypto/subtle` comparison and the `acs_` minting;
- `vos.CIDRBlock` — a `netip`-backed VO with normalisation, which is a composition of parse,
  mask and two refusals rather than a pattern;
- the `client_roles → roles` foreign key and the reverse index in the migration (a reference
  to another aggregate is outside the spec language — the same hook the `User` migration
  documents);
- the seven catalog entries for every new notification.

**This §D decides nothing.** Gate 1d is where the maintainer picks `omnicore-gen` or manual,
and it is asked after this spec is approved.

## §E — Verify

`../../../CLAUDE.md` rule 6 governs: **95 % coverage minimum**, not the skill's 80 %. Three
checks this entity adds to the standard gate:

1. **The secret never appears twice.** Create a client, capture the response, then
   `GET /clients/{id}`, list, `?fields=` naming every field, and the audit row — and assert
   the value is in none of them. Same for the rotate.
2. **The comparison is constant-time.** `grep` for a `==` over either hash; the only
   permitted comparator is `crypto/subtle`.
3. **A zero grace window kills the old secret.** Rotate with `gracePeriodSeconds: 0` and
   assert both `previous_*` columns are NULL — not stamped with a past timestamp.

## §F — What this run does NOT build, and what it owes the next one

**`POST /auth/client/token` is not in this run.** It is a capability, not an entity, and its
owner is `/omnicore:implement` — exactly the split `User` took, where the entity landed first
and `specs/implement/authentication-token/plan.md` followed.

What this spec hands that run:

- the row shape it reads — `secret_hash`, and `previous_secret_hash` + its expiry: **two
  verifies on the miss path**, which is what Q4's cost argument was about and what Q4's
  answer makes cheap;
- the eligibility checks it must make: `status = active`, `deleted_at IS NULL`, and the
  owning tenant neither archived nor suspended — all of it answered by one aggregate load
  through the read joins;
- `identity_kind = 'client'` on every `authentication_attempts` row, which the table already
  documents and which makes the existing lockout apply unchanged;
- **no refresh token** — RFC 6749 §4.4.3, and the plan and README both already say so: the
  secret *is* the long-lived credential;
- the claim set: `sub` (the client id), `tenant_id`, `tenant_workspace`, **`name`** (the
  client's label), **`identity_kind: "client"`**, `permissions`, `roles` — and **no
  `email`, no `groups`, no `must_change_password`**;
- **`identity_kind` on the USER token too**, value `"user"`. The claim is only useful if
  both kinds carry it; a claim minted by one side is an inference on the other, which
  §B-Q8d rejected;
- **`name` is in that list for the audit trail, not for the token's own sake.**
  `auth.auditClaims` is *"exactly the values that have no other vehicle"* — `Actor` and
  `TenantID` are already top-level columns, `tenant_workspace` is there because no other
  service can resolve the UUID, and `email` is there because `Actor` on its own is a UUID.
  A client has no e-mail, so its analogue is its label. The list becomes
  **`[tenant_workspace, email, name, identity_kind]`** in both profiles, and the user token
  already carries `name` — one vocabulary, no per-subject-kind special case. The cost, said
  plainly: user audit rows gain a short `name` string beside the `email` they already
  carry.

**Two hard prerequisites that block that run, not this one:**

1. **The source IP must be resolved correctly behind the proxy**, or the allow-list is
   theatre. `X-Forwarded-For` unguarded is spoofable — an attacker sets the header to an
   allowed range and walks through. The socket IP alone is the load balancer, which blocks
   everybody. The answer is a trusted-proxy configuration, and it is
   `/omnicore:configure`'s territory rather than this run's.
2. **The allow-list check is enforced at token mint and nowhere else.** authcore does not
   see the requests a client later makes to other services, so an allowed range constrains
   where a token is *obtained*, not where it is *used*. Any documentation of this feature has
   to say that, or it will be read as something it is not.
