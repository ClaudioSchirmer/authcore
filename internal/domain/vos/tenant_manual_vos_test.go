package vos

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The three text value objects are declared `manual` in the spec, so nothing
// generated tests them. Their rule IS the entity's substance check, which is
// why each one is exercised on its own terms here.

// raised reports which notifications a value object emitted for a value, by
// type name, so a test can assert WHICH complaint the caller reads rather than
// only that something failed.
func raised(t *testing.T, run func(ctx *domain.NotificationContext) bool) (bool, []string) {
	t.Helper()
	ctx := domain.NewNotificationContext("test")
	ok := run(ctx)
	var names []string
	for _, msg := range ctx.Messages() {
		// A notification is identified by its Go type name — the same name the
		// translation catalogs key on.
		names = append(names, reflect.TypeOf(msg.Notification).Name())
	}
	return ok, names
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

func TestDisplayNameAccepts(t *testing.T) {
	for _, in := range []string{
		"Acme Comércio e Serviços Ltda",
		"3M", // two runes, two distinct — min(3, len) is what makes this legal
		"GE", // the same, and a real company
		"IBM",
		"日本電気株式会社",                // no Latin letter at all
		"aaabc",                   // a run of exactly 3 is allowed; 4 would not be
		strings.Repeat("abc", 40), // exactly 120 runes — the upper bound itself
	} {
		ok, names := raised(t, func(ctx *domain.NotificationContext) bool {
			return DisplayName(in).IsValid("Name", ctx)
		})
		if !ok {
			t.Errorf("DisplayName(%q) was rejected: %v", in, names)
		}
	}
}

func TestDisplayNameRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "RequiredFieldNotification"},
		{"one rune", "A", "InvalidDisplayNameNotification"},
		{"121 runes", strings.Repeat("abc", 40) + "d", "InvalidDisplayNameNotification"},
		{"no letter", "123 456", "InvalidDisplayNameNotification"},
		{"one distinct rune", "aa", "InvalidDisplayNameNotification"},
		{"held key", "aaaa", "InvalidDisplayNameNotification"},
		{"leading space", " Acme Corp", "InvalidDisplayNameNotification"},
		{"trailing space", "Acme Corp ", "InvalidDisplayNameNotification"},
		{"double space", "Acme  Corp", "InvalidDisplayNameNotification"},
	} {
		ok, names := raised(t, func(ctx *domain.NotificationContext) bool {
			return DisplayName(tc.in).IsValid("Name", ctx)
		})
		if ok {
			t.Errorf("%s: DisplayName(%q) was accepted", tc.name, tc.in)
			continue
		}
		if !containsName(names, tc.want) {
			t.Errorf("%s: DisplayName(%q) raised %v, want %s", tc.name, tc.in, names, tc.want)
		}
	}
}

func TestDescriptionAccepts(t *testing.T) {
	for _, in := range []string{
		"Retail operations of the Acme group in Brazil.",
		"Operações de varejo do grupo Acme no Brasil.",
		"アクメ グループ の 小売事業", // no Latin vowel — the non-Latin clause carries it
		"Описание торговых операций группы.",
		strings.Repeat("ab cd ", 2) + "efghi", // 17 runes, two words, five distinct
		strings.Repeat("abcd ", 99) + "abcde", // exactly 500 runes — the upper bound itself
	} {
		ok, names := raised(t, func(ctx *domain.NotificationContext) bool {
			return Description(in).IsValid("Description", ctx)
		})
		if !ok {
			t.Errorf("Description(%q) was rejected: %v", in, names)
		}
	}
}

func TestDescriptionRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "RequiredFieldNotification"},
		{"14 runes", "Retail opera.", "InvalidDescriptionNotification"},
		{"501 runes", strings.Repeat("abcd ", 100) + "e", "InvalidDescriptionNotification"},
		{"one word", "Retailoperations", "InvalidDescriptionNotification"},
		{"too few distinct", "abab abab abab ab", "InvalidDescriptionNotification"},
		{"held key", "Retail aaaa operations", "InvalidDescriptionNotification"},
		{"latin masher, no vowel", "zzt xxr fftg hhnv", "InvalidDescriptionNotification"},
	} {
		ok, names := raised(t, func(ctx *domain.NotificationContext) bool {
			return Description(tc.in).IsValid("Description", ctx)
		})
		if ok {
			t.Errorf("%s: Description(%q) was accepted", tc.name, tc.in)
			continue
		}
		if !containsName(names, tc.want) {
			t.Errorf("%s: Description(%q) raised %v, want %s", tc.name, tc.in, names, tc.want)
		}
	}
}

func TestTenantWorkspaceAccepts(t *testing.T) {
	for _, in := range []string{
		"acme-comercio",
		"3m9",       // a leading DIGIT is legal — RFC 1123 relaxed RFC 1035's leading-letter rule
		"3m-brasil", // the same, in the shape a real handle takes
		"acme",
		"a1b",
		strings.Repeat("ab", 31) + "c", // exactly 63 runes
	} {
		ok, names := raised(t, func(ctx *domain.NotificationContext) bool {
			return TenantWorkspace(in).IsValid("Workspace", ctx)
		})
		if !ok {
			t.Errorf("TenantWorkspace(%q) was rejected: %v", in, names)
		}
	}
}

func TestTenantWorkspaceRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "RequiredFieldNotification"},
		{"two runes", "ab", "InvalidTenantWorkspaceNotification"},
		{"64 runes", strings.Repeat("ab", 32), "InvalidTenantWorkspaceNotification"},
		{"uppercase is refused, never lowercased", "ACME-CORP", "InvalidTenantWorkspaceNotification"},
		{"padded is refused, never trimmed", " acme ", "InvalidTenantWorkspaceNotification"},
		{"leading hyphen", "-acme", "InvalidTenantWorkspaceNotification"},
		{"trailing hyphen", "acme-", "InvalidTenantWorkspaceNotification"},
		{"doubled hyphen", "acme--corp", "InvalidTenantWorkspaceNotification"},
		{"underscore", "acme_corp", "InvalidTenantWorkspaceNotification"},
		{"accented", "comércio", "InvalidTenantWorkspaceNotification"},
		{"two distinct runes", "aba", "InvalidTenantWorkspaceNotification"},
		{"held key", "aaaab", "InvalidTenantWorkspaceNotification"},
	} {
		ok, names := raised(t, func(ctx *domain.NotificationContext) bool {
			return TenantWorkspace(tc.in).IsValid("Workspace", ctx)
		})
		if ok {
			t.Errorf("%s: TenantWorkspace(%q) was accepted", tc.name, tc.in)
			continue
		}
		if !containsName(names, tc.want) {
			t.Errorf("%s: TenantWorkspace(%q) raised %v, want %s", tc.name, tc.in, names, tc.want)
		}
	}
}

// The reserved list is checked on its own notification: "reserved" and
// "malformed" are different problems, and a caller reading the wrong one edits
// the wrong thing.
func TestTenantWorkspaceRefusesReservedHandlesSeparately(t *testing.T) {
	// The five that are not hypothetical — they are routes this service serves
	// today — plus the collection segment and a phishing-prone one.
	for _, in := range []string{"docs", "graphql", "livez", "readyz", "openapi", "tenants", "admin", "www"} {
		ok, names := raised(t, func(ctx *domain.NotificationContext) bool {
			return TenantWorkspace(in).IsValid("Workspace", ctx)
		})
		if ok {
			t.Errorf("the reserved handle %q was accepted", in)
			continue
		}
		if !containsName(names, "ReservedTenantWorkspaceNotification") {
			t.Errorf("%q raised %v, want ReservedTenantWorkspaceNotification", in, names)
		}
	}
}

// The derivation is the reason this value object is entity-specific, and its
// three properties are the ones the whole model rests on.
func TestDeriveTenantIDIsPureStableAndCollisionFree(t *testing.T) {
	const handle = "acme-comercio"

	first := TenantWorkspace(handle).DeriveTenantID()
	if first.IsEmpty() {
		t.Fatal("the derivation produced an empty id")
	}

	// Pure: the same handle always reaches the same value, which is what lets
	// another service resolve it offline with no call back to authcore.
	if second := TenantWorkspace(handle).DeriveTenantID(); first != second {
		t.Errorf("the derivation is not pure: %s then %s", first.Value(), second.Value())
	}

	// Stable: this exact value is a published contract. It is pinned rather
	// than recomputed, so that a change to the namespace constant — which
	// would invalidate every issued token and every foreign key — fails here
	// instead of in production.
	// Computed independently of the code under test (python: uuid.uuid5(ns, handle)),
	// so this pin is an oracle rather than a photograph of the implementation.
	const pinned = "427a1df1-e2a0-53a9-8f53-fe46edad93f1"
	if first.Value() != pinned {
		t.Errorf("the derived tenant id moved: got %s, want %s.\n"+
			"If TenantIDNamespace changed, this is the alarm: every issued token and every "+
			"foreign key derived under the old namespace is now orphaned.", first.Value(), pinned)
	}

	// A different handle is a different tenant — and that value is pinned too.
	other := TenantWorkspace("acme-servicos").DeriveTenantID()
	if other == first {
		t.Error("two different handles derived the same public key")
	}
	if want := "60396fb0-98fb-5371-af43-49c64a9382e8"; other.Value() != want {
		t.Errorf("acme-servicos derived %s, want %s", other.Value(), want)
	}

	// It is a real UUID, which is what the native UUID column requires.
	if _, err := first.UUID(); err != nil {
		t.Errorf("the derived id is not a parseable UUID: %v", err)
	}
}

func TestTenantIDNamespaceIsTheDeclaredConstant(t *testing.T) {
	const want = "e2937874-80cb-4b5f-b113-21741931ac1a"
	if TenantIDNamespace != want {
		t.Errorf("TenantIDNamespace = %q, want %q — this constant must never change", TenantIDNamespace, want)
	}
}

func TestManualValueObjectsReadBackTheirUnderlying(t *testing.T) {
	if got := DisplayName("Acme").Value(); got != "Acme" {
		t.Errorf("DisplayName.Value() = %q", got)
	}
	if got := Description("some prose").Value(); got != "some prose" {
		t.Errorf("Description.Value() = %q", got)
	}
	if got := TenantWorkspace("acme").Value(); got != "acme" {
		t.Errorf("TenantWorkspace.Value() = %q", got)
	}
}

// A KNOWN LIMIT of the description's two-word rule, pinned here on purpose so
// that changing it is a deliberate act and not an accident.
//
// "At least two words" counts runs of two or more letters separated by
// something that is not a letter. Japanese, Chinese and Thai do not put spaces
// between words, so a perfectly good description in one of them reads as ONE
// word and is refused as junk — the same failure mode the vowel rule was
// written to avoid, in a different predicate.
//
// It is shipped as the approved model specifies, and the practical reach is
// small: all seven catalogs this service serves are space-separated Latin
// scripts, so this only bites an operator describing a tenant IN a
// scriptio-continua language. The fix, if it is ever wanted, is to count a run
// of non-spacing-script letters as satisfying the rule — a change to
// countWords and to this test, and to nothing else.
func TestDescriptionWordRuleRefusesScriptioContinua(t *testing.T) {
	const japaneseWithoutSpaces = "アクメグループの小売事業です。"

	ok, names := raised(t, func(ctx *domain.NotificationContext) bool {
		return Description(japaneseWithoutSpaces).IsValid("Description", ctx)
	})
	if ok {
		t.Fatal("the two-word rule started accepting unspaced text — if that was intended, " +
			"update this test and the note above it; it is not a free change")
	}
	if !containsName(names, "InvalidDescriptionNotification") {
		t.Errorf("raised %v, want InvalidDescriptionNotification", names)
	}
	// The same text WITH separators is accepted, which is what shows the rule
	// is about spacing and not about the script.
	if okSpaced, _ := raised(t, func(ctx *domain.NotificationContext) bool {
		return Description("アクメ グループ の 小売事業").IsValid("Description", ctx)
	}); !okSpaced {
		t.Error("the same Japanese text with separators was refused — the rule is rejecting the script, not the spacing")
	}
}
