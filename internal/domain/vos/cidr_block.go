// Hand-written: declared in the spec as `kind: manual`.
//
// The rule is a COMPOSITION rather than a shape, and that is why no generated
// `raw` value object could carry it: parse, NORMALISE, and refuse two specific
// values. A regex can state the first only, and badly — the number of patterns
// that match a well-formed IPv6 prefix and reject a malformed one is not one
// somebody should be maintaining by hand when `net/netip` already answers it.
//
// THE CANONICAL FORM IS THE HALF THAT IS EASY TO SKIP AND EXPENSIVE TO ADD
// LATER. `203.0.113.5/24` and `203.0.113.0/24` are the same range written two
// ways; stored as typed, the collection's unique index sees two different
// strings and accepts both, and an operator auditing the allow-list reads two
// entries where there is one range. Requiring the masked form on the way in is
// what makes that index tell the truth about duplicates.
//
// THE UNIVERSAL PREFIXES ARE REFUSED, and that refusal is a security decision
// rather than a validation nicety. `0.0.0.0/0` and `::/0` mean exactly what an
// EMPTY collection means — no restriction — and having two spellings for it is
// how a reviewer comes to believe a client is restricted when it is not. One
// spelling, and the empty list is it.
//
// What is deliberately NOT here is a minimum prefix length. Refusing anything
// shorter than, say, /16 would catch a `/2` typed for `/24`, and it would also
// refuse a legitimately large cloud egress range — a floor that is wrong blocks
// real configuration, and the universal case is the one that actually matters.

package vos

import (
	"net/netip"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The two prefixes that mean "everywhere". They are compared after parsing and
// masking, so `0.0.0.0/0` and any spelling that normalises to it are one case.
var universalPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/0"),
	netip.MustParsePrefix("::/0"),
}

// CIDRBlock is one network range an integration may authenticate from.
type CIDRBlock string

// Value returns the underlying string. The mappers convert with
// vos.CIDRBlock(x) and read back through here; it is the contract the
// generated code was written against.
func (v CIDRBlock) Value() string { return string(v) }

// IsValid is the framework's entry point, found by TYPE with no registration.
//
// Emptiness is the framework's own answer — RequiredFieldNotification — which
// is exactly why the spec declares no `required` rule on top of this type.
// Declaring one would tell the caller "Required field" twice for one empty
// value.
//
// It reports at most ONE notification, and the two it can report are different
// problems with different fixes: "this is not a network range" and "this range
// is everything, which is not how you say no restriction".
func (v CIDRBlock) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)
	if s == "" {
		ctx.AddNotification(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	prefix, err := netip.ParsePrefix(s)
	if err != nil {
		ctx.AddNotification(fieldName, InvalidCIDRBlockNotification{}, s)
		return false
	}

	masked := prefix.Masked()
	for _, universal := range universalPrefixes {
		if masked == universal {
			ctx.AddNotification(fieldName, UniversalCIDRNotAllowedNotification{}, s)
			return false
		}
	}

	// THE CANONICAL FORM IS REQUIRED, NOT PRODUCED, and the difference is
	// mechanical rather than stylistic: the generated mapper converts the
	// caller's string straight to this type, so there is no seat between the
	// wire and the value in which a rewrite could happen. Given that, refusing
	// beats normalising elsewhere — a caller told "write 203.0.113.0/24" learns
	// the stored form, where a silent rewrite would hand back a value they did
	// not send and could not predict.
	//
	// What it buys is a unique index that tells the truth. Two spellings of one
	// range are two different strings, and the collection's index would accept
	// both while an operator auditing the allow-list reads two entries where
	// there is one.
	//
	// The corrected spelling rides in the notification payload, so the answer is
	// the fix rather than a complaint about the input.
	if masked.String() != s {
		ctx.AddNotification(fieldName, CIDRHasHostBitsSetNotification{}, masked.String())
		return false
	}

	return true
}
