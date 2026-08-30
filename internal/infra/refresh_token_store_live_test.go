//go:build integration && postgres

// Drives the REAL store against the REAL Postgres dev bench, through the real
// engine — so every statement is rendered by the framework's own pg dialect and
// executed, rather than inspected as a string.
//
// THIS IS WHAT REPLACED THE STATEMENT ASSERTIONS. While the store wrote its own
// SQL, the unit suite could assert the text of it and be asserting a decision this
// file made. The statements are the framework's now, so the only honest proof that
// they are the right ones is running them: the columns resolve, the hash lookup
// finds its row, the family update reaches every descendant, and the sweep removes
// what expired and nothing else.
//
// The rows it writes are namespaced by a family nobody else uses and removed on
// cleanup, so it can run against a bench with real data in it.

package infra

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/omnicore/infra/db/engine/postgres"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

const liveFamily = "live-refresh-proof"

func liveRefreshStore(t *testing.T) (*RefreshTokenStore, *postgres.Postgres) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://omnicore:omnicore@localhost:5432/authcore_db?sslmode=disable"
	}
	eng, err := postgres.NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Skipf("no dev bench reachable: %v", err)
	}
	clear := func() {
		_, _ = eng.Pool().Exec(context.Background(),
			`DELETE FROM authentication_refresh_tokens WHERE family_id = $1`, liveFamily)
	}
	clear()
	t.Cleanup(func() { clear(); eng.Close() })
	return NewRefreshTokenStore(eng, slog.Default()), eng
}

func liveRecord(hash string, expiresAt time.Time) authcore.RefreshTokenRecord {
	return authcore.RefreshTokenRecord{
		Hash:      hash,
		FamilyID:  liveFamily,
		Subject:   "user-live",
		Audience:  []string{"authcore", "users-api"},
		ExpiresAt: expiresAt,
	}
}

// A row saved is a row found, field for field. This is the assertion that every
// column in the schema resolves to the one the migration created — a mismatch
// would otherwise surface as a token that saves fine and can never be redeemed.
func TestLive_SaveThenLookupRoundTrips(t *testing.T) {
	store, _ := liveRefreshStore(t)
	ctx := context.Background()
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)

	if err := store.Save(ctx, liveRecord("live-roundtrip-hash", expires)); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Lookup(ctx, "live-roundtrip-hash")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Subject != "user-live" || got.FamilyID != liveFamily {
		t.Errorf("record came back as %+v", got)
	}
	if len(got.Audience) != 2 || got.Audience[0] != "authcore" {
		t.Errorf("audience = %v, want both entries in order", got.Audience)
	}
	if got.Used || got.Revoked {
		t.Errorf("a fresh token came back used=%v revoked=%v", got.Used, got.Revoked)
	}
	if !got.ExpiresAt.Equal(expires) {
		t.Errorf("expiry = %v, want %v — a timestamp that does not round-trip means the "+
			"sweep and the Issuer disagree about when a token dies", got.ExpiresAt, expires)
	}
}

// An unknown hash is the port's sentinel, not an error and not a zero record the
// caller might mistake for a session.
func TestLive_LookupMissIsTheSentinel(t *testing.T) {
	store, _ := liveRefreshStore(t)

	if _, err := store.Lookup(context.Background(), "live-no-such-hash"); err != authcore.ErrRefreshTokenNotFound {
		t.Errorf("err = %v, want ErrRefreshTokenNotFound", err)
	}
}

// MarkUsed burns exactly the row named and leaves its siblings alone; RevokeFamily
// does the opposite. Proving both against the same two rows is what catches a
// predicate keyed on the wrong column — the two would still compile and each
// would look right on its own.
func TestLive_MarkUsedBurnsOneRowAndRevokeFamilyKillsAll(t *testing.T) {
	store, _ := liveRefreshStore(t)
	ctx := context.Background()
	expires := time.Now().UTC().Add(time.Hour)

	if err := store.Save(ctx, liveRecord("live-burn-a", expires)); err != nil {
		t.Fatalf("save a: %v", err)
	}
	if err := store.Save(ctx, liveRecord("live-burn-b", expires)); err != nil {
		t.Fatalf("save b: %v", err)
	}

	if err := store.MarkUsed(ctx, "live-burn-a"); err != nil {
		t.Fatalf("mark used: %v", err)
	}
	a, _ := store.Lookup(ctx, "live-burn-a")
	b, _ := store.Lookup(ctx, "live-burn-b")
	if !a.Used {
		t.Error("the burned token did not come back used")
	}
	if b.Used {
		t.Error("MarkUsed burned a sibling — the predicate is not keyed on the hash")
	}

	if err := store.RevokeFamily(ctx, liveFamily); err != nil {
		t.Fatalf("revoke family: %v", err)
	}
	a, _ = store.Lookup(ctx, "live-burn-a")
	b, _ = store.Lookup(ctx, "live-burn-b")
	if !a.Revoked || !b.Revoked {
		t.Errorf("revoking a family left survivors: a=%v b=%v", a.Revoked, b.Revoked)
	}
}

// THE SWEEP, end to end: it removes what is past expiry AND the grace margin, and
// leaves everything else — including a token that expired seconds ago, which must
// still be found so the refusal says "expired" rather than "unknown".
func TestLive_SweepRemovesOnlyTheLongExpired(t *testing.T) {
	store, _ := liveRefreshStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.Save(ctx, liveRecord("live-sweep-dead", now.Add(-2*sweepGrace))); err != nil {
		t.Fatalf("save dead: %v", err)
	}
	if err := store.Save(ctx, liveRecord("live-sweep-justexpired", now.Add(-time.Minute))); err != nil {
		t.Fatalf("save just-expired: %v", err)
	}
	// Saving this one runs the sweep that judges the two above.
	if err := store.Save(ctx, liveRecord("live-sweep-alive", now.Add(time.Hour))); err != nil {
		t.Fatalf("save alive: %v", err)
	}

	if _, err := store.Lookup(ctx, "live-sweep-dead"); err != authcore.ErrRefreshTokenNotFound {
		t.Errorf("the long-expired row survived the sweep: %v", err)
	}
	if _, err := store.Lookup(ctx, "live-sweep-justexpired"); err != nil {
		t.Errorf("a token inside the grace margin was swept: %v — the refusal would change "+
			"from \"expired\" to \"unknown\" and the log line would lie", err)
	}
	if _, err := store.Lookup(ctx, "live-sweep-alive"); err != nil {
		t.Errorf("the live token was swept: %v", err)
	}
}

// AN UNREADABLE AUDIENCE MUST NOT LOCK A USER OUT. The column is written only by
// Save, so a value that will not decode means the row was tampered with or
// predates a format change — and refusing a whole redemption over the AUDIENCE
// list would turn a formatting problem into a lost session the Issuer would have
// defaulted past anyway.
//
// The bad value is written straight through the pool: it is a state Save cannot
// produce, which is the point of testing it.
func TestLive_UnreadableAudienceDegradesToEmpty(t *testing.T) {
	store, eng := liveRefreshStore(t)
	ctx := context.Background()

	if err := store.Save(ctx, liveRecord("live-bad-audience", time.Now().UTC().Add(time.Hour))); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := eng.Pool().Exec(ctx,
		`UPDATE authentication_refresh_tokens SET audience = $1 WHERE hash = $2`,
		"not json at all", "live-bad-audience"); err != nil {
		t.Fatalf("corrupting the audience column: %v", err)
	}

	rec, err := store.Lookup(ctx, "live-bad-audience")
	if err != nil {
		t.Fatalf("a bad audience column failed the lookup: %v", err)
	}
	if rec.Audience != nil {
		t.Errorf("audience = %v, want nil", rec.Audience)
	}
	if rec.Subject != "user-live" {
		t.Errorf("the rest of the record was lost: %+v", rec)
	}
}

// SubjectForRefreshToken hashes the RAW value and answers the session's owner.
//
// The empty case asks the database nothing — there is no value to hash — and a
// miss answers "" rather than an error, because an unknown token is an ordinary
// refusal the handler declines without needing to know this store distinguishes
// the two.
func TestLive_SubjectForRefreshTokenResolvesAndMissesQuietly(t *testing.T) {
	store, _ := liveRefreshStore(t)
	ctx := context.Background()

	const raw = "a-refresh-value-nobody-else-uses"
	sum := sha256.Sum256([]byte(raw))
	if err := store.Save(ctx, liveRecord(hex.EncodeToString(sum[:]), time.Now().UTC().Add(time.Hour))); err != nil {
		t.Fatalf("save: %v", err)
	}

	subject, err := store.SubjectForRefreshToken(ctx, raw)
	if err != nil {
		t.Fatalf("resolving a live value: %v", err)
	}
	if subject != "user-live" {
		t.Errorf("subject = %q, want user-live", subject)
	}

	if subject, err := store.SubjectForRefreshToken(ctx, "never-minted"); err != nil || subject != "" {
		t.Errorf("an unknown value answered (%q, %v), want an empty answer with no error", subject, err)
	}
	if subject, err := store.SubjectForRefreshToken(ctx, ""); err != nil || subject != "" {
		t.Errorf("an empty value answered (%q, %v), want an empty answer with no error", subject, err)
	}
}
