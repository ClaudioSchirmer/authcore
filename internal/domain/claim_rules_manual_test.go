// Hand-written: the two rules the spec could not express declaratively.
//
// The generated suite stubs the service so every probe answers "nothing found",
// which is what lets a valid fixture through. Every case here overrides exactly
// one probe, so a failure names the rule under test.
//
// The case this file exists for above all others is
// TestClaimDefaultValueMayBeNullForEveryType: a null default is ALWAYS valid,
// and it is the branch a suite written from the happy path forgets. "No
// default" and "an empty default" are not the same thing to a consumer — the
// first leaves the claim out of the token entirely.

package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// probingClaimService answers "nothing found" like the generated stub, except
// where a case overrides one question. It RECORDS what it was asked, which is
// how the scope tests below prove the tenant probe fires on insert and not on
// update.
type probingClaimService struct {
	domain.ServiceBase

	nameTaken         bool
	tenantUnavailable bool
	heldByAUser       bool
	heldByAClient     bool

	askedTenant     int
	askedTenantWith []domain.ID

	// The narrowing guard must ask about the kinds a change DROPS and about
	// nothing else, so these counters are the assertion for the widening cases:
	// a widening that queries two tables is a correctness bug no assertion on
	// the ANSWER would catch, because both answers would be "nobody holds it".
	askedHeldByAUser   int
	askedHeldByAClient int

	// The catalog cap counts ONE enum member per call and adds two answers to
	// get a bucket. A member absent from the map answers 0 — the same empty
	// catalog the generated stub reports — so every case written before this
	// rule existed still passes the cap untouched.
	activeByAppliesTo map[string]int64
	askedActiveWith   []string
}

func (s *probingClaimService) ClaimNameTaken(_ domain.ID, _ string, _ domain.ID) bool {
	return s.nameTaken
}

func (s *probingClaimService) TenantIsUnavailable(tenantID domain.ID) bool {
	s.askedTenant++
	s.askedTenantWith = append(s.askedTenantWith, tenantID)
	return s.tenantUnavailable
}

func (s *probingClaimService) ClaimIsHeldByAUser(_ domain.ID, _ string) bool {
	s.askedHeldByAUser++
	return s.heldByAUser
}

func (s *probingClaimService) ClaimIsHeldByAClient(_ domain.ID, _ string) bool {
	s.askedHeldByAClient++
	return s.heldByAClient
}

func (s *probingClaimService) ActiveClaimsWithAppliesTo(_ domain.ID, appliesTo string) int64 {
	s.askedActiveWith = append(s.askedActiveWith, appliesTo)
	return s.activeByAppliesTo[appliesTo]
}

// claimWithDefault returns a valid claim whose declared type and default are
// the pair under test. A nil default means the column holds NULL.
func claimWithDefault(valueType vos.ClaimValueType, defaultValue *string) *Claim {
	e := validClaim()
	e.ValueType = valueType
	e.DefaultValue = defaultValue
	return e
}

// storedClaim is a claim that already has a row — an id. Every UPDATE case
// needs one: the framework refuses an update with no id, and the refusal would
// otherwise be mistaken for the rule under test refusing it.
func storedClaim() *Claim {
	e := validClaim()
	e.SetID(domain.NewID(claimRowID))
	return e
}

const claimRowID = "7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"

// ── tenant-must-exist ───────────────────────────────────────────────────────

func TestClaimInsertIsRefusedWhenTheOwningTenantIsUnavailable(t *testing.T) {
	svc := &probingClaimService{tenantUnavailable: true}

	_, err := domain.GetInsertable(validClaim(), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a claim was created under a missing, archived or suspended tenant")
	}
	if !claimBlames(err, "TenantID") {
		t.Errorf("the refusal blamed %v, want TenantID", claimRejectedFields(err))
	}
}

func TestClaimInsertIsAcceptedWhenTheOwningTenantIsAvailable(t *testing.T) {
	svc := &probingClaimService{}
	if _, err := domain.GetInsertable(validClaim(), svc, "GetInsertable"); err != nil {
		t.Fatalf("a valid claim under an available tenant was refused: %v", err)
	}
	if svc.askedTenant != 1 {
		t.Errorf("the tenant probe was asked %d times on an insert, want exactly 1", svc.askedTenant)
	}
}

// The rule is scoped to INSERT, and that is deliberate rather than an omission:
// a tenant suspended AFTER a definition was created must not make that
// definition impossible to correct. Role's identically-scoped rule makes the
// same call.
func TestClaimUpdateDoesNotAskWhetherTheTenantIsAvailable(t *testing.T) {
	svc := &probingClaimService{tenantUnavailable: true}

	_, err := domain.GetUpdatable(storedClaim(), func(x *Claim) error {
		x.Description = vos.Description("A corrected explanation of what this value means.")
		return nil
	}, svc, "GetUpdatable")
	if err != nil {
		t.Fatalf("an update was refused because the tenant is unavailable: %v", err)
	}
	if svc.askedTenant != 0 {
		t.Errorf("the tenant probe was asked %d times on an update, want 0", svc.askedTenant)
	}
}

// The probe is handed the OWNER's id, not the caller's tenant. They are the
// same value on an ordinary write and different ones for a super-admin
// creating a definition inside a customer's tenant — which is exactly the case
// that would silently check the wrong row.
func TestClaimTenantProbeIsAskedAboutTheOwnerNotTheCaller(t *testing.T) {
	svc := &probingClaimService{}
	e := validClaim()
	e.RequestingMayCrossScope = true
	e.RequestingTenant = "00000000-0000-0000-0000-0000000000ff"

	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err != nil {
		t.Fatalf("a super-admin insert into another tenant was refused: %v", err)
	}
	if len(svc.askedTenantWith) != 1 || svc.askedTenantWith[0] != e.TenantID {
		t.Errorf("the probe was asked about %v, want the owner %v", svc.askedTenantWith, e.TenantID)
	}
}

// ── default-value-matches-value-type ────────────────────────────────────────

// A NULL default is valid for EVERY declared type. It means the claim has no
// default and simply does not enter the token — a legitimate state, not a
// missing value, and the branch this rule is most likely to get wrong.
func TestClaimDefaultValueMayBeNullForEveryType(t *testing.T) {
	for _, valueType := range []vos.ClaimValueType{
		vos.ClaimValueTypeString,
		vos.ClaimValueTypeNumber,
		vos.ClaimValueTypeBool,
	} {
		t.Run(valueType.Value(), func(t *testing.T) {
			e := claimWithDefault(valueType, nil)
			if _, err := domain.GetInsertable(e, &probingClaimService{}, "GetInsertable"); err != nil {
				t.Errorf("a null default under %q was refused: %v", valueType.Value(), err)
			}
		})
	}
}

func TestClaimDefaultValueIsAcceptedWhenItMatchesTheDeclaredType(t *testing.T) {
	accepted := []struct {
		valueType vos.ClaimValueType
		value     string
	}{
		{vos.ClaimValueTypeString, "1000"},
		{vos.ClaimValueTypeString, "any prose at all"},
		{vos.ClaimValueTypeNumber, "1000"},
		{vos.ClaimValueTypeNumber, "12.5"},
		{vos.ClaimValueTypeNumber, "-3"},
		{vos.ClaimValueTypeNumber, "0"},
		{vos.ClaimValueTypeBool, "true"},
		{vos.ClaimValueTypeBool, "false"},
	}
	for _, c := range accepted {
		t.Run(c.valueType.Value()+"/"+c.value, func(t *testing.T) {
			e := claimWithDefault(c.valueType, new(c.value))
			if _, err := domain.GetInsertable(e, &probingClaimService{}, "GetInsertable"); err != nil {
				t.Errorf("%q was refused as a %s default: %v", c.value, c.valueType.Value(), err)
			}
		})
	}
}

func TestClaimDefaultValueIsRefusedWhenItContradictsTheDeclaredType(t *testing.T) {
	refused := []struct {
		valueType vos.ClaimValueType
		value     string
		why       string
	}{
		{vos.ClaimValueTypeString, "", "an empty default is not 'no default' — that is what NULL is for"},
		{vos.ClaimValueTypeNumber, "abc", "not a number at all"},
		{vos.ClaimValueTypeNumber, "", "empty is not a number"},
		{vos.ClaimValueTypeNumber, " 1000", "not trimmed into a number — nothing here normalizes"},
		{vos.ClaimValueTypeNumber, "NaN", "ParseFloat accepts it; a consumer cannot branch on it"},
		{vos.ClaimValueTypeNumber, "Inf", "same, and it would ride a token as literal text"},
		{vos.ClaimValueTypeNumber, "-Inf", "same in the other direction"},
		{vos.ClaimValueTypeBool, "TRUE", "one spelling per truth value, not ParseBool's vocabulary"},
		{vos.ClaimValueTypeBool, "1", "ParseBool accepts it; the token carries true or false"},
		{vos.ClaimValueTypeBool, "yes", "not a boolean in any spelling this service mints"},
	}
	for _, c := range refused {
		t.Run(c.valueType.Value()+"/"+c.why, func(t *testing.T) {
			e := claimWithDefault(c.valueType, new(c.value))
			_, err := domain.GetInsertable(e, &probingClaimService{}, "GetInsertable")
			if err == nil {
				t.Fatalf("%q was accepted as a %s default (%s)", c.value, c.valueType.Value(), c.why)
			}
			if !claimBlames(err, "DefaultValue") {
				t.Errorf("the refusal blamed %v, want DefaultValue", claimRejectedFields(err))
			}
		})
	}
}

// An unknown ValueType is answered ONCE, by the value object, and this rule
// stays quiet.
//
// The switch reads the enum member rather than the raw string precisely so the
// Unknown sentinel falls through without a verdict: refusing here too would
// tell the caller about one bad field in two different sentences, and the
// second one would point at DefaultValue, which is fine.
func TestClaimUnknownValueTypeIsAnsweredOnceAndNotBlamedOnTheDefault(t *testing.T) {
	e := claimWithDefault(vos.ClaimValueType("decimal"), new("12.5"))

	_, err := domain.GetInsertable(e, &probingClaimService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a claim with a value type outside the closed set was accepted")
	}
	if !claimBlames(err, "ValueType") {
		t.Errorf("the refusal blamed %v, want ValueType", claimRejectedFields(err))
	}
	if claimBlames(err, "DefaultValue") {
		t.Errorf("the default was blamed too: one bad field answered twice — %v", claimRejectedFields(err))
	}
}

// The rule fires on UPDATE as well as on insert: correcting a default is the
// ordinary write this catalog exists for, and it is exactly where a wrong value
// would otherwise slip in unchecked.
func TestClaimDefaultValueIsCheckedOnUpdateToo(t *testing.T) {
	e := claimWithDefault(vos.ClaimValueTypeNumber, new("1000"))
	e.SetID(domain.NewID(claimRowID))

	_, err := domain.GetUpdatable(e, func(x *Claim) error {
		x.DefaultValue = new("not a number")
		return nil
	}, &probingClaimService{}, "GetUpdatable")
	if err == nil {
		t.Fatal("an update set a default that contradicts the declared type")
	}
	if !claimBlames(err, "DefaultValue") {
		t.Errorf("the refusal blamed %v, want DefaultValue", claimRejectedFields(err))
	}
}

// ── branches of the GENERATED rules the generated suite does not reach ───────
//
// Three of them, and they are here rather than in claim_test.go because that
// file is generated and hashed. Each one is a rule the spec declares and the
// generator wrote correctly; what was missing was a case that runs it.

// The claim-size budget, enforced by the declared length rule before the
// column width would answer the same thing as a 500.
func TestClaimDefaultValueIsRefusedWhenItExceedsTheBudget(t *testing.T) {
	tooLong := strings.Repeat("a", 257)
	e := claimWithDefault(vos.ClaimValueTypeString, &tooLong)

	_, err := domain.GetInsertable(e, &probingClaimService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a default one rune past the budget was accepted")
	}
	if !claimBlames(err, "DefaultValue") {
		t.Errorf("the refusal blamed %v, want DefaultValue", claimRejectedFields(err))
	}
}

// The pre-check half of the uniqueness enforcement: the duplicate is reported
// TOGETHER with the other validation errors instead of arriving alone as a 409
// from the database after everything else has passed.
func TestClaimInsertIsRefusedWhenTheNameIsAlreadyTakenInTheTenant(t *testing.T) {
	svc := &probingClaimService{nameTaken: true}

	_, err := domain.GetInsertable(validClaim(), svc, "GetInsertable")
	if err == nil {
		t.Fatal("a second active claim took a name already held in this tenant")
	}
	if !claimBlames(err, "Name") {
		t.Errorf("the refusal blamed %v, want Name", claimRejectedFields(err))
	}
}

// TenantID immutability, reached through the ONLY caller who can get to it.
//
// For everybody else the write dies earlier and for a different reason: moving
// the row to another tenant makes it foreign, so refuseForeignTenant answers
// 403 and the barrier ends the pass before the update gate runs. A super-admin
// crosses that scope, so the pass continues and the immutability rule is what
// refuses the move — which is exactly what it is there for.
func TestClaimTenantIsImmutableEvenForACallerWhoCrossesTheScope(t *testing.T) {
	e := storedClaim()
	e.RequestingMayCrossScope = true

	_, err := domain.GetUpdatable(e, func(x *Claim) error {
		x.TenantID = domain.NewID("00000000-0000-0000-0000-000000000002")
		return nil
	}, &probingClaimService{}, "GetUpdatable")
	if err == nil {
		t.Fatal("a super-admin moved a claim definition to another tenant")
	}
	if !claimBlames(err, "TenantID") {
		t.Errorf("the refusal blamed %v, want TenantID", claimRejectedFields(err))
	}
}

// ── applies-to-narrowing-refused ────────────────────────────────────────────
//
// AppliesTo is mutable in BOTH directions, and only one of them is safe.
// Widening is the ordinary operational move — a definition that started `user`
// becoming `both` the day the machine side of an integration arrives. Narrowing
// against values that are already held STRANDS those rows: they stay in the
// table, they stay readable, and they would be refused if anybody tried to
// write them, with nothing anywhere complaining.
//
// Six transitions are possible and four of them narrow. Every one of the six
// has a case below, because a rule written as "did it stop being Both?" passes
// the obvious pair and misses the cross pair entirely.

// narrowTo runs an update that moves AppliesTo from `from` to `to` and returns
// the service, so a case can assert BOTH the outcome and what was asked.
func narrowTo(from, to vos.ClaimAppliesTo, svc *probingClaimService) (*probingClaimService, error) {
	e := storedClaim()
	e.AppliesTo = from

	_, err := domain.GetUpdatable(e, func(x *Claim) error {
		x.AppliesTo = to
		return nil
	}, svc, "GetUpdatable")
	return svc, err
}

func TestNarrowingBothToUserIsRefusedWhenAClientHoldsAValue(t *testing.T) {
	_, err := narrowTo(vos.ClaimAppliesToBoth, vos.ClaimAppliesToUser,
		&probingClaimService{heldByAClient: true})

	if err == nil {
		t.Fatal("a definition was narrowed away from clients that still hold values for it")
	}
	if !claimBlames(err, "AppliesTo") {
		t.Errorf("the refusal blamed %v, want AppliesTo", claimRejectedFields(err))
	}
}

func TestNarrowingBothToClientIsRefusedWhenAUserHoldsAValue(t *testing.T) {
	_, err := narrowTo(vos.ClaimAppliesToBoth, vos.ClaimAppliesToClient,
		&probingClaimService{heldByAUser: true})

	if err == nil {
		t.Fatal("a definition was narrowed away from users that still hold values for it")
	}
}

// THE CROSS PAIR. user -> client keeps neither side of `both`, so it drops
// users — a rule that only watched for the loss of `both` would wave it
// through, and this is the case that catches that mistake.
func TestNarrowingUserToClientIsRefusedWhenAUserHoldsAValue(t *testing.T) {
	_, err := narrowTo(vos.ClaimAppliesToUser, vos.ClaimAppliesToClient,
		&probingClaimService{heldByAUser: true})

	if err == nil {
		t.Fatal("a definition moved from users to clients while users still hold values for it")
	}
}

func TestNarrowingClientToUserIsRefusedWhenAClientHoldsAValue(t *testing.T) {
	_, err := narrowTo(vos.ClaimAppliesToClient, vos.ClaimAppliesToUser,
		&probingClaimService{heldByAClient: true})

	if err == nil {
		t.Fatal("a definition moved from clients to users while clients still hold values for it")
	}
}

// A definition NOBODY holds a value for narrows freely, and that has to keep
// working. Refusing every narrowing would make the field effectively immutable,
// which the model gate decided the other way — so this case is as load-bearing
// as the four refusals above.
func TestNarrowingIsAllowedWhenNobodyHoldsAValue(t *testing.T) {
	svc, err := narrowTo(vos.ClaimAppliesToBoth, vos.ClaimAppliesToUser, &probingClaimService{})

	if err != nil {
		t.Fatalf("a definition nobody holds a value for could not be narrowed: %v", err)
	}
	if svc.askedHeldByAClient != 1 {
		t.Errorf("the client side was asked %d times, want once — that is the kind being dropped",
			svc.askedHeldByAClient)
	}
	// The USER side is not being dropped, so it must not be asked at all.
	if svc.askedHeldByAUser != 0 {
		t.Errorf("the user side was asked %d times for a narrowing that keeps users", svc.askedHeldByAUser)
	}
}

// A WIDENING ASKS NOTHING, and the assertion is on the CALL COUNT rather than
// on the outcome. Both probes would answer "nobody holds it" under this stub,
// so an implementation that queried two tables on every widening would pass an
// outcome assertion while doing two pointless reads per write, forever.
func TestWideningAsksNothingAtAll(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to vos.ClaimAppliesTo
	}{
		{"user to both", vos.ClaimAppliesToUser, vos.ClaimAppliesToBoth},
		{"client to both", vos.ClaimAppliesToClient, vos.ClaimAppliesToBoth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := narrowTo(tc.from, tc.to, &probingClaimService{heldByAUser: true, heldByAClient: true})

			if err != nil {
				t.Fatalf("a widening was refused: %v", err)
			}
			if svc.askedHeldByAUser != 0 || svc.askedHeldByAClient != 0 {
				t.Fatalf("a widening queried the edge tables: user=%d client=%d",
					svc.askedHeldByAUser, svc.askedHeldByAClient)
			}
		})
	}
}

// AppliesTo unchanged is not a narrowing, whatever else the update touches.
func TestAnUpdateThatLeavesAppliesToAloneAsksNothing(t *testing.T) {
	e := storedClaim()
	svc := &probingClaimService{heldByAUser: true, heldByAClient: true}

	_, err := domain.GetUpdatable(e, func(x *Claim) error {
		x.Description = vos.Description("A reworded explanation of the same thing.")
		return nil
	}, svc, "GetUpdatable")

	if err != nil {
		t.Fatalf("an ordinary edit was refused: %v", err)
	}
	if svc.askedHeldByAUser != 0 || svc.askedHeldByAClient != 0 {
		t.Fatalf("an edit that left AppliesTo alone queried the edge tables: user=%d client=%d",
			svc.askedHeldByAUser, svc.askedHeldByAClient)
	}
}

// The rule is scoped to UPDATE. An insert has no old state to narrow from, and
// a definition being created is held by nobody.
func TestTheNarrowingGuardNeverFiresOnAnInsert(t *testing.T) {
	e := validClaim()
	e.AppliesTo = vos.ClaimAppliesToUser
	svc := &probingClaimService{heldByAUser: true, heldByAClient: true}

	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err != nil {
		t.Fatalf("an insert was refused by the narrowing guard: %v", err)
	}
	if svc.askedHeldByAUser != 0 || svc.askedHeldByAClient != 0 {
		t.Fatalf("an insert queried the edge tables: user=%d client=%d",
			svc.askedHeldByAUser, svc.askedHeldByAClient)
	}
}

// ── claims-per-tenant-cap-users / claims-per-tenant-cap-clients ─────────────
//
// The catalog budget: at most 20 ACTIVE definitions per tenant may admit a
// given identity kind. The two buckets OVERLAP — a `both` definition spends a
// slot on each side — so the cases below are written against the arithmetic
// (`user` + `both`, `client` + `both`) and not against the enum's members.
//
// Which side refused matters as much as whether it refused, so these cases
// assert the NOTIFICATION TYPE and not only the blamed field: both rules attach
// to AppliesTo, and a body that always raised the user half would satisfy every
// field assertion while telling an operator to look at the wrong bucket.

// catalogOf builds a stub whose tenant already holds this many ACTIVE
// definitions of each enum member.
func catalogOf(user, client, both int64) *probingClaimService {
	return &probingClaimService{activeByAppliesTo: map[string]int64{
		vos.ClaimAppliesToUser.Value():   user,
		vos.ClaimAppliesToClient.Value(): client,
		vos.ClaimAppliesToBoth.Value():   both,
	}}
}

// claimRaised reports whether the refusal carries a notification of this type.
// The name is matched on the suffix so the package qualifier %T prints does not
// have to be spelled at every call site.
func claimRaised(err error, notification string) bool {
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		return false
	}
	for _, ctx := range carrier.NotificationContexts() {
		for _, msg := range ctx.Messages() {
			if name := fmt.Sprintf("%T", msg.Notification); name == notification ||
				strings.HasSuffix(name, "."+notification) {
				return true
			}
		}
	}
	return false
}

// insertWithAppliesTo creates a definition of one kind into the given catalog.
func insertWithAppliesTo(to vos.ClaimAppliesTo, svc *probingClaimService) (*probingClaimService, error) {
	e := validClaim()
	e.AppliesTo = to
	_, err := domain.GetInsertable(e, svc, "GetInsertable")
	return svc, err
}

// THE BOUNDARY FROM BELOW. Nineteen held plus the row being written is exactly
// twenty, which is allowed — an off-by-one here would cost every tenant a slot
// silently, and no other case in this file would notice.
func TestTheTwentiethUserClaimIsStillAccepted(t *testing.T) {
	_, err := insertWithAppliesTo(vos.ClaimAppliesToUser, catalogOf(19, 0, 0))
	if err != nil {
		t.Fatalf("the 20th user-admitting definition was refused: %v", err)
	}
}

func TestTheTwentyFirstUserClaimIsRefused(t *testing.T) {
	_, err := insertWithAppliesTo(vos.ClaimAppliesToUser, catalogOf(20, 0, 0))
	if err == nil {
		t.Fatal("a tenant created a 21st user-admitting claim definition")
	}
	if !claimBlames(err, "AppliesTo") {
		t.Errorf("the refusal blamed %v, want AppliesTo", claimRejectedFields(err))
	}
	if !claimRaised(err, "TooManyUserClaimsInTenantNotification") {
		t.Error("the refusal did not name the USER bucket as the full one")
	}
}

// THE BUCKETS ARE INDEPENDENT. A full user side must never block a definition
// that admits only clients — the two sides ride different tokens.
func TestAFullUserBucketDoesNotBlockAClientOnlyClaim(t *testing.T) {
	svc, err := insertWithAppliesTo(vos.ClaimAppliesToClient, catalogOf(20, 0, 0))
	if err != nil {
		t.Fatalf("a client-only definition was refused because the USER bucket is full: %v", err)
	}
	// The user member is never counted for a write that admits no user.
	for _, asked := range svc.askedActiveWith {
		if asked == vos.ClaimAppliesToUser.Value() {
			t.Errorf("the user member was counted for a client-only insert: %v", svc.askedActiveWith)
		}
	}
}

// `both` SPENDS A SLOT ON BOTH SIDES, which is the arithmetic the whole rule
// rests on: ten `user` rows plus ten `both` rows is a FULL user bucket, even
// though no member alone reaches twenty.
func TestBothCountsTowardsEachBucket(t *testing.T) {
	_, err := insertWithAppliesTo(vos.ClaimAppliesToUser, catalogOf(10, 5, 10))
	if err == nil {
		t.Fatal("the `both` definitions were not counted towards the user bucket")
	}
	if !claimRaised(err, "TooManyUserClaimsInTenantNotification") {
		t.Error("the refusal did not name the USER bucket as the full one")
	}
}

// An insert of `both` into a tenant whose two buckets are full is TWO problems,
// and the caller reads both in one response. A body that returned after the
// first refusal would leave an operator fixing one bucket only to be refused
// again by the other.
func TestInsertingBothIntoTwoFullBucketsReportsBothSides(t *testing.T) {
	_, err := insertWithAppliesTo(vos.ClaimAppliesToBoth, catalogOf(20, 20, 0))
	if err == nil {
		t.Fatal("a `both` definition was created into two full buckets")
	}
	if !claimRaised(err, "TooManyUserClaimsInTenantNotification") {
		t.Error("the user half of the budget did not report")
	}
	if !claimRaised(err, "TooManyClientClaimsInTenantNotification") {
		t.Error("the client half of the budget did not report")
	}
}

// A `both` insert asks each member exactly ONCE. The `both` count belongs to
// both buckets, so a body that read it per side would pay for four queries
// where three do — invisible to every assertion about the outcome.
func TestInsertingBothAsksEachMemberOnce(t *testing.T) {
	svc, err := insertWithAppliesTo(vos.ClaimAppliesToBoth, catalogOf(0, 0, 0))
	if err != nil {
		t.Fatalf("a valid `both` definition was refused: %v", err)
	}
	seen := map[string]int{}
	for _, asked := range svc.askedActiveWith {
		seen[asked]++
	}
	for member, count := range seen {
		if count != 1 {
			t.Errorf("the %q member was counted %d times, want once: %v", member, count, svc.askedActiveWith)
		}
	}
	if len(svc.askedActiveWith) != 3 {
		t.Errorf("a `both` insert asked %d counts, want 3: %v", len(svc.askedActiveWith), svc.askedActiveWith)
	}
}

// THE WIDENING HOLE. Without this the cap is bypassed in two writes: create a
// `user` definition while the user bucket has room, then widen it to `both`
// into a client bucket that is already full.
func TestWideningIntoAFullBucketIsRefused(t *testing.T) {
	svc := catalogOf(0, 20, 0)
	_, err := narrowTo(vos.ClaimAppliesToUser, vos.ClaimAppliesToBoth, svc)

	if err == nil {
		t.Fatal("a definition was widened into a client bucket that is already full")
	}
	if !claimRaised(err, "TooManyClientClaimsInTenantNotification") {
		t.Error("the refusal did not name the CLIENT bucket as the full one")
	}
}

// The mirror: widening into a bucket with room is the ordinary operational
// move and must keep working.
func TestWideningIntoABucketWithRoomIsAccepted(t *testing.T) {
	_, err := narrowTo(vos.ClaimAppliesToUser, vos.ClaimAppliesToBoth, catalogOf(0, 19, 0))
	if err != nil {
		t.Fatalf("a widening into a bucket with room was refused: %v", err)
	}
}

// A NARROWING ASKS NOTHING. It frees a slot, so a cap that fired on the way
// down would be a bug — and the assertion is on the CALL COUNT, because a
// narrowing that queried would pass every outcome assertion while paying for
// reads that can only ever say "there is room".
func TestNarrowingAsksTheCatalogNothing(t *testing.T) {
	svc, err := narrowTo(vos.ClaimAppliesToBoth, vos.ClaimAppliesToUser, catalogOf(20, 20, 20))
	if err != nil {
		t.Fatalf("a narrowing was refused by the catalog cap: %v", err)
	}
	if len(svc.askedActiveWith) != 0 {
		t.Fatalf("a narrowing counted the catalog: %v", svc.askedActiveWith)
	}
}

// THE CASE THE "ONLY WHAT THE WRITE ADDS" READING EXISTS FOR. A tenant already
// over the line — rows seeded by migration, or written before this rule did —
// must stay repairable: an edit that leaves AppliesTo alone is not consuming a
// slot and must neither be refused nor cost a round trip.
func TestAnEditThatConsumesNoSlotIsAcceptedInAnOverBudgetTenant(t *testing.T) {
	e := storedClaim()
	svc := catalogOf(25, 25, 25)

	_, err := domain.GetUpdatable(e, func(x *Claim) error {
		x.Description = vos.Description("A reworded explanation of the same thing.")
		return nil
	}, svc, "GetUpdatable")

	if err != nil {
		t.Fatalf("an ordinary edit was refused in a tenant that is over the cap: %v", err)
	}
	if len(svc.askedActiveWith) != 0 {
		t.Fatalf("an edit that left AppliesTo alone counted the catalog: %v", svc.askedActiveWith)
	}
}

// THE BARRIER, pinned. The counts bind TenantID into a comparison against a
// UUID column, so an unparseable owner reaching them would be a driver error
// and a 500 rather than the 422 it deserves. It cannot: the generated
// `tenant-is-a-usable-id` guard sits in the IfInsertOrUpdate gate ABOVE this
// rule and ends the pass. That is a property of the ORDER two gates are
// declared in, invisible in the rule itself, which is why it is asserted here.
func TestTheCatalogCapIsNeverAskedWithAnUnusableTenant(t *testing.T) {
	e := validClaim()
	e.TenantID = domain.NewID("not-a-uuid")
	svc := catalogOf(20, 20, 20)

	if _, err := domain.GetInsertable(e, svc, "GetInsertable"); err == nil {
		t.Fatal("a claim was created under an unparseable tenant id")
	}
	if len(svc.askedActiveWith) != 0 {
		t.Fatalf("the catalog was counted under an unparseable tenant id: %v", svc.askedActiveWith)
	}
}
