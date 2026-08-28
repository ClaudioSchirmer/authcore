// Hand-written: ClaimName is declared `kind: manual` in the spec, so the
// generator wrote neither the type nor its test.
//
// Two decisions taken at the model gate live in this type, and this file is
// where they are pinned rather than described:
//
//  1. THE PREFIX IS CALLER-OWNED. `x_` is typed by the caller and stored
//     verbatim. TestClaimNameReturnsTheValueUntouched is the case that fails
//     the day somebody adds a "convenience" prepend or trim.
//  2. THE DOUBLE PREFIX IS A REFUSAL, NOT A REPAIR — the paste error a
//     caller-owned prefix makes possible, and the one case shape alone lets
//     through.

package vos

import (
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

func claimNameIsValid(t *testing.T, value string) (bool, *domain.NotificationContext) {
	t.Helper()
	ctx := domain.NewNotificationContext("Claim")
	ok := ClaimName(value).IsValid("Name", ctx)
	return ok, ctx
}

// The prefix has ONE spelling in this service, and every stored name carries
// it from the first insert. A change here is a change to data already written,
// so it is pinned as a literal rather than read off the constant it tests.
func TestClaimNameReservedPrefixIsPinned(t *testing.T) {
	if ClaimNameReservedPrefix != "x_" {
		t.Fatalf("the reserved prefix is %q, want %q — every name already stored carries the old one",
			ClaimNameReservedPrefix, "x_")
	}
}

func TestClaimNameAcceptsTheShapesRealClaimsUse(t *testing.T) {
	accepted := []string{
		"x_hr",                          // the four-rune floor: prefix plus a real handle
		"x_cost_center",                 // the canonical shape
		"x_region",                      // one group
		"x_erp_id",                      // several groups
		"x_2fa_enabled",                 // a leading digit inside a group — a real claim name
		"x_" + strings.Repeat("ab", 31), // exactly 64 runes, the ceiling
		"x_plan_tier_2026",              // digits in a trailing group
	}
	for _, v := range accepted {
		t.Run(v, func(t *testing.T) {
			ok, ctx := claimNameIsValid(t, v)
			if !ok || ctx.HasErrors() {
				t.Errorf("%q was refused, want accepted", v)
			}
		})
	}
}

func TestClaimNameRefusesEverythingOutsideItsShape(t *testing.T) {
	refused := map[string]string{
		"cost_center":                         "no reserved prefix — the caller owns it, so its absence is the caller's error",
		"ext_cost_center":                     "a different prefix is not this service's namespace",
		"x_x_cost_center":                     "the double prefix: well-formed snake_case that shape alone would accept",
		"X_cost_center":                       "an uppercase prefix is refused, never folded to lowercase",
		"x_Cost_Center":                       "uppercase in the remainder is refused, not normalized",
		"x_":                                  "the prefix alone: there is no name left",
		"x_a":                                 "three runes, below the floor",
		"x_" + strings.Repeat("ab", 31) + "c": "65 runes, one past the ceiling",
		"x__cost_center":                      "an underscore may not double",
		"x_cost_center_":                      "an underscore may not trail",
		"x_cost-center":                       "a hyphen belongs to the role handle, not here",
		"x_cost center":                       "a space is not in the alphabet",
		" x_cost_center":                      "leading whitespace is refused, not trimmed",
		"x_cost_center ":                      "trailing whitespace is refused, not trimmed",
		"x_123":                               "digits only: an id, not a claim name",
		"x_aa":                                "one distinct rune in the remainder — keyboard junk",
		"x_abaaaa":                            "a run of four identical runes",
		"x_cost:center":                       "a colon belongs to the permission key",
	}
	for v, why := range refused {
		t.Run(why, func(t *testing.T) {
			ok, ctx := claimNameIsValid(t, v)
			if ok || !ctx.HasErrors() {
				t.Errorf("%q was accepted (%s)", v, why)
			}
		})
	}
}

// The anti-junk floor is applied to the REMAINDER, never to the whole value.
//
// Counting the prefix would add two guaranteed distinct runes to every name and
// weaken the floor for exactly the values it exists to catch: `x_aa` has three
// distinct runes as a whole and one where it matters.
func TestClaimNameAntiJunkIgnoresThePrefix(t *testing.T) {
	if ok, _ := claimNameIsValid(t, "x_aa"); ok {
		t.Error(`"x_aa" was accepted — the distinct-rune floor is counting the prefix`)
	}
}

// NOTHING IS NORMALIZED, and this is the test that says so in the direction
// that matters: a value the type ACCEPTS comes back byte for byte.
//
// A prepend, a trim or a case fold anywhere in this type would still let the
// value pass IsValid and would change what the column stores and what a token
// carries — silently, and only visibly to a consumer weeks later.
func TestClaimNameReturnsTheValueUntouched(t *testing.T) {
	const typed = "x_cost_center"
	if got := ClaimName(typed).Value(); got != typed {
		t.Fatalf("Value() = %q, want %q — something in this type is normalizing", got, typed)
	}
}

// An EMPTY value answers with the framework's own required-field notification,
// not with the shape one. They are different problems with different fixes, and
// it is why the aggregate declares no `required` rule on top of this type —
// doing so would tell the caller the same thing twice.
func TestClaimNameAnswersEmptyWithTheRequiredNotification(t *testing.T) {
	ok, ctx := claimNameIsValid(t, "")
	if ok {
		t.Fatal("an empty claim name was accepted")
	}
	messages := ctx.Messages()
	if len(messages) != 1 {
		t.Fatalf("an empty value raised %d notifications, want exactly 1", len(messages))
	}
	if _, isRequired := messages[0].Notification.(domain.RequiredFieldNotification); !isRequired {
		t.Errorf("an empty value answered %T, want domain.RequiredFieldNotification", messages[0].Notification)
	}
}

// However many shape rules a value breaks, the caller reads ONE sentence.
//
// Emitting the same notification per broken rule would show the identical text
// four times for one bad value.
func TestClaimNameReportsOneProblemHoweverManyRulesBreak(t *testing.T) {
	// No prefix, uppercase, a hyphen, whitespace and past the ceiling at once.
	ok, ctx := claimNameIsValid(t, " Cost-Center "+strings.Repeat("z", 70))
	if ok {
		t.Fatal("a value breaking every shape rule was accepted")
	}
	if got := len(ctx.Messages()); got != 1 {
		t.Errorf("raised %d notifications, want exactly 1", got)
	}
}
