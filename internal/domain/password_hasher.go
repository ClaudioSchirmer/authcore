// Hand-written, and not a hook: no generator declares this file. It is the port
// the credential path is built on, and it lives in the domain for one reason —
// the aggregate and the rules must be able to say "hash this" and "does this
// match" without ever learning WHICH algorithm answers.
//
// That separation is what makes the algorithm a configuration decision rather
// than a domain change. Raising Argon2id's parameters, or moving off it
// entirely, is a new adapter and nothing else: no rule moves, no test of a rule
// moves, and no column changes, because the stored form carries its own
// parameters.

package domain

// PasswordHasher turns a plaintext into an irreversible hash and verifies one
// against it.
//
// Two methods and no third. There is deliberately no "unhash", no "extract
// parameters" and no "compare two hashes": everything a caller could want from
// a credential is either "store this" or "is this the one", and any surface
// beyond those two is a way to get it wrong.
type PasswordHasher interface {
	// Hash returns the encoded hash of plaintext, including the parameters and
	// the salt it was produced with.
	//
	// It returns no error. That is a decision, not an omission: the only
	// failures a hasher has are a broken random source and a parameter set
	// rejected at construction, and neither is a thing a domain rule can
	// sensibly branch on. The adapter panics on them instead, which the write
	// pipeline turns into a 500 with the transaction rolled back — the correct
	// outcome for "this process cannot produce credentials right now".
	Hash(plaintext string) string

	// Matches reports whether plaintext produced encoded.
	//
	// It answers false for a malformed or empty encoded value rather than
	// raising: a row whose hash cannot be parsed is a row nobody can
	// authenticate as, which is exactly what false means here.
	//
	// The comparison is constant-time with respect to the hash — that is the
	// adapter's obligation, and it is why this is one method rather than
	// "give me the hash and I will compare it myself".
	Matches(plaintext, encoded string) bool

	// DummyMatches burns the same work Matches would, against a fixed hash, and
	// always answers false.
	//
	// It exists for ONE caller: the public change-password route, on an e-mail
	// that resolves to no user. Without it that path returns in about a
	// millisecond while a real address takes about a hundred, and the generic
	// "invalid username or password" leaks by timing exactly what it was
	// written to hide. The result is thrown away; the cost IS the point.
	DummyMatches(plaintext string)
}
