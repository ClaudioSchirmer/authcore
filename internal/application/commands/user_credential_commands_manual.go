// Hand-written, and not a hook: no generator declares this file.
//
// The password reset. It is not a shape the spec language has —
// `authz.permissions` takes a closed set of verbs and this is not one of them — so
// the command, the handler and the route are all by hand. The
// JUDGEMENT is not: both go through domain.GetUpdatable and the aggregate's own
// rules, so a refusal here answers in the same envelope, with the same seven
// catalogs, as every other write in this service.
//
// THEY ARE NOT THE SAME OPERATION TWICE:
//
// A SECOND OPERATION USED TO LIVE HERE — a public change-password by e-mail,
// verifying the current password — and was removed on 2026-08-26. It rested on a
// premise this service does not have: that somebody needing a new password might
// be unable to obtain a token. There is no login route here, so nothing blocks
// authentication for a caller who knows their password, and the endpoint
// therefore did what the reset does, without a token. What it was reaching for
// is a FORGOT-PASSWORD flow, which needs e-mail and an expiring link and has not
// been started.

package commands

import (
	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/persistence"
	"github.com/ClaudioSchirmer/omnicore/application/pipeline"
	"github.com/ClaudioSchirmer/omnicore/application/results"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// UserCredentialStore is what the two handlers need from infra, and no more.
//
// It is declared HERE rather than taken as *infra.UserRepository so the
// application layer keeps depending on an interface it owns: the handlers need
// to find a user by e-mail, find one by id, and write one, and naming exactly
// those three is what keeps a later change to the repository from reaching in.
type UserCredentialStore interface {
	// ScopedReader rather than the repository's own FindByID: the bound reader
	// carries the request context, so the read runs under the same deadline,
	// cancellation and trace as the write that follows it.
	ScopedReader(ctx *configuration.AppContext) domain.Reader[*appdomain.User]
	Scope(ctx *configuration.AppContext, opts ...persistence.WriteOption[*appdomain.User]) domain.Writer
}

// ── the AUTHENTICATED reset ─────────────────────────────────────────────────

// ResetPasswordCommand is what the authenticated route binds. It carries no
// current password: not knowing it is the whole point of a reset.
type ResetPasswordCommand struct {
	pipeline.CommandWithBodyIDBase

	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

// ResetPasswordHandler serves the operator-and-self path.
type ResetPasswordHandler struct {
	Store   UserCredentialStore
	Service domain.Service
}

// Handle applies the reset. AUTHORIZATION HAPPENS AT THE ROUTE, not here — the
// three acceptors (self, the row-scope bypass, or the reset permission) are a
// web-layer question, and mixing them into the handler would put a claim check
// in the layer that is supposed to be transport-agnostic.
//
// A missing user answers the framework's own not-found, NOT the generic
// credential refusal: this caller is authenticated and already holds a
// permission or is the subject, so there is no enumeration to protect against
// and a 404 is the honest answer.
func (h *ResetPasswordHandler) Handle(ctx *configuration.AppContext, cmd *ResetPasswordCommand) (results.None, error) {
	user, err := h.Store.ScopedReader(ctx).FindByID(domain.NewID(cmd.PathID()))
	if err != nil {
		return results.None{}, err
	}

	updatable, err := domain.GetUpdatable(user, func(e *appdomain.User) error {
		e.StageNewPassword(cmd.Password, cmd.PasswordConfirmation)
		feedRowScope(ctx, e)
		return nil
	}, persistence.ScopeService(h.Service, ctx), appdomain.ActionResetPassword)
	if err != nil {
		return results.None{}, err
	}
	// Nothing is returned but "done". A credential operation that answered with
	// the user's row would turn a password change into a profile read — and the
	// public one would do it without a token.
	return results.None{}, h.Store.Scope(ctx).Update(updatable)
}

// feedRowScope fills the identity-derived fields the aggregate's row-scope guard
// reads. It is the same three lines every generated command mapper writes, and
// it is here for the same reason: a hand-written command that skips them leaves
// refuseForeignTenant standing down, because that guard asks whether an identity
// was PRESENT before it compares anything.
//
// The consequence of forgetting it is not theoretical and it is not visible in a
// green build: a holder of user:reset-password in tenant A could reset a
// password in tenant B, and the read side would hide the damage from the side
// that caused it. The permission on the route says WHO may attempt the verb; it
// says nothing about WHOSE row.
//
// On the PUBLIC change there is no identity, so nothing is filled and the guard
// stands down — which is correct there and is the one route in this service
// where that is true in production. The e-mail is globally unique, so the row
// the caller proved possession of is the only row they can reach.
func feedRowScope(ctx *configuration.AppContext, e *appdomain.User) {
	id := ctx.Identity()
	if id == nil {
		return
	}
	e.RequestingIdentityPresent = true
	e.RequestingTenant = id.TenantID()
	// A super-admin crosses the scope. Not asked through HasPermission, which
	// panics on the *:* the claim carries — the wildcard has its own question.
	e.RequestingMayCrossScope = id.IsSuperAdmin()
}
