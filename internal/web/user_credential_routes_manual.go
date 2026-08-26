// Hand-written, and not a hook: no generator declares this file.
//
// ONE route: the password reset. It is here rather than in the generated
// user_routes.go because it is not a shape the spec language can declare — the
// spec gates operations by a closed set of verbs and "reset a credential" is not
// one of them — so putting it in a file the generator owns would mean
// hand-editing generated code on every run.
//
// A SECOND ROUTE USED TO LIVE HERE and was removed on 2026-08-26: a PUBLIC
// change-password, identified by e-mail, that verified the current password. It
// was built on a premise that does not hold in this service — that somebody
// needing a new password might be unable to obtain a token. There is no login
// route here, so `mustChangePassword` blocks no authentication; there is no
// credential expiry; and a caller who knows the current password can simply
// authenticate. It therefore did exactly what this route does, without a token,
// which is strictly worse. What it was reaching for is a FORGOT-PASSWORD flow —
// e-mail, a one-time link, an expiring token — and that is a real piece of work
// nobody has started.
//
// It is mounted the same way the nine generated routes are, through
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
// the generated mount: these handlers resolve a user by e-mail, which is not a
// method the generic repository interface has and should not be — no other route
// in this service reads a row without a tenant scope.
func MountUserCredentials(
	app *fiber.App,
	store commands.UserCredentialStore,
	svc domain.Service,
	d bootstrap.Deps,
) {
	// Both answer 204 through fwresponses.NoBody: nothing but "done". A
	// credential operation that answered with the user's row would turn a
	// password change into a profile read — and the public one would do it
	// without a token.
	group := app.Group("/users")

	// ── the AUTHENTICATED reset ──────────────────────────────────────────────
	//
	// user:reset-password, and `*:*` satisfies it for free — HasPermission
	// answers true for any concrete permission when the claim set carries the
	// wildcard, so the platform operator needs no second acceptor.
	//
	// THE SELF ACCEPTOR IS DELIBERATELY GONE. It was in the approved model and it
	// was removed on 2026-08-26 for two reasons that point the same way. The
	// mechanical one: auth.authorization.enabled=true makes the framework refuse
	// to boot with a non-public route that declares no permission — proven, not
	// inferred, by booting a profile with the flag on and reading the panic — and
	// RequirePermission expresses exactly ONE permission, so three alternatives
	// were not declarable here at all.
	//
	// The better one: a logged-in user who KNOWS their password already has the
	// change endpoint above, and one who does NOT know it is precisely who needs
	// a helpdesk. Letting a session replace a credential without proving the
	// previous one is the attack `currentPassword` exists to stop, so the route
	// that skips that proof is the one that should require somebody else's
	// permission.
	resetH, resetSpec := fwweb.CommandWithBodyIDSpec(d.Pipeline,
		requests.ResetPasswordRequest{},
		fwresponses.NoBody,
		&commands.ResetPasswordHandler{Store: store, Service: svc},
		fiber.StatusNoContent)
	fwopenapi.Mount(d.OpenAPIRegistry, group, fiber.MethodPatch, "/:id/password-reset",
		resetH, resetSpec,
		fwopenapi.Doc{
			Tags:    []string{"Users"},
			Summary: "Reset a user's password",
			Description: "Requires a token, and accepts THREE callers: the user themselves, a *:* " +
				"platform operator, or a holder of user:reset-password. `user:update` deliberately " +
				"reaches none of them — whoever can fix a typo in a name must not be able to take over " +
				"an account.\n\n" +
				"It carries no current password: not knowing it is the point of a reset. The password " +
				"it leaves is somebody else's choice, so the next sign-in must replace it — the " +
				"must-change flag is set here and cleared only by the change endpoint above.",
			RequestExamples: map[string]fwopenapi.Example{
				"reset": {
					Summary:     "Set a temporary password",
					Description: "The helpdesk case. The user is forced to change it on the next sign-in.",
					Value: requests.ResetPasswordRequest{
						Password:             "An0ther!Passphrase",
						PasswordConfirmation: "An0ther!Passphrase",
					},
				},
			},
		},
		fwopenapi.RequirePermission("user:reset-password"))
}
