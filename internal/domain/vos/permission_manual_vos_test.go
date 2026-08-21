package vos

import (
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// PermissionKey is declared `written: manual` in the spec — its shape is
// generated, its file and therefore its whole rule are hand-written, so nothing
// generated tests it. Its rule IS this entity's substance: what a permission
// may look like, and which of the two halves a caller got wrong.
//
// `raised` and `containsName` are the helpers the Tenant value-object tests
// already define in this package; reusing them keeps one definition of "which
// notification did the caller read".

func key(resource, action string) PermissionKey {
	return PermissionKey{Resource: resource, Action: action}
}

// validate runs IsValid under the entity's own field name, which is what the
// framework passes: the two whole-key complaints land on it, while each part's
// complaint lands on the part's name.
func validate(t *testing.T, v PermissionKey) (bool, []string) {
	t.Helper()
	return raised(t, func(ctx *domain.NotificationContext) bool {
		return v.IsValid("Key", ctx)
	})
}

func TestPermissionKeyAccepts(t *testing.T) {
	for _, tc := range []struct {
		resource string
		action   string
		why      string
	}{
		{"tenant", "read", "the ordinary case — one segment each"},
		{"tenant", "insert", "the taxonomy this service already grants"},
		{"permission", "archive", "…and its own"},
		{"user:profile", "read", "a hierarchical resource, which the framework's own docs gate"},
		{"user:profile:photo", "read", "a deeper path — the LAST colon segment is always the action"},
		{"rotate-key", "issue", "hyphenated slugs on both halves"},
		{"api-key", "rotate-key", "a hyphen is legal inside a segment, never at an edge"},
		{"tenant", "*", "a wildcard action: every action on tenants"},
		{"*", "*", "the super-admin row — one row, total power"},
		{"a1", "b2", "two runes is the minimum segment, digits allowed"},
		{"aaa", "bbb", "a run of exactly 3 identical runes is allowed; 4 is not"},
		{strings.Repeat("a1", 32), "read", "exactly 64 runes — the upper bound itself"},
		{"tenant", strings.Repeat("b2", 32), "the same bound on the action"},
	} {
		ok, names := validate(t, key(tc.resource, tc.action))
		if !ok {
			t.Errorf("PermissionKey{%q, %q} was rejected (%s): %v", tc.resource, tc.action, tc.why, names)
		}
	}
}

func TestPermissionKeyRefusesResource(t *testing.T) {
	for _, tc := range []struct {
		resource string
		want     string
		why      string
	}{
		{"", "RequiredFieldNotification", "empty — the composite answers presence, since a plain string part cannot"},
		{"Tenant", "InvalidResourceNameNotification", "uppercase is refused, never folded: the value is compared byte-for-byte against a token claim"},
		{" tenant ", "InvalidResourceNameNotification", "untrimmed is refused, never repaired"},
		{"tenant_read", "InvalidResourceNameNotification", "underscore is not in the slug alphabet"},
		{"-tenant", "InvalidResourceNameNotification", "a hyphen may never lead"},
		{"tenant-", "InvalidResourceNameNotification", "…nor trail"},
		{"ten--ant", "InvalidResourceNameNotification", "…nor double"},
		{"t", "InvalidResourceNameNotification", "one rune is below the two-rune segment minimum"},
		{"user:", "InvalidResourceNameNotification", "an empty trailing segment"},
		{":read", "InvalidResourceNameNotification", "an empty leading segment"},
		{"user::profile", "InvalidResourceNameNotification", "an empty interior segment"},
		{"aaaa", "InvalidResourceNameNotification", "a run of 4 identical runes is the held-key signature"},
		{"ten*", "InvalidResourceNameNotification", "a partial wildcard fits none of the three shapes the claim matcher honors"},
		{"user:*", "InvalidResourceNameNotification", "…and a wildcard segment inside a path is the same problem"},
		{strings.Repeat("a1", 32) + "b", "InvalidResourceNameNotification", "65 runes — one past the column"},
	} {
		ok, names := validate(t, key(tc.resource, "read"))
		if ok {
			t.Errorf("PermissionKey resource %q was accepted (%s)", tc.resource, tc.why)
			continue
		}
		if !containsName(names, tc.want) {
			t.Errorf("resource %q (%s): want %s, got %v", tc.resource, tc.why, tc.want, names)
		}
	}
}

func TestPermissionKeyRefusesAction(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   string
		why    string
	}{
		{"", "RequiredFieldNotification", "empty"},
		{"Read", "InvalidActionNameNotification", "uppercase"},
		{"read:write", "InvalidActionNameNotification", "an action is EXACTLY one segment — a colon there would make the rendering ambiguous"},
		{"r", "InvalidActionNameNotification", "below the two-rune minimum"},
		{"read_all", "InvalidActionNameNotification", "underscore"},
		{"-read", "InvalidActionNameNotification", "leading hyphen"},
		{"rrrr", "InvalidActionNameNotification", "a run of 4 identical runes"},
		{"re*d", "InvalidActionNameNotification", "a partial wildcard"},
		{strings.Repeat("b2", 32) + "c", "InvalidActionNameNotification", "65 runes"},
	} {
		ok, names := validate(t, key("tenant", tc.action))
		if ok {
			t.Errorf("PermissionKey action %q was accepted (%s)", tc.action, tc.why)
			continue
		}
		if !containsName(names, tc.want) {
			t.Errorf("action %q (%s): want %s, got %v", tc.action, tc.why, tc.want, names)
		}
	}
}

// A caller who got BOTH halves wrong must read both complaints, not the first.
func TestPermissionKeyReportsBothHalves(t *testing.T) {
	ok, names := validate(t, key("Tenant", "Read"))
	if ok {
		t.Fatal("a key with two malformed halves was accepted")
	}
	if !containsName(names, "InvalidResourceNameNotification") ||
		!containsName(names, "InvalidActionNameNotification") {
		t.Errorf("want both halves reported, got %v", names)
	}

	ok, names = validate(t, key("", ""))
	if ok {
		t.Fatal("an empty key was accepted")
	}
	if len(names) != 2 {
		t.Errorf("want one complaint per empty part, got %v", names)
	}
}

// Rule 6 — the invariant that made this concept a composite: it cannot be
// stated about either half alone.
func TestPermissionKeyRefusesUnmatchableWildcard(t *testing.T) {
	for _, action := range []string{"read", "insert", "rotate-key"} {
		ok, names := validate(t, key("*", action))
		if ok {
			t.Errorf("*:%s was accepted — the claim matcher honors only exact, resource:* and *:*", action)
			continue
		}
		if !containsName(names, "UnmatchablePermissionKeyNotification") {
			t.Errorf("*:%s: want UnmatchablePermissionKeyNotification, got %v", action, names)
		}
	}

	// The mirror case is legal: a specific resource with a wildcard action is
	// exactly the "resource:*" shape the matcher does honor.
	if ok, names := validate(t, key("tenant", "*")); !ok {
		t.Errorf("tenant:* was rejected: %v", names)
	}
}

// A malformed part reports its OWN problem and stops there — the pair-level
// rules are only meaningful once both halves are well-formed, so the caller is
// never handed a second, derived complaint about a value they already know is
// wrong.
func TestPermissionKeyDoesNotPileOnDerivedComplaints(t *testing.T) {
	ok, names := validate(t, key("*", "Read"))
	if ok {
		t.Fatal("*:Read was accepted")
	}
	if containsName(names, "UnmatchablePermissionKeyNotification") {
		t.Errorf("the malformed action should be the only complaint, got %v", names)
	}
	if !containsName(names, "InvalidActionNameNotification") {
		t.Errorf("want InvalidActionNameNotification, got %v", names)
	}
}

func TestPermissionKeyString(t *testing.T) {
	for _, tc := range []struct {
		key  PermissionKey
		want string
	}{
		{key("tenant", "read"), "tenant:read"},
		{key("user:profile", "read"), "user:profile:read"},
		{key("*", "*"), "*:*"},
		{key("tenant", "*"), "tenant:*"},
	} {
		if got := tc.key.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

// The rendered form is what a JWT claim carries, so the round trip a token
// issuer performs must land back on the same two halves. This is what makes
// the single-segment action rule load-bearing rather than cosmetic: the LAST
// colon segment is always the action.
func TestPermissionKeyRendersUnambiguously(t *testing.T) {
	for _, v := range []PermissionKey{
		key("tenant", "read"),
		key("user:profile", "read"),
		key("user:profile:photo", "insert"),
	} {
		rendered := v.String()
		i := strings.LastIndex(rendered, ":")
		if rendered[:i] != v.Resource || rendered[i+1:] != v.Action {
			t.Errorf("%q does not split back to {%q, %q}", rendered, v.Resource, v.Action)
		}
	}
}

// The separator lives in exactly ONE place. If a second join ever appears, the
// two definitions can disagree and a permission that reads right stops matching.
func TestPermissionKeyIsTheOnlyPlaceTheColonLives(t *testing.T) {
	if got := key("tenant", "read").String(); got != "tenant"+":"+"read" {
		t.Fatalf("String() no longer renders resource:action: %q", got)
	}
}
