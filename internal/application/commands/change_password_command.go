// Hand-written, and not a hook: no generator declares this file.
//
// The two credential operations' COMMANDS, one level above the handlers, per the
// layout standard. Neither answers with a result: both are results.None.

package commands

import "github.com/ClaudioSchirmer/omnicore/application/pipeline"

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
