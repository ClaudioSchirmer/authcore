//go:build integration && postgres

// Drives the REAL store against the REAL Postgres dev bench, through the real
// engine — so the upserts are rendered by the framework's own pg dialect rather
// than by any stub. This is the only place the statements are executed instead of
// inspected, and it is what proves the schema and the arithmetic agree.

package infra

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/engine/postgres"
)

const liveIdentity = "rollup-proof@acme.test"

type rollupRow struct {
	total, current, blocked int64
	windowStart             *time.Time
	lastAt                  time.Time
	lastIP                  string
	existed                 *bool
}

func liveStore(t *testing.T) (*AuthenticationAttemptStore, *postgres.Postgres) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://omnicore:omnicore@localhost:5432/authcore_db?sslmode=disable"
	}
	eng, err := postgres.NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Skipf("no dev bench reachable: %v", err)
	}
	// THE PRODUCTION CLOCK, because the rollup now depends on it. Both instants on
	// this row — the window anchor and the last attempt — are stamped columns, so
	// their value is read from the database once per write transaction rather than
	// from whichever pod served the request. Testing under the process clock would
	// prove the arithmetic and leave the configuration every profile declares
	// (`relational.clock: db`) unexercised, which is the half a drifting replica
	// would break.
	eng.SetClock(core.ClockDB)
	t.Cleanup(func() {
		_, _ = eng.Pool().Exec(context.Background(),
			`DELETE FROM authentication_attempts WHERE identity = $1`, liveIdentity)
		eng.Close()
	})
	_, err = eng.Pool().Exec(context.Background(),
		`DELETE FROM authentication_attempts WHERE identity = $1`, liveIdentity)
	if err != nil {
		t.Fatalf("clearing: %v", err)
	}
	return NewAuthenticationAttemptStore(eng), eng
}

func readRow(t *testing.T, eng *postgres.Postgres, outcome string) rollupRow {
	t.Helper()
	var r rollupRow
	err := eng.Pool().QueryRow(context.Background(),
		`SELECT total_count, current_count, total_blocked, window_started_at, last_at, last_ip, identity_existed
		   FROM authentication_attempts WHERE identity = $1 AND identity_kind = $2 AND outcome = $3`,
		liveIdentity, IdentityKindUser, outcome,
	).Scan(&r.total, &r.current, &r.blocked, &r.windowStart, &r.lastAt, &r.lastIP, &r.existed)
	if err != nil {
		t.Fatalf("reading the %s row: %v", outcome, err)
	}
	return r
}

func rowCount(t *testing.T, eng *postgres.Postgres) int {
	t.Helper()
	var n int
	if err := eng.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM authentication_attempts WHERE identity = $1`, liveIdentity).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return n
}

// THE WHOLE STORY, in the order it happens to a real identity under attack.
func TestLive_RollupHoldsTheLockoutAndTheCeiling(t *testing.T) {
	store, eng := liveStore(t)
	ctx := context.Background()
	yes := true

	// ── four failures ────────────────────────────────────────────────────────
	for i := 0; i < 4; i++ {
		if err := store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "203.0.113.7", &yes); err != nil {
			t.Fatalf("failure %d: %v", i+1, err)
		}
	}
	if n := rowCount(t, eng); n != 1 {
		t.Fatalf("four failures produced %d rows, want 1 — the whole point of the rollup", n)
	}
	row := readRow(t, eng, "failure")
	if row.total != 4 || row.current != 4 {
		t.Errorf("after 4 failures: total=%d current=%d, want 4/4", row.total, row.current)
	}
	if row.existed == nil || !*row.existed {
		t.Errorf("identity_existed = %v, want true", row.existed)
	}
	if row.lastIP != "203.0.113.7" {
		t.Errorf("last_ip = %q", row.lastIP)
	}
	if _, locked, _, err := store.LockedUntil(ctx, liveIdentity, IdentityKindUser); err != nil || locked {
		t.Errorf("locked = %v (err %v) below the threshold", locked, err)
	}
	anchor := *row.windowStart

	// ── the fifth locks ──────────────────────────────────────────────────────
	if err := store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "203.0.113.7", &yes); err != nil {
		t.Fatalf("fifth failure: %v", err)
	}
	until, locked, existed, err := store.LockedUntil(ctx, liveIdentity, IdentityKindUser)
	if err != nil || !locked {
		t.Fatalf("the fifth failure did not lock (err %v)", err)
	}
	if existed == nil || !*existed {
		t.Errorf("the probe lost the existence verdict: %v", existed)
	}
	// The expiry is the anchor plus the window, derived and never stored.
	if drift := until.Sub(anchor.Add(LockoutWindow)); drift > time.Second || drift < -time.Second {
		t.Errorf("expiry drifted %v from anchor+window", drift)
	}

	// ── hammering through the lock must NOT extend it ────────────────────────
	locked5 := readRow(t, eng, "failure")
	for i := 0; i < 3; i++ {
		if err := store.RecordLocked(ctx, liveIdentity, IdentityKindUser, "198.51.100.9"); err != nil {
			t.Fatalf("blocked %d: %v", i+1, err)
		}
	}
	after := readRow(t, eng, "failure")
	if after.blocked != 3 {
		t.Errorf("total_blocked = %d, want 3", after.blocked)
	}
	if after.current != locked5.current {
		t.Errorf("current_count moved from %d to %d during a lock", locked5.current, after.current)
	}
	if !after.windowStart.Equal(*locked5.windowStart) {
		t.Errorf("THE LOCK WAS EXTENDED: anchor moved from %v to %v", *locked5.windowStart, *after.windowStart)
	}
	if after.total != locked5.total {
		t.Errorf("total_count moved from %d to %d during a lock", locked5.total, after.total)
	}
	if n := rowCount(t, eng); n != 1 {
		t.Fatalf("blocked attempts produced %d rows, want 1", n)
	}

	// ── a nil verdict must not erase the one on file ─────────────────────────
	if err := store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "203.0.113.7", nil); err != nil {
		t.Fatalf("failure with unknown existence: %v", err)
	}
	if r := readRow(t, eng, "failure"); r.existed == nil || !*r.existed {
		t.Errorf("identity_existed = %v after an unknown answer, want the stored true to survive", r.existed)
	}

	// ── success clears the counter and keeps the history ─────────────────────
	beforeSuccess := readRow(t, eng, "failure")
	if err := store.RecordSuccess(ctx, liveIdentity, IdentityKindUser, "203.0.113.7"); err != nil {
		t.Fatalf("success: %v", err)
	}
	cleared := readRow(t, eng, "failure")
	if cleared.current != 0 {
		t.Errorf("current_count = %d after a successful sign-in, want 0", cleared.current)
	}
	if cleared.total != beforeSuccess.total {
		t.Errorf("total_count went %d → %d; a sign-in must not erase history",
			beforeSuccess.total, cleared.total)
	}
	if cleared.windowStart != nil {
		t.Errorf("window_started_at = %v after a success, want NULL", *cleared.windowStart)
	}
	if _, locked, _, err := store.LockedUntil(ctx, liveIdentity, IdentityKindUser); err != nil || locked {
		t.Errorf("still locked after a successful sign-in (err %v)", err)
	}

	success := readRow(t, eng, "success")
	if success.total != 1 {
		t.Errorf("lifetime successes = %d, want 1", success.total)
	}
	if success.lastAt.IsZero() {
		t.Error("the success row carries no last sign-in date")
	}
	if success.current != 0 || success.windowStart != nil || success.blocked != 0 {
		t.Errorf("the success row carries lockout machinery: current=%d window=%v blocked=%d",
			success.current, success.windowStart, success.blocked)
	}

	// ── THE CEILING ──────────────────────────────────────────────────────────
	if n := rowCount(t, eng); n != 2 {
		t.Fatalf("the identity holds %d rows after the whole sequence, want exactly 2", n)
	}
}

// A window that has aged out restarts the count at one instead of climbing the
// old burst — the branch that decides whether a lock ever releases.
func TestLive_AnAgedOutWindowRestartsTheCount(t *testing.T) {
	store, eng := liveStore(t)
	ctx := context.Background()
	yes := true

	for i := 0; i < LockoutThreshold; i++ {
		if err := store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "", &yes); err != nil {
			t.Fatalf("failure %d: %v", i+1, err)
		}
	}
	if _, locked, _, _ := store.LockedUntil(ctx, liveIdentity, IdentityKindUser); !locked {
		t.Fatal("the threshold did not lock")
	}

	// Age the anchor past the window — what the clock would do on its own.
	if _, err := eng.Pool().Exec(ctx,
		`UPDATE authentication_attempts SET window_started_at = window_started_at - $1::interval
		  WHERE identity = $2 AND outcome = 'failure'`,
		"20 minutes", liveIdentity); err != nil {
		t.Fatalf("ageing the window: %v", err)
	}

	if _, locked, _, err := store.LockedUntil(ctx, liveIdentity, IdentityKindUser); err != nil || locked {
		t.Fatalf("an aged-out window still reads as locked (err %v)", err)
	}
	if err := store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "", &yes); err != nil {
		t.Fatalf("failure after the window aged out: %v", err)
	}
	row := readRow(t, eng, "failure")
	if row.current != 1 {
		t.Errorf("current_count = %d after a stale window, want 1 — the burst must start over", row.current)
	}
	if row.total != LockoutThreshold+1 {
		t.Errorf("total_count = %d, want %d — the lifetime total never resets", row.total, LockoutThreshold+1)
	}
	if _, locked, _, _ := store.LockedUntil(ctx, liveIdentity, IdentityKindUser); locked {
		t.Error("one failure in a fresh window locked the identity")
	}
}

// THE ORDER THAT WAS NEVER EXERCISED, and the one a real account actually lives
// in: sign in successfully FIRST, then start failing.
//
// A success clears the counter and NULLs the window anchor. If nothing re-anchors
// it, the failure counter climbs against a NULL anchor forever, no expiry can be
// derived, and the identity never locks — with the count visibly rising in the
// table the whole time, which is what makes the failure so quiet. Every account
// that had ever signed in would be exempt from the lockout.
func TestLive_AnIdentityThatSignedInBeforeStillLocks(t *testing.T) {
	store, eng := liveStore(t)
	ctx := context.Background()
	yes := true

	// One failure first, so the failure ROW exists — a success alone would only
	// write the success row and the next failure would insert a fresh one with a
	// perfectly good anchor. The trap needs an EXISTING failure row whose anchor a
	// success has NULLed, which is the ordinary life of any real account: mistype
	// once, sign in, come back later and start failing.
	if err := store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "203.0.113.7", &yes); err != nil {
		t.Fatalf("priming failure: %v", err)
	}
	if err := store.RecordSuccess(ctx, liveIdentity, IdentityKindUser, "203.0.113.7"); err != nil {
		t.Fatalf("success: %v", err)
	}
	if row := readRow(t, eng, "failure"); row.windowStart != nil {
		t.Fatalf("a success left the anchor at %v, want NULL — the precondition this test exists for",
			*row.windowStart)
	}

	for i := 0; i < LockoutThreshold; i++ {
		if err := store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "203.0.113.7", &yes); err != nil {
			t.Fatalf("failure %d: %v", i+1, err)
		}
	}

	row := readRow(t, eng, "failure")
	if row.windowStart == nil {
		t.Fatal("the anchor is still NULL after five failures — no expiry can be derived and the identity can never lock")
	}
	if row.current != LockoutThreshold {
		t.Errorf("current_count = %d, want %d — the burst after the success must be counted on its own",
			row.current, LockoutThreshold)
	}
	if row.total != LockoutThreshold+1 {
		t.Errorf("total_count = %d, want %d — the priming failure is history and must survive the success",
			row.total, LockoutThreshold+1)
	}
	until, locked, _, err := store.LockedUntil(ctx, liveIdentity, IdentityKindUser)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !locked {
		t.Fatal("an identity that had signed in successfully did not lock after the threshold")
	}
	if remaining := time.Until(until); remaining <= 0 || remaining > LockoutWindow {
		t.Errorf("remaining = %v, want a live window", remaining)
	}
}

// The same trap one layer along: a success does not merely fail to block the NEXT
// lock, it must not shorten it either. The burst that follows gets a FULL window,
// anchored at its own first failure rather than at anything the success left.
func TestLive_TheBurstAfterASuccessGetsAFullWindow(t *testing.T) {
	store, eng := liveStore(t)
	ctx := context.Background()
	yes := true

	if err := store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "", &yes); err != nil {
		t.Fatalf("priming failure: %v", err)
	}
	if err := store.RecordSuccess(ctx, liveIdentity, IdentityKindUser, ""); err != nil {
		t.Fatalf("success: %v", err)
	}
	before := time.Now().UTC()
	for i := 0; i < LockoutThreshold; i++ {
		if err := store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "", &yes); err != nil {
			t.Fatalf("failure %d: %v", i+1, err)
		}
	}
	row := readRow(t, eng, "failure")
	if row.windowStart.Before(before.Add(-time.Second)) {
		t.Errorf("the anchor is %v, older than the burst that opened it (%v) — the lock would expire early",
			*row.windowStart, before)
	}
}

// EVERY WRITE AND THE PROBE SURFACE A DATABASE FAILURE, none of them swallows it.
//
// A lockout that stops counting in silence is worse than one that was never
// built: nobody would know. And a probe that answered "not locked" because it
// could not read would hand an attacker unlimited guesses against every account
// in the service, for as long as the trouble lasted.
//
// The store is pointed at a schema that holds no tables, so every statement it
// issues fails at the database for a reason it cannot anticipate — which is the
// only honest way to reach these branches now that the statements are the
// framework's.
func TestLive_EveryWriteSurfacesADatabaseFailure(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://omnicore:omnicore@localhost:5432/authcore_db?sslmode=disable"
	}
	sep := "&"
	if !strings.Contains(dsn, "?") {
		sep = "?"
	}
	eng, err := postgres.NewPostgres(context.Background(), dsn+sep+"search_path=authcore_no_such_schema")
	if err != nil {
		t.Skipf("no dev bench reachable: %v", err)
	}
	t.Cleanup(eng.Close)

	store := NewAuthenticationAttemptStore(eng)
	ctx := context.Background()
	yes := true

	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"failure", func() error {
			return store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "203.0.113.7", &yes)
		}},
		{"failure with an unknown verdict", func() error {
			return store.RecordFailure(ctx, liveIdentity, IdentityKindUser, "203.0.113.7", nil)
		}},
		{"success", func() error {
			return store.RecordSuccess(ctx, liveIdentity, IdentityKindUser, "203.0.113.7")
		}},
		{"blocked", func() error {
			return store.RecordLocked(ctx, liveIdentity, IdentityKindUser, "203.0.113.7")
		}},
		{"probe", func() error {
			_, locked, _, err := store.LockedUntil(ctx, liveIdentity, IdentityKindUser)
			if locked {
				t.Error("a probe that could not read reported a LOCK; the answer has to be an error")
			}
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Error("the failure was swallowed")
			}
		})
	}
}
