// Hand-written, and not a hook: no generator declares this file.
//
// The RESET — an operator setting somebody else's credential, as opposed to the
// owner changing their own. One file per hand-written handler, per the layout
// standard; the store port and the row-scope feed they share live beside the
// change handler next door.

package handlers

import (
	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/utils"
	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/persistence"
	"github.com/ClaudioSchirmer/omnicore/application/results"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// ResetPasswordHandler serves the helpdesk path.
type ResetPasswordHandler struct {
	Store   utils.UserCredentialStore
	Service domain.Service
}

// Handle applies the reset.
//
// A missing user answers the framework's own not-found, NOT a generic credential
// utils.Refusal: this caller is authenticated and already holds the permission, so
// there is no enumeration to protect against and a 404 is the honest answer.
func (h *ResetPasswordHandler) Handle(ctx *configuration.AppContext, cmd *commands.ResetPasswordCommand) (results.None, error) {
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
