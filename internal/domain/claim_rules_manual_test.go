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

	askedTenant     int
	askedTenantWith []domain.ID
}

func (s *probingClaimService) ClaimNameTaken(_ domain.ID, _ string, _ domain.ID) bool {
	return s.nameTaken
}

func (s *probingClaimService) TenantIsUnavailable(tenantID domain.ID) bool {
	s.askedTenant++
	s.askedTenantWith = append(s.askedTenantWith, tenantID)
	return s.tenantUnavailable
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
