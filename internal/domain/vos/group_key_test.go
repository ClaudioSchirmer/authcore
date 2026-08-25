// Hand-written: GroupKey is declared `kind: manual` in the spec, so the
// generator wrote neither the type nor its test.
//
// The value is immutable once set and is what every API caller, every audit
// line and every directory mapping references, so the cases below lean on the
// two properties that matter: the shape is enforced at both bounds, and NOTHING
// is normalized — a value that does not already comply is refused, never
// quietly repaired.

package vos

import (
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

func groupKeyIsValid(t *testing.T, value string) (bool, *domain.NotificationContext) {
	t.Helper()
	ctx := domain.NewNotificationContext("Group")
	ok := GroupKey(value).IsValid("Key", ctx)
	return ok, ctx
}

func TestGroupKeyAcceptsTheShapesRealGroupsUse(t *testing.T) {
	accepted := []string{
		"hr",                       // the two-rune floor: a real org unit
		"engineering",              // the canonical shape
		"3m-approvers",             // a leading digit — real companies have them
		"a1",                       // alphanumeric, minimal
		"finance-accounts-payable", // several groups
		strings.Repeat("ab", 32),   // exactly 64 runes, the ceiling
	}
	for _, v := range accepted {
		t.Run(v, func(t *testing.T) {
			ok, ctx := groupKeyIsValid(t, v)
			if !ok || ctx.HasErrors() {
				t.Errorf("%q was refused, want accepted", v)
			}
		})
	}
}

func TestGroupKeyRefusesEverythingOutsideItsShape(t *testing.T) {
	refused := map[string]string{
		"a":                            "one rune, below the floor",
		strings.Repeat("ab", 32) + "c": "65 runes, one past the ceiling",
		"Engineering":                  "uppercase is not normalized to lowercase, it is refused",
		"-engineering":                 "a hyphen may not lead",
		"engineering-":                 "a hyphen may not trail",
		"engineering--team":            "a hyphen may not double",
		"engineering_team":             "underscore is not in the alphabet",
		"engineering team":             "a space is not in the alphabet",
		" engineering":                 "leading whitespace is refused, not trimmed",
		"engineering:team":             "a colon belongs to the permission key, not here",
		"aa":                           "one distinct rune — keyboard junk, not a handle",
		"abaaaa":                       "a run of four identical runes",
	}
	for v, why := range refused {
		t.Run(why, func(t *testing.T) {
			ok, ctx := groupKeyIsValid(t, v)
			if ok || !ctx.HasErrors() {
				t.Errorf("%q was accepted (%s)", v, why)
			}
		})
	}
}

// An EMPTY value answers with the framework's own required-field notification,
// not with the shape one. The two are different problems with different fixes,
// and a caller is entitled to be told which one they hit.
func TestGroupKeyReportsAnEmptyValueAsRequiredRatherThanMalformed(t *testing.T) {
	ok, ctx := groupKeyIsValid(t, "")
	if ok {
		t.Fatal("an empty group key was accepted")
	}

	messages := ctx.Messages()
	if len(messages) != 1 {
		t.Fatalf("an empty key raised %d notification(s), want exactly 1", len(messages))
	}
	if _, isRequired := messages[0].Notification.(domain.RequiredFieldNotification); !isRequired {
		t.Errorf("an empty key answered with %T, want RequiredFieldNotification", messages[0].Notification)
	}
}

// A malformed value answers with the VO's OWN notification, and with ONE
// notification. The type matters as much as the count: a group key that
// answered InvalidRoleKeyNotification would tell a caller their ROLE key is
// malformed while they were creating a group — which is the whole reason this
// type exists instead of a reuse of vos.RoleKey.
func TestGroupKeyReportsAMalformedValueOnceAndAsItsOwn(t *testing.T) {
	ok, ctx := groupKeyIsValid(t, "Engineering_Team!")
	if ok {
		t.Fatal("a malformed group key was accepted")
	}

	messages := ctx.Messages()
	if len(messages) != 1 {
		t.Fatalf("a malformed key raised %d notification(s), want exactly 1", len(messages))
	}
	if _, isShape := messages[0].Notification.(InvalidGroupKeyNotification); !isShape {
		t.Errorf("a malformed key answered with %T, want InvalidGroupKeyNotification", messages[0].Notification)
	}
}

// Value is what the mappers read back, so it must return the string as stored
// — no trimming, no case folding, nothing.
func TestGroupKeyValueReturnsTheStoredStringUntouched(t *testing.T) {
	if got := GroupKey("engineering").Value(); got != "engineering" {
		t.Errorf("Value() = %q, want the stored string unchanged", got)
	}
}

// GroupKey deliberately does NOT carry the tenant handle's two extra rules. If
// someone ever "unifies" the two types, this is the test that fails.
func TestGroupKeyHasNoReservedList(t *testing.T) {
	// Every one of these is refused as a TENANT workspace and must be a
	// perfectly ordinary group key.
	for _, v := range []string{"admin", "billing", "support", "security", "api"} {
		if ok, _ := groupKeyIsValid(t, v); !ok {
			t.Errorf("%q was refused — a group key has no reserved list, only the tenant handle does", v)
		}
	}
}

// GroupKey and RoleKey are the same RULE and two different TYPES, which is a
// recorded decision rather than an accident (spec §C-6). This pins the "same
// rule" half: the day someone changes one bound and not the other, the
// duplication has silently become a divergence.
func TestGroupKeyEnforcesTheSameShapeAsRoleKey(t *testing.T) {
	for _, v := range []string{
		"hr", "engineering", "3m-approvers", strings.Repeat("ab", 32),
		"a", "Engineering", "-x", "x-", "x--y", "aa", "abaaaa", "",
	} {
		groupOK, _ := groupKeyIsValid(t, v)
		roleOK := RoleKey(v).IsValid("Key", domain.NewNotificationContext("Role"))
		if groupOK != roleOK {
			t.Errorf("%q: GroupKey says %v, RoleKey says %v — the two handles must enforce one shape", v, groupOK, roleOK)
		}
	}
}
