// Hand-written: declared in the spec as a composite with `written: manual`.
// The SHAPE stays declared there (the schema decomposes the two parts into
// columns, the mappers fold them, the migration sizes them, the catalogs
// translate them); this file is the rule.
//
// Two things put it beyond the spec language. The parts are validated by this
// package's anti-junk predicates — a run-of-identical-runes bound and a
// trimmed-and-single-spaced shape — which no regex states on its own. And the
// RENDERING lives here: joining a given name to a family name with a space is
// a fact about names, not about User, so the day a locale wants the other
// order it is this method that changes rather than every read.
//
// It is deliberately NOT DisplayName. That file's own header draws the line —
// DisplayName is for THINGS, and it reserves this type for people — and the
// difference is not cosmetic: DisplayName requires at least min(3, length)
// DISTINCT runes, which is right for an organization and wrong for a family
// name. "Ng", "Wu" and "Li" are ordinary names and would all be refused.

package vos

import "github.com/ClaudioSchirmer/omnicore/domain"

const (
	// One HALF of a name, not a whole one: 75 runes each, which is what the
	// two columns are sized to. A person carrying more than that in one half
	// is real but is not who this bound is for — the ceiling exists so a
	// pasted paragraph is refused, not so a long name is.
	personNamePartMinRunes = 1
	personNamePartMaxRunes = 75

	// The longest run of one repeated rune a real name can justify. "Aaa"
	// happens in transliterations; "aaaa" is a lean on the keyboard.
	personNameMaxIdenticalRun = 4

	// personNameSeparator is the one place this service knows what joins the
	// two halves of a name. Every consumer of the rendered form calls
	// FullName() — the read side's derivation, and the token issuer when it
	// arrives.
	personNameSeparator = " "
)

// PersonName is a person's name in two halves.
//
// It declares NO Value(), and that absence is load-bearing: it is what tells
// the framework the value spans SEVERAL columns and has to be decomposed
// rather than stored as one rendering. Adding one would make the framework
// treat it as a scalar and panic at schema construction.
type PersonName struct {
	Given  string `labelKey:"UserGivenNameField"`
	Family string `labelKey:"UserFamilyNameField"`
}

// FullName renders the two halves the way a listing shows them.
//
// Named FullName rather than String() on purpose: String() is what a Stringer
// answers to, and a value object that renders itself through fmt is one bad
// log line away from putting a person's name somewhere nobody decided it
// should be. A caller that wants the rendering asks for it by name.
//
// It is also the single definition of the ORDER. A locale that writes the
// family name first — ja-JP is the ordinary example — is a change to this
// method and to nothing else: the two columns, the two filters and the
// computed read field all stay exactly as they are.
func (v PersonName) FullName() string {
	if v.Given == "" {
		return v.Family
	}
	if v.Family == "" {
		return v.Given
	}
	return v.Given + personNameSeparator + v.Family
}

// IsValid is the framework's entry point, found by TYPE with no registration.
//
// fieldName is deliberately unused: a composite reports on its PARTS, not on
// the field carrying it, so both notifications below are emitted under "Given"
// or "Family". That is what lets a caller read WHICH half is wrong, and it
// resolves each part's labelKey from the tag inside this struct without the
// entity declaring anything.
//
// Both halves are checked and NEITHER short-circuits the other: a call
// carrying two malformed names is told about both in one answer rather than
// one per round-trip. There is no pair-level rule here — unlike PermissionKey,
// the two halves of a name constrain each other in no way at all, and
// inventing a rule that they do (a minimum combined length, a "must differ")
// would refuse real people.
func (v PersonName) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	givenValid := validateNameHalf(v.Given, "Given", ctx)
	familyValid := validateNameHalf(v.Family, "Family", ctx)
	return givenValid && familyValid
}

// validateNameHalf enforces the whole shape of one half in a single pass, and
// reports at most ONE InvalidPersonNameNotification however many of the shape
// rules failed — emitting one per broken rule would show the caller the same
// sentence three times for one bad value. Emptiness is the separate answer,
// and it is the framework's own RequiredFieldNotification, which is exactly
// why the aggregate declares no `required` rule on top of this type.
func validateNameHalf(s, part string, ctx *domain.NotificationContext) bool {
	if s == "" {
		ctx.AddNotification(part, domain.RequiredFieldNotification{})
		return false
	}

	length := runeLen(s)

	// No word count and no distinct-rune floor, both deliberately. A mononym
	// is a real given name, and a two-rune family name is ordinary in a large
	// part of the world; either check would refuse people rather than junk.
	// What is left is the junk test that survives translation: length, at
	// least one letter, no keyboard lean, and no stray whitespace.
	valid := length >= personNamePartMinRunes &&
		length <= personNamePartMaxRunes &&
		hasLetter(s) &&
		!hasRunOfIdenticalRunes(s, personNameMaxIdenticalRun) &&
		isTrimmedAndSingleSpaced(s)

	if !valid {
		ctx.AddNotification(part, InvalidPersonNameNotification{}, s)
		return false
	}
	return true
}
