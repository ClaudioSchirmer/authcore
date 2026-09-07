// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to ONE endpoint.
//
// LEVEL 1 OF THE CLAIM CHAIN: the value this client holds for one definition.
//
// NO JOIN INTO THE CATALOG, and that is a correction rather than an omission: the
// resolution walks the catalog — which is the vocabulary — and looks each
// definition's value up here by id. A join filling a name and a value type on every
// entry would be paid on every sign-in and discarded.
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

// HeldClientClaimValue is level 1: the value this client holds for one definition.
//
// It is a separate type from HeldClaimValue next door, and not a shared one, for
// the reason every schema here is per-path: a schema is checked against the type it
// is anchored to, and these two anchor on different tables.
//
// NO JOIN INTO THE CATALOG, for the same reason the user's has none: the resolution
// walks the catalog — which is the vocabulary — and looks each definition's value up
// here by id. A join filling a name and a value type on every entry would be paid
// on every sign-in and discarded.
type HeldClientClaimValue struct {
	ID      domain.ID
	ClaimID domain.ID
	Value   string
}

// HeldClientClaimValueSchema maps HeldClientClaimValue to `client_claims`.
func HeldClientClaimValueSchema() *core.TableSchema {
	return core.NewDirectSchema[HeldClientClaimValue]("client_claims").
		ID("id").
		ParentID("client_id").
		Field("ClaimID", "claim_id").
		Field("Value", "value").
		ArchivedAt("archived_at")
}
