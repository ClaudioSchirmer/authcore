// Hand-written: declared in the spec as `kind: manual` — the rule is a
// composition of anti-junk predicates, not a shape or a closed set.
//
// SHARED with Group and Role, for the same reason DisplayName is: every rule
// below is about human-typed prose, and none of it is about a tenant.

package vos

import "github.com/ClaudioSchirmer/omnicore/domain"

const (
	descriptionMinRunes = 15
	descriptionMaxRunes = 500

	// Prose repeats characters less than a name does, but "..." and "!!!" are
	// ordinary punctuation, so the run bound stays at four.
	descriptionMaxIdenticalRun = 4

	// Higher than a name's floor: fifteen runes of genuine prose cannot be
	// built from four distinct characters.
	descriptionMinDistinct = 5

	// A description is a sentence about the tenant, not a restatement of its
	// name — so it has to carry at least two words.
	descriptionMinWords = 2
)

// Description is human-typed prose saying what a record is, in the platform
// operators' own words.
type Description string

func (v Description) Value() string { return string(v) }

// IsValid enforces the whole shape in one pass, raising a single
// InvalidDescriptionNotification for any combination of shape failures — see
// DisplayName.IsValid for why one is better than one per broken rule.
//
// The vowel clause is what makes this more than a length check: it is the
// predicate that separates prose from a lean on the keyboard. Its Unicode
// definition (see text_predicates.go) is load-bearing — an ASCII-only vowel
// test would reject a description written in any of the non-Latin scripts this
// platform's operators may write in, AS KEYBOARD JUNK.
func (v Description) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)
	if s == "" {
		ctx.AddNotificationNamed(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	length := runeLen(s)
	valid := length >= descriptionMinRunes &&
		length <= descriptionMaxRunes &&
		countWords(s) >= descriptionMinWords &&
		distinctRunes(s) >= descriptionMinDistinct &&
		!hasRunOfIdenticalRunes(s, descriptionMaxIdenticalRun) &&
		hasVowel(s)

	if !valid {
		ctx.AddNotificationNamed(fieldName, InvalidDescriptionNotification{}, s)
		return false
	}
	return true
}
