// Hand-written, and not a hook: no generator declares this file.
//
// The two credential operations. Neither is a shape the spec language has —
// `authz.permissions` takes a closed set of verbs and neither of these is one of
// them — so the command, the handler and the route are all by hand. The
// JUDGEMENT is not: both go through domain.GetUpdatable and the aggregate's own
// rules, so a refusal here answers in the same envelope, with the same seven
// catalogs, as every other write in this service.
//
// THEY ARE NOT THE SAME OPERATION TWICE:
//
//	CHANGE   public, no token. Identified by E-MAIL, because a caller who needs a
//	         new password may be exactly the caller who cannot obtain a token.
//	         Proves possession of the CURRENT password. One generic answer for
//	         every credential failure.
//	RESET    authenticated. Identified by the row id. Proves WHO the caller is
//	         instead, and does not know the current password. Sets the
//	         must-change flag, because the password it leaves is somebody else's
//	         choice.

package commands

import (
	"time"

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
	FindOneByEmail(ctx *configuration.AppContext, email string) (*appdomain.User, error)
	// ScopedReader rather than the repository's own FindByID: the bound reader
	// carries the request context, so the read runs under the same deadline,
	// cancellation and trace as the write that follows it.
	ScopedReader(ctx *configuration.AppContext) domain.Reader[*appdomain.User]
	Scope(ctx *configuration.AppContext, opts ...persistence.WriteOption[*appdomain.User]) domain.Writer
}

// ── the PUBLIC change ───────────────────────────────────────────────────────

// ChangePasswordCommand is what the open route binds.
//
// Email rather than an id: an unauthenticated caller does not know their own
// UUID, and the framework's public-route bypass matches an exact METHOD /path
// with no globs, so a route carrying a path parameter cannot be declared public
// at all. The by-e-mail shape is forced by the mechanism, not chosen.
type ChangePasswordCommand struct {
	pipeline.CommandBase

	Email                string `json:"email"`
	CurrentPassword      string `json:"currentPassword"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

// ChangePasswordHandler serves the only unauthenticated write in this service.
type ChangePasswordHandler struct {
	Store  UserCredentialStore
	Hasher appdomain.PasswordHasher
	// Service is the domain service the aggregate requires — the rules ask it
	// for the hash and for the unchanged-password check.
	Service domain.Service
}

// Handle runs the credential barrier and then the change.
//
// THE ORDER IS THE SECURITY DESIGN, and it is worth reading as one piece:
//
//  1. resolve the address. A miss burns a full verification against a dummy hash
//     and answers the generic refusal — without that, a miss returns in about a
//     millisecond where a hit takes about a hundred, and the timing enumerates
//     users through a response body that says nothing;
//  2. refuse a locked account with the SAME answer. A lockout that announced
//     itself would be the oracle the generic message exists to close, and it
//     would tell an attacker exactly when to come back;
//  3. verify the current password. A failure increments the counter and may
//     lock — a write that happens on a REJECTED request, which is why it is a
//     separate Scope call rather than a rule;
//  4. only then stage the new password and let the aggregate's rules judge it.
//
// Everything up to (4) answers InvalidCredentialsNotification and nothing else,
// so an unregistered address, a wrong password, a suspended user, an
// unavailable tenant and a locked account are indistinguishable. Step (4)
// reports normally: a caller who proved possession has to be told why their NEW
// password was refused, or they can never succeed.
func (h *ChangePasswordHandler) Handle(ctx *configuration.AppContext, cmd *ChangePasswordCommand) (results.None, error) {
	now := time.Now().UTC()

	user, err := h.Store.FindOneByEmail(ctx, cmd.Email)
	if err != nil || user == nil {
		// The cost IS the answer. Discarding the result is deliberate.
		h.Hasher.DummyMatches(cmd.CurrentPassword)
		return results.None{}, invalidCredentials()
	}

	if user.IsLockedAt(now) {
		h.Hasher.DummyMatches(cmd.CurrentPassword)
		return results.None{}, invalidCredentials()
	}

	if !h.Hasher.Matches(cmd.CurrentPassword, user.PasswordHash) {
		h.registerFailure(ctx, user, now)
		return results.None{}, invalidCredentials()
	}

	// The mutation goes INSIDE the apply closure rather than before the call:
	// that is what gives the framework its pre-write snapshot, which every
	// immutability rule on this aggregate compares against.
	updatable, err := domain.GetUpdatable(user, func(e *appdomain.User) error {
		e.StageNewPassword(cmd.Password, cmd.PasswordConfirmation)
		return nil
	}, persistence.ScopeService(h.Service, ctx), appdomain.ActionChangePassword)
	if err != nil {
		return results.None{}, err
	}
	// Nothing is returned but "done". A credential operation that answered with
	// the user's row would turn a password change into a profile read — and the
	// public one would do it without a token.
	return results.None{}, h.Store.Scope(ctx).Update(updatable)
}

// registerFailure persists the counter on a request that is about to be
// REFUSED.
//
// It is its own write for a reason no comment can make optional: a rejected
// aggregate write persists nothing, so a rule could not do this. The write is
// best-effort — if it fails, the caller still gets the refusal, because the
// alternative is answering 500 and telling an attacker that the address exists.
func (h *ChangePasswordHandler) registerFailure(ctx *configuration.AppContext, user *appdomain.User, now time.Time) {
	// NOT a credential action: this write must not run the password rules — it
	// carries no new password, and the whole point is that it happens on a
	// refusal.
	updatable, err := domain.GetUpdatable(user, func(e *appdomain.User) error {
		e.RegisterFailedCredentialAttempt(now)
		return nil
	}, persistence.ScopeService(h.Service, ctx), "RegisterFailedCredentialAttempt")
	if err != nil {
		return
	}
	_ = h.Store.Scope(ctx).Update(updatable)
}

// invalidCredentials is the ONE answer every credential failure gets.
//
// It is a function rather than a value so each refusal carries its own
// notification instance — and so there is exactly one place to change if the
// wording ever moves.
func invalidCredentials() error {
	return domain.SingleNotificationError("User", "Email", appdomain.InvalidCredentialsNotification{})
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
