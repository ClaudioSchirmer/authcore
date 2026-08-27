// Hand-written, and not a hook: no generator declares this file.
//
// The two routes that make this service an IdP rather than merely a consumer of
// one. The framework ships the Issuer and mounts the JWKS document; every HTTP
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
// BOTH ARE PUBLIC, and they have to be: a sign-in cannot require the credential
// it is about to grant. Doc.Public satisfies the boot-time authorization scan,
// but it does NOT bypass the middleware — that comes only from auth.publicRoutes
// in the yaml, exact "METHOD /path" match, and both profiles list these two. Omit
// either entry and the route answers 401 before this handler ever runs.

package web

import (
	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/authcore/internal/web/requests"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	fwweb "github.com/ClaudioSchirmer/omnicore/web"
	fwopenapi "github.com/ClaudioSchirmer/omnicore/web/openapi"
	"github.com/gofiber/fiber/v3"
)

// MountAuthentication mounts the sign-in and the rotation.
//
// It takes the ports rather than the concrete adapters, for the same reason
// MountUserCredentials does: the interfaces are declared in the application layer
// and name exactly what these two operations need.
func MountAuthentication(
	app *fiber.App,
	store commands.AuthenticationStore,
	lookup commands.RefreshTokenLookup,
	attempts commands.AttemptRecorder,
	issuer commands.TokenIssuer,
	d bootstrap.Deps,
) {
	// The group is /auth and the SUBJECT TYPE is the next segment, not a field in
	// the body.
	//
	// A client-credentials token is coming, and it is a different operation wearing
	// a similar name: it takes a client id and a secret instead of an e-mail and a
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
	// It is scoped to THIS group rather than registered globally: only the sign-in
	// reads it, and a middleware on every route in the service would be a cost
	// every route pays for one that benefits.
	//
	// c.IP() honours the proxy headers Fiber is configured to trust. On a
	// deployment behind a load balancer that trust has to be configured, or every
	// attempt in the log records the balancer's address and the spray dimension is
	// worthless — that is a deployment decision, and this comment is where whoever
	// makes it should find out that it matters.
	group.Use(func(c fiber.Ctx) error {
		if appCtx := fwweb.AppContext(c); appCtx != nil {
			appCtx.Set(commands.ContextKeyClientIP, c.IP())
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
		&commands.IssueTokenHandler{Store: store, Attempts: attempts, Issuer: issuer},
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
				"**When `mustChangePassword` is true the token is restricted**: it carries " +
				"`user:change-password` and nothing else, so an expired credential can be " +
				"rotated and nothing else can be done until it is.",
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
		&commands.RefreshTokenHandler{Store: store, Lookup: lookup, Issuer: issuer},
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
				"waiting for the next full sign-in. The restriction that applies to a " +
				"`mustChangePassword` account applies here too, so a limited session cannot " +
				"widen itself by refreshing.\n\n" +
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
}
