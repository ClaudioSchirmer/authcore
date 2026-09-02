// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to ONE endpoint.
//
// THE MACHINE SIGN-IN'S OWN READ MODEL, and this file is its entry point. POST
// /auth/client/token asks a question no other caller in this service asks — "which
// integration is this id, and everything a token has to say about it" — once per
// attempt, on a security path as hot as the user one.
//
// IT IS NOT A CLIENT AGGREGATE AND MUST NOT BECOME ONE. There is no invariant to
// protect here and no lifecycle to drive: the credential is checked against the two
// hashes, the two statuses decide whether the account may proceed, and the rest
// becomes claims. A write to any of these columns goes through the Client
// aggregate, which is where the rules live.
//
// THE CREDENTIAL IS TWO COLUMNS, NOT ONE. `users.password_hash` is a single value;
// a client carries `secret_hash` AND `previous_secret_hash` with its expiry, because
// a rotation OVERLAPS rather than swaps. Both ride on the root row, which is
// precisely the decision the Client spec's §4 took so the token path could read them
// in the ONE query that already loads the client — a sibling would have put a second
// round trip on the hottest security path to save two nullable columns.
//
// ONE SCHEMA PER FILE, per the layout standard's "No bundling" rule for schemas/.
// The sign-in reads a SUBSET of each table on purpose, so this has a generated twin
// describing the same rows for the write side; the test next door asserts the two
// agree, column by column.

package schemas

import (
	"time"

	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// SignInClient is everything the machine sign-in needs about the integration and
// its tenant, in one row.
//
// IT IS NOT A CLIENT AGGREGATE AND MUST NOT BECOME ONE. There is no invariant to
// protect here and no lifecycle to drive: the credential is checked against the two
// hashes, the two statuses decide whether the account may proceed, and the rest
// becomes claims. A write to any of these columns goes through the Client
// aggregate, which is where the rules live.
type SignInClient struct {
	ID       domain.ID
	TenantID domain.ID
	Name     string
	Status   string

	// THE CREDENTIAL, BOTH HALVES. SecretHash is the live one. The previous pair is
	// the rotation still in flight: non-nil together, and accepted only while
	// PreviousSecretExpiresAt is in the future. Nil means no rotation is pending,
	// which is the ordinary state.
	SecretHash              string
	PreviousSecretHash      *string
	PreviousSecretExpiresAt *time.Time

	// Filled by the declared join into `tenants`, never persisted.
	TenantWorkspace string
	TenantStatus    string
}

// SignInClientSchema maps SignInClient to `clients`.
//
// THE TWO HASH COLUMNS ARE ORDINARY FIELDS HERE, where the entity's schema declares
// both RedactedField. The redaction governs what leaves the service in a sync
// payload or an audit event; this row never becomes either — it is scanned,
// compared against a presented credential inside SecretMatches, and dropped.
// Declaring them redacted here would hand the comparison a masked value and refuse
// every sign-in. Exactly the call SignInAccountSchema makes for `password_hash`.
func SignInClientSchema() *core.TableSchema {
	return core.NewDirectSchema[SignInClient]("clients").
		ID("id").
		Field("TenantID", "tenant_id").
		Field("Name", "name").
		Field("Status", "status").
		Field("SecretHash", "secret_hash").
		Field("PreviousSecretHash", "previous_secret_hash").
		Field("PreviousSecretExpiresAt", "previous_secret_expires_at").
		DeletedAt("deleted_at")
}
