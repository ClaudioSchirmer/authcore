# Backlog

Ideas and open questions that are **not** decided yet. Nothing here is a commitment: an
entry earns a spec (`specs/scaffold-entity/<entity>/spec.md`) and a README row only after
the maintainer approves it. Entries stay until they are either promoted or dropped with a
reason.

---

## Declined: custom claims on `Group` (2026-09-01)

Raised 2026-08-24, never specified, and now turned down by the maintainer: **a group will not
carry claims.** It is recorded here with its reason rather than deleted, so a future reader
sees a decision instead of an omission.

The reason is the question the entry never got past. A user reaches several groups, so two of
them can set the same key to different values, and resolving that needs a precedence rule.
Effective permissions today have **no precedence and no deny rule**, and a claim map with
last-writer-wins would be the first place that stops being true — the Keycloak case the
`Claim` survey below documents, where two mappers target the same name and only one value
survives in an order that is neither documented nor stable.

The need it described is served by the `Claim` catalog, built 2026-08-28, which removes the
collision by construction instead of by a rule: one name is one definition, and a principal
holds at most one value per definition.

---

## Declined at the `Client` model gate (2026-08-26)

Two additions were weighed and turned down. They are here with the reason rather than as bare
ideas, so a future reader sees a decision instead of an omission.

**A hard expiry on a client secret** (`secretExpiresAt`). Real hygiene — it is what forces
rotation instead of letting a credential live for years — and also a scheduled production
outage the first time it fires. To be worth taking it needs a warning path: something has to
tell somebody thirty days out. This service has no outbound channel at all — no CDC relay, no
mail — so the feature would ship as a timer that silently breaks an integration at 03:00.
**Revisit when an outbound channel exists**, not before.

**Scope-down claims on a grant** — letting a role grant say "this client may act only on
tenant X's billing resources". It is the custom-claims question from the entry above wearing a
different hat, and it inherits the same unanswered problem: effective permissions today have
**no precedence and no deny rule**, and a scoped grant is a deny rule by another name. The
precedence question has to be answered first, and answering it for clients alone would mean
two authorization models in one service.

## Built: `POST /auth/client/token` (2026-09-01)

**Status: DONE.** The route exists, and the plan that built it is
`specs/implement/client-credentials-token/plan.md`. This entry stays rather than being
deleted, because the run departed from the contract in two places and a future reader should
meet the departures as decisions rather than as omissions.

It honours what `specs/scaffold-entity/client/spec.md` §F wrote down when the entity shipped:
the two hashes with the grace window, `status = active` plus a live tenant, `identity_kind`
on both token routes, no refresh token, and the claim set — `sub`, `tenant_id`,
`tenant_workspace`, `name`, `identity_kind`, `permissions`, `roles`, and no `email`, no
`groups`, no `must_change_password`. It also resolves the tenant's `x_` claims from
`client_claims`, which §F predates. `auth.auditClaims` gained `name` and `identity_kind` in
both profiles, as §F prescribed.

**Departure 1 — the lockout counts but never locks.** §F said the existing lockout would
"apply unchanged". It does not, deliberately. The lockout makes guessing a ~30-bit human
password expensive; a client secret is 32 bytes from `crypto/rand`, so no rate guesses it and
the lock buys nothing against the attack it was designed for. What it would buy an attacker is
a lever: **a client id is not a secret** — it is the row id, the `sub` of every token that
client presents, and a column of `GET /clients` — so five wrong guesses would take a
production integration off the air for fifteen minutes, repeatable forever. Every outcome is
still written to `authentication_attempts` under `identity_kind = 'client'` and to the log
stream, so the forensic and alerting surface is whole; only the refusal is gone.

**Departure 2 — §F's proxy warning describes a risk this pin does not have.** It said an
unguarded `X-Forwarded-For` is spoofable. At `omnicore v0.68.0` no header is read at all:
the framework builds its Fiber app with neither `TrustProxy` nor `ProxyHeader`, and its
`http:` block carries no key that could reach them, so `c.IP()` is always the socket peer.
Not spoofable. The real consequence is the other half — **behind an ingress or a load
balancer every request carries the balancer's address**, so an allow-list of real egress
ranges refuses everybody and one holding the balancer's range admits everybody. That is
stated in the OpenAPI description of the route, in the README and in `ACCESS_MATRIX.md`, and
a framework feature request for a `http.proxy` block was raised rather than reading the
header here — which would have built the exact vulnerability §F warned about. **Reopen this
entry when that block ships**: filling in a deployment's trusted range is a
`/omnicore:configure` job, not this route's.

**Still true, and worth repeating wherever this feature is documented:** an allowed CIDR
constrains where a token is *obtained*, never where it is *used*. authcore does not see the
requests a client later makes to other services.

---

## Custom claims via a `Claim` catalog (tenant-owned, two levels)

**Status: PROMOTED — the catalog was approved and built on 2026-08-28.** The approved model is
`specs/scaffold-entity/claim/spec.md`; the generator spec is
`specs/omnicore-gen/claim.omnicore.yaml`; the entity is described in the README under
`### Claim` and its reach in `ACCESS_MATRIX.md` under `## Claim`. This entry stays rather than
being deleted, because most of what it asks is still open — what changed is that those
questions are now reachable instead of blocked.

**What the model gate settled, and what the answers cost:**

- **The reserved prefix is `x_`.** `ext_` was two runes dearer, a URI namespace (Auth0's
  answer) safest and by far the most expensive per token, and an explicit reserved LIST of the
  nine platform names cheapest of all — and refused, because it protects only against the names
  that exist NOW: the tenth platform claim would collide with a definition a customer already
  created, and there is no migration out of that.
- **The prefix is CALLER-OWNED, not server-owned.** The caller types it, the column stores it,
  a token would mint it, and nothing prepends or strips it anywhere. The alternative — a bare
  `cost_center` on the wire that the server prefixes — is this entry's own argument against a
  postiche internal name, wearing a prefix instead of a second column: the wire name and the
  token name would differ, so a consumer reading `x_cost_center` out of a JWT and searching the
  catalog for it would find nothing.
- **The reserved-platform-tenant dependency was BROKEN, not inherited.** This entry predicted
  the catalog would become the third entity blocked on that tenant. It is not: the prefix rule
  applies to every definition created through the API with no exception carved for a tenant, so
  the platform's own nine would enter by migration — the same door the `*:*` role enters by —
  and nothing in the entity needs to know which tenant is reserved.
- **Scope: the catalog only.** The two owned collections below are children of `User` and of
  `Client`, which already exist, so they are `/omnicore:evolve-entity` work — one run per
  parent — not this one. *(Both runs landed on 2026-08-28, plus a third
  on `Claim` itself for the narrowing hole they exposed.)*

**The two edge collections were built on 2026-08-28**, one run per parent —
`specs/evolve-entity/user/spec.md` and `specs/evolve-entity/client/spec.md`. Four of the
questions above are therefore answered, and they are recorded here as decisions rather than
struck out, because each one cost something:

- **Which verb sets a value: a new one.** `user:set-claim` and `client:set-claim`, not
  `user:grant` / `client:grant`. The criterion was the one this entry states — "riding
  `user:grant` means whoever may hand out roles may also set claims, and those are not
  obviously the same job" — settled by the precedent `ClientAllowedCIDR` had just set two days
  earlier with `client:manage-network`, and by one turn more: authcore cannot see what a
  consumer does with `x_cost_center`, so setting a value is potentially conferring privilege
  **in a way this service cannot audit**. Cost: two more literals nobody guesses from the
  pattern, taking that list from five to seven.
- **The correction verb is a PATCH carrying only `value`**, and getting there took two passes.
  The first pass mounted the generator's `change`, which emits a full-body PUT: it made the
  caller re-send `claimID` on every correction AND made the definition an entry points at
  mutable — this entry's own `role_permissions` argument turned back on it. The verb was
  dropped, the gap reported, and omnicore-gen 0.49.0 (on framework v0.63.0) added
  `children[].change` with `shape` and `patchExcludes`. The collection now declares
  `shape: patch` and `patchExcludes: [ClaimID]`: the definition is read off the stored entry,
  so there is no field in which to send a different one. Framework v0.63.0 sits underneath,
  refusing a child change that collides with another ACTIVE entry's identity (409) — the
  duplicate half, for every consumer.

- *(Superseded by the bullet above, kept because it is where the reasoning lives.)* **No
  `change` verb, decided on review the same day.** The entry was going to mount one, on
  the argument two paragraphs above — the identity is the `ClaimID`, `Value` is a separate
  mutable column, so a correction is one entry changing. The verb the generator emits does not
  honour that: it is a PUT whose body carries the whole entry, `claimID` included, so it makes
  the caller re-send what the server already knows AND makes the definition an entry points at
  mutable — which is this entry's own `role_permissions` argument turned back on it. The right
  shape is a PATCH carrying only `value`, and the spec language cannot say it (the root has
  `update.patchExcludes`, a collection entry has nothing). **Recorded as an omnicore-gen gap.**
  Correcting a value is archive + add meanwhile, which also keeps the old value readable on the
  archived row instead of overwriting it — `value` is one column with no history.

- **Removal on the edge: soft, like every other collection here.** The argument for a hard
  delete was "a claim value is not a privilege", and it falls with the verb decision above: if
  a consumer authorizes on `x_plan_tier`, an access review has to be able to read what a past
  value meant.
- **Type of the value: string, validated against the definition.** `valueType` is enforced at
  BOTH levels by one function, so the two cannot disagree about what a `bool` is.
- **A per-principal cap: 20.** A header budget before it is a count — 20 × 256 runes is ~5 KB
  of level-1 values riding on every request, which fits the 8 KB buffer most proxies default
  to. Fifty, the cap `groups` and `roles` carry, would be ~12.8 KB and would stop being a cap.

**One thing the build FOUND rather than inherited**, and it is closed too
(`specs/evolve-entity/claim/spec.md`): `appliesTo` was mutable in both directions, and
narrowing it while an edge held a value stranded that value invisibly — present, readable, and
un-writable. It is now refused for all four narrowing transitions; widening is untouched.

**The emission landed on 2026-08-28** — `specs/implement/emit-custom-claims-on-user-token/plan.md`.
`buildClaims` now resolves the chain for a USER and mints it beside the fixed nine. Four of the
decisions above were taken there, and each one cost something:

- **The value is TYPED on the wire**, not a string. That is what makes `valueType` mean something
  at emission rather than only at write time, and it is why the read join carries `ClaimValueType`
  up to the entry at all. The gate is the domain's own `ClaimValueMatchesValueType`, so there is no
  fourth reading of what a `bool` is and a value the catalog accepted as a default can never be one
  the emission refuses. Cost: a value written by direct SQL that does not parse has to go somewhere,
  and it is omitted with a `Warn` rather than coerced — a wrong answer being worse than no answer.
- **The walk is over the CATALOG, not over the user's entries**, and that was the one thing the
  build found rather than decided. Level 1 arrives free — the read join fills the name and the type
  on every loaded entry — so iterating the entries would have been the cheap shape. It is wrong
  twice: level 2's defaults belong to definitions the user holds no entry for, so they would never
  be reached; and a read join is deliberately not archive-gated on its target, so an entry whose
  definition was RETIRED would keep minting. Driving from the catalog drops both, fail-closed.
- **The size budget is 20 per token**, the same number the per-principal cap carries, with values
  set on the user spent before any tenant-wide default and the overflow dropped under a `Warn` that
  NAMES what it dropped. Truncating is the fail-closed direction here — a custom claim only ever
  ADDS a fact, so dropping one can deny a consumer and can never grant. **The hole it did not close
  is now closed** (`specs/evolve-entity/claim-catalog-cap/spec.md`, 2026-08-28): the catalog caps
  itself at **20 ACTIVE definitions per tenant per identity kind** — `user`+`both` on one side,
  `client`+`both` on the other, so a `both` definition spends a slot on each. The bucket it caps is
  the SAME predicate `ClaimDefinitionsOfTenant` walks, so the truncation above is now unreachable
  through the API and remains only as the seatbelt for rows a migration or a direct `UPDATE` wrote.
  The cap asks only about the kinds a write ADDS, which is what keeps a tenant already over the line
  able to repair its own catalog rather than being frozen out of every update.
- **A `mustChangePassword` session carries none of them**, the same line already drawn for its
  permissions, and the response body mirrors the token exactly, restriction included — one
  resolution per request with two readers, because a body advertising claims the token omits is the
  bug that made `effectivePermissions` a single function in the first place.

The reserved-name question the entry above raises turned out to need no runtime answer beyond a
seatbelt: the `x_` prefix stops a collision at the API, and the merge assigns the fixed set LAST so
that a definition written straight into the table by migration cannot displace `permissions` or
`tenant_id` either. The two guards fail in opposite directions on purpose.

**The third-level question is CLOSED by the construction, 2026-09-01** — it is struck from the
open list rather than answered by a decision, because the shipped chain leaves it no object. The
candidate set is `Eq(TenantID, account.TenantID)` AND `In(AppliesTo, user, both)` under the ACTIVE
scope (`internal/infra/authentication_reader.go`, the definitions read in `ResolveSignIn`), so a definition owned by the
reserved platform tenant is never in a member tenant's walk: there is no platform default for a
tenant to override. What the entry asked for is what the two levels already are — the resolution is
the SUM of the tenant's catalog and the principal's own entries, and where both carry the same
definition the principal's value wins (`authentication_claims_manual.go:141-147`). Two residues,
both deliberate and neither open: an entry pointing at a RETIRED definition mints nothing, since
the catalog is the vocabulary and the entries only overlay it; and the vocabulary can diverge
between tenants, which is the trade this entry recorded when it chose two levels over three.

**Both halves now reach a token.** The user one on 2026-08-28, the client one on 2026-09-01
when `POST /auth/client/token` was built — the same two levels through the same function, over
`client_claims` and the `client`/`both` half of the catalog. The **audit allowlist** closed in
that run too: `auth.auditClaims` gained `name` and `identity_kind` in both profiles. Nothing
from this entry's original list is still open.

The rest of this entry is the original draft, kept because it is where the reasoning lives.

Consumers need extra facts on a token that are neither permissions nor platform identity:
`cost_center`, `region`, `plan_tier`, `erp_id`. Two cases have to hold **at the same time**: a
**default** value shared by many accounts, and a **specialised** value that differs per
account. `Client` cannot borrow `Group` for this — it has no groups — and a single map on
`Tenant` mints the same claim into every token of both identity kinds, which is the case that
started the survey.

### What other systems do

- **Okta** — two named sources and an explicit fallback operator: `user.<x>` per account,
  `app.profile.<x>` per application, combined per claim with the Elvis operator
  (`user.costCenter ?: app.profile.costCenter`).
- **Auth0** — the same split under different names: `app_metadata` (per user) and
  `client_metadata` (per application); an Action merges them with `??`, checking the client id
  on the M2M path.
- **Cognito / Ory** — no declaration at all: a pre-token Lambda, or a Jsonnet mapper, does the
  whole merge in code.
- **AD FS** — the naming precedent: its registry is called `Claim Description` (claim type,
  name, description, publishing state). A catalog of CLAIMS, not of attributes.
- **Keycloak** — the cautionary one, and the closest to a "grant N bundles" design. It has a
  hardcoded-claim mapper (the default) and a user-attribute mapper (the specialised value) in
  the same pipeline; when two mappers target the SAME claim name only one value survives, and
  the priority order is neither documented nor stable (keycloak#25774, keycloak#16347). The
  practical advice that remains is "use different names".

The transversal lesson: **every system that sustains default + specialised has ONE ordered
place where precedence is written.** None lets precedence emerge from the binding — and the
one that came closest is the one with undefined behaviour.

### Why the registry is called `Claim` and not `Attribute`

The industry distinction is real but does not apply here. AD FS keeps the data in an
*attribute store* and a *Claim Description* names what leaves in the token; Keycloak keeps a
user *attribute* and a protocol mapper turns it into a *claim*. Both need two words because
the data exists independently of the token.

In this service it does not: the row exists to be minted. A registry entry would carry a `Key`
and a `ClaimName` that are always 1:1, with no transformation and no N:1 — two fields for one
value, which is what a postiche internal name looks like. Called `Claim`, they collapse into
one. The indirection only earns its keep alongside an `emitAsClaim` flag and an emission
policy, and neither is proposed here.

If a need ever appears for principal data that must NOT reach the token, it deserves its own
model rather than a boolean on this one.

### The chain — two levels

The claim name lives in the DEFINITION, never in the binding. One name is one definition, and
a principal holds at most one value per definition (a unique constraint on the edge).
Collision is therefore impossible by construction, and precedence is not a merge rule between
peers but an ordered chain for the same name — first non-null wins:

```
  user_claims.value  /  client_claims.value        level 1 — the specialised value
            ↓ if null
  claims.default_value                             level 2 — the default, of the
     (of the tenant that owns the definition)                tenant that owns it
            ↓ if null
  the claim does not enter the token at all
```

**Two levels rather than three, and `tenant_id` is why.** An earlier draft had a third level —
a `tenant_claims` collection holding a per-tenant default for a globally defined claim. Once
the definition itself belongs to a tenant, its `default_value` IS that tenant's default, and
the middle collection has no work left to do: one whole aggregate, with its verbs, its
permission and its seven translation catalogs, disappears.

What that trades away is recorded under the open questions: two tenants needing `region` write
two definitions, so the vocabulary can diverge between them.

### The registry — basic fields

Table `claims`. Tenant-owned, following the decision already taken for `Role`: *the platform's
own rows live in the reserved platform tenant rather than in a null scope*. **This entry
therefore inherits `Role`'s and `Group`'s dependency on that reserved tenant**, which the
README still lists as not started.

| Field | Type | Notes |
|---|---|---|
| `TenantID` | id | the owner, NOT NULL. The reserved platform tenant is what marks a row as the platform's — no `isPlatform` boolean is needed |
| `Name` | string | the exact name minted into the token. Unique PER TENANT over active rows |
| `ValueType` | enum | `string` · `number` · `bool` — what the edges are validated against |
| `AppliesTo` | enum | `user` · `client` · `both` — which identity kinds may hold a value |
| `DefaultValue` | string, nullable | level 2 of the chain; null means "no default" |
| `Description` | string | what the value means, for the operator filling it in |

Managed columns as everywhere else (`revision`, `created_at`, `updated_at`, `deleted_at`).

**Uniqueness is per tenant, and the reserved prefix is what keeps that safe.** Global
uniqueness would forbid two tenants from both naming a claim `region`, which is harmless — a
token carries exactly one tenant. But per-tenant uniqueness alone would let a tenant define
`permissions` or `tenant_id`. The prefix closes it by construction: a definition owned by any
tenant OTHER than the reserved one must carry it, and no platform claim does. The runtime
check is then a seatbelt rather than the mechanism.

### The two owned collections — basic fields

The SAME two fields on each parent, so the edge is one shape learned once:

| Field | Type | Notes |
|---|---|---|
| `ClaimID` | id | FK to `claims.id`; the entry's business identity |
| `Value` | string | validated against the definition's `ValueType`; never null on the edge |

| Parent | Child | Table | Parent column |
|---|---|---|---|
| `User` | `UserClaim` | `user_claims` | `user_id` |
| `Client` | `ClientClaim` | `client_claims` | `client_id` |

`businessIdentity: [ClaimID]`, unique per parent — that constraint IS the anti-collision
argument above, not a hygiene detail.

`editStrategy: per-child` with `operations: [add, remove, change]`. Note the difference from
`role_permissions`, which deliberately refuses `change`: there the single stored column IS the
business identity, so a change turns grant A into grant B while keeping A's row id. Here the
identity is the `ClaimID` and `Value` is a separate mutable column, so "correct this cost
center" is genuinely one entry changing rather than two events.

### Seeding the platform's own nine

Because the catalog is of CLAIMS, it can hold **all** of them — including the nine this
service already mints (`identity_kind`, `tenant_id`, `tenant_workspace`, `email`, `name`,
`permissions`, `groups`, `roles`, `must_change_password`), seeded by migration into the
reserved platform tenant. Worth doing for three reasons beyond tidiness:

- the catalog becomes the living documentation of the token's vocabulary, instead of a prose
  list that ages;
- `AppliesTo` then states as DATA what the built `POST /auth/client/token` states as
  text — that a client token carries no `email`, no `groups` and no `must_change_password`;
- two of those names (`permissions`, `tenant_id`) are read by the framework across the whole
  mesh, so having them present and platform-owned makes their reservation visible to anyone
  reading the table.

### Emission

`buildClaims` (`internal/application/commands/handlers/utils/authentication.go`) keeps
its fixed set and merges the resolved map on top. A null at both levels means the claim is
simply absent — an absent claim and an empty one are not the same thing to a consumer. The
refresh path already rebuilds claims from the database on every redemption, so a corrected
value propagates on the next refresh with no special invalidation.

### What has to be answered before this becomes a spec

- **Does a tenant need to override the default of a PLATFORM-defined claim?** This is the one
  question that reopens the third level. If yes, `tenant_claims` comes back and the chain is
  three deep again. If no, the two levels above stand and the vocabulary simply lives per
  tenant.
- **The reserved prefix, literally.** `x_`? `ext_`? A URI, as Auth0 requires for
  collision-resistance? Whatever it is, it is the mechanism, so it has to be decided before
  the uniqueness rule can be written.
- **Who writes, and under which verb.** The catalog is tenant-owned, so a tenant admin creates
  definitions — but is setting a VALUE on a user the same job? A new `*:set-claim`, or the
  existing `*:grant`? The criterion is exact: riding `user:grant` means whoever may hand out
  roles may also set claims, and those are not obviously the same job.
- **Removal semantics on the edge.** `role_permissions` soft-removes because an access review
  has to read what a past grant meant. A claim value is not a privilege — is a hard delete
  right, or does the audit story want the same stamp?
- **`auth.auditClaims`.** Forwarding an arbitrary map into every audit row, one per write,
  forever, is a separate decision from putting it in a token. The allowlist is deliberately
  two entries today.
- **Claim-size budget.** The shipped token carries group and role KEYS rather than display
  names precisely because it rides in a header on every request. Whatever this adds lands on
  top of a budget already argued down to the minimum.

### Deliberately left out of this shape: the bundle

Granting claims in a PACKAGE — a `claim_sets` aggregate in `Role`'s shape — is the one
addition that brings a genuinely new problem. Two packages granted to the same principal can
carry the same definition with different values, and that is exactly the Keycloak case:
undefined precedence reaching the token. Resolving it needs a rule (grant order, a priority on
the set, or refusing the conflicting grant at bind time — only the last keeps the
indeterminacy away from the token). The two levels above already deliver "default plus
specialised" without it.

The earlier `Custom claims on Group` entry asked a related question through a different
carrier; this shape does not answer it, since a user reaches several groups and the collision
returns there. **That entry was declined on 2026-09-01** (see the top of this file) rather than
answered: the built catalog removes the collision by construction only along the chain it
defines — one name is one definition, and a principal holds at most one value per definition —
and a group carrier would reintroduce exactly the multiplicity that construction avoids.

---

## Resolved by measurement: the sign-in read's four concurrent connections (2026-09-01)

**Status: CLOSED — measured 2026-09-01, the shape is KEPT and nothing changed.** Raised the same
day it was closed: it arrived with the sign-in read redesign
(`specs/implement/authentication-token-reads/`), which shipped with only sequential numbers behind
it. The entry asked three questions and a load test answered all three; it stays as the record,
because "we measured and left it alone" is a decision, and the next person to see four goroutines
in `ResolveSignIn` deserves to find it already asked.

Measured on the dev bench (16 cores, Postgres in Docker on the same host) against a tenant of 5k
roles / 2k permissions / 10k users, with a principal holding 10 direct grants, 5 groups (20
inherited roles) and 10 claim values. The concurrent shape was compared against a SERIAL one — the
same four statements, one after another — under a bounded pool.

**Is four the right fan-out? YES, and cutting it would be strictly worse.** The fan-out does not
consume more connection-time, it CONCENTRATES it: four connections for 200µs is the same product as
one connection for 800µs. So throughput is identical between the two shapes at every pool size
(pool=4, 64 workers: 1968 rps concurrent vs 2033 serial; pool=16: 4153 vs 3961), and the only place
they differ is the unqueued latency the concurrency was bought for — 727µs against 1.34ms at
pool=4. The optimisation this entry floated — skipping the two claim reads for a tenant with no
catalog — would buy nothing anywhere it was measured.

**Or is the pool the thing to size? NO, and this is the answer that changed the recommendation.**
Widening the pool from 4 to 8, 16 and 32 did not improve the one measurement that degrades. The
default is `max(4, NumCPU)` and it was never the bottleneck: **the credential is**. One Argon2id
verification (m=19MiB, t=2) costs **15.6ms** and the four-statement read costs **795µs**, so the
read is under 5% of a sign-in and the endpoint saturates its cores long before it saturates a
connection. On a 4-core box the same arithmetic holds with more room, not less: Argon2 caps that
box near 256 sign-ins/s, which asks about a QUARTER of one connection on average against the four
it has. `relational.pool` is therefore deliberately still unset — a fixed number chosen without
production data (the backend's `max_connections` over the replica count) can be worse than a
default that adapts to the node.

**What does it do when the pool is exhausted? It QUEUES, linearly.** Pool of ONE against 32
workers — four goroutines per request competing for a single connection — completed 320 resolutions
in 502ms with a p99 of 61ms and zero errors. No deadline, no refusal, and no deadlock: nothing here
holds a connection while waiting for one, because `ResolveSignIn` runs outside any transaction.
**That is the property worth guarding**, and it is the one thing about this entry that is not
self-evident from the code — the day somebody wraps the sign-in in a transaction, four goroutines
per request stop being harmless.

**What the measurement FOUND rather than answered**, recorded because it is the real cost and it is
not what the entry expected. The sign-in does not hurt itself; it can hurt its NEIGHBOURS — any
other route drawing on the same pool. An ordinary single-statement read, timed beside a realistic
sign-in load:

| load | neighbour p50, concurrent | serial | ratio |
|---|---|---|---|
| idle | 256µs | 256µs | — |
| 8 workers ≈ 378 sign-ins/s | 318µs | 306µs | 1.04x |
| 16 workers ≈ 558 sign-ins/s | 3.45ms | 1.17ms | 2.9x–4.0x |

The 16-worker row reproduced three times (3.96x / 3.44x / 2.86x) and, decisively, **a wider pool
did not fix it** — which is what identifies it as CPU rather than connections. At that width all 16
cores are inside Argon2 and the concurrent shape amplifies because it puts four goroutines per
request on a scheduler with no free core. Below roughly 378 sign-ins/s per instance it costs
nobody anything.

**The limit of the experiment, stated rather than buried.** It is single-host, with Postgres on the
same machine. A remote backend lengthens every hold by its round trip, which makes the burst wider
— but the connection-time totals stay identical between the shapes, so the prediction is that
throughput is unchanged and burstiness is not. That is a prediction, not a measurement, and it is
the thing to re-measure if this ever comes back.
