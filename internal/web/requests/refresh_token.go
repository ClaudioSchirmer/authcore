// Hand-written, and not a hook: no generator declares this file.
//
// The ROTATION's wire shape. One file per operation, per the layout standard; the
// response it answers with is shared with the sign-in and declared beside it.

package requests

import "github.com/ClaudioSchirmer/authcore/internal/application/commands"

// RefreshTokenRequest is the body of the rotation.
//
// It carries the opaque refresh value and NOTHING else — no subject, no tenant,
// no claims. Everything about who this is and what they may do is re-read from
// the database at redemption time, which is the whole reason a rotation propagates
// a revoked permission within minutes instead of at the next full sign-in. A
// caller-supplied identity here would be a caller-supplied authorization.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken" example:"Qk9HVVMtUkVGUkVTSC1UT0tFTi1FWEFNUExF"`
}

// ToCommand maps the wire shape onto the application command.
func (r RefreshTokenRequest) ToCommand() *commands.RefreshTokenCommand {
	return &commands.RefreshTokenCommand{RefreshToken: r.RefreshToken}
}
