// Tests for the shared anti-junk predicates. They are hand-written because the
// predicates are — the generator declares the value objects, never their rules.
//
// The three Unicode definitions from text_predicates.go are what these pin:
// runes over bytes, a Unicode-aware vowel, and Unicode letters and words. Each
// carries at least one accented-Latin and one non-Latin case, because those are
// the ones an ASCII-reflex implementation silently gets wrong.

package vos

import "testing"

// Every bound in this package is expressed in runes. A byte-based length is
// wrong by two for the exact naming style this service exists to serve.
func TestRuneLenCountsRunesNotBytes(t *testing.T) {
	const accented = "Acme Comércio e Serviços Ltda"
	if got, want := runeLen(accented), 29; got != want {
		t.Errorf("runeLen(%q) = %d, want %d", accented, got, want)
	}
	if len(accented) == runeLen(accented) {
		t.Fatal("the fixture is no longer multi-byte; it cannot prove anything")
	}
	if got, want := runeLen("日本語"), 3; got != want {
		t.Errorf("runeLen(japanese) = %d, want %d", got, want)
	}
	if got, want := runeLen(""), 0; got != want {
		t.Errorf("runeLen(empty) = %d, want %d", got, want)
	}
}

func TestDistinctRunesCountsDistinctRunes(t *testing.T) {
	cases := map[string]int{
		"":      0,
		"aaaa":  1,
		"Acme":  4,
		"3M":    2,
		"aábbé": 4, // the b repeats: {a, á, b, é}
		"aábcé": 5,
	}
	for in, want := range cases {
		if got := distinctRunes(in); got != want {
			t.Errorf("distinctRunes(%q) = %d, want %d", in, got, want)
		}
	}
}

// The run is over identical RUNES. A repeated multi-byte character must count
// as one repetition, not as its byte width — the trap a byte-wise scan falls
// into, where "éé" would read as a run of four.
func TestHasRunOfIdenticalRunesIsRuneWise(t *testing.T) {
	if hasRunOfIdenticalRunes("éé", 4) {
		t.Error("two accented runes were read as a run of four bytes")
	}
	if !hasRunOfIdenticalRunes("aaaa", 4) {
		t.Error("a run of four identical runes was not detected")
	}
	if hasRunOfIdenticalRunes("aaa", 4) {
		t.Error("a run of three fired a bound of four")
	}
	if !hasRunOfIdenticalRunes("Acme oooo Ltda", 4) {
		t.Error("a run in the middle of a value was missed")
	}
	if hasRunOfIdenticalRunes("", 4) {
		t.Error("the empty value cannot contain a run")
	}
}

func TestHasLetterIsUnicodeAware(t *testing.T) {
	for _, in := range []string{"a", "Ç", "日", "Ω", "щ"} {
		if !hasLetter(in) {
			t.Errorf("hasLetter(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"", "123", "-- --", "!!!"} {
		if hasLetter(in) {
			t.Errorf("hasLetter(%q) = true, want false", in)
		}
	}
}

// A word is a run of two or more Unicode letters, in any script.
func TestCountWordsRequiresTwoLetters(t *testing.T) {
	cases := map[string]int{
		"":                     0,
		"a b c":                0, // single letters are not words
		"Retail operations":    2,
		"Operações da Acme":    3, // accented letters are letters
		"a2b":                  0,
		"Операции группы Acme": 3, // Cyrillic words count
	}
	for in, want := range cases {
		if got := countWords(in); got != want {
			t.Errorf("countWords(%q) = %d, want %d", in, got, want)
		}
	}
}

// The vowel rule is the one that would quietly turn into a language filter.
// A Latin-script masher must still fail; a non-Latin writer must never pay.
func TestHasVowelAcceptsAccentsAndNonLatinScripts(t *testing.T) {
	for _, in := range []string{
		"Acme",     // plain ASCII vowel
		"Ação",     // accented Latin vowel
		"Öresund",  // umlaut, from the widened set
		"日本語のテナント", // Japanese — no Latin vowel at all
		"شركة",     // Arabic
		"Операции", // Cyrillic
		"עסקי",     // Hebrew
	} {
		if !hasVowel(in) {
			t.Errorf("hasVowel(%q) = false — a non-Latin writer was charged for the rule", in)
		}
	}
	// It still has to cost a Latin-script masher something, or it enforces nothing.
	for _, in := range []string{"", "zzz xxx", "bcdfg", "123 456"} {
		if hasVowel(in) {
			t.Errorf("hasVowel(%q) = true, want false", in)
		}
	}
}

// A validation, never a repair: this service refuses padded input rather than
// storing something the caller did not send.
func TestIsTrimmedAndSingleSpaced(t *testing.T) {
	for _, in := range []string{"Acme", "Acme Comércio e Serviços Ltda", ""} {
		if !isTrimmedAndSingleSpaced(in) {
			t.Errorf("isTrimmedAndSingleSpaced(%q) = false, want true", in)
		}
	}
	for _, in := range []string{" Acme", "Acme ", "Acme  Ltda", "\tAcme", "Acme\n"} {
		if isTrimmedAndSingleSpaced(in) {
			t.Errorf("isTrimmedAndSingleSpaced(%q) = true, want false", in)
		}
	}
}

// The degenerate bound. No caller passes it — every value object here uses 4 —
// so it is a defensive guard, and this pins what it answers rather than leaving
// the branch to be discovered by whoever first reaches for it.
func TestHasRunOfIdenticalRunesDegenerateBound(t *testing.T) {
	if !hasRunOfIdenticalRunes("a", 1) {
		t.Error("at n=1 any non-empty value trivially contains a run")
	}
	if hasRunOfIdenticalRunes("", 1) {
		t.Error("the empty value contains no run at any bound")
	}
	if hasRunOfIdenticalRunes("", 0) {
		t.Error("the empty value contains no run at any bound")
	}
}
