// Hand-written, and not a hook: no generator declares this file.
//
// The two credential routes. They are separate from the generated user_routes.go
// for a reason that is structural rather than stylistic: neither is a shape the
// spec language can declare, and putting them in a file the generator owns would
// mean hand-editing generated code on every regeneration.
//
// ⚠️ THE MOUNT ORDER IN HERE IS LOAD-BEARING. `PATCH /users/password` and
// `PATCH /users/:id` are the same shape to a router — `password` matches `:id` —
// so the open route must be registered FIRST. This file is mounted before
// MountUsers by the feature that owns it, and there is a test that asserts the
// failure direction: a valid body to /users/password answering 401 means the
// by-id route swallowed it and the auth middleware got there first, which looks
// like a permissions bug and is a routing bug.

package web

import (
	"encoding/json"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/pipeline"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	"github.com/ClaudioSchirmer/omnicore/domain"
	fwweb "github.com/ClaudioSchirmer/omnicore/web"
	fwopenapi "github.com/ClaudioSchirmer/omnicore/web/openapi"
	"github.com/gofiber/fiber/v3"
)

// ChangePasswordPath is the public route, and it is declared here so the two
// places that must agree about it — this mount and `auth.publicRoutes` in both
// profile yamls — can be checked against one constant instead of against each
// other.
const ChangePasswordPath = "/users/password"

// MountUserCredentials mounts the change and the reset.
//
// It takes the CONCRETE store rather than persistence.ScopedRepository, unlike
// the generated mount: these handlers resolve a user by e-mail, which is not a
// method the generic repository interface has and should not be — no other
// route in this service reads a row without a tenant scope.
func MountUserCredentials(
	app *fiber.App,
	store commands.UserCredentialStore,
	svc domain.Service,
	hasher appdomain.PasswordHasher,
	d bootstrap.Deps,
) {
	mountChangePassword(app, store, svc, hasher, d)
	mountResetPassword(app, store, svc, d)
}

// mountChangePassword mounts the ONE unauthenticated write in this service.
//
// It is mounted RAW rather than through the CommandWithBody wrapper because the
// wrapper carries a permission and an identity, and this route has neither by
// design. What it does keep is the pipeline: the same dispatch, the same
// notification envelope and the same seven catalogs every other write answers
// with.
func mountChangePassword(
	app *fiber.App,
	store commands.UserCredentialStore,
	svc domain.Service,
	hasher appdomain.PasswordHasher,
	d bootstrap.Deps,
) {
	handler := &commands.ChangePasswordHandler{Store: store, Service: svc, Hasher: hasher}

	fwopenapi.MountRaw(d.OpenAPIRegistry, app, fiber.MethodPatch, ChangePasswordPath,
		func(c fiber.Ctx) error {
			var cmd commands.ChangePasswordCommand
			if err := json.Unmarshal(c.Body(), &cmd); err != nil {
				return fwweb.RespondWithBadRequest(c)
			}
			appCtx := fwweb.AppContext(c)
			appCtx.SetParent(c)

			result := pipeline.Dispatch(d.Pipeline, appCtx, &cmd, handler)
			return fwweb.RespondFromResult(c, result, fiber.StatusNoContent)
		},
		fwopenapi.RawSpec{
			// Public also tells the OpenAPI assembler to leave the bearerAuth
			// entry off this operation — so the page does not show a padlock on
			// the one route that must work without a token.
			Public:  true,
			Summary: "Change your own password",
			Description: "Public. Identified by e-mail, because a caller who needs a new password may be " +
				"exactly the caller who cannot obtain a token. Requires the current password. Every " +
				"credential failure — an unregistered address, a wrong password, a suspended account, a " +
				"locked one — answers the same generic refusal, deliberately: telling them apart would " +
				"enumerate every tenant's staff directory.",
			Tags: []string{"Users"},
		})
}

// mountResetPassword mounts the authenticated reset.
//
// THREE ACCEPTORS, and the order they are checked in is the order they are
// cheapest in: the subject, the row-scope bypass, then the permission. Any one
// of them is enough.
//
// The self acceptor must never depend on a grant — every user has to be able to
// rotate their own credential, and gating that on a permission would let an
// operator lock a whole tenant out of their own passwords simply by never
// issuing it. The other two are what make a helpdesk possible: `*:*` for the
// platform, `user:reset-password` for a tenant that wants to run its own.
func mountResetPassword(
	app *fiber.App,
	store commands.UserCredentialStore,
	svc domain.Service,
	d bootstrap.Deps,
) {
	handler := &commands.ResetPasswordHandler{Store: store, Service: svc}

	fwopenapi.MountRaw(d.OpenAPIRegistry, app, fiber.MethodPatch, "/users/:id/password-reset",
		func(c fiber.Ctx) error {
			appCtx := fwweb.AppContext(c)
			appCtx.SetParent(c)

			id := c.Params("id")
			if !mayResetPassword(appCtx, id) {
				return fwweb.RespondWithForbidden(c)
			}

			var cmd commands.ResetPasswordCommand
			if err := json.Unmarshal(c.Body(), &cmd); err != nil {
				return fwweb.RespondWithBadRequest(c)
			}
			cmd.SetPathID(id)

			result := pipeline.Dispatch(d.Pipeline, appCtx, &cmd, handler)
			return fwweb.RespondFromResult(c, result, fiber.StatusNoContent)
		},
		fwopenapi.RawSpec{
			Summary: "Reset a user's password",
			Description: "Requires a token. Accepted for the user themselves, for a *:* platform operator, " +
				"or for a holder of user:reset-password — user:update deliberately does NOT reach it, " +
				"because whoever can fix a typo in a name must not be able to take over an account. " +
				"Carries no current password: not knowing it is the point of a reset, and the password " +
				"it leaves must be changed on the next sign-in.",
			Tags: []string{"Users"},
		})
}

// mayResetPassword answers the three-acceptor gate.
//
// An ABSENT identity stands down and is allowed, matching authz.noIdentity for
// the rest of this entity: a nil identity happens only under auth.mode disabled,
// which the framework's own boot guard permits in the dev profile alone. An
// identity that IS present and satisfies none of the three is refused.
func mayResetPassword(ctx *configuration.AppContext, targetID string) bool {
	identity := ctx.Identity()
	if identity == nil {
		return true
	}
	if identity.Subject == targetID {
		return true
	}
	// IsSuperAdmin and never HasPermission("*:*") — the latter panics on a
	// wildcard, which on this route would be a 500 on the request a platform
	// operator makes to help a customer.
	if identity.IsSuperAdmin() {
		return true
	}
	return identity.HasPermission("user:reset-password")
}
