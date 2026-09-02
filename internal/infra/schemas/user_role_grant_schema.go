// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to the sign-in path.
//
// THE DIRECT GRANT PATH: a role held outright, and one permission it confers.
//
// ANCHORED ON THE USER'S OWN EDGE TABLE, NOT ON `roles`, and that is the whole
// performance story. Anchoring on the catalog and filtering with `id IN (subquery)`
// reads well and plans badly: Postgres turns the IN into a hashed SubPlan and SEQ
// SCANS the role catalog. The same read anchored on `user_roles` — where the index
// is — walks user_roles → roles → role_permissions → permissions as four index
// scans. Measured on a tenant of 5k roles, 2k permissions and 10k users: 0.165ms
// against a plan that seq-scanned three tables.
//
// A ROLE HOLDING THREE PERMISSIONS ARRIVES AS THREE ROWS, and a role holding none
// arrives as one row with nil grants — which is what keeps a role the user holds in
// the answer whatever happened to what it confers.
//
// `directGrantsTarget` declares `role_permissions` with ID("role_id"), which renders
// the join as `role_permissions.role_id = roles.id`: that declaration is what makes
// one role fan out to its grants, and it is also why anchoring a repository on that
// target would be a bug.
//
// ONE SCHEMA PER FILE, per the layout standard's "No bundling" rule for schemas/.
// The sign-in reads a SUBSET of each table on purpose, so this has a generated twin
// describing the same rows for the write side.

package schemas

import (
	"time"

	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// UserRoleGrant is one row of the DIRECT path: a role held outright, and one
// permission it confers.
//
// A ROLE HOLDING THREE PERMISSIONS ARRIVES AS THREE ROWS, and a role holding none
// arrives as one row with nil grants — which is what keeps a role the user holds
// in the answer whatever happened to what it confers.
type UserRoleGrant struct {
	ID     domain.ID
	RoleID domain.ID

	// Declared so the hop out of a role can join on it. Never mapped as a join
	// FIELD and never read — a join field carries no domain type, and nothing
	// here wants the value.
	HopPermissionID domain.ID

	// From `roles`, the first hop.
	RoleKey        string
	RoleName       string
	RoleArchivedAt *time.Time

	// From `role_permissions`, the 1:N fan-out. Nil when the role confers nothing.
	GrantArchivedAt *time.Time
	// From `permissions`, one hop further out.
	Resource             *string
	Action               *string
	PermissionArchivedAt *time.Time
}

// UserRoleGrantSchema anchors the direct path on `user_roles`, keyed by the
// member — which is the index every sign-in enters by.
func UserRoleGrantSchema() *core.TableSchema {
	return core.NewDirectSchema[UserRoleGrant]("user_roles").
		ID("id").
		ParentID("user_id").
		Field("RoleID", "role_id").
		DeletedAt("deleted_at")
}

func directRolesTarget() *core.TableSchema {
	return core.NewDirectSchema[UserRoleGrant]("roles").
		ID("id").
		Field("RoleKey", "role_key").
		Field("RoleName", "name").
		DeletedAt("deleted_at")
}

// directGrantsTarget is `role_permissions` reached FROM a role, and the
// declaration that makes it 1:N: ID("role_id") renders the join as
// `role_permissions.role_id = roles.id`, so one role fans out to its grants.
func directGrantsTarget() *core.TableSchema {
	return core.NewDirectSchema[UserRoleGrant]("role_permissions").
		ID("role_id").
		Field("HopPermissionID", "permission_id").
		DeletedAt("deleted_at")
}

func directCatalogTarget() *core.TableSchema {
	return core.NewDirectSchema[UserRoleGrant]("permissions").
		ID("id").
		Field("Resource", "resource_name").
		Field("Action", "action_name").
		DeletedAt("deleted_at")
}

// The traversals the reader declares, exported as ONE call per path so the join
// chain and the schemas that shape it stay in the same file.
func DirectGrantJoins() (roles, grants, catalog *core.TableSchema) {
	return directRolesTarget(), directGrantsTarget(), directCatalogTarget()
}
