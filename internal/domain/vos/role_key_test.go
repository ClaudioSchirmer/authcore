// Hand-written: RoleKey is declared `kind: manual` in the spec, so the
// generator wrote neither the type nor its test.
//
// The value is immutable once set and is what every API caller and every audit
// line references, so the cases below lean on the two properties that matter:
// the shape is enforced at both bounds, and NOTHING is normalized — a value
// that does not already comply is refused, never quietly repaired.

package vos

import (
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

func roleKeyIsValid(t *testing.T, value string) (bool, *domain.NotificationContext) {
	t.Helper()
	ctx := domain.NewNotificationContext("Role")
	ok := RoleKey(value).IsValid("Key", ctx)
	return ok, ctx
}

func TestRoleKeyAcceptsTheShapesRealRolesUse(t *testing.T) {
	accepted := []string{
		"hr",                       // the two-rune floor: a real handle
		"billing-manager",          // the canonical shape
		"3m-approvers",             // a leading digit — real companies have them
		"a1",                       // alphanumeric, minimal
		"finance-accounts-payable", // several groups
		strings.Repeat("ab", 32),   // exactly 64 runes, the ceiling
	}
	for _, v := range accepted {
		t.Run(v, func(t *testing.T) {
			ok, ctx := roleKeyIsValid(t, v)
			if !ok || ctx.HasErrors() {
				t.Errorf("%q was refused, want accepted", v)
			}
		})
	}
}

func TestRoleKeyRefusesEverythingOutsideItsShape(t *testing.T) {
	refused := map[string]string{
		"a":                            "one rune, below the floor",
		strings.Repeat("ab", 32) + "c": "65 runes, one past the ceiling",
		"Billing-Manager":              "uppercase is not normalized to lowercase, it is refused",
		"-billing":                     "a hyphen may not lead",
		"billing-":                     "a hyphen may not trail",
		"billing--manager":             "a hyphen may not double",
		"billing_manager":              "underscore is not in the alphabet",
		"billing manager":              "a space is not in the alphabet",
		" billing":                     "leading whitespace is refused, not trimmed",
		"billing:manager":              "a colon belongs to the permission key, not here",
		"aa":                           "one distinct rune — keyboard junk, not a handle",
		"abaaaa":                       "a run of four identical runes",
	}
	for v, why := range refused {
		t.Run(why, func(t *testing.T) {
			ok, ctx := roleKeyIsValid(t, v)
			if ok || !ctx.HasErrors() {
				t.Errorf("%q was accepted (%s)", v, why)
			}
		})
	}
}

// An EMPTY value answers with the framework's own required-field notification,
// not with the shape one. The two are different problems with different fixes,
// and a caller is entitled to be told which one they hit.
func TestRoleKeyReportsAnEmptyValueAsRequiredRatherThanMalformed(t *testing.T) {
	ok, ctx := roleKeyIsValid(t, "")
	if ok {
		t.Fatal("an empty role key was accepted")
	}

	messages := ctx.Messages()
	if len(messages) != 1 {
		t.Fatalf("an empty key raised %d notification(s), want exactly 1", len(messages))
	}
	if _, isRequired := messages[0].Notification.(domain.RequiredFieldNotification); !isRequired {
		t.Errorf("an empty key answered with %T, want RequiredFieldNotification", messages[0].Notification)
	}
}

// A malformed value answers with the VO's own notification, and with ONE
// notification — this type has a single way to be wrong, unlike the tenant
// handle, which can be malformed and reserved at the same time.
func TestRoleKeyReportsAMalformedValueOnce(t *testing.T) {
	ok, ctx := roleKeyIsValid(t, "Billing_Manager!")
	if ok {
		t.Fatal("a malformed role key was accepted")
	}

	messages := ctx.Messages()
	if len(messages) != 1 {
		t.Fatalf("a malformed key raised %d notification(s), want exactly 1", len(messages))
	}
	if _, isShape := messages[0].Notification.(InvalidRoleKeyNotification); !isShape {
		t.Errorf("a malformed key answered with %T, want InvalidRoleKeyNotification", messages[0].Notification)
	}
}

// Value is what the mappers read back, so it must return the string as stored
// — no trimming, no case folding, nothing.
func TestRoleKeyValueReturnsTheStoredStringUntouched(t *testing.T) {
	if got := RoleKey("billing-manager").Value(); got != "billing-manager" {
		t.Errorf("Value() = %q, want the stored string unchanged", got)
	}
}

// RoleKey deliberately does NOT carry the tenant handle's two extra rules. If
// someone ever "unifies" the two types, this is the test that fails.
func TestRoleKeyHasNoReservedList(t *testing.T) {
	// Every one of these is refused as a TENANT workspace and must be a
	// perfectly ordinary role key.
	for _, v := range []string{"admin", "billing", "support", "security", "api"} {
		if ok, _ := roleKeyIsValid(t, v); !ok {
			t.Errorf("%q was refused — a role key has no reserved list, only the tenant handle does", v)
		}
	}
}
