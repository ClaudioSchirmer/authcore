package vos

import (
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// RoleKey is declared `manual` in the spec, so nothing generated tests it. Its
// rule IS the entity's substance check on the handle, which is why it is
// exercised on its own terms here.
//
// It reuses raised() and containsName() from tenant_manual_vos_test.go: one
// harness, so a change to how notifications are read cannot leave this file
// asserting against a stale one.

// checkRoleKey runs IsValid and reports the outcome plus the notifications the
// caller would read.
func checkRoleKey(t *testing.T, in string) (bool, []string) {
	t.Helper()
	return raised(t, func(ctx *domain.NotificationContext) bool {
		return RoleKey(in).IsValid("Key", ctx)
	})
}

func TestRoleKeyAccepts(t *testing.T) {
	for _, in := range []string{
		"billing-manager",
		"admin",
		// Two runes is the floor, and it is deliberately low: these are ordinary
		// role handles that a three-rune floor would refuse.
		"hr",
		"qa",
		"it",
		"a1",
		"role-2",
		"tenant-read-only",
		// A run of exactly 3 identical runes is allowed; 4 is not.
		"aaabc",
		// Exactly 64 runes — the upper bound itself, matching the permission
		// key's part bound so a handle can never overflow a rendered claim.
		// Three distinct runes, not two: "abab…" is refused however long it is,
		// which is the anti-junk rule doing its job rather than a bound.
		strings.Repeat("abc", 21) + "d",
	} {
		ok, names := checkRoleKey(t, in)
		if !ok {
			t.Errorf("role key %q was rejected: %v", in, names)
		}
	}
}

func TestRoleKeyRefusesMalformed(t *testing.T) {
	for _, tc := range []struct {
		in  string
		why string
	}{
		{"a", "one rune — below the two-rune floor"},
		{strings.Repeat("abc", 21) + "de", "65 runes — one past the bound"},
		{strings.Repeat("ab", 32), "64 runes but only two distinct — long does not mean substantial"},
		{"Billing-Manager", "uppercase is refused, never folded"},
		{"billing manager", "a space is not a separator here"},
		{"billing_manager", "an underscore is not a hyphen"},
		{"-billing", "a leading hyphen"},
		{"billing-", "a trailing hyphen"},
		{"billing--manager", "a doubled hyphen"},
		{"billing:manager", "a colon — that is a permission key, not a role key"},
		{" billing", "padding is refused, never trimmed"},
		{"billing ", "…on either side"},
		{"aaaa", "a run of four identical runes — the held-key signature"},
		{"aaaaaaaa", "…and a longer one"},
		{"111", "three identical digits held down"},
	} {
		ok, names := checkRoleKey(t, tc.in)
		if ok {
			t.Errorf("role key %q was accepted (%s)", tc.in, tc.why)
			continue
		}
		if !containsName(names, "InvalidRoleKeyNotification") {
			t.Errorf("role key %q (%s): expected InvalidRoleKeyNotification, got %v", tc.in, tc.why, names)
		}
	}
}

// An empty value is answered by the value object itself, with the framework's
// own RequiredFieldNotification — which is exactly why the spec declares no
// `required` rule beside this field. Declaring one would make the caller read
// the same complaint twice for one empty value.
func TestRoleKeyEmptyIsTheFrameworksRequiredComplaint(t *testing.T) {
	ok, names := checkRoleKey(t, "")
	if ok {
		t.Fatal("an empty role key was accepted")
	}
	if !containsName(names, "RequiredFieldNotification") {
		t.Errorf("an empty role key should raise RequiredFieldNotification, got %v", names)
	}
	if containsName(names, "InvalidRoleKeyNotification") {
		t.Errorf("an empty role key should not ALSO be reported as malformed, got %v", names)
	}
}

// One malformed value raises the complaint ONCE, however many bounds it breaks.
// The caller reads one actionable message, not a pile of them for one field.
func TestRoleKeyReportsMalformedOnlyOnce(t *testing.T) {
	// Uppercase, padded, over-long AND a run of identical runes: four problems.
	in := " " + strings.Repeat("A", 80) + " "
	ok, names := checkRoleKey(t, in)
	if ok {
		t.Fatal("a thoroughly malformed role key was accepted")
	}
	var n int
	for _, name := range names {
		if name == "InvalidRoleKeyNotification" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected exactly one InvalidRoleKeyNotification, got %d (%v)", n, names)
	}
}

// Value is what the persister binds and the mappers read back, so the round
// trip is pinned: the generated mappers convert with vos.RoleKey(x) and read
// with .Value(), and a change to either half would break both.
func TestRoleKeyValueRoundTrips(t *testing.T) {
	const in = "billing-manager"
	if got := RoleKey(in).Value(); got != in {
		t.Errorf("Value() returned %q, want %q", got, in)
	}
}
