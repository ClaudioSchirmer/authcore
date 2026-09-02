// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to ONE endpoint.
//
// The client half of the claim edge, read without entering the aggregate.
//
// ONE SCHEMA PER FILE, per the layout standard's "No bundling" rule for schemas/.
// The sign-in reads a SUBSET of each table on purpose, so every one of these has a
// generated twin describing the same rows for the write side; the test next door
// asserts the two agree, column by column.

package schemas

import (
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// ClientClaimEdge is UserClaimEdge's twin over client_claims. Two types and not
// one shared: a Direct repository cross-checks its type parameter against the
// schema's anchored type — one schema, one row type — so a single struct behind
// both tables would be refused at construction.
type ClientClaimEdge struct {
	ID      domain.ID
	ClaimID domain.ID
}

// ClientClaimEdgeSchema maps ClientClaimEdge to client_claims. Same shape, same
// reasoning, the other side of the chain.
func ClientClaimEdgeSchema() *core.TableSchema {
	return core.NewDirectSchema[ClientClaimEdge]("client_claims").
		ID("id").
		Field("ClaimID", "claim_id").
		DeletedAt("deleted_at")
}
