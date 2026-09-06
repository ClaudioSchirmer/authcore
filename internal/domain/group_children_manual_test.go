// Hand-written: the collection's own behaviour — the cap, the two duplicate
// paths, and the entry type's contract.
//
// The generated suite exercises the mappers and the declarative rules through
// the ROOT. What it cannot know is the shape of the entry itself: that its
// business identity is RoleID and nothing else, and that a collection carrying
// the same role twice is refused even when the grant route could not have
// produced it.

package domain

import (
	"reflect"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// groupWithRoles builds a group conferring n distinct roles, bypassing
// AddGroupRole so the collection is populated the way a whole-body insert
// populates it.
func groupWithRoles(n int) *Group {
	e := validGroup()
	for i := 0; i < n; i++ {
		domain.AddAggregateChild(e, aggregatevos.GroupRole{RoleID: domain.NewID(groupUUIDForIndex(i))})
	}
	return e
}

// A distinct, well-formed UUID per index — the cap is about COUNT, so the ids
// only have to differ.
func groupUUIDForIndex(i int) string {
	const hex = "0123456789abcdef"
	suffix := []byte{hex[(i>>8)&0xf], hex[(i>>4)&0xf], hex[i&0xf]}
	return "0198f3e0-9c25-7a1f-b73d-5e08c4a29" + string(suffix)
}

func TestAGroupAtTheCapIsAccepted(t *testing.T) {
	e := groupWithRoles(50)

	if _, err := domain.GetInsertable(e, &probingGroupService{}, "GetInsertable"); err != nil {
		t.Fatalf("a group conferring exactly 50 roles was refused: %v", groupRejectedFields(err))
	}
}

// The cap is lower than Role's 200 on purpose, and it MULTIPLIES against it: a
// member inherits every permission of every role in the bundle, so 50 roles is
// already a 10,000-permission ceiling on what one group can confer.
func TestAGroupOnePastTheCapIsRefused(t *testing.T) {
	e := groupWithRoles(51)

	_, err := domain.GetInsertable(e, &probingGroupService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a group conferring 51 roles was accepted — the cap does not bind")
	}
	if !groupBlames(err, "Roles") {
		t.Errorf("the refusal blamed %v, want Roles", groupRejectedFields(err))
	}
}

// The interpolated bound must actually reach the message. A notification
// declaring tvars and raised as an empty struct renders "at most  roles", with
// a hole, in all seven languages — a defect only an end user ever sees, because
// it survives build, vet and every generated test.
func TestTheCapNotificationCarriesItsBound(t *testing.T) {
	if got := (TooManyRolesInGroupNotification{Max: "50"}).Max; got != "50" {
		t.Errorf("the cap notification carried Max=%q, want the bound the rule enforces", got)
	}
	if (TooManyRolesInGroupNotification{}).Max != "" {
		t.Fatal("the zero value already carries a bound — this test cannot detect the empty-struct defect")
	}
}

// The duplicate guard has TWO paths and the by-id one is tested beside the
// rules. This is the other: a collection that already carries the same role
// twice — which the attach route cannot produce, but a whole-body insert can.
func TestADuplicateInsideTheRolesCollectionIsRefused(t *testing.T) {
	e := validGroup()
	id := domain.NewID(attachedRoleID)
	domain.AddAggregateChild(e, aggregatevos.GroupRole{RoleID: id})
	domain.AddAggregateChild(e, aggregatevos.GroupRole{RoleID: id})

	_, err := domain.GetInsertable(e, &probingGroupService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a group conferring the same role twice was accepted")
	}
	// NOTE the field name: the childDuplicate rule blames "Roles" — the
	// DECLARED COLLECTION — which is what every other refusal about this
	// collection already blamed. Framework v0.73.0 ended the old split where
	// this one path spelled the same place as the entry TYPE ("GroupRole");
	// aggregate_root.go now builds the segment from CollectionName().
	if !groupBlames(err, "Roles") {
		t.Errorf("the refusal blamed %v, want Roles", groupRejectedFields(err))
	}
}

// The business identity is RoleID and NOTHING ELSE, and this is what says so.
//
// It is load-bearing rather than stylistic: the entry carries two read-join
// fields that are blank on a freshly attached entry and populated on a stored
// one, so a comparison over all three would answer "different" for an
// attachment duplicating a stored entry — the duplicate guard failing open.
func TestTheEntrysBusinessIdentityIsTheRoleReferenceAlone(t *testing.T) {
	stored := aggregatevos.GroupRole{
		RoleID:   domain.NewID(attachedRoleID),
		RoleKey:  "billing-manager",
		RoleName: "Billing Manager",
	}
	// What the mapper builds from a request body: the reference, nothing else.
	attached := aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)}

	if !stored.IsSameBusinessIdentity(attached) {
		t.Error("a stored entry and an attachment of the same role read as different entries")
	}

	other := aggregatevos.GroupRole{RoleID: domain.NewID("0198f400-1111-7000-8000-aaaaaaaaaaaa")}
	if stored.IsSameBusinessIdentity(other) {
		t.Error("two different roles read as the same entry")
	}
}

// A value object of another collection is never the same entry, whatever its
// fields hold. The type assertion is what guarantees it.
func TestTheEntryIsNeverTheSameAsAnotherCollectionsEntry(t *testing.T) {
	entry := aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)}

	if entry.IsSameBusinessIdentity(aggregatevos.RolePermission{PermissionID: domain.NewID(attachedRoleID)}) {
		t.Error("a RolePermission matched a GroupRole — the type assertion does not hold")
	}
}

// The collection segment name is what the projection nests the entries under,
// what the read DTO declares, and what a notification path spells. Three
// consumers, one constant of the type — so a rename that missed one of them
// fails here rather than in a client.
func TestTheCollectionIsNamedRoles(t *testing.T) {
	if got := (aggregatevos.GroupRole{}).CollectionName(); got != "Roles" {
		t.Errorf("CollectionName() = %q, want Roles", got)
	}
}

// The entry declares no rule of its own: RoleID is a domain.ID, which the
// framework validates by type, and there is nothing else on the entry to check.
// Calling it proves the seat exists and stays empty — the day somebody adds a
// rule here, this is the test that has to be updated deliberately.
func TestTheEntryDeclaresNoRuleOfItsOwn(t *testing.T) {
	entry := aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)}
	ctx := domain.NewNotificationContext("GroupRole")
	rules := domain.NewRules(domain.ModeInsert, ctx, reflect.TypeOf(entry))

	entry.BuildRules("GetInsertable", nil, rules)

	if ctx.HasErrors() {
		t.Errorf("the entry raised %v, want no rule of its own", ctx.Messages())
	}
}
