// Hand-written, and not a hook: no generator declares this file. It is the one
// implementation of domain.PasswordHasher, and the only place in this service
// that knows what Argon2id is.
//
// PARAMETERS ARE THE OWASP BASELINE — 19 MiB of memory, 2 iterations, 1 degree
// of parallelism — which lands around 100 ms on a modern server core. That
// number is the whole design of a password hash: fast enough that a person does
// not notice a login, slow enough that an attacker holding the table pays it
// once per guess.
//
// The memory figure is a CAPACITY fact as much as a security one, and it is the
// one that surprises people: 19 MiB is held for the duration of every
// verification, so a hundred concurrent logins is roughly two gigabytes. That is
// the cost of the algorithm being memory-hard, which is exactly why it resists
// GPU and ASIC cracking where a CPU-only hash does not.
//
// THE STORED FORM CARRIES ITS OWN PARAMETERS. A hash reads
// `$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>`, so raising the cost later
// needs no migration and no column change: new passwords are written with the
// new parameters, old ones keep verifying with the ones they were made under,
// and a rehash-on-next-login can upgrade them one at a time. Nothing outside
// this file ever parses that string.

package infra

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"golang.org/x/crypto/argon2"
)

const (
	// The OWASP baseline. Changing any of these three is safe at any time —
	// see the note on the stored form above — and changing them DOWN is the
	// only one that needs a second thought, because rows already written keep
	// their own stronger parameters and only new ones get the weaker set.
	argonMemoryKiB  uint32 = 19 * 1024
	argonIterations uint32 = 2
	argonThreads    uint8  = 1

	// 16 bytes of salt and 32 of output: the sizes the Argon2 specification
	// recommends, and what every other implementation writes, which is what
	// keeps these hashes readable by anything else that speaks PHC.
	argonSaltBytes = 16
	argonKeyBytes  = 32

	// The version tag Argon2id itself carries. It is written into every hash
	// and checked on the way back, because a hash produced by a different
	// version is not one this build can verify.
	argonVersion = argon2.Version
)

// Argon2idHasher is the only implementation of domain.PasswordHasher.
//
// It is stateless and safe for concurrent use: every call draws its own salt,
// and nothing is shared but the constants above.
type Argon2idHasher struct{}

// NewArgon2idHasher returns the hasher the wiring injects.
func NewArgon2idHasher() *Argon2idHasher { return &Argon2idHasher{} }

// dummyEncoded is what DummyMatches verifies against.
//
// It is built ONCE, at package initialisation, from a value no caller can
// produce. Building it lazily would put the very first miss on a different
// timing path from every later one, which is the leak this exists to close.
var dummyEncoded = (&Argon2idHasher{}).Hash("dummy-password-for-constant-time-comparison")

// Hash returns the PHC-encoded Argon2id hash of plaintext.
//
// It PANICS if the system random source fails. That is deliberate: a process
// that cannot draw 16 random bytes cannot mint a credential, and the two
// alternatives are both worse — returning an error the domain has nowhere to
// put, or falling back to a weaker source and writing a hash whose salt is
// guessable. The write pipeline turns the panic into a 500 with the transaction
// rolled back.
//
// It must never log its input, on any path, and it does not.
func (h *Argon2idHasher) Hash(plaintext string) string {
	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		panic("password hasher: the system random source failed")
	}
	return h.encode(plaintext, salt)
}

// Matches reports whether plaintext produced encoded.
//
// Every failure to understand `encoded` answers FALSE rather than raising: a
// row whose hash is malformed, empty, or written by a version this build does
// not speak is a row nobody can authenticate as, which is what false means.
// Raising instead would turn a corrupt row into a 500 and, worse, would tell an
// attacker that the row exists.
//
// The final comparison is subtle.ConstantTimeCompare, so the time it takes does
// not depend on HOW WRONG the password is. A byte-wise `==` leaks the length of
// the matching prefix, which is enough to walk a hash out one byte at a time
// over enough attempts.
func (h *Argon2idHasher) Matches(plaintext, encoded string) bool {
	p, err := decode(encoded)
	if err != nil {
		return false
	}
	// THE STORED PARAMETERS, never the constants above. A hash written under a
	// cheaper setting has to keep verifying after those constants are raised,
	// or the raise would lock every existing user out at once — silently, and
	// all at the same moment.
	got := argon2.IDKey([]byte(plaintext), p.salt, p.iterations, p.memory, p.threads, uint32(len(p.hash)))
	return subtle.ConstantTimeCompare(got, p.hash) == 1
}

// DummyMatches burns a full verification against a fixed hash and answers
// nothing.
//
// The public change-password route calls it when the e-mail resolves to no
// user. Without it that path returns in about a millisecond and a registered
// address takes about a hundred — a 100x timing oracle that enumerates users
// just as well as a distinct error message would, while the response body says
// nothing.
//
// The result is deliberately discarded and the signature returns nothing, so
// there is no value a caller can accidentally branch on: the only thing this
// call produces is elapsed time.
func (h *Argon2idHasher) DummyMatches(plaintext string) {
	_ = h.Matches(plaintext, dummyEncoded)
}

// encode renders the hash in the PHC string format every Argon2 implementation
// reads.
//
// base64.RawStdEncoding — standard alphabet, NO padding — is what the format
// specifies. Writing padded base64 here produces a string that this file would
// still verify and that nothing else would.
func (h *Argon2idHasher) encode(plaintext string, salt []byte) string {
	key := argon2.IDKey([]byte(plaintext), salt, argonIterations, argonMemoryKiB, argonThreads, argonKeyBytes)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argonVersion, argonMemoryKiB, argonIterations, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}

// decode reads a PHC string back into the salt, the expected hash and THE
// PARAMETERS IT WAS PRODUCED WITH.
//
// Returning the parameters is the whole reason they are stored. A hash written
// under an older, cheaper setting must keep verifying after the constants above
// are raised, or raising them would lock every existing user out at once. Only
// the algorithm and the version are held to what this build speaks.
type argonParams struct {
	memory     uint32
	iterations uint32
	threads    uint8
	salt       []byte
	hash       []byte
}

func decode(encoded string) (argonParams, error) {
	var p argonParams

	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, fmt.Errorf("password hasher: not an argon2id hash")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return p, fmt.Errorf("password hasher: unreadable version")
	}
	if version != argonVersion {
		return p, fmt.Errorf("password hasher: unsupported argon2 version %d", version)
	}

	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.threads); err != nil {
		return p, fmt.Errorf("password hasher: unreadable parameters")
	}
	// A zero in any of the three would make argon2.IDKey panic, and a hash
	// carrying one is corrupt rather than cheap. Refusing here keeps that a
	// false answer instead of a 500 on a login attempt.
	if p.memory == 0 || p.iterations == 0 || p.threads == 0 {
		return p, fmt.Errorf("password hasher: zero parameter")
	}

	var err error
	if p.salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return p, fmt.Errorf("password hasher: unreadable salt")
	}
	if p.hash, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return p, fmt.Errorf("password hasher: unreadable hash")
	}
	if len(p.salt) == 0 || len(p.hash) == 0 {
		return p, fmt.Errorf("password hasher: empty salt or hash")
	}
	return p, nil
}

var _ appdomain.PasswordHasher = (*Argon2idHasher)(nil)
