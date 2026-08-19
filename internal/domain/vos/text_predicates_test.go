package vos

import "testing"

// The anti-junk predicates are shared by three value objects, so they are
// tested here once rather than through each holder.
//
// Three definitions are BINDING and each has at least one non-Latin and one
// accented-Latin case, because those are the two the obvious Go implementation
// gets wrong: byte-based lengths and an ASCII-only vowel test.

func TestRuneLenCountsRunesNotBytes(t *testing.T) {
	// 29 runes, 31 bytes — the exact naming style this service exists to serve.
	const accented = "Acme Comércio e Serviços Ltda"
	if got, want := runeLen(accented), 29; got != want {
		t.Errorf("runeLen(%q) = %d, want %d — a byte-based bound is wrong by %d here", accented, got, want, len(accented)-want)
	}
	if got, want := runeLen("日本語"), 3; got != want {
		t.Errorf("runeLen(japanese) = %d, want %d", got, want)
	}
	if got, want := runeLen(""), 0; got != want {
		t.Errorf("runeLen(empty) = %d, want %d", got, want)
	}
}

func TestDistinctRunes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"aaaa", 1},
		{"3M", 2},
		{"abc", 3},
		{"日本語", 3},
		{"ááa", 2}, // an accented rune is not the same rune as its bare form
	} {
		if got := distinctRunes(tc.in); got != tc.want {
			t.Errorf("distinctRunes(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestHasRunOfIdenticalRunes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		n    int
		want bool
	}{
		{"aaaa", 4, true},
		{"aaa", 4, false},
		{"xaaaay", 4, true},
		{"abab", 4, false},
		{"", 4, false},
		{"ééééa", 4, true},     // identical RUNES, not bytes
		{"ああああ", 4, true},      // and in any script
		{"anything", 1, false}, // n below 2 is meaningless and never fires
	} {
		if got := hasRunOfIdenticalRunes(tc.in, tc.n); got != tc.want {
			t.Errorf("hasRunOfIdenticalRunes(%q, %d) = %v, want %v", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestHasLetter(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"", false},
		{"123", false},
		{"-- --", false},
		{"a", true},
		{"3M", true},
		{"日本", true}, // a letter in any script is a letter
	} {
		if got := hasLetter(tc.in); got != tc.want {
			t.Errorf("hasLetter(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestCountWordsIsUnicodeAndNeedsTwoLetters(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a b c", 0}, // single letters are not words
		{"ab", 1},
		{"Retail operations", 2},
		{"Operações do grupo", 3}, // accented Latin
		{"日本 語学", 2},              // any script
		{"ab1cd", 2},              // a digit breaks the run
	} {
		if got := countWords(tc.in); got != tc.want {
			t.Errorf("countWords(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// The vowel rule must cost a Latin-script masher something and must cost a
// non-Latin writer nothing. Both halves are asserted, because dropping the
// second one turns the rule into "no Japanese descriptions".
func TestHasVowelNeverRejectsNonLatinText(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want bool
	}{
		{"plain latin vowel", "acme", true},
		{"accented portuguese", "Comércio", true},
		{"umlaut", "Düsseldorf", true},
		{"uppercase only", "ACME", true},
		{"latin masher", "zzz xxx", false},
		{"digits and punctuation", "123 -- 456", false},
		{"japanese", "日本語の説明", true},
		{"arabic", "وصف الشركة", true},
		{"russian", "Описание", true},
		{"hebrew", "תיאור", true},
		{"chinese", "公司描述", true},
	} {
		if got := hasVowel(tc.in); got != tc.want {
			t.Errorf("hasVowel(%s: %q) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestIsTrimmedAndSingleSpaced(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"Acme Comércio", true},
		{"", true},
		{" Acme", false},
		{"Acme ", false},
		{"Acme  Comércio", false},
		{"Acme\tComércio", true},    // one whitespace rune is one space
		{"Acme \t Comércio", false}, // a run of them is not
	} {
		if got := isTrimmedAndSingleSpaced(tc.in); got != tc.want {
			t.Errorf("isTrimmedAndSingleSpaced(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
