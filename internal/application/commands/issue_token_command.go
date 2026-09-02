// Hand-written, and not a hook: no generator declares this file.
//
// The two token operations' COMMANDS and RESULTS. They live here, one level above
// the handlers that consume them, because that is where the layout standard puts
// them: "the Command/Query and its Result stay one level up beside the Auto ones;
// only the handler moves down".
//
// THE RESULT CARRIES MORE THAN THE TOKEN, deliberately. The access token's claims
// are kept to identity and authorization — they ride in a header on every request
// to every service, and a group description is exactly the field that grows
// unnoticed until a proxy truncates the header — so the richer profile travels
// here, in a body read once at sign-in and never re-sent.

package commands

import "github.com/ClaudioSchirmer/omnicore/application/pipeline"

// IssueTokenCommand is what the sign-in route binds.
type IssueTokenCommand struct {
	pipeline.CommandWithBodyBase

	Email    string `json:"email"`
	Password string `json:"password"`
}
