// Hand-written, and deliberately NOT generated: no spec declares it, and none
// should — it belongs to the sign-in path.
//
// THE INHERITED GRANT PATH: a live membership, a role that group confers, and one
// permission that role confers.
//
// TWO READS BECAUSE THERE ARE TWO PATHS, and each has its own index to enter by: the
// direct grant enters at `user_roles`, this one at `user_groups`. A single read
// reaching both would have to start above them, which is exactly the seq scan this
// shape exists to avoid. They run concurrently.
//
// IT ALSO ANSWERS THE MEMBERSHIPS. It anchors on `user_groups` and its first hop is
// the group itself, so the token's `groups` claim falls out of a read that had to
// happen anyway — one statement fewer than asking separately.
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

// UserGroupGrant is one row of the INHERITED path: a live membership, a role that
// group confers, and one permission that role confers.
//
// IT ALSO CARRIES THE MEMBERSHIP ITSELF. A user in a group that confers no role
// arrives as one row with nil role — so this read answers the token's `groups`
// claim as well, and the separate membership statement is gone.
type UserGroupGrant struct {
	ID      domain.ID
	GroupID domain.ID

	// Declared so each hop can join on the next key. Never mapped, never read.
	HopRoleID       domain.ID
	HopPermissionID domain.ID

	// From `groups`, the first hop.
	GroupKey        string
	GroupName       string
	GroupArchivedAt *time.Time

	// From `group_roles`, a 1:N fan-out. Nil when the group confers no role.
	GroupGrantArchivedAt *time.Time
	// From `roles`, one hop further out.
	RoleID         *domain.ID
	RoleKey        *string
	RoleName       *string
	RoleArchivedAt *time.Time

	// From `role_permissions`, a second 1:N fan-out, and then the catalog.
	GrantArchivedAt      *time.Time
	Resource             *string
	Action               *string
	PermissionArchivedAt *time.Time
}

// UserGroupGrantSchema anchors the inherited path on `user_groups`, keyed by the
// member.
func UserGroupGrantSchema() *core.TableSchema {
	return core.NewDirectSchema[UserGroupGrant]("user_groups").
		ID("id").
		ParentID("user_id").
		Field("GroupID", "group_id").
		DeletedAt("deleted_at")
}

func inheritedGroupsTarget() *core.TableSchema {
	return core.NewDirectSchema[UserGroupGrant]("groups").
		ID("id").
		Field("GroupKey", "group_key").
		Field("GroupName", "name").
		DeletedAt("deleted_at")
}

// inheritedGroupRolesTarget is `group_roles` reached FROM a group — the same 1:N
// move one level up: ID("group_id") renders `group_roles.group_id = groups.id`.
func inheritedGroupRolesTarget() *core.TableSchema {
	return core.NewDirectSchema[UserGroupGrant]("group_roles").
		ID("group_id").
		Field("HopRoleID", "role_id").
		DeletedAt("deleted_at")
}

func inheritedRolesTarget() *core.TableSchema {
	return core.NewDirectSchema[UserGroupGrant]("roles").
		ID("id").
		Field("RoleKey", "role_key").
		Field("RoleName", "name").
		DeletedAt("deleted_at")
}

func inheritedGrantsTarget() *core.TableSchema {
	return core.NewDirectSchema[UserGroupGrant]("role_permissions").
		ID("role_id").
		Field("HopPermissionID", "permission_id").
		DeletedAt("deleted_at")
}

func inheritedCatalogTarget() *core.TableSchema {
	return core.NewDirectSchema[UserGroupGrant]("permissions").
		ID("id").
		Field("Resource", "resource_name").
		Field("Action", "action_name").
		DeletedAt("deleted_at")
}

func InheritedGrantJoins() (groups, groupRoles, roles, grants, catalog *core.TableSchema) {
	return inheritedGroupsTarget(), inheritedGroupRolesTarget(), inheritedRolesTarget(),
		inheritedGrantsTarget(), inheritedCatalogTarget()
}
