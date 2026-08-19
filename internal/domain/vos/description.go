package vos

import "github.com/ClaudioSchirmer/omnicore/domain"

// Description is human-typed prose saying what something IS, in the operators'
// own words.
//
// Unlike a display name it DOES carry a word count, and the asymmetry is
// deliberate: a one-word description really is junk, while a one-word name is
// most of the companies in the country.
//
// Shared with Group and Role for the same reason DisplayName is — every rule
// here says "human-typed prose, not junk" and none of them says "tenant".
type Description string

// Value is the underlying primitive the persister binds and the mappers read.
func (v Description) Value() string { return string(v) }

// IsValid is the framework's entry point, run on every write. It reports every
// problem it finds rather than returning at the first.
func (v Description) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)

	if s == "" {
		ctx.AddNotification(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	valid := true
	invalid := func() {
		if valid {
			ctx.AddNotification(fieldName, InvalidDescriptionNotification{}, s)
			valid = false
		}
	}

	if n := runeLen(s); n < 15 || n > 500 {
		invalid()
	}
	if countWords(s) < 2 {
		invalid()
	}
	if distinctRunes(s) < 5 {
		invalid()
	}
	if hasRunOfIdenticalRunes(s, 4) {
		invalid()
	}
	// Any letter outside the Latin script counts as a vowel — see
	// text_predicates.go. Without that clause this line would reject a
	// description written in any of the non-Latin languages this service
	// serves, as keyboard junk.
	if !hasVowel(s) {
		invalid()
	}

	return valid
}
