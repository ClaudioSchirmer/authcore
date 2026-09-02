// Hand-written, and not a hook: no generator declares this file.
//
// The RESET's wire shape — an operator setting somebody else's credential. One
// file per operation, per the layout standard.

package requests

import "github.com/ClaudioSchirmer/authcore/internal/application/commands"

// ResetPasswordRequest is the body of the helpdesk reset.
//
// No current password: the caller is identified by their token and the target by
// the path, and not knowing the current password is the whole point of a reset.
type ResetPasswordRequest struct {
	Password             string `json:"password" example:"An0ther!Passphrase"`
	PasswordConfirmation string `json:"passwordConfirmation" example:"An0ther!Passphrase"`
}

// ResetUserPasswordGraphQLResponse acknowledges the reset, and says nothing
// else.
type ResetUserPasswordGraphQLResponse struct {
	Success bool `json:"success"`
}

// ToCommand maps the wire shape onto the application command.
func (r ResetPasswordRequest) ToCommand() *commands.ResetPasswordCommand {
	return &commands.ResetPasswordCommand{
		Password:             r.Password,
		PasswordConfirmation: r.PasswordConfirmation,
	}
}
