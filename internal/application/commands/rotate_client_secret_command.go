// Hand-written, and not a hook: no generator declares this file.
//
// The secret rotation's COMMAND and RESULT, one level above the handler, per the
// layout standard.

package commands

import (
	"time"

	"github.com/ClaudioSchirmer/omnicore/application/pipeline"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// RotateClientSecretCommand is what the rotate route binds.
//
// The grace window is a POINTER, and that is the whole reason this type does not
// carry a plain int: ZERO and ABSENT are different requests. Zero means "kill
// the old secret now", which is exactly what somebody rotating a LEAKED
// credential is asking for; absent means "use the default", which is a day. A
// value type would collapse the two and quietly turn every unstated rotation
// into an immediate cutover — the outage this overlap exists to prevent.
type RotateClientSecretCommand struct {
	pipeline.CommandWithBodyIDBase

	GracePeriodSeconds *int `json:"gracePeriodSeconds"`
}

// RotateClientSecretResult is the ONE result in this service that carries a
// credential.
//
// It deliberately does not carry the client's row. What a caller needs from a
// rotation is the new secret and the deadline the old one now has; everything
// else about the client they already had, and answering with it would put a
// credential response on the same page as a profile read.
type RotateClientSecretResult struct {
	ID     domain.ID
	Secret string
	// SecretChangedAt is when this rotation happened — the value the row now
	// carries, so a caller can confirm the write landed without a second read.
	SecretChangedAt time.Time
	// PreviousSecretExpiresAt is when the OLD secret stops working. Absent means
	// it already has: either the caller asked for a zero window, or there was
	// nothing to retire.
	PreviousSecretExpiresAt *time.Time
}
