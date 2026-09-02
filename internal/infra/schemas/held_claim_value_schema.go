// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to the sign-in path.
//
// LEVEL 1 OF THE CLAIM CHAIN: the value this user holds for one definition.
//
// NO JOIN INTO THE CATALOG, and that is a correction rather than an omission. The
// User aggregate's own child join fills a name and a value type on every entry, and
// the resolution reads NEITHER — it walks the catalog, which is the vocabulary, and
// looks each definition's value up here by id. That join was paid on every sign-in
// and discarded.
//
// ONE SCHEMA PER FILE, per the layout standard's "No bundling" rule for schemas/.
// The sign-in reads a SUBSET of each table on purpose, so this has a generated twin
// describing the same rows for the write side.

package schemas

import (
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// HeldClaimValue is level 1: the value this user holds for one definition.
//
// NO JOIN INTO THE CATALOG, and that is a correction rather than an omission. The
// User aggregate's own child join fills a name and a value type on every entry,
// and the resolution reads NEITHER — it walks the catalog, which is the
// vocabulary, and looks each definition's value up here by id. That join was paid
// on every sign-in and discarded.
type HeldClaimValue struct {
	ID      domain.ID
	ClaimID domain.ID
	Value   string
}

// HeldClaimValueSchema maps HeldClaimValue to `user_claims`.
func HeldClaimValueSchema() *core.TableSchema {
	return core.NewDirectSchema[HeldClaimValue]("user_claims").
		ID("id").
		ParentID("user_id").
		Field("ClaimID", "claim_id").
		Field("Value", "value").
		DeletedAt("deleted_at")
}
