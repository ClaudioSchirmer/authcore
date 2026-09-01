// Tests for the two-level claim chain.
//
// WHAT THESE ASSERT is the same class of property the handler suite next door
// protects: things that are invisible in a green build and expensive to lose.
//
//	the specialised value BEATS the default, which is the whole chain;
//	a claim with neither level is ABSENT, not empty — the two differ to a consumer;
//	a definition missing from the catalog mints nothing, even when the user holds
//	  a value for it — that is how a retired definition stops minting;
//	a platform name is never taken over by a tenant definition;
//	a must-change-password session carries none of it;
//	the budget spends specialised values before defaults, and truncates the same
//	  way every time.
//
// The resolution is a pure function, so none of this needs a database, a handler
// or a signing key — which is why it is a file of its own.

package commands

import (
	"fmt"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// ── fixtures ────────────────────────────────────────────────────────────────

// definition builds one catalog row. The id is what an entry points at, so it
// is always set: a definition with none can match nothing.
func definition(id, name string, valueType vos.ClaimValueType, def *string) schemas.ClaimDefinition {
	return schemas.ClaimDefinition{
		ID:           domain.NewID(id),
		Name:         name,
		ValueType:    valueType.Value(),
		AppliesTo:    vos.ClaimAppliesToUser.Value(),
		DefaultValue: def,
	}
}

// bundleOf is the resolution's input: the catalog, and no value held by anyone.
func bundleOf(defs ...schemas.ClaimDefinition) infra.SignInBundle {
	return infra.SignInBundle{Definitions: defs, ClaimValues: map[domain.ID]string{}}
}

// holding adds a level-1 value to a bundle — what THIS user set, against the
// definition of the same id.
func holding(b infra.SignInBundle, claimID, value string) infra.SignInBundle {
	b.ClaimValues[domain.NewID(claimID)] = value
	return b
}

func stringValue(v string) *string { return &v }

// holds attaches a level-1 value to the user, pointing at a definition's id.

// claimID builds the nth distinct definition id, so a test needing more rows
// than the budget does not spell out twenty-five UUIDs.
func claimID(n int) string {
	return fmt.Sprintf("33333333-3333-3333-3333-%012d", n)
}

// ── the chain ───────────────────────────────────────────────────────────────

func TestResolveCustomClaims_SpecialisedValueBeatsTheDefault(t *testing.T) {
	account := usableAccount()

	resolved := resolveCustomClaims(account, holding(bundleOf(
		definition(claimID(1), "x_cost_center", vos.ClaimValueTypeNumber, stringValue("1000")),
	), claimID(1), "9000"))

	// THE POINT OF THE WHOLE CHAIN. A default that survived a value set on the
	// principal would make level 1 decorative.
	if got := resolved["x_cost_center"]; got != float64(9000) {
		t.Fatalf("x_cost_center = %#v, want the user's own 9000 and not the tenant default", got)
	}
}

func TestResolveCustomClaims_DefaultFillsInWhenTheUserHoldsNothing(t *testing.T) {
	resolved := resolveCustomClaims(usableAccount(), bundleOf(
		definition(claimID(1), "x_region", vos.ClaimValueTypeString, stringValue("emea")),
	))

	if got := resolved["x_region"]; got != "emea" {
		t.Fatalf("x_region = %#v, want the tenant-wide default", got)
	}
}

func TestResolveCustomClaims_AbsentWhenBothLevelsAreEmpty(t *testing.T) {
	resolved := resolveCustomClaims(usableAccount(), bundleOf(
		definition(claimID(1), "x_plan_tier", vos.ClaimValueTypeString, nil),
	))

	// ABSENT, not empty. A consumer distinguishes "this tenant sets no plan
	// tier" from "the plan tier is the empty string", and minting a zero value
	// to fill the hole would erase that difference.
	if _, present := resolved["x_plan_tier"]; present {
		t.Fatalf("a claim with no value at either level entered the token: %#v", resolved)
	}
}

func TestResolveCustomClaims_ADefinitionOutsideTheCatalogMintsNothing(t *testing.T) {
	account := usableAccount()

	// The catalog is what an ARCHIVED definition drops out of — the reader's
	// active-only scope removes it — while the read join would still hand the
	// entry a name and a type. This is the test that pins that behaviour at the
	// resolution: an entry whose definition is not in the catalog mints nothing.
	resolved := resolveCustomClaims(account, bundleOf(
		definition(claimID(2), "x_region", vos.ClaimValueTypeString, stringValue("emea")),
	))

	if len(resolved) != 1 {
		t.Fatalf("resolved = %#v, want only the definition still in the catalog", resolved)
	}
	if resolved["x_region"] != "emea" {
		t.Errorf("the surviving definition lost its default: %#v", resolved)
	}
}

// ── typing ──────────────────────────────────────────────────────────────────

func TestResolveCustomClaims_TypesEachValueByItsDefinition(t *testing.T) {
	account := usableAccount()

	b := bundleOf(
		definition(claimID(1), "x_cost_center", vos.ClaimValueTypeNumber, nil),
		definition(claimID(2), "x_beta_enabled", vos.ClaimValueTypeBool, nil),
		definition(claimID(3), "x_region", vos.ClaimValueTypeString, nil),
	)
	b = holding(b, claimID(1), "1000")
	b = holding(b, claimID(2), "true")
	b = holding(b, claimID(3), "emea")

	resolved := resolveCustomClaims(account, b)

	// Each assertion is on the Go TYPE as much as the value: a number arriving
	// as "1000" would marshal into the token as a quoted string, which is the
	// exact failure the join carries ClaimValueType up to prevent.
	if got := resolved["x_cost_center"]; got != float64(1000) {
		t.Errorf("x_cost_center = %#v, want the number 1000", got)
	}
	if got := resolved["x_beta_enabled"]; got != true {
		t.Errorf("x_beta_enabled = %#v, want the boolean true", got)
	}
	if got := resolved["x_region"]; got != "emea" {
		t.Errorf("x_region = %#v, want the string", got)
	}
}

func TestResolveCustomClaims_TypesADefaultTheSameWayAsAValue(t *testing.T) {
	// The promise the shared gate exists for: a value the catalog accepted as a
	// default can never be one the emission refuses to mint, and both levels
	// reach the same JSON type.
	resolved := resolveCustomClaims(usableAccount(), bundleOf(
		definition(claimID(1), "x_seat_count", vos.ClaimValueTypeNumber, stringValue("12")),
	))

	if got := resolved["x_seat_count"]; got != float64(12) {
		t.Fatalf("x_seat_count = %#v, want the default typed as a number", got)
	}
}

func TestResolveCustomClaims_OmitsAValueThatDoesNotParse(t *testing.T) {
	account := usableAccount()

	resolved := resolveCustomClaims(account, bundleOf(
		definition(claimID(1), "x_cost_center", vos.ClaimValueTypeNumber, nil),
	))

	// Unreachable through the API — both write levels validate against this same
	// type — and reachable by a migration or a direct UPDATE. OMITTED and never
	// coerced: minting 0 for a value that did not parse is a wrong answer, and
	// no answer is the honest one.
	if _, present := resolved["x_cost_center"]; present {
		t.Fatalf("an unreadable value reached the token: %#v", resolved)
	}
}

func TestResolveCustomClaims_OmitsAnUnknownValueType(t *testing.T) {
	account := usableAccount()

	// A value_type outside the closed set, which no write can produce and a
	// migration can. The domain's shared gate answers TRUE for it, because on
	// the write path the value object raises its own notification straight
	// after — there is no such pass here, so this seat has to decide.
	resolved := resolveCustomClaims(account, bundleOf(
		definition(claimID(1), "x_mystery", vos.ClaimValueType("json"), nil),
	))

	if _, present := resolved["x_mystery"]; present {
		t.Fatalf("a value whose type nobody declared reached the token: %#v", resolved)
	}
}

// ── the guards ──────────────────────────────────────────────────────────────

func TestResolveCustomClaims_SkipsAPlatformClaimName(t *testing.T) {
	account := usableAccount()

	// Unwritable through the API — vos.ClaimName's reserved prefix refuses a
	// bare name — and writable by a migration into the catalog. Two of the nine
	// (`permissions`, `tenant_id`) are read by the framework across the whole
	// mesh, so a definition taking one over would not confuse one consumer, it
	// would change what every service authorizes.
	resolved := resolveCustomClaims(account, bundleOf(
		definition(claimID(1), claimPermissions, vos.ClaimValueTypeString, nil),
	))

	if len(resolved) != 0 {
		t.Fatalf("a platform claim name was resolved from the catalog: %#v", resolved)
	}
}

func TestResolveCustomClaims_MustChangePasswordCarriesNone(t *testing.T) {
	account := usableAccount()
	account.MustChangePassword = true

	resolved := resolveCustomClaims(account, bundleOf(
		definition(claimID(1), "x_region", vos.ClaimValueTypeString, stringValue("apac")),
	))

	// Same reading that already governs the permissions claim: a session that
	// exists to rotate an expired credential carries nothing but the one thing
	// it is for.
	if len(resolved) != 0 {
		t.Fatalf("a restricted session carried tenant claims: %#v", resolved)
	}
	// And non-nil, so the body renders {} rather than null.
	if resolved == nil {
		t.Fatal("the restricted session got a nil map, which would render as null")
	}
}

// A nil ACCOUNT is still worth tolerating: it is what a caller hands in when the
// lookup answered absence, and the honest answer is an empty non-nil map rather
// than a panic on the sign-in path.
//
// The nil DEFINITION half of this test is gone with the pointer it needed. The
// catalog now arrives as a slice of rows scanned from `claims`, and a scanned row
// is a value — there is no nil to tolerate and no way to construct one.
func TestResolveCustomClaims_ToleratesANilAccount(t *testing.T) {
	if got := resolveCustomClaims(nil, bundleOf()); len(got) != 0 || got == nil {
		t.Fatalf("resolveCustomClaims(nil, …) = %#v, want an empty non-nil map", got)
	}
}

// ── the budget ──────────────────────────────────────────────────────────────

func TestResolveCustomClaims_BudgetSpendsSpecialisedValuesBeforeDefaults(t *testing.T) {
	account := usableAccount()
	catalog := make([]schemas.ClaimDefinition, 0, customClaimBudget+5)
	values := map[domain.ID]string{}

	// Five values set on this user, under names that sort LAST — so a plain
	// alphabetical truncation would drop exactly them.
	for i := range 5 {
		id := claimID(100 + i)
		values[domain.NewID(id)] = fmt.Sprintf("mine-%d", i)
		catalog = append(catalog, definition(id, fmt.Sprintf("x_zz_%02d", i), vos.ClaimValueTypeString, nil))
	}
	// Twenty definitions carrying a default, under names that sort first.
	for i := range customClaimBudget {
		catalog = append(catalog,
			definition(claimID(200+i), fmt.Sprintf("x_aa_%02d", i), vos.ClaimValueTypeString, stringValue("fallback")))
	}

	resolved := resolveCustomClaims(account, infra.SignInBundle{Definitions: catalog, ClaimValues: values})

	if len(resolved) != customClaimBudget {
		t.Fatalf("resolved %d claims, want the budget of %d", len(resolved), customClaimBudget)
	}
	// EVERY value somebody set on this user survived. A default crowding one out
	// would mean an operator's deliberate act losing to a tenant-wide fallback.
	for i := range 5 {
		name := fmt.Sprintf("x_zz_%02d", i)
		if got := resolved[name]; got != fmt.Sprintf("mine-%d", i) {
			t.Errorf("%s = %#v, want the value set on the user to survive the budget", name, got)
		}
	}
	// Which leaves room for fifteen of the twenty defaults.
	if got := len(resolved) - 5; got != customClaimBudget-5 {
		t.Errorf("defaults kept = %d, want %d", got, customClaimBudget-5)
	}
}

func TestResolveCustomClaims_BudgetTruncationIsReproducible(t *testing.T) {
	account := usableAccount()
	catalog := make([]schemas.ClaimDefinition, 0, customClaimBudget+10)
	for i := range customClaimBudget + 10 {
		catalog = append(catalog,
			definition(claimID(300+i), fmt.Sprintf("x_key_%02d", i), vos.ClaimValueTypeString, stringValue("v")))
	}

	first := resolveCustomClaims(account, bundleOf(catalog...))
	second := resolveCustomClaims(account, bundleOf(catalog...))

	// The same rows must always lose the same claims, or "why did x_key_25
	// disappear" has no answer. Sorting inside each bucket is what buys it —
	// ranging a map to pick survivors would not.
	if len(first) != customClaimBudget {
		t.Fatalf("resolved %d claims, want the budget of %d", len(first), customClaimBudget)
	}
	for name, value := range first {
		if second[name] != value {
			t.Fatalf("truncation is not reproducible: %s survived one run and not the other", name)
		}
	}
	// And it keeps the alphabetically first ones, which is what makes the set
	// predictable rather than merely stable within a process.
	if _, present := first["x_key_00"]; !present {
		t.Error("the first name by sort order was dropped")
	}
	if _, present := first["x_key_29"]; present {
		t.Error("a name past the budget survived")
	}
}

// The case that USED to live here — a definition carrying no id, which matched no
// entry and fell through to its default — is gone with the shape that allowed it.
// A definition now arrives as a row read from `claims`, and a row has an id; the
// only way to hold one without was an aggregate that had never been persisted, and
// nothing on this path constructs one.
