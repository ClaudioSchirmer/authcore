package domain

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The four rules in role_rules_manual.go are the only logic in this entity with
// no generated test behind it — which is exactly why they are the ones worth
// testing by hand.
//
// They reuse validRole(), roleBlames() and roleRejectedFields() from the
// generated role_test.go: one fixture, so a change to the aggregate's valid
// shape cannot leave this file asserting against a stale one. What it does NOT
// reuse is stubRoleService, which answers every probe "nothing found" — these
// rules only fire when a probe says otherwise.

// factStub answers each probe from a field, so one test sets exactly the fact
// it is about and leaves the rest at the "nothing wrong" default. It also
// COUNTS the calls, which is how the ordering guarantees below are asserted:
// the rules promise not merely a verdict but that certain questions are never
// asked.
type factStub struct {
	domain.ServiceBase

	keyTaken     bool
	tenantGone   bool
	notInCatalog map[string]bool
	isWildcard   map[string]bool
	callerLacks  map[string]bool

	tenantCalls   int
	catalogCalls  int
	wildcardCalls int
	callerCalls   int
}

func (s *factStub) RoleKeyTaken(_ domain.ID, _ string, _ domain.ID) bool { return s.keyTaken }

func (s *factStub) TenantIsUnavailable(_ domain.ID) bool {
	s.tenantCalls++
	return s.tenantGone
}

func (s *factStub) PermissionIsNotInCatalog(id domain.ID) bool {
	s.catalogCalls++
	return s.notInCatalog[id.Value()]
}

func (s *factStub) PermissionIsWildcard(id domain.ID) bool {
	s.wildcardCalls++
	return s.isWildcard[id.Value()]
}

func (s *factStub) CallerDoesNotHoldPermission(id domain.ID) bool {
	s.callerCalls++
	return s.callerLacks[id.Value()]
}

// Two catalog ids the cases below grant, name and break individually.
const (
	grantA = "9f14b0a2-6d38-4c5e-b7a1-2e0c5d81f4a3"
	grantB = "1b7c33d5-4e29-4a71-9f02-6d5e8c14b7a2"
)

// roleGranting is validRole() carrying the given catalog ids as grants.
func roleGranting(ids ...string) *Role {
	e := validRole()
	for _, id := range ids {
		e.AddRolePermission(aggregatevos.RolePermission{PermissionID: domain.NewID(id)})
	}
	return e
}

// persisted stamps an id on a fixture, which every UPDATE path requires: an
// entity with no id is refused with UnableToUpdateWithoutIDNotification before
// any rule verdict can be read, so a case that omitted it would pass while
// proving nothing about the rule it names.
func persisted(e *Role) *Role {
	e.SetID(domain.NewID("7c9e6679-7425-40de-944b-e07fc1f90ae7"))
	return e
}

// raisedNotifications lists the notification TYPES a rejection carried, so a
// case can assert WHICH complaint the caller reads rather than only that
// something failed — the three grant rules all blame the same field.
func raisedNotifications(err error) []string {
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		return nil
	}
	var out []string
	for _, ctx := range carrier.NotificationContexts() {
		for _, msg := range ctx.Messages() {
			out = append(out, reflect.TypeOf(msg.Notification).Name())
		}
	}
	return out
}

func countNotification(err error, want string) int {
	var n int
	for _, name := range raisedNotifications(err) {
		if name == want {
			n++
		}
	}
	return n
}

func raisedNotification(err error, want string) bool { return countNotification(err, want) > 0 }

// ── tenant-must-be-active ──────────────────────────────────────────────────

// A role under a tenant that is gone would be grantable and unreachable at once.
func TestRole_TenantMustBeActive_RefusesAnUnavailableTenant(t *testing.T) {
	svc := &factStub{tenantGone: true}
	_, err := domain.GetInsertable(validRole(), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a role under an unknown or archived tenant was accepted")
	}
	if !roleBlames(err, "TenantID") {
		t.Errorf("the rejection should name TenantID, it named %v", roleRejectedFields(err))
	}
	if !raisedNotification(err, "RoleTenantDoesNotExistNotification") {
		t.Errorf("expected RoleTenantDoesNotExistNotification, got %v", raisedNotifications(err))
	}
}

// An empty id is refused WITHOUT asking the database. The probe would answer
// "not found" anyway, and this is the one write where the answer is knowable
// without a query — so the assertion is on the call count, not on the verdict.
func TestRole_TenantMustBeActive_EmptyIDIsRefusedWithoutAProbe(t *testing.T) {
	e := validRole()
	e.TenantID = domain.ID{}
	// The caller is a super-admin, so the tenant-scope guard stands aside and
	// the only thing left to refuse the write is the rule under test.
	e.RequestingMayCrossScope = true

	svc := &factStub{}
	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	if err == nil {
		t.Fatal("a role with no tenant was accepted")
	}
	if !raisedNotification(err, "RoleTenantDoesNotExistNotification") {
		t.Errorf("expected RoleTenantDoesNotExistNotification, got %v", raisedNotifications(err))
	}
	if svc.tenantCalls != 0 {
		t.Errorf("an empty tenant id should cost no probe, it cost %d", svc.tenantCalls)
	}
}

// Insert only. The tenant is immutable afterwards, so a tenant archived LATER
// must not freeze edits to the roles that already exist under it — those rows
// are exactly what an access review still needs to read.
func TestRole_TenantMustBeActive_DoesNotFireOnUpdate(t *testing.T) {
	svc := &factStub{tenantGone: true}
	_, err := domain.GetUpdatable(persisted(validRole()), func(*Role) error { return nil }, svc, "GetUpdatable")
	if err != nil {
		t.Fatalf("an update was refused because the tenant is archived: %v (fields: %v)",
			err, roleRejectedFields(err))
	}
	if svc.tenantCalls != 0 {
		t.Errorf("the tenant probe should not run on update, it ran %d time(s)", svc.tenantCalls)
	}
}

// ── the three grant rules ──────────────────────────────────────────────────

// A valid role carrying ordinary grants is accepted — otherwise every negative
// case below would pass for the wrong reason.
func TestRole_GrantsThatAreFineAreAccepted(t *testing.T) {
	svc := &factStub{}
	if _, err := domain.GetInsertable(roleGranting(grantA, grantB), svc, "GetInsertable"); err != nil {
		t.Fatalf("a role with two ordinary grants was rejected: %v (fields: %v)",
			err, roleRejectedFields(err))
	}
	// The happy path asks all three questions of both entries, and nothing more.
	if svc.catalogCalls != 2 || svc.wildcardCalls != 2 || svc.callerCalls != 2 {
		t.Errorf("expected 2 of each probe, got catalog=%d wildcard=%d caller=%d",
			svc.catalogCalls, svc.wildcardCalls, svc.callerCalls)
	}
}

func TestRole_GrantOfARetiredPermission_IsRefused(t *testing.T) {
	svc := &factStub{notInCatalog: map[string]bool{grantA: true}}
	_, err := domain.GetInsertable(roleGranting(grantA), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a grant of a permission outside the catalog was accepted")
	}
	if !raisedNotification(err, "PermissionNotInCatalogNotification") {
		t.Errorf("expected PermissionNotInCatalogNotification, got %v", raisedNotifications(err))
	}
	if !roleBlames(err, "Permissions") {
		t.Errorf("the rejection should name Permissions, it named %v", roleRejectedFields(err))
	}
}

func TestRole_GrantOfAWildcardPermission_IsRefused(t *testing.T) {
	svc := &factStub{isWildcard: map[string]bool{grantA: true}}
	_, err := domain.GetInsertable(roleGranting(grantA), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a wildcard grant was accepted")
	}
	if !raisedNotification(err, "CannotGrantWildcardPermissionNotification") {
		t.Errorf("expected CannotGrantWildcardPermissionNotification, got %v", raisedNotifications(err))
	}
}

func TestRole_GrantingWhatTheCallerDoesNotHold_IsRefused(t *testing.T) {
	svc := &factStub{callerLacks: map[string]bool{grantA: true}}
	_, err := domain.GetInsertable(roleGranting(grantA), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a caller granted a permission they do not hold")
	}
	if !raisedNotification(err, "CannotGrantUnheldPermissionNotification") {
		t.Errorf("expected CannotGrantUnheldPermissionNotification, got %v", raisedNotifications(err))
	}
}

// ── the ordering guarantees ────────────────────────────────────────────────

// THE panic guard. Identity.HasPermission panics on any argument containing
// '*', so the caller-holds question must never be asked about a wildcard grant.
// The verdict alone would not prove it — the call count does.
func TestRole_WildcardGrant_NeverReachesTheCallerHoldsQuestion(t *testing.T) {
	svc := &factStub{
		isWildcard: map[string]bool{grantA: true},
		// Set deliberately: if the walk ever asked, this would answer and the
		// rejection would carry the wrong notification too.
		callerLacks: map[string]bool{grantA: true},
	}
	_, err := domain.GetInsertable(roleGranting(grantA), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a wildcard grant was accepted")
	}
	if svc.callerCalls != 0 {
		t.Errorf("the caller-holds question was asked about a wildcard %d time(s) — it panics on one", svc.callerCalls)
	}
	if raisedNotification(err, "CannotGrantUnheldPermissionNotification") {
		t.Errorf("a wildcard grant should be refused as a wildcard, got %v", raisedNotifications(err))
	}
}

// An UNKNOWN id must read as "not in the catalog", never as "you tried to
// escalate". PermissionIsWildcard deliberately answers true for an id it cannot
// resolve — so asking it first would blame the caller for privilege escalation
// (403) when the honest answer is that the permission is not there (422).
func TestRole_UnresolvableGrant_ReadsAsNotInCatalogAndNotAsAWildcard(t *testing.T) {
	svc := &factStub{
		notInCatalog: map[string]bool{grantA: true},
		// What the real fact does for an unresolvable id.
		isWildcard: map[string]bool{grantA: true},
	}
	_, err := domain.GetInsertable(roleGranting(grantA), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a grant of an unknown permission was accepted")
	}
	if !raisedNotification(err, "PermissionNotInCatalogNotification") {
		t.Errorf("expected PermissionNotInCatalogNotification, got %v", raisedNotifications(err))
	}
	if raisedNotification(err, "CannotGrantWildcardPermissionNotification") {
		t.Errorf("an unknown permission was reported as a wildcard grant: %v", raisedNotifications(err))
	}
	// And the entry is abandoned: neither later question has anything to work
	// with once the id resolves to no key at all.
	if svc.wildcardCalls != 0 || svc.callerCalls != 0 {
		t.Errorf("an unresolvable entry should be abandoned, got wildcard=%d caller=%d",
			svc.wildcardCalls, svc.callerCalls)
	}
}

// Each refusal is raised AT MOST ONCE. A role may carry up to 200 grants, and a
// caller who pasted the wrong list learns nothing from reading the same
// complaint two hundred times.
func TestRole_ARepeatedProblemIsReportedOnce(t *testing.T) {
	svc := &factStub{callerLacks: map[string]bool{grantA: true, grantB: true}}
	_, err := domain.GetInsertable(roleGranting(grantA, grantB), svc, "GetInsertable")
	if err == nil {
		t.Fatal("two unheld grants were accepted")
	}
	if n := countNotification(err, "CannotGrantUnheldPermissionNotification"); n != 1 {
		t.Errorf("expected exactly one CannotGrantUnheldPermissionNotification, got %d (%v)",
			n, raisedNotifications(err))
	}
}

// Different problems on different entries are reported side by side: the caller
// fixes both in one round rather than discovering the second after fixing the
// first.
func TestRole_DifferentProblemsAreReportedTogether(t *testing.T) {
	svc := &factStub{
		notInCatalog: map[string]bool{grantA: true},
		callerLacks:  map[string]bool{grantB: true},
	}
	_, err := domain.GetInsertable(roleGranting(grantA, grantB), svc, "GetInsertable")
	if err == nil {
		t.Fatal("two separately broken grants were accepted")
	}
	for _, want := range []string{
		"PermissionNotInCatalogNotification",
		"CannotGrantUnheldPermissionNotification",
	} {
		if !raisedNotification(err, want) {
			t.Errorf("expected %s among the answers, got %v", want, raisedNotifications(err))
		}
	}
}

// The grant rules run on UPDATE too, which is where they matter most: a grant
// arrives through the child ops, and those dispatch as an update.
func TestRole_GrantRulesFireOnUpdateToo(t *testing.T) {
	svc := &factStub{callerLacks: map[string]bool{grantA: true}}
	e := persisted(roleGranting(grantA))
	_, err := domain.GetUpdatable(e, func(*Role) error { return nil }, svc, "GetUpdatable")
	if err == nil {
		t.Fatal("an unheld grant was accepted on update")
	}
	if !raisedNotification(err, "CannotGrantUnheldPermissionNotification") {
		t.Errorf("expected CannotGrantUnheldPermissionNotification, got %v", raisedNotifications(err))
	}
}

// A role with no grants at all asks nothing and is accepted. It is the shape a
// role is created in before its first grant, so it must not be refused.
func TestRole_NoGrantsAsksNothing(t *testing.T) {
	svc := &factStub{}
	if _, err := domain.GetInsertable(validRole(), svc, "GetInsertable"); err != nil {
		t.Fatalf("a role with no grants was rejected: %v (fields: %v)", err, roleRejectedFields(err))
	}
	if svc.catalogCalls != 0 || svc.wildcardCalls != 0 || svc.callerCalls != 0 {
		t.Errorf("an empty collection should cost no probe, got catalog=%d wildcard=%d caller=%d",
			svc.catalogCalls, svc.wildcardCalls, svc.callerCalls)
	}
}
