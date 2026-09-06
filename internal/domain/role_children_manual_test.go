// Hand-written: the two child-mutation paths and their refusals.
//
// `../../specs/scaffold-entity/role/task_tests.md` names both explicitly — the
// duplicate-grant guard on the by-id path, and the child not-found path
// "asserting the 404-mapped notification and not the 422 one". The distinction
// is the whole reason the guard lives in a domain method instead of a loop in
// the command mapper.

package domain

import (
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

func notificationsOf(e *Role) []domain.NotificationMessage {
	return e.GetAggregateRoot().NotificationContext().Messages()
}

// ── GRANT ───────────────────────────────────────────────────────────────────

func TestGrantAddsTheEntryToTheCollection(t *testing.T) {
	e := validRole()
	e.AddRolePermission(aggregatevos.RolePermission{
		PermissionID: domain.NewID(grantedPermissionID),
	})

	if got := len(domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())); got != 1 {
		t.Fatalf("the collection holds %d entries after one grant, want 1", got)
	}
	if len(notificationsOf(e)) != 0 {
		t.Errorf("granting a fresh permission raised %v", notificationsOf(e))
	}
}

// Granting the same permission twice is an ANSWER, not a silent merge: the
// caller named one entry, so the collision has to reach them.
func TestGrantingTheSamePermissionTwiceIsRefused(t *testing.T) {
	e := validRole()
	grant := aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)}

	e.AddRolePermission(grant)
	e.AddRolePermission(grant)

	if got := len(domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())); got != 1 {
		t.Errorf("the collection holds %d entries, want 1 — the second grant was stored instead of refused", got)
	}

	messages := notificationsOf(e)
	if len(messages) != 1 {
		t.Fatalf("the duplicate grant raised %d notification(s), want 1", len(messages))
	}
	if _, isDuplicate := messages[0].Notification.(RoleAlreadyGrantsPermissionNotification); !isDuplicate {
		t.Errorf("the duplicate answered with %T, want RoleAlreadyGrantsPermissionNotification", messages[0].Notification)
	}
}

// The join fields must not enter the comparison. A stored entry carries them and
// a freshly granted one does not, so a whole-struct comparison would let the
// duplicate through — which is the failure this test exists to catch.
func TestGrantingADuplicateIsRefusedEvenWhenTheJoinFieldsDiffer(t *testing.T) {
	e := validRole()
	id := domain.NewID(grantedPermissionID)

	e.AddRolePermission(aggregatevos.RolePermission{
		PermissionID: id,
		Resource:     "tenant",
		Action:       "read",
	})
	e.AddRolePermission(aggregatevos.RolePermission{PermissionID: id})

	if got := len(domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())); got != 1 {
		t.Errorf("the collection holds %d entries, want 1 — sameness consulted the join fields", got)
	}
}

// ── REVOKE ──────────────────────────────────────────────────────────────────

// A revoke of an entry that is not there answers with the CANONICAL not-found,
// which the framework maps to 404. The framework's own does-not-exist
// notification maps to 422, and a caller addressing a child by id is entitled to
// the first: they named something that is not there, they did not send something
// invalid.
func TestRevokingAnEntryThatIsNotThereAnswersNotFound(t *testing.T) {
	e := validRole()
	e.RemoveRolePermissionByID("c4e81d55-9a02-4f7b-8c31-6b5d0e2a91ff")

	messages := notificationsOf(e)
	if len(messages) != 1 {
		t.Fatalf("revoking an absent entry raised %d notification(s), want 1", len(messages))
	}
	if _, isNotFound := messages[0].Notification.(domain.RecordNotFoundNotification); !isNotFound {
		t.Errorf("the absent entry answered with %T, want RecordNotFoundNotification (404, not the 422 does-not-exist)",
			messages[0].Notification)
	}
}

// The happy path: an entry addressed by its OWN id — not by the permission id —
// leaves the collection.
func TestRevokingByTheEntryIDRemovesIt(t *testing.T) {
	e := validRole()
	entryID := "c4e81d55-9a02-4f7b-8c31-6b5d0e2a91ff"

	grant := aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)}
	grant.SetID(domain.NewID(entryID))
	e.AddRolePermission(grant)

	e.RemoveRolePermissionByID(entryID)

	if got := len(domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())); got != 0 {
		t.Errorf("the collection still holds %d entries after the revoke", got)
	}
	if len(notificationsOf(e)) != 0 {
		t.Errorf("a valid revoke raised %v", notificationsOf(e))
	}
}

// Addressing the entry by the PERMISSION id is not-found, which is what keeps
// the two ids from being used interchangeably: the revoke route takes the
// grant's own id and nothing else.
func TestRevokingByThePermissionIDDoesNotMatch(t *testing.T) {
	e := validRole()

	grant := aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)}
	grant.SetID(domain.NewID("c4e81d55-9a02-4f7b-8c31-6b5d0e2a91ff"))
	e.AddRolePermission(grant)

	e.RemoveRolePermissionByID(grantedPermissionID)

	if got := len(domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot())); got != 1 {
		t.Error("the entry was removed by the permission id — the two ids are being treated as one")
	}
	if len(notificationsOf(e)) != 1 {
		t.Error("addressing an entry by the wrong id did not answer not-found")
	}
}

// ── the per-role cap, at the boundary and one past it ───────────────────────

func roleWithGrants(n int) *Role {
	e := validRole()
	for i := 0; i < n; i++ {
		// AddAggregateChild directly: AddRolePermission's own duplicate guard
		// would answer first, and what is under test here is the RULE.
		domain.AddAggregateChild(e, aggregatevos.RolePermission{
			PermissionID: domain.NewID(uuidForIndex(i)),
		})
	}
	return e
}

// A distinct, well-formed UUID per index — the cap is about COUNT, so the ids
// only have to differ.
func uuidForIndex(i int) string {
	const hex = "0123456789abcdef"
	suffix := []byte{hex[(i>>8)&0xf], hex[(i>>4)&0xf], hex[i&0xf]}
	return "9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f" + string(suffix)
}

func TestARoleAtTheCapIsAccepted(t *testing.T) {
	e := roleWithGrants(200)

	if _, err := domain.GetInsertable(e, &probingRoleService{}, "GetInsertable"); err != nil {
		t.Fatalf("a role holding exactly 200 permissions was refused: %v", roleRejectedFields(err))
	}
}

func TestARoleOnePastTheCapIsRefused(t *testing.T) {
	e := roleWithGrants(201)

	_, err := domain.GetInsertable(e, &probingRoleService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a role holding 201 permissions was accepted — the cap does not bind")
	}
	if !roleBlames(err, "Permissions") {
		t.Errorf("the refusal blamed %v, want Permissions", roleRejectedFields(err))
	}
}

// The duplicate guard has TWO paths and the by-id one is tested above. This is
// the other: a collection that already carries the same permission twice —
// which the grant route cannot produce, but a whole-body insert can.
func TestADuplicateInsideTheCollectionIsRefused(t *testing.T) {
	e := validRole()
	id := domain.NewID(grantedPermissionID)
	domain.AddAggregateChild(e, aggregatevos.RolePermission{PermissionID: id})
	domain.AddAggregateChild(e, aggregatevos.RolePermission{PermissionID: id})

	_, err := domain.GetInsertable(e, &probingRoleService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a role granting the same permission twice was accepted")
	}
	// NOTE the field name: the childDuplicate rule blames "Permissions" — the
	// DECLARED COLLECTION — which is what every other refusal about this
	// collection already blamed. Framework v0.73.0 ended the old split where
	// this one path spelled the same place as the entry TYPE ("RolePermission");
	// aggregate_root.go now builds the segment from CollectionName().
	if !roleBlames(err, "Permissions") {
		t.Errorf("the refusal blamed %v, want Permissions", roleRejectedFields(err))
	}
}
