// Tests for secret_hasher.go.
//
// The spec asks this one file for 100%, and the reason is not symmetry: it needs
// no database and no fixture, and a defect in it is a credential defect. Every
// branch below is one sentence of the contract.

package infra

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

const (
	aSecret      = "acs_8xQvR2mK9dLpN4wZ7tYcB1hJ6sF3gA5eU0iO8rTvXyM"
	anotherOne   = "acs_ZZZvR2mK9dLpN4wZ7tYcB1hJ6sF3gA5eU0iO8rTvXyM"
	sha256HexLen = 64
)

func TestHashIsTheSHA256InLowercaseHex(t *testing.T) {
	h := NewSHA256SecretHasher()
	got := h.Hash(aSecret)

	want := sha256.Sum256([]byte(aSecret))
	if got != hex.EncodeToString(want[:]) {
		t.Fatalf("Hash returned %q, which is not the SHA-256 of the input", got)
	}
	// The column is VARCHAR(64) precisely because this is fixed-width. A hash
	// that grew would be silently truncated by the database.
	if len(got) != sha256HexLen {
		t.Fatalf("Hash returned %d characters; the column is sized for %d", len(got), sha256HexLen)
	}
	if strings.ToLower(got) != got {
		t.Fatalf("Hash returned %q; the stored form is lowercase hex", got)
	}
}

// TestHashIsDeterministic is the property that makes a salt unnecessary AND
// makes it a design statement rather than an omission: the same secret always
// produces the same row value, which is what lets the token path look a
// credential up by hashing what was presented.
func TestHashIsDeterministic(t *testing.T) {
	h := NewSHA256SecretHasher()
	if h.Hash(aSecret) != h.Hash(aSecret) {
		t.Fatal("Hash is not deterministic; nothing could verify against a stored value")
	}
}

func TestHashDoesNotContainThePlaintext(t *testing.T) {
	h := NewSHA256SecretHasher()
	if strings.Contains(h.Hash(aSecret), aSecret) {
		t.Fatal("the plaintext is inside the hash")
	}
}

func TestMatchesAcceptsTheSecretThatProducedTheHash(t *testing.T) {
	h := NewSHA256SecretHasher()
	if !h.Matches(aSecret, h.Hash(aSecret)) {
		t.Fatal("a secret does not verify against its own hash")
	}
}

func TestMatchesRefusesADifferentSecret(t *testing.T) {
	h := NewSHA256SecretHasher()
	if h.Matches(anotherOne, h.Hash(aSecret)) {
		t.Fatal("a different secret verified")
	}
}

// TestMatchesRefusesAnEmptyStoredValue — a row with no credential is a row
// nobody can authenticate as. Answering true for an empty secret against an
// empty hash would make exactly that row the easiest one to sign in to.
func TestMatchesRefusesAnEmptyStoredValue(t *testing.T) {
	h := NewSHA256SecretHasher()
	if h.Matches("", "") {
		t.Fatal("an empty secret verified against an empty stored value")
	}
	if h.Matches(aSecret, "") {
		t.Fatal("a secret verified against an empty stored value")
	}
}

// TestMatchesRefusesAMalformedStoredValue covers the length branch. It is not a
// leak: every hash this adapter writes is the same 64 characters, so a different
// length is a corrupt row rather than a near miss.
func TestMatchesRefusesAMalformedStoredValue(t *testing.T) {
	h := NewSHA256SecretHasher()
	for _, encoded := range []string{
		"deadbeef",                    // too short
		h.Hash(aSecret) + "00",        // too long
		"$argon2id$v=19$m=19456,t=2$", // a password hash, in the wrong column
	} {
		if h.Matches(aSecret, encoded) {
			t.Errorf("a malformed stored value %q verified", encoded)
		}
	}
}

// TestMatchesRefusesAHashThatDiffersOnlyInCase — the stored form is lowercase,
// and a comparison that folded case would accept a value this adapter never
// wrote.
func TestMatchesRefusesAHashThatDiffersOnlyInCase(t *testing.T) {
	h := NewSHA256SecretHasher()
	if h.Matches(aSecret, strings.ToUpper(h.Hash(aSecret))) {
		t.Fatal("an upper-cased hash verified; the comparison is case-folding somewhere")
	}
}
