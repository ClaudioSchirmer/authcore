// Hand-written, and not a hook: no generator declares this file.
//
// The wire shapes for the two credential operations. They exist as their own
// types, in the same package as every generated request, for the reason the
// layering already implies: the command is an APPLICATION value and the body is
// a WEB one, and the `example:` tags below are presentation — they belong where
// the other request DTOs keep theirs, not on a command.
//
// TWO TYPES AND NOT ONE WITH AN OPTIONAL FIELD. A single body carrying a
// currentPassword that is required on one route and ignored on the other is a
// schema that documents neither: the OpenAPI page would show one operation's
// field on the other's form, and the "required" would have to live in prose. The
// split is what lets each page say exactly what its own call takes.

package requests

import (
	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	fwresults "github.com/ClaudioSchirmer/omnicore/application/results"
)

// ChangePasswordRequest is the body of the self-service change.
//
// The current password is what separates this operation from the reset: the
// caller proves the credential they hold before replacing it.
type ChangePasswordRequest struct {
	CurrentPassword      string `json:"currentPassword" example:"Str0ng!Passphrase"`
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
func (r ChangePasswordRequest) ToCommand() *commands.ChangePasswordCommand {
	return &commands.ChangePasswordCommand{
		CurrentPassword:      r.CurrentPassword,
		Password:             r.Password,
		PasswordConfirmation: r.PasswordConfirmation,
	}
}

// ── the GraphQL payloads ────────────────────────────────────────────────────
//
// TWO TYPES AND NOT ONE SHARED ACKNOWLEDGEMENT, for the reason the two request
// bodies are also two types: the schema names a payload after the field it
// answers, so a shared Go type would still publish two SDL types — and the day
// one of them grows a field, the other would grow it too, silently.
//
// The REST verbs answer 204 with no body. A GraphQL field cannot: it must
// resolve to a type, and a payload with no fields is not publishable. So each is
// a single true, exactly as every other mutation in this service that mirrors a
// 204. Nothing about the credential is echoed back — that is as true here as it
// is on REST, and for the same reason.

// ChangeUserPasswordGraphQLResponse acknowledges the change, and says nothing
// else.
type ChangeUserPasswordGraphQLResponse struct {
	Success bool `json:"success"`
}

// FromResult answers true: the pipeline only projects a result it succeeded
// with.
func (ChangeUserPasswordGraphQLResponse) FromResult(fwresults.None) ChangeUserPasswordGraphQLResponse {
	return ChangeUserPasswordGraphQLResponse{Success: true}
}

// FromResult answers true: the pipeline only projects a result it succeeded
// with.
func (ResetUserPasswordGraphQLResponse) FromResult(fwresults.None) ResetUserPasswordGraphQLResponse {
	return ResetUserPasswordGraphQLResponse{Success: true}
}
