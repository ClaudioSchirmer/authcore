// Tests for the two value objects this entity brings that the generator did not
// write: PersonName (a composite, `written: manual`) and Password (`kind:
// manual`). Their rules are compositions of predicates rather than shapes, which
// is exactly why no generated test stands behind them.
//
// The naming and the collector below follow tenant_manual_vos_test.go, so the
// three files read as one suite.

package vos

import (
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// This file declares no collector of its own: `notified` and `typeName` already
// live in tenant_manual_vos_test.go, in this package, and they return the
// notification KEYS rather than a count — which is what lets a case assert WHICH
// answer a caller receives instead of merely that something failed.

// ── PersonName ─────────────────────────────────────────────────────────────

func TestPersonNameAcceptsRealNames(t *testing.T) {
	// Every one of these is a name a person actually has, and every one of them
	// would be refused by at least one rule that looks reasonable in isolation.
	cases := map[string]PersonName{
		"ordinary":                 {Given: "Maria", Family: "Souza Lima"},
		"two-rune family name":     {Given: "Wei", Family: "Ng"},
		"apostrophe":               {Given: "Sean", Family: "O'Brien"},
		"hyphenated":               {Given: "Anne-Marie", Family: "Ng-Chen"},
		"accented":                 {Given: "José", Family: "d'Ávila"},
		"non-latin script":         {Given: "أحمد", Family: "الحسن"},
		"cyrillic":                 {Given: "Ирина", Family: "Петрова"},
		"repeated but not a lean":  {Given: "Aaron", Family: "Aalto"},
		"single-rune given name":   {Given: "K", Family: "Silva"},
		"long but under the bound": {Given: "Maria", Family: strings.Repeat("ab", 37)},
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if ok, keys := notified(t, func(c *domain.NotificationContext) bool {
				return value.IsValid("Name", c)
			}); !ok {
				t.Fatalf("a real name was refused with %v: %+v", keys, value)
			}
		})
	}
}

func TestPersonNameRefusesJunk(t *testing.T) {
	cases := map[string]PersonName{
		"empty given":        {Given: "", Family: "Souza"},
		"empty family":       {Given: "Maria", Family: ""},
		"digits only":        {Given: "12345", Family: "Souza"},
		"keyboard lean":      {Given: "aaaa", Family: "Souza"},
		"leading space":      {Given: " Maria", Family: "Souza"},
		"trailing space":     {Given: "Maria ", Family: "Souza"},
		"double space":       {Given: "Maria  Clara", Family: "Souza"},
		"over the rune cap":  {Given: strings.Repeat("a", 76), Family: "Souza"},
		"punctuation only":   {Given: "!!!", Family: "Souza"},
		"whitespace only":    {Given: "   ", Family: "Souza"},
		"both halves broken": {Given: "", Family: ""},
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if ok, _ := notified(t, func(c *domain.NotificationContext) bool {
				return value.IsValid("Name", c)
			}); ok {
				t.Fatalf("junk was accepted as a name: %+v", value)
			}
		})
	}
}

// TestPersonNameReportsBothHalvesAtOnce is the reason IsValid does not
// short-circuit.
//
// A caller who got both halves wrong should be told both, in one answer, rather
// than fixing one and meeting the other on the next round trip.
func TestPersonNameReportsBothHalvesAtOnce(t *testing.T) {
	value := PersonName{Given: "aaaa", Family: "12345"}

	_, keys := notified(t, func(c *domain.NotificationContext) bool {
		return value.IsValid("Name", c)
	})
	if len(keys) != 2 {
		t.Fatalf("expected one notification per broken half, got %v", keys)
	}
}

// TestPersonNameReportsOncePerHalf is the other half of that contract: one bad
// value gets ONE sentence, however many of the shape rules it broke.
func TestPersonNameReportsOncePerHalf(t *testing.T) {
	// Too long, no letter, and a keyboard lean, all at once.
	value := PersonName{Given: strings.Repeat("1", 90), Family: "Souza"}

	_, keys := notified(t, func(c *domain.NotificationContext) bool {
		return value.IsValid("Name", c)
	})
	if len(keys) != 1 || keys[0] != "InvalidPersonNameNotification" {
		t.Fatalf("one bad half should answer once with InvalidPersonNameNotification, got %v", keys)
	}
}

// TestPersonNameFullName pins the rendering, which is the whole reason the
// method lives on the value object rather than in the read model.
func TestPersonNameFullName(t *testing.T) {
	cases := map[string]struct {
		value PersonName
		want  string
	}{
		"both halves":  {PersonName{Given: "Maria", Family: "Souza Lima"}, "Maria Souza Lima"},
		"given only":   {PersonName{Given: "Maria"}, "Maria"},
		"family only":  {PersonName{Family: "Souza"}, "Souza"},
		"neither half": {PersonName{}, ""},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// The half-filled cases are not hypothetical: a read that projected
			// only one of the two sources arrives exactly like this, and a naive
			// join would answer with a leading or trailing space.
			if got := c.value.FullName(); got != c.want {
				t.Fatalf("FullName() = %q, want %q", got, c.want)
			}
		})
	}
}

// ── Password ───────────────────────────────────────────────────────────────

func TestPasswordAcceptsPolicyCompliant(t *testing.T) {
	cases := map[string]Password{
		"at the floor":       "Ab1!efgh",
		"passphrase":         "Correct Horse Battery Staple 9!",
		"unicode classes":    "Çãö1!abcd",          // lower/upper are Unicode, not ASCII
		"symbol is math":     "Abcdefg1+",          // '+' is a Symbol, not Punct
		"internal spaces":    "A quiet room 4 me!", // a passphrase is a sentence
		"at the ceiling":     Password("Ab1!" + strings.Repeat("x", 124)),
		"non-latin with all": "Añ1!авгд",
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if ok, keys := notified(t, func(c *domain.NotificationContext) bool {
				return value.IsValid("Password", c)
			}); !ok {
				t.Fatalf("a compliant password was refused with %v: %q", keys, value)
			}
		})
	}
}

func TestPasswordRefusesWhatThePolicySays(t *testing.T) {
	cases := map[string]Password{
		"empty":               "",
		"too short":           "Ab1!efg",
		"over the ceiling":    Password("Ab1!" + strings.Repeat("x", 125)),
		"no lowercase":        "AB1!EFGH",
		"no uppercase":        "ab1!efgh",
		"no digit":            "Abcd!efgh",
		"no symbol":           "Abcd1efgh",
		"leading whitespace":  " Ab1!efgh",
		"trailing whitespace": "Ab1!efgh ",
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if ok, _ := notified(t, func(c *domain.NotificationContext) bool {
				return value.IsValid("Password", c)
			}); ok {
				t.Fatalf("a password the policy forbids was accepted: %q", value)
			}
		})
	}
}

// TestPasswordNeverEchoesTheValue is a SECURITY assertion, not a style one.
//
// Every other value object in this package passes the rejected input to
// AddNotification so a caller can see what was refused. This one must not: the
// notification payload reaches the 422 body and any log that renders one, and
// the value is a credential the caller is in the middle of choosing.
func TestPasswordNeverEchoesTheValue(t *testing.T) {
	const secret = "short1!A" // fails the length rule

	ctx := domain.NewNotificationContext("test")
	Password(secret).IsValid("Password", ctx)

	for _, m := range ctx.Messages() {
		// FieldValue is the echo slot — what every other value object in this
		// package fills, and what reaches the 422 body.
		if strings.Contains(m.FieldValue, secret) {
			t.Fatalf("the plaintext reached the notification payload: %q", m.FieldValue)
		}
	}
}

// TestPasswordReportsOnce is the counterpart of the name test: a password that
// breaks four clauses is told the policy once, not four times.
func TestPasswordReportsOnce(t *testing.T) {
	_, keys := notified(t, func(c *domain.NotificationContext) bool {
		return Password("abc").IsValid("Password", c) // short, no upper, no digit, no symbol
	})
	if len(keys) != 1 || keys[0] != "WeakPasswordNotification" {
		t.Fatalf("a password breaking four clauses should answer once with WeakPasswordNotification, got %v", keys)
	}
}
