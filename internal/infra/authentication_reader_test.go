// Tests for the sign-in reader.
//
// WHAT THIS FILE PROTECTS is the timing decoy — the one decision here that is
// arithmetic over no data at all, and the one whose failure is invisible: a burn
// that returned early would turn the response time into an answer to "does this
// address have an account here", which is precisely the question the shared
// refusal message exists to refuse.
//
// THE GRANT WALK IS PROVEN AGAINST A DATABASE, in authentication_reader_live_test.go,
// and it has to be. What matters about it is not the shape of a statement — the
// statements are the framework's now — but that a retired role, a retired group
// and a revoked permission each disappear from the answer. A declared traversal is
// NOT gated on its target's archived state, so those three gates are predicates the
// reader states; asserting them without a database would mean asserting that the
// code contains the line that was written, which proves nothing about the row that
// comes back. The schema pairs the walk relies on are guarded next door, in
// schemas/grant_edge_direct_schemas_test.go.

package infra

import (
	"strings"
	"testing"
)

// ── the timing decoy ────────────────────────────────────────────────────────

// The equalisation hash must be a real, verifiable Argon2id hash that NOTHING
// matches — otherwise the burn either costs nothing (defeating its purpose) or
// could be made to return early.
func TestEqualisationHash_IsARealHashNothingMatches(t *testing.T) {
	if !strings.HasPrefix(equalisationHash, "$argon2id$") {
		t.Fatalf("equalisation hash = %.32q…, want a PHC-encoded Argon2id hash", equalisationHash)
	}
	if userHasher.Matches("timing-equalisation", equalisationHash) {
		t.Error("the decoy matched the string the burn verifies — the burn would return early")
	}
}

// An empty stored hash answers false WITHOUT hashing: a row with no credential
// authenticates nobody, and there is nothing to compare against.
func TestPasswordMatches_EmptyStoredHash(t *testing.T) {
	r := &AuthenticationReader{}
	if r.PasswordMatches("anything", "") {
		t.Error("an empty stored hash must never match")
	}
}

// A stored hash that IS a credential answers true for the right plaintext and
// false for any other — the pair the sign-in branches on.
func TestPasswordMatches_RoundTripsARealHash(t *testing.T) {
	r := &AuthenticationReader{}
	encoded := userHasher.Hash("correct horse battery staple")

	if !r.PasswordMatches("correct horse battery staple", encoded) {
		t.Error("the right plaintext did not match its own hash")
	}
	if r.PasswordMatches("correct horse battery stapler", encoded) {
		t.Error("a wrong plaintext matched")
	}
}
