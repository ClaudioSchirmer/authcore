package vos

import (
	"strings"
	"unicode"
)

// Anti-junk predicates shared by the human-typed text value objects.
//
// They are pure functions, written and tested once, so that DisplayName,
// Description and TenantWorkspace compose the same definitions with their own
// bounds and their own notifications instead of each re-deriving them.
//
// Three definitions are binding here, and each exists because the obvious Go
// implementation is wrong for this service:
//
//   - Everything counts RUNES, never bytes. "Acme Comércio e Serviços Ltda" is
//     29 runes and 31 bytes; a byte-based bound is wrong by 2 for the exact
//     naming style this service exists to serve, and wrong by more for every
//     accented name after it.
//   - "A letter" and "a word" are Unicode, not [a-zA-Z] — a word is a run of
//     two or more letters in ANY script.
//   - "At least one vowel" counts any letter outside the Latin script as one.
//     This service ships seven translation catalogs; an ASCII-only vowel test
//     would reject a description written in Japanese, Arabic, Chinese, Russian
//     or Hebrew AS KEYBOARD JUNK. The rule still costs a Latin-script masher
//     something ("zzz xxx" fails) and must never cost a non-Latin writer
//     anything.
//
// They raise the cost of garbage; they do not prevent it. "asdf asdf" passes
// every one of them. They catch the lazy case — a held key, one character
// repeated, the name pasted into the description — and nothing more.

// latinVowels is the Latin-script vowel set, accented forms included. Any
// letter OUTSIDE the Latin script is handled by hasVowel's second clause and
// is deliberately not enumerated here.
const latinVowels = "aeiou" +
	"áéíóú" + "àèìòù" + "âêîôû" + "ãõñ" + "äëïöü" + "åøæ" + "ýÿ" +
	"AEIOU" +
	"ÁÉÍÓÚ" + "ÀÈÌÒÙ" + "ÂÊÎÔÛ" + "ÃÕÑ" + "ÄËÏÖÜ" + "ÅØÆ" + "ÝŸ"

// runeLen is the length of s in runes.
func runeLen(s string) int {
	return len([]rune(s))
}

// distinctRunes is how many different runes s is made of.
func distinctRunes(s string) int {
	seen := make(map[rune]struct{}, len(s))
	for _, r := range s {
		seen[r] = struct{}{}
	}
	return len(seen)
}

// hasRunOfIdenticalRunes reports whether s contains n or more of the same rune
// in a row — the held-key signature. n must be at least 2.
func hasRunOfIdenticalRunes(s string, n int) bool {
	if n < 2 {
		return false
	}
	run := 0
	var prev rune
	for i, r := range []rune(s) {
		if i > 0 && r == prev {
			run++
		} else {
			run = 1
		}
		if run >= n {
			return true
		}
		prev = r
	}
	return false
}

// hasLetter reports whether s carries at least one Unicode letter.
func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// countWords counts the maximal runs of two or more Unicode letters. A single
// stray letter is not a word, which is what makes "a b c d e f g h" fail a
// two-word rule the way a reader would expect it to.
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

// hasVowel reports whether s carries at least one vowel, where a letter
// outside the Latin script always counts as one. See the note above: this is
// the clause that keeps the rule from rejecting non-Latin text as junk.
func hasVowel(s string) bool {
	for _, r := range s {
		if strings.ContainsRune(latinVowels, r) {
			return true
		}
		if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
			return true
		}
	}
	return false
}

// isTrimmedAndSingleSpaced reports whether s carries no leading or trailing
// whitespace and no run of two or more whitespace runes inside it.
//
// The value is REFUSED rather than repaired: storing something the caller did
// not send is worse than a 422, because the caller then believes they own a
// value that is not the one on record.
func isTrimmedAndSingleSpaced(s string) bool {
	if s != strings.TrimSpace(s) {
		return false
	}
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if prevSpace {
				return false
			}
			prevSpace = true
			continue
		}
		prevSpace = false
	}
	return true
}
