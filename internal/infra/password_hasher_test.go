// Tests for the one file in this service where a silent bug is a breach rather
// than a defect. Everything here runs without an engine and without an app, so
// there is no excuse for it not to be complete: the hasher is the piece the
// contract suite cannot reach and the generated suite never knew about.
//
// Argon2id at the OWASP baseline costs ~100 ms per call by design, so these
// tests are deliberately few and each one earns its place. The table cases that
// would be free elsewhere are not free here.

package infra

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	// A password with all four classes, so the tests exercise the hasher rather
	// than tripping over the value object's policy.
	testPassword = "Str0ng!Passphrase"
	testOther    = "An0ther!Passphrase"
)

func TestArgon2idHashThenMatch(t *testing.T) {
	h := NewArgon2idHasher()

	encoded := h.Hash(testPassword)

	if !h.Matches(testPassword, encoded) {
		t.Fatal("the password that produced the hash does not verify against it")
	}
	if h.Matches(testOther, encoded) {
		t.Fatal("a different password verified against the hash")
	}
}

// TestArgon2idHashIsSaltedPerCall is the test that catches the single worst
// mistake this file could make: a fixed salt.
//
// With one, two users who chose the same password would hold byte-identical
// hashes — which turns a stolen table into a frequency analysis, and lets one
// cracked password unlock every account that shares it. The hashes must differ
// and both must still verify.
func TestArgon2idHashIsSaltedPerCall(t *testing.T) {
	h := NewArgon2idHasher()

	first := h.Hash(testPassword)
	second := h.Hash(testPassword)

	if first == second {
		t.Fatal("two hashes of the same password are identical — the salt is not per call")
	}
	if !h.Matches(testPassword, first) || !h.Matches(testPassword, second) {
		t.Fatal("a salted hash does not verify against its own password")
	}
}

// TestArgon2idEncodingIsPHC pins the stored FORM, not just the behaviour.
//
// The format is what makes the parameters travel with the value, which is what
// makes raising the cost later a code change rather than a migration. It is
// also what lets anything else that speaks PHC read these hashes — so the
// padding-free base64 is part of the contract, not a detail.
func TestArgon2idEncodingIsPHC(t *testing.T) {
	encoded := NewArgon2idHasher().Hash(testPassword)

	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("the encoded hash does not carry the algorithm and the OWASP baseline parameters: %q", encoded)
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		t.Fatalf("expected six PHC segments, got %d: %q", len(parts), encoded)
	}
	// Only the SALT and HASH segments. Looking for "=" in the whole string finds
	// the parameter separators (v=19, m=19456) and never the padding — which is
	// what the first version of this assertion did, and it failed against a
	// perfectly correct hash.
	for i, segment := range []string{parts[4], parts[5]} {
		if strings.Contains(segment, "=") {
			t.Fatalf("base64 segment %d is padded; PHC specifies the raw alphabet: %q", i, segment)
		}
	}
	// VARCHAR(255) is what the column is sized to, and this is the assertion
	// that catches a parameter change quietly outgrowing it.
	if len(encoded) > 255 {
		t.Fatalf("the encoded hash is %d bytes and the column holds 255", len(encoded))
	}
}

// TestArgon2idMatchesHonoursStoredParameters is the regression test for a bug
// this file actually had: Matches used the package constants instead of the
// parameters written into the hash.
//
// With that bug, raising the cost would have locked every existing user out at
// once — silently, at the moment of the deploy. The fixture below is a hash
// produced under DELIBERATELY CHEAPER parameters than the constants above, so
// it can only verify if the decoded values are the ones used.
func TestArgon2idMatchesHonoursStoredParameters(t *testing.T) {
	h := NewArgon2idHasher()

	// m=8, t=1, p=1: nothing this build would ever write, which is the point.
	cheap := "$argon2id$v=19$m=8,t=1,p=1$" +
		"c29tZXNhbHQxMjM0NQ" + "$" +
		encodeCheapHash(t, testPassword)

	if !h.Matches(testPassword, cheap) {
		t.Fatal("a hash written under cheaper parameters no longer verifies — Matches is using the constants")
	}
	if h.Matches(testOther, cheap) {
		t.Fatal("the wrong password verified against the cheap-parameter hash")
	}
}

// TestArgon2idMatchesRefusesMalformed covers every way a stored value can fail
// to be a hash.
//
// All of them answer FALSE rather than raising, and that is the contract: a row
// whose hash cannot be read is a row nobody can authenticate as. Raising would
// turn a corrupt row into a 500 and — worse, on the public route — would tell
// an attacker that the row exists at all, which is exactly the difference the
// generic answer is there to hide.
func TestArgon2idMatchesRefusesMalformed(t *testing.T) {
	h := NewArgon2idHasher()

	cases := map[string]string{
		"empty":               "",
		"not a PHC string":    "not-a-hash",
		"wrong algorithm":     "$argon2i$v=19$m=19456,t=2,p=1$c29tZXNhbHQ$aGFzaA",
		"unreadable version":  "$argon2id$v=xx$m=19456,t=2,p=1$c29tZXNhbHQ$aGFzaA",
		"unsupported version": "$argon2id$v=16$m=19456,t=2,p=1$c29tZXNhbHQ$aGFzaA",
		"unreadable params":   "$argon2id$v=19$m=abc,t=2,p=1$c29tZXNhbHQ$aGFzaA",
		"zero parameter":      "$argon2id$v=19$m=0,t=2,p=1$c29tZXNhbHQ$aGFzaA",
		"bad salt base64":     "$argon2id$v=19$m=19456,t=2,p=1$!!!$aGFzaA",
		"bad hash base64":     "$argon2id$v=19$m=19456,t=2,p=1$c29tZXNhbHQ$!!!",
		"empty salt":          "$argon2id$v=19$m=19456,t=2,p=1$$aGFzaA",
		"too few segments":    "$argon2id$v=19$m=19456,t=2,p=1$c29tZXNhbHQ",
	}

	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if h.Matches(testPassword, encoded) {
				t.Fatalf("a malformed stored value verified: %q", encoded)
			}
		})
	}
}

// TestArgon2idDummyMatchesCostsTheSameAsAReal one is the timing guard, and it
// is asserted as a RATIO rather than as an absolute: what matters is that a
// miss and a hit are indistinguishable to a stopwatch, not how long either
// takes on this machine.
//
// Without DummyMatches the public change-password route answers an unknown
// e-mail in about a millisecond and a known one in about a hundred — a 100x
// oracle that enumerates users through a response body that says nothing.
//
// The bound is deliberately loose (a factor of three) because a CI runner is a
// noisy clock. It still fails hard on the bug it exists for, which is not a
// 20% skew but two orders of magnitude.
func TestArgon2idDummyMatchesCostsTheSameAsAReal(t *testing.T) {
	h := NewArgon2idHasher()
	encoded := h.Hash(testPassword)

	onAHit := timeIt(func() { h.Matches("wrong-password", encoded) })
	onAMiss := timeIt(func() { h.DummyMatches("wrong-password") })

	ratio := float64(onAHit) / float64(onAMiss)
	if ratio > 3 || ratio < 1.0/3 {
		t.Fatalf("the dummy path costs %.2fx a real verification — the timing oracle is open", ratio)
	}
}

// encodeCheapHash produces the hash half of the cheap-parameter fixture above,
// using the SAME salt string the fixture declares.
//
// It is computed rather than pasted so the fixture cannot rot into a value that
// is merely well-formed: if the algorithm or the encoding ever changes, this
// recomputes and the test still asserts the thing it is about — that the stored
// parameters, not the constants, are the ones used.
func encodeCheapHash(t *testing.T, password string) string {
	t.Helper()
	salt, err := base64.RawStdEncoding.DecodeString("c29tZXNhbHQxMjM0NQ")
	if err != nil {
		t.Fatalf("the fixture salt is not valid raw base64: %v", err)
	}
	key := argon2.IDKey([]byte(password), salt, 1, 8, 1, argonKeyBytes)
	return base64.RawStdEncoding.EncodeToString(key)
}

// timeIt returns how long fn took. One sample, deliberately: the assertion it
// feeds is a two-orders-of-magnitude question, and averaging several 100 ms
// Argon2id calls would triple the suite's runtime to sharpen a bound that does
// not need sharpening.
func timeIt(fn func()) time.Duration {
	start := time.Now()
	fn()
	return time.Since(start)
}
