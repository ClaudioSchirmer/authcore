# Capability plan — authentication-token

> **Superseded 2026-09-06** — omnicore v0.74.0 renamed the managed archive slot
> `DeletedAt` → `ArchivedAt` (builder, logical name and the `deletedAt` wire token),
> and this service renamed the physical column `deleted_at` → `archived_at` in the same
> run. The vocabulary below was rewritten accordingly; the decisions it records are
> unchanged. See `../../upgrade/v0.73.0-to-v0.74.0/migration-plan.md`.

- **Status:** APPLIED (2026-08-26) — round 2 complete and proven against a live boot.
  Round 1 (the lockout counter, `/omnicore:evolve-entity`) is NOT started; see §3.
- **Framework pin:** `github.com/ClaudioSchirmer/omnicore v0.61.0` (latest published — checked
  against proxy.golang.org; that pin's docs are the authority for every line below)

## §1 The request (restated)

> vamos implementar um novo endpoint sob o grupo "authentication" para gerar o token JWT do
> usuário, regra simples, receber email + senha, validar, se validado, gerar o token e
> refreshToken JWt, com os dados do usuário preenchido, os grupos, caso tenha grupos, e todas
> as permissions que estão abaixo dos grupos -> roles -> permissions ou roles -> permissions.
> Se falhar autenticação, recusar no padrão canonico um 401, com mensagem genérica, usuário ou
> senha inválidos, traduzível via notifications. Seguir o padrão do framework para os
> endpoints. E padrões de grandes empresas e sistemas de geração de tokens. Me ajude a pensar.

When this is done, authcore stops being a service that only *validates* tokens somebody else
minted and becomes the platform's own IdP. A public `POST /auth/user/token` takes an
e-mail and a password, verifies the credential against the Argon2id hash already on the
`users` row, resolves the caller's effective permission set across both grant paths
(`user → roles → permissions` and `user → groups → roles → permissions`), and answers with a
signed RS256 access token plus an opaque, single-use refresh token; `POST
/auth/user/token/refresh` rotates that pair. Every other service in the mesh accepts the
token with configuration alone — `auth.jwt.jwksUrl` pointed at authcore's JWKS document — and
no framework change. A failed authentication answers one generic, translated 401 in the
canonical envelope, indistinguishable across "no such e-mail", "wrong password", "suspended
account" and "unavailable tenant". (Not "locked out": the lockout is round 1, and round 1
was not built — see §3.)

**This plan is the SECOND of two rounds.** The maintainer chose a failed-attempt lockout
counter on the `users` row, which is a schema change to an existing aggregate and therefore
belongs to `/omnicore:evolve-entity`, not to this skill. Round 1 adds the three columns and
their rules to `User`; this plan consumes them. The sequence is stated in §3 and the dependency
is explicit in §5 — nothing here is interleaved with that round.

## §2 Routing evidence — the owning docs

| Capability piece | Owning section(s) at this pin | Existence check |
|---|---|---|
| Minting access + refresh tokens (`authcore.Issuer`, `IssueWithRefresh`, `RedeemRefreshToken`, `TokenRequest.Claims`, key states, `auth.issuer:` yaml, JWKS route) | `token-issuance.html` — the whole surface, plus the explicit statement that the framework ships methods and NEVER HTTP endpoints: `POST /auth/login` is the consuming service's job | `CLAUDE.md` Documentation Map row "Token issuance"; verified in code: `web/authcore/issuer.go` (`NewIssuer`, `Issue`, `IssueWithRefresh`, `RedeemRefreshToken`, `JWKS`), `bootstrap/deps.go:191` (`Deps.Issuer`), `bootstrap/wiring.go:83` (`Wiring.RefreshTokenStore`) |
| Refresh-token persistence (`RefreshTokenStore`: `Save`/`Lookup`/`MarkUsed`/`RevokeFamily`, hash-only seam, family revocation on reuse, `ErrRefreshTokenReused`) | `token-issuance.html` § Refresh tokens | `web/authcore/issuer.go:515`; the framework ships NO implementation — grep over the module returns only the port, the wiring field and its tests |
| The claim vocabulary the mesh reads back (`permissions`, `tenant_id`, `sub`) | `authz-seams.html` (3 concentric layers), `auth-middleware.html` (Identity population) | `application/configuration/authz_config.go:10-11` (`defaultPermissionsClaim = "permissions"`, `defaultTenantClaim = "tenant_id"`); `identity.go` `HasPermission`/`IsSuperAdmin`/`TenantID`; `parsePermissionsClaim` accepts `[]string` of `resource:action` |
| The route seat — a hand-written handler on the canonical pipeline, with OpenAPI derived from the DTO | `custom-command-handler.html`, `openapi.html` | `web/spec_command.go:51` `CommandWithBodySpec` (body, no path id, typed response); already the pattern used by `internal/web/user_credential_routes_manual.go` |
| The generic 401 as a translated notification | `status-mapping.html` | `web/from_result.go:48` maps `domain.SemanticUnauthorized → 401`; `domain.SingleNotificationError` (`domain/exception.go:41`) is the carrier a handler returns |
| Public route / boot validation | `bootstrap.html`, `yaml-reference.html`, `shared/boot-contract.md` | `bootstrap/route_scan.go` — `scanAuthorization` accepts `Doc.Public`, but the auth MIDDLEWARE bypass comes only from `auth.publicRoutes` (`bootstrap.go:794-813`); entries are exact `METHOD /path`, validated at boot |
| Resolving permissions two and three hops out | — (deliberately: no framework primitive covers it) | `read-joins.html` is 1:1, ONE hop, no collections; `relational-view.html` refuses a filter/sort over a 1:N child with a typed 400. The honest path is the neutral read seam `core.Querier.Query` + `core.Dialect` (`infra/db/core/read.go:40`, `dialect.go:53`), which is what the framework's own composer uses |
| Rate limiting / lockout | — (the framework ships none, by design) | `web/error_handler.go:169`: "the framework ships no rate limiter — this branch exists so a middleware that…"; `notifications.TooManyRequestsNotification` exists for a consumer to raise. The chosen counter-on-the-row shape is a schema change → `/omnicore:evolve-entity` |

Routing outcome: **offered at pin**. No `/omnicore:upgrade` (already on v0.61.0), no
`/omnicore:configure` — token issuance is in the "works everywhere" set
(`shared/capabilities.md`); it needs no broker, no Mongo, no relay. Two pieces the framework
deliberately does NOT offer: the multi-hop permission resolution (§3 takes the named legitimate
path — the neutral read seam — rather than approximating it with a join) and the lockout
(round 1, `evolve-entity`).

## §3 Integration semantics [high-risk — propose + CONFIRM]

### The two rounds, and why they are two

| Round | Skill | What it does |
|---|---|---|
| 1 | ~~`/omnicore:evolve-entity`~~ → this plan | **REDESIGNED and SHIPPED 2026-08-26.** No longer an entity change: see *The attempt log* below. |
| 2 | this plan | **SHIPPED 2026-08-26.** The endpoints, the issuer wiring, the permission resolution, the refresh store. |

There is no second round left. Both halves are in the code.

### The attempt log — why the counter left the `users` row

The approved design put `FailedLoginAttempts` / `LastFailedLoginAt` / `LockedUntil` on the
`User` aggregate. **That was wrong, and the maintainer caught it.**

Where a counter is STORED leaks nothing on its own — a column is invisible from outside. What
leaks is a consequence of storing it there: **a counter on the user row only exists for users
that exist.** So an unknown address can never lock, a real one locks on the sixth attempt, and
that difference in BEHAVIOUR is an existence oracle no choice of wording repairs. It is worse
than a wording leak, because it also shows up in the response TIME: a locked account is refused
without paying Argon2id, while an unknown address keeps costing a full verification — reopening
the exact timing oracle the sign-in path was built to close.

**Keying the log by the ATTEMPTED IDENTITY closes it, and rescues the specific 429 message.**
A row exists for any string somebody tries, whether or not it names an account, so an unknown
address locks on the sixth attempt too and gets the same "blocked for 15 minutes". The response
stops distinguishing while keeping the UX the maintainer asked for — the same reasoning behind
Entra's smart lockout.

Three consequences, all improvements on the approved shape:

- **No `/omnicore:evolve-entity` round.** This is a table and an adapter, like the refresh
  store — not a change to an aggregate. The two-round split dissolves.
- **Append-only, so no write contention.** The original counter was an UPDATE to an aggregate
  root, which carries the revision guard: parallel failed attempts against one account would
  have collided in 409s on the security path. An INSERT has nothing to collide with, and the
  lockout becomes a windowed COUNT rather than a stored number.
- **Auto-release is free.** The window slides; nothing has to clear a counter, and "a
  successful sign-in resets it" becomes "count only failures since the last success".

**It is `identity`, not `email`** — the same table serves the coming `POST /auth/client/token`,
where the attempted identity is a client id. `identity_kind` says which.

**IT IS NOT SWEPT, deliberately, and that is the opposite of the refresh table's rule.** The
refresh table sweeps because a dead hash earns nothing; this one is kept because the failures
ARE the product: a security reviewer reads it to find the pattern — one identity hammered, one
IP spraying a hundred addresses, a burst at 04:00 — and a table that deleted its own history
would delete the evidence. **Retention is the maintainer's policy call**, not this service's,
so nothing here expires a row.

Two rules that follow from keeping it forever:

- **The attempted PASSWORD never enters this table** — not in clear, not hashed, not truncated.
  What is recorded is who somebody tried to be, never what they presented.
- **The identity is stored in clear**, not hashed. Hashing was considered and rejected: it would
  leave a reviewer able to see that *some* identity was tried four hundred times but not WHICH,
  which is most of the value gone. The privacy argument is weak here — `users.email` already
  stores addresses in clear, so this is not a new class of data — but it does mean the table
  accumulates identifiers of people who never registered, and that retention is the policy call
  named above.

**Each row also records whether the identity EXISTED at the moment it was tried.** It costs
nothing — the sign-in has already performed that lookup — and it opens no oracle, because it is
a column an attacker cannot read while the response stays uniform. What it buys the reviewer is
the distinction that decides urgency:

- hundreds of attempts, all `identity_existed = false` → credential stuffing from a leaked
  list, noise;
- a few dozen against three identities, all `true` → a targeted attack on real accounts, and
  those people should be told;
- and the ratio itself is the sharpest signal available here: an attacker hitting real accounts
  far above chance is an attacker holding a valid user list, which means an enumeration leak
  already happened somewhere and this is how it surfaces.

The column is NULLABLE, and the null is not laziness: when the lookup itself fails, the service
genuinely does not know whether the account exists, and writing `false` there would be a
recorded falsehood in the one table that exists to be trusted later. It is also named in the
PAST tense — it records what was true at that moment, not what is true now; an account created
or archived afterwards does not rewrite history.

One consequence, stated because it is the honest reading: with a clear-text identity and this
flag, a dump of this table IS a user-enumeration list. It adds no exposure that the `users`
table does not already carry to anyone who can dump one of them — but it is a second copy, and
it is another reason retention is a decision somebody has to take rather than inherit.

**ONE table, not two.** A rollup keyed by identity — running totals, first and last seen — was
proposed and rejected. Every number it would hold is already a query over the detail: the total
is a `COUNT(*)`, the last attempt a `MAX(occurred_at)`, the first a `MIN`. The only argument for
it was that a total should survive a prune of the detail, and that argument inverts on
inspection: **a rollup keyed by the clear-text identity is exactly the record a retention prune
was meant to remove**, quietly outliving the deletion the security owner chose to perform. The
totals are therefore "over what was retained", which is the honest semantics rather than a
number pretending to be older than the evidence behind it.

**Policy (unchanged from the approved one):** 5 credential failures for one identity inside a
15-minute window → refused with 429 naming the remaining window, auto-releasing. An attempt made
while ALREADY locked is recorded (so the reviewer sees the persistence) but does NOT extend the
window — otherwise anyone could hold somebody else's account shut indefinitely by continuing to
try.

A change to an existing entity's write side is `evolve-entity`'s turf by the plugin's own
router; folding it in here would put schema, DTO and translation lockstep in a skill that does
not own it. **Nothing in §5 touches `User`'s schema.**

### Seam and shape

- **Seam:** hand-written `pipeline.Handler`s mounted through `fwweb.CommandWithBodySpec` +
  `fwopenapi.Mount`, exactly as the two credential routes are (proposed).
  *Alternative rejected:* `fwopenapi.MountRaw` with a bare Fiber closure — which is what the
  framework's own doc example uses. It answers `fiber.ErrUnauthorized`, an untranslated bare
  status. The request asks for a translatable notification, and the pipeline is what carries
  the seven catalogs and the canonical envelope. MountRaw stays available if a non-JSON body
  shape is ever wanted; it is not wanted here.
- **Routes:** `POST /auth/user/token` (e-mail + password) and `POST
  /auth/user/token/refresh` (opaque refresh value). OpenAPI tag `Auth`; the existing
  credential routes keep their `Users` tag.

  **THE SUBJECT TYPE IS A PATH SEGMENT, NOT A BODY FIELD** — this is the decision worth
  recording, because it is the one that is expensive to reverse. A client-credentials token
  is planned, and it is a different operation wearing a similar name: a client id and a
  secret instead of an e-mail and a password, claims with no e-mail and no groups, and per
  RFC 6749 §4.4.3 no refresh token at all, since the client's secret already IS its
  long-lived credential. `/auth/client/token` will sit beside `/auth/user/token`, each with
  its own DTO, its own OpenAPI page and its own gate.

  *Alternative rejected — one endpoint with a `grantType` field*, which is what Auth0, Okta
  and Keycloak do. It would rebuild exactly the schema this service already rejected once:
  `internal/web/requests/user_credential_requests_manual.go` records why the two credential
  routes were split rather than share a body whose required fields depend on which operation
  the caller meant. The usual argument for the single endpoint is RFC 6749 compatibility —
  and it does not apply here, because this API is not RFC 6749 in either direction: JSON in
  rather than form-encoded, and the canonical omnicore envelope out rather than a flat
  snake_case `access_token`/`expires_in`. No off-the-shelf OAuth2 client works against it
  either way, so the compatibility a single endpoint would buy was never on the table.

  *Also considered and dropped:* `/auth/login` (the framework doc's own wording) — it names
  a session rather than the thing returned, and leaves no room for the client subtree.

  **Path history:** `/authentication/token` at first boot → `/auth/token` → `/auth/user/token`,
  all on 2026-08-26, all before anything consumed them. The cost of moving now was one
  `publicRoutes` edit; after the first consumer it would have been a breaking change.
- **Sync or async:** fully synchronous inside the request. No hook, no event, no outbox.
- **Failure policy:** the endpoint depends on nothing external. Database unreachable → the
  error escapes as an exception → 500, which is correct: an authentication that cannot read
  the credential must not answer "invalid credentials". Issuer failure (a broken signing key)
  → 500 likewise. Neither is degraded, neither is queued.
- **Idempotency / replay:** a login is deliberately not idempotent — every call mints a new
  access token and a NEW refresh family. Replaying a refresh token is what the framework's
  reuse detection is for: `RedeemRefreshToken` rotates single-use, and a second redemption of
  an already-used value revokes the whole family and returns `ErrRefreshTokenReused`. The
  store must therefore be honest about `Used` and `Revoked` rather than treating them as
  advisory. **`ErrRefreshTokenReused` maps to the same generic 401** — a distinct reply would
  tell an attacker holding a stolen token that it had already been redeemed by the victim.

### The generic refusal, and the ONE deliberate exception

`InvalidCredentialsNotification`, `Semantic() = SemanticUnauthorized` → **401**, answers every
one of:

- the e-mail matches no row;
- the e-mail matches, the password does not;
- the account is `suspended` or archived;
- the owning tenant is archived or `suspended`;
- a replayed refresh token (`ErrRefreshTokenReused`).

Distinguishing any of them is a user-, tenant- or state-enumeration oracle. The notification's
field name is a neutral `"credentials"`, never `"email"` or `"password"`, so the envelope
itself does not say which half was wrong.

**The lockout is the exception, decided by the maintainer.** A locked account answers
`AccountTemporarilyLockedNotification`, `Semantic() = SemanticTooManyRequests` → **429**, with
a specific translated message naming the remaining window ("blocked for 15 minutes"). The
notification is parameterised with the minutes left, the way
`TooManyGroupsForUserNotification{Max: "50"}` already is in this service.

The reasoning is the maintainer's and it is the industry norm (Microsoft and Google both do
it): a user who is silently refused with the same message they get for a typo has no way to
learn that waiting is the fix, and every one of them becomes a support ticket.

**The disclosure this accepts, stated once so it is on the record:** an attacker who sends six
wrong passwords learns whether the address exists here — a 429 means it does, a continued 401
means it does not. The 401 branch keeps its timing equalisation, so the leak is bounded to
accounts an attacker was already willing to lock out (which is itself a denial they could
perform regardless). The trade is deliberate, not an oversight.

**`Retry-After` is not set.** The canonical 429 header would be the complete answer, but a
`pipeline.Handler` receives an `AppContext`, not the Fiber `Ctx`, so there is no seat to set a
response header from where this decision is made. The remaining window travels in the
translated message instead. Closing this properly would mean a middleware or a `MountRaw`
route, and neither is worth giving up the notification envelope for — recorded as a known gap,
not a defect.

**Timing equalisation — not optional at this bar.** Argon2id verification costs ~100 ms; a
SELECT that matches nothing costs ~1 ms. Without equalisation the response *time* answers "does
this address have an account here", which is the exact oracle the shared message exists to
close. On the not-found and the ineligible-account paths the handler runs one verification
against a fixed, process-local dummy PHC hash and discards the result.
`internal/infra/password_hasher.go` already owns Argon2id; the dummy hash is derived once at
package init from a random secret, so it is never a value an attacker can precompute against.

### The token's contents

`TokenRequest.Claims` is free-form; the framework interprets none of it. Reserved claims
(`iss`/`aud`/`exp`/`iat`/`nbf`/`jti`) are the Issuer's and supplying one is a rejected call.
Three claims are load-bearing because the mesh reads them back:

- `sub` — the user's id, set through `TokenRequest.Subject` (never as a claim).
- `tenant_id` — `User.TenantID`, the same value the row-level isolation filter compares
  against, with no translation step.
- `permissions` — `[]string` of `resource:action`. `parsePermissionsClaim` accepts exactly
  this shape. A user holding `*:*` is a super-admin to every service in the mesh.

**The criterion, decided with the maintainer, is NOT size.** The first draft of this plan cut
the token by byte count and got it wrong. The rule that survived scrutiny is:

> A value belongs in the token when it is a STABLE IDENTIFIER the mesh decides on, or when the
> OTHER services' audit trail is illegible without it. Mutable display text that nothing
> decides on and nothing records stays out.

The audit half is the part that is easy to miss and impossible to work around later. The
framework stamps `ActorClaims` into **every audit event of every service** from the token's
claims (`bootstrap/auth_config.go:85` → `WithAudit(..., cfg.Auth.AuditClaims)` →
`write.populateContext`), and `AuditEvent.TenantID` is read from the raw `tenant_id` claim.
The other nine services do not have authcore's `tenants` table — they cannot resolve that UUID
at any price. So without `tenant_workspace` in the token, every audit line across the platform
reads `tenant_id: 8f3e…` and identifies nobody, forever. That is not a display concern.

And the domain already decided who represents a group or a role in an audit line. From the
`groups` migration, on `group_key`: *"Stable machine handle … What an API caller, an audit line
and a directory mapping reference; never the display name."* `role_key` says the same. The
token honours that declaration instead of re-litigating it.

**The resulting claim set:**

| Claim | In? | Why |
|---|---|---|
| `sub` | in | `Identity.Subject`; the `AuditEvent.Actor` of the whole mesh |
| `tenant_id` | in | the row-level isolation filter; `AuditEvent.TenantID` reads this exact claim |
| `permissions` | in | `HasPermission` / `IsSuperAdmin` — the authorization, entire |
| `tenant_workspace` | in | audit legibility across the mesh (above). Immutable by domain rule (`TenantWorkspaceIsImmutableNotification`), so it can never go stale |
| `email` | in | makes `Actor` legible in the other services. Immutable by rule (`UserEmailIsImmutableNotification`) |
| `mustChangePassword` | in | drives the restricted bundle below |
| `groups` (keys only) | in | immutable (`GroupKeyIsImmutableNotification`); the schema names the key as what an audit line references |
| `roles` (keys only) | in | immutable (`RoleKeyIsImmutableNotification`); same declaration |
| `name` (the user's) | in | the weakest of the set, and kept knowingly: it is mutable by PATCH and `email` already carries legibility. It earns its place because recording the actor's name *as of the action* is correct audit behaviour, not a staleness defect |
| `name` of a group / role | **out** | mutable, and the schema is explicit that an audit line references the key, *never the display name* |
| `description` of a group / role | **out** | `VARCHAR(500)` apiece, mutable, and nothing decides or records on it |
| `status` (user's or tenant's) | **out** | a token only exists when both are active, so the claim would be a constant that ages badly |

A user in 20 groups lands around 1.3 KB of claims — comfortable against the ~8 KB header
ceiling proxies commonly impose.

The login *response body* still carries the richer profile (group and role display names, the
tenant's name, account status) because a client renders it once at sign-in and never re-sends
it. That is a convenience for the caller, not a hole in the token: nothing on the authorization
or audit path depends on it.

### The audit allowlist is a SECOND, smaller list — not the claim set

`filterClaims` drops every claim not named in `auth.auditClaims`, so a claim reaches
`ActorClaims` only if the service asks for it by name. But the two lists answer different
questions, and conflating them was a mistake in an earlier draft of this plan:

- the **token claim set** answers *what does a service need to decide and render without a
  network hop* — `groups` and `roles` belong there, so a service can branch on membership
  statelessly;
- the **audit allowlist** answers *who was the actor* — authorization state does not belong
  there at all. `groups` and `roles` are variable-length arrays that would be copied into
  every audit row, one per write, forever.

Two of the four identity values are already first-class fields on `AuditEvent` and need no
allowlist entry:

- `Actor` is filled from `ctx.ActorSubject()` — the user id, already there;
- `TenantID` is extracted from the raw `tenant_id` claim into a **top-level indexed column**
  (`application/audit/event.go:52-60`: *"Surfaced as a top-level column in audit_events so
  per-tenant retention and filter queries stay on an indexed path"*). Listing it in
  `auditClaims` would put a second copy inside the `actorClaims` JSON blob, which is strictly
  worse to query than the column.

So the allowlist is exactly the two values that have **no other vehicle**:

```yaml
auth:
  auditClaims: [tenant_workspace, email]
```

`tenant_workspace` because no other service has the `tenants` table to resolve the UUID, and
`email` because `Actor` on its own is a UUID.

**The tenant claim must literally be named `tenant_id`.** The same doc comment records that a
customized `auth.authorization.tenant.claim` is *not* honored by audit — the column simply
stays empty for a service that diverges from the default. The plan uses the default name; this
is why.

### The must-change-password token — a restricted session

**Decided.** A user whose row carries `MustChangePassword` still authenticates, and still gets
a token — but that token's `permissions` claim carries **only** `user:change-password`, and
only if the user's own resolved bundle actually contains it. Everything else is dropped,
`*:*` included.

The consequence is the point: a session that exists solely to rotate an expired credential
cannot read a user, cannot list a tenant, cannot do anything but the one thing it is for. The
claim `mustChangePassword: true` tells the client to route straight to the change screen.

Two edges, both named rather than discovered later:

- If the user's bundle does NOT contain `user:change-password`, the token ships with
  `permissions: []` and the account is a dead end until a helpdesk reset. That is the honest
  fail-closed reading — inventing the permission would hand out a grant nobody issued — and it
  is why `user:change-password` belongs in whatever role a tenant grants by default (the
  existing route file already says so).
- **The same restriction applies on refresh.** `RedeemRefreshToken` takes claims fresh from
  the caller at redemption time, so the handler re-reads the row and re-applies the rule. A
  restricted token must not launder itself into a full one by refreshing.

### Resolving the permission set — one query, not a walk

Two grant paths must be unioned:
`user_roles → role_permissions → permissions` and
`user_groups → group_roles → role_permissions → permissions`, every hop archive-gated.

- **Proposed:** ONE `SELECT DISTINCT` over the neutral read seam (`repo.Engine.Querier()`),
  composed at runtime from the five `TableSchema` declarations — `Table()`, `ColumnOf(...)`,
  `IDColumn()`, `ParentIDColumn()` and `Resolve("ArchivedAt")` are all public
  (`infra/db/core/table_schema.go:845-1010`) — with `Dialect().Placeholder(n)`,
  `QuoteIdent(...)` and `EncodeArg(...)` for the engine-specific bits. No identifier is
  hardcoded, so a schema rename moves the statement with it, and the query is dialect-neutral
  rather than Postgres-only. One round trip. Every index it needs already exists
  (`user_roles_parent_idx`, `user_groups_parent_idx`, `group_roles_parent_idx`,
  `role_permissions_parent_idx`, plus the primary keys).
- **Alternative rejected:** reusing the memoised `roleRow`/`groupRow` walk that
  `internal/infra/user_service_manual.go` already carries. It is correct and already written —
  but it costs `1 + G + R` aggregate loads (each root plus its children), so a user in 3 groups
  of 4 roles plus 2 direct grants pays ~18 aggregate loads on the hottest security path in the
  platform. Those probes exist to guard a WRITE, where one extra read is irrelevant; a token
  endpoint is a different budget. The two must not drift, so the new reader becomes the single
  owner of "the effective permission set of a user", and nothing else re-derives it.
- The user's own row, the group keys and the direct role keys come free from the aggregate
  load: `UserRepository` already declares read joins filling `GroupKey`/`GroupName` on every
  `UserGroup` entry and `RoleKey`/`RoleName` on every `UserRole` entry
  (`internal/infra/user_repository.go`). Roles reached THROUGH a group are not on that
  aggregate and are projected by the same resolution query.

### Refresh-token storage

**Decided: a dedicated table plus a hand-written store.** A new migration pair creates
`authentication_refresh_tokens`; a small type in `internal/infra` implements the four port
methods over the same neutral seam (`Querier.Query` for `Lookup`, `core.Exec` for
`Save`/`MarkUsed`/`RevokeFamily`). Only the SHA-256 hash of the opaque value ever reaches it —
never the raw secret, never claims.

The alternative — modelling it as a full omnicore aggregate — was declined: it would buy audit
trail, notifications, archive semantics and a REST surface, none of which a refresh token
wants, and the REST surface in particular would be a credential-exfiltration endpoint. The
cost accepted is that these writes sit outside the framework's write guarantees, which for an
append-and-mark table with no invariants is the right trade.

Row shape: `hash` (PK), `family_id`, `subject`, `audience`, `expires_at`, `used`, `revoked`,
`created_at`. Indexes on `family_id` (reuse revocation sweeps a family) and on `expires_at`
(the self-cleaning sweep below rides it).

**The table cleans itself; no scheduled job.** Every successful redemption runs one bounded
`DELETE` over rows already past `expires_at`, on the `expires_at` index — an index range scan
of exactly the dead rows, never a table scan, and small after the first pass. Three properties
make it safe to hang off the request path:

- it runs AFTER the redemption has succeeded, and **its failure is swallowed and logged**,
  never propagated: a cleanup error must not fail a refresh the user legitimately earned (the
  same posture the framework takes for domain-event publishing);
- it deletes only rows whose expiry is already past a grace margin, so it never races a token
  expiring mid-flight;
- it is bounded per call, so a first run against a large backlog spreads across several
  refreshes instead of holding one request open.

The residue this leaves is honest: rows belonging to users who never refresh again are removed
only when *somebody* refreshes. Since the sweep is global rather than per-subject, an active
system drains them anyway; a system with no refresh traffic at all has no growth to drain.

### The lockout write on the login path — what round 2 must respect

Round 1 owns the columns and the rules; this round calls them. Three consequences of putting a
counter on the `users` row, all of which shape the handler:

- **A failed login becomes a write.** Through the aggregate it is a full write pass: audit
  event, revision bump, the works. That is the price of keeping it inside the framework's
  guarantees, and it is the right side of the trade for a security counter.
- **The revision guard can 409.** Every root update pins the loaded revision in its own
  `WHERE` (`lifecycle-map.html`); parallel failed attempts against one account will collide.
  **The outcome of the lockout write must never change the response** — a collision, a
  conflict, any failure of the counter still answers the same generic 401. The counter is
  best-effort bookkeeping around a refusal that was already decided.
- **The happy path stays read-only when it can.** On a successful login the reset write fires
  only if there is something to reset (`FailedLoginAttempts > 0 || LockedUntil != nil`), so an
  ordinary login remains reads plus one refresh-token INSERT.

### Wire/API impact

Two brand-new public contracts (`POST /auth/user/token`, `POST
/auth/user/token/refresh`) plus, when the `jwks:` block is declared, one
framework-mounted public `GET /.well-known/jwks.json`. Nothing existing changes shape.
**`auth.issuer.selfUrl` must equal `auth.jwt.issuer` in the prd profile** — boot-enforced
(`bootstrap/auth_config.go:348`): authcore validates its own tokens and cannot disagree with
itself about who it is. And `Doc.Public: true` does NOT bypass the auth middleware — it only
satisfies the authorization scan; the bypass comes from `auth.publicRoutes`, exact match, or
both routes answer 401 in prd before the handler ever runs.

## §4 External contract (integrations only)

`N/A — no external system.` The service becomes the issuer; nothing outbound is called. The one
contract it publishes is the JWKS document, which the framework mounts and owns.

## §5 Impact map — every artifact touched

**AS BUILT.** This section was rewritten on 2026-08-26 after the work shipped, because a plan
that still describes what was intended is a plan nobody can review against the code. What
changed while building is listed under *Deviations* below rather than quietly edited away.

**Round 1 (`/omnicore:evolve-entity`, the lockout counter) was NOT built.** The order was
inverted at the maintainer's request so the endpoints could be exercised first, and the
lockout is additive: the sign-in works without it, it simply cannot answer 429 yet. Every row
below shipped without depending on it.

| Artifact | Change | Owning doc section |
|---|---|---|
| `microservice.dev.yaml` | new `auth.issuer:` block (enabled, `selfUrl`, `audience`, TTLs, one `current` RS256 key via `${JWT_SIGNING_KEY}`/`${JWT_SIGNING_KID}`, `jwks:` sub-block) — **and `auth.mode` changed from `disabled` to `jwt`**, with `authorization.enabled: true` and a required tenant claim. See Deviations #1 | `token-issuance.html`, `yaml-reference.html` |
| `microservice.prd.yaml` | same `auth.issuer:` block with bare `${VARS}` and the retiring key commented in place for a rotation; `auth.jwt.issuer` == `auth.issuer.selfUrl` from ONE variable (boot-enforced equality); `authorization` block; `auth.publicRoutes` gains `POST /auth/user/token` and `POST /auth/user/token/refresh` | `shared/boot-contract.md`, `token-issuance.html` |
| `auth.auditClaims` in BOTH profiles | `[tenant_workspace, email]` — the two identity values with no other vehicle; `Actor` and `TenantID` are already top-level `AuditEvent` fields, and authorization state (`groups`/`roles`) is deliberately absent | `audit.html`, `yaml-reference.html` |
| infra prerequisite | `N/A — token issuance is posture-independent` | `shared/capabilities.md` |
| build/run commands | `N/A — unchanged` (`-tags postgres`; no transport tag involved) | `shared/boot-contract.md` (Build tags) |
| `migrations/postgres/0006_refresh_tokens_manual.{up,down}.sql` | the refresh-token table + its two indexes | `migrations.html` |
| `migrations/postgres/0007_authentication_attempts_manual.{up,down}.sql` | the append-only attempt log + its two indexes. NOT swept, unlike 0006 — the failures are the evidence | `migrations.html` |
| `internal/infra/authentication_attempts_manual.go` | the log's adapter: three named recording methods and the windowed lockout probe, over the same narrow `SQLSeam` | — |
| `internal/application/commands/notifications_manual.go` | **NEW, not in the original map.** `InvalidCredentialsNotification` and `AccountTemporarilyLockedNotification`, in the APPLICATION layer with `ApplicationNotificationBase` — these endpoints dispatch no rule and touch no aggregate, so nothing in the domain raises them | `status-mapping.html` |
| `internal/domain/` | **NOTHING.** Two types were written here and DELETED — see Deviations #8 | — |
| `internal/application/translations/{ptbr,eng,esp,fra,deu,ita,nld}.go` | `InvalidCredentialsNotification` + the `Authentication` context label, in all seven | — |
| `internal/application/commands/authentication_commands_manual.go` | `IssueTokenCommand`/`Handler`, `RefreshTokenCommand`/`Handler`, and THREE application-owned ports: `AuthenticationStore`, `RefreshTokenLookup` and `TokenIssuer` | `custom-command-handler.html` |
| `internal/infra/authentication_reader_manual.go` | the schema-composed resolution statement (built at construction, panics on an unresolved field), the by-e-mail and by-id loads, and the timing-equalisation decoy | `table-schema.html`, `relational-view.html` (why not a view) |
| `internal/infra/refresh_token_store_manual.go` | `authcore.RefreshTokenStore` over a narrowed `SQLSeam`, the bounded self-cleaning sweep, and `SubjectForRefreshToken` | `token-issuance.html` |
| `internal/web/requests/authentication_requests_manual.go` | `IssueTokenRequest`, `RefreshTokenRequest`, and the shared `TokenResponse` | `auto-handlers.html` (DTO conventions) |
| `internal/web/authentication_routes_manual.go` | `MountAuthentication` — `POST /auth/user/token` and `POST /auth/user/token/refresh`, tag `Auth`, `Doc.Public: true` | `openapi.html` |
| `bootstrap/authentication_feature_manual.go` | the feature; builds and OWNS both adapters, and exposes the store through a getter so `Wire` forwards rather than constructs | `bootstrap.html` |
| `bootstrap/wire.go` | register the feature and forward `Wiring.RefreshTokenStore` from it | `bootstrap.html` |
| `start.sh` / `start.cmd` / `start.ps1` | generate the dev signing key on first run and export it with newlines as literal `\n`, all three in step | §6 |
| `.gitignore` | **NEW, not in the original map.** `/devops/dev-signing-key.pem` — a generated key must never be committed | — |
| tests | 4 new suites: the handler branches (`commands`, → 97.9%), the statement composition and the timing decoy (`infra`), the store's shapes and the hash mirror (`infra`), the DTO projections (`requests`, → 100%) | — |

### Deviations from the approved plan

Every one of these was a decision taken while building, and each is listed because a reviewer
comparing the plan to the diff would otherwise have to work out which were deliberate.

1. **`auth.mode` in dev went from `disabled` to `jwt`, with the permission gate on.** The plan
   kept the bench open. The maintainer closed it mid-build, and the reasoning is now in the
   yaml: every row-scope guard stands down when no identity is present, so an open bench was
   not exercising the guards that matter most. This also forced the JWKS self-reference, which
   resolves itself — documented in the profile and proven in §7b.
2. **The order of the two rounds was inverted, and then round 1 was redesigned out of
   existence.** Endpoints first, lockout second — and when the lockout's turn came, keying it by
   the attempted identity removed the need to touch the `User` aggregate at all. The three
   columns were never added; there was no `/omnicore:evolve-entity` round. Both halves ship.
3. **`RefreshTokenLookup` and `SubjectForRefreshToken` did not exist in the plan.** They are
   forced by a framework ordering the plan did not anticipate: `RedeemRefreshToken` takes the
   claim map BY VALUE and only then looks up the record, so the handler must learn the subject
   before the framework would tell it. The lookup decides nothing — revoked, used and expired
   all stay the Issuer's call.
4. **The sweep hangs off `Save`, not off redemption.** `Save` is called by BOTH the sign-in and
   the rotation, so every event that adds a row also gets a chance to remove dead ones. Strictly
   better than what was planned, same cost.
5. **The store depends on a narrowed `SQLSeam`, not on `core.RelationalEngine`.** It uses two
   methods; taking the whole engine put its typed write verbs and the rebuild lock in reach of
   a component with no aggregate to write. Narrowing also cut the test fake from eleven methods
   to two.
6. **Paths and tag were shortened after the first boot** — `/authentication/*` → `/auth/*`,
   tag `Authentication` → `Auth`, at the maintainer's request. Re-verified against a live boot,
   including the exact-match `publicRoutes` validation that would have aborted it.
7. **No tenant predicate in the resolution query, and none tested.** The plan said the same;
   recorded here because "tenant isolation" appeared in the planned test list and is
   deliberately absent from the delivered one. Every write path already refuses a foreign
   tenant and both owning tenants are immutable, so no row can exist for such a filter to catch
   — adding one would hide a data problem rather than guard against it.
8. **Two types were invented in the domain and then deleted.** `EffectiveGrants` and an
   `AuthenticationAttempt` row shape were placed in `internal/domain/` because infra produces
   them and the application consumes them, and the framework's dependency rules leave no other
   package both may import (`infra` may reach `domain` and `application/persistence`, nothing
   else of `application/*`). The maintainer rejected it, correctly: neither had an identity, a
   rule or an `IsValid`, so neither was an entity or a value object, and a row shape carrying an
   IP is not a domain concept whatever the import graph says. The resolution was to stop having
   the types — the reader returns two slices, and the recorder takes primitives through three
   methods NAMED for the outcome, which also removed the shared `failure`/`success`/`locked`
   vocabulary. **The pressure is real and will return at the next port**: the rules genuinely
   leave no home for a shared type, and the answer is that there should not be one.
9. **The two notifications moved from the domain to the application.** They were written into
   `internal/domain/notifications.go` beside the credential ones, which was filing by
   resemblance: the credential routes' notifications are raised by the AGGREGATE from inside
   its rules, while these are raised by a handler. These endpoints dispatch no rule, call no
   `GetUpdatable` and write no aggregate. They now embed `ApplicationNotificationBase` and
   travel in an `*exception.ApplicationError`, and a test asserts a refusal is NOT a
   `*domain.DomainError` so nobody re-routes it through the domain by accident.
10. **Absence and outage are recorded differently.** `FindUserByEmail` answers `(nil, nil)` for
    an unknown address and `(nil, err)` for a lookup that could not run. The CALLER refuses both
    identically — it must, or the status code becomes an oracle — but the log keeps `false` for
    the first and NULL for the second, which is the difference between "credential stuffing
    against addresses that are not here" and "an attack during an outage".

## §6 Config & secrets

`JWT_SIGNING_KEY` — a PKCS#8 RSA private key PEM (≥2048 bits, enforced at construction),
supplied by `${JWT_SIGNING_KEY}` substitution in both profiles. Never a literal PEM in the
yaml: interpolation runs on raw file text before parsing, so the framework cannot tell a
literal from a substitution and will not catch the mistake.

**The dev bench key — decided.** `prd` uses a bare `${JWT_SIGNING_KEY}` with no default, like
every other prd endpoint. `dev` gets its key from `start.sh`, which generates a PKCS#8 RSA
keypair into the environment on first run and reuses it afterwards. No key material in git,
and each developer's bench signs with its own. The three start wrappers (`start.sh`,
`start.cmd`, `start.ps1`) must stay in step, or the Windows benches boot without a key and the
issuer refuses to build.

Key rotation stays the documented config-plus-redeploy runbook (`next` → wait one propagation
window ≥5 min → promote to `current`, old `current` → `previous` → drop `previous` after the
longest access-token TTL). Nothing schedules it and nothing here needs it on day one, but the
yaml is shaped as a LIST from the start so a rotation is an edit rather than a restructure.

## §7b What was actually proven (2026-08-26)

Executed against the dev bench with a user inserted by hand in SQL — no seed, no
fixture, no bypass. `go build`/`go vet`/`go test -tags postgres ./...` all clean.

| Claim | Result |
|---|---|
| Both grant paths resolve in one query | `permissions: [tenant:update, user:change-password, user:read]` — `tenant:update` reached only through `engineering → tenant-admin`, the other two direct through `viewer` |
| The archive gate holds | a `retired` role, granted DIRECTLY and carrying `tenant:update`, contributed nothing and is absent from `roles` |
| A minted token validates in-process | `GET /users` answers 401 bare and 200 with the token — the self-referential JWKS resolved on the first authenticated request, exactly as the boot WARN predicted |
| Every refusal is identical | unknown e-mail, wrong password, unknown refresh value and empty refresh value all answer 401 / `InvalidCredentialsNotification` / same message |
| The refusal is translated | `Accept-Language: pt-BR` → "E-mail ou senha inválidos."; `de-DE` → "E-Mail-Adresse oder Passwort ungültig." |
| Timing is equalised | unknown e-mail 19.1 ms vs wrong password 18.6 ms over 5 samples each (without the burn the first would be ~1 ms) |
| Rotation works | a refresh redeems into a NEW pair with claims rebuilt from the database |
| Reuse kills the family | replaying a redeemed value answers 401, and the descendant minted moments earlier is dead too — visible in the table as two rows of one family, `used=t/revoked=t` and `used=f/revoked=t` |
| The restricted session is real | with `must_change_password`, the claim set is exactly `[user:change-password]` — `tenant:update` and `user:read` dropped — and `GET /users` answers **403** |
| The audit allowlist works | an audited write left `actorClaims: {email, tenant_workspace}` beside the top-level `actor` and `tenantId` UUIDs. This is the whole `auditClaims` argument, demonstrated: without `tenant_workspace` that `tenantId` names nobody in a service with no `tenants` table |
| The lockout locks | five wrong passwords answer 401; the SIXTH answers **429** naming the remaining window, and a CORRECT password inside that window still answers 429 |
| **The lockout is not an oracle** | the identical sequence against an address that names NO account produced the identical five 401s and the identical 429, same message, same key. This is the whole reason the counter is not a column on `users` |
| The success anchor resets the window | a user with 3 failures, then a success, then 4 more failures was still admitted — 8 failures total, only the 5 after the success counted, and the lock fell exactly there |
| The forensic split works | the log separates a real account from an address that is not here, with NULL reserved for "nothing established it" |
| **A locked row says which it was** | `WHERE outcome = 'locked'` ALONE answers it: a locked real account shows `true`, a locked unknown address `false`. The verdict rides back from the lockout probe, which read those rows anyway — no extra query, and no lookup on the path whose whole point is to stop paying for guesses |
| The service drains | SIGTERM → `draining`/`drained` per stage → `shutdown complete`, port released |

**Still unproven, and unprovable here:** that a SECOND service accepts the token.
That needs a second service running; the exact yaml is in §7 item 5.

## §7 Verify step — how this will be PROVEN

1. `go build -tags postgres ./... && go vet -tags postgres ./...` clean.
2. `go test -tags postgres ./... -count=1` green, coverage ≥ 95%.
3. Boot the dev profile against the local Postgres and drive the real routes:
   - a correct e-mail + password answers 200 with an access token, an `expiresAt` and a
     refresh token; decoding the access token shows `sub`, `tenant_id` and a `permissions`
     array whose contents match a hand-checked expectation for a seeded user holding
     permissions through BOTH paths (one direct role, one group role) with no duplicates;
   - a user with `mustChangePassword` gets a token whose `permissions` is exactly
     `["user:change-password"]`, and refreshing it does not widen that set;
   - a wrong password, an unknown e-mail, a suspended user, a user in a suspended tenant and a
     replayed refresh token each answer **401** with the *same* body, and the message changes
     with `Accept-Language: pt-BR` — that is the "translatable notification" requirement,
     proven rather than assumed;
   - *(round 1, not yet runnable)* the sixth consecutive wrong password answers **429**
     naming the remaining window, and a CORRECT password inside that window still answers 429
     until it expires — then succeeds;
   - a redemption leaves no expired rows behind in `authentication_refresh_tokens`;
   - `GET /.well-known/jwks.json` answers 200 without a bearer.
   - a write performed WITH the minted token produces an `audit_events` row whose
     `actorClaims` block carries `tenant_workspace` and `email`, and whose top-level `actor`
     and `tenant_id` columns are filled — proving the allowlist half, which is the one that
     fails silently.
4. The round trip: a `Validator` built from that JWKS accepts the minted token — the
   framework's own criterion for this feature ("issue and validate are two independent
   surfaces"). Proven in a test, not by inspection.
5. What CANNOT be proven here: that a SECOND service accepts the token. That is configuration
   in the consumer and needs a second service running. The exact hand-off, for whoever wires
   the first one:

   ```yaml
   auth:
     mode: jwt
     jwt:
       algorithms: [RS256]
       issuer:   "<authcore's auth.issuer.selfUrl, verbatim>"
       audience: "<that service's own entry in authcore's auth.issuer.audience list>"
       jwksUrl:  "<selfUrl>/.well-known/jwks.json"
     auditClaims: [tenant_workspace, email]
   ```

   The `auditClaims` line is not optional boilerplate: omit it and that service's audit trail
   records the tenant as a bare UUID it has no table to resolve — the exact failure this
   plan's claim set exists to prevent.
