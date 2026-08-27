// Hand-written, and not a hook: no generator declares this file.
//
// It implements authcore.RefreshTokenStore — a FRAMEWORK PORT, not an aggregate
// of this domain. The split the framework draws is deliberate and this file stays
// on its side of it: the Issuer owns the security-critical algorithm (opaque
// values, single use, rotation on every redemption, family revocation on reuse)
// because that logic must not be copy-pasted per service; the service owns only
// where the rows live.
//
// WHY THIS IS NOT AN ENTITY. Modelling refresh tokens as an omnicore aggregate
// would have bought audit events, notifications, archive semantics and a REST
// surface. Every one of those is wrong here: a refresh token has no invariants to
// validate, no caller edits it, "archived" is meaningless for a credential that is
// simply dead, and a REST surface over this table would be a credential
// exfiltration endpoint whatever permission guarded it. So this reaches the
// neutral read seam directly and accepts, knowingly, that these writes sit
// outside the framework's write guarantees — which for an append-and-mark table
// with no invariants is the right trade, and is recorded as such in
// specs/implement/authentication-token/plan.md.
//
// THE RAW TOKEN NEVER ARRIVES HERE. Only the SHA-256 hash crosses the port. That
// is what makes a dump of this table survivable.

package infra

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// The physical names, from migrations/postgres/0006_refresh_tokens_manual.up.sql.
//
// They are string constants rather than a TableSchema because this table is NOT
// an entity: there is no Go struct for the framework to map, so there is no
// schema to ask. That makes this the one place in the service where a column name
// is written by hand, and the reason it is safe is that one migration and one
// file are the only things that know these names — refresh_token_store_test.go
// asserts the SELECT still matches the shipped DDL.
const (
	refreshTokenTable = "authentication_refresh_tokens"

	refreshColHash      = "hash"
	refreshColFamilyID  = "family_id"
	refreshColSubject   = "subject"
	refreshColAudience  = "audience"
	refreshColExpiresAt = "expires_at"
	refreshColUsed      = "used"
	refreshColRevoked   = "revoked"
)

// sweepGrace holds a just-expired row back from the self-cleaning DELETE.
//
// Without it the sweep races expiry: a token that lapsed seconds ago would be
// deleted rather than found, and the caller's refusal would change from "expired"
// to "unknown token". Both are refusals and both reach the caller as the same
// generic 401, so nothing user-visible turns on it — but the LOG line does, and a
// support question about a client that stopped refreshing is answered by the
// difference between those two words.
const sweepGrace = time.Hour

// sweepBatch bounds one sweep.
//
// The first run against a table nobody has ever cleaned could otherwise delete an
// unbounded number of rows while a user waits for a token. Bounded, that backlog
// drains across several refreshes instead of holding one request open, and the
// steady state — a handful of rows per pass — is reached within minutes of
// ordinary traffic.
const sweepBatch = 500

// SQLSeam is the sliver of the relational engine this store uses: the neutral
// read surface and the dialect that renders the engine-specific bits.
//
// core.RelationalEngine satisfies it, so nothing at the call site changes. What
// changes is what this file can DO: the engine's typed write verbs, the audit
// wiring and the rebuild lock are all out of reach, which is the honest shape for
// a component that has no aggregate to write and no view to rebuild. It also
// means a test fakes two methods instead of eleven.
type SQLSeam interface {
	Querier() core.Querier
	Dialect() core.Dialect
}

// RefreshTokenStore persists refresh-token rotation state for the framework's
// Issuer.
//
// It holds the seam rather than a repository: there is no aggregate to load. Both
// come from the service's one engine, so these statements run on the same
// connection pool, and render in the same dialect, as every other read here.
type RefreshTokenStore struct {
	engine SQLSeam
	logger *slog.Logger
}

// NewRefreshTokenStore builds the store over the service's engine.
//
// A nil logger falls back to slog.Default() rather than panicking: the only thing
// this store logs is a swept-rows line and a swallowed sweep failure, and neither
// is worth refusing to start over.
func NewRefreshTokenStore(engine SQLSeam, logger *slog.Logger) *RefreshTokenStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &RefreshTokenStore{engine: engine, logger: logger}
}

// Compile-time proof that this satisfies the port. Without it a signature drift
// in a future framework release would surface as a confusing wiring error in
// bootstrap rather than as an error on this line.
var _ authcore.RefreshTokenStore = (*RefreshTokenStore)(nil)

// Save writes one freshly minted refresh token.
//
// It is called from BOTH paths — the login's IssueWithRefresh and the rotation
// inside RedeemRefreshToken — which is exactly why the self-cleaning sweep hangs
// off it rather than off a redemption-only seat: every event that adds a row also
// gets a chance to remove the dead ones, so the table cannot grow without also
// draining.
func (s *RefreshTokenStore) Save(ctx context.Context, rec authcore.RefreshTokenRecord) error {
	d := s.engine.Dialect()

	audience, err := json.Marshal(rec.Audience)
	if err != nil {
		return fmt.Errorf("refresh token store: encode audience: %w", err)
	}

	stmt := fmt.Sprintf(
		"INSERT INTO %s (%s, %s, %s, %s, %s, %s, %s) VALUES (%s, %s, %s, %s, %s, %s, %s)",
		d.QuoteIdent(refreshTokenTable),
		d.QuoteIdent(refreshColHash),
		d.QuoteIdent(refreshColFamilyID),
		d.QuoteIdent(refreshColSubject),
		d.QuoteIdent(refreshColAudience),
		d.QuoteIdent(refreshColExpiresAt),
		d.QuoteIdent(refreshColUsed),
		d.QuoteIdent(refreshColRevoked),
		d.Placeholder(1), d.Placeholder(2), d.Placeholder(3),
		d.Placeholder(4), d.Placeholder(5), d.Placeholder(6), d.Placeholder(7),
	)

	if err := core.Exec(s.engine.Querier(), ctx, stmt,
		d.EncodeArg(rec.Hash),
		d.EncodeArg(rec.FamilyID),
		d.EncodeArg(rec.Subject),
		d.EncodeArg(string(audience)),
		d.EncodeArg(rec.ExpiresAt),
		d.EncodeArg(rec.Used),
		d.EncodeArg(rec.Revoked),
	); err != nil {
		return fmt.Errorf("refresh token store: save: %w", err)
	}

	// AFTER the row that matters is committed, and never in front of it. A sweep
	// that ran first and failed would cost the caller a token they had earned.
	s.sweepExpired(ctx)
	return nil
}

// Lookup returns the record behind a hash.
//
// A miss answers authcore.ErrRefreshTokenNotFound, which is the port's documented
// contract and is matched with errors.Is by the Issuer. It deliberately does NOT
// distinguish "never existed" from "swept after expiry": both mean the value in
// the caller's hand redeems nothing, and the Issuer treats them identically.
func (s *RefreshTokenStore) Lookup(ctx context.Context, hash string) (authcore.RefreshTokenRecord, error) {
	d := s.engine.Dialect()

	stmt := fmt.Sprintf(
		"SELECT %s, %s, %s, %s, %s, %s, %s FROM %s WHERE %s = %s",
		d.QuoteIdent(refreshColHash),
		d.QuoteIdent(refreshColFamilyID),
		d.QuoteIdent(refreshColSubject),
		d.QuoteIdent(refreshColAudience),
		d.QuoteIdent(refreshColExpiresAt),
		d.QuoteIdent(refreshColUsed),
		d.QuoteIdent(refreshColRevoked),
		d.QuoteIdent(refreshTokenTable),
		d.QuoteIdent(refreshColHash),
		d.Placeholder(1),
	)

	var (
		rec         authcore.RefreshTokenRecord
		rawAudience string
	)
	row := s.engine.Querier().QueryRow(ctx, stmt, d.EncodeArg(hash))
	err := row.Scan(&rec.Hash, &rec.FamilyID, &rec.Subject, &rawAudience,
		&rec.ExpiresAt, &rec.Used, &rec.Revoked)
	switch {
	case err == nil:
		// Empty rather than an error on a malformed audience: the column is
		// written only by Save above, so a value that will not decode means the
		// row was tampered with or predates a format change — and refusing the
		// whole redemption over the AUDIENCE list would lock a user out over
		// something the Issuer would have defaulted anyway.
		if uerr := json.Unmarshal([]byte(rawAudience), &rec.Audience); uerr != nil {
			s.logger.WarnContext(ctx, "refresh token store: unreadable audience column, treating as empty",
				slog.String("familyId", rec.FamilyID), slog.String("error", uerr.Error()))
			rec.Audience = nil
		}
		return rec, nil
	case isNoRows(err):
		return authcore.RefreshTokenRecord{}, authcore.ErrRefreshTokenNotFound
	default:
		return authcore.RefreshTokenRecord{}, fmt.Errorf("refresh token store: lookup: %w", err)
	}
}

// MarkUsed burns one token.
//
// It is the single-use half of the rotation, and the Issuer calls it BEFORE
// minting the replacement — so a crash between the two loses a session rather
// than leaving a token that can be redeemed twice. That ordering is the
// framework's and this method must not soften it: no upsert, no "only if not
// already used", nothing that could turn a second redemption into a success.
func (s *RefreshTokenStore) MarkUsed(ctx context.Context, hash string) error {
	d := s.engine.Dialect()

	stmt := fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s = %s",
		d.QuoteIdent(refreshTokenTable),
		d.QuoteIdent(refreshColUsed), d.Placeholder(1),
		d.QuoteIdent(refreshColHash), d.Placeholder(2),
	)

	if err := core.Exec(s.engine.Querier(), ctx, stmt,
		d.EncodeArg(true), d.EncodeArg(hash)); err != nil {
		return fmt.Errorf("refresh token store: mark used: %w", err)
	}
	return nil
}

// RevokeFamily kills a whole session lineage.
//
// Called when the Issuer detects that an already-redeemed token was presented
// again — the standard signal a refresh token was stolen. Revoking the FAMILY and
// not just the replayed value is the point: the thief and the legitimate owner
// both hold descendants of the same login, and there is no way to tell which is
// which, so the only safe answer is that neither continues.
func (s *RefreshTokenStore) RevokeFamily(ctx context.Context, familyID string) error {
	d := s.engine.Dialect()

	stmt := fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s = %s",
		d.QuoteIdent(refreshTokenTable),
		d.QuoteIdent(refreshColRevoked), d.Placeholder(1),
		d.QuoteIdent(refreshColFamilyID), d.Placeholder(2),
	)

	if err := core.Exec(s.engine.Querier(), ctx, stmt,
		d.EncodeArg(true), d.EncodeArg(familyID)); err != nil {
		return fmt.Errorf("refresh token store: revoke family: %w", err)
	}
	s.logger.WarnContext(ctx, "refresh token reuse detected: session family revoked",
		slog.String("familyId", familyID))
	return nil
}

// SubjectForRefreshToken answers whose session a refresh value belongs to, or an
// empty string when it belongs to none.
//
// IT EXISTS BECAUSE OF AN ORDERING THE FRAMEWORK IMPOSES. RedeemRefreshToken
// takes the claim map by VALUE — it is handed the claims and then looks up the
// record — so the caller has to know whose claims to build before the framework
// tells it whose token this is. This method closes that loop, and it lives HERE
// rather than in the handler for one reason: hashing the value is the store's
// business, and the handler must never see a raw refresh secret it has no use for.
//
// IT DECIDES NOTHING. Revoked, used and expired are all left to the Issuer, which
// is the component that owns reuse detection and family revocation. This is a
// name lookup and no more — a token this method resolves may still be refused a
// moment later, which is correct: the authoritative check is the redemption
// itself, and duplicating it here would be a second opinion free to disagree.
//
// The hash form mirrors authcore's own — hex-encoded SHA-256 of the opaque value
// — because the Issuer's hashRefreshValue is unexported. That duplication is
// pinned by a test that round-trips a real IssueWithRefresh through this lookup,
// so a framework change to the hashing fails the suite instead of silently
// turning every refresh into a 401.
func (s *RefreshTokenStore) SubjectForRefreshToken(ctx context.Context, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	sum := sha256.Sum256([]byte(value))
	rec, err := s.Lookup(ctx, hex.EncodeToString(sum[:]))
	switch {
	case err == nil:
		return rec.Subject, nil
	case errors.Is(err, authcore.ErrRefreshTokenNotFound):
		// Not an error to the caller: an unknown value is an ordinary refusal, and
		// answering "" lets the handler decline without having to know that this
		// store distinguishes the two.
		return "", nil
	default:
		return "", err
	}
}

// sweepExpired deletes rows already past their expiry, bounded, and NEVER
// propagates a failure.
//
// This is what replaces a scheduled cleanup job. The trade is explicit: a job
// would also drain a table with no traffic, while this drains only while somebody
// is signing in or refreshing — but a job is a second deployable that can be
// forgotten, misconfigured, or silently not running, and a table nobody writes to
// is also a table that is not growing.
//
// ITS ERROR IS SWALLOWED ON PURPOSE. The caller has already been granted a token;
// failing their request because housekeeping did not work would trade a real
// outcome for a bookkeeping detail. It is logged instead, which is the same
// posture the framework takes for post-commit domain-event publishing.
func (s *RefreshTokenStore) sweepExpired(ctx context.Context) {
	d := s.engine.Dialect()

	// Bounded through a subquery rather than a bare `DELETE ... LIMIT`: only some
	// dialects accept a limit on DELETE, and ApplyLimit is the framework's own
	// renderer for capping a complete SELECT in whichever position the engine
	// wants it.
	inner := fmt.Sprintf("SELECT %s FROM %s WHERE %s < %s",
		d.QuoteIdent(refreshColHash),
		d.QuoteIdent(refreshTokenTable),
		d.QuoteIdent(refreshColExpiresAt),
		d.Placeholder(1),
	)
	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)",
		d.QuoteIdent(refreshTokenTable),
		d.QuoteIdent(refreshColHash),
		d.ApplyLimit(inner, sweepBatch),
	)

	cutoff := time.Now().UTC().Add(-sweepGrace)
	if err := core.Exec(s.engine.Querier(), ctx, stmt, d.EncodeArg(cutoff)); err != nil {
		s.logger.WarnContext(ctx, "refresh token store: expired-row sweep failed (ignored)",
			slog.String("error", err.Error()))
	}
}

// isNoRows reports whether a scan error means "that row does not exist".
//
// It is NOT isRecordNotFound from role_service_manual.go: that one recognises the
// *domain.DomainError the aggregate LOADER raises, and nothing here goes through
// a loader — this store scans a raw Row.
//
// Every engine reports an empty result differently (database/sql answers
// sql.ErrNoRows, pgx answers pgx.ErrNoRows), and this file must not import either
// driver: doing so would pin the store to one dialect while the rest of the
// service is deliberately free of any. Matching the message is the honest price
// of that neutrality — both drivers spell it "no rows in result set", and the
// store's test pins that assumption so a driver that stops spelling it that way
// fails here rather than turning every unknown token into a 500.
func isNoRows(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows in result set")
}
