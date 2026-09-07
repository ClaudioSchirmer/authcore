// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to the sign-in path.
//
// LEVEL 2 OF THE CLAIM CHAIN, and the VOCABULARY the resolution iterates: one
// active definition of the tenant.
//
// The walk is over these rather than over the principal's entries because of the
// DEFAULTS: a definition carrying one mints a claim for somebody who holds no value
// for it, so entering through the entries would never reach it. The archive gate
// rides along for free — the anchor's own scope — where the aggregate's join could
// not apply one.
//
// DefaultValue is a pointer because NULL is meaningful: null at both levels means
// the claim is ABSENT from the token, not empty and not zero.
//
// BOTH TOKEN ROUTES READ IT. The user path filters `appliesTo` to user|both, the
// machine path to client|both, which is why the schema is shared and the predicate
// is not.
//
// ONE SCHEMA PER FILE, per the layout standard's "No bundling" rule for schemas/.
// The sign-in reads a SUBSET of each table on purpose, so this has a generated twin
// describing the same rows for the write side.

package schemas

import (
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// ClaimDefinition is level 2, and the VOCABULARY the resolution iterates: one
// active definition of the tenant.
//
// The walk is over these rather than over the user's entries because of the
// DEFAULTS: a definition carrying one mints a claim for a user who holds no value
// for it, so entering through the entries would never reach it. The archive gate
// rides along for free — the anchor's own scope — where the aggregate's join could
// not apply one.
//
// DefaultValue is a pointer because NULL is meaningful: null at both levels means
// the claim is ABSENT from the token, not empty and not zero.
type ClaimDefinition struct {
	ID           domain.ID
	TenantID     domain.ID
	Name         string
	ValueType    string
	AppliesTo    string
	DefaultValue *string
}

// ClaimDefinitionSchema maps ClaimDefinition to `claims`.
func ClaimDefinitionSchema() *core.TableSchema {
	return core.NewDirectSchema[ClaimDefinition]("claims").
		ID("id").
		Field("TenantID", "tenant_id").
		Field("Name", "name").
		Field("ValueType", "value_type").
		Field("AppliesTo", "applies_to").
		Field("DefaultValue", "default_value").
		ArchivedAt("archived_at")
}
