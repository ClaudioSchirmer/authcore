// Hand-written, and deliberately not generated: these are the shared anti-junk
// predicates the human-typed text value objects of this service compose.
//
// Each of the three text types (DisplayName, Description, TenantWorkspace)
// pairs these with its own bounds and its own notification, so the semantics
// below are written and tested ONCE instead of drifting between three copies.
//
// Three definitions here are binding rather than stylistic, because the obvious
// Go implementation is wrong for this service:
//
//  1. Everything counts RUNES, never bytes. "Acme Comércio e Serviços Ltda" is
//     29 runes and 31 bytes — a byte-based bound is wrong by 2 for the exact
//     naming style this service exists to serve, and wrong by more for every
//     accented Portuguese name after it.
//
//  2. A vowel is defined over Unicode, not over [aeiou]. Any letter outside the
//     Latin script counts as one, so a description written in Japanese, Arabic,
//     Chinese, Russian or Hebrew is never rejected as keyboard junk. The rule is
//     kept because it still costs a Latin-script masher something ("zzz xxx"
//     fails) and must never cost a non-Latin writer anything.
//
//  3. "A letter" and "a word" are Unicode too — unicode.IsLetter — so a word is
//     a run of two or more letters in any script.

package vos

import (
	"strings"
	"unicode"
)

// latinVowels is the accepted Latin vowel set, accents included. It is wider
// than a bare "aeiou" on purpose: the service serves seven Latin-script
// catalogs, and rejecting "Ação" or "Öresund" as junk would be a defect.
//
// Comparison is done case-folded, so only the lowercase forms are listed.
const latinVowels = "aeiouáéíóúãõâêôàüäëïöåøæñýÿ"

// runeLen is the length every bound in this package is expressed in.
func runeLen(s string) int { return len([]rune(s)) }

// distinctRunes counts how many DIFFERENT runes a value is built from. It is
// the cheapest signal that separates a typed name from a mashed keyboard:
// "Acme" has 4, "aaaa" has 1.
func distinctRunes(s string) int {
	seen := make(map[rune]struct{}, len(s))
	for _, r := range s {
		seen[r] = struct{}{}
	}
	return len(seen)
}

// hasRunOfIdenticalRunes reports whether s contains a run of n or more
// identical runes — "aaaa" at n=4. Identical RUNES, not bytes: a repeated
// multi-byte character has to count as one repetition, not as its byte width.
func hasRunOfIdenticalRunes(s string, n int) bool {
	if n <= 1 {
		return s != ""
	}
	run := 0
	var previous rune
	for i, r := range []rune(s) {
		if i > 0 && r == previous {
			run++
		} else {
			run = 1
		}
		if run >= n {
			return true
		}
		previous = r
	}
	return false
}

// hasLetter reports whether s carries at least one Unicode letter, in any
// script. A value made only of digits and punctuation is not a name.
func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// countWords counts runs of two or more Unicode letters. The two-letter floor
// is what stops a stray initial or a dangling consonant from counting as a
// word, so "a b c d e f" does not pass for a description.
//
// KNOWN LIMIT, accepted deliberately: this counts runs separated by
// non-letters, so a scriptio-continua language — Japanese, Chinese, Thai —
// reads as ONE word however long the text is. See
// TestDescriptionWordRuleRefusesScriptioContinua, which pins the limit so that
// changing it later is a deliberate act.
func countWords(s string) int {
	words, run := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			run++
			if run == 2 {
				words++
			}
			continue
		}
		run = 0
	}
	return words
}

// hasVowel reports whether s carries at least one vowel, where "vowel" is the
// Unicode-aware definition from the package comment: a Latin vowel including
// its accented forms, OR any letter outside the Latin script.
func hasVowel(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		// A letter that is not Latin belongs to a script whose vowel notion this
		// service does not model. Counting it as a vowel is what keeps the rule
		// from rejecting a perfectly good non-Latin description.
		if !unicode.Is(unicode.Latin, r) {
			return true
		}
		if strings.ContainsRune(latinVowels, unicode.ToLower(r)) {
			return true
		}
	}
	return false
}

// isTrimmedAndSingleSpaced reports whether s carries no leading or trailing
// whitespace and no run of two or more whitespace characters inside it.
//
// It is a VALIDATION and never a repair: this service refuses " Acme " rather
// than quietly storing "Acme", because a caller who believes they registered
// the first and finds the second has no way to tell when it happened.
func isTrimmedAndSingleSpaced(s string) bool {
	if s != strings.TrimSpace(s) {
		return false
	}
	previousWasSpace := false
	for _, r := range s {
		isSpace := unicode.IsSpace(r)
		if isSpace && previousWasSpace {
			return false
		}
		previousWasSpace = isSpace
	}
	return true
}
