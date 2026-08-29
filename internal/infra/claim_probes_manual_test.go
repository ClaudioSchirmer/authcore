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
	for _, id := range []string{"", "tatu"} {
		if row := svc.claimRow(domain.NewID(id)); row.found {
			t.Errorf("%q resolved to a claim definition", id)
		}
	}
}

func TestAnUnusableClaimIDResolvesToNotFoundWithoutTouchingTheClientStore(t *testing.T) {
	svc := &ClientServiceImpl{}
	for _, id := range []string{"", "tatu"} {
		if row := svc.claimRow(domain.NewID(id)); row.found {
			t.Errorf("%q resolved to a claim definition", id)
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

	if !svc.ClaimIsUnavailableInTenant(tenant, unusable) {
		t.Error("an unusable claim id was reported as available")
	}
	if !svc.ClaimDoesNotApplyToUser(unusable) {
		t.Error("an unusable claim id was reported as applying to users")
	}
	if !svc.ClaimValueDoesNotMatchValueType(unusable, "1000") {
		t.Error("an unusable claim id was reported as type-matching")
	}
}

func TestTheThreeClientClaimFactsAllFailClosedOnAnUnusableID(t *testing.T) {
	svc := &ClientServiceImpl{}
	unusable := domain.NewID("tatu")
	tenant := domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410")

	if !svc.ClaimIsUnavailableInTenant(tenant, unusable) {
		t.Error("an unusable claim id was reported as available")
	}
	if !svc.ClaimDoesNotApplyToClient(unusable) {
		t.Error("an unusable claim id was reported as applying to clients")
	}
	if !svc.ClaimValueDoesNotMatchValueType(unusable, "sa-east-1") {
		t.Error("an unusable claim id was reported as type-matching")
	}
}

// The held-value probes fail the OTHER way, and that asymmetry is deliberate
// rather than an oversight. "Nobody holds a value under a tenant that cannot
// exist" is simply true, and the guard exists so an unparseable owner never
// reaches a UUID comparison — not to make the narrowing rule fail closed.
func TestTheHeldValueProbesAnswerNobodyForAnUnusableTenant(t *testing.T) {
	svc := &ClaimServiceImpl{}
	for _, id := range []string{"", "tatu"} {
		if svc.ClaimIsHeldByAUser(domain.NewID(id), "x_cost_center") {
			t.Errorf("a user was reported as holding a value under tenant %q", id)
		}
		if svc.ClaimIsHeldByAClient(domain.NewID(id), "x_cost_center") {
			t.Errorf("a client was reported as holding a value under tenant %q", id)
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
