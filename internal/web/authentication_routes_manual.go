// Hand-written, and not a hook: no generator declares this file.
//
// The three routes that make this service an IdP rather than merely a consumer of
// one: a person signs in, rotates, and a machine signs in. The framework ships the Issuer and mounts the JWKS document; every HTTP
// endpoint on top of that is the consuming service's, and its own manual says so
// outright — "POST /auth/login, POST /auth/refresh, an introspection endpoint —
// all of it is built by the consuming service".
//
// THEY GO THROUGH THE PIPELINE, not through MountRaw. The framework's own example
// for this feature uses a bare Fiber closure answering fiber.ErrUnauthorized, and
// that would have been shorter — but a bare status is untranslated, and a refusal
// nobody can read in their own language is not the refusal this service was asked
// for. Going through CommandWithBodySpec keeps the canonical envelope, the seven
// catalogs, the AppContext, the audit actor and the OpenAPI schema derived from
// the DTO rather than from whatever the author remembered to type.
//
// ALL THREE ARE PUBLIC, and they have to be: a sign-in cannot require the
// credential it is about to grant. Doc.Public satisfies the boot-time
// authorization scan, but it does NOT bypass the middleware — that comes only from
// auth.publicRoutes in the yaml, exact "METHOD /path" match, and both profiles list
// all three. Omit an entry and the route answers 401 before its handler ever runs.

package web

import (
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers"
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/dtos"
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/utils"
	"github.com/ClaudioSchirmer/authcore/internal/web/requests"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	fwweb "github.com/ClaudioSchirmer/omnicore/web"
	fwopenapi "github.com/ClaudioSchirmer/omnicore/web/openapi"
	"github.com/gofiber/fiber/v3"
)

// MountAuthentication mounts the two user token routes and the machine one.
//
// It takes the ports rather than the concrete adapters, for the same reason
// MountUserCredentials does: the interfaces are declared in the application layer
// and name exactly what these two operations need.
func MountAuthentication(
	app *fiber.App,
	store dtos.AuthenticationStore,
	clients dtos.ClientAuthenticationStore,
	lookup dtos.RefreshTokenLookup,
	attempts dtos.AttemptRecorder,
	events dtos.AuthenticationEventPublisher,
	issuer dtos.TokenIssuer,
	accessIssuer dtos.AccessTokenIssuer,
	d bootstrap.Deps,
) {
	// The group is /auth and the SUBJECT TYPE is the next segment, not a field in
	// the body.
	//
	// The client-credentials token below is a different operation wearing a similar
	// name: it takes a client id and a secret instead of an e-mail and a
	// password, its claims carry no e-mail and no groups, and per RFC 6749 §4.4.3 it
	// issues no refresh token at all — the client's secret IS its long-lived
	// credential. Folding that into one endpoint behind a `grantType` field would
	// rebuild exactly the schema this service already rejected once: see
	// internal/web/requests/user_credential_requests_manual.go, where the two
	// credential routes were split rather than share a body whose required fields
	// depend on which operation you meant.
	//
	// The usual counter-argument is that the market standard is a single token
	// endpoint (Auth0, Okta, Keycloak: POST /oauth/token + grant_type). That standard
	// exists because those APIs ARE RFC 6749 — form-encoded in, a flat snake_case
	// access_token/expires_in out. This one is neither: JSON in, and the canonical
	// omnicore envelope out. No off-the-shelf OAuth2 client works against it either
	// way, so the compatibility a single endpoint would buy is not on the table.
	group := app.Group("/auth")

	// THE ORIGIN ADDRESS, stashed for the handler.
	//
	// A pipeline.Handler receives the AppContext, not the Fiber one, and the
	// framework's AppContext exposes no IP — so the address is put in its generic
	// bag here, where the Fiber context still exists. The alternative was MountRaw
	// to reach c.IP() directly, which would have cost the canonical envelope and
	// the seven catalogs to carry one string.
	//
	// It is scoped to THIS group rather than registered globally: only the sign-ins
	// read it, and a middleware on every route in the service would be a cost every
	// route pays for the few that benefit.
	//
	// IT IS ALSO WHY BOTH TOKEN ROUTES ARE MOUNTED FROM THIS ONE FUNCTION. Fiber
	// binds a group middleware in registration order, so a client-token route
	// mounted from a second feature under the same `/auth` prefix would run this or
	// not depending on which feature Wire listed first — and the symptom would be an
	// empty origin address, which for a client holding an allow-list is a refusal.
	// One owner of the group is what makes that deterministic.
	//
	// ON THE CLIENT ROUTE THIS ADDRESS IS AN AUTHORIZATION INPUT, not merely a
	// forensic one: Client.allowedCIDRs is compared against it. Read the note on
	// c.IP() below before changing anything here.
	//
	// c.IP() IS THE SOCKET PEER, ALWAYS, at this framework pin. Fiber reads a proxy
	// header only when both TrustProxy and ProxyHeader are set, and omnicore sets
	// neither — the `http:` block carries no key that could. So nothing a caller
	// sends can move this value, which is what makes the allow-list unspoofable.
	//
	// The flip side is the one whoever deploys this has to know: behind an ingress
	// or a load balancer, every request carries the BALANCER's address. The attempt
	// log then records it instead of the caller, and an allow-list of real egress
	// ranges refuses everybody while one holding the balancer's range admits
	// everybody. Until omnicore ships a trusted-proxy block, such a deployment
	// should leave allowedCIDRs empty rather than believe a restriction it does not
	// have.
	//
	// READING X-Forwarded-For HERE WOULD BE A BUG, not a fix: an unguarded header is
	// caller-controlled, so an attacker would simply name an allowed range. That is
	// precisely the vulnerability the framework's silence currently prevents.
	group.Use(func(c fiber.Ctx) error {
		if appCtx := fwweb.AppContext(c); appCtx != nil {
			appCtx.Set(utils.ContextKeyClientIP, c.IP())
		}
		return c.Next()
	})

	// ── the SIGN-IN ──────────────────────────────────────────────────────────
	//
	// 200 and not 201: nothing was created that a caller could go and fetch. A
	// token is an answer, not a resource — there is no GET /auth/user/token
	// for a Location header to point at.
	issueH, issueSpec := fwweb.CommandWithBodySpec(d.Pipeline,
		requests.IssueTokenRequest{},
		requests.TokenResponse{}.FromResult,
		&handlers.IssueTokenHandler{Store: store, Attempts: attempts, Events: events, Issuer: issuer},
		fiber.StatusOK)
	fwopenapi.Mount(d.OpenAPIRegistry, group, fiber.MethodPost, "/user/token",
		issueH, issueSpec,
		fwopenapi.Doc{
			Tags: []string{"Auth"},
			// Declares no permission, legitimately: this is the route that HANDS
			// OUT the credential every other route demands. Public here satisfies
			// the boot-time authorization scan; the middleware bypass itself comes
			// from auth.publicRoutes in the yaml.
			Public:  true,
			Summary: "Sign in and receive a token pair",
			Description: "Exchanges an e-mail and a password for a signed access token and an " +
				"opaque refresh token.\n\n" +
				"The access token carries the caller's identity and their effective " +
				"permissions — the union of what their directly granted roles confer and what " +
				"the roles of every group they belong to confer, with archived roles, groups, " +
				"memberships and grants excluded at every hop. Any service in the mesh accepts " +
				"it by pointing `auth.jwt.jwksUrl` at this service's JWKS document; no code " +
				"change is involved anywhere.\n\n" +
				"**A failed sign-in always answers the same 401**, whether the address is " +
				"unknown, the password is wrong, the account is suspended or its tenant is. " +
				"Telling those apart would let anyone discover which addresses have accounts " +
				"here, so the message is deliberately one message — and the response takes the " +
				"same time in every case, because a faster refusal would have answered the " +
				"question the wording refuses to answer.\n\n" +
				"**The token also carries the tenant's own claims**, under the reserved `x_` " +
				"namespace and never under a platform name. Each one is resolved down a " +
				"two-level chain: the value set on this user wins, the definition's " +
				"tenant-wide default fills in when none is, and a claim with neither is " +
				"absent from the token entirely — an absent claim and an empty one are not " +
				"the same thing. Values are rendered in the type their definition declares, " +
				"so a `number` claim is a number and not a quoted string. A retired " +
				"definition mints nothing, including for a user who still holds a value for " +
				"it. At most 20 of them ride on one token: a claim rides in a header on " +
				"every request to every service, and the values set on this user are spent " +
				"before any tenant-wide default.\n\n" +
				"**When `mustChangePassword` is true the token is restricted**: it carries " +
				"`user:change-password` and nothing else — no tenant claims either — so an " +
				"expired credential can be rotated and nothing else can be done until it is.",
			RequestExamples: map[string]fwopenapi.Example{
				"signIn": {
					Summary:     "Sign in",
					Description: "The everyday case.",
					Value: requests.IssueTokenRequest{
						Email:    "ada@acme.test",
						Password: "Str0ng!Passphrase",
					},
				},
			},
		},
	)

	// ── the ROTATION ─────────────────────────────────────────────────────────
	refreshH, refreshSpec := fwweb.CommandWithBodySpec(d.Pipeline,
		requests.RefreshTokenRequest{},
		requests.TokenResponse{}.FromResult,
		&handlers.RefreshTokenHandler{Store: store, Lookup: lookup, Issuer: issuer},
		fiber.StatusOK)
	fwopenapi.Mount(d.OpenAPIRegistry, group, fiber.MethodPost, "/user/token/refresh",
		refreshH, refreshSpec,
		fwopenapi.Doc{
			Tags: []string{"Auth"},
			// Public for the same reason as the sign-in: the caller's access token
			// may well have expired — that is precisely when a rotation is needed —
			// so demanding one here would make the endpoint useless exactly when it
			// matters. The opaque refresh value in the body IS the credential.
			Public:  true,
			Summary: "Rotate a refresh token into a new pair",
			Description: "Exchanges an unused refresh token for a fresh access token and a fresh " +
				"refresh token. The old one is burned in the process — refresh tokens are " +
				"single-use, and that is what makes a stolen one detectable.\n\n" +
				"**The claims are rebuilt from the database on every rotation**, never copied " +
				"from the previous token. A permission revoked while a session is live " +
				"therefore reaches the whole mesh at the next rotation — minutes — instead of " +
				"waiting for the next full sign-in, and **a corrected claim value propagates " +
				"the same way**, with no invalidation step anywhere. The restriction that " +
				"applies to a `mustChangePassword` account applies here too, so a limited " +
				"session cannot widen itself by refreshing.\n\n" +
				"**Presenting an already-redeemed token revokes the entire session family** — " +
				"every token descended from that sign-in — because a value being replayed means " +
				"either the holder or somebody else has a copy, and there is no way to tell " +
				"which. The answer is the same generic 401 as any other failure: confirming " +
				"that the token was real and already used would tell a thief exactly what they " +
				"wanted to know.",
			RequestExamples: map[string]fwopenapi.Example{
				"rotate": {
					Summary:     "Rotate",
					Description: "Send back the refreshToken from the previous response.",
					Value: requests.RefreshTokenRequest{
						RefreshToken: "Qk9HVVMtUkVGUkVTSC1UT0tFTi1FWEFNUExF",
					},
				},
			},
		},
	)

	// ── the MACHINE SIGN-IN ──────────────────────────────────────────────────
	//
	// The sibling of the route above, and a different operation: a client id and a
	// server-minted secret instead of an e-mail and a password.
	//
	// NO /client/token/refresh COMPANION, and there never will be one. RFC 6749
	// §4.4.3 says the client-credentials grant SHOULD NOT issue a refresh token, and
	// the reason is structural rather than ceremonial: a refresh token exists for a
	// credential that cannot be re-presented, and a client holds its secret in a file
	// or a vault by construction. Issuing one would be a second long-lived credential
	// to store, rotate and revoke, buying nothing.
	//
	// 200 for the same reason the user sign-in is 200: a token is an answer, not a
	// resource.
	issueClientH, issueClientSpec := fwweb.CommandWithBodySpec(d.Pipeline,
		requests.IssueClientTokenRequest{},
		requests.ClientTokenResponse{}.FromResult,
		&handlers.IssueClientTokenHandler{
			Store: clients, Attempts: attempts, Events: events, Issuer: accessIssuer,
		},
		fiber.StatusOK)
	fwopenapi.Mount(d.OpenAPIRegistry, group, fiber.MethodPost, "/client/token",
		issueClientH, issueClientSpec,
		fwopenapi.Doc{
			Tags: []string{"Auth"},
			// Public for the same reason as the two above: this is the route that
			// hands out the credential every other route demands.
			Public:  true,
			Summary: "Authenticate an integration and receive an access token",
			Description: "Exchanges a client id and its secret for a signed access token. " +
				"This is the machine-to-machine sign-in; the human one is " +
				"`POST /auth/user/token`.\n\n" +
				"**No refresh token is issued, and that is deliberate** — the client's " +
				"secret already IS its long-lived credential, so an integration renews " +
				"simply by calling this route again when `expiresAt` passes. There is no " +
				"`/auth/client/token/refresh`.\n\n" +
				"The access token carries `identity_kind: \"client\"`, the client id as " +
				"`sub`, its tenant, its label, and the effective permissions its granted " +
				"roles confer — with archived roles, grants and catalog entries excluded at " +
				"every hop. It carries **no `email`, no `groups` and no " +
				"`must_change_password`**: a machine has no address, a client belongs to no " +
				"groups, and there is no credential it can be told to rotate itself.\n\n" +
				"**It also carries the tenant's own claims**, under the reserved `x_` " +
				"namespace, resolved down the same two-level chain the user token uses — " +
				"the value set on this client wins, the definition's tenant-wide default " +
				"fills in when none is, and a claim with neither is absent entirely. Only " +
				"definitions whose `appliesTo` admits a client take part.\n\n" +
				"**A failed sign-in always answers the same 401**, whether the id is " +
				"unknown, the secret is wrong, the secret was rotated out, the integration " +
				"is suspended, its tenant is, or the request came from an address the " +
				"client does not allow. Telling those apart would confirm to whoever holds " +
				"a stolen secret that the secret itself is good.\n\n" +
				"**A rotation in flight keeps working.** After `POST /clients/{id}/secret` " +
				"the retiring secret is accepted until its `previousSecretExpiresAt` " +
				"passes, so an integration can be redeployed without a window in which " +
				"neither value works. A rotation asking for a zero grace period kills the " +
				"old secret immediately, which is what to do with a leaked one.\n\n" +
				"**Where from:** if the client declares `allowedCIDRs`, the request's " +
				"origin address must fall inside one of them; an empty collection means any " +
				"address. The address compared is the one this service sees on the socket — " +
				"no proxy header is read, so it cannot be forged, but behind a load " +
				"balancer it is the balancer's address rather than the caller's. On such a " +
				"deployment leave the collection empty. And in every deployment the " +
				"allow-list constrains where a token is *obtained*, never where it is " +
				"*used*: authcore does not see the requests an integration later makes to " +
				"other services.",
			// ⚠️ THESE ARE THE LOCAL DEV FIXTURE'S REAL VALUES, not a placeholder.
			//
			// The row they name is hand-seeded into the dev bench's `clients` table
			// (`Fixture integration`, tenant `acme`), because this bench's permission
			// catalog carries no `client:insert` and so no operator here can create one
			// through POST /clients. Having them here means the /docs page is a working
			// "try it" against a local bench instead of a form to fill in by hand.
			//
			// THE COST, SAID OUT LOUD: this is a credential that authenticates, sitting
			// in the repository and rendered on a page anyone who reaches /docs can read.
			// It is harmless only while the row exists nowhere but a developer's own
			// Postgres. **Never seed this fixture into a shared or deployed
			// environment** — the pair would be a known-good credential for anyone with
			// the source. If it ever is, rotate it there and replace these two literals.
			RequestExamples: map[string]fwopenapi.Example{
				"signIn": {
					Summary:     "Authenticate",
					Description: "The everyday case. On the local dev bench these are the seeded fixture's real values.",
					Value: requests.IssueClientTokenRequest{
						ClientID:     "0f1c7e5a-0000-4000-8000-c1e17f170001",
						ClientSecret: "acs_E0a8gj-IkEn1qht-STfTmUwJnmF8iQBVmbbdyrgzjVQ",
					},
				},
			},
		},
	)
}
