// Hand-written, and not a hook: no generator declares this file.
//
// THE ONE PATH EVERY AUTHENTICATION TAKES to the auxiliary tables and to the log
// stream. It exists because there is about to be more than one route through it:
// POST /auth/user/token authenticates a person, POST /auth/client/token will
// authenticate a machine, and the two differ in what they verify — never in what
// they must record.
//
// WHAT THEY MUST RECORD IS A PAIR, AND THE PAIR IS THE WHOLE POINT. Every outcome
// moves a counter in `authentication_attempts` AND publishes one record on the
// service's log stream. Splitting those across two call sites per branch per
// route is how a branch ends up counted but unannounced, or announced but
// uncounted — and both failures are invisible in a green build. Here each outcome
// is one method that does both, so a new route cannot half-implement it.
//
// THE SUBJECT KIND IS BOUND ONCE, at construction, instead of being passed at
// every call. That is what keeps a client route from accidentally recording its
// attempts as a user's: the kind is a property of the route, not of the attempt,
// and the attempt table's natural key includes it — so a client id that happens
// to spell an e-mail address counts on its own row.
//
// NOTHING A CREDENTIAL COULD RIDE IN crosses this seam, on either surface. There
// is no parameter a password could arrive in, no column it could be written to,
// and no payload key it could occupy.

package utils

import (
	"context"
	"log/slog"
	"time"

	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/persistence"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The kinds of subject this service authenticates, and the whole vocabulary of
// the `identity_kind` claim and column.
//
// They are spelled here as well as in infra and in the domain, and that is
// deliberate rather than an oversight: each layer owns the vocabulary it
// pronounces, and sharing one constant would mean the application importing
// infra or the domain holding a storage value. Three spellings of two strings is
// the price of the boundary; the spec file is the source they all answer to.
const (
	IdentityKindUser   = "user"
	IdentityKindClient = "client"
)

// EventClass is what every announcement from this file is filed
// under. It surfaces as `entityType` on the log record, so one filter reaches
// every authentication outcome — of either kind — and nothing else.
//
// It is not an entity name and does not pretend to be: the framework's publisher
// documents this field as optional precisely because system-level facts — an
// authentication among them — are not about one entity class. Naming it anyway is
// what makes the stream queryable.
const EventClass = "Authentication"

// Journal records what happened to an authentication attempt, in
// both places it has to be recorded.
//
// A VALUE, NOT A POINTER, and cheap enough to build per request: two interfaces
// and a string. A route holds its ports the way it always did and asks for this
// where it needs it, so nothing about the composition root changes when a second
// route arrives.
type Journal struct {
	attempts AttemptRecorder
	events   AuthenticationEventPublisher
	kind     string
}

// NewJournal binds the ports to one subject kind.
func NewJournal(
	attempts AttemptRecorder, events AuthenticationEventPublisher, kind string,
) Journal {
	return Journal{attempts: attempts, events: events, kind: kind}
}

// LockedUntilFor asks whether this identity is refused outright, and hands back what
// its failures established about whether it names a real account.
//
// The existence verdict never reaches the caller of the endpoint; it exists so
// the record published about a blocked attempt can say whether a REAL account is
// the one under attack — the difference between a targeted attack and
// credential-stuffing noise.
func (j Journal) LockedUntilFor(
	ctx *configuration.AppContext, identity string,
) (until time.Time, locked bool, identityExisted *bool, err error) {
	return j.attempts.LockedUntil(ctx, identity, j.kind)
}

// RefusedWhileLocked counts an attempt made through a live lock and announces it.
//
// The count lands on a lifetime column that moves neither the live counter nor
// the window anchor, so the persistence stays visible to a reviewer and yet
// cannot extend the lock.
func (j Journal) RefusedWhileLocked(
	ctx *configuration.AppContext, identity, ip string, until time.Time, identityExisted *bool,
) error {
	if err := j.attempts.RecordLocked(ctx, identity, j.kind, ip); err != nil {
		return err
	}
	vals := j.values(identity, ip, identityExisted)
	vals["LockedUntilFor"] = until.UTC()
	j.announce(ctx, domain.EventWarning, "sign-in refused: identity is locked", vals)
	return nil
}

// Failed counts a rejected credential and announces WHY it was rejected.
//
// THE REASON IS NOT THE ANSWER THE CALLER GETS. Every Refusal answers the caller
// with one indistinguishable notification; this string separates the branches for
// whoever reads the stream later. Holding those two apart is what lets the log be
// useful without the response becoming an existence oracle — so the reason is the
// caller's to phrase, and this method never invents one.
//
// identityExisted is nil when the lookup itself Failed and nothing was
// established; the store then leaves the stored verdict alone and the
// announcement omits the key, rather than either of them recording a guess.
func (j Journal) Failed(
	ctx *configuration.AppContext, identity, ip, reason string, identityExisted *bool,
) error {
	if err := j.attempts.RecordFailure(ctx, identity, j.kind, ip, identityExisted); err != nil {
		return err
	}
	j.announce(ctx, domain.EventWarning, reason, j.values(identity, ip, identityExisted))
	return nil
}

// Succeeded counts a credential that verified, CLEARS the lockout, and announces
// the sign-in at routine severity.
//
// The existence flag is not a parameter: a credential cannot verify against an
// account that is not there, so it is implied by the outcome and taking it in
// would let a caller record a contradiction.
func (j Journal) Succeeded(ctx *configuration.AppContext, identity, ip string) error {
	if err := j.attempts.RecordSuccess(ctx, identity, j.kind, ip); err != nil {
		return err
	}
	existed := true
	j.announce(ctx, domain.EventLog, "sign-in Succeeded", j.values(identity, ip, &existed))
	return nil
}

// values is the payload every announcement shares.
//
// identityExisted is OMITTED when nothing established it, rather than emitted as
// null: the column behaves the same way for the same reason, and a reader should
// be able to tell "we know this address has no account" from "nobody ever found
// out".
func (j Journal) values(identity, ip string, identityExisted *bool) map[string]any {
	vals := map[string]any{
		"identity":     identity,
		"identityKind": j.kind,
		"ip":           ip,
	}
	if identityExisted != nil {
		vals["identityExisted"] = *identityExisted
	}
	return vals
}

// announce publishes one record about an authentication outcome to the service's
// log stream — the per-attempt narrative the rollup table no longer holds.
//
// ITS FAILURE IS SWALLOWED, and the asymmetry with the counter write is the whole
// design rather than an oversight. A counter that fails to record costs the
// lockout its evidence and must refuse the request — which is why every method
// above returns that error and stops. An announcement that fails costs a log
// line, and turning that into a 500 would let a full log buffer refuse valid
// credentials. This is the framework's own posture for the same port, down to the
// message the warning carries.
func (j Journal) announce(
	ctx *configuration.AppContext, severity domain.EventType, message string, vals map[string]any,
) {
	if j.events == nil {
		return
	}
	if err := j.events.Publish(ctx, domain.DomainEvent{
		Type:  severity,
		Class: EventClass,
		Msg:   message,
		Vals:  vals,
	}); err != nil {
		// slog.Default() is the service's configured logger: bootstrap installs
		// it as the default before anything here can run.
		slog.Default().Warn("event.publish.error", "error", err, "message", message)
	}
}

// AttemptRecorder is the lockout: the counters that decide whether an identity is
// refused outright, and the lifetime totals beside them.
//
// It is a SEPARATE port from AuthenticationStore for the same reason
// RefreshTokenLookup is: a different adapter implements it, over a different
// table, and folding them together would give whichever grew first methods it has
// no business owning.
//
// IT IS NOT THE FORENSIC LOG. The per-attempt narrative — every attempt, its
// origin, its order — is published to the service's log stream through
// AuthenticationEventPublisher below; what this port owns is the state that has
// to be transactional, and nothing else.
//
// THREE RECORDING METHODS RATHER THAN ONE WITH AN OUTCOME PARAMETER. An earlier
// draft passed an outcome value and a struct describing the attempt, which forced
// both a shared enum and a shared type — and neither had a home, since a type both
// layers name can live only in the domain, where a row shape with an IP in it does
// not belong. Naming the outcome in the METHOD dissolves the problem: the storage
// vocabulary stays with the table that owns it, and nothing crosses this seam but
// strings the caller already had.
//
// NOTHING ABOUT THE CREDENTIAL CROSSES IT EITHER. There is no parameter a password
// could arrive in — see the store and the 0007 migration for why that is a rule
// rather than an oversight.
type AttemptRecorder interface {
	// LockedUntil reports whether an identity is currently refused outright,
	// until when, and what its failures already established about whether it
	// names a real account.
	//
	// It answers identically for an identity that names an account and one that
	// does not — that uniformity IS the feature. The existence flag it returns
	// never reaches the caller of the endpoint; it exists so the log record this
	// handler publishes about a blocked attempt can say whether a real account is
	// the one under attack.
	LockedUntil(ctx context.Context, identity, kind string) (until time.Time, locked bool, identityExisted *bool, err error)

	// RecordFailure counts a credential presented and rejected. The only outcome
	// that moves the lockout. identityExisted is nil when the lookup itself
	// Failed and the answer is genuinely unknown — the store then leaves the
	// stored verdict alone rather than overwriting it with a guess.
	RecordFailure(ctx context.Context, identity, kind, ip string, identityExisted *bool) error

	// RecordSuccess counts a credential that verified and CLEARS the lockout:
	// the live counter goes to zero while the lifetime total is untouched, which
	// is how "a successful sign-in resets the counter" works without erasing
	// history.
	RecordSuccess(ctx context.Context, identity, kind, ip string) error

	// RecordLocked counts an attempt refused because the identity was already
	// locked. It bumps one lifetime counter and moves neither the live count nor
	// the window anchor, so the persistence is visible to a reviewer and yet
	// cannot extend the lock.
	//
	// It takes no existence flag: this path deliberately never performs a lookup,
	// and the flag already sits on the row that the failures causing this lock
	// wrote.
	RecordLocked(ctx context.Context, identity, kind, ip string) error
}

// AuthenticationEventPublisher is where the per-attempt record goes now that the
// attempt table is a rollup: one structured record on the service's always-on log
// stream for every sign-in outcome, success or failure.
//
// IT IS THE FRAMEWORK'S OWN PORT, NARROWED — deliberately spelled with the same
// signature as omnicore's events.Publisher so *events.SlogPublisher satisfies it
// structurally and the composition root hands one straight in. No adapter is
// written, and no type is invented to carry an event across a layer: both
// persistence.RequestContext and domain.Event are already legal imports above
// infra, and *configuration.AppContext already satisfies the first.
//
// PublishAll is deliberately absent. The framework's write path publishes an
// entity's accumulated events in a batch; a sign-in has exactly one fact to
// announce per outcome, and a port that only offers what the caller needs cannot
// be misused into batching them.
//
// WHAT CROSSES IT IS NEVER A CREDENTIAL. The same rule as the table, for the
// stronger reason: this stream leaves the box.
type AuthenticationEventPublisher interface {
	Publish(ctx persistence.RequestContext, event domain.Event) error
}
