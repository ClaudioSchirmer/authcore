// Tests for the attempt log.
//
// The statements are read rather than executed, because what matters here cannot
// be seen by running one against clean data. A lockout query that silently lost
// its window predicate would keep working, keep passing an integration test, and
// lock people out on failures from last week. A query that lost the SUCCESS
// ANCHOR would lock a user who mistyped once after signing in fine. Both are
// invisible until somebody is locked out and nobody can say why.

package infra

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func newAttemptStore(q *recordingQuerier) *AuthenticationAttemptStore {
	return NewAuthenticationAttemptStore(&fakeSeam{q: q})
}

// ── the lockout query ───────────────────────────────────────────────────────

// THE ONE THAT MATTERS. Both predicates have to be there: the sliding window, and
// the anchor at the most recent success.
func TestLockoutQuery_CarriesTheWindowAndTheSuccessAnchor(t *testing.T) {
	q := &recordingQuerier{}
	store := newAttemptStore(q)

	if _, _, _, err := store.LockedUntil(context.Background(), "ada@acme.test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.queries) != 1 {
		t.Fatalf("expected one statement, got %d", len(q.queries))
	}
	stmt := q.queries[0]

	if !strings.Contains(stmt, `"identity" = $1`) {
		t.Errorf("the query is not scoped to one identity:\n%s", stmt)
	}
	// The window. Without it the count reaches back forever and a user who failed
	// five times last month can never sign in again.
	if !strings.Contains(stmt, `"occurred_at" > $3`) {
		t.Errorf("no sliding window in the query:\n%s", stmt)
	}
	// The anchor. Without it "a successful sign-in clears the counter" is false:
	// four old failures plus one new typo would lock a user who signed in fine in
	// between.
	if !strings.Contains(stmt, "COALESCE((SELECT MAX(") || !strings.Contains(stmt, `"outcome" = $5`) {
		t.Errorf("no success anchor in the query:\n%s", stmt)
	}
	// Newest first, because the CALLER takes the last of N as the oldest of the
	// batch and derives the expiry from it. Reverse the order and the lock lasts
	// as long as the newest failure allows, which is always too long.
	if !strings.Contains(stmt, `ORDER BY "occurred_at" DESC`) {
		t.Errorf("the ordering the expiry derivation depends on is missing:\n%s", stmt)
	}
	// Bounded at the threshold: five rows answer the question, and a locked
	// identity under attack could otherwise return thousands.
	if !strings.Contains(stmt, "LIMIT 5") {
		t.Errorf("the query is not bounded at the threshold:\n%s", stmt)
	}
}

// The outcomes bound into the query are the two the policy names, and they are
// bound as ARGUMENTS rather than inlined — so a value can never be spliced into
// the statement text.
func TestLockoutQuery_BindsTheTwoOutcomes(t *testing.T) {
	q := &recordingQuerier{}
	store := newAttemptStore(q)

	if _, _, _, err := store.LockedUntil(context.Background(), "ada@acme.test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	args := q.queryArgs[0]
	if args[0] != "ada@acme.test" || args[3] != "ada@acme.test" {
		t.Errorf("the identity must be bound to both branches, got %v", args)
	}
	if args[1] != "failure" {
		t.Errorf("counted outcome = %v, want failure — nothing else may count", args[1])
	}
	if args[4] != "success" {
		t.Errorf("anchor outcome = %v, want success", args[4])
	}
	windowStart, ok := args[2].(time.Time)
	if !ok {
		t.Fatalf("window start = %#v, want a time", args[2])
	}
	if elapsed := time.Since(windowStart); elapsed < LockoutWindow || elapsed > LockoutWindow+time.Minute {
		t.Errorf("window start is %v ago, want about %v", elapsed, LockoutWindow)
	}
}

// ── the decision ────────────────────────────────────────────────────────────

func TestLockedUntil_BelowTheThresholdIsNotLocked(t *testing.T) {
	q := &recordingQuerier{rows: timesAgo(time.Minute, 2*time.Minute, 3*time.Minute, 4*time.Minute)}
	store := newAttemptStore(q)

	_, locked, _, err := store.LockedUntil(context.Background(), "ada@acme.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locked {
		t.Errorf("locked on %d failures, want the threshold to be %d", 4, LockoutThreshold)
	}
}

// THE EXPIRY IS DERIVED FROM THE OLDEST OF THE FIVE, which is what makes the
// release exact: the lock ends at the moment that failure ages out of the window,
// the same moment the count drops back below the threshold.
func TestLockedUntil_ExpiryComesFromTheOldestCountedFailure(t *testing.T) {
	// Newest first, as the query orders them. The oldest is 10 minutes ago, so the
	// lock has 5 of the 15 minutes left.
	q := &recordingQuerier{rows: timesAgo(
		1*time.Minute, 3*time.Minute, 6*time.Minute, 8*time.Minute, 10*time.Minute,
	)}
	store := newAttemptStore(q)

	until, locked, _, err := store.LockedUntil(context.Background(), "ada@acme.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Fatal("five failures inside the window did not lock")
	}
	remaining := time.Until(until)
	if remaining < 4*time.Minute || remaining > 6*time.Minute {
		t.Errorf("remaining = %v, want about 5 minutes (the oldest failure was %v ago, window %v)",
			remaining, 10*time.Minute, LockoutWindow)
	}
}

// Five failures whose oldest has already aged past the window answer NOT locked.
// The window moved between the query and the decision; the honest answer is that
// the lock is over.
func TestLockedUntil_ExpiredWindowIsNotLocked(t *testing.T) {
	q := &recordingQuerier{rows: timesAgo(
		16*time.Minute, 17*time.Minute, 18*time.Minute, 19*time.Minute, 20*time.Minute,
	)}
	store := newAttemptStore(q)

	if _, locked, _, err := store.LockedUntil(context.Background(), "x"); err != nil || locked {
		t.Errorf("locked = %v (err %v), want an aged-out batch to release", locked, err)
	}
}

func TestLockedUntil_QueryFailureSurfaces(t *testing.T) {
	q := &recordingQuerier{queryErr: errors.New("connection reset")}
	store := newAttemptStore(q)

	if _, _, _, err := store.LockedUntil(context.Background(), "x"); err == nil {
		t.Error("a probe failure must surface — answering 'not locked' would silently disable the lockout")
	}
}

// ── the writes ──────────────────────────────────────────────────────────────

func TestRecord_EachOutcomeWritesItsOwnValueAndFlag(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name        string
		call        func(*AuthenticationAttemptStore) error
		wantOutcome string
		wantExisted any
	}{
		{"failure, identity known to exist",
			func(s *AuthenticationAttemptStore) error {
				return s.RecordFailure(context.Background(), "a@b.test", IdentityKindUser, "1.2.3.4", &yes)
			}, "failure", true},
		{"failure, identity known absent",
			func(s *AuthenticationAttemptStore) error {
				return s.RecordFailure(context.Background(), "a@b.test", IdentityKindUser, "1.2.3.4", &no)
			}, "failure", false},
		{"failure, existence unknown",
			func(s *AuthenticationAttemptStore) error {
				return s.RecordFailure(context.Background(), "a@b.test", IdentityKindUser, "1.2.3.4", nil)
			}, "failure", nil},
		{"success implies existence without being told",
			func(s *AuthenticationAttemptStore) error {
				return s.RecordSuccess(context.Background(), "a@b.test", IdentityKindUser, "1.2.3.4")
			}, "success", true},
		{"locked with nothing established stays unknown",
			func(s *AuthenticationAttemptStore) error {
				return s.RecordLocked(context.Background(), "a@b.test", IdentityKindUser, "1.2.3.4", nil)
			}, "locked", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &recordingQuerier{}
			if err := tc.call(newAttemptStore(q)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(q.execs) != 1 {
				t.Fatalf("expected one insert, got %d", len(q.execs))
			}
			if !strings.HasPrefix(q.execs[0], `INSERT INTO "authentication_attempts"`) {
				t.Errorf("unexpected statement: %s", q.execs[0])
			}
			args := q.execArgs[0]
			if args[3] != tc.wantOutcome {
				t.Errorf("outcome = %v, want %v", args[3], tc.wantOutcome)
			}
			if args[4] != tc.wantExisted {
				t.Errorf("identity_existed = %#v, want %#v", args[4], tc.wantExisted)
			}
		})
	}
}

// THE RULE THAT HAS NO TEST BECAUSE IT HAS NO CODE, asserted here anyway: nothing
// in the insert can carry a credential. If somebody ever adds a parameter that
// could, this count changes and this test says so.
func TestRecord_TheInsertCannotCarryACredential(t *testing.T) {
	q := &recordingQuerier{}
	if err := newAttemptStore(q).RecordFailure(
		context.Background(), "a@b.test", IdentityKindUser, "1.2.3.4", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// id, identity, kind, outcome, existed, ip, occurred_at — and no eighth column
	// for anything that was presented.
	if got := len(q.execArgs[0]); got != 7 {
		t.Fatalf("the insert binds %d values, want 7; a new one must not be a credential", got)
	}
	for i, arg := range q.execArgs[0] {
		if s, ok := arg.(string); ok && strings.Contains(strings.ToLower(s), "password") {
			t.Errorf("argument %d looks like a credential: %q", i, s)
		}
	}
}

func TestRecord_FailurePropagates(t *testing.T) {
	q := &recordingQuerier{execErr: errors.New("disk full")}
	err := newAttemptStore(q).RecordFailure(context.Background(), "a@b.test", IdentityKindUser, "", nil)
	if err == nil {
		t.Error("a recording failure must propagate — a lockout that stops counting in silence is worse than none")
	}
}

// timesAgo builds scan rows newest-first, the order the query produces. The
// second column is the existence flag the probe carries back; nil here means the
// row established nothing.
func timesAgo(ds ...time.Duration) [][]any {
	now := time.Now().UTC()
	out := make([][]any, 0, len(ds))
	for _, d := range ds {
		out = append(out, []any{now.Add(-d), (*bool)(nil)})
	}
	return out
}

// timesAgoExisting is timesAgo with every row asserting the identity was real —
// the shape a locked REAL account produces.
func timesAgoExisting(ds ...time.Duration) [][]any {
	yes := true
	out := timesAgo(ds...)
	for i := range out {
		out[i][1] = &yes
	}
	return out
}

// THE REASON THE COLUMN EXISTS, asserted end to end at this layer: the probe
// hands back what the counted failures established, so the `locked` row the
// caller writes can carry it — and somebody filtering `WHERE outcome = 'locked'`
// alone still sees which locks are real accounts under attack.
func TestLockedUntil_CarriesBackWhatTheFailuresEstablished(t *testing.T) {
	q := &recordingQuerier{rows: timesAgoExisting(
		1*time.Minute, 3*time.Minute, 6*time.Minute, 8*time.Minute, 10*time.Minute,
	)}
	store := newAttemptStore(q)

	_, locked, existed, err := store.LockedUntil(context.Background(), "ada@acme.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Fatal("five failures inside the window did not lock")
	}
	if existed == nil || !*existed {
		t.Fatalf("existence = %v, want true — without it a locked row cannot say whether a real account is under attack", existed)
	}
}

// Nothing established, nothing claimed. An identity whose failures all landed
// during an outage carries no verdict, and the probe must not invent one.
func TestLockedUntil_ClaimsNothingWhenNothingWasEstablished(t *testing.T) {
	q := &recordingQuerier{rows: timesAgo(
		1*time.Minute, 3*time.Minute, 6*time.Minute, 8*time.Minute, 10*time.Minute,
	)}
	if _, _, existed, err := newAttemptStore(q).LockedUntil(context.Background(), "x"); err != nil || existed != nil {
		t.Errorf("existence = %v (err %v), want nil", existed, err)
	}
}
