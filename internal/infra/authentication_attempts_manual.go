// Hand-written, and not a hook: no generator declares this file.
//
// The append-only attempt log — the lockout's counter and the forensic record in
// one table. migrations/postgres/0007_authentication_attempts_manual.up.sql
// carries the full reasoning; the short version is that a counter on the `users`
// row would have existed only for users that EXIST, which is an existence oracle
// in both the wording and the response time.
//
// NOTHING HERE EVER SEES A PASSWORD. The sign-in hands this store an identity, an
// outcome and an origin. There is no parameter a credential could arrive in, and
// no column it could be written to.

package infra

import (
	"context"
	"fmt"
	"time"

	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// The physical names, from the 0007 migration. Written by hand for the same
// reason the refresh store's are: this table is not an entity, so there is no
// TableSchema to ask.
const (
	attemptTable = "authentication_attempts"

	attemptColID         = "id"
	attemptColIdentity   = "identity"
	attemptColKind       = "identity_kind"
	attemptColOutcome    = "outcome"
	attemptColExisted    = "identity_existed"
	attemptColIP         = "ip"
	attemptColOccurredAt = "occurred_at"
)

// The values the `outcome` column accepts. They are UNEXPORTED, and that is the
// point: the column's vocabulary belongs to whoever owns the table, and nothing
// outside this file needs to spell it.
//
// The application asks for a failure, a success or a locked attempt by CALLING A
// METHOD NAMED FOR IT — RecordFailure, RecordSuccess, RecordLocked — rather than
// by passing a string. So there is no shared vocabulary to keep in step, no
// exported enum, and no type invented to carry one across a layer boundary.
//
// The three are not symmetric, and the asymmetry is the lockout policy:
// failure counts; success ANCHORS the window, so a good sign-in stops everything
// before it from counting; locked is recorded and never counted — if attempts
// made during a lock extended it, anyone could hold somebody else's account shut
// indefinitely just by continuing to try.
const (
	outcomeFailure = "failure"
	outcomeSuccess = "success"
	outcomeLocked  = "locked"
)

// The kinds of subject an attempt can claim to be. `client` has no route yet; the
// constant exists because the log was built generic on purpose, so
// POST /auth/client/token needs no schema change and no second table.
const (
	IdentityKindUser   = "user"
	IdentityKindClient = "client"
)

// LockoutThreshold and LockoutWindow are the policy, and they are a pair: N
// failures inside W minutes locks for the remainder of W, counted from the OLDEST
// of those N.
//
// Counting from the oldest is what makes the release exact rather than
// approximate: the lock ends at the moment that failure ages out of the window,
// which is the same moment the count drops back below the threshold. No separate
// expiry is stored, and none can drift from the data it was derived from.
const (
	LockoutThreshold = 5
	LockoutWindow    = 15 * time.Minute
)

// AuthenticationAttemptStore appends attempts and answers the lockout question.
//
// It takes the same narrow SQLSeam the refresh store does: two methods, no typed
// write verbs, no rebuild lock. A component whose entire job is one INSERT and
// one SELECT has no business holding the engine's write surface.
type AuthenticationAttemptStore struct {
	engine SQLSeam

	insertStmt string
	countStmt  string
}

// NewAuthenticationAttemptStore builds the store and both statements.
//
// The statements are assembled once, at construction, for the same reason the
// permission resolver's is: whatever is wrong with them should be wrong at boot.
func NewAuthenticationAttemptStore(engine SQLSeam) *AuthenticationAttemptStore {
	s := &AuthenticationAttemptStore{engine: engine}
	d := engine.Dialect()
	q := d.QuoteIdent

	s.insertStmt = fmt.Sprintf(
		"INSERT INTO %s (%s, %s, %s, %s, %s, %s, %s) VALUES (%s, %s, %s, %s, %s, %s, %s)",
		q(attemptTable),
		q(attemptColID), q(attemptColIdentity), q(attemptColKind), q(attemptColOutcome),
		q(attemptColExisted), q(attemptColIP), q(attemptColOccurredAt),
		d.Placeholder(1), d.Placeholder(2), d.Placeholder(3), d.Placeholder(4),
		d.Placeholder(5), d.Placeholder(6), d.Placeholder(7),
	)

	// The N most recent COUNTED failures for one identity.
	//
	// Two predicates narrow it, and the second is the one that is easy to miss:
	// the window itself, and the anchor at the most recent SUCCESS. Without the
	// anchor a user who failed four times, signed in, then mistyped once would be
	// locked by the five failures still sitting inside the window — the opposite
	// of "a successful sign-in clears the counter".
	//
	// ApplyLimit rather than a literal LIMIT: the cap is a tail clause on some
	// engines and a SELECT-head rewrite on others, and the dialect owns which.
	inner := fmt.Sprintf(
		"SELECT %s, %s FROM %s WHERE %s = %s AND %s = %s AND %s > %s AND %s > COALESCE((SELECT MAX(%s) FROM %s WHERE %s = %s AND %s = %s), %s) ORDER BY %s DESC",
		q(attemptColOccurredAt), q(attemptColExisted), q(attemptTable),
		q(attemptColIdentity), d.Placeholder(1),
		q(attemptColOutcome), d.Placeholder(2),
		q(attemptColOccurredAt), d.Placeholder(3),
		q(attemptColOccurredAt),
		q(attemptColOccurredAt), q(attemptTable),
		q(attemptColIdentity), d.Placeholder(4),
		q(attemptColOutcome), d.Placeholder(5),
		d.Placeholder(6),
		q(attemptColOccurredAt),
	)
	s.countStmt = d.ApplyLimit(inner, LockoutThreshold)
	return s
}

// RecordFailure appends a credential that was presented and rejected. THE ONLY
// OUTCOME THAT COUNTS toward a lockout.
//
// identityExisted is a pointer because nil is a distinct, meaningful answer: when
// the lookup itself failed the service genuinely does not know whether the address
// names an account, and false would be a recorded falsehood in the one table that
// exists to be trusted later.
func (s *AuthenticationAttemptStore) RecordFailure(ctx context.Context, identity, kind, ip string, identityExisted *bool) error {
	var existed any
	if identityExisted != nil {
		existed = *identityExisted
	}
	return s.append(ctx, identity, kind, outcomeFailure, existed, ip)
}

// RecordSuccess appends a credential that verified.
//
// It writes identity_existed = true without being told: a credential cannot
// verify against an account that is not there, so the flag is not an input here —
// it is implied by the outcome, and taking it as a parameter would let a caller
// record a contradiction.
func (s *AuthenticationAttemptStore) RecordSuccess(ctx context.Context, identity, kind, ip string) error {
	return s.append(ctx, identity, kind, outcomeSuccess, true, ip)
}

// RecordLocked appends an attempt refused because the identity was already locked.
//
// identityExisted is CARRIED IN rather than looked up. The lock is checked BEFORE
// the account lookup — that is the whole economy of a lockout, one indexed read
// instead of a lookup plus a ~100 ms verification — so this path never asks the
// question itself. What it uses is the answer the COUNTED FAILURES already gave:
// LockedUntil reads those rows anyway, and they carry the flag, so it comes back
// for free and lands here.
//
// It is still NULL when nothing established it — an identity whose failures were
// all recorded during an outage, for instance. The column never guesses.
func (s *AuthenticationAttemptStore) RecordLocked(ctx context.Context, identity, kind, ip string, identityExisted *bool) error {
	var existed any
	if identityExisted != nil {
		existed = *identityExisted
	}
	return s.append(ctx, identity, kind, outcomeLocked, existed, ip)
}

// append is the one writer the three entry points share.
//
// ITS FAILURE PROPAGATES, unlike the refresh store's housekeeping sweep. A sweep
// that fails costs a little disk; an attempt that fails to record costs the
// lockout its evidence, and a brute-force protection that silently stops counting
// is worse than one that was never built — nobody would know. The caller turns it
// into a 500, the same posture the sign-in takes for any other store failure: an
// authentication that cannot reach its store must not answer.
func (s *AuthenticationAttemptStore) append(ctx context.Context, identity, kind, outcome string, existed any, ip string) error {
	d := s.engine.Dialect()

	// A random UUID from the framework's own minter, so this table needs no
	// sequence. Random and NOT time-ordered — `occurred_at` is what orders these
	// rows, and it is the column both the lockout and every report read.
	id := domain.NewRandomID()

	if err := core.Exec(s.engine.Querier(), ctx, s.insertStmt,
		d.EncodeArg(id),
		d.EncodeArg(identity),
		d.EncodeArg(kind),
		d.EncodeArg(outcome),
		d.EncodeArg(existed),
		d.EncodeArg(ip),
		d.EncodeArg(time.Now().UTC()),
	); err != nil {
		return fmt.Errorf("attempt store: record: %w", err)
	}
	return nil
}

// LockedUntil reports whether an identity is currently locked, until when, and
// what the counted failures established about its existence.
//
// A locked identity answers (expiry, true); anything else answers (zero, false).
// It makes NO judgement about whether the identity names a real account — that is
// the entire point: the answer is identical either way, so the refusal cannot be
// used to discover which addresses have accounts here.
//
// The expiry is DERIVED, never stored: it is the oldest of the N counted failures
// plus the window, so the lock releases at exactly the moment the count drops back
// below the threshold. Nothing has to be cleared and nothing can drift.
//
// THE EXISTENCE FLAG RIDES ALONG FOR FREE. This statement already reads the
// identity's failure rows, and those rows carry it — so the most recent one that
// was actually established comes back with the verdict, and the caller can stamp
// it on the `locked` row it is about to write. That is what lets somebody query
// `WHERE outcome = 'locked'` ALONE and still see which locks are real accounts
// under attack and which are noise; without it that question needs a group-by over
// the identity, and nobody writes that by accident.
func (s *AuthenticationAttemptStore) LockedUntil(
	ctx context.Context, identity string,
) (until time.Time, locked bool, identityExisted *bool, err error) {
	d := s.engine.Dialect()
	windowStart := time.Now().UTC().Add(-LockoutWindow)

	rows, qerr := s.engine.Querier().Query(ctx, s.countStmt,
		d.EncodeArg(identity),
		d.EncodeArg(outcomeFailure),
		d.EncodeArg(windowStart),
		d.EncodeArg(identity),
		d.EncodeArg(outcomeSuccess),
		// The floor for the anchor when the identity has never succeeded. The
		// window start, not a zero time: a failure older than the window is out
		// of scope regardless, so this keeps both predicates saying the same
		// thing instead of leaving one wider than the other.
		d.EncodeArg(windowStart),
	)
	if qerr != nil {
		return time.Time{}, false, nil, fmt.Errorf("attempt store: lockout probe: %w", qerr)
	}
	defer func() { _ = rows.Close() }()

	var counted []time.Time
	for rows.Next() {
		var (
			at     time.Time
			exists *bool
		)
		if serr := rows.Scan(&at, &exists); serr != nil {
			return time.Time{}, false, nil, fmt.Errorf("attempt store: scan attempt: %w", serr)
		}
		counted = append(counted, at)
		// Rows arrive newest-first, so the FIRST non-null wins: the most recent
		// answer anybody actually established, not the oldest one on file.
		if identityExisted == nil && exists != nil {
			identityExisted = exists
		}
	}
	if rerr := rows.Err(); rerr != nil {
		return time.Time{}, false, nil, fmt.Errorf("attempt store: iterate attempts: %w", rerr)
	}

	if len(counted) < LockoutThreshold {
		return time.Time{}, false, identityExisted, nil
	}
	// Ordered newest-first by the statement, so the last one is the oldest of the
	// N that locked this identity — and the one whose ageing-out ends the lock.
	oldest := counted[len(counted)-1]
	until = oldest.Add(LockoutWindow)
	if !until.After(time.Now().UTC()) {
		// Defensive: the window moved between the query and here. Not locked.
		return time.Time{}, false, identityExisted, nil
	}
	return until, true, identityExisted, nil
}
