// Hand-written, and not a hook: no generator declares this file.
//
// TWO routes: the self-service password change and the helpdesk reset. They are
// here rather than in the generated user_routes.go because neither is a shape
// the spec language can declare — the spec gates operations by a closed set of
// verbs and "change a credential" is not one of them — so putting them in a file
// the generator owns would mean hand-editing generated code on every run.
//
// ONE PERMISSION EACH, AND THAT IS WHY THERE ARE TWO ROUTES. RequirePermission
// expresses exactly one permission, and the framework refuses to boot with a
// non-public route that declares none — proven, not inferred, by booting a
// profile with auth.authorization.enabled on and reading the panic. A single
// anfibious route would have had to fold "the owner" and "the helpdesk" behind
// one key, which would have changed what an already-published key grants. Split,
// each door keeps its own meaning: user:reset-password still means what it
// always meant, and the self case gets a key nobody holds yet.
//
// A THIRD ROUTE USED TO LIVE HERE and was removed on 2026-08-26: a PUBLIC
// change-password, identified by e-mail. It was built on a premise that does not
// hold in this service — that somebody needing a new password might be unable to
// obtain a token. What it was reaching for is a FORGOT-PASSWORD flow — e-mail, a
// one-time link, an expiring token — and that is a real piece of work nobody has
// started.
//
// Both are mounted the same way the nine generated routes are, through
// CommandWithBodyIDSpec, so the OpenAPI page derives the schema, the field list
// and the examples from the Request DTO instead of from whatever the author
// remembered to type.

package web

import (
	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/authcore/internal/web/requests"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	"github.com/ClaudioSchirmer/omnicore/domain"
	fwweb "github.com/ClaudioSchirmer/omnicore/web"
	fwopenapi "github.com/ClaudioSchirmer/omnicore/web/openapi"
	fwresponses "github.com/ClaudioSchirmer/omnicore/web/responses"
	"github.com/gofiber/fiber/v3"
)

// MountUserCredentials mounts the change and the reset.
//
// It takes the CONCRETE store rather than persistence.ScopedRepository, unlike
// the generated mount: the interface the handlers need is declared in the
// application layer and names exactly the two things they do.
func MountUserCredentials(
	app *fiber.App,
	store commands.UserCredentialStore,
	svc domain.Service,
	d bootstrap.Deps,
) {
	// Both answer 204 through fwresponses.NoBody: nothing but "done". A
	// credential operation that answered with the user's row would turn a
	// password change into a profile read.
	group := app.Group("/users")

	// ── the CHANGE ───────────────────────────────────────────────────────────
	//
	// user:change-password. It is the door for EVERY user, so it belongs in
	// whatever role a tenant grants by default — a caller without it cannot set
	// their own password at all. `*:*` satisfies it for free, like any concrete
	// permission.
	//
	// The route says who may attempt the verb. That the id on the path is the
	// CALLER'S OWN is decided in the aggregate, beside the tenant scope guard,
	// because it is a question about a row rather than about admission.
	changeH, changeSpec := fwweb.CommandWithBodyIDSpec(d.Pipeline,
		requests.ChangePasswordRequest{},
		fwresponses.NoBody,
		&commands.ChangePasswordHandler{Store: store, Service: svc},
		fiber.StatusNoContent)
	fwopenapi.Mount(d.OpenAPIRegistry, group, fiber.MethodPatch, "/:id/password",
		changeH, changeSpec,
		fwopenapi.Doc{
			Tags:    []string{"Users"},
			Summary: "Change your own password",
			Description: "Requires a token, and the id on the path must be the caller's own — " +
				"pointing this at another user answers 403 no matter what the caller holds. " +
				"The current password is required and verified: proving the credential you " +
				"hold is what separates this from a reset.\n\n" +
				"It CLEARS the must-change flag, because the password it leaves is the " +
				"caller's own choice. To set somebody else's password, see the reset below.",
			RequestExamples: map[string]fwopenapi.Example{
				"change": {
					Summary:     "Rotate your own credential",
					Description: "The everyday case: the caller proves what they hold and chooses what replaces it.",
					Value: requests.ChangePasswordRequest{
						CurrentPassword:      "Str0ng!Passphrase",
						Password:             "An0ther!Passphrase",
						PasswordConfirmation: "An0ther!Passphrase",
					},
				},
			},
		},
		fwopenapi.RequirePermission("user:change-password"))

	// ── the RESET ────────────────────────────────────────────────────────────
	//
	// user:reset-password, unchanged in meaning and in name: whoever held it
	// before this split still resets other people's passwords and nothing else.
	// `*:*` satisfies it for free — HasPermission answers true for any concrete
	// permission when the claim set carries the wildcard, so the platform
	// operator needs no second acceptor.
	//
	// `user:update` deliberately reaches neither route — whoever can fix a typo
	// in a name must not be able to take over an account.
	resetH, resetSpec := fwweb.CommandWithBodyIDSpec(d.Pipeline,
		requests.ResetPasswordRequest{},
		fwresponses.NoBody,
		&commands.ResetPasswordHandler{Store: store, Service: svc},
		fiber.StatusNoContent)
	fwopenapi.Mount(d.OpenAPIRegistry, group, fiber.MethodPatch, "/:id/password-reset",
		resetH, resetSpec,
		fwopenapi.Doc{
			Tags:    []string{"Users"},
			Summary: "Reset another user's password",
			Description: "The helpdesk operation. Requires a token and `user:reset-password`, " +
				"which a `*:*` claim satisfies. It carries no current password: not knowing " +
				"it is the point of a reset.\n\n" +
				"The id on the path must NOT be the caller's own — resetting yourself here " +
				"would replace your credential without proving the previous one, which is " +
				"what the change endpoint exists to demand. Use that one for your own " +
				"password.\n\n" +
				"The row must belong to the caller's tenant; a `*:*` operator crosses that. " +
				"The password it leaves is somebody else's choice, so the must-change flag is " +
				"set here and cleared only by the change above.",
			RequestExamples: map[string]fwopenapi.Example{
				"reset": {
					Summary:     "Set a temporary password",
					Description: "The user is forced to change it on the next sign-in.",
					Value: requests.ResetPasswordRequest{
						Password:             "An0ther!Passphrase",
						PasswordConfirmation: "An0ther!Passphrase",
					},
				},
			},
		},
		fwopenapi.RequirePermission("user:reset-password"))
}
