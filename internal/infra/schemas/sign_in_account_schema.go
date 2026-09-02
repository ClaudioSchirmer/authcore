// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to the sign-in path.
//
// THE ACCOUNT: everything the user sign-in needs about the person and their
// tenant, in one row.
//
// IT IS NOT A USER AGGREGATE AND MUST NOT BECOME ONE. There is no invariant to
// protect here and no lifecycle to drive: the credential is checked against
// PasswordHash, the two statuses decide whether the account may proceed, and the
// rest becomes claims. A write to any of these columns goes through the User
// aggregate, which is where the rules live.
//
// PasswordHash is an ORDINARY field here, where the entity's schema declares it
// redacted. The redaction governs what leaves the service in a payload or an audit
// event; this row never becomes either. Declaring it redacted here would hand the
// comparison a masked value and refuse every sign-in.
//
// ONE SCHEMA PER FILE, per the layout standard's "No bundling" rule for schemas/.
// The sign-in reads a SUBSET of each table on purpose, so this has a generated twin
// describing the same rows for the write side.

package schemas

import (
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// SignInAccount is everything the sign-in needs about the person and their
// tenant, in one row.
//
// IT IS NOT A USER AGGREGATE AND MUST NOT BECOME ONE. There is no invariant to
// protect here and no lifecycle to drive: the credential is checked against
// PasswordHash, the two statuses decide whether the account may proceed, and the
// rest becomes claims. A write to any of these columns goes through the User
// aggregate, which is where the rules live.
type SignInAccount struct {
	ID                 domain.ID
	TenantID           domain.ID
	Email              string
	GivenName          string
	FamilyName         string
	PasswordHash       string
	Status             string
	MustChangePassword bool

	// Filled by the declared join into `tenants`, never persisted.
	TenantWorkspace string
	TenantStatus    string
}

// SignInAccountSchema maps SignInAccount to `users`.
//
// PasswordHash is an ORDINARY field here, where the entity's schema declares it
// redacted. The redaction governs what leaves the service in a payload or an audit
// event; this row never becomes either — it is scanned, compared against a
// presented credential inside PasswordMatches, and dropped. Declaring it redacted
// here would hand the comparison a masked value and refuse every sign-in.
func SignInAccountSchema() *core.TableSchema {
	return core.NewDirectSchema[SignInAccount]("users").
		ID("id").
		Field("TenantID", "tenant_id").
		Field("Email", "email").
		Field("GivenName", "given_name").
		Field("FamilyName", "family_name").
		Field("PasswordHash", "password_hash").
		Field("Status", "status").
		Field("MustChangePassword", "must_change_password").
		DeletedAt("deleted_at")
}
