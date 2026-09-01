// Hand-written, and not a hook: no generator declares this file.
//
// ONE route: the secret rotation. It is here rather than in the generated
// client_routes.go because it is not a shape the spec language can declare — the
// spec gates operations by a closed set of verbs and "rotate a credential" is
// not one of them — so putting it in a file the generator owns would mean
// hand-editing generated code on every run.
//
// IT IS A POST AND NOT A PATCH, and that is the honest verb: the operation
// CREATES a credential. A PATCH on the client would say the client changed,
// which is not what a caller means when they ask for a new secret — and it would
// sit on the same shape as the generated `PATCH /clients/:id`, where the two
// would have to be told apart by a suffix that reads like a field name.
//
// IT ANSWERS 200 WITH A BODY, unlike the two credential routes on User, which
// answer 204. That is not an inconsistency: those two SET a credential the caller
// already chose, and this one MINTS a credential the caller has no other way to
// learn. The plaintext is never stored, so this response is the only place it
// can ever be read.

package web

import (
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers"
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/dtos"
	"github.com/ClaudioSchirmer/authcore/internal/web/requests"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	"github.com/ClaudioSchirmer/omnicore/domain"
	fwweb "github.com/ClaudioSchirmer/omnicore/web"
	fwgraphql "github.com/ClaudioSchirmer/omnicore/web/graphql"
	fwopenapi "github.com/ClaudioSchirmer/omnicore/web/openapi"
	"github.com/gofiber/fiber/v3"
)

// MountClientSecrets mounts the rotation.
//
// It takes the CONCRETE store rather than persistence.ScopedRepository, unlike
// the generated mount: the interface the handler needs is declared in the
// application layer and names exactly the two things it does.
func MountClientSecrets(
	app *fiber.App,
	store dtos.ClientSecretStore,
	svc domain.Service,
	d bootstrap.Deps,
) {
	group := app.Group("/clients")

	// client:rotate-secret, and it is a SIXTH verb rather than client:update for
	// the reason user:reset-password is its own: replacing a credential is not
	// editing a label, and an operator who may fix a typo in an integration's
	// description is not automatically an operator who may hand out a new
	// production credential. `*:*` satisfies it for free, like any concrete
	// permission.
	//
	// The route says who may attempt the verb. WHOSE row they reached — the
	// caller's own tenant, and, once the client token endpoint exists, a
	// client-subject caller's own row — is decided in the aggregate, beside
	// every other rule.
	rotateH, rotateSpec := fwweb.CommandWithBodyIDSpec(d.Pipeline,
		requests.RotateClientSecretRequest{},
		requests.RotateClientSecretResponse{}.FromResult,
		&handlers.RotateClientSecretHandler{Store: store, Service: svc},
		fiber.StatusOK)
	fwopenapi.Mount(d.OpenAPIRegistry, group, fiber.MethodPost, "/:id/secret",
		rotateH, rotateSpec,
		fwopenapi.Doc{
			Tags:    []string{"Clients"},
			Summary: "Rotate a client's secret",
			Description: "Mints a new secret and starts retiring the current one. **The " +
				"plaintext is in this response and nowhere else** — nothing stores it, so " +
				"there is no endpoint that can show it again. A caller who loses it rotates " +
				"again.\n\n" +
				"Rotation OVERLAPS rather than swaps: the old secret keeps working until " +
				"`previousSecretExpiresAt`, so consumers can be redeployed without an outage. " +
				"`gracePeriodSeconds` sets that window, 0 to 604800; omit it for 86400 (a " +
				"day). **Send 0 when the old secret leaked** — it stops working immediately " +
				"and the response carries no expiry.\n\n" +
				"The client must be `active`: a suspended one was switched off deliberately, " +
				"and handing it a fresh credential is the opposite of what that meant. " +
				"The row must belong to the caller's tenant; a `*:*` operator crosses that.",
			RequestExamples: map[string]fwopenapi.Example{
				"planned": {
					Summary:     "Planned rotation, a day to switch over",
					Description: "The everyday case: consumers keep working on the old secret while they are redeployed.",
					Value:       requests.RotateClientSecretRequest{GracePeriodSeconds: intPtr(86400)},
				},
				"leaked": {
					Summary:     "The secret leaked — kill it now",
					Description: "A zero window clears the retiring slot instead of stamping a deadline: the old secret stops working with this call.",
					Value:       requests.RotateClientSecretRequest{GracePeriodSeconds: intPtr(0)},
				},
			},
		},
		fwopenapi.RequirePermission("client:rotate-secret"))
}

// intPtr exists for the OpenAPI examples above, which need addressable values.
func intPtr(v int) *int { return &v }

// MountClientSecretsGraphQL exposes the rotation on the GraphQL surface.
//
// Its own mount for the reason the REST route has one: the generated
// MountClientsGraphQL is a file the generator owns and rewrites, so a field
// added there would be an edit the next run refuses. The handler and the
// permission are the REST route's own — one implementation, two surfaces.
//
// NO PAYLOAD TYPE OF ITS OWN, unlike the two user credential fields. Those
// mirror a 204 and had to invent an acknowledgement; this one already answers
// with a body, so the REST Response projects the same Result here. The secret is
// therefore shown once on this surface too, and nowhere else.
//
// THE GRACE WINDOW STAYS NULLABLE IN THE SCHEMA, which is not cosmetic: the
// command distinguishes ZERO ("kill the old secret now" — the leaked case) from
// ABSENT ("use the default day"). A non-null Int would collapse the two and turn
// an omitted argument into an immediate revocation.
func MountClientSecretsGraphQL(
	reg *fwgraphql.Registry,
	store dtos.ClientSecretStore,
	svc domain.Service,
) {
	reg.Register(fwgraphql.MutationWithBodyID[requests.RotateClientSecretRequest](
		"rotateClientSecret", requests.RotateClientSecretResponse{}.FromResult,
		&handlers.RotateClientSecretHandler{Store: store, Service: svc},
		fwgraphql.RequirePermission("client:rotate-secret")))
}
