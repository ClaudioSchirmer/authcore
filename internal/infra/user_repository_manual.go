// Hand-written, and NOT part of the generated repository — which is the whole
// point of it being its own file.
//
// The first version of this method was appended to user_repository.go, and
// `omnicore-gen doctor` caught it within the hour: that file is owned and
// hashed, so the next regeneration would have REFUSED it and left the edit
// stranded — the service would still build, and the refusal would surface as a
// line in a report nobody was reading yet.
//
// A method on a generated type does not have to live in the generated file. Go
// puts methods wherever the package does, so this costs nothing and keeps the
// generated tree regenerable.

package infra

import (
	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// FindOneByEmail resolves an address to the one user that holds it.
//
// HAND-WRITTEN, and the only read on this repository that is not scoped by a
// tenant — because the one caller has no tenant to scope by. The public
// change-password route carries no token, so there is no claim, and what makes
// that safe rather than reckless is the README's central decision: the e-mail is
// unique across the WHOLE platform over active rows, so one address resolves to
// exactly one user or to none.
//
// The ACTIVE scope is the query default and is left alone: an archived user has
// released their address, and a caller presenting it is not that person.
//
// It returns (nil, nil) for an address nobody holds rather than an error. The
// caller's next move is a generic refusal either way, and forcing it to
// distinguish "not found" from "the query failed" is how a 500 ends up telling
// an attacker that a row exists.
func (r *UserRepository) FindOneByEmail(ctx *configuration.AppContext, email string) (*appdomain.User, error) {
	if email == "" {
		return nil, nil
	}
	found, err := r.Loader.FindOne(ctx, criteria.Where(criteria.Eq("Email", email)))
	if err != nil {
		if isRecordNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return found, nil
}
