// Tests for the claim probes that need no database.
//
// The three parents' claim facts and Claim's two held-value facts all guard an
// unusable id BEFORE they touch a store, and that guard is the whole reason
// these tests can exist: a nil repository is what proves the store was never
// reached, because reaching it would panic on the nil rather than return.
//
// It is also the seat that matters most. The domain already refuses an entry id
// that is not a usable UUID, but an unparseable value binds into a criterion
// against a UUID column, the driver rejects the statement, and the probe's own
// panic turns a plain validation problem into a 500.

package infra

import (
	"testing"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

func TestAnUnusableClaimIDResolvesToNotFoundWithoutTouchingTheUserStore(t *testing.T) {
	svc := &UserServiceImpl{}

	// ASKED AS A SET, which is what the batched resolver takes — and a set of
	// nothing but unusable ids must still reach no store: the read closure runs
	// only for ids that parse, so a nil repository is never dereferenced.
	unusable := []domain.ID{domain.NewID(""), domain.NewID("tatu")}
	rows := svc.claimRows(unusable)
	for _, id := range unusable {
		if rows[id].found {
			t.Errorf("%q resolved to a claim definition", id.String())
		}
	}
}

func TestAnUnusableClaimIDResolvesToNotFoundWithoutTouchingTheClientStore(t *testing.T) {
	svc := &ClientServiceImpl{}

	unusable := []domain.ID{domain.NewID(""), domain.NewID("tatu")}
	rows := svc.claimRows(unusable)
	for _, id := range unusable {
		if rows[id].found {
			t.Errorf("%q resolved to a claim definition", id.String())
		}
	}
}

// FAIL CLOSED, all three, and this is the property the rules depend on: a fact
// that cannot resolve its subject reports the problem as PRESENT, so an
// unresolvable entry can never pass a check by accident.
func TestTheThreeUserClaimFactsAllFailClosedOnAnUnusableID(t *testing.T) {
	svc := &UserServiceImpl{}
	unusable := domain.NewID("tatu")
	tenant := domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410")

	set := []domain.ID{unusable}

	if !svc.ClaimIsUnavailableInTenant(tenant, set)[unusable] {
		t.Error("an unusable claim id was reported as available")
	}
	if !svc.ClaimDoesNotApplyToUser(set)[unusable] {
		t.Error("an unusable claim id was reported as applying to users")
	}
	entries := []appdomain.UserClaimValueDoesNotMatchValueTypeEntry{{ClaimID: unusable, Value: "1000"}}
	if !svc.ClaimValueDoesNotMatchValueType(entries)[unusable] {
		t.Error("an unusable claim id was reported as type-matching")
	}
}

func TestTheThreeClientClaimFactsAllFailClosedOnAnUnusableID(t *testing.T) {
	svc := &ClientServiceImpl{}
	unusable := domain.NewID("tatu")
	tenant := domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410")

	set := []domain.ID{unusable}

	if !svc.ClaimIsUnavailableInTenant(tenant, set)[unusable] {
		t.Error("an unusable claim id was reported as available")
	}
	if !svc.ClaimDoesNotApplyToClient(set)[unusable] {
		t.Error("an unusable claim id was reported as applying to clients")
	}
	entries := []appdomain.ClientClaimValueDoesNotMatchValueTypeEntry{{ClaimID: unusable, Value: "sa-east-1"}}
	if !svc.ClaimValueDoesNotMatchValueType(entries)[unusable] {
		t.Error("an unusable claim id was reported as type-matching")
	}
}

// The held-value probes now fail the SAME way as the rest, and the flip is the
// consequence of what they are asked about rather than a change of policy.
//
// They used to take the owning TENANT, and "nobody holds a value under a tenant
// that cannot exist" was simply true — answering "not held" was the honest
// reading, not a relaxed one. They now take the DEFINITION'S OWN ID, and an id
// this probe cannot use is not a definition nobody holds values for: it is a
// question with no answer. Clearing the narrowing on it would strand every value
// already written for the row, invisibly, so the refusal is the fail-closed
// reading — the same one TenantIsUnavailable gives its unusable argument.
//
// Neither branch reaches a store, which is what makes the zero-value service a
// legitimate fixture here.
func TestTheHeldValueProbesRefuseAnUnusableClaimID(t *testing.T) {
	svc := &ClaimServiceImpl{}
	for _, id := range []string{"", "tatu"} {
		if !svc.ClaimIsHeldByAUser(domain.NewID(id)) {
			t.Errorf("the user probe cleared a narrowing for the unusable claim id %q", id)
		}
		if !svc.ClaimIsHeldByAClient(domain.NewID(id)) {
			t.Errorf("the client probe cleared a narrowing for the unusable claim id %q", id)
		}
	}
}

// The SHARED predicate the two ClaimDoesNotApplyTo… facts read, exercised for
// every member of the closed set plus the Unknown sentinel. It is asserted here
// rather than only in the domain package because these two facts are the seat
// where getting it wrong is silent and worst: a client holding a user-only
// claim, written and never complained about.
func TestAppliesToAdmitsExactlyTheKindsItNames(t *testing.T) {
	for _, tc := range []struct {
		value          vos.ClaimAppliesTo
		users, clients bool
	}{
		{vos.ClaimAppliesToUser, true, false},
		{vos.ClaimAppliesToClient, false, true},
		{vos.ClaimAppliesToBoth, true, true},
		// Outside the closed set: admits NOBODY. A value the enum refuses is
		// not a permission to hold anything.
		{vos.ClaimAppliesToUnknown, false, false},
	} {
		if got := appdomain.ClaimAdmitsUsers(tc.value); got != tc.users {
			t.Errorf("ClaimAdmitsUsers(%q) = %v, want %v", tc.value, got, tc.users)
		}
		if got := appdomain.ClaimAdmitsClients(tc.value); got != tc.clients {
			t.Errorf("ClaimAdmitsClients(%q) = %v, want %v", tc.value, got, tc.clients)
		}
	}
}
