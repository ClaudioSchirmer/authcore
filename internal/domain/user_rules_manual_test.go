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
	"fmt"
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
	passwordUnchanged bool
	claimUnavailable  bool
	claimAppliesElse  bool
	claimValueBadType bool

	askedTenant        int
	askedGroupAvail    []scopedQuestion
	askedGroupWildcard []domain.ID
	askedGroupEscalate []domain.ID
	askedRoleAvail     []scopedQuestion
	askedRoleWildcard  []domain.ID
	askedRoleEscalate  []domain.ID
	hashedPlaintexts   []string
	askedUnchanged     []string
	askedClaimAvail    []scopedQuestion
	askedClaimApplies  []domain.ID
	askedClaimType     []claimTypeQuestion
}

// claimTypeQuestion records BOTH arguments of the value-type probe: which
// definition was asked about AND the value that was judged. The second is what
// proves a CHANGED entry reaches the rule with its NEW value rather than the
// stored one.
type claimTypeQuestion struct {
	claimID domain.ID
	value   string
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

// passwordUnchanged is off by default: the stub's whole posture is "nothing is
// wrong", so a valid fixture passes and each negative case fails for the rule it
// is testing.
func (s *probingUserService) PasswordIsUnchanged(password, hash string) bool {
	s.askedUnchanged = append(s.askedUnchanged, password)
	return s.passwordUnchanged
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
// the server-assigned fields, and it asserts every one: leaving any of them out
// is how a column silently keeps its zero value with nothing reporting it.
func TestInsertDerivesTheWholeCredentialState(t *testing.T) {
	e := validUser()
	// Deliberately WRONG starting values, so the assertions below prove the rule
	// wrote them rather than that the fixture happened to carry them.
	e.PasswordHash = ""
	e.PasswordChangedAt = time.Time{}
	e.MustChangePassword = false
	e.EmailVerifiedAt = nil

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
	// LEFT NIL on purpose: nothing in this service verifies an address, so
	// writing anything here would be a claim it cannot support — and nil is what
	// tells a caller "not verified" instead of a year-zero date.
	if e.EmailVerifiedAt != nil {
		t.Fatalf("EmailVerifiedAt was written (%v) — no verification flow exists to justify it", *e.EmailVerifiedAt)
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

func (s *probingUserService) ClaimIsUnavailableInTenant(tenantID domain.ID, claimID domain.ID) bool {
	s.askedClaimAvail = append(s.askedClaimAvail, scopedQuestion{tenantID: tenantID, targetID: claimID})
	return s.claimUnavailable
}

func (s *probingUserService) ClaimDoesNotApplyToUser(claimID domain.ID) bool {
	s.askedClaimApplies = append(s.askedClaimApplies, claimID)
	return s.claimAppliesElse
}

func (s *probingUserService) ClaimValueDoesNotMatchValueType(claimID domain.ID, value string) bool {
	s.askedClaimType = append(s.askedClaimType, claimTypeQuestion{claimID: claimID, value: value})
	return s.claimValueBadType
}

// ── the claims collection: the three per-entry rules ────────────────────────
//
// Every case below judges what the write CARRIES, which for this collection is
// two sets rather than one: the entries it adds and the entries it changes.
// That second set is what makes this collection different from groups and
// roles, whose entries have nothing to change.

const (
	someClaimID      = "0198f3e0-7b31-7c02-8a55-1f9d2e6b4c17"
	someOtherClaimID = "0198f3e0-8c42-7d13-9b66-2a0e3f7c5d28"
)

// userHolding returns a valid user that SETS one claim value — the shape all
// three per-entry claim rules judge on an insert.
func userHolding(claimID, value string) *User {
	e := validUser()
	e.AddUserClaim(aggregatevos.UserClaim{
		ClaimID: domain.NewID(claimID),
		Value:   vos.ClaimValue(value),
	})
	return e
}

func TestClaimValueIsRefusedWhenTheDefinitionIsNotAvailableInTheTenant(t *testing.T) {
	e := userHolding(someClaimID, "1000")
	svc := insertUser(t, e, &probingUserService{claimUnavailable: true})

	if !hasKey(userNotificationKeys(e), "ClaimNotAvailableInTenantNotification") {
		t.Fatalf("a value was set against an absent, archived or foreign definition; answers were %v",
			userNotificationKeys(e))
	}
	if len(svc.askedClaimAvail) != 1 {
		t.Fatalf("the availability probe was asked %d times, want once per added entry", len(svc.askedClaimAvail))
	}
	// THE ROW's tenant, not the caller's. On the ordinary path they are the same
	// value, but a *:* super-admin crosses that scope — and when they do, "this
	// tenant" has to mean the user's or the rule stops isolating anything.
	if svc.askedClaimAvail[0].tenantID != e.TenantID {
		t.Errorf("the probe was scoped by %v, want the row's tenant %v",
			svc.askedClaimAvail[0].tenantID, e.TenantID)
	}
}

// The three conditions must be INDISTINGUISHABLE to the caller. A separate
// "belongs to another tenant" answer would confirm that a specific UUID is a
// live definition in some other tenant — an existence oracle over a
// competitor's claim vocabulary. The fact collapses them, and this asserts the
// rule does not un-collapse them by reporting something extra.
func TestAnUnavailableDefinitionIsNotDistinguishedFromAForeignOne(t *testing.T) {
	e := userHolding(someClaimID, "1000")
	insertUser(t, e, &probingUserService{claimUnavailable: true})

	keys := userNotificationKeys(e)
	if hasKey(keys, "ClaimDoesNotApplyToUserNotification") ||
		hasKey(keys, "ClaimValueDoesNotMatchValueTypeNotification") {
		t.Fatalf("one bad definition produced more than one answer: %v", keys)
	}
}

func TestNothingBelowIsAskedAboutADefinitionThatIsNotThere(t *testing.T) {
	svc := insertUser(t, userHolding(someClaimID, "1000"), &probingUserService{claimUnavailable: true})

	if len(svc.askedClaimApplies) != 0 || len(svc.askedClaimType) != 0 {
		t.Fatalf("the later probes ran on an unresolvable definition: applies=%d type=%d",
			len(svc.askedClaimApplies), len(svc.askedClaimType))
	}
}

// appliesTo finally means something. Until this rule existed, the column stated
// as data what nothing enforced — the README said so in as many words.
func TestClaimValueIsRefusedWhenTheDefinitionDoesNotApplyToUsers(t *testing.T) {
	e := userHolding(someClaimID, "1000")
	insertUser(t, e, &probingUserService{claimAppliesElse: true})

	if !hasKey(userNotificationKeys(e), "ClaimDoesNotApplyToUserNotification") {
		t.Fatalf("a user holds a value for a client-only definition; answers were %v",
			userNotificationKeys(e))
	}
}

func TestClaimValueIsRefusedWhenItDoesNotParseAsTheDeclaredType(t *testing.T) {
	e := userHolding(someClaimID, "abc")
	svc := insertUser(t, e, &probingUserService{claimValueBadType: true})

	if !hasKey(userNotificationKeys(e), "ClaimValueDoesNotMatchValueTypeNotification") {
		t.Fatalf("a value of the wrong type was stored; answers were %v", userNotificationKeys(e))
	}
	// The VALUE reaches the probe, not just the id: the whole question is
	// whether THIS string parses, and a rule that passed only the id would ask
	// something unanswerable.
	if len(svc.askedClaimType) != 1 || svc.askedClaimType[0].value != "abc" {
		t.Fatalf("the value handed to the type probe was %v", svc.askedClaimType)
	}
}

// The happy path. Without it every case above could pass because the fixture is
// broken rather than because the rule fired.
func TestAValidClaimValueIsAccepted(t *testing.T) {
	e := userHolding(someClaimID, "1000")
	if _, err := domain.GetInsertable(e, &probingUserService{}, "Insert"); err != nil {
		t.Fatalf("a valid claim value was refused: %v", err)
	}
}

// A CHANGED entry is judged like an added one — the PATCH path.
// PATCH /users/{id}/claims/{entryId} carries only `value`; the definition is
// read off the stored entry. So a correction arrives with a string nothing has
// judged, and judging only the additions would let it write anything at all.
func TestACorrectedValueIsJudgedLikeANewOne(t *testing.T) {
	e := validUser()
	original := aggregatevos.UserClaim{ClaimID: domain.NewID(someClaimID), Value: vos.ClaimValue("1000")}
	e.AddUserClaim(original)
	// Add-then-change moves the entry from ADDED to CHANGED, which is the state
	// a correction arrives in.
	domain.ChangeAggregateChild(e, original, aggregatevos.UserClaim{
		ClaimID: domain.NewID(someClaimID),
		Value:   vos.ClaimValue("not-a-number"),
	})

	svc := insertUser(t, e, &probingUserService{claimValueBadType: true})

	if !hasKey(userNotificationKeys(e), "ClaimValueDoesNotMatchValueTypeNotification") {
		t.Fatalf("a corrected value skipped the type check; answers were %v", userNotificationKeys(e))
	}
	if len(svc.askedClaimType) != 1 || svc.askedClaimType[0].value != "not-a-number" {
		t.Fatalf("the probe was handed %v, want the NEW value", svc.askedClaimType)
	}
}

// A second entry for the same definition is the collision the whole two-level
// chain exists to make impossible: one principal holds at most one value per
// definition, so precedence never becomes a question along this chain.
func TestASecondValueForTheSameDefinitionIsRefused(t *testing.T) {
	e := userHolding(someClaimID, "1000")
	e.AddUserClaim(aggregatevos.UserClaim{
		ClaimID: domain.NewID(someClaimID),
		Value:   vos.ClaimValue("2000"),
	})

	if !hasKey(userNotificationKeys(e), "UserAlreadyHoldsClaimNotification") {
		t.Fatalf("one user holds two values for one definition; answers were %v", userNotificationKeys(e))
	}
}

// Two DIFFERENT definitions are the ordinary case and must not collide — the
// duplicate guard is over the definition, not over the collection.
func TestTwoDifferentDefinitionsCoexist(t *testing.T) {
	e := userHolding(someClaimID, "1000")
	e.AddUserClaim(aggregatevos.UserClaim{
		ClaimID: domain.NewID(someOtherClaimID),
		Value:   vos.ClaimValue("sa-east-1"),
	})

	if hasKey(userNotificationKeys(e), "UserAlreadyHoldsClaimNotification") {
		t.Fatalf("two distinct definitions were treated as a duplicate: %v", userNotificationKeys(e))
	}
}

// The cap is a HEADER BUDGET before it is a count: 20 values of up to 256 runes
// is ~5 KB riding on every request to every service, on top of permissions,
// groups and roles.
func TestAUserMayNotHoldMoreThanTwentyClaimValues(t *testing.T) {
	e := validUser()
	for i := 0; i < 21; i++ {
		e.AddUserClaim(aggregatevos.UserClaim{
			ClaimID: domain.NewID(fmt.Sprintf("0198f3e0-7b31-7c02-8a55-1f9d2e6b%04d", i)),
			Value:   vos.ClaimValue("v"),
		})
	}
	insertUser(t, e, &probingUserService{})

	if !hasKey(userNotificationKeys(e), "TooManyClaimsForUserNotification") {
		t.Fatalf("21 claim values were accepted; answers were %v", userNotificationKeys(e))
	}
}

func TestTwentyClaimValuesAreAccepted(t *testing.T) {
	e := validUser()
	for i := 0; i < 20; i++ {
		e.AddUserClaim(aggregatevos.UserClaim{
			ClaimID: domain.NewID(fmt.Sprintf("0198f3e0-7b31-7c02-8a55-1f9d2e6b%04d", i)),
			Value:   vos.ClaimValue("v"),
		})
	}
	insertUser(t, e, &probingUserService{})

	if hasKey(userNotificationKeys(e), "TooManyClaimsForUserNotification") {
		t.Fatal("the cap fired at exactly 20, which is the number the spec allows")
	}
}
