// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to ONE endpoint.
//
// THE GRANT WALK, and the one file where the traversal's shape is decided.
//
// ANCHORED ON `client_roles`, NOT ON `roles`, and that is the whole performance
// story. Anchoring on the role catalog and filtering with `id IN (subquery)` reads
// well and plans badly: Postgres turns the IN into a hashed SubPlan and seq-scans the
// catalog. Anchored where the index is, client_roles → roles → role_permissions →
// permissions is four index scans.
//
// ONE GRANT PATH, NOT TWO. A user reaches roles twice — directly and through a group
// — so the user's read declares two anchors. A Client has no groups (the model gate
// refused them outright), so there is exactly one traversal here.
//
// EVERY ARCHIVE STAMP IS A MAPPED COLUMN, and the filtering happens in GO rather
// than in the predicate: a declared traversal is NOT gated on the archived state of
// its target, and the row is a ROLE-AND-GRANT pair — a `WHERE grant.archived_at IS
// NULL` would drop the role along with what it confers, and a role whose every
// permission was revoked is still a role the client holds.
//
// THE THREE TARGETS ARE JOIN TARGETS AND NOTHING ELSE. `clientGrantsTarget` names a
// FOREIGN key as its ID — that is what makes the traversal fan out 1:N — so anchoring
// a repository on it would key every operation on the wrong column. Nothing does; the
// test next door asserts it.
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

// ClientRoleGrant is one row of the grant graph: a role this client holds, and one
// permission it confers.
//
// A ROLE HOLDING THREE PERMISSIONS ARRIVES AS THREE ROWS, and a role holding none
// arrives as one row with nil grants — which is what keeps a role the client holds
// in the answer whatever happened to what it confers.
type ClientRoleGrant struct {
	ID     domain.ID
	RoleID domain.ID

	// Declared so the hop out of a role can join on it. Never mapped as a join
	// FIELD and never read — a join field carries no domain type, and nothing here
	// wants the value.
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

// ClientRoleGrantSchema anchors the grant path on `client_roles`, keyed by the
// client — which is the index every machine sign-in enters by.
func ClientRoleGrantSchema() *core.TableSchema {
	return core.NewDirectSchema[ClientRoleGrant]("client_roles").
		ID("id").
		ParentID("client_id").
		Field("RoleID", "role_id").
		ArchivedAt("archived_at")
}

func clientRolesTarget() *core.TableSchema {
	return core.NewDirectSchema[ClientRoleGrant]("roles").
		ID("id").
		Field("RoleKey", "role_key").
		Field("RoleName", "name").
		ArchivedAt("archived_at")
}

// clientGrantsTarget is `role_permissions` reached FROM a role, and the declaration
// that makes it 1:N: ID("role_id") renders the join as
// `role_permissions.role_id = roles.id`, so one role fans out to its grants.
func clientGrantsTarget() *core.TableSchema {
	return core.NewDirectSchema[ClientRoleGrant]("role_permissions").
		ID("role_id").
		Field("HopPermissionID", "permission_id").
		ArchivedAt("archived_at")
}

func clientCatalogTarget() *core.TableSchema {
	return core.NewDirectSchema[ClientRoleGrant]("permissions").
		ID("id").
		Field("Resource", "resource_name").
		Field("Action", "action_name").
		ArchivedAt("archived_at")
}

// ClientGrantJoins is the traversal the reader declares, exported as ONE call so
// the join chain and the schemas that shape it stay in the same file.
func ClientGrantJoins() (roles, grants, catalog *core.TableSchema) {
	return clientRolesTarget(), clientGrantsTarget(), clientCatalogTarget()
}
