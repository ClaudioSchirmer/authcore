// Tests for the refresh-token store.
//
// Three things are worth protecting here and the rest is plumbing:
//
//  1. THE HASH FORM. This store mirrors authcore's hashing because the
//     framework's hashRefreshValue is unexported. If a release ever changed it,
//     every rotation in production would answer 401 and nothing would say why —
//     so the mirror is pinned against a REAL round trip through the framework's
//     own Issuer rather than against a constant somebody copied over.
//  2. THE SINGLE-USE MARKER. MarkUsed must be an unconditional write. Softening
//     it into "only if not already used" would turn a replay into a success,
//     which is the one failure reuse detection exists to prevent.
//  3. THE SWEEP IS BOUNDED AND SWALLOWED. It runs on the request path, so an
//     unbounded delete would hold a sign-in open, and a propagated error would
//     fail a token the caller had already earned.

package infra

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"crypto/sha256"

	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// ── a recording seam ────────────────────────────────────────────────────────

// recordingQuerier captures every statement and argument list the store issues,
// which is what lets these tests assert the SHAPE of a write without a database.
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
	return &recordingRows{rows: q.rows, scanErr: q.rowsScanErr, iterErr: q.rowsIterErr}, nil
}

// recordingRows replays a fixed result set. Deliberately minimal: the tests that
// use it assert the DECISION taken over the rows, not the driver's behaviour.
type recordingRows struct {
	rows    [][]any
	at      int
	scanErr error
	iterErr error
}

func (r *recordingRows) Next() bool {
	if r.at >= len(r.rows) {
		return false
	}
	r.at++
	return true
}

func (r *recordingRows) Scan(dest ...any) error {
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

func (r *recordingRows) Err() error   { return r.iterErr }
func (r *recordingRows) Close() error { return nil }

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

// fakeSeam is the whole dependency the store now has — which is the point of
// narrowing it from the full engine.
type fakeSeam struct{ q *recordingQuerier }

func (s *fakeSeam) Querier() core.Querier { return s.q }
func (s *fakeSeam) Dialect() core.Dialect { return testDialect{} }

func newTestStore(q *recordingQuerier) *RefreshTokenStore {
	return NewRefreshTokenStore(&fakeSeam{q: q}, slog.Default())
}

// ── the hash mirror ─────────────────────────────────────────────────────────

// THE PIN. A refresh value minted by the framework's own Issuer must hash, under
// this store's mirror, to the hash the framework asked the store to Save. If a
// future release changes its hashing, this fails here instead of turning every
// rotation in production into a silent 401 nobody can explain.
func TestSubjectForRefreshToken_HashMirrorsTheFramework(t *testing.T) {
	captured := &capturingStore{}
	issuer, err := authcore.NewIssuer(authcore.IssuerOptions{
		SelfURL:         "http://localhost",
		Audience:        []string{"authcore"},
		TokenTTL:        time.Minute,
		MaxTokenTTL:     time.Hour,
		RefreshTokenTTL: time.Hour,
		Keys: []authcore.SigningKey{{
			KID:        "test",
			Algorithm:  "RS256",
			PrivatePEM: testSigningKeyPEM(t),
			State:      authcore.KeyCurrent,
		}},
		RefreshStore: captured,
	})
	if err != nil {
		t.Fatalf("building the issuer: %v", err)
	}

	_, refresh, err := issuer.IssueWithRefresh(context.Background(), authcore.TokenRequest{Subject: "user-1"})
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	sum := sha256.Sum256([]byte(refresh.Value))
	mirrored := hex.EncodeToString(sum[:])
	if mirrored != captured.saved.Hash {
		t.Fatalf("this store hashes a refresh value to %q, the framework stored %q — "+
			"SubjectForRefreshToken would find nothing and every rotation would answer 401",
			mirrored, captured.saved.Hash)
	}
	if captured.saved.Subject != "user-1" {
		t.Errorf("subject = %q, want the one the token was minted for", captured.saved.Subject)
	}
}

// capturingStore is the minimal port implementation the pin above needs.
type capturingStore struct{ saved authcore.RefreshTokenRecord }

func (s *capturingStore) Save(_ context.Context, rec authcore.RefreshTokenRecord) error {
	s.saved = rec
	return nil
}
func (s *capturingStore) Lookup(context.Context, string) (authcore.RefreshTokenRecord, error) {
	return authcore.RefreshTokenRecord{}, authcore.ErrRefreshTokenNotFound
}
func (s *capturingStore) MarkUsed(context.Context, string) error     { return nil }
func (s *capturingStore) RevokeFamily(context.Context, string) error { return nil }

// testSigningKeyPEM generates a throwaway PKCS#8 RSA key.
//
// Generated rather than embedded: a committed private key in a test fixture is a
// committed private key, and the ones that leak are always the ones somebody was
// sure did not matter.
func testSigningKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating a test key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("encoding the test key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// ── the statement shapes ────────────────────────────────────────────────────

func TestMarkUsed_IsAnUnconditionalWrite(t *testing.T) {
	q := &recordingQuerier{}
	store := newTestStore(q)

	if err := store.MarkUsed(context.Background(), "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.execs) != 1 {
		t.Fatalf("expected one statement, got %d", len(q.execs))
	}
	stmt := q.execs[0]
	if !strings.HasPrefix(stmt, `UPDATE "authentication_refresh_tokens" SET "used"`) {
		t.Errorf("unexpected statement: %s", stmt)
	}
	// The guard that matters: keyed on the hash ALONE, with no condition on the
	// current value. A replay has to stay visible — "only if not already used"
	// would erase the very evidence reuse detection runs on.
	if !strings.HasSuffix(stmt, `WHERE "hash" = $2`) {
		t.Errorf("MarkUsed must key on the hash alone and add no other condition: %s", stmt)
	}
	if strings.Count(stmt, "WHERE") != 1 {
		t.Errorf("MarkUsed grew a second condition: %s", stmt)
	}
}

func TestRevokeFamily_TargetsTheWholeFamily(t *testing.T) {
	q := &recordingQuerier{}
	store := newTestStore(q)

	if err := store.RevokeFamily(context.Background(), "fam-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stmt := q.execs[0]
	if !strings.Contains(stmt, `SET "revoked" = $1`) || !strings.HasSuffix(stmt, `WHERE "family_id" = $2`) {
		t.Errorf("a reuse must kill the whole lineage, not one row: %s", stmt)
	}
	if q.execArgs[0][1] != "fam-1" {
		t.Errorf("bound %v, want the family id", q.execArgs[0][1])
	}
}

// The sweep rides Save, runs AFTER the insert, is bounded, and respects the
// grace margin.
func TestSave_SweepsBoundedAndAfterTheInsert(t *testing.T) {
	q := &recordingQuerier{}
	store := newTestStore(q)

	err := store.Save(context.Background(), authcore.RefreshTokenRecord{
		Hash: "h", FamilyID: "f", Subject: "s",
		Audience: []string{"authcore"}, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.execs) != 2 {
		t.Fatalf("expected the insert and the sweep, got %d statements", len(q.execs))
	}
	if !strings.HasPrefix(q.execs[0], `INSERT INTO "authentication_refresh_tokens"`) {
		t.Errorf("the row the caller earned must be written first: %s", q.execs[0])
	}
	sweep := q.execs[1]
	if !strings.HasPrefix(sweep, `DELETE FROM "authentication_refresh_tokens"`) {
		t.Errorf("expected the sweep second: %s", sweep)
	}
	if !strings.Contains(sweep, "LIMIT 500") {
		t.Errorf("the sweep must be bounded — an unbounded first pass holds a sign-in open: %s", sweep)
	}
	// The cutoff carries the grace margin, so the sweep never races a token
	// expiring mid-flight into a "not found" where "expired" was the truth.
	cutoff, ok := q.execArgs[1][0].(time.Time)
	if !ok {
		t.Fatalf("sweep argument = %#v, want a time", q.execArgs[1][0])
	}
	if time.Since(cutoff) < sweepGrace {
		t.Errorf("cutoff %v is inside the grace margin", cutoff)
	}
}

// The audience round-trips as JSON, because only one supported dialect has arrays.
func TestSave_EncodesAudienceAsJSON(t *testing.T) {
	q := &recordingQuerier{}
	store := newTestStore(q)

	err := store.Save(context.Background(), authcore.RefreshTokenRecord{
		Hash: "h", Audience: []string{"users-api", "orders-api"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := q.execArgs[0][3]; got != `["users-api","orders-api"]` {
		t.Errorf("audience bound as %#v, want a JSON array", got)
	}
}

// An INSERT failure propagates: the caller did not get the row they needed.
func TestSave_InsertFailurePropagates(t *testing.T) {
	q := &recordingQuerier{execErr: errors.New("insert exploded")}
	store := newTestStore(q)

	if err := store.Save(context.Background(), authcore.RefreshTokenRecord{Hash: "h"}); err == nil {
		t.Error("an insert failure must propagate — the token would be unusable")
	}
}

// A nil logger must not panic: the only thing this store logs is housekeeping.
func TestNewRefreshTokenStore_NilLoggerFallsBack(t *testing.T) {
	if store := NewRefreshTokenStore(&fakeSeam{q: &recordingQuerier{}}, nil); store.logger == nil {
		t.Error("a nil logger must fall back rather than being stored as nil")
	}
}

// ── the lookup ──────────────────────────────────────────────────────────────

func TestSubjectForRefreshToken_EmptyValueAsksNothing(t *testing.T) {
	q := &recordingQuerier{}
	store := newTestStore(q)

	subject, err := store.SubjectForRefreshToken(context.Background(), "")
	if err != nil || subject != "" {
		t.Errorf("got (%q, %v), want an empty answer with no error", subject, err)
	}
	if len(q.queried) != 0 {
		t.Error("an empty value must not reach the database")
	}
}

// A miss is an ordinary refusal, not an error the handler has to know about.
func TestSubjectForRefreshToken_MissIsNotAnError(t *testing.T) {
	q := &recordingQuerier{scanErr: errors.New("no rows in result set")}
	store := newTestStore(q)

	subject, err := store.SubjectForRefreshToken(context.Background(), "unknown")
	if err != nil {
		t.Errorf("a miss must not surface as an error: %v", err)
	}
	if subject != "" {
		t.Errorf("subject = %q, want empty", subject)
	}
}

// A REAL failure must surface: answering "" would turn an outage into a refusal
// and tell a caller holding a good token that it was rejected.
func TestSubjectForRefreshToken_RealFailureSurfaces(t *testing.T) {
	q := &recordingQuerier{scanErr: errors.New("connection reset by peer")}
	store := newTestStore(q)

	if _, err := store.SubjectForRefreshToken(context.Background(), "v"); err == nil {
		t.Error("a store outage must not be silently reported as an unknown token")
	}
}

func TestSubjectForRefreshToken_ReturnsTheStoredSubject(t *testing.T) {
	q := &recordingQuerier{scanFill: func(dest ...any) error {
		// hash, family_id, subject, audience, expires_at, used, revoked
		*(dest[0].(*string)) = "h"
		*(dest[1].(*string)) = "fam"
		*(dest[2].(*string)) = "user-42"
		*(dest[3].(*string)) = `["authcore"]`
		*(dest[4].(*time.Time)) = time.Now().Add(time.Hour)
		*(dest[5].(*bool)) = false
		*(dest[6].(*bool)) = false
		return nil
	}}
	store := newTestStore(q)

	subject, err := store.SubjectForRefreshToken(context.Background(), "v")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subject != "user-42" {
		t.Errorf("subject = %q, want user-42", subject)
	}
}

func TestLookup_MissMapsToTheFrameworkSentinel(t *testing.T) {
	q := &recordingQuerier{scanErr: errors.New("sql: no rows in result set")}
	store := newTestStore(q)

	_, err := store.Lookup(context.Background(), "h")
	if !errors.Is(err, authcore.ErrRefreshTokenNotFound) {
		t.Errorf("err = %v, want the port's documented sentinel (the Issuer matches it with errors.Is)", err)
	}
}

// An unreadable audience must not lock a user out: the Issuer would have
// defaulted it anyway, and refusing a whole redemption over that column would
// turn a formatting problem into a lost session.
func TestLookup_UnreadableAudienceDegradesToEmpty(t *testing.T) {
	q := &recordingQuerier{scanFill: func(dest ...any) error {
		*(dest[0].(*string)) = "h"
		*(dest[1].(*string)) = "fam"
		*(dest[2].(*string)) = "user-42"
		*(dest[3].(*string)) = "not json at all"
		*(dest[4].(*time.Time)) = time.Now()
		*(dest[5].(*bool)) = false
		*(dest[6].(*bool)) = false
		return nil
	}}
	store := newTestStore(q)

	rec, err := store.Lookup(context.Background(), "h")
	if err != nil {
		t.Fatalf("a bad audience column must not fail the lookup: %v", err)
	}
	if rec.Audience != nil {
		t.Errorf("audience = %v, want nil", rec.Audience)
	}
	if rec.Subject != "user-42" {
		t.Errorf("the rest of the record was lost: %+v", rec)
	}
}

func TestIsNoRows(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"pgx form", errors.New("no rows in result set"), true},
		{"database/sql form", errors.New("sql: no rows in result set"), true},
		{"wrapped", errors.New("lookup: sql: no rows in result set"), true},
		{"an actual failure", errors.New("connection refused"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNoRows(tc.err); got != tc.want {
				t.Errorf("isNoRows(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
