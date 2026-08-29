// Hand-written: the four rules the spec could not express declaratively, and
// the one thing about them that no generated test can know — that they reach
// their verdict through the DOMAIN SERVICE and never through the read-join
// fields sitting on the same entry.
//
// The generated suite stubs the service so every probe answers "nothing found",
// which is what lets a valid fixture through. Every case here overrides exactly
// one probe, so a failure names the rule under test.

package domain

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// probingRoleService answers "nothing found" like the generated stub, except
// where a case overrides one question. It also RECORDS what it was asked, which
// is how the ordering tests below prove a wildcard never reaches the escalation
// probe.
type probingRoleService struct {
	domain.ServiceBase

	keyTaken          bool
	tenantUnavailable bool
	notInCatalog      bool
	isWildcard        bool
	doesNotHold       bool

	askedNotInCatalog []domain.ID
	askedWildcard     []domain.ID
	askedDoesNotHold  []domain.ID

	// callsPerFact counts how many times each collection fact was ASKED, as
	// opposed to how many entries it was asked about. `perEntry` promises ONE
	// call per write whatever the size of the collection.
	callsPerFact map[string]int
	keyTakenSelf []domain.ID
	askedTenant  int
}

func (s *probingRoleService) RoleKeyTaken(_ domain.ID, _ string, selfID domain.ID) bool {
	s.keyTakenSelf = append(s.keyTakenSelf, selfID)
	return s.keyTaken
}
func (s *probingRoleService) CallerIsSuperAdmin() bool { return false }

func (s *probingRoleService) TenantIsUnavailable(_ domain.ID) bool {
	s.askedTenant++
	return s.tenantUnavailable
}

// The three collection facts, each asked ONCE for the whole set.
//
// The stub keeps recording ONE QUESTION PER ID — which ids reach a fact is a
// property the batch did not change and several cases below still assert.
// asked() counts the CALLS, the only thing that can catch a regression to one
// call per entry: every assertion about the outcome would pass either way.
func (s *probingRoleService) PermissionIsNotInCatalog(permissionIDSet []domain.ID) map[domain.ID]bool {
	s.asked("PermissionIsNotInCatalog")
	out := make(map[domain.ID]bool, len(permissionIDSet))
	for _, id := range permissionIDSet {
		s.askedNotInCatalog = append(s.askedNotInCatalog, id)
		out[id] = s.notInCatalog
	}
	return out
}

func (s *probingRoleService) PermissionIsWildcard(permissionIDSet []domain.ID) map[domain.ID]bool {
	s.asked("PermissionIsWildcard")
	out := make(map[domain.ID]bool, len(permissionIDSet))
	for _, id := range permissionIDSet {
		s.askedWildcard = append(s.askedWildcard, id)
		out[id] = s.isWildcard
	}
	return out
}

func (s *probingRoleService) CallerDoesNotHoldPermission(permissionIDSet []domain.ID) map[domain.ID]bool {
	s.asked("CallerDoesNotHoldPermission")
	out := make(map[domain.ID]bool, len(permissionIDSet))
	for _, id := range permissionIDSet {
		s.askedDoesNotHold = append(s.askedDoesNotHold, id)
		out[id] = s.doesNotHold
	}
	return out
}

// roleNotificationKeysOf lists the notification TYPES a refusal carries, which
// is what "one answer per entry" is asserted on: the blamed field is "Permissions"
// for all three rules, so counting fields would count the same name three times.
func roleNotificationKeysOf(err error) []string {
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		return nil
	}
	var out []string
	for _, ctx := range carrier.NotificationContexts() {
		for _, msg := range ctx.Messages() {
			out = append(out, fmt.Sprintf("%T", msg.Notification))
		}
	}
	return out
}

// asked records one CALL of a collection fact.
func (s *probingRoleService) asked(fact string) {
	if s.callsPerFact == nil {
		s.callsPerFact = map[string]int{}
	}
	s.callsPerFact[fact]++
}

const grantedPermissionID = "9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a3"

// A second and a third, for the one case that needs a collection rather than a
// single entry: a batch of one cannot tell "asked once" apart from "asked once
// per entry".
const (
	secondPermissionID = "9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a4"
	thirdPermissionID  = "9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a5"
)

// roleGranting returns a valid role that ADDS one grant — the shape all three
// per-entry rules judge.
func roleGranting(grant aggregatevos.RolePermission) *Role {
	e := validRole()
	e.AddRolePermission(grant)
	return e
}

// THE COST THE `perEntry` FACTS BUY, pinned where a regression would otherwise
// be invisible: every collection fact is asked ONCE for the whole write,
// however many grants it carries. Nothing about the OUTCOME changes if this
// regresses to one call per entry, which is why the call count is asserted.
func TestEveryRoleCollectionFactIsAskedOnceForTheWholeWrite(t *testing.T) {
	e := validRole()
	for _, id := range []string{grantedPermissionID, secondPermissionID, thirdPermissionID} {
		e.AddRolePermission(aggregatevos.RolePermission{PermissionID: domain.NewID(id)})
	}

	svc := &probingRoleService{}
	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err != nil {
		t.Fatalf("a role granting three catalogued permissions was refused: %v", roleRejectedFields(err))
	}

	for _, fact := range []string{
		"PermissionIsNotInCatalog", "PermissionIsWildcard", "CallerDoesNotHoldPermission",
	} {
		if got := svc.callsPerFact[fact]; got != 1 {
			t.Errorf("%s was asked %d times for a write carrying three grants, want exactly 1", fact, got)
		}
	}

	// And it was asked about EVERY entry: one call, three subjects.
	if len(svc.askedNotInCatalog) != 3 {
		t.Errorf("the single call carried %d grants, want all three", len(svc.askedNotInCatalog))
	}
}

// ── tenant-must-exist ───────────────────────────────────────────────────────

func TestInsertIsRefusedWhenTheOwningTenantIsUnavailable(t *testing.T) {
	svc := &probingRoleService{tenantUnavailable: true}

	_, err := domain.GetInsertable(validRole(), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a role was created under a missing or archived tenant")
	}
	if !roleBlames(err, "TenantID") {
		t.Errorf("the refusal blamed %v, want TenantID", roleRejectedFields(err))
	}
}

func TestInsertIsAcceptedWhenTheOwningTenantIsAvailable(t *testing.T) {
	if _, err := domain.GetInsertable(validRole(), &probingRoleService{}, "GetInsertable"); err != nil {
		t.Fatalf("a role under a live tenant was rejected: %v", roleRejectedFields(err))
	}
}

// With an unusable owner the hook is never reached AT ALL, so it asks nothing.
//
// That is the barrier's doing, not this rule's: `tenant-is-a-usable-id` pulls
// domain.ID's own validation forward and ends the pass, so customRules — where
// the tenant probe lives — never runs. The rule therefore carries NO emptiness
// guard of its own, and this is what says that is safe rather than forgotten.
//
// If the barrier is ever dropped, the probe starts receiving the bad value and
// this test fails before production does.
func TestAnUnusableOwnerNeverReachesTheTenantProbe(t *testing.T) {
	for _, owner := range []string{"", "tatu"} {
		t.Run("owner="+owner, func(t *testing.T) {
			e := validRole()
			e.TenantID = domain.NewID(owner)
			svc := &probingRoleService{tenantUnavailable: true}

			if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err == nil {
				t.Fatalf("a role with %q as its owner was accepted", owner)
			}
			if svc.askedTenant != 0 {
				t.Errorf("the tenant probe ran %d time(s) with %q — the barrier did not hold", svc.askedTenant, owner)
			}
		})
	}
}

// And with a USABLE owner it does run — otherwise the test above would pass for
// the wrong reason, on a hook that never runs at all.
func TestAUsableOwnerDoesReachTheTenantProbe(t *testing.T) {
	svc := &probingRoleService{}
	if _, err := domain.GetInsertable(validRole(), svc, "GetInsertable"); err != nil {
		t.Fatalf("a valid role was rejected: %v", roleRejectedFields(err))
	}
	if svc.askedTenant != 1 {
		t.Errorf("the tenant probe ran %d time(s), want exactly 1", svc.askedTenant)
	}
}

// ── granted-permissions-are-in-the-catalog ──────────────────────────────────

func TestGrantingAPermissionOutsideTheCatalogIsRefused(t *testing.T) {
	svc := &probingRoleService{notInCatalog: true}
	e := roleGranting(aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("a grant pointing outside the catalog was accepted")
	}
	if !roleBlames(err, "Permissions") {
		t.Errorf("the refusal blamed %v, want Permissions", roleRejectedFields(err))
	}
}

// ONE PROBLEM PER ENTRY. An id that is not in the catalog is one refusal:
// nothing below can say anything true about a permission that is not there, and
// every later fact answers "the problem is present" for an unknown id, so
// reporting all three would be noise the caller has to read past.
//
// The interlock is about the ANSWER rather than about the question — the three
// facts are asked once each for the whole collection, so a bad entry's later
// verdicts are computed and simply not read. The stub answers TRUE to all
// three, which is what makes this assertion mean something.
func TestAnUnknownPermissionIsReportedOnceAndNothingElse(t *testing.T) {
	svc := &probingRoleService{notInCatalog: true, isWildcard: true, doesNotHold: true}
	e := roleGranting(aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if got := len(roleNotificationKeysOf(err)); got != 1 {
		t.Errorf("one unknown permission produced %d answers, want exactly 1: %v",
			got, roleNotificationKeysOf(err))
	}
}

// ── no-wildcard-grant, and its ORDER relative to the escalation rule ────────

func TestGrantingAWildcardPermissionIsRefused(t *testing.T) {
	svc := &probingRoleService{isWildcard: true}
	e := roleGranting(aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("a wildcard permission was granted on a role")
	}
	if !roleBlames(err, "Permissions") {
		t.Errorf("the refusal blamed %v, want Permissions", roleRejectedFields(err))
	}
}

// THE ORDERING CASE, and where its load now sits.
//
// Identity.HasPermission PANICS on any argument containing '*'. The rule can no
// longer promise the escalation fact is not ASKED about a wildcard grant — it is
// asked about every entry at once — so what keeps the panic away is the
// guarantee the fact's own description carries and its body implements: it
// GUARDS THE WILDCARD ITSELF and answers "does not hold" instead of calling
// through (internal/infra/role_service_manual.go).
//
// What the domain still owns, and what this pins, is that a wildcard grant is
// reported as a wildcard and as nothing else.
func TestAWildcardGrantIsReportedAsWildcardAndNothingElse(t *testing.T) {
	svc := &probingRoleService{isWildcard: true, doesNotHold: true}
	e := roleGranting(aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	keys := roleNotificationKeysOf(err)
	if len(keys) != 1 {
		t.Errorf("one wildcard grant produced %d answers, want exactly 1: %v", len(keys), keys)
	}
	if len(svc.askedWildcard) == 0 {
		t.Fatal("the wildcard fact never ran — the rule this test pins does not exist")
	}
}

// ── no-privilege-escalation ─────────────────────────────────────────────────

func TestGrantingAPermissionTheCallerDoesNotHoldIsRefused(t *testing.T) {
	svc := &probingRoleService{doesNotHold: true}
	e := roleGranting(aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("a caller granted a permission they do not hold")
	}
	if !roleBlames(err, "Permissions") {
		t.Errorf("the refusal blamed %v, want Permissions", roleRejectedFields(err))
	}
}

// The dev-profile half of the two-state table. With NO identity at all the
// escalation gate stands down — collapsing this into a fail-closed test makes
// the entity unusable on a bench that issues no tokens.
func TestWithNoIdentityAtAllTheEscalationGateStandsDown(t *testing.T) {
	svc := &probingRoleService{doesNotHold: true}
	e := roleGranting(aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)})
	e.RequestingIdentityPresent = false
	e.RequestingTenant = ""

	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err != nil {
		t.Fatalf("the escalation gate refused a request that carried no identity: %v", roleRejectedFields(err))
	}
	if len(svc.askedDoesNotHold) != 0 {
		t.Error("the escalation probe ran even though no identity was present")
	}
}

// ── what the rules must NOT read ────────────────────────────────────────────

// The join fields are blank on an entry the write is ADDING, and this pins it.
//
// A rule that consulted grant.ArchivedAt would read nil here — the same nil a
// LIVE permission carries — and wave the entry through. That is fail-open on
// exactly the path the in-catalog rule exists to close, so the fixture below
// carries an archived-looking counterpart and must still be refused BY THE
// PROBE.
func TestAnAddedGrantIsJudgedByTheProbeAndNotByItsJoinFields(t *testing.T) {
	svc := &probingRoleService{notInCatalog: true}

	// Join fields deliberately left at their zero values, which is what an added
	// entry really carries: nothing traversed a foreign key for it.
	grant := aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)}
	if grant.Resource != "" || grant.Action != "" {
		t.Fatal("an added entry arrived with joined values — the premise of every rule here is gone")
	}

	if _, err := domain.GetInsertable(roleGranting(grant), svc, "GetInsertable"); err == nil {
		t.Fatal("a grant on a retired permission was accepted — the rule read the join instead of the probe")
	}
	if len(svc.askedNotInCatalog) != 1 {
		t.Errorf("the catalog probe ran %d time(s), want exactly 1 per added entry", len(svc.askedNotInCatalog))
	}
}

// A grant with no reference at all is the field's own complaint, so the probes
// must not be spent on it.
func TestAGrantWithNoReferenceIsNotProbed(t *testing.T) {
	svc := &probingRoleService{}
	e := roleGranting(aggregatevos.RolePermission{})

	_, _ = domain.GetInsertable(e, svc, "GetInsertable")
	if len(svc.askedNotInCatalog) != 0 {
		t.Error("an empty permission reference was sent to the catalog probe")
	}
}

// A write that adds NOTHING asks nothing. A revoke is an update, and re-judging
// the grants already in the row would make it fail for a permission that was
// retired long after it was granted.
func TestAWriteThatAddsNoGrantAsksNoProbe(t *testing.T) {
	svc := &probingRoleService{notInCatalog: true, isWildcard: true, doesNotHold: true}

	if _, err := domain.GetInsertable(validRole(), svc, "GetInsertable"); err != nil {
		t.Fatalf("a role with no grants was refused by the per-entry rules: %v", roleRejectedFields(err))
	}
	if len(svc.askedNotInCatalog)+len(svc.askedWildcard)+len(svc.askedDoesNotHold) != 0 {
		t.Error("the per-entry probes ran for a write that adds no grant")
	}
}

// ── the manual rules under the OTHER verbs ──────────────────────────────────

// storedRole is a role that already EXISTS: update and archive both require an
// id, so a fixture without one is refused for that before any rule runs. The
// generated suite only ever asserts refusals here, so it never needed this.
func storedRole() *Role {
	e := validRole()
	e.SetID(domain.NewID("7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"))
	return e
}

// The per-entry rules run under IfInsertOrUpdate, so an UPDATE that adds a
// grant is judged exactly like an insert. This is the path the grant route
// takes — a child op is a command on the ROOT and dispatches ModeUpdate.
func TestAnUpdateThatAddsAGrantIsJudgedLikeAnInsert(t *testing.T) {
	svc := &probingRoleService{doesNotHold: true}

	_, err := domain.GetUpdatable(storedRole(), func(x *Role) error {
		x.AddRolePermission(aggregatevos.RolePermission{PermissionID: domain.NewID(grantedPermissionID)})
		return nil
	}, svc, "GetUpdatable")

	if err == nil {
		t.Fatal("a grant added through an update escaped the escalation rule")
	}
	if !roleBlames(err, "Permissions") {
		t.Errorf("the refusal blamed %v, want Permissions", roleRejectedFields(err))
	}
}

// tenant-must-exist is IfInsert ONLY. A tenant archived AFTER the role was
// created must not make the role unwritable — the row is already there, and
// refusing every later edit would strand it.
func TestTheTenantProbeDoesNotRunOnUpdate(t *testing.T) {
	svc := &probingRoleService{tenantUnavailable: true}

	_, err := domain.GetUpdatable(storedRole(), func(*Role) error { return nil }, svc, "GetUpdatable")
	if err != nil {
		t.Fatalf("an update was refused because the tenant is archived: %v", roleRejectedFields(err))
	}
}

// Nor on archive: retiring a role whose tenant is already gone is exactly the
// cleanup somebody needs to be able to do.
func TestTheTenantProbeDoesNotRunOnArchive(t *testing.T) {
	svc := &probingRoleService{tenantUnavailable: true}

	if _, err := domain.GetArchivable(storedRole(), svc, "GetArchivable"); err != nil {
		t.Fatalf("archiving a role under an archived tenant was refused: %v", roleRejectedFields(err))
	}
}

// The WRITE-side row scope binds archive too, and this is the gate that is easy
// to leave untested: a caller who may archive is a different question from
// WHICH row they may archive.
func TestArchivingAnotherTenantsRoleIsRefused(t *testing.T) {
	e := storedRole()
	e.RequestingTenant = "a-different-tenant"

	_, err := domain.GetArchivable(e, &probingRoleService{}, "GetArchivable")
	if err == nil {
		t.Fatal("a caller archived a role belonging to another tenant")
	}
}

// A super-admin crosses the scope on archive as well — that is what lets a
// platform operator retire a customer's role while supporting them.
func TestASuperAdminMayArchiveAnotherTenantsRole(t *testing.T) {
	e := storedRole()
	e.RequestingTenant = "a-different-tenant"
	e.RequestingMayCrossScope = true

	if _, err := domain.GetArchivable(e, &probingRoleService{}, "GetArchivable"); err != nil {
		t.Fatalf("the bypass holder was refused: %v", roleRejectedFields(err))
	}
}

// ── the uniqueness pre-check, and its exclude-self half ─────────────────────

// The pre-check is what lets a duplicate report TOGETHER with the other
// validation errors instead of arriving alone as a 409 after everything else
// passed. The database index is the race backstop behind it, not a replacement.
func TestARoleKeyAlreadyTakenInTheTenantIsRefused(t *testing.T) {
	svc := &probingRoleService{keyTaken: true}

	_, err := domain.GetInsertable(validRole(), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a second role took a key already held in the same tenant")
	}
	if !roleBlames(err, "Key") {
		t.Errorf("the refusal blamed %v, want Key", roleRejectedFields(err))
	}
}

// On UPDATE the row must be excluded from colliding with itself — without the
// exclusion, renaming anything on a role would report its own key as taken.
func TestTheKeyPreCheckExcludesTheRowBeingUpdated(t *testing.T) {
	svc := &probingRoleService{}
	e := storedRole()

	if _, err := domain.GetUpdatable(e, func(*Role) error { return nil }, svc, "GetUpdatable"); err != nil {
		t.Fatalf("an update was refused: %v", roleRejectedFields(err))
	}

	if len(svc.keyTakenSelf) == 0 {
		t.Fatal("the uniqueness pre-check never ran on update")
	}
	last := svc.keyTakenSelf[len(svc.keyTakenSelf)-1]
	if last.IsEmpty() {
		t.Error("the update asked the pre-check with an EMPTY self id — the row would collide with itself")
	}
}

// ── the owner is required, and the probe is what made that matter ───────────

// The owner now travels IN THE REQUEST rather than being taken from the claim,
// and that is what makes the two cases below expressible at all.

// A caller may not create a role in somebody else's tenant — the claim decides
// what they MAY write, even though the request is what carries the value.
func TestCreatingInAnotherTenantIsRefused(t *testing.T) {
	e := validRole()
	e.TenantID = domain.NewID("11111111-1111-5111-8111-111111111111")

	_, err := domain.GetInsertable(e, &probingRoleService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a caller created a role inside another tenant")
	}
	if !roleBlames(err, "TenantID") {
		t.Errorf("the refusal blamed %v, want TenantID", roleRejectedFields(err))
	}
}

// A super-admin MAY, and this is the case the earlier model could not express
// at all: with the owner server-assigned from the claim, the field was absent
// from every write DTO, so a platform operator had no way to say which tenant.
func TestASuperAdminMayCreateInAnyTenant(t *testing.T) {
	e := validRole()
	e.TenantID = domain.NewID("11111111-1111-5111-8111-111111111111")
	e.RequestingMayCrossScope = true

	if _, err := domain.GetInsertable(e, &probingRoleService{}, "GetInsertable"); err != nil {
		t.Fatalf("the bypass holder could not create in another tenant: %v", roleRejectedFields(err))
	}
}

// ── the barrier ─────────────────────────────────────────────────────────────

// An owner that is not a usable id must never reach the uniqueness pre-check.
//
// The pre-check is scoped BY the owner and hands it straight to a criterion, so
// a value a UUID column refuses makes the probe's query error and the probe
// panic — a 500 on a request whose problem is plain validation.
//
// This is a REGRESSION test for a live 500, and it covers BOTH shapes, because
// they used to fail for different reasons and were fixed at different times:
// the empty string, and a non-empty value that is not a UUID. The second one
// survived the first fix — with no identity the scope guard stands down, so
// nothing had rejected anything yet and a `required` rule saw a non-empty value
// and passed it through.
//
// It is the `valueObject` rule with `guard: true` that closes both: it pulls
// domain.ID's own IsValid forward (uuid.Parse refuses "" and "tatu" alike) and
// ends the pass. Drop either half from the spec and the panic comes back.
func TestAnUnusableOwnerNeverReachesTheUniquenessProbe(t *testing.T) {
	for _, owner := range []struct{ name, value string }{
		{"empty", ""},
		{"not a uuid", "tatu"},
	} {
		for _, identity := range []struct {
			name    string
			present bool
		}{
			{"with an identity", true},
			// The dev bench: auth disabled, so the row-scope guard stands down
			// and has nothing to reject. This is the combination that was still
			// answering 500 after the first fix.
			{"with no identity at all", false},
		} {
			t.Run(owner.name+", "+identity.name, func(t *testing.T) {
				e := validRole()
				e.TenantID = domain.NewID(owner.value)
				e.RequestingIdentityPresent = identity.present
				if !identity.present {
					e.RequestingTenant = ""
				}

				svc := &barrierWatch{}
				_, err := domain.GetInsertable(e, svc, "GetInsertable")

				if err == nil {
					t.Fatalf("a role was accepted with %q as its owner", owner.value)
				}
				if svc.probed {
					t.Errorf("the uniqueness probe ran with %q — the real service would bind it to a UUID column and panic into a 500", owner.value)
				}
			})
		}
	}
}

// barrierWatch records whether the uniqueness probe was reached at all.
type barrierWatch struct {
	domain.ServiceBase
	probed bool
}

func (s *barrierWatch) RoleKeyTaken(domain.ID, string, domain.ID) bool {
	s.probed = true
	return false
}
func (s *barrierWatch) TenantIsUnavailable(domain.ID) bool         { return false }
func (s *barrierWatch) PermissionIsNotInCatalog(domain.ID) bool    { return false }
func (s *barrierWatch) PermissionIsWildcard(domain.ID) bool        { return false }
func (s *barrierWatch) CallerDoesNotHoldPermission(domain.ID) bool { return false }
func (s *barrierWatch) CallerIsSuperAdmin() bool                   { return false }

// The same hole the root's owner had, one level down: a grant id that is not a
// usable UUID must not reach the catalog probes, which bind it to a UUID column.
//
// The root is covered by its `valueObject` barrier. A CHILD id is not — the
// framework validates children after the rules, and this loop runs inside them —
// so the guard is explicit and must test USABILITY, not emptiness.
func TestAnUnusableGrantIDNeverReachesTheCatalogProbes(t *testing.T) {
	for _, id := range []string{"", "tatu"} {
		t.Run("id="+id, func(t *testing.T) {
			e := validRole()
			e.AddRolePermission(aggregatevos.RolePermission{PermissionID: domain.NewID(id)})

			svc := &probingRoleService{}
			_, _ = domain.GetInsertable(e, svc, "GetInsertable")

			asked := len(svc.askedNotInCatalog) + len(svc.askedWildcard) + len(svc.askedDoesNotHold)
			if asked != 0 {
				t.Errorf("a grant id of %q reached %d probe(s) — the real service would bind it to a UUID column and panic into a 500", id, asked)
			}
		})
	}
}
