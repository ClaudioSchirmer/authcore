// Hand-written: declared in the spec as `kind: manual`.
//
// The rule is a COMPOSITION of four class tests, and the reason it cannot be a
// generated `raw` value object is mechanical rather than stylistic: "contains
// at least one lowercase letter, one uppercase letter, one digit and one
// symbol" is four independent existence checks over the same string, which a
// single pattern expresses only with lookahead — and Go's regexp engine (RE2)
// has none, by design.
//
// THE POLICY IS A DECISION MADE AGAINST A CURRENT STANDARD, ON THE RECORD.
// NIST SP 800-63B-4 states, as a SHALL NOT, that composition requirements must
// not be imposed, and sets 15 runes as the floor for a secret that is the ONLY
// factor — which is this service today, since no second factor exists. The
// maintainer chose composition rules knowingly at the model gate; the argument
// on both sides is in specs/scaffold-entity/user/spec.md §B Q3. What that
// buys is auditability against the frameworks that still require classes (PCI
// DSS v4.0 asks for 12 runes with letters and digits); what it costs is what
// NIST says it costs — classes push a population toward `Senha123!` rather
// than toward length.
//
// This file is where that decision is cheap to revisit: it is one type, and
// nothing above it knows what the rule is.
//
// The value NEVER reaches a column. The field carrying it is declared
// `runtime: true` with `source: body`, so it crosses the request, the command
// and the aggregate and stops — there is no TableSchema entry, no migration,
// no outbox payload, no audit event and no response. Nothing in this file may
// log, wrap or render it either: a value object that printed itself would put
// a plaintext credential wherever the caller happened to be printing.

package vos

import (
	"unicode"
	"unicode/utf8"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

const (
	// The floor the model gate chose. It is BELOW the NIST single-factor floor
	// of 15 and is what the four required classes were taken in exchange for —
	// stated here so the trade is visible at the constant rather than only in
	// the spec.
	passwordMinRunes = 8

	// The ceiling. It is not a security bound: Argon2id costs the same over 8
	// runes and over 128, so this exists to keep an unbounded body from
	// becoming an unbounded hashing cost. NIST asks that at least 64 be
	// accepted; 128 clears it with room for a passphrase.
	passwordMaxRunes = 128
)

// Password is a plaintext password on its way in, and nothing else. It exists
// for the length of one request.
type Password string

// Value returns the underlying string. The mappers convert with
// vos.Password(x) and read back through here; it is the contract the generated
// code was written against.
func (v Password) Value() string { return string(v) }

// IsValid is the framework's entry point, found by TYPE with no registration.
//
// It reports at most ONE WeakPasswordNotification however many clauses failed,
// and that is the deliberate choice: the message states the whole policy in
// one sentence, so a caller who is told "8 to 128 characters with a lowercase
// letter, an uppercase letter, a digit and a symbol" already knows everything
// four separate notifications would have said. Emitting one per missing class
// would also describe the caller's own password back to them field by field,
// which is a worse thing to put in a log or a screenshot than a single answer.
//
// Emptiness is the one separate answer, and it is the framework's own
// RequiredFieldNotification — which is exactly why the aggregate declares no
// `required` rule on top of this type. Declaring one would tell the caller
// "Required field" twice for one empty password.
func (v Password) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)
	if s == "" {
		ctx.AddNotification(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	length := runeLen(s)
	lower, upper, digit, symbol := passwordClasses(s)

	valid := utf8.ValidString(s) &&
		length >= passwordMinRunes &&
		length <= passwordMaxRunes &&
		lower && upper && digit && symbol &&
		!hasLeadingOrTrailingSpace(s)

	if !valid {
		// The value is NOT echoed. Every other value object in this package
		// passes the offending input to AddNotification so a caller can see
		// what was rejected; here that would put the plaintext into the
		// notification payload, the response body and any log that renders
		// one. The caller already knows what they typed.
		ctx.AddNotification(fieldName, WeakPasswordNotification{})
		return false
	}
	return true
}

// passwordClasses reports which of the four required classes are present, in
// ONE pass over the runes.
//
// The classes are UNICODE, not ASCII, and that is what makes the rule usable
// in the seven languages this service serves: `ç` is a lowercase letter, `Ä`
// is an uppercase one, `٣` is a digit, and `£` is a symbol. An ASCII-only test
// would quietly demand that a Portuguese or Arabic speaker include a character
// from an alphabet they are not typing in.
//
// A symbol is punctuation OR a symbol proper: Unicode splits `!` (punctuation)
// from `+` (math symbol), and no user has ever meant that distinction when
// asked for "a special character".
func passwordClasses(s string) (lower, upper, digit, symbol bool) {
	for _, r := range s {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r), unicode.IsSymbol(r):
			symbol = true
		}
	}
	return lower, upper, digit, symbol
}

// hasLeadingOrTrailingSpace refuses a password that begins or ends with
// whitespace.
//
// It is NOT isTrimmedAndSingleSpaced, which the names in this package use: a
// password may legitimately contain runs of spaces, because a passphrase is a
// sentence and the strongest thing a human reliably remembers. What is refused
// is the edge, where a space is almost always an accident of copy-paste and is
// invisible in every input box — the caller would set a credential they cannot
// retype.
func hasLeadingOrTrailingSpace(s string) bool {
	runes := []rune(s)
	if len(runes) == 0 {
		return false
	}
	return unicode.IsSpace(runes[0]) || unicode.IsSpace(runes[len(runes)-1])
}
