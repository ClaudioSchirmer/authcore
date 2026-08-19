package vos

import "github.com/ClaudioSchirmer/omnicore/domain"

// DisplayName is the human-typed display name of a THING — an organization, a
// group, a role. It is deliberately not a person's name: a person's name
// follows different cultural norms, so User will carry its own PersonName and
// a change made for it will not silently move the bounds of everything else.
//
// The rules are substance checks, not a word count. Single-word corporate
// names are ordinary — Nubank, Stone, Ambev, IBM, Google, Petrobras — and a
// two-word rule would reject them at registration.
//
// Shared on purpose: Tenant carries it today, Group and Role will carry it
// next. Nothing in it knows what a tenant is, and the holder's labelKey tag
// supplies which field failed, so the 422 still names the tenant's name.
type DisplayName string

// Value is the underlying primitive the persister binds and the mappers read.
func (v DisplayName) Value() string { return string(v) }

// IsValid is the framework's entry point. It is found by TYPE, with no
// registration, and runs on every write.
//
// It reports every problem it finds through the context instead of returning
// at the first, so one call tells the caller everything that is wrong with the
// value.
func (v DisplayName) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)

	// Presence is answered here, and NOT again in BuildRules: a rule declaring
	// this field required would make the caller read the same complaint twice
	// for one empty value.
	if s == "" {
		ctx.AddNotification(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	valid := true
	invalid := func() {
		if valid {
			ctx.AddNotification(fieldName, InvalidDisplayNameNotification{}, s)
			valid = false
		}
	}

	if n := runeLen(s); n < 2 || n > 120 {
		invalid()
	}
	if !hasLetter(s) {
		invalid()
	}
	// At least 3 distinct runes, or as many as the value is long when it is
	// shorter than that. A flat "at least 3" would be unsatisfiable below three
	// characters and would reject 3M and GE, which are real display names.
	if distinctRunes(s) < min(3, runeLen(s)) {
		invalid()
	}
	if hasRunOfIdenticalRunes(s, 4) {
		invalid()
	}
	if !isTrimmedAndSingleSpaced(s) {
		invalid()
	}

	return valid
}
