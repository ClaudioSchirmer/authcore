# Capability plan — client-credentials-token

- **Status:** APPROVED (2026-09-01)
- **Framework pin:** `github.com/ClaudioSchirmer/omnicore v0.68.0` — the latest published
  release (checked against proxy.golang.org: `v0.68.0`, 2026-09-01). No upgrade is offered
  because none exists. That pin's docs are the authority for every line below.

## §1 The request (restated)

> vamos implementar o irmão do user/token, para o client, seguindo a mesma ideia de
> permissões e de endpoint, mas agora com o kind: client e informações relevantes de
> client, ao inves de user.

When this is done, `POST /auth/client/token` exists and authcore authenticates **machines**
as well as people. A client id and a server-minted secret are exchanged for one signed RS256
access token whose claims carry `identity_kind: "client"`, the client's own label, its tenant,
its effective permission set across `client_roles → roles → role_permissions → permissions`,
and the tenant-defined `x_` claims the `ClientClaim` collection has been holding since
2026-08-28 with no token to reach. **No refresh token is issued** — the secret is already the
long-lived credential (RFC 6749 §4.4.3), so `POST /auth/client/token/refresh` does not exist
and will not. A failed exchange answers one generic, translated 401 exactly as the user route
does; the lockout counters and the log stream receive the attempt under
`identity_kind = 'client'`, on their own row, through the journal that was built for this.

The contract is not invented here. `specs/scaffold-entity/client/spec.md` §F wrote it down
when the entity shipped, `backlog.md` §*Not started: `POST /auth/client/token`* repeats it,
and `README.md` states it publicly. This plan honours that contract and names the three places
where reality at this pin differs from what §F assumed.

## §2 Routing evidence — the owning docs

| Capability piece | Owning section(s) at this pin | Existence check |
|---|---|---|
| Minting one access token with no refresh companion — `Issuer.Issue(ctx, TokenRequest) (IssuedToken, error)` | `token-issuance.html` — the Issuer's four public methods; *"`Issue`/`IssueWithRefresh` mint for a subject the service already decided is authentic"* | Documentation Map row *Token issuance*; `features.html` |
| The claim vocabulary is ours, and only `iss/aud/exp/iat/nbf/jti` are reserved | `token-issuance.html` — *"`TokenRequest.Claims` is where groups, permissions, `tenant_id`, or any other RBAC shape ride — the framework never interprets it"*; *"Reserved claims … supplying one in `Claims` is a rejected call"* | same row |
| The route itself is ours to build — the framework ships methods, never endpoints | `token-issuance.html` — *"`POST /auth/login`, `POST /auth/refresh`, an introspection endpoint — all of it is built by the consuming service"*; *"What stays in the consumer service: … lockout and rate limiting … and every HTTP route"* | same row |
| Reading the client, its grants, its claim values and its allow-list without loading the aggregate | `direct-schema.html` (*THE READ IS UNRESTRICTED*) + `read-joins.html` (traversal rules, identical on both anchors) + `table-schema.html` (`AsDirectSchema()`, the join-target reduction) | Documentation Map rows *Direct schema*, *Read joins*, *TableSchema* |
| Which primitive answers each question (rows to walk → `FindAll`; the scope gate drops archived rows) | `shared/query-primitives.md` | owner file, no version gate |
| The refusal is an APPLICATION notification, declared beside the handler | `shared/notification-bases.md` — `ApplicationNotificationBase` is *"a hand-written command/query handler"*, declared in `internal/application/` | owner file; the precedent is `commands/notifications_manual.go` |
| 401 / 429 from the notification's `Semantic()` | `status-mapping.html` | Documentation Map row *Status mapping* |
| A new public route must be listed exact-match in `auth.publicRoutes` of every booting profile | `shared/boot-contract.md` — *"validated at boot against the registered route set, exact-match … a typo, wrong method, or trailing slash … ABORTS boot"* | owner file |
| Where each new file lands | `service-layout.html` | Documentation Map row *Service layout* |
| **The honest no — trusted proxy.** `http:` carries `addr · requestTimeoutSeconds · bodyLimitBytes · readTimeoutSeconds · idleTimeoutSeconds · accessLog` and nothing else; the framework builds `fiber.Config` with `AppName`, `ErrorHandler` and those three knobs, and sets neither `TrustProxy` nor `ProxyHeader` | `yaml-reference.html` §*Core — service · http · …* (the absence IS the answer); `bootstrap/bootstrap.go:761-778` at the pin; `gofiber/fiber/v3@v3.3.0/req.go:543` — `IP()` reads a header only when `IsProxyTrusted() && ProxyHeader != ""` | **not offered at this pin** — see §3 |

**Routing outcome: offered at pin**, for every piece the route is built from. The one thing
that is **not offered** is a trusted-proxy configuration, and §3 states what that changes —
it does not block the route, it bounds what the allow-list can mean.

## §3 Integration semantics [high-risk — propose + CONFIRM]

- **Seam:** a hand-written `pipeline.Handler` mounted through `fwweb.CommandWithBodySpec` +
  `fwopenapi.Mount`, exactly as the two user routes are *(proposed)*. The alternative the
  framework's own example uses — `MountRaw` with a bare Fiber closure — is refused for the
  reason `authentication_routes_manual.go` already records: a bare status is untranslated,
  and it would cost the canonical envelope, the seven catalogs, the AppContext and the
  DTO-derived OpenAPI schema.
- **Where it mounts:** inside the existing `MountAuthentication`, on the same `app.Group("/auth")`
  *(proposed)*. Not a new feature and not a second group: the origin-address middleware is
  registered on that group object, and a route mounted from another feature under the same
  prefix depends on registration order for that middleware to run at all. One owner of the
  `/auth` group is what makes the IP reach this handler deterministically. `AuthenticationFeature`
  therefore grows one adapter (the client reader) and passes it through.
- **Sync or async:** fully synchronous inside the request. Nothing here is deferrable — the
  answer IS the token.
- **Failure policy:** every refusal is the same generic 401 through one new notification;
  every INFRASTRUCTURE failure (the client read, the grant burst, the counter write, the
  Issuer) escapes as an exception → 500, never as a 401. That asymmetry is the user route's
  and it is load-bearing: answering 401 on a store failure tells a caller holding a correct
  secret that it was wrong, and buries an outage inside a credential problem.
- **Idempotency / replay:** `N/A` — the operation mints a new token per call by construction;
  there is nothing to deduplicate. A replayed body is a second legitimate sign-in.
- **Cache slots:** `N/A — nothing here is cached.` Grants and claims are re-read on every
  mint, which is the property that makes a revoked permission reach the mesh at the next
  token rather than at a TTL boundary.
- **Wire/API impact:** ONE new public route and ONE new public response shape. Nothing
  existing changes shape. **Two things do change behaviour elsewhere, and both are intended:**
  1. `Client`'s two dormant row rules WAKE UP. `refuseForeignClientCaller` reads
     `RequestingIdentityKind`, which until now was always `""` because nothing minted the
     claim (`internal/domain/client_rules_manual.go:292-304`, `ACCESS_MATRIX.md` — *"`dormant`
     means nothing reaches it yet"*). From the first client token onward, a client-subject
     caller can no longer rotate a sibling client's secret. That is the rule doing its job.
  2. `auth.auditClaims` gains two entries (§6), so every audit row — user rows included —
     starts carrying `name` and `identity_kind`.

### ✅ DECIDED 1 (2026-09-01) — the lockout COUNTS but never LOCKS on this route

**Answer: B.** `RecordFailure` / `RecordSuccess` write exactly as they do on the user route,
so the counters, the lifetime totals and the log stream keep every attempt; `LockedUntil` is
**not consulted** by the client handler and `RecordLocked` is never reachable from it. This
overturns §F's *"makes the existing lockout apply unchanged"*, knowingly, on the argument
below.

### The question that was asked

**§F says yes** (*"`identity_kind = 'client'` on every `authentication_attempts` row, which
makes the existing lockout apply unchanged"*), and the table was built for it — `identity` is
`VARCHAR(320)` and the migration says outright *"it is `identity` and not `email` because the
same table serves the coming client-credentials route"*.

**The argument for looking again before shipping it.** The lockout is 5 failures in 15 minutes
(`infra.LockoutThreshold`, `infra.LockoutWindow`). It exists because a human password carries
perhaps 30 bits and guessing has to be made expensive. A client secret is `acs_` + 32 bytes
from `crypto/rand` — 256 bits — so no rate at any budget guesses it, and the lockout buys
nothing against the attack it was designed for. What it does buy is a lever: **a client id is
not a secret.** It is the row id, it is the `sub` of every token that client presents, and it
appears in `GET /clients`. Anyone who knows one can send five wrong secrets and take a
production integration off the air for fifteen minutes, repeatable indefinitely — a one-request
outage against an unguessable credential.

Three answers, and the criterion is *what is the lockout protecting here*:

| | What happens | Cost |
|---|---|---|
| **A — apply it, as §F wrote** | identical to the user route | the DoS lever above is real and cheap |
| **B — count, never lock** ← **CHOSEN** | `RecordFailure`/`RecordSuccess` write exactly as today, `LockedUntil` is never consulted on this route | the counters stay a forensic and alerting surface; nothing an attacker sends can refuse a valid secret. Loses: a brute-force attempt is not throttled — which against 256 bits costs nothing to lose |
| **C — apply it with a higher ceiling** | a separate threshold for `client` | keeps a throttle and makes the outage more expensive, without removing it; two policies to explain |

I propose **B**, and I flag it rather than deciding it because it contradicts a line the
`Client` spec already approved.

### ✅ DECIDED 2 (2026-09-01) — enforce, document the deployment truth, and ask omnicore for the knob

**Answer: B1 + B2.** The allow-list starts being honoured against the socket peer this run;
the limitation is written into the OpenAPI description, the README and the ACCESS_MATRIX; and
a framework feature request for `http.trustedProxies` + `http.proxyHeader` is raised so a
later pin can make the read truer with no change here. Reading `X-Forwarded-For` inside this
handler stays refused.

### The question that was asked

The `allowedCIDRs` collection has been enforced by nothing since it shipped
(`README.md` — *"Nothing enforces the list yet: it is read by no code until
`POST /auth/client/token` exists"*). This run is where it starts meaning something, and §F's
prerequisite 1 has to be restated because **it describes a risk this pin does not have, and
misses the one it does**:

> §F: *"`X-Forwarded-For` unguarded is spoofable — an attacker sets the header to an allowed
> range and walks through."*

At this pin that cannot happen. `fiber.Config.ProxyHeader` is never set by the framework, and
Fiber v3 reads a proxy header only when `IsProxyTrusted() && ProxyHeader != ""`
(`req.go:543`), so `c.IP()` is **always the socket peer**. No header this service receives can
influence it. Not spoofable — fail-closed.

The real consequence is the other half: **behind an ingress or a load balancer every request
carries the balancer's address**, so an allow-list of real egress ranges refuses everyone, and
an allow-list containing the balancer's range allows everyone. The framework offers no knob to
fix this (§2, last row), so the honest paths are:

| | |
|---|---|
| **B1 — enforce, and document the deployment truth** ← **CHOSEN** | the list is honoured against the socket peer. Correct when authcore is reached directly (today's bench, and a mesh where it is dialled pod-to-pod). Behind an L7 proxy the operator must either leave the list empty or list the proxy's range and know it means "any caller through this proxy". Said in the OpenAPI description, the README and the ACCESS_MATRIX, not only here |
| **B2 — enforce, and ask omnicore for the knob** ← **CHOSEN, as the follow-up** | same code, plus a framework feature request for `http.trustedProxies` + `http.proxyHeader`. Reversible: the day it lands, this route reads a truer IP with no change here |
| **B3 — do not enforce yet** | the collection stays decorative for another release. Refused as a proposal: it leaves a documented security control that does nothing, which is worse than one whose limits are written down |

I propose **B1 now and B2 as a follow-up**, which is `/omnicore:implement`'s "honest no" path:
the framework does not offer it, the closest legitimate path is a framework feature request,
and nothing is hand-rolled inside the service to fake it. **Reading `X-Forwarded-For` in this
handler is explicitly refused** — that is exactly the spoofable design §F warned about, and
building it here would be reimplementing, badly, something the framework should own.

Two edges inside B1, decided rather than asked (say so if either is wrong):
- **An empty collection means any address.** The README already fixed this as fail-open by
  design, and `vos.CIDRBlock` refuses `0.0.0.0/0` and `::/0` so there is exactly one spelling
  of "no restriction".
- **A non-empty collection with no usable origin address refuses.** `c.IP()` returning `""` is
  not a hole to wave through when the operator has stated a restriction.

### ✅ DECIDED 3 (2026-09-01) — the same generic 401, reason on the log stream only

**Answer: the generic refusal.** No `ClientNotAllowedFromThisNetworkNotification` is declared.

**The same generic 401 as every other refusal**, with the reason on the log stream
only. It is the user route's doctrine and it holds for the same reason: a distinct message
tells the holder of a stolen secret that the secret is good and only the network is wrong,
which is the one thing worth confirming to them.

The counter-argument is real and it is the one that won on the lockout: an integration whose
egress IP changed gets a generic 401 with no clue, and that is a support ticket. I take the
disclosure side because unlike the lockout — where *waiting* is a fix the caller can act on —
the fix here is a configuration change only the tenant's operator can make, and they have the
log line. It stayed overturnable and was not overturned: the operator's clue is the log line, which
carries the reason and the origin address.

### ✅ DECIDED 4 (2026-09-01) — the access token's lifetime is the one already configured

**`TokenRequest.TTL` is left at 0**, so the Issuer applies `auth.issuer.tokenTtlSeconds` —
900 seconds in both profiles, the same lifetime a user token gets. No new yaml key, no new
constant, no per-kind policy.

`TokenRequest.TTL` does exist (`web/authcore/issuer.go:400`, `0 = IssuerOptions.TokenTTL,
capped by MaxTokenTTL`), so a longer client lifetime up to `maxTokenTtlSeconds` (3600) was
reachable with no config change, and was weighed: it would cut re-authentications 4x. It
loses because **this service has no revocation of a token in flight** — `Wiring.TokenChecker`
is a framework seam and `bootstrap/wire.go` does not fill it — so until it does, the TTL IS
the revocation time. Fifteen minutes of a leaked machine credential beats an hour of it, and
a machine secret is exactly the one that leaks into a CI log and is used unnoticed.

Recorded so the absence of a knob reads as a decision: if re-authentication load ever becomes
the problem, raising `tokenTtlSeconds` is one line — and wiring `TokenChecker` is the real
answer rather than a shorter window.

### ✅ DECIDED 5 (2026-09-01) — the six field-level proposals of §5, as written

Approved without amendment: `clientId` + `clientSecret` on the body; a `ClientTokenResponse`
of its own rather than the user's; `auditClaims` += `name`, `identity_kind` in both profiles;
the route mounted inside the existing `MountAuthentication` on the one `/auth` group; a narrow
`AccessTokenIssuer` port carrying only `Issue`; and `resolveCustomClaims` generalised so both
token paths resolve the claim chain through one function.

## §4 External contract (integrations only)

`N/A — no external system.` Everything this route reads is in this service's own database,
and the only library seam is the framework's own `authcore.Issuer`.

## §5 Impact map — every artifact touched

| Artifact | Change | Owning doc section |
|---|---|---|
| `microservice.dev.yaml`, `microservice.prd.yaml` | `auth.publicRoutes` += `POST /auth/client/token`; `auth.auditClaims` += `name`, `identity_kind` | `shared/boot-contract.md`; `yaml-reference.html` (`auth`) |
| build/run commands | `N/A — unchanged` (`go build -tags 'postgres' ./...`, per `start.sh:50`; no transport, no new tag) | `shared/boot-contract.md` (Build tags) |
| `internal/infra/schemas/{sign_in_client,client_role_grant,held_client_claim_value,client_allowed_range}_schema.go` **(new)** | `SignInClient` on `clients`; `ClientRoleGrant` on `client_roles` + its three join targets (`roles`, `role_permissions` with `ID("role_id")`, `permissions`); `HeldClientClaimValue` on `client_claims`; `ClientAllowedRange` on `client_allowed_cidrs`. `ClaimDefinition` is REUSED from `authentication_read_schemas.go` | `direct-schema.html`, `table-schema.html`, `read-joins.html` |
| `internal/infra/schemas/client_authentication_read_schemas_test.go` **(new)** | each schema agrees with the generated schema of the same table; no repository is anchored on a join target | — |
| `internal/infra/client_authentication_reader.go` **(new)** | `ClientAuthenticationReader`: `LoadClientByID` (one statement, active row, live tenant via `criteria.Exists`), `ResolveClientSignIn` (four concurrent `FindAll`s → one round trip), `SecretMatches` (current, then previous inside its grace window), `BurnSecretVerification` | `direct-schema.html`, `criteria.html`, `shared/query-primitives.md` |
| `internal/application/commands/issue_client_token_command.go` **(new)** | `IssueClientTokenCommand` + `ClientTokenResult` + `AuthenticatedClientResult` — the Command and its Result stay one level ABOVE the handler | `service-layout.html` |
| `internal/application/commands/handlers/issue_client_token_command_handler.go` **(new)** | the handler itself, one file for one hand-written `pipeline.Handler` · `clientIsUsable` · `buildClientClaims` · `buildClientProfile` | `service-layout.html` |
| `internal/application/commands/handlers/utils/ports.go` | += `ClientAuthenticationStore`, `AccessTokenIssuer` | `service-layout.html`; `shared/domain-membership.md` |
| `internal/application/commands/handlers/notifications.go` | += `InvalidClientCredentialsNotification` — an application notification, declared beside the handler that raises it | `shared/notification-bases.md` |
| `internal/application/commands/handlers/utils/claims.go` | `resolveCustomClaims` is generalised to take `(mustRestrict bool, held map[domain.ID]string, definitions []schemas.ClaimDefinition)` instead of `*schemas.SignInAccount`, so both token paths resolve the chain through ONE function. `platformClaimNames` is unchanged and still guards all nine names on both paths | `shared/notification-bases.md` n/a; the file's own contract |
| `internal/application/commands/authentication_journal_manual.go` | no change — `identityKindClient` and the kind-bound journal were built for this run | — |
| ~~`internal/application/commands/notifications_manual.go`~~ (superseded by the row above) | += `InvalidClientCredentialsNotification` (embeds `domain.ApplicationNotificationBase`, `Semantic() → SemanticUnauthorized`). A separate type from `InvalidCredentialsNotification` because that one's message is *"Invalid e-mail or password"* in seven languages and a machine has neither. Two endpoints, two vocabularies — not an oracle, since the caller already chose the endpoint | `shared/notification-bases.md`, `status-mapping.html` |
| `internal/application/translations/{ptbr,eng,esp,fra,deu,ita,nld}.go` | one key each: `InvalidClientCredentialsNotification` | — |
| `internal/web/requests/issue_client_token.go` **(new)**, `requests/dtos/named_grant.go` | `IssueClientTokenRequest{clientId, clientSecret}` + `ToCommand`; `ClientTokenResponse` + `AuthenticatedClientResponse` + `FromResult` | `service-layout.html`, `openapi.html` |
| `internal/web/authentication_routes_manual.go` | `MountAuthentication` grows one parameter (the client store) and mounts `POST /auth/client/token` on the same group, `Doc.Public`, 200, with the OpenAPI description carrying the allow-list truth from §3 | `openapi.html` |
| `bootstrap/authentication_feature_manual.go` | builds `appinfra.NewClientAuthenticationReader(d.DB)` and passes it to `MountAuthentication` | `bootstrap.html` |
| `README.md` · `ACCESS_MATRIX.md` · `backlog.md` | the route moves from *planned* to *built*: API-shape block, the `Client` matrix (`dormant` → live), the two claim-catalog rows that say `ClientClaim` reaches no token, and the backlog entry retired with its outcome | — |
| tests | `client_authentication_commands_manual_test.go` (every refusal branch, the grace-window hit, the allow-list in/out/empty/no-IP, the claim set exactly, body ≡ token on permissions and claims), `client_authentication_reader_manual_test.go` + a live test beside the existing ones, `client_authentication_requests_manual_test.go`, and the seven-catalog completeness test already in place picks up the new key | — |

Phase 2 edits ONLY these rows, in dependency order: yaml → schemas → reader → application →
web → bootstrap → docs → tests.

### ⚠️ DEFECT IN THIS PLAN, found by the maintainer after the code shipped (2026-09-01)

**§2 cites `service-layout.html` as the authority for where each file lands, and the file
was never opened.** The paths above were taken from the surrounding code instead, which put
the handler at `commands/` root beside its Command — and the standard says a hand-written
`pipeline.Handler` goes one level down, in `commands/handlers/`, with only the Command and
its Result staying up. Citing evidence without reading it is worse than not knowing: the plan
looked sourced.

The rows above are the CORRECTED placement, applied after the fact. The same run also moved
the five handlers that predated it, so the service has one convention rather than two, and
the layout survey that followed is in this file's sibling directory listing — several
GENERATED shapes deviate too, and those are the generator's to fix, not this run's.

## §6 Config & secrets

No new secret and no new env var — the signing key is the one already configured.

```yaml
auth:
  auditClaims:
    - tenant_workspace
    - email
    - name             # NEW — the client's analogue of email; the user token already mints it
    - identity_kind    # NEW — so an audit row says person or machine without joining `clients`
  publicRoutes:
    - GET /livez
    - GET /readyz
    - POST /auth/user/token
    - POST /auth/user/token/refresh
    - POST /auth/client/token   # NEW — a sign-in cannot require the credential it grants
```

Both profiles, identically. `auditClaims` is §F's own prescription and its cost is stated
there: user audit rows gain a short `name` string beside the `email` they already carry.

**Not added:** `POST /auth/client/token/refresh`. There is none.

## §7 Verify step — how this will be PROVEN

1. `go build -tags 'postgres' ./... && go vet -tags 'postgres' ./...` clean.
2. `go test -tags 'postgres' ./... -count=1` green, coverage ≥ 95% on the new files.
3. **Against a booted service, on the dev bench** (`./start.sh`, Postgres up):
   - create a client, rotate its secret to get a plaintext;
   - `POST /auth/client/token` with it → **200**, and the decoded access token carries
     `identity_kind: "client"`, `sub` = the client id, `tenant_id`, `tenant_workspace`,
     `name`, `permissions`, `roles`, plus any `x_` claim the tenant defined for
     `appliesTo: client|both` — and carries **no** `email`, **no** `groups`, **no**
     `must_change_password`, and the response body carries **no** `refreshToken`;
   - that token is then accepted by this service's own middleware on an ordinary route the
     client's roles cover — which is the real proof, since the same token validates through
     the unmodified `Validator` from the published JWKS;
   - a wrong secret → **401** with the new translated message; the previous secret inside its
     grace window → **200**; the same one after `previousSecretExpiresAt` → **401**;
   - a suspended client, and a client whose tenant is suspended → **401**;
   - `authentication_attempts` shows the rows under `identity_kind = 'client'`, and the log
     stream carries one record per outcome;
   - with one `allowedCIDR` covering `127.0.0.1/32` → **200**; with one that does not → **401**
     and the reason on the stream; with the collection empty → **200**;
   - an audit row written by that token shows `name` and `identity_kind` in `actorClaims`.
4. **What cannot be proven locally:** the behaviour behind a real L7 proxy (§3, OPEN 2). The
   step that closes it is the operator's: deploy behind the actual ingress, call the route,
   and read `last_ip` on the attempt row — if it is the balancer's address, the allow-list on
   that deployment can only mean "through this balancer", and the framework feature request is
   what changes that.
