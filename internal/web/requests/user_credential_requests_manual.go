// Hand-written, and not a hook: no generator declares this file.
//
// The wire shape for the password reset. It exists as its own
// type, in the same package as every generated request, for the reason the
// layering already implies: the command is an APPLICATION value and the body is
// a WEB one, and the `example:` tags below are presentation — they belong where
// the other request DTOs keep theirs, not on a command.
//
// It also exists because the first version of this route had none: it was
// mounted with a RawSpec carrying nothing but a summary, so the OpenAPI page
// rendered an operation with no body schema, no field list and no example — the
// one thing on that page a caller actually reads before trying a call.

package requests

import "github.com/ClaudioSchirmer/authcore/internal/application/commands"

// ResetPasswordRequest is the body of the authenticated reset.
//
// No current password and no e-mail: the caller is identified by their token and
// the target by the path. Not knowing the current password is the whole point of
// a reset.
type ResetPasswordRequest struct {
	Password             string `json:"password" example:"An0ther!Passphrase"`
	PasswordConfirmation string `json:"passwordConfirmation" example:"An0ther!Passphrase"`
}

// ToCommand maps the wire shape onto the application command.
//
// It takes NO id, and that is the framework's contract rather than an omission:
// RequestDTO[TCmd] declares `ToCommand() TCmd`, and the CommandWithBodyID
// wrapper calls SetPathID itself from the route's own segment. A DTO that
// reached for the id would be doing the wrapper's job with a value it had to be
// handed anyway.
func (r ResetPasswordRequest) ToCommand() *commands.ResetPasswordCommand {
	return &commands.ResetPasswordCommand{
		Password:             r.Password,
		PasswordConfirmation: r.PasswordConfirmation,
	}
}
