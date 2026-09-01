// Hand-written, and not a hook: no generator declares this file.
//
// The attempt ROLLUP — at most two rows per identity, one `failure` and one
// `success`. migrations/postgres/0007_authentication_attempts_manual.up.sql
// carries the full reasoning; the short version is that counters on the `users`
// row would only exist for users that EXIST, which is an existence oracle in both
// the wording and the response time, so this table is keyed by the ATTEMPTED
// identity instead.
//
// IT IS NOT THE FORENSIC RECORD. The evidence — one record per attempt — lives on
// the service's log stream, published by the sign-in handler. What lives here is
// the part that has to be transactional and queryable: the counters that decide
// the lock.
//
// NOTHING HERE EVER SEES A PASSWORD. The sign-in hands this store an identity, an
// outcome and an origin. There is no parameter a credential could arrive in, and
// no column it could be written to.
//
// EVERY STATEMENT IS THE FRAMEWORK'S. The table is described once as a Direct
// schema and reached through a DirectRepository, so the arithmetic that makes a
// counter safe under concurrency, the instant that dates a row, the identity
// minted for a new one, the placeholders, the quoting and the per-dialect upsert
// rendering are all the engine's. Changing relational.dialect stays a
// configuration change here exactly as it is everywhere else in this service.

package infra

import (
	"context"
	"fmt"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/infra/db/command/read"
	"github.com/ClaudioSchirmer/omnicore/infra/db/command/write"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// The values the `outcome` column accepts — the whole vocabulary, and the
// database enforces it with a CHECK. They are UNEXPORTED, and that is the point:
// the column's vocabulary belongs to whoever owns the table, and nothing outside
// this file needs to spell it.
//
// The application asks for a failure, a success or a blocked attempt by CALLING A
// METHOD NAMED FOR IT — RecordFailure, RecordSuccess, RecordLocked — rather than
// by passing a string. So there is no shared vocabulary to keep in step, no
// exported enum, and no type invented to carry one across a layer boundary.
//
// THERE IS NO `locked` OUTCOME. A refusal made while an identity is already
// locked is not a third row: it bumps `total_blocked` on the failure row, which
// keeps the persistence visible without giving the attacker a way to extend the
// lock — see RecordLocked.
const (
	outcomeFailure = "failure"
	outcomeSuccess = "success"
)

// The kinds of subject an attempt can claim to be. `client` has no route yet; the
// constant exists because the rollup was built generic on purpose, so
// POST /auth/client/token needs no schema change and no second table.
const (
	IdentityKindUser   = "user"
	IdentityKindClient = "client"
)

// LockoutThreshold and LockoutWindow are the policy, and they are a pair: N
// failures inside W minutes locks for the remainder of W, counted from the FIRST
// failure of the burst.
//
// Counting from the first is what makes the release exact rather than
// approximate: the lock ends at the moment that failure ages out of the window,
// which is the same moment the counter stops applying. No separate expiry is
// stored, and none can drift from the data it was derived from.
const (
	LockoutThreshold = 5
	LockoutWindow    = 15 * time.Minute
)

// naturalKey names the row every statement here addresses. `id` is a surrogate
// and is never used to find anything: the question is always "what has this
// identity done under this outcome", which is what the UNIQUE constraint on
// (identity, identity_kind, outcome) indexes.
func naturalKey(identity, kind, outcome string) criteria.Expr {
	return criteria.And(
		criteria.Eq("Identity", identity),
		criteria.Eq("IdentityKind", kind),
		criteria.Eq("Outcome", outcome),
	)
}

// onNaturalKey is the same key, as an upsert's conflict target. It is named per
// call because the framework will not guess it from the schema — the key that
// decides whether the row already exists is the statement's whole premise.
func onNaturalKey() write.UpsertOption {
	return write.OnConflict("Identity", "IdentityKind", "Outcome")
}

// AuthenticationAttemptStore keeps the counters and answers the lockout question.
type AuthenticationAttemptStore struct {
	rows *read.DirectRepository[schemas.AuthenticationAttempt]
}

// NewAuthenticationAttemptStore binds the store to the engine.
//
// The repository validates the schema at CONSTRUCTION — that it is Direct, that
// it declares a primary key, that it is anchored to the row type — so a schema
// that stopped agreeing with the table aborts the boot instead of surfacing as a
// failed write at three in the morning.
func NewAuthenticationAttemptStore(engine core.RelationalEngine) *AuthenticationAttemptStore {
	return &AuthenticationAttemptStore{
		rows: read.NewDirectRepository[schemas.AuthenticationAttempt](
			engine, schemas.AuthenticationAttemptSchema()),
	}
}

// RecordFailure counts a credential that was presented and rejected. THE ONLY
// OUTCOME THAT MOVES THE LOCKOUT.
//
// identityExisted is a pointer because nil is a distinct, meaningful answer: when
// the lookup itself failed the service genuinely does not know whether the
// address names an account, and false would be a recorded falsehood in the one
// place that exists to be trusted later. A nil answer leaves the column out of
// the write entirely, so the verdict some earlier attempt earned survives — a
// column omitted from Values is a column the statement does not touch.
func (s *AuthenticationAttemptStore) RecordFailure(
	ctx context.Context, identity, kind, ip string, identityExisted *bool,
) error {
	if err := s.reAnchorStaleWindow(ctx, identity, kind); err != nil {
		return err
	}

	// BOTH COUNTERS CLIMB, and the increment is the server's: `col = col + 1`
	// computed under the row's lock, so two failures arriving at the same instant
	// cannot collapse into one.
	//
	// `WindowStartedAt` is scoped to the INSERT half, which is what keeps a live
	// window pointing at the burst's FIRST failure — the anchor the expiry is
	// derived from. If a failure re-anchored it, five failures followed by one
	// more every fourteen minutes would hold an account shut forever.
	values := write.Values{
		"Identity":        identity,
		"IdentityKind":    kind,
		"Outcome":         outcomeFailure,
		"TotalCount":      write.Stamp,
		"CurrentCount":    write.Stamp,
		"WindowStartedAt": write.OnInsert(write.Stamp),
		"LastAt":          write.Stamp,
		"LastIP":          ip,
	}
	if identityExisted != nil {
		values["IdentityExisted"] = *identityExisted
	}
	if err := s.rows.Upsert(ctx, values, onNaturalKey()); err != nil {
		return fmt.Errorf("attempt store: record failure: %w", err)
	}
	return nil
}

// reAnchorStaleWindow opens a NEW burst whenever there is no live one, and runs
// before every recorded failure.
//
// It matches ZERO ROWS in the case that matters — an attack in progress, window
// still live — where it costs one probe of the natural-key index and nothing
// else.
//
// "NO LIVE BURST" IS TWO STATES, AND MISSING THE SECOND MADE ACCOUNTS
// PERMANENTLY UNLOCKABLE:
//
//   - the anchor has aged out of the window — the ordinary case; and
//   - THE ANCHOR IS NULL, which is what a successful sign-in leaves behind.
//
// The second needs its own predicate because `NULL <= x` is NULL, not TRUE, so a
// bare comparison silently skips exactly the rows that most need re-anchoring.
// And since the failure upsert scopes the anchor to its insert half, nothing else
// would ever set it again: the counter would climb forever against a NULL anchor,
// and LockedUntil — which cannot derive an expiry without one — would answer "not
// locked" at any count. Every identity that had ever signed in successfully would
// be immune to the lockout, with the failure counter visibly climbing in the
// table the whole time.
//
// It re-anchors rather than nulling: the next failure is the first of a new
// burst, so the window opens at this moment.
//
// THE CUTOFF IS THE ONE INSTANT THIS FILE COMPUTES, and it is a predicate rather
// than a column. The anchor it is compared against was written by the framework —
// from the database's own clock under `relational.clock: db` — so a pod whose
// clock has drifted shifts only which rows this statement considers stale, never
// what gets stored.
func (s *AuthenticationAttemptStore) reAnchorStaleWindow(ctx context.Context, identity, kind string) error {
	cutoff := time.Now().UTC().Add(-LockoutWindow)
	_, err := s.rows.Update(ctx, write.Values{
		"CurrentCount":    write.StampEmpty,
		"WindowStartedAt": write.Stamp,
	}, criteria.Where(criteria.And(
		naturalKey(identity, kind, outcomeFailure),
		criteria.Or(
			criteria.IsNull("WindowStartedAt"),
			criteria.Lte("WindowStartedAt", cutoff),
		),
	)))
	if err != nil {
		return fmt.Errorf("attempt store: re-anchor window: %w", err)
	}
	return nil
}

// RecordSuccess counts a credential that verified and CLEARS THE LOCKOUT.
//
// The clear goes FIRST and the count second, deliberately: the reset is the
// load-bearing half, so a failure between the two writes leaves the user unlocked
// rather than spuriously locked.
//
// It writes identity_existed = true without being told: a credential cannot
// verify against an account that is not there, so the flag is implied by the
// outcome, and taking it as a parameter would let a caller record a contradiction.
func (s *AuthenticationAttemptStore) RecordSuccess(ctx context.Context, identity, kind, ip string) error {
	// THE SUCCESS RESET — what "a successful sign-in clears the counter" means,
	// literally. It zeroes the live count and closes the window, against the
	// FAILURE row; the lifetime total is untouched, because a sign-in must not
	// erase history.
	if _, err := s.rows.Update(ctx, write.Values{
		"CurrentCount":    write.StampEmpty,
		"WindowStartedAt": write.StampNull,
	}, criteria.Where(naturalKey(identity, kind, outcomeFailure))); err != nil {
		return fmt.Errorf("attempt store: clear window: %w", err)
	}

	// The success row carries no live counter and no window: the lockout is
	// failure-only, and both columns take the DEFAULT the migration declared.
	// Naming them here to write a zero and a NULL would say the same thing in
	// more words, and would invite the reader to wonder what else the success
	// path does to the lock.
	if err := s.rows.Upsert(ctx, write.Values{
		"Identity":        identity,
		"IdentityKind":    kind,
		"Outcome":         outcomeSuccess,
		"TotalCount":      write.Stamp,
		"LastAt":          write.Stamp,
		"LastIP":          ip,
		"IdentityExisted": true,
	}, onNaturalKey()); err != nil {
		return fmt.Errorf("attempt store: record success: %w", err)
	}
	return nil
}

// RecordLocked counts an attempt refused because the identity was already locked.
//
// IT BUMPS ONE COUNTER AND TOUCHES NOTHING ELSE ON AN EXISTING ROW. Not the live
// count, not the window anchor — so the derived expiry is unmoved and the lock
// CANNOT be extended by continuing to try. If it could, anyone could hold
// somebody else's account shut indefinitely.
//
// `last_at`/`last_ip` are scoped to the insert half for the same reason: they
// describe the most recent FAILURE, and an attempt that never reached a
// credential check is not one. They are written on the creating path only because
// the column is NOT NULL and a row has to be born with something; every blocked
// attempt is still announced on the log stream by the handler.
//
// It takes no existence flag: this path never performs a lookup — that is the
// whole economy of a lockout, one indexed read instead of a lookup plus a ~100 ms
// verification — and the flag already sits on the row from the failures that
// caused the lock. Writing it again from a value carried in would say nothing new.
func (s *AuthenticationAttemptStore) RecordLocked(ctx context.Context, identity, kind, ip string) error {
	if err := s.rows.Upsert(ctx, write.Values{
		"Identity":     identity,
		"IdentityKind": kind,
		"Outcome":      outcomeFailure,
		"TotalBlocked": write.Stamp,
		"LastAt":       write.OnInsert(write.Stamp),
		"LastIP":       write.OnInsert(ip),
	}, onNaturalKey()); err != nil {
		return fmt.Errorf("attempt store: record blocked: %w", err)
	}
	return nil
}

// LockedUntil reports whether an identity is currently locked, until when, and
// what its failures established about whether it names a real account.
//
// A locked identity answers (expiry, true); anything else answers (zero, false).
// It makes NO judgement about whether the identity names a real account — that is
// the entire point: the answer is identical either way, so the refusal cannot be
// used to discover which addresses have accounts here.
//
// ONE ROW, BY THE NATURAL KEY. No window predicate, no ordering, no limit: the
// window is a comparison against a single stored timestamp, done here.
//
// The expiry is DERIVED, never stored — the anchor plus the window — so the lock
// releases at exactly the moment the count stops applying. Nothing has to be
// cleared and nothing can drift.
//
// THE EXISTENCE FLAG RIDES ALONG FOR FREE, because it is a column on the row this
// read already brings back. Nothing else asks for it, and no second query exists
// to produce it.
func (s *AuthenticationAttemptStore) LockedUntil(
	ctx context.Context, identity, kind string,
) (until time.Time, locked bool, identityExisted *bool, err error) {
	row, err := s.rows.FindOne(ctx, criteria.Where(naturalKey(identity, kind, outcomeFailure)))
	switch {
	case err == nil:
	case isRecordNotFound(err):
		// No row at all means this identity has never failed here. Not locked,
		// and nothing established about it.
		return time.Time{}, false, nil, nil
	default:
		// A READ THAT FAILED IS NOT AN ANSWER. Reporting "not locked" here would
		// turn a database problem into unlimited guesses against every account.
		return time.Time{}, false, nil, fmt.Errorf("attempt store: lockout probe: %w", err)
	}

	until, locked = lockoutOf(row, time.Now().UTC())
	return until, locked, row.IdentityExisted, nil
}

// lockoutOf is the policy, applied to one row at one instant: the whole of what
// "is this identity locked" means, with nothing about storage in it.
//
// It is separate from LockedUntil because the two answer different kinds of
// question. LockedUntil finds the row; this decides about it — and the decision
// is where a mistake is expensive and silent, so it is worth being able to state
// every case of it directly.
//
// `now` is a parameter rather than a call to the clock for the same reason: a
// window that is about to close and one that just did are one nanosecond apart,
// and a rule that cannot be asked about that boundary is a rule nobody has
// checked at it.
func lockoutOf(row schemas.AuthenticationAttempt, now time.Time) (until time.Time, locked bool) {
	// A NULL anchor with a live count should be unreachable — the re-anchor above
	// sets one before any failure counts — and this stays as a guard rather than a
	// policy: an expiry cannot be derived from nothing, and inventing one would be
	// worse than declining to lock.
	if row.WindowStartedAt == nil || row.CurrentCount < LockoutThreshold {
		return time.Time{}, false
	}
	until = row.WindowStartedAt.Add(LockoutWindow)
	if !until.After(now) {
		// The window aged out. The counter still holds the old burst's value —
		// the next recorded failure re-anchors it — but the identity is free now.
		return time.Time{}, false
	}
	return until, true
}
