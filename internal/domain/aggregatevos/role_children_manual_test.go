package aggregatevos

import (
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The generated role_children_test.go pins the collection name and proves
// sameness between two RolePermission entries. What it does not reach is the
// type guard and the entry's own rule pass — both cheap, and both load-bearing.

// otherChild is an aggregate value object of a DIFFERENT type, used to prove
// the type guard below. It is deliberately minimal: nothing about it needs to
// be meaningful except that it is not a RolePermission.
type otherChild struct {
	domain.Managed
}

func (otherChild) CollectionName() string { return "Others" }

func (otherChild) IsSameBusinessIdentity(domain.AggregateValueObject) bool { return false }

func (otherChild) BuildRules(string, domain.Service, *domain.Rules) {}

// A RolePermission is never "the same entry" as a value of another type.
//
// The framework matches children through this method, so an unguarded type
// assertion here would panic on a mixed collection rather than answer false —
// and an aggregate with two child types is one spec edit away.
func TestRoleRolePermission_IsNeverTheSameAsAnotherChildType(t *testing.T) {
	entry := RolePermission{PermissionID: domain.NewID("9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a3")}

	if entry.IsSameBusinessIdentity(otherChild{}) {
		t.Error("a RolePermission reported itself the same entry as a different child type")
	}
}

// The entry carries no rule of its own: its single field is a plain id, and the
// three questions worth asking about it — is it in the catalog, is it a
// wildcard, does the caller hold it — need the catalog, which one entry cannot
// see. They live on the root, where the domain service is reachable.
//
// The pass is exercised anyway, because "no rules" is a claim that only stays
// true while nothing is added without a test noticing.
func TestRoleRolePermission_RaisesNothingOnItsOwn(t *testing.T) {
	ctx, rules := rulesForRoleRolePermission()

	entry := RolePermission{PermissionID: domain.NewID("9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a3")}
	entry.BuildRules("GetInsertable", nil, rules)

	if ctx.HasErrors() {
		t.Errorf("a RolePermission entry raised something on its own: %v", ctx.Messages())
	}
}

// An entry with no permission id raises nothing HERE either — the same claim,
// asserted on the value that would most plausibly break it. Presence is the
// root's business (the catalog probe refuses an id that resolves to nothing),
// not the entry's.
func TestRoleRolePermission_EmptyIDIsStillNotTheEntrysProblem(t *testing.T) {
	ctx, rules := rulesForRoleRolePermission()

	RolePermission{}.BuildRules("GetInsertable", nil, rules)

	if ctx.HasErrors() {
		t.Errorf("an empty RolePermission raised something on its own: %v", ctx.Messages())
	}
}
