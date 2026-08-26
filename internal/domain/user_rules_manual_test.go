// Tests for user_rules_manual.go — the invariants the spec declared as ones the
// DSL could not express, which is exactly why no generated test stands behind
// them.
//
// They follow group_rules_manual_test.go: a probing service that answers
// "nothing found" like the generated stub except where a case overrides one
// question, and that RECORDS what it was asked — which is how the ordering
// cases below prove that a wildcard never reaches the escalation probe, and how
// the cross-tenant cases prove WHICH tenant was compared.

package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

const (
	joinedGroupID  = "0198f3e0-9c25-7a1f-b73d-5e08c4a29f61"
	grantedRoleID  = "0198f3e0-1a44-7bb2-9c31-77c0d5e1b904"
	someOtherTenID = "0198f4aa-1111-7c9e-9f2a-6d3b1e77a410"
)

// probingUserService records every question and answers "nothing wrong" by
// default — the same posture the generated suite's stub takes, which is what
// lets a valid fixture through and makes each negative case fail for the rule
// it is testing rather than for something the stub invented.
type probingUserService struct {
	domain.ServiceBase

	emailTaken        bool
	tenantUnavailable bool
	groupUnavailable  bool
	groupWildcard     bool
	lacksGroupPerm    bool
	roleUnavailable   bool
	roleWildcard      bool
	lacksRolePerm     bool

	askedTenant        int
	askedGroupAvail    []scopedQuestion
	askedGroupWildcard []domain.ID
	askedGroupEscalate []domain.ID
	askedRoleAvail     []scopedQuestion
	askedRoleWildcard  []domain.ID
	askedRoleEscalate  []domain.ID
	hashedPlaintexts   []string
}

// scopedQuestion records BOTH arguments, because which tenant the rule passes is
// itself a security property — see the super-admin case below.
type scopedQuestion struct {
	tenantID domain.ID
	targetID domain.ID
}

func (s *probingUserService) EmailTaken(string, domain.ID) bool { return s.emailTaken }

func (s *probingUserService) HashPassword(password string) string {
	s.hashedPlaintexts = append(s.hashedPlaintexts, password)
	return "$argon2id$v=19$m=19456,t=2,p=1$c29tZXNhbHQ$" + password
}

func (s *probingUserService) TenantIsUnavailable(domain.ID) bool {
	s.askedTenant++
	return s.tenantUnavailable
}

func (s *probingUserService) GroupIsUnavailableInTenant(tenantID, groupID domain.ID) bool {
	s.askedGroupAvail = append(s.askedGroupAvail, scopedQuestion{tenantID, groupID})
	return s.groupUnavailable
}

func (s *probingUserService) GroupGrantsWildcard(id domain.ID) bool {
	s.askedGroupWildcard = append(s.askedGroupWildcard, id)
	return s.groupWildcard
}

func (s *probingUserService) CallerLacksAnyPermissionOfGroup(id domain.ID) bool {
	s.askedGroupEscalate = append(s.askedGroupEscalate, id)
	return s.lacksGroupPerm
}

func (s *probingUserService) RoleIsUnavailableInTenant(tenantID, roleID domain.ID) bool {
	s.askedRoleAvail = append(s.askedRoleAvail, scopedQuestion{tenantID, roleID})
	return s.roleUnavailable
}

func (s *probingUserService) RoleGrantsWildcard(id domain.ID) bool {
	s.askedRoleWildcard = append(s.askedRoleWildcard, id)
	return s.roleWildcard
}

func (s *probingUserService) CallerLacksAnyPermissionOfRole(id domain.ID) bool {
	s.askedRoleEscalate = append(s.askedRoleEscalate, id)
	return s.lacksRolePerm
}

// userNotificationsOf reads what the aggregate itself recorded — the seat the
// rules report through.
func userNotificationsOf(e *User) []domain.NotificationMessage {
	return e.GetAggregateRoot().NotificationContext().Messages()
}

func userNotificationKeys(e *User) []string {
	var keys []string
	for _, m := range userNotificationsOf(e) {
		keys = append(keys, strings.TrimPrefix(userTypeName(m.Notification), "*"))
	}
	return keys
}

// userTypeName is the notification's Go type name, which IS its translation key
// — so asserting on it is asserting on the answer the caller actually receives,
// not on a message that could be reworded.
func userTypeName(n domain.Notification) string {
	if n == nil {
		return "<nil>"
	}
	t := reflect.TypeOf(n)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// insertUser runs the insert-side rules over e and returns the service so a case
// can assert what was asked.
func insertUser(t *testing.T, e *User, svc *probingUserService) *probingUserService {
	t.Helper()
	_, _ = domain.GetInsertable(e, svc, "Insert")
	return svc
}

// userJoining returns a valid user that JOINS one group — the shape all three
// per-entry group rules judge.
func userJoining(groupID string) *User {
	e := validUser()
	e.AddUserGroup(aggregatevos.UserGroup{GroupID: domain.NewID(groupID)})
	return e
}

// userGranting returns a valid user that is granted one role directly.
func userGranting(roleID string) *User {
	e := validUser()
	e.AddUserRole(aggregatevos.UserRole{RoleID: domain.NewID(roleID)})
	return e
}

func hasKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}

// ── credential-derivation ──────────────────────────────────────────────────

// TestInsertDerivesTheWholeCredentialState is the test for the rule that fills
// six server-assigned fields, and it asserts all six: leaving any of them out is
// how a column silently keeps its zero value with nothing reporting it.
func TestInsertDerivesTheWholeCredentialState(t *testing.T) {
	e := validUser()
	// Deliberately WRONG starting values, so the assertions below prove the rule
	// wrote them rather than that the fixture happened to carry them.
	e.PasswordHash = ""
	e.PasswordChangedAt = time.Time{}
	e.MustChangePassword = false
	e.FailedLoginAttempts = 7
	e.LockedUntil = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	e.EmailVerifiedAt = time.Time{}

	svc := insertUser(t, e, &probingUserService{})

	if e.PasswordHash == "" {
		t.Fatal("the hash was not derived — the row would authenticate nobody")
	}
	if len(svc.hashedPlaintexts) != 1 || svc.hashedPlaintexts[0] != "Str0ng!Passphrase" {
		t.Fatalf("the plaintext handed to the hasher was %v", svc.hashedPlaintexts)
	}
	if e.PasswordChangedAt.IsZero() {
		t.Fatal("PasswordChangedAt was not stamped — the post-incident report has nothing to filter on")
	}
	if !e.MustChangePassword {
		t.Fatal("MustChangePassword is false — the creator knows the password and nothing forces a rotation")
	}
	if e.FailedLoginAttempts != 0 || !e.LockedUntil.IsZero() {
		t.Fatalf("the lockout pair did not start at rest: attempts=%d lockedUntil=%v", e.FailedLoginAttempts, e.LockedUntil)
	}
	// LEFT at zero on purpose: nothing in this service verifies an address, so
	// writing anything here would be a claim it cannot support.
	if !e.EmailVerifiedAt.IsZero() {
		t.Fatalf("EmailVerifiedAt was written (%v) — no verification flow exists to justify it", e.EmailVerifiedAt)
	}
}

// TestTheDerivationNeverStoresThePlaintext is the assertion that the hash is a
// hash. It would catch the one-character mistake of assigning Password instead
// of its hash — which compiles, passes every shape test, and stores credentials
// in the clear.
func TestTheDerivationNeverStoresThePlaintext(t *testing.T) {
	e := validUser()
	e.PasswordHash = ""

	insertUser(t, e, &probingUserService{})

	if e.PasswordHash == "Str0ng!Passphrase" {
		t.Fatal("the plaintext was stored as the hash")
	}
	if !strings.HasPrefix(e.PasswordHash, "$argon2id$") {
		t.Fatalf("what landed in PasswordHash is not an encoded hash: %q", e.PasswordHash)
	}
}

// ── password-echoes-identity ───────────────────────────────────────────────

func TestPasswordEchoingTheIdentityIsRefused(t *testing.T) {
	cases := map[string]vos.Password{
		"the e-mail local part":     "Maria@2026!x",  // local part is "maria"
		"the given name":            "XMaria2026!x",  // "maria", 5 runes
		"the family name":           "Souza2026!xY",  // "souza", 5 runes
		"a later word of the name":  "XLima2026!xYz", // "lima", exactly the 4-rune floor
		"different case in the pwd": "MARIA2026!xY",
	}

	for name, password := range cases {
		t.Run(name, func(t *testing.T) {
			e := validUser()
			e.Password = password
			e.PasswordConfirmation = string(password)

			insertUser(t, e, &probingUserService{})

			if !hasKey(userNotificationKeys(e), "PasswordEchoesIdentityNotification") {
				t.Fatalf("a password echoing the identity was accepted: %q (raised %v)", password, userNotificationKeys(e))
			}
		})
	}
}

// TestAShortNameWordDoesNotBanEveryPassword is the guard on the four-rune floor.
//
// Without it a family name like "Ng" would refuse every password containing
// "ng" — a rule that reads sensible and locks a population out.
func TestAShortNameWordDoesNotBanEveryPassword(t *testing.T) {
	e := validUser()
	e.Name = vos.PersonName{Given: "Wei", Family: "Ng"}
	e.Email = vos.Email("wei@acme.com")
	e.Password = "Strong1!ng"
	e.PasswordConfirmation = "Strong1!ng"

	insertUser(t, e, &probingUserService{})

	if hasKey(userNotificationKeys(e), "PasswordEchoesIdentityNotification") {
		t.Fatal("a two-rune family name banned a password that merely contains those letters")
	}
}

// TestTheEchoRuleNeverEchoesThePassword — the same security property the value
// object has, at the rule level.
func TestTheEchoRuleNeverEchoesThePassword(t *testing.T) {
	const secret = "Maria@2026!x"

	e := validUser()
	e.Password = vos.Password(secret)
	e.PasswordConfirmation = secret

	insertUser(t, e, &probingUserService{})

	for _, m := range userNotificationsOf(e) {
		if strings.Contains(m.FieldValue, secret) {
			t.Fatalf("the plaintext reached a notification payload: %q", m.FieldValue)
		}
	}
}

// ── tenant-available ───────────────────────────────────────────────────────

func TestUserInsertIsRefusedWhenTheOwningTenantIsUnavailable(t *testing.T) {
	e := validUser()
	insertUser(t, e, &probingUserService{tenantUnavailable: true})

	if !hasKey(userNotificationKeys(e), "UserTenantDoesNotExistNotification") {
		t.Fatalf("a user was created inside an unavailable tenant (raised %v)", userNotificationKeys(e))
	}
}

// ── the group walk: order, scope and what it judges ────────────────────────

func TestJoiningAnUnavailableGroupIsRefusedAndStopsTheWalk(t *testing.T) {
	e := userJoining(joinedGroupID)
	svc := insertUser(t, e, &probingUserService{groupUnavailable: true})

	if !hasKey(userNotificationKeys(e), "GroupNotAvailableInTenantNotification") {
		t.Fatalf("an unavailable group was accepted (raised %v)", userNotificationKeys(e))
	}
	// Nothing below can say anything true about a group that is not there, and
	// the wildcard probe answers TRUE for an unknown id — so reporting all three
	// for one bad id would be noise the caller has to read past.
	if len(svc.askedGroupWildcard) != 0 || len(svc.askedGroupEscalate) != 0 {
		t.Fatalf("an unavailable group still reached the later probes: wildcard=%v escalation=%v",
			svc.askedGroupWildcard, svc.askedGroupEscalate)
	}
}

// TestTheGroupWalkAsksTheUsersTenantNotTheCallers is the super-admin case, and
// it is a security property rather than a detail.
//
// A *:* operator creating a user inside a customer's tenant has a DIFFERENT
// RequestingTenant from the row's. Comparing the group against the caller's
// tenant would refuse every legitimate join an operator makes.
func TestTheGroupWalkAsksTheUsersTenantNotTheCallers(t *testing.T) {
	e := userJoining(joinedGroupID)
	e.RequestingTenant = someOtherTenID // the operator's own tenant
	e.RequestingMayCrossScope = true

	svc := insertUser(t, e, &probingUserService{})

	if len(svc.askedGroupAvail) != 1 {
		t.Fatalf("expected one availability question, got %d", len(svc.askedGroupAvail))
	}
	if svc.askedGroupAvail[0].tenantID != e.TenantID {
		t.Fatalf("the probe was asked with %q; the row's tenant is %q",
			svc.askedGroupAvail[0].tenantID.String(), e.TenantID.String())
	}
}

// TestAWildcardBearingGroupNeverReachesTheEscalationProbe is the interlock, and
// the order is load-bearing rather than stylistic: Identity.HasPermission PANICS
// on any argument containing '*', so this is what removes the input that would
// crash the request into a 500 — on exactly the case the escalation rule exists
// to stop.
func TestAWildcardBearingGroupNeverReachesTheEscalationProbe(t *testing.T) {
	e := userJoining(joinedGroupID)
	svc := insertUser(t, e, &probingUserService{groupWildcard: true})

	if !hasKey(userNotificationKeys(e), "CannotJoinWildcardGroupNotification") {
		t.Fatalf("a wildcard-bearing group was accepted (raised %v)", userNotificationKeys(e))
	}
	if len(svc.askedGroupEscalate) != 0 {
		t.Fatalf("the wildcard group reached the escalation probe: %v", svc.askedGroupEscalate)
	}
}

func TestJoiningAGroupTheCallerDoesNotFullyHoldIsRefused(t *testing.T) {
	e := userJoining(joinedGroupID)
	insertUser(t, e, &probingUserService{lacksGroupPerm: true})

	if !hasKey(userNotificationKeys(e), "CannotJoinGroupWithUnheldPermissionsNotification") {
		t.Fatalf("a caller conferred a group they do not fully hold (raised %v)", userNotificationKeys(e))
	}
}

// TestTheEscalationProbeStandsDownWithNoIdentity keeps the two absent states
// apart: no identity at all is auth.mode disabled, which the framework permits
// only on a dev bench, and collapsing it into "refuse" would make the entity
// unusable there.
func TestTheEscalationProbeStandsDownWithNoIdentity(t *testing.T) {
	e := userJoining(joinedGroupID)
	e.RequestingIdentityPresent = false

	svc := insertUser(t, e, &probingUserService{lacksGroupPerm: true})

	if len(svc.askedGroupEscalate) != 0 {
		t.Fatalf("the escalation probe ran with no identity: %v", svc.askedGroupEscalate)
	}
	if hasKey(userNotificationKeys(e), "CannotJoinGroupWithUnheldPermissionsNotification") {
		t.Fatal("the escalation rule refused a write that carried no identity at all")
	}
}

// TestAnUnusableGroupIdNeverReachesAProbe is the 500-avoidance case: an id
// uuid.Parse refuses would bind into a criterion against a UUID column, the
// driver would reject it, and the probe would panic — turning a validation
// problem into a 500. The framework's own child validation reports the bad id,
// so the rule says nothing and simply does not ask.
func TestAnUnusableGroupIdNeverReachesAProbe(t *testing.T) {
	e := userJoining("not-a-uuid")
	svc := insertUser(t, e, &probingUserService{})

	if len(svc.askedGroupAvail) != 0 {
		t.Fatalf("an unusable id reached the availability probe: %v", svc.askedGroupAvail)
	}
}

// ── the role walk: the same three, one hop shorter ──────────────────────────

func TestGrantingAnUnavailableRoleIsRefusedAndStopsTheWalk(t *testing.T) {
	e := userGranting(grantedRoleID)
	svc := insertUser(t, e, &probingUserService{roleUnavailable: true})

	if !hasKey(userNotificationKeys(e), "RoleNotAvailableInTenantNotification") {
		t.Fatalf("an unavailable role was granted (raised %v)", userNotificationKeys(e))
	}
	if len(svc.askedRoleWildcard) != 0 || len(svc.askedRoleEscalate) != 0 {
		t.Fatalf("an unavailable role still reached the later probes: wildcard=%v escalation=%v",
			svc.askedRoleWildcard, svc.askedRoleEscalate)
	}
}

func TestAWildcardBearingDirectRoleNeverReachesTheEscalationProbe(t *testing.T) {
	e := userGranting(grantedRoleID)
	svc := insertUser(t, e, &probingUserService{roleWildcard: true})

	if !hasKey(userNotificationKeys(e), "CannotGrantWildcardRoleNotification") {
		t.Fatalf("a wildcard-bearing role was granted (raised %v)", userNotificationKeys(e))
	}
	if len(svc.askedRoleEscalate) != 0 {
		t.Fatalf("the wildcard role reached the escalation probe: %v", svc.askedRoleEscalate)
	}
}

func TestGrantingARoleTheCallerDoesNotHoldIsRefused(t *testing.T) {
	e := userGranting(grantedRoleID)
	insertUser(t, e, &probingUserService{lacksRolePerm: true})

	if !hasKey(userNotificationKeys(e), "CannotGrantRoleWithUnheldPermissionsNotification") {
		t.Fatalf("a caller granted a role they do not hold (raised %v)", userNotificationKeys(e))
	}
}

// ── archive-forces-suspended ───────────────────────────────────────────────

// TestArchivingForcesSuspended asserts the mutation on the ENTITY, not on the
// audit event. A rule that reached only the trail would look right in the event
// and leave an archived-and-active row in the table.
func TestArchivingForcesSuspended(t *testing.T) {
	e := validUser()
	e.SetID(domain.NewID("7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"))
	e.Status = vos.UserStatus("active")

	_, _ = domain.GetArchivable(e, &probingUserService{}, "Archive")

	if e.Status.Value() != "suspended" {
		t.Fatalf("archiving left the status %q — archived+active is meant to be unrepresentable", e.Status.Value())
	}
}
