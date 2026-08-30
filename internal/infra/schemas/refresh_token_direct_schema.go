// Hand-written, and no generator declares it: a refresh token is not a business
// aggregate, and migrations/postgres/0006_refresh_tokens_manual.up.sql explains at
// length why it has no revision, no archive stamp and no REST surface.
//
// What it DOES have, since that migration was corrected, is this service's
// ordinary identity: a UUID `id` primary key with the hash carrying a UNIQUE index
// beside it. That is what lets the table be described here once and read and
// written through the framework's own Direct engine, instead of through
// hand-rolled SQL that had to render placeholders, quoting and argument encoding
// per dialect.

package schemas

import (
	"time"

	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// RefreshToken is one row of authentication_refresh_tokens.
//
// It is an INFRASTRUCTURE row and not a domain type: it exists to be scanned into
// and written from, and the port's own authcore.RefreshTokenRecord is what crosses
// back to the framework. Keeping the two apart is what lets the column set change
// without touching a contract the framework owns.
//
// Audience is the JSON array as stored, not a decoded slice. The decode belongs to
// the store, which is where the "unreadable audience degrades to empty" decision
// lives; a schema that decoded it would have to decide what a bad value means, and
// that is not a schema's call.
//
// CreatedAt is absent on purpose: the column exists and the database's own DEFAULT
// fills it, nothing reads it back, and declaring it here would only add a slot for
// the framework to stamp.
type RefreshToken struct {
	ID        domain.ID
	Hash      string
	FamilyID  string
	Subject   string
	Audience  string
	ExpiresAt time.Time
	Used      bool
	Revoked   bool
}

// RefreshTokenSchema maps RefreshToken to authentication_refresh_tokens.
//
// NO DeletedAt, and its absence is a decision the migration already argued: an
// expired token is DELETED outright, because keeping the hash of a dead credential
// earns nothing and costs a growing table. Declaring an archive column here would
// also silently gate every read on it — against a column that does not exist.
func RefreshTokenSchema() *core.TableSchema {
	return core.NewDirectSchema[RefreshToken]("authentication_refresh_tokens").
		ID("id").
		Field("Hash", "hash").
		Field("FamilyID", "family_id").
		Field("Subject", "subject").
		Field("Audience", "audience").
		Field("ExpiresAt", "expires_at").
		Field("Used", "used").
		Field("Revoked", "revoked")
}
