# Capability plan — emit-custom-claims-on-user-token

- **Status:** APPROVED
- **Framework pin:** `github.com/ClaudioSchirmer/omnicore v0.63.0` (the latest published release;
  no upgrade offered because none is available)

## §1 The request (restated)

> precisamos ajustar a implementação do auth/user, para incluir as claims personalizadas que
> agora podem vir de 2 níveis, direto pelas claims que são de tenant, ou ainda com valor
> personalizado no usuário. Veja backlog.md que eu acho que explica bem.

This is the **emission** step `backlog.md` lists under *“What is still open”* for the
`Claim` catalog: `buildClaims` currently mints a fixed set of nine names and nothing else, so
the two-level chain that was built on 2026-08-28 — `user_claims.value` first, then
`claims.default_value` of the definition’s tenant — can be filled, read and audited while
changing no token. When this run is done, `POST /auth/user/token` and
`POST /auth/user/token/refresh` resolve that chain and mint the result **beside** the fixed
nine, typed per the definition’s `valueType`, bounded by a size budget, and mirrored in the
response body so the body and the token cannot disagree. `POST /auth/client/token` does not
exist yet and is untouched.

## §2 Routing evidence — the owning docs

| Capability piece | Owning section(s) at this pin | Existence check |
|---|---|---|
| The claim map handed to the Issuer, and the fact the framework never interprets it | `token-issuance.html` — *“`TokenRequest.Claims` is where groups, permissions, `tenant_id`, or any other RBAC shape ride — the framework never interprets it”*; reserved claims are only `iss/aud/exp/iat/nbf/jti`, so **protecting the platform’s nine is this service’s job, not the Issuer’s** | Documentation Map row *“Token issuance”*; `features.html` |
| Claims rebuilt fresh on every rotation | `token-issuance.html` — *“`RedeemRefreshToken` takes claims fresh from the caller at redemption time … so a permission revoked between logins reaches the mesh on the next refresh”* | same row |
| Which read primitive answers “the tenant’s active claim definitions” | `shared/query-primitives.md` — *“give me the rows, I am going to walk them → `FindAll`”*, and *“Same … scope gate (active rows by default)”* | owner file, no version gate |
| Level 1 needs no query at all | `read-joins.html` (Documentation Map row) + `internal/infra/user_repository.go:83` — `InnerJoinInChild(UserClaim).To(Claim).On("claim_id").Field("ClaimName","name").Field("ClaimValueType","value_type")` | already declared in this service |
| Where the new port and its implementation land | `service-layout.html` — the port with its CONSUMER (`internal/application/commands/`), the adapter under `internal/infra/`; `shared/domain-membership.md` | NORMATIVE for placement |
| The criteria vocabulary used by the catalog read | `custom-command-handler.html` (“Loading by criteria” / `AggregateLoader`); `criteria.And`/`Eq`/`In` verified at `infra/db/criteria/builder.go:15,24,64` | verified at the pin |

**Routing outcome: offered at pin.** No `/omnicore:upgrade` and no `/omnicore:configure` —
the capability is a plain relational read plus a map handed to an Issuer this service already
holds. No new infra of any kind.

## §3 Integration semantics [high-risk — the four wire-visible picks were CONFIRMED]

- **Seam:** inside the two existing hand-written handlers (`IssueTokenHandler.Handle`,
  `RefreshTokenHandler.Handle`), synchronously, in the same place `ResolveGrants` already
  sits. Not a lifecycle hook and not middleware: nothing here is a side effect of a write, and
  the value is needed *before* the token is signed. Alternative weighed and rejected: resolving
  inside `AuthenticationReader.FindUserByEmail` — it would hide an authorization decision
  inside a loader and leave the refresh path to re-derive it separately.
- **Sync or async:** synchronous. One additional indexed `SELECT` per token operation, on
  `claims (tenant_id, …)` — the leading column of the existing `claims_tenant_id_name_key`
  index, so the filter is served by an index that already exists.
- **Failure policy:** the catalog read failing **propagates as an exception → 500**, exactly
  like `ResolveGrants` does today (`authentication_commands_manual.go:341`). It is *not* a
  credential refusal and *not* a counted attempt: the caller proved who they are and this
  service failed them, and counting it would let a database problem lock out the users it is
  already failing. Minting a token that is silently missing claims a consumer branches on is
  the alternative, and it is worse — a wrong answer instead of no answer.
- **Idempotency / replay:** N/A — no event, no external call, no write. The resolution is a
  pure function of rows read in the same request.
- **Cache slots:** N/A — nothing is cached. Caching the catalog would mean a corrected value
  reaching the mesh later than the refresh path promises.
- **Wire/API impact — four decisions, all confirmed by the maintainer on 2026-08-28:**

  1. **The value is TYPED on the wire.** `valueType: number` mints a JSON number,
     `bool` a JSON boolean, `string` a string. This is what makes the joined
     `ClaimValueType` mean something at emission. `valueType` is immutable
     (`value-type-immutable` in `specs/omnicore-gen/claim.omnicore.yaml`), so a claim’s JSON
     type never changes under a consumer. A stored value that does not parse — impossible
     through the API, reachable by direct SQL or a migration — is **omitted with a `Warn`**,
     never coerced.
  2. **The budget is 20 custom claims per token,** the same number the per-principal
     `claims-cap` rule carries, and it is filled **level-1 values first**, then defaults, each
     half ordered by claim name. What does not fit is dropped with a `Warn` naming it.
     Truncating is the fail-closed direction here: a custom claim only ever *adds* a fact, so
     dropping one can deny a consumer and can never grant. **Reported, not fixed by this run:**
     the catalog itself has no cap on definitions per tenant, so the loud fix — refusing the
     definition that would blow the budget, at the moment an operator creates it — is an
     `/omnicore:evolve-entity` run on `Claim`, and this emission cap does not remove the need
     for it.
  3. **A `must_change_password` session carries no custom claims at all.** Same reading that
     already governs `restrictToPasswordChange`: a session that exists to rotate an expired
     credential carries nothing but the one thing it is for. It costs nothing — the
     change-password route is this service’s own and reads no custom claim.
  4. **The response body mirrors the token.** `AuthenticatedUserResult` /
     `AuthenticatedUserResponse` gain a `claims` object. **It therefore mirrors the
     `must_change_password` restriction too** — otherwise this rebuilds exactly the bug that
     made `effectivePermissions` a single function: a body advertising facts the token does not
     carry. One resolution runs per request and both readers consume it; neither re-derives.

- **Not in this run, deliberately:** `auth.auditClaims` stays at its two entries. Forwarding an
  arbitrary tenant-defined map into every audit row, one per write, forever, is the separate
  decision `backlog.md` records it as, and nothing here forces it.

## §4 External contract (integrations only)

N/A — no external system. Every value read comes from this service’s own tables.

## §5 Impact map — every artifact touched

| Artifact | Change | Owning doc section |
|---|---|---|
| `microservice.dev.yaml`, `microservice.prd.yaml` | **no change** — nothing new is configurable; the budget is a code constant, not an operator knob, because a deployment lowering it would silently change what consumers authorize on | `yaml-reference.html` |
| infra prerequisite | N/A — no new infra | `shared/capabilities.md` |
| build/run commands | unchanged — `go build -tags postgres ./...`, same tag set as today | `shared/boot-contract.md` |
| `bootstrap/authentication_feature_manual.go` | **no change** — `NewAuthenticationReader(d.DB)` already receives the engine the new `ClaimRepository` needs; the composition root learns no new name | `bootstrap.html` |
| `internal/application/commands/authentication_commands_manual.go` | `AuthenticationStore` gains `ClaimDefinitionsOfTenant(ctx, tenantID) ([]*appdomain.Claim, error)` — **no invented type**: the aggregate is the shape, as `FindUserByEmail` already returns `*appdomain.User`. `buildClaims` and `buildProfile` each take the resolved map as a parameter. `AuthenticatedUserResult` gains `Claims map[string]any`. Both handlers call the reader after `ResolveGrants` and resolve once | `custom-command-handler.html` · `shared/domain-membership.md` |
| `internal/application/commands/authentication_claims_manual.go` **(new)** | The resolution itself: the chain, the typing, the budget, the platform-name guard, the `must_change_password` stand-down. A file of its own beside `authentication_journal_manual.go`, which is the precedent for splitting a self-contained concern out of the handlers | `service-layout.html` |
| `internal/infra/authentication_reader_manual.go` | A `claims *ClaimRepository` field built in `NewAuthenticationReader`, and `ClaimDefinitionsOfTenant` = `claims.Loader.FindAll` under `TenantID = ? AND AppliesTo IN ('user','both')`. **The loader’s default active-only scope is load-bearing**: an archived definition drops out, so a retired claim mints neither its default nor a value somebody still holds — the fail-closed reading, and the one the read join alone could not give (a join is not archive-gated on the target) | `shared/query-primitives.md` · `read-joins.html` |
| `internal/web/requests/authentication_requests_manual.go` | `AuthenticatedUserResponse` gains `Claims map[string]any \`json:"claims"\``, non-nil so it renders `{}` and never `null`, per the `nonNilStrings` precedent in the same file | `auto-handlers.html` |
| `internal/web/authentication_routes_manual.go` | The two OpenAPI `Description` blocks say what the token now carries: the chain, the `x_` namespace, the budget, and that a `mustChangePassword` token carries none of it | `openapi.html` |
| notification type(s) + the seven translation catalogs | **N/A — this capability raises no typed rejection.** Every refusal it can reach was already decided elsewhere: an unusable definition is refused at write time by `claim-available-in-tenant`, and a value that cannot be typed is dropped from the token with a log line, not answered to a caller — the sign-in has exactly one refusal and this must not become a second | `shared/notification-bases.md` |
| `README.md` | Rows 44 and 49 say *“It changes no token yet, by decision”* — that sentence stops being true and is replaced by what the token now carries | — |
| `backlog.md` | The `Claim` entry’s *“What is still open”* loses the emission and the size budget, keeps the third-level question and the audit allowlist, and gains the catalog-cap gap this run found but does not close | — |
| `internal/application/commands/authentication_commands_manual_test.go` (+ a new `authentication_claims_manual_test.go`) | held-beats-default · default-with-no-held-value · neither-⇒-absent · archived-definition-⇒-absent · the three typings · unparseable-value-⇒-omitted · platform-name-⇒-skipped · `must_change_password`-⇒-empty · budget overflow keeps held values and drops deterministically · **body equals token**, the anti-drift test mirroring the existing permissions one | — |
| `internal/infra/authentication_reader_manual_test.go` | the criteria the catalog read builds, and the active-only scope | — |

Phase 2 edits ONLY these rows, in dependency order.

## §6 Config & secrets

No new keys, in any profile, and no secrets. The 20-claim budget is a named constant in
`internal/application/commands/`, next to the rule it mirrors. `auth.auditClaims` is left at
`tenant_workspace` and `email` in both profiles — see §3.

## §7 Verify step — how this will be PROVEN

1. `go build -tags postgres ./... && go vet -tags postgres ./...` clean.
2. `go test -tags postgres ./... -count=1` green, with the new branches above covered and no
   existing test weakened. Coverage stays at or above the project’s 95% floor.
3. **Capability proof, locally provable in full** — the resolution is a pure function over a
   loaded aggregate and a slice of definitions, so every branch of the chain is proven by unit
   test without a database, and the reader’s criteria are proven against the existing
   `authentication_reader_manual_test.go` harness.
4. **What a local run cannot prove**, and the exact step to close it: that a real Postgres
   returns the catalog under the isolation the token path runs in, and that a signed token
   actually carries the typed values. Closed by booting the service (`/omnicore:run`),
   inserting one `x_`-prefixed definition with a default, setting a level-1 value on one user
   through `POST /users/:id/claims`, signing in, and decoding the JWT — offered at the end of
   Phase 2.

---

## Verification outcome — 2026-08-28

| Gate | Result |
|---|---|
| `go build -tags postgres ./...` | **clean** |
| `go vet -tags postgres ./...` | **clean** |
| `go test -tags postgres ./... -count=1` | **green**, every package; no existing test weakened, one call site updated for the widened `buildProfile` signature |
| Coverage, `internal/application/commands` | **97.7%** — `resolveCustomClaims` and `heldClaimValues` at 100% |
| Coverage, `internal/web/requests` | **100.0%** |
| Build tag set | unchanged, as §5 predicted |

**Capability proof, executed.** Every branch of the chain is covered by unit test:
specialised-beats-default · default-fills-in · absent-when-both-null · definition-outside-the-
catalog-mints-nothing · the three typings at both levels · unparseable-value-omitted ·
unknown-value-type-omitted · platform-name-skipped · must-change-password-carries-none ·
unsaved-definition-matches-nothing · budget-spends-specialised-first ·
budget-truncation-is-reproducible. At the handler seam: the catalog is read for the caller's own
tenant, its failure escapes as an exception rather than a credential refusal on **both** paths,
the fixed set survives a hostile custom map, the rotation RE-READS the catalog exactly once, and
the body's claims equal the token's — including on a restricted session, where both are empty.

**Two things deliberately left at the coverage they have**, stated rather than papered over:

- `AuthenticationReader.ClaimDefinitionsOfTenant` is at 0%, exactly like its siblings
  `FindUserByEmail`, `FindUserByID` and `ResolveGrants`. Every DB-touching method on that adapter
  is proven by the live suite rather than by a fake engine, and this run did not invent a harness
  none of them has.
- `typedClaimValue`'s number branch keeps a guard the shared gate already makes unreachable. It
  stays because dropping it would mean minting `0` for a value that did not parse if that gate
  ever changed, and an uncoverable defensive line is the cheaper of the two.

**What a local run still cannot prove**, unchanged from §7: that a real Postgres returns the
catalog under the token path's isolation, and that a signed JWT carries the typed values. The
step is in §7 and was offered at the end of the run.
