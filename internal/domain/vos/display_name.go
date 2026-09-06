// Hand-written: declared in the spec as `kind: manual` because the rule is a
// COMPOSITION of anti-junk predicates, which neither a regex nor a closed
// member list can state.
//
// SHARED, not tenant-specific. Every rule below says "human-typed text, not
// keyboard junk"; only the 2..120 bound is a calibration, and none of it is
// about a tenant. Group and Role will carry this same type.
//
// The line drawn so the sharing does not sprawl: DisplayName is for THINGS,
// not people. When a person arrives they get their own PersonName — a person's
// name follows different cultural norms, and a change made for it must not
// silently move the bounds of every organization's name.

package vos

import "github.com/ClaudioSchirmer/omnicore/domain"

// The bounds. Deliberately generous at the top: legal-ish trading names in
// Brazil run long ("Companhia Brasileira de Distribuição e Participações").
const (
	displayNameMinRunes = 2
	displayNameMaxRunes = 120

	// The longest run of one repeated character a real name can justify. "Ooo"
	// happens; "oooo" is a lean on the keyboard.
	displayNameMaxIdenticalRun = 4

	// The distinct-rune floor, before the min() below applies it.
	displayNameDistinctFloor = 3
)

// DisplayName is a human-typed display name of a thing: an organization, a
// group, a role — never a person.
type DisplayName string

func (v DisplayName) Value() string { return string(v) }

// IsValid enforces the whole shape in one pass.
//
// It reports at most ONE InvalidDisplayNameNotification however many of the
// shape rules failed, because they all raise the same answer: emitting it per
// broken rule would show the caller the identical sentence three times for one
// bad value. Emptiness is the one separate answer, and it is the framework's
// own RequiredFieldNotification — which is exactly why the aggregate declares
// no `required` rule on top of this type.
func (v DisplayName) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)
	if s == "" {
		ctx.AddNotificationNamed(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	length := runeLen(s)

	// distinct >= min(3, length), NOT a flat ">= 3": a flat rule is
	// unsatisfiable below three characters and would reject "3M" and "GE",
	// which are real display names. Read as one sentence: at least three
	// distinct characters, or as many as the value is long when it is shorter
	// than that.
	requiredDistinct := displayNameDistinctFloor
	if length < requiredDistinct {
		requiredDistinct = length
	}

	valid := length >= displayNameMinRunes &&
		length <= displayNameMaxRunes &&
		hasLetter(s) &&
		distinctRunes(s) >= requiredDistinct &&
		!hasRunOfIdenticalRunes(s, displayNameMaxIdenticalRun) &&
		isTrimmedAndSingleSpaced(s)

	if !valid {
		ctx.AddNotificationNamed(fieldName, InvalidDisplayNameNotification{}, s)
		return false
	}
	return true
}
