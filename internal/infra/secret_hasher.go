// Hand-written, and not a hook: no generator declares this file.
//
// IT DECLARES NO PORT, and that absence is deliberate. The domain never calls
// this: the aggregate asks ClientService.HashSecret, which is the port, and
// this is the thing that answers it. An interface here would exist only so this
// file could name itself across a layer nobody crosses.
//
// SHA-256, no salt, no work factor — and every one of those three is a decision
// with an argument behind it, not a simpler version of the password adapter
// sitting one file over.
//
// WHY NOT ARGON2ID. Memory-hardness exists to make OFFLINE guessing expensive
// after a database leak, which is worth 19 MiB and ~100 ms per verify against a
// human password of perhaps 30 bits. The secret this hashes is 32 bytes from
// the operating system's random source: not guessable at any cost per guess, so
// multiplying an impossible search by 10^5 buys nothing. What it would cost is
// paid on POST /auth/client/token — unauthenticated by construction, so every
// wrong request an attacker cares to send allocates 19 MiB and burns 100 ms,
// and twice that on the miss path once a rotation is in flight.
//
// WHY NO SALT. A salt defends against precomputation across a POPULATION of
// low-entropy inputs — one rainbow table, many victims. A table over 2^256
// random values does not exist and cannot, so a salt here would be ceremony
// that also makes the stored form longer and the lookup no safer. This is how
// GitHub stores personal access tokens.
//
// WHAT IS NOT NEGOTIABLE is the comparison. A fast digest makes the equality
// check the only place a timing oracle can live, and a plain `==` over the hex
// string returns as soon as two bytes differ — which leaks how many leading
// characters an attacker got right, one request at a time. crypto/subtle is
// what closes that, and it is why verification is a METHOD here rather than
// something a caller does with the string Hash returns.

package infra

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// SHA256SecretHasher turns a machine secret into an irreversible hash and
// verifies one against it. Two methods and no third: everything a caller could
// want from a credential is either "store this" or "is this the one", and any
// surface beyond those two is a way to get it wrong.
//
// It carries no state and no configuration: there are no parameters to tune, so
// there is nothing to hold and nothing to travel with the stored value. That is
// the whole reason the column is a fixed VARCHAR(64) rather than the 255 a PHC
// string needs.
type SHA256SecretHasher struct{}

// NewSHA256SecretHasher returns the adapter. It takes no arguments, and there is
// none it could take.
func NewSHA256SecretHasher() *SHA256SecretHasher { return &SHA256SecretHasher{} }

// Hash returns the lowercase hex SHA-256 of the secret.
//
// It never logs, wraps or copies its input anywhere but into the digest.
func (SHA256SecretHasher) Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Matches reports whether secret produced encoded, in constant time with respect
// to the stored value.
//
// An empty encoded value answers false rather than matching an empty secret: a
// row with no credential is a row nobody can authenticate as, which is exactly
// what false means here. The length check before the comparison is not a leak —
// every hash this adapter writes is the same 64 characters, so a different
// length is a malformed row and not a near miss.
func (h SHA256SecretHasher) Matches(secret, encoded string) bool {
	if encoded == "" {
		return false
	}
	candidate := h.Hash(secret)
	if len(candidate) != len(encoded) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(encoded)) == 1
}
