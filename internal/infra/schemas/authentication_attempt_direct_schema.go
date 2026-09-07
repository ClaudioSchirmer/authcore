// Hand-written, and no generator declares it: the attempt rollup is not a
// business aggregate. migrations/postgres/0007_authentication_attempts_manual.up.sql
// carries the full reasoning — the short version is that counters on the `users`
// row would only exist for users that EXIST, which is an existence oracle in both
// the wording and the response time, so the table is keyed by the ATTEMPTED
// identity instead.
//
// It has this service's ordinary identity — a UUID `id` primary key with the
// natural key (identity, identity_kind, outcome) carrying a UNIQUE constraint
// beside it — which is what lets the table be described here once and read and
// written through the framework's own Direct engine.
//
// THREE COUNTERS AND TWO INSTANTS ARE THE FRAMEWORK'S, NOT THE CALLER'S. That is
// the whole reason this schema is worth having: `total_count`, `current_count`
// and `total_blocked` are declared StampedCounterField, so filling one means
// `col = col + 1` evaluated by the server under the row's lock — two sign-in
// attempts landing on the same identity at the same moment cannot both read the
// old value and both write back the same number. `window_started_at` and
// `last_at` are StampedTimeField, so their value is the write operation's own
// instant, read from the database itself under `relational.clock: db` rather than
// from whichever pod happened to serve the request.

package schemas

import (
	"time"

	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// AuthenticationAttempt is one row of authentication_attempts: the rollup of
// everything that happened to one identity under one outcome.
//
// It is an INFRASTRUCTURE row and not a domain type. There is no invariant to
// protect here and no lifecycle to drive — the policy that reads it (how many
// failures lock, and for how long) lives in the store, and the answer that
// crosses back to the application is a time and two booleans, never this struct.
//
// NOTHING HERE CAN HOLD A CREDENTIAL. There is no field a password could be
// scanned into and no column it could be written to, which is a property of the
// table this type has to keep rather than a comment about the current code.
//
// LastAt is *time.Time against a NOT NULL column, and that is the stamped
// contract rather than an oversight: a stamped instant is declared as a pointer
// because until the framework fills it the fact has not happened. Every write
// that touches this row fills it, so the column never sees an absence.
type AuthenticationAttempt struct {
	ID           domain.ID
	Identity     string
	IdentityKind string
	Outcome      string

	// The lifetime total for this row kind. Never reset — neither a successful
	// sign-in nor an expiring window may erase history.
	TotalCount int64
	// The failures inside the live lockout window: the resettable counter that
	// IS the lock. Failure row only.
	CurrentCount int64
	// The lifetime count of attempts refused because the identity was already
	// locked. Bumping it cannot extend the lock — see the store.
	TotalBlocked int64

	// When the live lockout window opened. The expiry is DERIVED from it and
	// never stored, so the lock releases at exactly the moment the count stops
	// applying and nothing can drift.
	WindowStartedAt *time.Time
	// The most recent attempt of this row kind.
	LastAt *time.Time
	// The origin of that attempt; empty when the request carried nothing usable.
	LastIP string

	// Whether the identity named a real account the last time anything
	// established it. A pointer because "nobody knows" is a distinct answer from
	// "no": a lookup that failed established nothing, and false would be a
	// recorded falsehood in the one place that exists to be trusted later.
	IdentityExisted *bool
}

// AuthenticationAttemptSchema maps AuthenticationAttempt to authentication_attempts.
//
// NO ArchivedAt, and its absence is a decision. An attempt rollup is operational
// state, not a record anyone archives: the failure row is cleared by a successful
// sign-in and re-anchored by the next burst, and there is no state in between
// worth keeping invisible. Declaring the column here would also silently gate
// every read on it — against a column the migration never created.
func AuthenticationAttemptSchema() *core.TableSchema {
	return core.NewDirectSchema[AuthenticationAttempt]("authentication_attempts").
		ID("id").
		Field("Identity", "identity").
		Field("IdentityKind", "identity_kind").
		Field("Outcome", "outcome").
		StampedCounterField("TotalCount", "total_count").
		StampedCounterField("CurrentCount", "current_count").
		StampedCounterField("TotalBlocked", "total_blocked").
		StampedTimeField("WindowStartedAt", "window_started_at").
		StampedTimeField("LastAt", "last_at").
		Field("LastIP", "last_ip").
		Field("IdentityExisted", "identity_existed")
}
