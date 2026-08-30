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
// exfiltration endpoint whatever permission guarded it.
//
// NOT BEING AN ENTITY IS NO LONGER A REASON TO WRITE SQL. It was until omnicore
// v0.64.0: the only door into the relational engine was a repository bound to a
// domain.Entity, so this file rendered its own statements — placeholders, quoting
// and argument encoding re-derived per dialect, and a table's column names typed
// out in Go. DirectSchema anchors a schema on a TABLE instead, and DirectRepository
// reads and writes it with the loader's own vocabulary, so every statement below is
// now a criteria and not a string.
//
// WHAT UNBLOCKED THAT WAS THE PRIMARY KEY. An earlier shape of the 0006 migration
// made `hash` the primary key, on the grounds that lookup is by hash on every
// redemption. That single choice put the table outside the engine entirely —
// DirectWriter.Insert mints the identity and refuses a caller-supplied one, and the
// ID slot always binds in the dialect's canonical identity form, which a CHAR(64)
// hex digest is not. The table now carries a UUID id like every other in this
// service, with the hash as a UNIQUE index that serves the same single-row lookup.
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
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/infra/db/command/read"
	"github.com/ClaudioSchirmer/omnicore/infra/db/command/write"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
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

// RefreshTokenStore persists refresh-token rotation state for the framework's
// Issuer.
//
// It holds a DIRECT repository — one table, no aggregate — built over the
// service's own engine, so these statements run on the same connection pool and
// render in the same dialect as every other read here.
//
// It no longer holds the narrow two-method seam it used to. That seam existed to
// keep a SQL-writing component away from the engine's typed verbs; with the
// statements gone there is nothing to keep it away from — the repository IS the
// bound, and wrapping it in a smaller interface would only be a second contract
// to maintain beside the one the framework already publishes.
type RefreshTokenStore struct {
	repo   *read.DirectRepository[schemas.RefreshToken]
	logger *slog.Logger
}

// NewRefreshTokenStore builds the store over the service's engine.
//
// A nil logger falls back to slog.Default() rather than panicking: the only thing
// this store logs is a swept-rows line and a swallowed sweep failure, and neither
// is worth refusing to start over.
func NewRefreshTokenStore(engine core.RelationalEngine, logger *slog.Logger) *RefreshTokenStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &RefreshTokenStore{
		repo:   read.NewDirectRepository[schemas.RefreshToken](engine, schemas.RefreshTokenSchema()),
		logger: logger,
	}
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
//
// THE ID IS NOT WRITTEN HERE, and cannot be: Insert mints it and refuses an `ID`
// key in Values. Nothing in this store addresses a row by it — every read and
// write below is keyed on the hash or on the family — so the value it returns is
// discarded. The id exists so the table lives inside the engine, not so this file
// has something to hold.
func (s *RefreshTokenStore) Save(ctx context.Context, rec authcore.RefreshTokenRecord) error {
	audience, err := json.Marshal(rec.Audience)
	if err != nil {
		return fmt.Errorf("refresh token store: encode audience: %w", err)
	}

	if _, err := s.repo.Insert(ctx, write.Values{
		"Hash":      rec.Hash,
		"FamilyID":  rec.FamilyID,
		"Subject":   rec.Subject,
		"Audience":  string(audience),
		"ExpiresAt": rec.ExpiresAt,
		"Used":      rec.Used,
		"Revoked":   rec.Revoked,
	}); err != nil {
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
//
// BY THE HASH, which is a UNIQUE index and therefore a single-row lookup — the
// same one the old primary key gave. FindOne is the right verb for exactly that
// reason: it refuses a criteria that matched more than one row instead of picking
// one, so a duplicate hash would surface as an error rather than as an arbitrary
// session.
func (s *RefreshTokenStore) Lookup(ctx context.Context, hash string) (authcore.RefreshTokenRecord, error) {
	row, err := s.repo.FindOne(ctx, criteria.Where(criteria.Eq("Hash", hash)))
	switch {
	case err == nil:
	case isRecordNotFound(err):
		return authcore.RefreshTokenRecord{}, authcore.ErrRefreshTokenNotFound
	default:
		return authcore.RefreshTokenRecord{}, fmt.Errorf("refresh token store: lookup: %w", err)
	}

	rec := authcore.RefreshTokenRecord{
		Hash:      row.Hash,
		FamilyID:  row.FamilyID,
		Subject:   row.Subject,
		ExpiresAt: row.ExpiresAt,
		Used:      row.Used,
		Revoked:   row.Revoked,
	}
	// Empty rather than an error on a malformed audience: the column is written
	// only by Save above, so a value that will not decode means the row was
	// tampered with or predates a format change — and refusing the whole
	// redemption over the AUDIENCE list would lock a user out over something the
	// Issuer would have defaulted anyway.
	if uerr := json.Unmarshal([]byte(row.Audience), &rec.Audience); uerr != nil {
		s.logger.WarnContext(ctx, "refresh token store: unreadable audience column, treating as empty",
			slog.String("familyId", rec.FamilyID), slog.String("error", uerr.Error()))
		rec.Audience = nil
	}
	return rec, nil
}

// MarkUsed burns one token.
//
// It is the single-use half of the rotation, and the Issuer calls it BEFORE
// minting the replacement — so a crash between the two loses a session rather
// than leaving a token that can be redeemed twice. That ordering is the
// framework's and this method must not soften it: no upsert, no "only if not
// already used", nothing that could turn a second redemption into a success.
//
// Update and not UpdateOne, deliberately. UpdateOne fails when the predicate
// matched nothing, and burning a hash that is no longer there is not a failure
// this store should raise: the row may have been swept between the lookup and
// here, and the redemption is refused by the Issuer either way. The count is
// discarded for the same reason.
func (s *RefreshTokenStore) MarkUsed(ctx context.Context, hash string) error {
	if _, err := s.repo.Update(ctx,
		write.Values{"Used": true},
		criteria.Where(criteria.Eq("Hash", hash)),
	); err != nil {
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
	if _, err := s.repo.Update(ctx,
		write.Values{"Revoked": true},
		criteria.Where(criteria.Eq("FamilyID", familyID)),
	); err != nil {
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
// TWO STATEMENTS, AND THE BOUND IS WHY. Delete takes a predicate, not a window —
// there is no verb for "delete at most N matching rows" — so the batch is found
// first, under the criteria's own Limit, and removed by the ids that came back.
// Dropping to a single unbounded DELETE would have been shorter and wrong: the
// first run against a table nobody has ever cleaned would delete an unbounded
// number of rows while a user waits for a token. Bounded, that backlog drains
// across several refreshes, and the steady state — a handful of rows per pass —
// is reached within minutes of ordinary traffic.
//
// AN EMPTY BATCH ISSUES NO DELETE, which is not merely an optimisation: a Direct
// write refuses an empty predicate outright, and `In` over no values is exactly
// that. On the steady-state path — nothing expired — this costs one indexed read
// and nothing else.
//
// ITS ERROR IS SWALLOWED ON PURPOSE. The caller has already been granted a token;
// failing their request because housekeeping did not work would trade a real
// outcome for a bookkeeping detail. It is logged instead, which is the same
// posture the framework takes for post-commit domain-event publishing.
func (s *RefreshTokenStore) sweepExpired(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-sweepGrace)

	dead, err := s.repo.FindAll(ctx, criteria.
		Where(criteria.Lt("ExpiresAt", cutoff)).
		Limit(sweepBatch))
	if err != nil {
		s.logger.WarnContext(ctx, "refresh token store: expired-row sweep failed (ignored)",
			slog.String("error", err.Error()))
		return
	}
	if len(dead) == 0 {
		return
	}

	ids := make([]any, 0, len(dead))
	for _, row := range dead {
		ids = append(ids, row.ID)
	}
	if _, err := s.repo.Delete(ctx, criteria.Where(criteria.In("ID", ids...))); err != nil {
		s.logger.WarnContext(ctx, "refresh token store: expired-row sweep failed (ignored)",
			slog.String("error", err.Error()))
	}
}
