// Hand-written, and deliberately NOT generated: no spec declares these.
//
// The four tables that carry a grant, declared a SECOND time — as DIRECT schemas,
// anchored on themselves rather than on the aggregate that owns them.
//
// WHY A SECOND DECLARATION EXISTS AT ALL. user_roles, user_groups, group_roles and
// role_permissions are collections of User, Group and Role, and their generated
// schemas next door describe them in that role: a child, reached through its root,
// load-only in a criteria. The sign-in path asks the opposite question — "which
// roles does this user hold, and what do they confer" — and that question enters
// through no root: it starts at a user id and walks OUTWARD across four tables.
// core.NewDirectSchema is the framework's answer for a table read on its own terms,
// and read.NewDirectRepository refuses any anchor that is not one, so the child
// schema cannot be reused here however much the two agree.
//
// WHY THE ROW TYPES ARE NOT THE AGGREGATE'S VALUE OBJECTS. A Direct schema requires
// an exported `ID domain.ID` on its anchored type — a Direct row has no SetID, so
// the id is scanned like any other column. The aggregatevos types deliberately have
// no such field, and adding one to satisfy this file would corrupt the aggregate
// path to serve a read.
//
// THE ARCHIVE COLUMN OF THE JOINED TABLE IS A FIELD LIKE ANY OTHER, and that is the
// single most important thing about these types. A declared join is NOT gated on the
// archived state of its target — the read scope governs which ROWS COME BACK, never
// which rows a traversal reaches into — so a retired role still arrives through the
// foreign key that points at it. The framework's own guidance is that the honest
// expression is a filter the criteria states, which is why every one of these types
// carries the target's `deleted_at` under a name the resolver can filter on. Drop one
// of those fields, or the predicate that reads it, and the sign-in hands out
// permissions the operator believes they took away.
//
// THE DUPLICATION IS REAL AND IS TESTED. Two declarations of one table can drift, and
// drift here is silent in the worst direction: a probe reading a renamed column finds
// nothing and mints a token with fewer permissions than the user holds — or, on the
// archive column, more. grant_edge_direct_schemas_test.go asserts each pair agrees on
// the table and on every column they both name.

package schemas

import (
	"time"

	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// UserRoleEdge is one row of user_roles as the grant walk reads it: the role the
// entry points at, plus what the traversal into `roles` brings back about it.
//
// The owner is ABSENT as a field on purpose. user_id is the child's ParentID,
// injected by infra and carried by no Go field — and it needs none: Resolve answers
// "ParentID" in a criteria exactly as it answers a declared field.
type UserRoleEdge struct {
	ID     domain.ID
	RoleID domain.ID

	// Filled by the declared join into `roles`, never persisted.
	RoleKey string
	// The joined role's archive stamp. Non-nil means the role was retired, and a
	// retired role confers nothing — see the file header for why the join cannot
	// answer this on its own.
	RoleArchivedAt *time.Time
}

// GroupRoleEdge is UserRoleEdge's twin over group_roles: the same question one hop
// further out, where the holder is a group rather than a user.
//
// Two types and not one shared: a Direct repository cross-checks its type parameter
// against the schema's anchored type — one schema, one row type — so a single struct
// behind both tables would be refused at construction.
type GroupRoleEdge struct {
	ID     domain.ID
	RoleID domain.ID

	RoleKey        string
	RoleArchivedAt *time.Time
}

// UserGroupEdge is one row of user_groups: the group the membership points at, and
// whether that group is still live.
//
// It carries no key and no name. This edge exists only to turn a user id into the
// set of group ids the next hop filters on; anything else read here would be read
// and discarded.
type UserGroupEdge struct {
	ID      domain.ID
	GroupID domain.ID

	// The joined group's archive stamp. A user removed from service by archiving
	// the group inherits nothing through it.
	GroupArchivedAt *time.Time
}

// RolePermissionEdge is one row of role_permissions, with the permission it points
// at resolved through the declared traversal.
//
// Resource and Action arrive separately because that is how the permission catalog
// stores them and how vos.PermissionKey is built; joining them into one string here
// would put a storage decision in a read.
type RolePermissionEdge struct {
	ID           domain.ID
	PermissionID domain.ID

	Resource string
	Action   string
	// The joined permission's archive stamp. A revoked permission confers nothing.
	PermissionArchivedAt *time.Time
}

// UserRoleEdgeSchema maps UserRoleEdge to user_roles for a Direct read.
//
// DeletedAt is declared because the scope gate reads the COLUMN off the schema,
// which is what makes `active` the query's default and keeps the archive predicate
// out of every call site. Dropping it would silently widen the walk to include
// revoked grants — the one mistake this table's soft remove exists to prevent.
func UserRoleEdgeSchema() *core.TableSchema {
	return core.NewDirectSchema[UserRoleEdge]("user_roles").
		ID("id").
		ParentID("user_id").
		Field("RoleID", "role_id").
		DeletedAt("deleted_at")
}

// GroupRoleEdgeSchema maps GroupRoleEdge to group_roles. The parent is the GROUP
// here, which is what the second hop of the inherited path filters on.
func GroupRoleEdgeSchema() *core.TableSchema {
	return core.NewDirectSchema[GroupRoleEdge]("group_roles").
		ID("id").
		ParentID("group_id").
		Field("RoleID", "role_id").
		DeletedAt("deleted_at")
}

// UserGroupEdgeSchema maps UserGroupEdge to user_groups.
func UserGroupEdgeSchema() *core.TableSchema {
	return core.NewDirectSchema[UserGroupEdge]("user_groups").
		ID("id").
		ParentID("user_id").
		Field("GroupID", "group_id").
		DeletedAt("deleted_at")
}

// RolePermissionEdgeSchema maps RolePermissionEdge to role_permissions.
func RolePermissionEdgeSchema() *core.TableSchema {
	return core.NewDirectSchema[RolePermissionEdge]("role_permissions").
		ID("id").
		ParentID("role_id").
		Field("PermissionID", "permission_id").
		DeletedAt("deleted_at")
}
