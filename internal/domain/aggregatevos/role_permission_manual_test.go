// Hand-written: the two branches of the grant entry the generated suite does
// not reach, and both of them are load-bearing.
//
// `../../../specs/scaffold-entity/role/task_tests.md` asks for sameness to be
// pinned including "that two entries pointing at the same permission stay the
// same entry whatever their join fields say". That is the case below, and it is
// the one that fails open if anybody ever "simplifies" IsSameBusinessIdentity
// into a whole-struct comparison.

package aggregatevos

import (
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The entry carries three fields and only one of them decides sameness.
//
// The other two are read-join values: filled on an entry LOADED from the row,
// blank on one a write just added. A comparison over all three would answer
// "different" for a grant that duplicates a stored one — same permission, blank
// Resource on one side, populated Resource on the other — and the duplicate
// guard would fail open. Naming PermissionID explicitly is what keeps it shut.
func TestSamenessIgnoresTheJoinFieldsEntirely(t *testing.T) {
	id := domain.NewID("9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a3")

	justAdded := RolePermission{PermissionID: id}
	loaded := RolePermission{
		PermissionID: id,
		Resource:     "tenant",
		Action:       "read",
	}

	if !justAdded.IsSameBusinessIdentity(loaded) {
		t.Error("a freshly added grant was not recognised as the stored one it duplicates — the duplicate guard is open")
	}
	if !loaded.IsSameBusinessIdentity(justAdded) {
		t.Error("sameness is not symmetric")
	}
}

// A value of another type is never the same entry. Without the type check the
// assertion would panic on the framework's own traversal of a mixed collection.
func TestSamenessRefusesAValueOfAnotherType(t *testing.T) {
	grant := RolePermission{PermissionID: domain.NewID("9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a3")}

	if grant.IsSameBusinessIdentity(otherAggregateValueObject{}) {
		t.Error("an entry of a different type was reported as the same entry")
	}
}

type otherAggregateValueObject struct{ domain.Managed }

func (otherAggregateValueObject) CollectionName() string { return "Others" }
func (otherAggregateValueObject) IsSameBusinessIdentity(domain.AggregateValueObject) bool {
	return false
}
func (otherAggregateValueObject) BuildRules(string, domain.Service, *domain.Rules) {}

// The entry declares no rule of its own: its single stored field is an id the
// root's rules judge through the domain service, and a rule here would be a
// second, weaker copy of that. Pinning the emptiness is what makes a future
// addition a deliberate act.
func TestTheEntryDeclaresNoRuleOfItsOwn(t *testing.T) {
	ctx := domain.NewNotificationContext("RolePermission")
	r := domain.NewRules(domain.ModeInsert, ctx, nil)

	RolePermission{}.BuildRules("GetInsertable", nil, r)

	if ctx.HasErrors() {
		t.Error("the grant entry raised a notification of its own — the root's rules are where a grant is judged")
	}
}
