// Hand-written, and not a hook: no generator declares this file.
//
// The attempt ROLLUP — at most two rows per identity, one `failure` and one
// `success`. migrations/postgres/0007_authentication_attempts_manual.up.sql
// carries the full reasoning; the short version is that counters on the `users`
// row would only exist for users that EXIST, which is an existence oracle in both
// the wording and the response time, so this table is keyed by the ATTEMPTED
// identity instead.
//
// IT IS NO LONGER THE FORENSIC RECORD. An earlier shape wrote one row per attempt
// and was both the lockout and the evidence. The evidence moved to the service's
// log stream, published by the sign-in handler; what stays here is the part that
// has to be transactional and queryable: the counters that decide the lock.
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

	attemptColID          = "id"
	attemptColIdentity    = "identity"
	attemptColKind        = "identity_kind"
	attemptColOutcome     = "outcome"
	attemptColTotal       = "total_count"
	attemptColCurrent     = "current_count"
	attemptColBlocked     = "total_blocked"
	attemptColWindowStart = "window_started_at"
	attemptColLastAt      = "last_at"
	attemptColLastIP      = "last_ip"
	attemptColExisted     = "identity_existed"
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

// AuthenticationAttemptStore keeps the counters and answers the lockout question.
//
// It takes the same narrow SQLSeam the refresh store does: two methods, no typed
// write verbs, no rebuild lock. A component whose entire job is one upsert and
// one point lookup has no business holding the engine's write surface.
//
// EVERY STATEMENT IS RENDERED THROUGH THE DIALECT, none is written by hand for a
// particular engine — including the upserts, which go through Dialect.BuildUpsert
// (Postgres and SQLite render ON CONFLICT, MySQL ON DUPLICATE KEY UPDATE, SQL
// Server and Oracle a MERGE). Changing relational.dialect stays a configuration
// change here as it is everywhere else in this service.
type AuthenticationAttemptStore struct {
	engine SQLSeam

	// The two failure upserts differ in ONE assignment and are otherwise
	// identical — see RecordFailure for why a nil existence answer needs its own
	// statement rather than a bound NULL.
	failureStmt            string
	failureKeepExistedStmt string

	successStmt string
	blockedStmt string

	// The stale-window reset, and the success reset. Both are plain UPDATEs by
	// the natural key; see LockedUntil and RecordSuccess.
	expireWindowStmt string
	clearWindowStmt  string

	probeStmt string
}

// NewAuthenticationAttemptStore builds the store and every statement.
//
// The statements are assembled once, at construction, for the same reason the
// permission resolver's is: whatever is wrong with them should be wrong at boot.
func NewAuthenticationAttemptStore(engine SQLSeam) *AuthenticationAttemptStore {
	s := &AuthenticationAttemptStore{engine: engine}
	d := engine.Dialect()
	q := d.QuoteIdent

	// The natural key every statement here addresses a row by. `id` is a
	// surrogate and is never used to find anything.
	conflict := []string{attemptColIdentity, attemptColKind, attemptColOutcome}

	// The insert columns, in the order the arguments are bound. `id` leads
	// because a fresh one is minted for the INSERT branch; it never appears in an
	// update set, so a row that already exists keeps the id it was born with —
	// the same shape the framework's own upserted registries use.
	insertCols := []string{
		attemptColID, attemptColIdentity, attemptColKind, attemptColOutcome,
		attemptColTotal, attemptColCurrent, attemptColWindowStart,
		attemptColLastAt, attemptColLastIP, attemptColExisted,
	}

	// A FAILURE: both counters climb, the origin and the moment are replaced.
	//
	// UpsertSetBump is the framework's portable "the existing row's value plus
	// one" — it exists precisely because no verbatim expression can name the
	// existing row across engines (EXCLUDED / new / target), which is also why
	// the window reset below cannot ride in here as a CASE and gets its own
	// statement.
	//
	// `window_started_at` is deliberately ABSENT from the update set, so on a live
	// window it keeps pointing at the burst's FIRST failure — which is exactly
	// what the expiry is derived from.
	//
	// That absence makes expireWindowStmt the ONLY writer of this column besides
	// the INSERT branch, which is why its predicate has to catch every state that
	// means "no live burst" — aged out AND null. See it for what happens when it
	// misses one.
	failureSets := []core.UpsertSet{
		{Col: attemptColTotal, Mode: core.UpsertSetBump},
		{Col: attemptColCurrent, Mode: core.UpsertSetBump},
		{Col: attemptColLastAt, Mode: core.UpsertSetNew},
		{Col: attemptColLastIP, Mode: core.UpsertSetNew},
	}
	s.failureKeepExistedStmt = d.BuildUpsert(attemptTable, insertCols, conflict, failureSets)
	s.failureStmt = d.BuildUpsert(attemptTable, insertCols, conflict,
		append(append([]core.UpsertSet(nil), failureSets...),
			core.UpsertSet{Col: attemptColExisted, Mode: core.UpsertSetNew}),
	)

	// A SUCCESS: the lifetime count climbs and the moment is replaced. The
	// existence flag is always true here and is written unconditionally — a
	// credential cannot verify against an account that is not there.
	s.successStmt = d.BuildUpsert(attemptTable, insertCols, conflict, []core.UpsertSet{
		{Col: attemptColTotal, Mode: core.UpsertSetBump},
		{Col: attemptColLastAt, Mode: core.UpsertSetNew},
		{Col: attemptColLastIP, Mode: core.UpsertSetNew},
		{Col: attemptColExisted, Mode: core.UpsertSetNew},
	})

	// A REFUSAL MADE WHILE ALREADY LOCKED. It bumps the blocked counter and
	// nothing else — not `current_count`, not `window_started_at`, so the derived
	// expiry is unmoved and the lock CANNOT be extended by continuing to try.
	//
	// `last_at`/`last_ip` are left alone too: they describe the most recent
	// FAILURE, and an attempt that never reached a credential check is not one.
	// The blocked attempt is still announced on the log stream by the handler.
	blockedCols := []string{
		attemptColID, attemptColIdentity, attemptColKind, attemptColOutcome,
		attemptColBlocked, attemptColLastAt, attemptColLastIP,
	}
	s.blockedStmt = d.BuildUpsert(attemptTable, blockedCols, conflict, []core.UpsertSet{
		{Col: attemptColBlocked, Mode: core.UpsertSetBump},
	})

	// THE RE-ANCHOR. Runs before every recorded failure and opens a NEW burst
	// whenever there is no live one. It matches ZERO ROWS in the case that
	// matters — an attack in progress, window still live — where it costs one
	// probe of the natural-key index and nothing else.
	//
	// "NO LIVE BURST" IS TWO STATES, AND MISSING THE SECOND MADE ACCOUNTS
	// PERMANENTLY UNLOCKABLE:
	//
	//   * the anchor has aged out of the window — the ordinary case; and
	//   * THE ANCHOR IS NULL, which is what a successful sign-in leaves behind.
	//
	// The second needs its own predicate because `NULL <= x` is NULL, not TRUE,
	// so a bare comparison silently skips exactly the rows that most need
	// re-anchoring. And since `window_started_at` is deliberately absent from the
	// failure upsert's update set, nothing else would ever set it again: the
	// counter would climb forever against a NULL anchor, and LockedUntil — which
	// cannot derive an expiry without one — would answer "not locked" at any
	// count. Every identity that had ever signed in successfully would be immune
	// to the lockout, with the failure counter visibly climbing in the table the
	// whole time.
	//
	// It re-anchors rather than nulling: the next failure is the first of a new
	// burst, so the window opens at this moment.
	s.expireWindowStmt = fmt.Sprintf(
		"UPDATE %s SET %s = 0, %s = %s WHERE %s = %s AND %s = %s AND %s = %s AND (%s IS NULL OR %s <= %s)",
		q(attemptTable),
		q(attemptColCurrent),
		q(attemptColWindowStart), d.NowExpr(),
		q(attemptColIdentity), d.Placeholder(1),
		q(attemptColKind), d.Placeholder(2),
		q(attemptColOutcome), d.Placeholder(3),
		q(attemptColWindowStart),
		q(attemptColWindowStart), d.Placeholder(4),
	)

	// THE SUCCESS RESET — what "a successful sign-in clears the counter" now
	// means, literally. It zeroes the live count and closes the window; the
	// lifetime total is untouched, because a sign-in must not erase history.
	s.clearWindowStmt = fmt.Sprintf(
		"UPDATE %s SET %s = 0, %s = NULL WHERE %s = %s AND %s = %s AND %s = %s",
		q(attemptTable),
		q(attemptColCurrent),
		q(attemptColWindowStart),
		q(attemptColIdentity), d.Placeholder(1),
		q(attemptColKind), d.Placeholder(2),
		q(attemptColOutcome), d.Placeholder(3),
	)

	// THE LOCKOUT PROBE: one row, by the natural key, on the index the UNIQUE
	// constraint already provides. No window, no ordering, no scan — the whole
	// reason the rollup exists.
	s.probeStmt = fmt.Sprintf(
		"SELECT %s, %s, %s FROM %s WHERE %s = %s AND %s = %s AND %s = %s",
		q(attemptColCurrent), q(attemptColWindowStart), q(attemptColExisted),
		q(attemptTable),
		q(attemptColIdentity), d.Placeholder(1),
		q(attemptColKind), d.Placeholder(2),
		q(attemptColOutcome), d.Placeholder(3),
	)
	return s
}

// RecordFailure counts a credential that was presented and rejected. THE ONLY
// OUTCOME THAT MOVES THE LOCKOUT.
//
// identityExisted is a pointer because nil is a distinct, meaningful answer: when
// the lookup itself failed the service genuinely does not know whether the
// address names an account, and false would be a recorded falsehood in the one
// place that exists to be trusted later. A nil answer leaves the column ALONE
// rather than writing NULL over a verdict some earlier attempt earned — which is
// why there are two failure statements and not one with a bound NULL.
func (s *AuthenticationAttemptStore) RecordFailure(ctx context.Context, identity, kind, ip string, identityExisted *bool) error {
	d := s.engine.Dialect()
	now := time.Now().UTC()

	// Age out a window that has expired, so the upsert below opens a new burst
	// instead of climbing a stale count. Zero rows on the hot path.
	if err := core.Exec(s.engine.Querier(), ctx, s.expireWindowStmt,
		d.EncodeArg(identity),
		d.EncodeArg(kind),
		d.EncodeArg(outcomeFailure),
		d.EncodeArg(now.Add(-LockoutWindow)),
	); err != nil {
		return fmt.Errorf("attempt store: expire window: %w", err)
	}

	stmt := s.failureKeepExistedStmt
	var existed any
	if identityExisted != nil {
		stmt = s.failureStmt
		existed = *identityExisted
	}

	// A random UUID from the framework's own minter, so this table needs no
	// sequence. It is consumed only when this upsert inserts; on a conflict the
	// existing row keeps its own id and this one is discarded.
	if err := core.Exec(s.engine.Querier(), ctx, stmt,
		d.EncodeArg(domain.NewRandomID()),
		d.EncodeArg(identity),
		d.EncodeArg(kind),
		d.EncodeArg(outcomeFailure),
		d.EncodeArg(int64(1)), // total_count on insert; bumped on conflict
		d.EncodeArg(1),        // current_count on insert; bumped on conflict
		d.EncodeArg(now),      // window_started_at — insert only, never updated
		d.EncodeArg(now),
		d.EncodeArg(ip),
		d.EncodeArg(existed),
	); err != nil {
		return fmt.Errorf("attempt store: record failure: %w", err)
	}
	return nil
}

// RecordSuccess counts a credential that verified and CLEARS THE LOCKOUT.
//
// The clear goes FIRST and the count second, deliberately: the reset is the
// load-bearing half, so a failure between the two statements leaves the user
// unlocked rather than spuriously locked.
//
// It writes identity_existed = true without being told: a credential cannot
// verify against an account that is not there, so the flag is implied by the
// outcome, and taking it as a parameter would let a caller record a contradiction.
func (s *AuthenticationAttemptStore) RecordSuccess(ctx context.Context, identity, kind, ip string) error {
	d := s.engine.Dialect()
	now := time.Now().UTC()

	if err := core.Exec(s.engine.Querier(), ctx, s.clearWindowStmt,
		d.EncodeArg(identity),
		d.EncodeArg(kind),
		d.EncodeArg(outcomeFailure),
	); err != nil {
		return fmt.Errorf("attempt store: clear window: %w", err)
	}

	if err := core.Exec(s.engine.Querier(), ctx, s.successStmt,
		d.EncodeArg(domain.NewRandomID()),
		d.EncodeArg(identity),
		d.EncodeArg(kind),
		d.EncodeArg(outcomeSuccess),
		d.EncodeArg(int64(1)), // total_count on insert; bumped on conflict
		d.EncodeArg(0),        // current_count — the lockout is failure-only
		d.EncodeArg(nil),      // window_started_at — likewise
		d.EncodeArg(now),
		d.EncodeArg(ip),
		d.EncodeArg(true),
	); err != nil {
		return fmt.Errorf("attempt store: record success: %w", err)
	}
	return nil
}

// RecordLocked counts an attempt refused because the identity was already locked.
//
// IT BUMPS ONE COUNTER AND TOUCHES NOTHING ELSE. Not the live count, not the
// window anchor — so the expiry derived from them is unmoved and the lock cannot
// be extended by continuing to try. If it could, anyone could hold somebody
// else's account shut indefinitely.
//
// It takes no existence flag: this path never performs a lookup — that is the
// whole economy of a lockout, one indexed read instead of a lookup plus a ~100 ms
// verification — and the flag already sits on the row from the failures that
// caused the lock. Writing it again from a value carried in would say nothing new.
func (s *AuthenticationAttemptStore) RecordLocked(ctx context.Context, identity, kind, ip string) error {
	d := s.engine.Dialect()
	now := time.Now().UTC()

	if err := core.Exec(s.engine.Querier(), ctx, s.blockedStmt,
		d.EncodeArg(domain.NewRandomID()),
		d.EncodeArg(identity),
		d.EncodeArg(kind),
		d.EncodeArg(outcomeFailure),
		d.EncodeArg(int64(1)), // total_blocked on insert; bumped on conflict
		d.EncodeArg(now),
		d.EncodeArg(ip),
	); err != nil {
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
// ONE ROW, BY THE NATURAL KEY. No window predicate in SQL, no ordering, no limit:
// the window is a comparison against a single stored timestamp, done here.
//
// The expiry is DERIVED, never stored — window_started_at plus the window — so
// the lock releases at exactly the moment the count stops applying. Nothing has
// to be cleared and nothing can drift.
//
// THE EXISTENCE FLAG RIDES ALONG FOR FREE, because it is a column on the row this
// statement already reads. Nothing else asks for it, and no second query exists
// to produce it.
func (s *AuthenticationAttemptStore) LockedUntil(
	ctx context.Context, identity, kind string,
) (until time.Time, locked bool, identityExisted *bool, err error) {
	d := s.engine.Dialect()

	rows, qerr := s.engine.Querier().Query(ctx, s.probeStmt,
		d.EncodeArg(identity),
		d.EncodeArg(kind),
		d.EncodeArg(outcomeFailure),
	)
	if qerr != nil {
		return time.Time{}, false, nil, fmt.Errorf("attempt store: lockout probe: %w", qerr)
	}
	defer func() { _ = rows.Close() }()

	var (
		current     int
		windowStart *time.Time
	)
	found := false
	if rows.Next() {
		found = true
		if serr := rows.Scan(&current, &windowStart, &identityExisted); serr != nil {
			return time.Time{}, false, nil, fmt.Errorf("attempt store: scan attempt: %w", serr)
		}
	}
	if rerr := rows.Err(); rerr != nil {
		return time.Time{}, false, nil, fmt.Errorf("attempt store: iterate attempts: %w", rerr)
	}
	// No row at all means this identity has never failed here. A NULL anchor with
	// a live count should now be unreachable — the re-anchor above sets one
	// before any failure counts — and this stays as a guard rather than a policy:
	// an expiry cannot be derived from nothing, and inventing one would be worse
	// than declining to lock.
	if !found || windowStart == nil || current < LockoutThreshold {
		return time.Time{}, false, identityExisted, nil
	}

	until = windowStart.Add(LockoutWindow)
	if !until.After(time.Now().UTC()) {
		// The window aged out. The counter still holds the old burst's value —
		// the next recorded failure re-anchors it — but the identity is free now.
		return time.Time{}, false, identityExisted, nil
	}
	return until, true, identityExisted, nil
}
