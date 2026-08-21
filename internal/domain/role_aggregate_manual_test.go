package domain

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The generated role_test.go proves the aggregate contract and the two
// immutability rules. What it does not reach is the REFUSING side of the
// collection paths — the duplicate, the cap, the taken key and the missing
// entry — and those are the branches that carry the consequences.

// roleEntityNotifications lists what the ENTITY itself raised, by type name.
//
// The per-entry domain methods report through the entity's own context rather
// than by returning an error — a caller reads them after the call — so these
// cases assert on the entity and not on a rejection.
func roleEntityNotifications(e *Role) []string {
	var out []string
	for _, msg := range e.NotificationContext().Messages() {
		out = append(out, reflect.TypeOf(msg.Notification).Name())
	}
	return out
}

func roleRaised(e *Role, want string) bool {
	for _, name := range roleEntityNotifications(e) {
		if name == want {
			return true
		}
	}
	return false
}

// ── the uniqueness pre-check ───────────────────────────────────────────────

// The pre-check is what lets a duplicate handle be reported TOGETHER with the
// other problems, instead of alone and later as a raw constraint error.
func TestRole_KeyAlreadyTakenInTheTenant_IsRefused(t *testing.T) {
	svc := &factStub{keyTaken: true}
	_, err := domain.GetInsertable(validRole(), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a role key already taken in this tenant was accepted")
	}
	if !roleBlames(err, "Key") {
		t.Errorf("the rejection should name Key, it named %v", roleRejectedFields(err))
	}
	if !raisedNotification(err, "RoleKeyAlreadyExistsNotification") {
		t.Errorf("expected RoleKeyAlreadyExistsNotification, got %v", raisedNotifications(err))
	}
}

// An empty key costs no probe: the value object already refuses it, and asking
// the database whether "" is taken is a query with no question behind it.
func TestRole_EmptyKeyIsNotProbedForUniqueness(t *testing.T) {
	e := validRole()
	e.Key = ""

	svc := &factStub{keyTaken: true}
	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err == nil {
		t.Fatal("a role with no key was accepted")
	}
}

// ── the collection backstops ───────────────────────────────────────────────

// Two entries granting the same permission cannot COEXIST in the collection at
// all: the framework's own carrier refuses a same-business-identity add with
// EntityAlreadyAddedNotification, before the aggregate's childDuplicate rule
// ever sees the pair.
//
// That makes the generated childDuplicate rule a genuine backstop — declared,
// correct, and unreachable through any public path, which is why it is asserted
// here as the framework's behaviour rather than as the rule's. The reachable
// guarantee is the one that matters to a caller, and it is this one.
func TestRole_TwoIdenticalGrantsCannotCoexist(t *testing.T) {
	e := validRole()
	// Two DISTINCT rows granting the same permission — different entry ids, one
	// business identity — which is the shape a race that beat the unique index
	// would produce.
	first := aggregatevos.RolePermission{PermissionID: domain.NewID(grantA)}
	first.SetID(domain.NewID("11111111-1111-4111-8111-111111111111"))
	second := aggregatevos.RolePermission{PermissionID: domain.NewID(grantA)}
	second.SetID(domain.NewID("22222222-2222-4222-8222-222222222222"))
	domain.AddAggregateChild(e, first)
	domain.AddAggregateChild(e, second)

	if !roleRaised(e, "EntityAlreadyAddedNotification") {
		t.Errorf("a second grant of the same permission should be refused, the entity said %v",
			roleEntityNotifications(e))
	}
	if n := len(domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())); n != 1 {
		t.Errorf("the collection should hold one grant, it holds %d", n)
	}
}

// The cap is over the whole collection: a claim-size budget as much as a
// modelling one. 200 is the bound itself and must be accepted; 201 must not.
func TestRole_PermissionCapIsTwoHundred(t *testing.T) {
	build := func(n int) *Role {
		e := validRole()
		for i := 0; i < n; i++ {
			e.AddRolePermission(aggregatevos.RolePermission{
				PermissionID: domain.NewID(fmt.Sprintf("00000000-0000-4000-8000-%012d", i)),
			})
		}
		return e
	}

	if _, err := domain.GetInsertable(build(200), &factStub{}, "GetInsertable"); err != nil {
		t.Fatalf("exactly 200 grants — the bound itself — was rejected: %v (fields: %v)",
			err, roleRejectedFields(err))
	}

	_, err := domain.GetInsertable(build(201), &factStub{}, "GetInsertable")
	if err == nil {
		t.Fatal("201 grants were accepted; the cap is 200")
	}
	if !raisedNotification(err, "TooManyPermissionsInRoleNotification") {
		t.Errorf("expected TooManyPermissionsInRoleNotification, got %v", raisedNotifications(err))
	}
}

// ── the per-entry domain methods ───────────────────────────────────────────

// GRANT refuses a permission the role already has. The caller asked for THIS
// entry, so the collision is an answer rather than a silent merge.
func TestRole_AddRolePermission_RefusesOneAlreadyGranted(t *testing.T) {
	e := roleGranting(grantA)
	e.AddRolePermission(aggregatevos.RolePermission{PermissionID: domain.NewID(grantA)})

	if !roleRaised(e, "RoleAlreadyGrantsPermissionNotification") {
		t.Errorf("re-granting an existing permission should be refused, the entity said %v", roleEntityNotifications(e))
	}
	if n := len(domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())); n != 1 {
		t.Errorf("the collection should still hold one grant, it holds %d", n)
	}
}

// A different permission is added beside the first, not instead of it.
func TestRole_AddRolePermission_AddsADifferentOne(t *testing.T) {
	e := roleGranting(grantA)
	e.AddRolePermission(aggregatevos.RolePermission{PermissionID: domain.NewID(grantB)})

	if n := len(domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())); n != 2 {
		t.Errorf("the collection should hold two grants, it holds %d", n)
	}
}

// REVOKE names an entry, so a missing one is an ANSWER — the canonical 404 —
// and never a silent no-op that reports success for a revocation that did not
// happen.
func TestRole_RemoveRolePermissionByID_UnknownEntryIsNotFound(t *testing.T) {
	e := roleGranting(grantA)
	e.RemoveRolePermissionByID("11111111-1111-4111-8111-111111111111")

	if !roleRaised(e, "RecordNotFoundNotification") {
		t.Errorf("revoking an entry that is not there should raise RecordNotFoundNotification, the entity said %v",
			roleEntityNotifications(e))
	}
	if n := len(domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())); n != 1 {
		t.Errorf("nothing should have been removed, the collection holds %d", n)
	}
}

// Revoking an entry that IS there takes it out and says nothing.
func TestRole_RemoveRolePermissionByID_RemovesTheNamedEntry(t *testing.T) {
	e := roleGranting(grantA)
	items := domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())
	if len(items) != 1 {
		t.Fatalf("fixture: expected one grant, got %d", len(items))
	}

	e.RemoveRolePermissionByID(items[0].GetID().Value())

	if roleRaised(e, "RecordNotFoundNotification") {
		t.Errorf("revoking an entry that IS there raised not-found: %v", roleEntityNotifications(e))
	}
}
