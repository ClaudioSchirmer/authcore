// Hand-written, and not a hook: no generator declares this file.
//
// The RESET's command — an operator setting somebody else's credential. One
// command per verb, per the layout standard. It answers with results.None, so
// there is no Result to co-locate.

package commands

import "github.com/ClaudioSchirmer/omnicore/application/pipeline"

// ResetPasswordCommand is what the reset route binds. It carries no current
// password: not knowing it is the whole point of a reset.
type ResetPasswordCommand struct {
	pipeline.CommandWithBodyIDBase

	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}
