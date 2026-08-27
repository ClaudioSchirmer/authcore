// Hand-written, and not a hook: no generator declares this file.
//
// The wire shapes for the secret rotation. They exist as their own types, in the
// same package as every generated request, for the reason the layering already
// implies: the command is an APPLICATION value and the body is a WEB one, and
// the `example:` tags below are presentation — they belong where the other
// request DTOs keep theirs, not on a command.

package requests

import (
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
)

// RotateClientSecretRequest is the body of the rotation.
//
// ONE OPTIONAL FIELD, and the pointer is the contract rather than a style. ZERO
// and ABSENT are different requests: zero means "the old secret stops working
// now", which is what somebody rotating a LEAKED credential is asking for, and
// absent means "give me the usual day". A value type would collapse the two and
// turn every unstated rotation into an immediate cutover — the outage the
// overlap exists to prevent.
type RotateClientSecretRequest struct {
	// GracePeriodSeconds is how long the OLD secret keeps working, 0 to 604800.
	// Omit it for 86400 (a day).
	GracePeriodSeconds *int `json:"gracePeriodSeconds,omitempty" example:"86400"`
}

// ToCommand maps the wire shape onto the application command.
//
// It takes NO id, and that is the framework's contract rather than an omission:
// RequestDTO[TCmd] declares `ToCommand() TCmd`, and the CommandWithBodyID
// wrapper calls SetPathID itself from the route's own segment.
func (r RotateClientSecretRequest) ToCommand() *commands.RotateClientSecretCommand {
	return &commands.RotateClientSecretCommand{GracePeriodSeconds: r.GracePeriodSeconds}
}

// RotateClientSecretResponse is the ONLY response in this service that carries a
// credential, and the only place the plaintext can ever be read.
//
// THE SECRET IS SHOWN ONCE. Nothing stores it — the column holds a SHA-256 —
// so there is no endpoint that could return it again, and a caller who loses it
// rotates rather than looks it up. Say that in the OpenAPI description, because
// a caller who assumes otherwise finds out at the worst moment.
//
// It deliberately does not carry the client's row: what a caller needs from a
// rotation is the new secret and the deadline the old one now has. Answering
// with the whole aggregate would put a credential response on the same page as a
// profile read.
//
// FromResult is written BY HAND rather than through the framework's generic
// mapper: `Auto` reads every field from a same-named Result field, and the id
// crosses as text here while the Result carries a domain type.
type RotateClientSecretResponse struct {
	ID string `json:"id" example:"7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"`
	// Secret is the plaintext, shown once and never again.
	Secret string `json:"secret" example:"acs_8xQvR2mK9dLpN4wZ7tYcB1hJ6sF3gA5eU0iO8rTvXyM"`
	// SecretChangedAt is when this rotation happened.
	SecretChangedAt time.Time `json:"secretChangedAt" example:"2026-08-26T14:03:11Z"`
	// PreviousSecretExpiresAt is when the OLD secret stops working. Absent means
	// it already has — either a zero window was asked for, or there was nothing
	// to retire.
	PreviousSecretExpiresAt *time.Time `json:"previousSecretExpiresAt,omitempty" example:"2026-08-27T14:03:11Z"`
}

// FromResult projects the application Result onto the response.
func (RotateClientSecretResponse) FromResult(r commands.RotateClientSecretResult) RotateClientSecretResponse {
	return RotateClientSecretResponse{
		ID:                      r.ID.Value(),
		Secret:                  r.Secret,
		SecretChangedAt:         r.SecretChangedAt,
		PreviousSecretExpiresAt: r.PreviousSecretExpiresAt,
	}
}
