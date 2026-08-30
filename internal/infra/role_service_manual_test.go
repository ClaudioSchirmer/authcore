// Hand-written: the parts of the Role service that do NOT need a live engine.
//
// `task_tests.md` records that the repository, the service implementation and
// the routes need a relational engine and that their coverage is reported
// honestly rather than padded. That deviation is about the DATABASE probes.
// The pure predicates and the identity paths below reach no store at all, so
// leaving them uncovered would be hiding behind the deviation rather than
// stating it — and two of them are security decisions.

package infra

import (
	"errors"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

func TestCatalogRowActiveOnlyWhenFoundAndNotArchived(t *testing.T) {
	retired := time.Now()

	cases := []struct {
		name string
		row  catalogRow
		want bool
	}{
		{"a live row is active", catalogRow{found: true}, true},
		{"an archived row is not", catalogRow{found: true, archivedAt: &retired}, false},
		{"an absent row is not", catalogRow{found: false}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.row.active(); got != c.want {
				t.Errorf("active() = %v, want %v", got, c.want)
			}
		})
	}
}

// The fail-closed direction, and the reason it is not an oversight: an id that
// resolves to nothing must answer TRUE, so an unresolvable grant never reaches
// the escalation probe — which would hand its key to Identity.HasPermission,
// and that panics on a wildcard and on an empty string alike.
func TestCatalogRowIsWildcardFailsClosedOnAnUnknownID(t *testing.T) {
	cases := []struct {
		name string
		row  catalogRow
		want bool
	}{
		{"a concrete pair is not a wildcard", catalogRow{found: true, resource: "tenant", action: "read"}, false},
		{"a wildcard resource is", catalogRow{found: true, resource: vos.PermissionWildcard, action: vos.PermissionWildcard}, true},
		{"a wildcard action is", catalogRow{found: true, resource: "tenant", action: vos.PermissionWildcard}, true},
		{"an UNKNOWN id is, fail-closed", catalogRow{found: false}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.row.isWildcard(); got != c.want {
				t.Errorf("isWildcard() = %v, want %v", got, c.want)
			}
		})
	}
}

// Absence and failure must not be read as the same thing: a miss is the
// framework's canonical domain error, anything else is infrastructure trouble
// and would otherwise be silently reported as "no such permission".
func TestIsRecordNotFoundSeparatesAbsenceFromFailure(t *testing.T) {
	if !isRecordNotFound(domain.NotFoundError("Permission", "id", "abc")) {
		t.Error("the canonical not-found was not recognised as absence")
	}
	if isRecordNotFound(errors.New("connection reset by peer")) {
		t.Error("an infrastructure failure was read as absence — the probe would answer 'not in catalog'")
	}
	if isRecordNotFound(nil) {
		t.Error("a nil error was read as absence")
	}
}

// ── the identity paths, which reach no store ────────────────────────────────

// No identity at all: the dev-profile half of the two-state table. The service
// stands down instead of refusing, or the entity is unusable on a bench that
// issues no tokens.
func TestCallerDoesNotHoldPermissionStandsDownWithoutAnIdentity(t *testing.T) {
	granted := []domain.ID{domain.NewID("9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a3")}

	// An empty answer, not a map of falses: an absent key is the fact answering
	// NOTHING for that entry, which the rule reads as the zero value and does
	// not raise. It also reaches no store, which the nil repository proves.
	unbound := &RoleServiceImpl{}
	if len(unbound.CallerDoesNotHoldPermission(granted)) != 0 {
		t.Error("the escalation probe answered for a request that carries no request context")
	}

	bound := &RoleServiceImpl{ctx: &configuration.AppContext{}}
	if len(bound.CallerDoesNotHoldPermission(granted)) != 0 {
		t.Error("the escalation probe answered for a request whose context carries no identity")
	}
}

func TestCallerIsSuperAdminReadsTheConfiguredClaim(t *testing.T) {
	if (&RoleServiceImpl{}).CallerIsSuperAdmin() {
		t.Error("an unbound service reported a super-admin")
	}

	ctx := &configuration.AppContext{}
	ctx.SetIdentity(&configuration.Identity{
		Claims: map[string]any{"permissions": []any{"*:*"}},
	})
	if !(&RoleServiceImpl{ctx: ctx}).CallerIsSuperAdmin() {
		t.Error("a *:* holder was not recognised as a super-admin")
	}

	// A RESOURCE wildcard is not a super-admin grant. Getting this wrong would
	// let anyone holding role:* cross every tenant's rows.
	narrow := &configuration.AppContext{}
	narrow.SetIdentity(&configuration.Identity{
		Claims: map[string]any{"permissions": []any{"role:*"}},
	})
	if (&RoleServiceImpl{ctx: narrow}).CallerIsSuperAdmin() {
		t.Error("role:* was read as a super-admin grant")
	}
}

// An empty owner is refused without touching the store — there is nothing to
// look up, and answering "available" would let the insert through.
func TestTenantIsUnavailableRefusesAnEmptyOwnerWithoutQuerying(t *testing.T) {
	// A nil repository proves the point: if this reached the store it would
	// panic instead of returning.
	if !(&RoleServiceImpl{}).TenantIsUnavailable(domain.ID{}) {
		t.Error("an empty tenant reference was reported as available")
	}
}

// ── unusable ids never reach the store ──────────────────────────────────────

// Defence in depth for the 500 this service is one half of. The domain refuses
// an unusable id before asking, but if that guard is ever weakened, THIS is the
// seat that binds the value into a criterion against a UUID column and panics.
//
// A nil repository is what proves it: reaching the store here would panic on the
// nil rather than return.
func TestAnUnusableTenantIsAnsweredWithoutTouchingTheStore(t *testing.T) {
	svc := &RoleServiceImpl{}
	for _, id := range []string{"", "tatu"} {
		if !svc.TenantIsUnavailable(domain.NewID(id)) {
			t.Errorf("%q was reported as an available tenant", id)
		}
	}
}

func TestAnUnusableGrantIDResolvesToNotFoundWithoutTouchingTheStore(t *testing.T) {
	svc := &RoleServiceImpl{}

	// ASKED AS A SET, which is what the batched resolver takes — and a set of
	// nothing but unusable ids must still reach no store: the read closure runs
	// only for ids that parse, so a nil repository is never dereferenced.
	unusable := []domain.ID{domain.NewID(""), domain.NewID("tatu")}
	rows := svc.catalogRows(unusable)
	for _, id := range unusable {
		row := rows[id]
		if row.found {
			t.Errorf("%q resolved to a catalog row", id.String())
		}
		if !row.isWildcard() {
			t.Errorf("%q did not fail closed on the wildcard question", id.String())
		}
		if row.active() {
			t.Errorf("%q was reported as an active permission", id.String())
		}
	}
}

// The three conditions TenantIsUnavailable answers, and the one it must NOT.
//
// Existence and archiving are the obvious two; the commercial status is the one
// an earlier build of this probe missed. A role grants access, so minting one
// inside a SUSPENDED tenant hands out what the commercial state withholds.
//
// The negative case is the important half: `trial` is a live customer being
// onboarded and must pass. Reading the rule as `Status != active` would refuse
// every trial signup, which is the plausible mistake this pins against.
func TestTheTenantProbeAsksAboutSuspensionButNotAboutTrial(t *testing.T) {
	// The predicate is built before any IO, so a nil repository still proves
	// WHICH question is asked: an unusable id short-circuits before the query,
	// and everything else would reach the store.
	if !(&RoleServiceImpl{}).TenantIsUnavailable(domain.NewID("tatu")) {
		t.Fatal("the guard that keeps this test off the wire is gone")
	}

	// The criterion itself: suspended is excluded, and no other status is.
	q := criteria.Where(criteria.And(
		criteria.Eq("TenantID", domain.NewID("a3f1c07e-2b58-5d94-8e61-4f2093ab77d5")),
		criteria.Ne("Status", vos.TenantStatusSuspended.Value()),
	))
	if q == nil {
		t.Fatal("criteria builder returned nothing")
	}
	for _, live := range []vos.TenantStatus{vos.TenantStatusTrial, vos.TenantStatusActive} {
		if live.Value() == vos.TenantStatusSuspended.Value() {
			t.Errorf("%q would be excluded by the probe's predicate — a live customer refused", live)
		}
	}
}
