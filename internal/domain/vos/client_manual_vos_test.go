// Tests for cidr_block.go — the value object declared `kind: manual`, which is
// exactly why no generated test stands behind it.
//
// It has THREE refusals and they are three different messages with three
// different fixes, so each is asserted on the notification's own type name and
// not merely on "the write failed". A test that only checked for failure would
// pass while the caller was told the wrong thing.

package vos

import (
	"testing"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// cidrVerdict runs the value object and reports the notification key it raised,
// or "" when it accepted the value.
//
// The type NAME is what is asserted, because that name IS the translation key —
// so the assertion is about the answer the caller receives rather than about a
// message somebody could reword.
func cidrVerdict(t *testing.T, raw string) (bool, string) {
	t.Helper()
	ctx := domain.NewNotificationContext("test")
	ok := CIDRBlock(raw).IsValid("CIDR", ctx)
	if ok {
		return true, ""
	}
	msgs := ctx.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected exactly one notification for %q, got %d", raw, len(msgs))
	}
	return false, typeName(msgs[0].Notification)
}

func TestCIDRBlockAcceptsCanonicalRanges(t *testing.T) {
	for _, raw := range []string{
		"203.0.113.0/24",    // an ordinary IPv4 network
		"203.0.113.5/32",    // ONE host — the exact-address case, expressed as a range
		"10.0.0.0/8",        // a large private range, deliberately accepted: see the VO header on why there is no floor
		"2001:db8::/32",     // IPv6
		"2001:db8::1/128",   // one IPv6 host
		"::1/128",           // loopback, which a developer bench legitimately allows
		"198.51.100.128/25", // a non-octet boundary, canonical
	} {
		if ok, key := cidrVerdict(t, raw); !ok {
			t.Errorf("%q was refused with %s; it is a valid canonical range", raw, key)
		}
	}
}

// TestCIDRBlockRefusesWhatDoesNotParse — the first of the three answers.
func TestCIDRBlockRefusesWhatDoesNotParse(t *testing.T) {
	for _, raw := range []string{
		"203.0.113.0",    // an address with no prefix at all
		"203.0.113.0/33", // out of range for IPv4
		"2001:db8::/129", // out of range for IPv6
		"not-an-address", // text
		"203.0.113.0/",   // a prefix marker with nothing behind it
		"203.0.113.0/-1", // negative
	} {
		ok, key := cidrVerdict(t, raw)
		if ok {
			t.Errorf("%q was accepted; it is not a network range", raw)
			continue
		}
		if key != "InvalidCIDRBlockNotification" {
			t.Errorf("%q answered %s; expected InvalidCIDRBlockNotification", raw, key)
		}
	}
}

// TestCIDRBlockRefusesHostBitsAndNamesTheFix is the answer a caller is most
// likely to meet, and the one whose PAYLOAD matters: the message is only useful
// if it carries the spelling that would have worked.
func TestCIDRBlockRefusesHostBitsAndNamesTheFix(t *testing.T) {
	ctx := domain.NewNotificationContext("test")
	if CIDRBlock("203.0.113.5/24").IsValid("CIDR", ctx) {
		t.Fatal("a range with host bits set was accepted; the unique index cannot see that duplicate")
	}
	msgs := ctx.Messages()
	if len(msgs) != 1 || typeName(msgs[0].Notification) != "CIDRHasHostBitsSetNotification" {
		t.Fatalf("expected CIDRHasHostBitsSetNotification, got %v", msgs)
	}
	// The corrected spelling must ride along, or the message is a complaint
	// rather than a fix.
	if msgs[0].FieldValue != "203.0.113.0/24" {
		t.Fatalf("the notification carries %q; expected the canonical form 203.0.113.0/24", msgs[0].FieldValue)
	}
}

// TestCIDRBlockRefusesTheUniversalPrefixes is the security half. Both spellings
// of "everywhere" must be refused, or the allow-list has a second way to say
// "no restriction" that reads like a restriction.
func TestCIDRBlockRefusesTheUniversalPrefixes(t *testing.T) {
	for _, raw := range []string{"0.0.0.0/0", "::/0"} {
		ok, key := cidrVerdict(t, raw)
		if ok {
			t.Errorf("%q was accepted; a range covering everything must be an empty list instead", raw)
			continue
		}
		if key != "UniversalCIDRNotAllowedNotification" {
			t.Errorf("%q answered %s; expected UniversalCIDRNotAllowedNotification", raw, key)
		}
	}
}

// TestCIDRBlockAnswersEmptinessWithTheFrameworksOwnNotification — and this is
// why the spec declares no `required` rule on top of the type. Declaring one
// would tell the caller "Required field" twice for one empty value.
func TestCIDRBlockAnswersEmptinessWithTheFrameworksOwnNotification(t *testing.T) {
	ok, key := cidrVerdict(t, "")
	if ok {
		t.Fatal("an empty range was accepted")
	}
	if key != "RequiredFieldNotification" {
		t.Fatalf("an empty value answered %s; expected RequiredFieldNotification", key)
	}
}

func TestCIDRBlockValueRoundTrips(t *testing.T) {
	const raw = "203.0.113.0/24"
	if got := CIDRBlock(raw).Value(); got != raw {
		t.Fatalf("Value() returned %q, want %q — the mappers read back through it", got, raw)
	}
}
