// Tests for the attempt rollup.
//
// TWO THINGS ARE TESTED HERE, and they are the two this file decides. The
// POLICY — how many failures lock, for how long, from which moment — is arithmetic
// over one row, and every boundary of it can be stated directly. The VOCABULARY —
// which row each write addresses and which outcomes it may name — is what keeps a
// statement pointed at the failure row instead of the success one.
//
// EVERYTHING ELSE LIVES IN attempt_rollup_live_test.go, and the reason is worth
// stating so nobody brings it back. While this store rendered its own SQL there
// was something to assert without a database: the text of a statement was a
// decision this file made, and a recording dialect could catch an upsert that
// silently started updating the window anchor. The statements are the framework's
// now — one Direct schema, four calls through its own verbs — so a unit test could
// only fake the engine and check that RecordFailure calls Upsert, which is the
// implementation copied into the assertion. It would break when the code was
// changed correctly and pass when the database disagreed with the schema, which
// is exactly backwards.
//
// What is worth proving about a write is what the ROW looks like afterwards: that
// a burst of failures produces one row and not five, that hammering through a
// lock cannot extend it, that a success clears the counter and keeps the history,
// and that an unknown existence verdict does not erase a known one. Every one of
// those is a statement against a real database, and every one of them is there.

package infra

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// failureRow builds the row the lockout decision reads: the live count and the
// anchor the expiry is derived from.
func failureRow(current int64, anchorAgo time.Duration, now time.Time) schemas.AuthenticationAttempt {
	at := now.Add(-anchorAgo)
	return schemas.AuthenticationAttempt{CurrentCount: current, WindowStartedAt: &at}
}

// ── the policy ──────────────────────────────────────────────────────────────

func TestLockoutOf_BelowTheThresholdIsNotLocked(t *testing.T) {
	now := time.Now().UTC()

	if _, locked := lockoutOf(failureRow(LockoutThreshold-1, time.Minute, now), now); locked {
		t.Errorf("locked on %d failures, want the threshold to be %d", LockoutThreshold-1, LockoutThreshold)
	}
}

// THE EXPIRY IS DERIVED FROM THE ANCHOR, never stored: the lock ends when the
// window opened plus the window, which is the same moment the counter stops
// applying. Nothing has to be cleared and nothing can drift.
func TestLockoutOf_ExpiryComesFromTheWindowAnchor(t *testing.T) {
	now := time.Now().UTC()
	row := failureRow(LockoutThreshold, 10*time.Minute, now)

	until, locked := lockoutOf(row, now)
	if !locked {
		t.Fatal("the threshold inside a live window did not lock")
	}
	if want := row.WindowStartedAt.Add(LockoutWindow); !until.Equal(want) {
		t.Errorf("expiry = %v, want the anchor plus the window (%v)", until, want)
	}
}

// A count still at the threshold whose window has aged out is NOT locked. The
// counter is not cleared eagerly — the next failure re-anchors it — so the
// decision has to be the one that reads the clock.
func TestLockoutOf_AnAgedOutWindowReleases(t *testing.T) {
	now := time.Now().UTC()

	if _, locked := lockoutOf(failureRow(LockoutThreshold+3, LockoutWindow+time.Minute, now), now); locked {
		t.Error("an aged-out window still locked; the identity would never be released")
	}
}

// THE BOUNDARY, stated exactly. The window closes AT anchor+window: one
// nanosecond before, the identity is locked; at the instant itself it is free.
// A rule nobody checked at its boundary is a rule nobody checked.
func TestLockoutOf_TheWindowClosesAtTheAnchorPlusTheWindow(t *testing.T) {
	now := time.Now().UTC()
	row := failureRow(LockoutThreshold, LockoutWindow, now)
	expiry := row.WindowStartedAt.Add(LockoutWindow)

	if _, locked := lockoutOf(row, expiry.Add(-time.Nanosecond)); !locked {
		t.Error("released one nanosecond BEFORE the window closes")
	}
	if _, locked := lockoutOf(row, expiry); locked {
		t.Error("still locked AT the moment the window closes; the release must be exact")
	}
}

// A NULL anchor cannot produce an expiry, and inventing one would be worse than
// declining to lock. This is the guard behind the re-anchor, not a policy anyone
// should be able to reach.
func TestLockoutOf_NoAnchorCannotLock(t *testing.T) {
	now := time.Now().UTC()
	row := schemas.AuthenticationAttempt{CurrentCount: LockoutThreshold * 10}

	if until, locked := lockoutOf(row, now); locked || !until.IsZero() {
		t.Errorf("got (%v, %v) with no window anchor, want a clean not-locked answer", until, locked)
	}
}

// ── which row every write addresses ─────────────────────────────────────────

// EVERY STATEMENT ADDRESSES THE ROW BY THE NATURAL KEY. Anything else — the
// surrogate id above all — and each attempt touches a different row, which is the
// unbounded table this whole design exists to stop.
func TestNaturalKey_AddressesOneRowByItsThreeParts(t *testing.T) {
	expr, ok := naturalKey("ada@acme.test", IdentityKindUser, outcomeFailure).(criteria.Logical)
	if !ok {
		t.Fatalf("the natural key is %T, want a conjunction of its three parts", expr)
	}
	if expr.Op != criteria.LogicalAnd {
		t.Errorf("the three parts are combined with %v, want AND — an OR would match every "+
			"row of every identity", expr.Op)
	}

	got := map[string]any{}
	for _, operand := range expr.Operands {
		cmp, ok := operand.(criteria.Comparison)
		if !ok {
			t.Fatalf("operand %T is not a comparison", operand)
		}
		if cmp.Op != criteria.OpEq {
			t.Errorf("%q is compared with %v, want equality", cmp.Field, cmp.Op)
		}
		got[cmp.Field] = cmp.Values[0]
	}

	want := map[string]any{
		"Identity":     "ada@acme.test",
		"IdentityKind": IdentityKindUser,
		"Outcome":      outcomeFailure,
	}
	if len(got) != len(want) {
		t.Fatalf("the key names %d fields (%v), want exactly %d", len(got), got, len(want))
	}
	for field, value := range want {
		if got[field] != value {
			t.Errorf("%s = %v, want %v", field, got[field], value)
		}
	}
}

// THE FIELDS THE KEY NAMES HAVE TO RESOLVE. A criteria over a field the schema
// does not know is an error raised at the database, on the sign-in path — and the
// three names here are spelled in this file, not taken from the schema.
func TestNaturalKey_EveryFieldResolvesOnTheSchema(t *testing.T) {
	schema := schemas.AuthenticationAttemptSchema()

	expr := naturalKey("x", IdentityKindUser, outcomeFailure).(criteria.Logical)
	for _, operand := range expr.Operands {
		field := operand.(criteria.Comparison).Field
		if _, ok := schema.Resolve(field); !ok {
			t.Errorf("the natural key filters on %q, which does not resolve on %s",
				field, schema.Table())
		}
	}
}

// ── the ceiling ─────────────────────────────────────────────────────────────

// THE TWO OUTCOMES ARE THE WHOLE VOCABULARY, and the database enforces it with a
// CHECK. A third value written here would be refused by the constraint at the
// worst possible moment; a value the CHECK allows but nothing writes would be a
// row kind nobody counts. Asserting both directions keeps the pair honest.
func TestOutcomes_AreExactlyWhatTheTableAccepts(t *testing.T) {
	const migration = "../../migrations/postgres/0007_authentication_attempts_manual.up.sql"
	raw, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("reading %s: %v", migration, err)
	}
	// The comment lines are dropped: that file argues at length about the outcome
	// this table no longer has, and prose about a value is not the value.
	var body strings.Builder
	for line := range strings.SplitSeq(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		body.WriteString(line)
		body.WriteString("\n")
	}
	ddl := body.String()

	want := "CHECK (\"outcome\" IN ('" + outcomeFailure + "', '" + outcomeSuccess + "'))"
	if !strings.Contains(ddl, want) {
		t.Errorf("the table does not declare %s — the store writes outcomes the CHECK would reject, "+
			"or accepts one nothing writes", want)
	}

	// THERE IS NO `locked` OUTCOME, and there must not be one: a refusal made while
	// an identity is already locked bumps a counter on the failure row instead, which
	// is what keeps the lock from being extendable by whoever keeps trying.
	if strings.Contains(ddl, "'locked'") {
		t.Error("the table accepts a 'locked' outcome; a third row per identity would make the " +
			"lock extendable by continuing to try")
	}
}
