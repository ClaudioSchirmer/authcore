// Hand-written, and not a hook: no generator declares this file.
//
// The two credential operations. Neither is a shape the spec language has —
// `authz.permissions` takes a closed set of verbs and neither of these is one of
// them — so the commands, the handlers and the routes are all by hand. The
// JUDGEMENT is not: both go through domain.GetUpdatable and the aggregate's own
// rules, so a refusal here answers in the same envelope, with the same seven
// catalogs, as every other write in this service.
//
// THEY ARE NOT THE SAME OPERATION TWICE:
//
//	the CHANGE is the caller setting their own password, proving the one they
//	hold today, and it clears the must-change flag;
//
//	the RESET is somebody else setting it, proving nothing, and it sets that
//	flag so the next sign-in has to rotate what a stranger chose.
//
// They are kept disjoint by the aggregate — the change refuses any row but the
// caller's, the reset refuses the caller's own — so the URL a caller picks
// cannot be used to skip the proof the other one demands.
//
// A PUBLIC change-password by e-mail used to live here and was removed on
// 2026-08-26. It rested on a premise this service does not have: that somebody
// needing a new password might be unable to obtain a token. What it was reaching
// for is a FORGOT-PASSWORD flow, which needs e-mail and an expiring link and has
// not been started.

package commands

import (
	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/persistence"
	"github.com/ClaudioSchirmer/omnicore/application/pipeline"
	"github.com/ClaudioSchirmer/omnicore/application/results"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// UserCredentialStore is what the handlers need from infra, and no more.
//
// It is declared HERE rather than taken as *infra.UserRepository so the
// application layer keeps depending on an interface it owns: the handlers need
// to read one user and write one, and naming exactly those two is what keeps a
// later change to the repository from reaching in.
type UserCredentialStore interface {
	// ScopedReader rather than the repository's own FindByID: the bound reader
	// carries the request context, so the read runs under the same deadline,
	// cancellation and trace as the write that follows it.
	ScopedReader(ctx *configuration.AppContext) domain.Reader[*appdomain.User]
	Scope(ctx *configuration.AppContext, opts ...persistence.WriteOption[*appdomain.User]) domain.Writer
}

// ── the CHANGE ──────────────────────────────────────────────────────────────

// ChangePasswordCommand is what the change route binds.
//
// It carries the current password, which is the whole difference between this
// operation and the reset below.
type ChangePasswordCommand struct {
	pipeline.CommandWithBodyIDBase

	CurrentPassword      string `json:"currentPassword"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

// ChangePasswordHandler serves the self-service path.
type ChangePasswordHandler struct {
	Store   UserCredentialStore
	Service domain.Service
}

// Handle applies the change.
//
// The ROW DECISION — that the id on the path is the caller's own — is NOT here.
// It lives in the aggregate, beside the tenant scope guard and every other rule,
// because it is authorization over a row rather than admission to a route: the
// permission on the route already answered who may attempt the verb. Putting it
// here would also put it out of reach of the notification context, and the
// caller would get a bare status instead of a translated answer.
func (h *ChangePasswordHandler) Handle(ctx *configuration.AppContext, cmd *ChangePasswordCommand) (results.None, error) {
	user, err := h.Store.ScopedReader(ctx).FindByID(domain.NewID(cmd.PathID()))
	if err != nil {
		return results.None{}, err
	}

	updatable, err := domain.GetUpdatable(user, func(e *appdomain.User) error {
		e.StagePasswordChange(cmd.CurrentPassword, cmd.Password, cmd.PasswordConfirmation)
		feedRowScope(ctx, e)
		return nil
	}, persistence.ScopeService(h.Service, ctx), appdomain.ActionChangePassword)
	if err != nil {
		return results.None{}, err
	}
	return results.None{}, h.Store.Scope(ctx).Update(updatable)
}

// ── the RESET ───────────────────────────────────────────────────────────────

// ResetPasswordCommand is what the reset route binds. It carries no current
// password: not knowing it is the whole point of a reset.
type ResetPasswordCommand struct {
	pipeline.CommandWithBodyIDBase

	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

// ResetPasswordHandler serves the helpdesk path.
type ResetPasswordHandler struct {
	Store   UserCredentialStore
	Service domain.Service
}

// Handle applies the reset.
//
// A missing user answers the framework's own not-found, NOT a generic credential
// refusal: this caller is authenticated and already holds the permission, so
// there is no enumeration to protect against and a 404 is the honest answer.
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
	// the user's row would turn a password reset into a profile read.
	return results.None{}, h.Store.Scope(ctx).Update(updatable)
}

// feedRowScope fills the identity-derived fields the aggregate's row rules read.
// It is the same handful of lines every generated command mapper writes, and it
// is here for the same reason: a hand-written command that skips them leaves
// those guards standing down, because each asks whether an identity was PRESENT
// before it compares anything.
//
// The consequence of forgetting it is not theoretical and it is not visible in a
// green build: a holder of user:reset-password in tenant A could reset a
// password in tenant B, and a caller could change a password that is not theirs.
// The permission on the route says WHO may attempt the verb; it says nothing
// about WHOSE row.
func feedRowScope(ctx *configuration.AppContext, e *appdomain.User) {
	id := ctx.Identity()
	if id == nil {
		return
	}
	e.RequestingIdentityPresent = true
	e.RequestingTenant = id.TenantID()
	// Identity.Subject and not Claims["sub"] — the canonical accessor, which is
	// what the generated mappers use for this field and what the aggregate's
	// comparison is written against.
	e.RequestingUserID = id.Subject
	// A super-admin crosses the scope. Not asked through HasPermission, which
	// panics on the *:* the claim carries — the wildcard has its own question.
	e.RequestingMayCrossScope = id.IsSuperAdmin()
}
