// Tests for the three hand-written value objects.
//
// They are hand-written because the types are: the spec declared each as
// `kind: manual`, so the generator wrote no file and no test for them. This is
// the only place their rules are proven.

package vos

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// notified runs a value object's IsValid and reports both the verdict and the
// notification keys raised, so a test can assert WHICH answer a caller gets and
// not merely that something failed.
func notified(t *testing.T, run func(ctx *domain.NotificationContext) bool) (bool, []string) {
	t.Helper()
	ctx := domain.NewNotificationContext("test")
	ok := run(ctx)
	var keys []string
	for _, m := range ctx.Messages() {
		keys = append(keys, strings.TrimPrefix(typeName(m.Notification), "*"))
	}
	return ok, keys
}

// typeName is the notification's Go type name, which IS its translation key —
// so asserting on it is asserting on the answer the caller actually receives.
func typeName(n domain.Notification) string {
	if n == nil {
		return "<nil>"
	}
	t := reflect.TypeOf(n)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// ── DisplayName ──────────────────────────────────────────────────────────────

func TestDisplayNameAcceptsRealCompanyNames(t *testing.T) {
	for _, in := range []string{
		"Acme Comércio e Serviços Ltda", // accented, multi-word
		"3M",                            // two runes, two distinct — the min(3, length) case
		"GE",
		"日本語テナント", // non-Latin
	} {
		ok, keys := notified(t, func(c *domain.NotificationContext) bool {
			return DisplayName(in).IsValid("Name", c)
		})
		if !ok {
			t.Errorf("DisplayName(%q) was refused, raising %v", in, keys)
		}
	}
}

func TestDisplayNameRefusesJunkAndPadding(t *testing.T) {
	cases := map[string]string{
		"a":                      "shorter than the two-rune floor",
		"aa":                     "only one distinct rune",
		"aaaa":                   "a run of four identical runes",
		"1234":                   "no letter at all",
		" Acme":                  "leading whitespace, never repaired",
		"Acme ":                  "trailing whitespace",
		"Acme  Ltda":             "an internal double space",
		strings.Repeat("a", 121): "past the 120-rune ceiling",
	}
	for in, why := range cases {
		ok, keys := notified(t, func(c *domain.NotificationContext) bool {
			return DisplayName(in).IsValid("Name", c)
		})
		if ok {
			t.Errorf("DisplayName(%q) was accepted — %s", in, why)
			continue
		}
		if len(keys) != 1 || keys[0] != "InvalidDisplayNameNotification" {
			t.Errorf("DisplayName(%q) raised %v, want exactly one InvalidDisplayNameNotification", in, keys)
		}
	}
}

// The ceiling counts runes, so an accented name at exactly the bound fits. A
// byte-based ceiling would refuse this value.
func TestDisplayNameCeilingCountsRunesNotBytes(t *testing.T) {
	// Varied on purpose: a single repeated rune would trip the run rule instead,
	// and prove nothing about the ceiling.
	in := strings.Repeat("ábé", 40)
	if len(in) <= 120 {
		t.Fatal("the fixture is not multi-byte; it cannot prove the bound")
	}
	ok, keys := notified(t, func(c *domain.NotificationContext) bool {
		return DisplayName(in).IsValid("Name", c)
	})
	if !ok {
		t.Errorf("120 accented runes were refused as too long, raising %v", keys)
	}
}

// An empty value is the framework's own RequiredFieldNotification, which is
// exactly why the aggregate declares no `required` rule on top of this type —
// declaring one would show the caller the same complaint twice.
func TestDisplayNameEmptyIsRequiredNotInvalid(t *testing.T) {
	ok, keys := notified(t, func(c *domain.NotificationContext) bool {
		return DisplayName("").IsValid("Name", c)
	})
	if ok {
		t.Fatal("the empty display name was accepted")
	}
	if len(keys) != 1 || keys[0] != "RequiredFieldNotification" {
		t.Errorf("empty DisplayName raised %v, want exactly one RequiredFieldNotification", keys)
	}
}

// ── Description ──────────────────────────────────────────────────────────────

func TestDescriptionAcceptsRealProse(t *testing.T) {
	for _, in := range []string{
		"Retail operations of the Acme group in Brazil.",
		"Operações de varejo do grupo Acme no Brasil.",
		"Операции группы Acme в Бразилии сегодня.",
	} {
		ok, keys := notified(t, func(c *domain.NotificationContext) bool {
			return Description(in).IsValid("Description", c)
		})
		if !ok {
			t.Errorf("Description(%q) was refused, raising %v", in, keys)
		}
	}
}

func TestDescriptionRefusesJunk(t *testing.T) {
	cases := map[string]string{
		"Too short":                "under the 15-rune floor",
		"zzzz zzzz zzzz zzzz":      "a run of four identical runes",
		"zbc zbc zbc zbcx":         "no vowel — a Latin-script masher",
		"Retailoperations":         "a single word",
		strings.Repeat("ab ", 200): "past the 500-rune ceiling",
	}
	for in, why := range cases {
		ok, keys := notified(t, func(c *domain.NotificationContext) bool {
			return Description(in).IsValid("Description", c)
		})
		if ok {
			t.Errorf("Description(%q) was accepted — %s", in, why)
			continue
		}
		if len(keys) != 1 || keys[0] != "InvalidDescriptionNotification" {
			t.Errorf("Description(%q) raised %v, want exactly one InvalidDescriptionNotification", in, keys)
		}
	}
}

func TestDescriptionEmptyIsRequiredNotInvalid(t *testing.T) {
	ok, keys := notified(t, func(c *domain.NotificationContext) bool {
		return Description("").IsValid("Description", c)
	})
	if ok {
		t.Fatal("the empty description was accepted")
	}
	if len(keys) != 1 || keys[0] != "RequiredFieldNotification" {
		t.Errorf("empty Description raised %v, want exactly one RequiredFieldNotification", keys)
	}
}

// The vowel clause must never become a language filter: a description with no
// Latin vowel in it at all is still prose when it is written in another script.
func TestDescriptionAcceptsNonLatinScripts(t *testing.T) {
	// Cyrillic: two words, no Latin vowel, comfortably past the rune floor.
	const cyrillic = "Розничные операции группы в Бразилии"
	ok, keys := notified(t, func(c *domain.NotificationContext) bool {
		return Description(cyrillic).IsValid("Description", c)
	})
	if !ok {
		t.Errorf("a Cyrillic description was refused as keyboard junk, raising %v", keys)
	}
}

// THE LIMIT, PINNED ON PURPOSE (spec.md, "One limit of §7 itself, shipped as
// approved). "At least two words" counts runs of letters separated by
// non-letters, so a language that does not space its words reads as ONE word
// and is refused — the exact failure the vowel clause exists to prevent, in a
// different predicate.
//
// It is accepted because all seven catalogs this service serves are
// space-separated Latin scripts, so it only bites an operator describing a
// tenant IN such a language. This test NAMES the limit so that changing it
// later is a deliberate act rather than an accident.
func TestDescriptionWordRuleRefusesScriptioContinua(t *testing.T) {
	// Japanese: long enough, vowel-bearing by the non-Latin rule, and refused
	// solely because it carries no word separator.
	const japanese = "アクメグループのブラジルにおける小売事業です"
	if runeLen(japanese) < descriptionMinRunes {
		t.Fatal("the fixture is too short; it would fail on length, not on the word rule")
	}
	if !hasVowel(japanese) {
		t.Fatal("the fixture fails the vowel rule too; it cannot isolate the word rule")
	}
	if countWords(japanese) >= descriptionMinWords {
		t.Fatal("the fixture is no longer scriptio-continua under countWords")
	}
	ok, _ := notified(t, func(c *domain.NotificationContext) bool {
		return Description(japanese).IsValid("Description", c)
	})
	if ok {
		t.Error("the scriptio-continua limit is gone — if that was deliberate, update this test and spec.md together")
	}
}

// ── TenantWorkspace ──────────────────────────────────────────────────────────

func TestTenantWorkspaceAcceptsDNSLabels(t *testing.T) {
	for _, in := range []string{
		"acme-comercio",
		"3m9",                     // MAY lead with a digit — RFC 1123 relaxed RFC 1035
		"3m-brasil",               // and a digit-led group before a hyphen
		"abc",                     // exactly the three-rune floor
		strings.Repeat("abc", 21), // exactly 63, the DNS ceiling
	} {
		ok, keys := notified(t, func(c *domain.NotificationContext) bool {
			return TenantWorkspace(in).IsValid("Workspace", c)
		})
		if !ok {
			t.Errorf("TenantWorkspace(%q) was refused, raising %v", in, keys)
		}
	}
}

func TestTenantWorkspaceRefusesMalformedHandles(t *testing.T) {
	cases := map[string]string{
		"ab":                    "under the three-rune floor",
		"ACME-CORP":             "uppercase, refused rather than lowercased",
		" acme":                 "padding, refused rather than trimmed",
		"-acme":                 "a leading hyphen",
		"acme-":                 "a trailing hyphen",
		"acme--corp":            "a doubled hyphen",
		"acme_corp":             "an underscore is not a DNS label character",
		"aaaa":                  "a run of four identical runes",
		"aba":                   "only two distinct runes",
		strings.Repeat("a", 64): "past the 63-rune DNS ceiling",
	}
	for in, why := range cases {
		ok, keys := notified(t, func(c *domain.NotificationContext) bool {
			return TenantWorkspace(in).IsValid("Workspace", c)
		})
		if ok {
			t.Errorf("TenantWorkspace(%q) was accepted — %s", in, why)
			continue
		}
		if len(keys) != 1 || keys[0] != "InvalidTenantWorkspaceNotification" {
			t.Errorf("TenantWorkspace(%q) raised %v, want exactly one InvalidTenantWorkspaceNotification", in, keys)
		}
	}
}

// A reserved handle is WELL FORMED, so it must raise its own answer and not the
// shape one — a caller is entitled to know which of the two problems they hit.
func TestTenantWorkspaceRefusesReservedHandlesWithTheirOwnAnswer(t *testing.T) {
	// These are WELL FORMED, so the reservation is the only thing wrong with
	// them and it must be the only answer raised.
	for _, in := range []string{"admin", "graphql", "openapi", "livez", "readyz", "tenants"} {
		ok, keys := notified(t, func(c *domain.NotificationContext) bool {
			return TenantWorkspace(in).IsValid("Workspace", c)
		})
		if ok {
			t.Errorf("the reserved handle %q was accepted", in)
			continue
		}
		if len(keys) != 1 || keys[0] != "ReservedTenantWorkspaceNotification" {
			t.Errorf("TenantWorkspace(%q) raised %v, want exactly one ReservedTenantWorkspaceNotification", in, keys)
		}
	}

	// Some reserved words are ALSO malformed — "www" is one rune repeated, "id"
	// is under the length floor, "app" and "sso" have two distinct runes. The
	// two rules are independent, so such a handle legitimately raises both
	// answers; what matters is that the reservation is always among them.
	for _, in := range []string{"www", "id", "app", "sso"} {
		ok, keys := notified(t, func(c *domain.NotificationContext) bool {
			return TenantWorkspace(in).IsValid("Workspace", c)
		})
		if ok {
			t.Errorf("the reserved handle %q was accepted", in)
			continue
		}
		found := false
		for _, k := range keys {
			if k == "ReservedTenantWorkspaceNotification" {
				found = true
			}
		}
		if !found {
			t.Errorf("TenantWorkspace(%q) raised %v, without the reservation answer", in, keys)
		}
	}
}

// The routes this service actually serves must be in the list — they are the
// half of it that is not a judgement call.
func TestTenantWorkspaceReservesTheRoutesThisServiceServes(t *testing.T) {
	for _, in := range []string{"docs", "graphql", "openapi", "livez", "readyz", "tenant", "tenants"} {
		if !TenantWorkspace(in).IsReserved() {
			t.Errorf("%q is a route this service serves and is not reserved", in)
		}
	}
	if TenantWorkspace("acme-comercio").IsReserved() {
		t.Error("an ordinary handle was reported as reserved")
	}
}

func TestTenantWorkspaceEmptyIsRequiredNotInvalid(t *testing.T) {
	ok, keys := notified(t, func(c *domain.NotificationContext) bool {
		return TenantWorkspace("").IsValid("Workspace", c)
	})
	if ok {
		t.Fatal("the empty workspace was accepted")
	}
	if len(keys) != 1 || keys[0] != "RequiredFieldNotification" {
		t.Errorf("empty TenantWorkspace raised %v, want exactly one RequiredFieldNotification", keys)
	}
}
