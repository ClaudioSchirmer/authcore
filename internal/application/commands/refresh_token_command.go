// Hand-written, and not a hook: no generator declares this file.
//
// The ROTATION's command. One command per verb, per the layout standard; the
// Result it answers with is shared with the sign-in and lives in utils/.

package commands

import "github.com/ClaudioSchirmer/omnicore/application/pipeline"

// RefreshTokenCommand is what the rotation route binds.
type RefreshTokenCommand struct {
	pipeline.CommandWithBodyBase

	RefreshToken string `json:"refreshToken"`
}
