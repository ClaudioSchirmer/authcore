package domain

import (
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The uniqueness pre-check is generated from the spec's
// `unique: {enforce: service-precheck+constraint, scope: active-only}` over the
// composite key, so the generated permission_test.go exercises the rule's happy
// path with a stub that answers "nothing found" — which means the branch that
// actually REFUSES a duplicate is never taken there.
//
// That branch is the whole point of the pre-check: it is what lets a duplicate
// be reported together with the caller's other mistakes instead of arriving
// alone as a 409 after everything else passed. The database's partial unique
// index is only the backstop for the race between this check and the commit.

// takenPermissionService answers every probe with "already held", which is what
// the pre-check exists to react to.
type takenPermissionService struct {
	domain.ServiceBase

	// gotResource, gotAction and gotSelfID record what the rule asked, so the
	// test can assert the probe is filtered by BOTH halves of the pair — a
	// check over either column alone would refuse a legitimate second action on
	// the same resource.
	gotResource string
	gotAction   string
	gotSelfID   domain.ID
}

func (s *takenPermissionService) PermissionKeyTaken(resource string, action string, selfID domain.ID) bool {
	s.gotResource = resource
	s.gotAction = action
	s.gotSelfID = selfID
	return true
}

// A pair another ACTIVE permission already holds is refused, and the refusal
// names the key.
func TestPermissionDuplicateKeyIsRefusedOnInsert(t *testing.T) {
	svc := &takenPermissionService{}
	_, err := domain.GetInsertable(validPermission(), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a duplicate resource:action pair was accepted")
	}
	if !permissionBlames(err, "Key") {
		t.Errorf("the rejection should name Key, it named %v", permissionRejectedFields(err))
	}

	// The probe is filtered by the PAIR, not by either column alone.
	if svc.gotResource != "tenant" || svc.gotAction != "read" {
		t.Errorf("the pre-check asked for {%q, %q}, want {\"tenant\", \"read\"}", svc.gotResource, svc.gotAction)
	}
}

// On an insert there is no row yet, so there is nothing to exclude — and the id
// is not minted until after the rules run. The probe must therefore carry an
// empty self id, or the very first permission would exclude itself from a
// lookup it is not in and the check would be a no-op.
func TestPermissionDuplicateProbeExcludesNothingOnInsert(t *testing.T) {
	svc := &takenPermissionService{}
	_, _ = domain.GetInsertable(validPermission(), svc, "GetInsertable")
	if !svc.gotSelfID.IsEmpty() {
		t.Errorf("the insert probe carried self id %q, want empty", svc.gotSelfID.Value())
	}
}

// On an update the row DOES exist, so it must be excluded — otherwise a
// permission would always collide with itself and no description could ever be
// edited.
func TestPermissionDuplicateProbeExcludesSelfOnUpdate(t *testing.T) {
	id := domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410")
	e := validPermission()
	e.SetID(id)

	svc := &takenPermissionService{}
	_, err := domain.GetUpdatable(e, func(x *Permission) error {
		x.Description = vos.Description("Read tenants: browse the registry and fetch one by id.")
		return nil
	}, svc, "GetUpdatable")

	// The stub answers "taken" for everything, so the write is still refused —
	// what this test asserts is WHAT was asked, not the verdict.
	if err == nil {
		t.Fatal("the stub answers taken for everything, so the update should have been refused")
	}
	if svc.gotSelfID != id {
		t.Errorf("the update probe carried self id %q, want %q", svc.gotSelfID.Value(), id.Value())
	}
}

// The pre-check is skipped when either half is empty: a missing part is the
// value object's complaint, and probing the database for "" would be a query
// nobody meant with a second, derived complaint on top.
func TestPermissionDuplicateProbeIsSkippedForAnIncompleteKey(t *testing.T) {
	for _, k := range []vos.PermissionKey{
		{Resource: "", Action: "read"},
		{Resource: "tenant", Action: ""},
		{Resource: "", Action: ""},
	} {
		e := validPermission()
		e.Key = k

		svc := &takenPermissionService{}
		if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err == nil {
			t.Errorf("an incomplete key %+v was accepted", k)
		}
		if svc.gotResource != "" || svc.gotAction != "" {
			t.Errorf("key %+v: the pre-check probed the store with an incomplete pair", k)
		}
	}
}
