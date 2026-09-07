// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to ONE endpoint.
//
// THE NETWORK RANGES this client may authenticate from.
//
// IT IS ITS OWN ANCHOR and cannot ride as a join on the client row: a 1:N join fans
// the root out to one row per entry, and the loader there answers "one row is the
// client, two is a data problem". It is read as a collection, in the same concurrent
// burst as the grants.
//
// The anchor's own scope drops an archived entry, which is the whole gate this
// collection needs: revoking a range IS archiving the entry, and an archived range
// must stop admitting immediately.
//
// ONE SCHEMA PER FILE, per the layout standard's "No bundling" rule for schemas/.
// The sign-in reads a SUBSET of each table on purpose, so this has a generated twin
// describing the same rows for the write side; the test next door asserts the two
// agree, column by column.

package schemas

import (
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// ClientAllowedRange is one network range this client may authenticate from.
//
// The LABEL is not read: it exists for the operator auditing the collection, and a
// decision about whether an address is inside a prefix has no use for it. Loading a
// column nothing consults would be paid on every machine sign-in.
type ClientAllowedRange struct {
	ID   domain.ID
	CIDR string
}

// ClientAllowedRangeSchema maps ClientAllowedRange to `client_allowed_cidrs`.
//
// The anchor's own scope drops an archived entry, which is the whole gate this
// collection needs: revoking a range IS archiving the entry, and an archived range
// must stop admitting immediately.
func ClientAllowedRangeSchema() *core.TableSchema {
	return core.NewDirectSchema[ClientAllowedRange]("client_allowed_cidrs").
		ID("id").
		ParentID("client_id").
		Field("CIDR", "cidr").
		ArchivedAt("archived_at")
}
