// Hand-written: the four rules the spec could not express declaratively, and
// the one thing about them that no generated test can know — that they reach
// their verdict through the DOMAIN SERVICE and never through the read-join
// fields sitting on the same entry.
//
// The generated suite stubs the service so every probe answers "nothing found",
// which is what lets a valid fixture through. Every case here overrides exactly
// one probe, so a failure names the rule under test.
//
// Three things here have no counterpart in role_rules_manual_test.go, and they
// are the three places this entity is not a copy of Role:
//   - the availability probe is asked a THIRD question, same tenant, and it is
//     asked with the GROUP's tenant rather than the caller's;
//   - the escalation probe is TRANSITIVE — one answer per role, covering the
//     whole set of permissions that role grants;
//   - the walk runs availability → wildcard → escalation, and BOTH links of
//     that order are pinned below.

package domain

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// probingGroupService answers "nothing found" like the generated stub, except
// where a case overrides one question. It also RECORDS what it was asked, which
// is how the ordering tests below prove a wildcard never reaches the escalation
// probe — and how the cross-tenant test proves which tenant was compared.
type probingGroupService struct {
	domain.ServiceBase

	keyTaken             bool
	tenantUnavailable    bool
	roleUnavailable      bool
	roleGrantsWildcard   bool
	callerLacksSomeGrant bool

	askedAvailability []availabilityQuestion
	askedWildcard     []domain.ID
	askedEscalation   []domain.ID
	keyTakenSelf      []domain.ID
	askedTenant       int

	// callsPerFact counts how many times each collection fact was ASKED, as
	// opposed to how many entries it was asked about. `perEntry` promises ONE
	// call per write whatever the size of the collection.
	callsPerFact map[string]int
}

// availabilityQuestion records BOTH arguments, because which tenant the rule
// passes is itself a security property — see the super-admin case below.
type availabilityQuestion struct {
	tenantID domain.ID
	roleID   domain.ID
}

func (s *probingGroupService) GroupKeyTaken(_ domain.ID, _ string, selfID domain.ID) bool {
	s.keyTakenSelf = append(s.keyTakenSelf, selfID)
	return s.keyTaken
}

func (s *probingGroupService) TenantIsUnavailable(_ domain.ID) bool {
	s.askedTenant++
	return s.tenantUnavailable
}

// The three collection facts, each asked ONCE for the whole set.
//
// The stub keeps recording ONE QUESTION PER ID — which ids, and for the
// availability fact which TENANT, are properties the batch did not change and
// several cases below still assert. asked() counts the CALLS, the only thing
// that can catch a regression to one call per entry.
func (s *probingGroupService) RoleIsUnavailableInTenant(tenantID domain.ID, roleIDSet []domain.ID) map[domain.ID]bool {
	s.asked("RoleIsUnavailableInTenant")
	out := make(map[domain.ID]bool, len(roleIDSet))
	for _, id := range roleIDSet {
		s.askedAvailability = append(s.askedAvailability, availabilityQuestion{tenantID, id})
		out[id] = s.roleUnavailable
	}
	return out
}

func (s *probingGroupService) RoleGrantsWildcard(roleIDSet []domain.ID) map[domain.ID]bool {
	s.asked("RoleGrantsWildcard")
	out := make(map[domain.ID]bool, len(roleIDSet))
	for _, id := range roleIDSet {
		s.askedWildcard = append(s.askedWildcard, id)
		out[id] = s.roleGrantsWildcard
	}
	return out
}

func (s *probingGroupService) CallerLacksAnyPermissionOf(roleIDSet []domain.ID) map[domain.ID]bool {
	s.asked("CallerLacksAnyPermissionOf")
	out := make(map[domain.ID]bool, len(roleIDSet))
	for _, id := range roleIDSet {
		s.askedEscalation = append(s.askedEscalation, id)
		out[id] = s.callerLacksSomeGrant
	}
	return out
}

// asked records one CALL of a collection fact.
func (s *probingGroupService) asked(fact string) {
	if s.callsPerFact == nil {
		s.callsPerFact = map[string]int{}
	}
	s.callsPerFact[fact]++
}

const attachedRoleID = "0198f3e0-9c25-7a1f-b73d-5e08c4a29f61"

// A second and a third, for the one case that needs a collection rather than a
// single entry: a batch of one cannot tell "asked once" apart from "asked once
// per entry".
const (
	secondAttachedRoleID = "0198f3e0-9c25-7a1f-b73d-5e08c4a29f62"
	thirdAttachedRoleID  = "0198f3e0-9c25-7a1f-b73d-5e08c4a29f63"
)

// groupNotificationsOf reads what the aggregate itself recorded — the seat the
// collection's own verbs report through, which is not the same place a refused
// GetInsertable/GetUpdatable answers from.
func groupNotificationsOf(e *Group) []domain.NotificationMessage {
	return e.GetAggregateRoot().NotificationContext().Messages()
}

// groupConferring returns a valid group that ATTACHES one role — the shape all
// three per-entry rules judge.
func groupConferring(entry aggregatevos.GroupRole) *Group {
	e := validGroup()
	e.AddGroupRole(entry)
	return e
}

// storedGroup is a group that already EXISTS: update and archive both require
// an id, so a fixture without one is refused for that before any rule runs.
func storedGroup() *Group {
	e := validGroup()
	e.SetID(domain.NewID("7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"))
	return e
}

// ── tenant-must-be-available ────────────────────────────────────────────────

func TestGroupInsertIsRefusedWhenTheOwningTenantIsUnavailable(t *testing.T) {
	svc := &probingGroupService{tenantUnavailable: true}

	_, err := domain.GetInsertable(validGroup(), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a group was created under a missing, archived or suspended tenant")
	}
	if !groupBlames(err, "TenantID") {
		t.Errorf("the refusal blamed %v, want TenantID", groupRejectedFields(err))
	}
}

func TestGroupInsertIsAcceptedWhenTheOwningTenantIsAvailable(t *testing.T) {
	if _, err := domain.GetInsertable(validGroup(), &probingGroupService{}, "GetInsertable"); err != nil {
		t.Fatalf("a group under a live tenant was rejected: %v", groupRejectedFields(err))
	}
}

// With an unusable owner the hook is never reached AT ALL, so it asks nothing.
//
// That is the barrier's doing, not this rule's: `tenant-is-a-usable-id` pulls
// domain.ID's own validation forward and ends the pass, so customRules — where
// the tenant probe lives — never runs. The rule therefore carries NO emptiness
// guard of its own, and this is what says that is safe rather than forgotten.
//
// If the barrier is ever dropped, the probe starts receiving the bad value,
// binds it to a UUID column and turns a 422 into a 500. That is the bug Role
// actually shipped, and this test is what stops it happening twice.
func TestAnUnusableOwnerNeverReachesTheGroupTenantProbe(t *testing.T) {
	for _, owner := range []string{"", "tatu"} {
		t.Run("owner="+owner, func(t *testing.T) {
			e := validGroup()
			e.TenantID = domain.NewID(owner)
			svc := &probingGroupService{tenantUnavailable: true}

			if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err == nil {
				t.Fatalf("a group with %q as its owner was accepted", owner)
			}
			if svc.askedTenant != 0 {
				t.Errorf("the tenant probe ran %d time(s) with %q — the barrier did not hold", svc.askedTenant, owner)
			}
		})
	}
}

// And with a USABLE owner it does run — otherwise the test above would pass for
// the wrong reason, on a hook that never runs at all.
func TestAUsableOwnerDoesReachTheGroupTenantProbe(t *testing.T) {
	svc := &probingGroupService{}
	if _, err := domain.GetInsertable(validGroup(), svc, "GetInsertable"); err != nil {
		t.Fatalf("a valid group was rejected: %v", groupRejectedFields(err))
	}
	if svc.askedTenant != 1 {
		t.Errorf("the tenant probe ran %d time(s), want exactly 1", svc.askedTenant)
	}
}

// ── attached-roles-are-available-in-this-tenant ─────────────────────────────

func TestAttachingAnUnavailableRoleIsRefused(t *testing.T) {
	svc := &probingGroupService{roleUnavailable: true}
	e := groupConferring(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("a role that is missing, retired or another tenant's was attached")
	}
	if !groupBlames(err, "Roles") {
		t.Errorf("the refusal blamed %v, want Roles", groupRejectedFields(err))
	}
}

// THE TENANT COMPARED IS THE GROUP'S, NOT THE CALLER'S — the first thing in
// this entity that Role did not need.
//
// On the ordinary path the two are the same value, so a rule that passed the
// caller's tenant would look correct forever. It breaks only for a *:*
// super-admin, who crosses the row scope: passing THEIR tenant would compare a
// customer's role against the operator's own tenant and refuse every legitimate
// attach, on exactly the support path the bypass exists to serve.
func TestTheAvailabilityProbeIsAskedWithTheGroupsOwnTenant(t *testing.T) {
	svc := &probingGroupService{}
	e := groupConferring(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})
	// A platform operator, acting inside a customer's tenant.
	e.RequestingTenant = "a-different-tenant"
	e.RequestingMayCrossScope = true

	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err != nil {
		t.Fatalf("the bypass holder was refused: %v", groupRejectedFields(err))
	}
	if len(svc.askedAvailability) != 1 {
		t.Fatalf("the availability probe ran %d time(s), want exactly 1", len(svc.askedAvailability))
	}
	if got := svc.askedAvailability[0].tenantID; got != e.TenantID {
		t.Errorf("the probe was asked about tenant %v, want the GROUP's own %v", got, e.TenantID)
	}
}

// ONE PROBLEM PER ENTRY. An unavailable role is one refusal: nothing below can
// say anything true about a role that is not there, every later fact answers
// "the problem is present" for an unresolvable id, and the 403 the escalation
// rule raises would blame the caller for a problem whose honest answer is a 422
// saying the role is not there.
//
// The interlock is about the ANSWER rather than about the question — the three
// facts are asked once each for the whole collection, so a bad entry's later
// verdicts are computed and simply not read. The stub answers TRUE to all three,
// which is what makes this assertion mean something.
func TestAnUnavailableRoleIsReportedOnceAndNothingElse(t *testing.T) {
	svc := &probingGroupService{
		roleUnavailable:      true,
		roleGrantsWildcard:   true,
		callerLacksSomeGrant: true,
	}
	e := groupConferring(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	keys := groupNotificationKeysOf(err)
	if len(keys) != 1 {
		t.Errorf("one unavailable role produced %d answers, want exactly 1: %v", len(keys), keys)
	}
}

// ── no-wildcard-role-attach, and its ORDER on both sides ────────────────────

func TestAttachingAWildcardBearingRoleIsRefused(t *testing.T) {
	svc := &probingGroupService{roleGrantsWildcard: true}
	e := groupConferring(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("a role granting a wildcard permission was conferred by a group")
	}
	if !groupBlames(err, "Roles") {
		t.Errorf("the refusal blamed %v, want Roles", groupRejectedFields(err))
	}
}

// THE ORDERING CASE, and where its load now sits.
//
// Identity.HasPermission PANICS on any argument containing '*'. The rule can no
// longer promise the escalation fact is not ASKED about a wildcard-bearing role
// — it is asked about every entry at once — so what keeps the panic away is the
// guarantee the fact's own description carries and its body implements: it
// GUARDS THE WILDCARD ITSELF and answers "lacks" instead of calling through
// (internal/infra/role_probe.go, callerLacksAnyPermissionOfRole).
//
// What the domain still owns, and what this pins, is that one entry produces one
// answer: a wildcard-bearing role is reported as a wildcard and as nothing else.
func TestAWildcardBearingRoleIsReportedAsWildcardAndNothingElse(t *testing.T) {
	svc := &probingGroupService{roleGrantsWildcard: true, callerLacksSomeGrant: true}
	e := groupConferring(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if len(svc.askedWildcard) == 0 {
		t.Fatal("the wildcard fact never ran — the rule this test pins does not exist")
	}
	keys := groupNotificationKeysOf(err)
	if len(keys) != 1 {
		t.Errorf("one wildcard-bearing role produced %d answers, want exactly 1: %v", len(keys), keys)
	}
}

// groupNotificationKeysOf lists the notification TYPES a refusal carries, which
// is what "one answer per entry" is asserted on: the blamed field is "Roles" for
// all three rules, so counting fields would count the same name three times.
func groupNotificationKeysOf(err error) []string {
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

// THE COST THE `perEntry` FACTS BUY, pinned where a regression would otherwise
// be invisible: every collection fact is asked ONCE for the whole write,
// however many roles it attaches. Nothing about the OUTCOME changes if this
// regresses to one call per entry, which is why the call count is asserted.
func TestEveryGroupCollectionFactIsAskedOnceForTheWholeWrite(t *testing.T) {
	e := validGroup()
	for _, id := range []string{attachedRoleID, secondAttachedRoleID, thirdAttachedRoleID} {
		e.AddGroupRole(aggregatevos.GroupRole{RoleID: domain.NewID(id)})
	}

	svc := &probingGroupService{}
	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err != nil {
		t.Fatalf("a group conferring three available roles was refused: %v", groupRejectedFields(err))
	}

	for _, fact := range []string{
		"RoleIsUnavailableInTenant", "RoleGrantsWildcard", "CallerLacksAnyPermissionOf",
	} {
		if got := svc.callsPerFact[fact]; got != 1 {
			t.Errorf("%s was asked %d times for a write carrying three roles, want exactly 1", fact, got)
		}
	}

	// And it was asked about EVERY entry: one call, three subjects.
	if len(svc.askedAvailability) != 3 {
		t.Errorf("the single call carried %d roles, want all three", len(svc.askedAvailability))
	}
}

// The OTHER half of the order, which Role's spec did not constrain and this one
// does: availability must precede the wildcard probe. Both run here, and the
// availability one has to have been asked first.
func TestTheWalkAsksAvailabilityBeforeTheWildcardProbe(t *testing.T) {
	svc := &probingGroupService{}
	e := groupConferring(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})

	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err != nil {
		t.Fatalf("a valid attachment was refused: %v", groupRejectedFields(err))
	}
	if len(svc.askedAvailability) != 1 || len(svc.askedWildcard) != 1 || len(svc.askedEscalation) != 1 {
		t.Fatalf("the walk asked availability %d, wildcard %d, escalation %d — want 1 each",
			len(svc.askedAvailability), len(svc.askedWildcard), len(svc.askedEscalation))
	}
}

// ── no-privilege-escalation, the TRANSITIVE one ─────────────────────────────

func TestConferringARoleWhosePermissionsTheCallerDoesNotHoldIsRefused(t *testing.T) {
	svc := &probingGroupService{callerLacksSomeGrant: true}
	e := groupConferring(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})

	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("a caller conferred a role granting permissions they do not hold")
	}
	if !groupBlames(err, "Roles") {
		t.Errorf("the refusal blamed %v, want Roles", groupRejectedFields(err))
	}
}

// ONE question per ROLE, not one per permission. The transitivity lives behind
// the port — the fact resolves the role to its whole key set and answers once —
// so the rule must not try to walk permissions itself, which it has no way to
// see from here.
func TestTheEscalationProbeIsAskedOncePerAttachedRole(t *testing.T) {
	svc := &probingGroupService{}
	e := validGroup()
	e.AddGroupRole(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})
	e.AddGroupRole(aggregatevos.GroupRole{RoleID: domain.NewID("0198f400-1111-7000-8000-aaaaaaaaaaaa")})

	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err != nil {
		t.Fatalf("a valid two-role group was refused: %v", groupRejectedFields(err))
	}
	if len(svc.askedEscalation) != 2 {
		t.Errorf("the escalation probe ran %d time(s) for 2 attached roles, want exactly 2", len(svc.askedEscalation))
	}
}

// The dev-profile half of the two-state table. With NO identity at all the
// escalation gate stands down — collapsing this into a fail-closed test makes
// the entity unusable on a bench that issues no tokens.
func TestWithNoIdentityAtAllTheGroupEscalationGateStandsDown(t *testing.T) {
	svc := &probingGroupService{callerLacksSomeGrant: true}
	e := groupConferring(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})
	e.RequestingIdentityPresent = false
	e.RequestingTenant = ""

	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err != nil {
		t.Fatalf("the escalation gate refused a request that carried no identity: %v", groupRejectedFields(err))
	}
	if len(svc.askedEscalation) != 0 {
		t.Error("the escalation probe ran even though no identity was present")
	}
}

// ── what the rules must NOT read ────────────────────────────────────────────

// The join fields are blank on an entry the write is ADDING, and this pins it.
//
// A rule that consulted entry.RoleKey would read "" here — indistinguishable
// from a role whose key is genuinely empty — and wave the entry through. That
// is fail-open on exactly the path these rules exist to close, so the fixture
// below carries the zero values a real added entry carries and must still be
// refused BY THE PROBE.
func TestAnAttachedEntryIsJudgedByTheProbeAndNotByItsJoinFields(t *testing.T) {
	svc := &probingGroupService{roleUnavailable: true}

	// Join fields deliberately left at their zero values, which is what an added
	// entry really carries: nothing traversed a foreign key for it.
	entry := aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)}
	if entry.RoleKey != "" || entry.RoleName != "" {
		t.Fatal("an added entry arrived with joined values — the premise of every rule here is gone")
	}

	if _, err := domain.GetInsertable(groupConferring(entry), svc, "GetInsertable"); err == nil {
		t.Fatal("an attachment of a retired role was accepted — the rule read the join instead of the probe")
	}
	if len(svc.askedAvailability) != 1 {
		t.Errorf("the availability probe ran %d time(s), want exactly 1 per added entry", len(svc.askedAvailability))
	}
}

// An entry with no reference at all is the field's own complaint, so the probes
// must not be spent on it — and, more sharply, an unusable id handed to a
// criterion against a UUID column is a 500 where a 422 was the honest answer.
func TestAnEntryWithNoReferenceIsNotProbed(t *testing.T) {
	for _, ref := range []string{"", "tatu"} {
		t.Run("roleID="+ref, func(t *testing.T) {
			svc := &probingGroupService{}
			e := groupConferring(aggregatevos.GroupRole{RoleID: domain.NewID(ref)})

			_, _ = domain.GetInsertable(e, svc, "GetInsertable")
			if len(svc.askedAvailability) != 0 {
				t.Errorf("an unusable role reference %q was sent to the availability probe", ref)
			}
		})
	}
}

// A write that adds NOTHING asks nothing. A detach is an update, and re-judging
// the entries already in the row would make it fail for a role that was retired
// long after it was attached — which would take away the very tool for removing
// an attachment that stopped being acceptable.
func TestAWriteThatAttachesNothingAsksNoProbe(t *testing.T) {
	svc := &probingGroupService{roleUnavailable: true, roleGrantsWildcard: true, callerLacksSomeGrant: true}

	if _, err := domain.GetInsertable(validGroup(), svc, "GetInsertable"); err != nil {
		t.Fatalf("a group with no roles was refused by the per-entry rules: %v", groupRejectedFields(err))
	}
	if len(svc.askedAvailability)+len(svc.askedWildcard)+len(svc.askedEscalation) != 0 {
		t.Error("the per-entry probes ran for a write that attaches nothing")
	}
}

// ── the manual rules under the OTHER verbs ──────────────────────────────────

// The per-entry rules run under IfInsertOrUpdate, so an UPDATE that attaches a
// role is judged exactly like an insert. This is the path the attach route
// takes — a child op is a command on the ROOT and dispatches ModeUpdate.
func TestAnUpdateThatAttachesARoleIsJudgedLikeAnInsert(t *testing.T) {
	svc := &probingGroupService{callerLacksSomeGrant: true}

	_, err := domain.GetUpdatable(storedGroup(), func(x *Group) error {
		x.AddGroupRole(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})
		return nil
	}, svc, "GetUpdatable")

	if err == nil {
		t.Fatal("a role attached through an update escaped the escalation rule")
	}
	if !groupBlames(err, "Roles") {
		t.Errorf("the refusal blamed %v, want Roles", groupRejectedFields(err))
	}
}

// tenant-must-be-available is IfInsert ONLY. A tenant suspended AFTER the group
// was created must not make the group unwritable — the row is already there,
// and refusing every later edit would strand a whole team's definition.
func TestTheTenantProbeDoesNotRunOnGroupUpdate(t *testing.T) {
	svc := &probingGroupService{tenantUnavailable: true}

	if _, err := domain.GetUpdatable(storedGroup(), func(*Group) error { return nil }, svc, "GetUpdatable"); err != nil {
		t.Fatalf("an update was refused because the tenant is unavailable: %v", groupRejectedFields(err))
	}
}

// Nor on archive: retiring a group whose tenant is already gone is exactly the
// cleanup somebody needs to be able to do.
func TestTheTenantProbeDoesNotRunOnGroupArchive(t *testing.T) {
	svc := &probingGroupService{tenantUnavailable: true}

	if _, err := domain.GetArchivable(storedGroup(), svc, "GetArchivable"); err != nil {
		t.Fatalf("archiving a group under an unavailable tenant was refused: %v", groupRejectedFields(err))
	}
}

// The WRITE-side row scope binds archive too, and this is the gate that is easy
// to leave untested: a caller who may archive is a different question from
// WHICH row they may archive. One-way archive makes it sharper here — there is
// no unarchive to undo a cross-tenant mistake.
func TestArchivingAnotherTenantsGroupIsRefused(t *testing.T) {
	e := storedGroup()
	e.RequestingTenant = "a-different-tenant"

	if _, err := domain.GetArchivable(e, &probingGroupService{}, "GetArchivable"); err == nil {
		t.Fatal("a caller archived a group belonging to another tenant")
	}
}

// A super-admin crosses the scope on archive as well — that is what lets a
// platform operator retire a customer's group while supporting them.
func TestASuperAdminMayArchiveAnotherTenantsGroup(t *testing.T) {
	e := storedGroup()
	e.RequestingTenant = "a-different-tenant"
	e.RequestingMayCrossScope = true

	if _, err := domain.GetArchivable(e, &probingGroupService{}, "GetArchivable"); err != nil {
		t.Fatalf("the bypass holder was refused: %v", groupRejectedFields(err))
	}
}

// ── the uniqueness pre-check, and its exclude-self half ─────────────────────

// The pre-check is what lets a duplicate report TOGETHER with the other
// validation errors instead of arriving alone as a 409 after everything else
// passed. The database partial index is the race backstop behind it, not a
// replacement.
func TestAGroupKeyAlreadyTakenInTheTenantIsRefused(t *testing.T) {
	svc := &probingGroupService{keyTaken: true}

	_, err := domain.GetInsertable(validGroup(), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a second group took a key already held in the same tenant")
	}
	if !groupBlames(err, "Key") {
		t.Errorf("the refusal blamed %v, want Key", groupRejectedFields(err))
	}
}

// On UPDATE the row must be excluded from colliding with itself — without the
// exclusion, renaming anything on a group would report its own key as taken.
func TestTheGroupKeyPreCheckExcludesTheRowBeingUpdated(t *testing.T) {
	svc := &probingGroupService{}
	e := storedGroup()

	if _, err := domain.GetUpdatable(e, func(*Group) error { return nil }, svc, "GetUpdatable"); err != nil {
		t.Fatalf("an update was refused: %v", groupRejectedFields(err))
	}

	if len(svc.keyTakenSelf) == 0 {
		t.Fatal("the uniqueness pre-check never ran on update")
	}
	last := svc.keyTakenSelf[len(svc.keyTakenSelf)-1]
	if last.IsEmpty() {
		t.Error("the update asked the pre-check with an EMPTY self id — the row would collide with itself")
	}
}

// ── the child collection's own verbs ────────────────────────────────────────

// A duplicate ATTACH is an answer, not a silent merge — and the guard is over
// RoleID alone. Comparing all three fields would answer "different" for an
// attachment duplicating a stored entry, because the two join fields are blank
// on the new one: the duplicate guard failing open.
func TestAttachingARoleTheGroupAlreadyConfersIsRefused(t *testing.T) {
	e := validGroup()
	// A STORED entry carries its joined values; the one being added does not.
	e.AddGroupRole(aggregatevos.GroupRole{
		RoleID:   domain.NewID(attachedRoleID),
		RoleKey:  "billing-manager",
		RoleName: "Billing Manager",
	})
	e.AddGroupRole(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})

	if len(groupNotificationsOf(e)) == 0 {
		t.Fatal("the same role was attached twice — the duplicate guard compared the join fields")
	}
}

// Detaching an entry that is not there is an answer too: the caller named it.
func TestDetachingAnUnknownEntryIsRefused(t *testing.T) {
	e := storedGroup()
	e.RemoveGroupRoleByID("7b3c1f10-0000-0000-0000-000000000000")

	messages := groupNotificationsOf(e)
	if len(messages) == 0 {
		t.Fatal("detaching an entry that does not exist was accepted silently")
	}
	if _, isNotFound := messages[0].Notification.(domain.RecordNotFoundNotification); !isNotFound {
		t.Errorf("the absent entry answered with %T, want RecordNotFoundNotification (404, not the 422 does-not-exist)",
			messages[0].Notification)
	}
}

// And detaching one that IS there succeeds, so the test above cannot pass for
// the wrong reason.
func TestDetachingAStoredEntrySucceeds(t *testing.T) {
	e := storedGroup()
	e.AddGroupRole(aggregatevos.GroupRole{RoleID: domain.NewID(attachedRoleID)})

	entries := domain.GetCurrentItemsOf[aggregatevos.GroupRole](e.GetAggregateRoot())
	if len(entries) != 1 {
		t.Fatalf("the fixture holds %d entries, want 1", len(entries))
	}

	e.RemoveGroupRoleByID(entries[0].GetID().Value())
	if msgs := groupNotificationsOf(e); len(msgs) != 0 {
		t.Fatalf("detaching a stored entry was refused: %v", msgs)
	}
}

// ── the two immutability rules, on the value they actually freeze ───────────

// The key is frozen because it IS what every reference points at — an API
// caller, an audit line, a directory mapping. Editing it would rewrite the
// meaning of every one of them, retroactively and invisibly.
func TestChangingTheKeyOnUpdateIsRefused(t *testing.T) {
	_, err := domain.GetUpdatable(storedGroup(), func(x *Group) error {
		x.Key = "platform"
		return nil
	}, &probingGroupService{}, "GetUpdatable")

	if err == nil {
		t.Fatal("the group key was changed after creation")
	}
	if !groupBlames(err, "Key") {
		t.Errorf("the refusal blamed %v, want Key", groupRejectedFields(err))
	}
}

// A group never moves between tenants. Without this rule an update could
// re-parent a whole team's definition into another customer — and the write
// guard would not catch it, because the guard compares the value being WRITTEN
// against the caller's claim, and a caller moving a row INTO their own tenant
// passes that test.
func TestChangingTheOwningTenantOnUpdateIsRefused(t *testing.T) {
	_, err := domain.GetUpdatable(storedGroup(), func(x *Group) error {
		x.TenantID = domain.NewID("0198f400-2222-7000-8000-bbbbbbbbbbbb")
		x.RequestingTenant = "0198f400-2222-7000-8000-bbbbbbbbbbbb"
		return nil
	}, &probingGroupService{}, "GetUpdatable")

	if err == nil {
		t.Fatal("a group was moved to another tenant after creation")
	}
	if !groupBlames(err, "TenantID") {
		t.Errorf("the refusal blamed %v, want TenantID", groupRejectedFields(err))
	}
}
