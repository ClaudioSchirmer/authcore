// Tests for PermissionKey, the hand-written composite value object.
//
// It is hand-written because the type is: the spec declared it as a composite
// with `written: manual`, so the generator wrote no file and no test for it.
// This is the only place its rules are proven.
//
// The harness — notified, typeName — comes from tenant_manual_vos_test.go in
// this same package, so a change to how a verdict is read lands in one place.

package vos

import (
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// validate is the one call every case below makes. The field name handed in is
// deliberately NOT "Resource" or "Action": a composite reports on its parts, so
// the name the entity would pass must never leak into an answer.
func validate(t *testing.T, resource, action string) (bool, []string) {
	t.Helper()
	return notified(t, func(c *domain.NotificationContext) bool {
		return PermissionKey{Resource: resource, Action: action}.IsValid("Key", c)
	})
}

// ── the shapes the catalog accepts ───────────────────────────────────────────

func TestPermissionKeyAcceptsTheShapesRoutesEnforce(t *testing.T) {
	for _, tc := range []struct {
		resource, action, why string
	}{
		{"tenant", "read", "the ordinary single-segment pair"},
		{"permission", "insert", "another literal this service already gates"},
		{"user", "rotate-key", "a hyphenated action — the vocabulary is open, not a CRUD enum"},
		{"user:profile", "read", "a colon-joined resource PATH, which the framework's own example gates"},
		{"a1:b2:c3", "read", "three segments, digits included"},
		{"3d-assets", "read", "a segment may LEAD with a digit"},
		{"tenant", "*", "resource:* — one of the three shapes the claim matcher honours"},
		{"*", "*", "*:* — the super-admin row"},
		{strings.Repeat("ab", 32), "read", "a resource of exactly 64 runes, the cap"},
		{"tenant", strings.Repeat("ab", 32), "an action of exactly 64 runes, the cap"},
	} {
		ok, keys := validate(t, tc.resource, tc.action)
		if !ok {
			t.Errorf("%q:%q was refused (%s), raising %v", tc.resource, tc.action, tc.why, keys)
		}
	}
}

// ── presence ─────────────────────────────────────────────────────────────────

// An empty part is answered with the FRAMEWORK's own required notification,
// not with the shape one. They are different problems and a caller fixes them
// differently: one value is missing, the other is malformed.
func TestPermissionKeyRefusesEmptyPartsAsRequired(t *testing.T) {
	for _, tc := range []struct{ resource, action, want string }{
		{"", "read", "RequiredFieldNotification"},
		{"tenant", "", "RequiredFieldNotification"},
	} {
		ok, keys := validate(t, tc.resource, tc.action)
		if ok {
			t.Fatalf("%q:%q was accepted with an empty part", tc.resource, tc.action)
		}
		if len(keys) == 0 || keys[0] != tc.want {
			t.Errorf("%q:%q raised %v, want %s", tc.resource, tc.action, keys, tc.want)
		}
	}
}

// Both halves are checked before either verdict is returned, so a caller
// sending two malformed parts is told about both in ONE answer rather than one
// per round-trip.
func TestPermissionKeyReportsBothMalformedPartsAtOnce(t *testing.T) {
	ok, keys := validate(t, "Tenant", "READ")
	if ok {
		t.Fatal("a pair with two malformed halves was accepted")
	}
	if len(keys) != 2 {
		t.Fatalf("raised %v, want one answer per malformed half", keys)
	}
	if keys[0] != "InvalidResourceNameNotification" || keys[1] != "InvalidActionNameNotification" {
		t.Errorf("raised %v, want the resource answer then the action one", keys)
	}
}

// ── the resource ─────────────────────────────────────────────────────────────

func TestPermissionKeyRefusesMalformedResources(t *testing.T) {
	cases := map[string]string{
		"Tenant":                         "uppercase, and never normalized to lowercase",
		" tenant":                        "leading whitespace, refused rather than trimmed",
		"tenant ":                        "trailing whitespace",
		"ten ant":                        "an internal space is not a slug",
		"tenant_read":                    "an underscore is outside the slug alphabet",
		"-tenant":                        "a leading hyphen",
		"tenant-":                        "a trailing hyphen",
		"ten--ant":                       "a doubled hyphen",
		":tenant":                        "a leading colon, which makes an empty first segment",
		"tenant:":                        "a trailing colon, which makes an empty last segment",
		"user::profile":                  "a doubled colon, which makes an empty middle segment",
		"a":                              "one rune, below the two-rune segment floor",
		"user:a":                         "a one-rune segment inside an otherwise valid path",
		"aaaa":                           "a run of four identical runes",
		"ten*":                           "a wildcard mixed into a slug",
		"user:*":                         "a wildcard as a SEGMENT — not one of the three shapes the matcher honours",
		"*:*":                            "the pair wildcard is not a resource",
		strings.Repeat("ab", 33):         "66 runes, over the 64-rune cap",
		strings.Repeat("ab:", 21) + "cd": "over the cap even though every segment is legal",
	}
	for in, why := range cases {
		ok, keys := validate(t, in, "read")
		if ok {
			t.Errorf("resource %q was accepted (%s)", in, why)
			continue
		}
		if len(keys) == 0 || keys[0] != "InvalidResourceNameNotification" {
			t.Errorf("resource %q (%s) raised %v, want InvalidResourceNameNotification", in, why, keys)
		}
	}
}

// ── the action ───────────────────────────────────────────────────────────────

// The action is EXACTLY one segment. That is what keeps the rendering
// unambiguous: the last colon segment is always the action, so
// `user:profile:read` parses back exactly one way.
func TestPermissionKeyRefusesMalformedActions(t *testing.T) {
	cases := map[string]string{
		"READ":                   "uppercase",
		"read write":             "a space",
		"profile:read":           "a COLON — the action is one segment, never a path",
		"-read":                  "a leading hyphen",
		"read-":                  "a trailing hyphen",
		"re--ad":                 "a doubled hyphen",
		"r":                      "one rune, below the segment floor",
		"aaaa":                   "a run of four identical runes",
		"re*ad":                  "a wildcard mixed into a slug",
		strings.Repeat("ab", 33): "66 runes, over the cap",
	}
	for in, why := range cases {
		ok, keys := validate(t, "tenant", in)
		if ok {
			t.Errorf("action %q was accepted (%s)", in, why)
			continue
		}
		if len(keys) == 0 || keys[0] != "InvalidActionNameNotification" {
			t.Errorf("action %q (%s) raised %v, want InvalidActionNameNotification", in, why, keys)
		}
	}
}

// ── the pair ─────────────────────────────────────────────────────────────────

// The rule that made this concept a composite rather than two loose fields: it
// is only expressible with both values in hand.
//
// `*:read` would match no route ever — the claim matcher honours exactly
// exact, `resource:*` and `*:*` — while the catalog row reads like a sweeping
// grant. The row is refused instead of shipping the illusion.
func TestPermissionKeyRefusesAWildcardResourceWithAConcreteAction(t *testing.T) {
	for _, action := range []string{"read", "insert", "rotate-key"} {
		ok, keys := validate(t, "*", action)
		if ok {
			t.Errorf("*:%s was accepted — it would match no route while reading like a grant", action)
			continue
		}
		if len(keys) != 1 || keys[0] != "UnmatchablePermissionKeyNotification" {
			t.Errorf("*:%s raised %v, want a single UnmatchablePermissionKeyNotification", action, keys)
		}
	}
}

// The pair rule runs only AFTER both parts pass, so a malformed half is
// reported as itself rather than as an unmatchable pair — the caller is told
// the problem they can act on.
func TestPermissionKeyPairRuleWaitsForBothParts(t *testing.T) {
	ok, keys := validate(t, "*", "READ")
	if ok {
		t.Fatal("*:READ was accepted")
	}
	for _, k := range keys {
		if k == "UnmatchablePermissionKeyNotification" {
			t.Errorf("raised %v — the malformed action must be reported as itself, not as an unmatchable pair", keys)
		}
	}
}

// ── the rendering ────────────────────────────────────────────────────────────

// String is the single home of the separator: the read side's derivation, the
// write responses and the future token issuer all render the pair through it.
func TestPermissionKeyRendersTheTokenString(t *testing.T) {
	for _, tc := range []struct{ resource, action, want string }{
		{"tenant", "read", "tenant:read"},
		{"user:profile", "read", "user:profile:read"},
		{"tenant", "*", "tenant:*"},
		{"*", "*", "*:*"},
	} {
		if got := (PermissionKey{Resource: tc.resource, Action: tc.action}).String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

// The rendered form cannot exceed the claim budget the two part bounds are
// FOR. This is the invariant the withdrawn rule 7 stated: it holds
// STRUCTURALLY, because each part is capped independently and the pair rules
// run only after both pass — which is exactly why checking it would have been
// a branch nothing could take.
func TestPermissionKeyRenderingStaysWithinTheClaimBudget(t *testing.T) {
	resource, action := strings.Repeat("ab", 32), strings.Repeat("cd", 32)
	if ok, keys := validate(t, resource, action); !ok {
		t.Fatalf("the two longest legal parts were refused, raising %v", keys)
	}
	if got := len(PermissionKey{Resource: resource, Action: action}.String()); got != maxRenderedRunes {
		t.Errorf("the longest legal rendering is %d runes, the declared bound says %d", got, maxRenderedRunes)
	}
}
