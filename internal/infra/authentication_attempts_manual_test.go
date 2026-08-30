// Tests for the attempt rollup.
//
// The statements are inspected rather than executed, because what matters here
// cannot be seen by running one against clean data. A recording upsert that
// silently started updating `window_started_at` would keep working, keep passing
// an integration test, and quietly make a lock extendable by anyone willing to
// keep trying — the exact abuse the design exists to prevent. A probe that lost
// its outcome predicate would read the SUCCESS row and never lock anybody.
//
// WHAT IS ASSERTED ABOUT THE UPSERTS IS WHAT THIS STORE DECIDES: the table, the
// conflict key, which columns are inserted and which assignments are applied on
// conflict. How those render per engine is the framework's own contract, tested
// there — asserting a rendering here would only be re-testing the stub below.

package infra

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// ── the recording dialect ───────────────────────────────────────────────────

// upsertCall is one BuildUpsert the store asked for.
type upsertCall struct {
	table    string
	cols     []string
	conflict []string
	sets     []core.UpsertSet
}

// recordingDialect is testDialect plus a memory of every upsert requested. It
// returns a statement text that names the table and the conflict key, so a
// failing assertion elsewhere still prints something readable.
type recordingDialect struct {
	testDialect
	calls *[]upsertCall
}

func (d recordingDialect) BuildUpsert(table string, cols, conflict []string, sets []core.UpsertSet) string {
	*d.calls = append(*d.calls, upsertCall{table: table, cols: cols, conflict: conflict, sets: sets})
	return "UPSERT INTO " + table + " ON (" + strings.Join(conflict, ",") + ")"
}

type recordingSeam struct {
	q *recordingQuerier
	d recordingDialect
}

func (s *recordingSeam) Querier() core.Querier { return s.q }
func (s *recordingSeam) Dialect() core.Dialect { return s.d }

// newAttemptStore builds the store and hands back the upserts it asked for at
// construction: [0] failure keeping the stored existence verdict, [1] failure
// writing a fresh one, [2] success, [3] blocked.
func newAttemptStore(q *recordingQuerier) (*AuthenticationAttemptStore, *[]upsertCall) {
	calls := &[]upsertCall{}
	return NewAuthenticationAttemptStore(&recordingSeam{q: q, d: recordingDialect{calls: calls}}), calls
}

const (
	upsertFailureKeepExisted = 0
	upsertFailure            = 1
	upsertSuccess            = 2
	upsertBlocked            = 3
)

// setFor returns the assignment applied to col on conflict, and whether there is
// one at all. "There is none" is load-bearing in several tests below.
func setFor(call upsertCall, col string) (core.UpsertSet, bool) {
	for _, s := range call.sets {
		if s.Col == col {
			return s, true
		}
	}
	return core.UpsertSet{}, false
}

// ── what the upserts are built to do ────────────────────────────────────────

// Every write addresses the row by the NATURAL KEY. Conflict on anything else —
// the surrogate id above all — and each attempt inserts a new row, which is the
// unbounded table this whole change exists to stop.
func TestUpserts_AllConflictOnTheNaturalKey(t *testing.T) {
	_, calls := newAttemptStore(&recordingQuerier{})
	if len(*calls) != 4 {
		t.Fatalf("built %d upserts, want 4 (failure×2, success, blocked)", len(*calls))
	}
	for i, call := range *calls {
		if call.table != "authentication_attempts" {
			t.Errorf("upsert %d targets %q", i, call.table)
		}
		want := []string{"identity", "identity_kind", "outcome"}
		if strings.Join(call.conflict, ",") != strings.Join(want, ",") {
			t.Errorf("upsert %d conflicts on %v, want %v — anything else lets the table grow per attempt",
				i, call.conflict, want)
		}
		if _, ok := setFor(call, "id"); ok {
			t.Errorf("upsert %d updates the surrogate id; an existing row must keep the one it was born with", i)
		}
	}
}

// THE ONE THAT MATTERS MOST. A failure climbs both counters — and must NOT touch
// the window anchor, because the anchor is what the expiry is derived from. If a
// failure re-anchored it, five failures followed by one more every fourteen
// minutes would hold an account shut forever.
func TestFailureUpsert_BumpsBothCountersAndNeverMovesTheAnchor(t *testing.T) {
	_, calls := newAttemptStore(&recordingQuerier{})

	for _, idx := range []int{upsertFailure, upsertFailureKeepExisted} {
		call := (*calls)[idx]
		for _, col := range []string{"total_count", "current_count"} {
			s, ok := setFor(call, col)
			if !ok {
				t.Fatalf("upsert %d does not update %s", idx, col)
			}
			if s.Mode != core.UpsertSetBump {
				t.Errorf("%s uses mode %v, want UpsertSetBump — a bound value would lose concurrent failures",
					col, s.Mode)
			}
		}
		if _, ok := setFor(call, "window_started_at"); ok {
			t.Errorf("upsert %d updates window_started_at on conflict; a live window must keep pointing "+
				"at the burst's FIRST failure or the lock never expires", idx)
		}
	}
}

// A REFUSAL MADE WHILE LOCKED BUMPS ONE COUNTER AND NOTHING ELSE. This is the
// property that replaced the old `locked` row: the persistence stays visible and
// yet cannot extend the lock.
func TestBlockedUpsert_CannotExtendTheLock(t *testing.T) {
	_, calls := newAttemptStore(&recordingQuerier{})
	call := (*calls)[upsertBlocked]

	s, ok := setFor(call, "total_blocked")
	if !ok || s.Mode != core.UpsertSetBump {
		t.Fatalf("total_blocked assignment = %#v (present %v), want a bump", s, ok)
	}
	if len(call.sets) != 1 {
		t.Fatalf("the blocked upsert applies %d assignments, want exactly 1 — anything else can move the lock",
			len(call.sets))
	}
	for _, col := range []string{"current_count", "window_started_at", "total_count"} {
		if _, ok := setFor(call, col); ok {
			t.Errorf("the blocked upsert updates %s — an attacker who keeps trying would extend their own lock", col)
		}
	}
}

// A success climbs its lifetime total and does not pretend to clear anything:
// the clearing is a separate statement against the FAILURE row, asserted below.
func TestSuccessUpsert_CountsLifetimeAndClearsNothingItself(t *testing.T) {
	_, calls := newAttemptStore(&recordingQuerier{})
	call := (*calls)[upsertSuccess]

	if s, ok := setFor(call, "total_count"); !ok || s.Mode != core.UpsertSetBump {
		t.Fatalf("total_count assignment = %#v (present %v), want a bump", s, ok)
	}
	if _, ok := setFor(call, "current_count"); ok {
		t.Error("the success upsert writes current_count; the lockout counter belongs to the failure row")
	}
}

// The nil existence answer gets its OWN statement rather than a bound NULL: a
// lookup that failed established nothing, and writing NULL would erase a verdict
// an earlier attempt earned.
func TestFailureUpsert_NilExistenceLeavesTheStoredVerdictAlone(t *testing.T) {
	_, calls := newAttemptStore(&recordingQuerier{})

	if _, ok := setFor((*calls)[upsertFailureKeepExisted], "identity_existed"); ok {
		t.Error("the keep-existing statement updates identity_existed; an unknown answer must not overwrite a known one")
	}
	if s, ok := setFor((*calls)[upsertFailure], "identity_existed"); !ok || s.Mode != core.UpsertSetNew {
		t.Errorf("the writing statement's identity_existed = %#v (present %v), want the proposed value", s, ok)
	}
}

// ── the two plain UPDATEs ───────────────────────────────────────────────────

// A failure ages out a stale window BEFORE it counts, so the next failure opens a
// new burst instead of climbing last week's count. The order is the point: run it
// after the upsert and the fresh failure is wiped along with the stale ones.
func TestRecordFailure_ExpiresAStaleWindowFirst(t *testing.T) {
	q := &recordingQuerier{}
	store, _ := newAttemptStore(q)

	if err := store.RecordFailure(context.Background(), "ada@acme.test", IdentityKindUser, "1.2.3.4", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.execs) != 2 {
		t.Fatalf("ran %d statements, want 2 (expire, then count)", len(q.execs))
	}
	if !strings.HasPrefix(q.execs[0], `UPDATE "authentication_attempts"`) {
		t.Fatalf("the first statement is not the window reset: %s", q.execs[0])
	}
	if !strings.HasPrefix(q.execs[1], "UPSERT INTO") {
		t.Fatalf("the second statement is not the count: %s", q.execs[1])
	}
	stmt := q.execs[0]
	if !strings.Contains(stmt, `"current_count" = 0`) || !strings.Contains(stmt, `"window_started_at" = NOW()`) {
		t.Errorf("the reset does not re-anchor the window:\n%s", stmt)
	}
	if !strings.Contains(stmt, `"window_started_at" <= $4`) {
		t.Errorf("the reset is not conditional on the window having aged out — it would zero a LIVE burst:\n%s", stmt)
	}
	// THE REGRESSION THAT MADE EVERY SIGNED-IN ACCOUNT UNLOCKABLE. A successful
	// sign-in leaves the anchor NULL, and `NULL <= x` is NULL rather than TRUE —
	// so without an explicit IS NULL branch this statement skips exactly the rows
	// that most need re-anchoring. Nothing else ever writes the column (it is
	// deliberately absent from the upsert's update set), so the counter climbs
	// forever against a NULL anchor and the identity never locks again.
	if !strings.Contains(stmt, `"window_started_at" IS NULL`) {
		t.Errorf("the reset does not re-anchor a NULL window; every identity that ever signed in "+
			"successfully would become permanently unlockable:\n%s", stmt)
	}

	args := q.execArgs[0]
	if args[2] != "failure" {
		t.Errorf("the reset targets outcome %v, want the failure row", args[2])
	}
	cutoff, ok := args[3].(time.Time)
	if !ok {
		t.Fatalf("window cutoff = %#v, want a time", args[3])
	}
	if elapsed := time.Since(cutoff); elapsed < LockoutWindow || elapsed > LockoutWindow+time.Minute {
		t.Errorf("cutoff is %v ago, want about %v", elapsed, LockoutWindow)
	}
}

// "A successful sign-in resets the counter" — literally, and against the FAILURE
// row. The clear goes first because it is the load-bearing half: a failure
// between the two statements must leave the user unlocked, never locked.
func TestRecordSuccess_ClearsTheFailureCounterFirstAndKeepsTheTotal(t *testing.T) {
	q := &recordingQuerier{}
	store, _ := newAttemptStore(q)

	if err := store.RecordSuccess(context.Background(), "ada@acme.test", IdentityKindUser, "1.2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.execs) != 2 {
		t.Fatalf("ran %d statements, want 2 (clear, then count)", len(q.execs))
	}
	stmt := q.execs[0]
	if !strings.Contains(stmt, `"current_count" = 0`) || !strings.Contains(stmt, `"window_started_at" = NULL`) {
		t.Errorf("the success clear does not release the lock:\n%s", stmt)
	}
	if strings.Contains(stmt, `"total_count"`) {
		t.Errorf("the success clear touches the lifetime total; a sign-in must not erase history:\n%s", stmt)
	}
	if q.execArgs[0][2] != "failure" {
		t.Errorf("the clear targets outcome %v, want the failure row", q.execArgs[0][2])
	}
}

// ── the lockout probe ───────────────────────────────────────────────────────

// One row, by the natural key. No window predicate, no ordering, no limit — the
// whole reason the rollup exists is that the question is now a point lookup.
func TestLockoutProbe_IsAPointLookupOnTheFailureRow(t *testing.T) {
	q := &recordingQuerier{}
	store, _ := newAttemptStore(q)

	if _, _, _, err := store.LockedUntil(context.Background(), "ada@acme.test", IdentityKindUser); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.queries) != 1 {
		t.Fatalf("expected one statement, got %d", len(q.queries))
	}
	stmt := q.queries[0]

	for _, want := range []string{`"identity" = $1`, `"identity_kind" = $2`, `"outcome" = $3`} {
		if !strings.Contains(stmt, want) {
			t.Errorf("the probe is missing %s — it would not address one row:\n%s", want, stmt)
		}
	}
	for _, unwanted := range []string{"ORDER BY", "LIMIT", "COALESCE", "MAX("} {
		if strings.Contains(stmt, unwanted) {
			t.Errorf("the probe still carries %q; it reads a single row now:\n%s", unwanted, stmt)
		}
	}
	args := q.queryArgs[0]
	if args[0] != "ada@acme.test" || args[1] != IdentityKindUser || args[2] != "failure" {
		t.Errorf("probe args = %v, want the identity, its kind and the failure outcome", args)
	}
}

// probeRow builds the single row the probe scans: the live count, the window
// anchor, and what the failures established about the identity.
func probeRow(current int, anchorAgo time.Duration, existed *bool) [][]any {
	at := time.Now().UTC().Add(-anchorAgo)
	return [][]any{{current, &at, existed}}
}

func TestLockedUntil_BelowTheThresholdIsNotLocked(t *testing.T) {
	q := &recordingQuerier{rows: probeRow(LockoutThreshold-1, time.Minute, nil)}
	store, _ := newAttemptStore(q)

	_, locked, _, err := store.LockedUntil(context.Background(), "ada@acme.test", IdentityKindUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locked {
		t.Errorf("locked on %d failures, want the threshold to be %d", LockoutThreshold-1, LockoutThreshold)
	}
}

// THE EXPIRY IS DERIVED FROM THE ANCHOR, never stored: the lock ends when the
// window opened plus the window, which is the same moment the counter stops
// applying. Nothing has to be cleared and nothing can drift.
func TestLockedUntil_ExpiryComesFromTheWindowAnchor(t *testing.T) {
	q := &recordingQuerier{rows: probeRow(LockoutThreshold, 10*time.Minute, nil)}
	store, _ := newAttemptStore(q)

	until, locked, _, err := store.LockedUntil(context.Background(), "ada@acme.test", IdentityKindUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Fatal("the threshold inside a live window did not lock")
	}
	remaining := time.Until(until)
	if remaining < 4*time.Minute || remaining > 6*time.Minute {
		t.Errorf("remaining = %v, want about 5 minutes (anchor %v ago, window %v)",
			remaining, 10*time.Minute, LockoutWindow)
	}
}

// A count still at the threshold whose window has aged out is NOT locked. The
// counter is not cleared eagerly — the next failure re-anchors it — so the
// decision has to be the one that reads the clock.
func TestLockedUntil_AgedOutWindowIsNotLocked(t *testing.T) {
	q := &recordingQuerier{rows: probeRow(LockoutThreshold+3, LockoutWindow+time.Minute, nil)}
	store, _ := newAttemptStore(q)

	if _, locked, _, err := store.LockedUntil(context.Background(), "x", IdentityKindUser); err != nil || locked {
		t.Errorf("locked = %v (err %v), want an aged-out window to release", locked, err)
	}
}

// An identity nobody has ever failed against has no row at all. Not locked, and
// nothing established about it.
func TestLockedUntil_NoRowIsNotLocked(t *testing.T) {
	store, _ := newAttemptStore(&recordingQuerier{})

	until, locked, existed, err := store.LockedUntil(context.Background(), "nobody@acme.test", IdentityKindUser)
	if err != nil || locked || existed != nil || !until.IsZero() {
		t.Errorf("got (%v, %v, %v, %v), want a clean not-locked answer", until, locked, existed, err)
	}
}

// THE REASON THE COLUMN EXISTS, asserted at this layer: the probe hands back what
// the failures established, so the record the caller publishes about a blocked
// attempt can say whether a REAL account is the one under attack — the difference
// between a targeted attack and credential-stuffing noise.
func TestLockedUntil_CarriesBackWhatTheFailuresEstablished(t *testing.T) {
	yes := true
	q := &recordingQuerier{rows: probeRow(LockoutThreshold, time.Minute, &yes)}
	store, _ := newAttemptStore(q)

	_, locked, existed, err := store.LockedUntil(context.Background(), "ada@acme.test", IdentityKindUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Fatal("the threshold inside a live window did not lock")
	}
	if existed == nil || !*existed {
		t.Fatalf("existence = %v, want true — without it a lock cannot say whether a real account is under attack", existed)
	}
}

// Nothing established, nothing claimed.
func TestLockedUntil_ClaimsNothingWhenNothingWasEstablished(t *testing.T) {
	q := &recordingQuerier{rows: probeRow(LockoutThreshold, time.Minute, nil)}
	store, _ := newAttemptStore(q)

	if _, _, existed, err := store.LockedUntil(context.Background(), "x", IdentityKindUser); err != nil || existed != nil {
		t.Errorf("existence = %v (err %v), want nil", existed, err)
	}
}

func TestLockedUntil_QueryFailureSurfaces(t *testing.T) {
	q := &recordingQuerier{queryErr: errors.New("connection reset")}
	store, _ := newAttemptStore(q)

	if _, _, _, err := store.LockedUntil(context.Background(), "x", IdentityKindUser); err == nil {
		t.Error("a probe failure must surface — answering 'not locked' would silently disable the lockout")
	}
}

// ── the rules that hold across every write ──────────────────────────────────

// A recording failure propagates on EITHER statement. A lockout that stops
// counting in silence is worse than one that was never built: nobody would know.
func TestRecord_FailurePropagates(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*AuthenticationAttemptStore) error
	}{
		{"failure", func(s *AuthenticationAttemptStore) error {
			return s.RecordFailure(context.Background(), "a@b.test", IdentityKindUser, "", nil)
		}},
		{"success", func(s *AuthenticationAttemptStore) error {
			return s.RecordSuccess(context.Background(), "a@b.test", IdentityKindUser, "")
		}},
		{"blocked", func(s *AuthenticationAttemptStore) error {
			return s.RecordLocked(context.Background(), "a@b.test", IdentityKindUser, "")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &recordingQuerier{execErr: errors.New("disk full")}
			store, _ := newAttemptStore(q)
			if err := tc.call(store); err == nil {
				t.Error("a recording failure must propagate")
			}
		})
	}
}

// THE CEILING, asserted as a fact about the code rather than only about the
// schema: no statement this store issues names any outcome but the two the table
// accepts. A third value would mean a third row per identity.
func TestWrites_NameOnlyTheTwoOutcomes(t *testing.T) {
	q := &recordingQuerier{}
	store, _ := newAttemptStore(q)
	ctx := context.Background()

	if err := store.RecordFailure(ctx, "a@b.test", IdentityKindUser, "1.2.3.4", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.RecordSuccess(ctx, "a@b.test", IdentityKindUser, "1.2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.RecordLocked(ctx, "a@b.test", IdentityKindUser, "1.2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, _, err := store.LockedUntil(ctx, "a@b.test", IdentityKindUser); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	seen := map[string]bool{}
	for _, args := range append(append([][]any{}, q.execArgs...), q.queryArgs...) {
		for _, a := range args {
			if s, ok := a.(string); ok && (s == "failure" || s == "success" || s == "locked") {
				seen[s] = true
			}
		}
	}
	if seen["locked"] {
		t.Error("a statement still binds the outcome 'locked'; the table's CHECK would reject the row")
	}
	if !seen["failure"] || !seen["success"] {
		t.Errorf("outcomes bound = %v, want both failure and success", seen)
	}
}

// THE RULE THAT HAS NO CODE, asserted anyway: nothing any write binds can carry a
// credential. If somebody ever adds a parameter that could, these counts change
// and this test says so.
func TestWrites_CannotCarryACredential(t *testing.T) {
	q := &recordingQuerier{}
	store, _ := newAttemptStore(q)
	ctx := context.Background()

	if err := store.RecordFailure(ctx, "a@b.test", IdentityKindUser, "1.2.3.4", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.RecordLocked(ctx, "a@b.test", IdentityKindUser, "1.2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The failure upsert: id, identity, kind, outcome, total, current,
	// window_started_at, last_at, last_ip, identity_existed — and no eleventh
	// column for anything that was presented.
	if got := len(q.execArgs[1]); got != 10 {
		t.Errorf("the failure upsert binds %d values, want 10; a new one must not be a credential", got)
	}
	// The blocked upsert: id, identity, kind, outcome, total_blocked, last_at,
	// last_ip.
	if got := len(q.execArgs[2]); got != 7 {
		t.Errorf("the blocked upsert binds %d values, want 7", got)
	}
	for stmtIdx, args := range q.execArgs {
		for i, arg := range args {
			if s, ok := arg.(string); ok && strings.Contains(strings.ToLower(s), "password") {
				t.Errorf("statement %d argument %d looks like a credential: %q", stmtIdx, i, s)
			}
		}
	}
}

// The SECOND statement of a two-statement write fails too. Reaching only the
// first would leave the branch that matters most untested: a window that was
// reset and a failure that never counted is a lockout quietly running one short.
func TestRecord_SecondStatementFailurePropagates(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*AuthenticationAttemptStore) error
	}{
		{"failure", func(s *AuthenticationAttemptStore) error {
			return s.RecordFailure(context.Background(), "a@b.test", IdentityKindUser, "", nil)
		}},
		{"success", func(s *AuthenticationAttemptStore) error {
			return s.RecordSuccess(context.Background(), "a@b.test", IdentityKindUser, "")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &recordingQuerier{execErr: errors.New("disk full"), execErrAfter: 1}
			store, _ := newAttemptStore(q)
			if err := tc.call(store); err == nil {
				t.Error("a failure on the counting statement must propagate")
			}
			if len(q.execs) != 2 {
				t.Errorf("ran %d statements, want both to have been attempted", len(q.execs))
			}
		})
	}
}

// A failure that carries a KNOWN existence verdict takes the other statement —
// the one that writes the column — and binds the verdict as the last argument.
func TestRecordFailure_KnownVerdictIsBound(t *testing.T) {
	for _, verdict := range []bool{true, false} {
		q := &recordingQuerier{}
		store, _ := newAttemptStore(q)
		if err := store.RecordFailure(context.Background(), "a@b.test", IdentityKindUser, "1.2.3.4", &verdict); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := q.execArgs[1][9]; got != verdict {
			t.Errorf("identity_existed = %#v, want %v", got, verdict)
		}
	}
}

// A scan that fails surfaces rather than answering "not locked". The probe is the
// only thing standing between an attacker and unlimited guesses; a driver problem
// must not read as an open door.
func TestLockedUntil_ScanFailureSurfaces(t *testing.T) {
	q := &recordingQuerier{
		rows:        probeRow(LockoutThreshold, time.Minute, nil),
		rowsScanErr: errors.New("bad column type"),
	}
	store, _ := newAttemptStore(q)

	if _, locked, _, err := store.LockedUntil(context.Background(), "x", IdentityKindUser); err == nil || locked {
		t.Errorf("locked = %v, err = %v — a scan failure must not read as 'not locked'", locked, err)
	}
}

// An iteration error surfaces for the same reason a scan error does: the honest
// answer to "I could not read the row" is not "nobody is locked".
func TestLockedUntil_IterationFailureSurfaces(t *testing.T) {
	q := &recordingQuerier{
		rows:        probeRow(LockoutThreshold, time.Minute, nil),
		rowsIterErr: errors.New("connection closed mid-read"),
	}
	store, _ := newAttemptStore(q)

	if _, locked, _, err := store.LockedUntil(context.Background(), "x", IdentityKindUser); err == nil || locked {
		t.Errorf("locked = %v, err = %v — an iteration failure must not read as 'not locked'", locked, err)
	}
}

// ── a recording seam ────────────────────────────────────────────────────────

// recordingQuerier captures every statement and argument list the store issues,
// which is what lets these tests assert the SHAPE of a write without a database.
//
// IT LIVES HERE BECAUSE THIS IS NOW ITS ONLY USER. The refresh token store shared
// it until it moved onto a DirectRepository and stopped writing statements at all;
// this store still renders its own — upserts with arithmetic in the conflict
// branch, which the Direct write verbs do not express — so asserting the rendered
// SQL is still asserting a decision this file makes.
type recordingQuerier struct {
	execs    []string
	execArgs [][]any
	execErr  error
	// execErrAfter lets N statements through before execErr starts applying, so a
	// test can fail the SECOND statement of a two-statement write. Zero — the
	// default — fails from the first, which is what every earlier test expects.
	execErrAfter int

	// rowsScanErr and rowsIterErr make the replayed result set fail mid-scan and
	// after iteration — the two branches a fixed set of rows cannot otherwise
	// reach.
	rowsScanErr error
	rowsIterErr error

	queried  []string
	scanErr  error
	scanFill func(dest ...any) error

	// The multi-row side, used by the attempt store's lockout probe. `rows` is
	// scanned one []any per row, in the order the statement would produce them.
	queries   []string
	queryArgs [][]any
	queryErr  error
	rows      [][]any
}

func (q *recordingQuerier) Query(_ context.Context, sql string, args ...any) (core.Rows, error) {
	q.queries = append(q.queries, sql)
	q.queryArgs = append(q.queryArgs, args)
	if q.queryErr != nil {
		return nil, q.queryErr
	}
	return &recordingCursor{rows: q.rows, scanErr: q.rowsScanErr, iterErr: q.rowsIterErr}, nil
}

// recordingCursor replays a fixed result set. Deliberately minimal: the tests that
// use it assert the DECISION taken over the rows, not the driver's behaviour.
type recordingCursor struct {
	rows    [][]any
	at      int
	scanErr error
	iterErr error
}

func (r *recordingCursor) Next() bool {
	if r.at >= len(r.rows) {
		return false
	}
	r.at++
	return true
}

func (r *recordingCursor) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.rows[r.at-1]
	for i := range dest {
		if i >= len(row) {
			break
		}
		switch target := dest[i].(type) {
		case *int:
			if v, ok := row[i].(int); ok {
				*target = v
			}
		case *time.Time:
			if v, ok := row[i].(time.Time); ok {
				*target = v
			}
		case **time.Time:
			if v, ok := row[i].(*time.Time); ok {
				*target = v
			}
		case **bool:
			if v, ok := row[i].(*bool); ok {
				*target = v
			}
		}
	}
	return nil
}

func (r *recordingCursor) Err() error   { return r.iterErr }
func (r *recordingCursor) Close() error { return nil }

func (q *recordingQuerier) QueryRow(_ context.Context, sql string, _ ...any) core.Row {
	q.queried = append(q.queried, sql)
	return &recordingRow{q: q}
}

func (q *recordingQuerier) QueryMaps(context.Context, string, ...any) ([]map[string]any, error) {
	panic("the refresh store never issues a dynamic-shape read")
}

// Exec is what core.Exec widens the querier to. Without it the store's writes
// would not reach this recorder at all.
func (q *recordingQuerier) Exec(_ context.Context, sql string, args ...any) error {
	q.execs = append(q.execs, sql)
	q.execArgs = append(q.execArgs, args)
	if len(q.execs) <= q.execErrAfter {
		return nil
	}
	return q.execErr
}

type recordingRow struct{ q *recordingQuerier }

func (r *recordingRow) Scan(dest ...any) error {
	if r.q.scanErr != nil {
		return r.q.scanErr
	}
	if r.q.scanFill != nil {
		return r.q.scanFill(dest...)
	}
	return nil
}

// fakeSeam is the whole dependency this store has — which is the point of the
// narrow SQLSeam it takes instead of the full engine: two methods to fake, not
// eleven.
type fakeSeam struct{ q *recordingQuerier }

func (s *fakeSeam) Querier() core.Querier { return s.q }
func (s *fakeSeam) Dialect() core.Dialect { return testDialect{} }
