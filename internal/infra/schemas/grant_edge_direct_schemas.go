// Hand-written, and deliberately NOT generated: no spec declares these.
//
// The two tables the sign-in's grant walk ANCHORS on, declared as Direct schemas.
//
// WHY ONLY TWO, when the walk touches seven tables. The other five are reached
// without an anchor of their own: `user_roles`, `user_groups`, `group_roles` and
// `groups` are read inside subqueries, which take any schema reduced with
// AsDirectSchema() and need no row type at all; and `permissions` is a join
// TARGET, which only reads. An anchor is needed only where rows are SCANNED into
// Go, and that happens twice — once for the roles, once for what they grant.
//
// WHY A SECOND DECLARATION EXISTS AT ALL. `roles` is an aggregate root and
// `role_permissions` is its child; their generated schemas next door describe them
// in that role — entered through the root, hydrated as a collection, with the
// revision guard and the write path attached. The sign-in asks a narrower
// question: which rows, and two columns of each. read.NewDirectRepository refuses
// any anchor that is not Direct, so the reduction is written here, once, where it
// can be read.
//
// WHY THE ROW TYPES ARE NOT THE DOMAIN'S. A Direct schema requires an exported
// `ID domain.ID` on its anchored type — a Direct row has no SetID, so the id is
// scanned like any other column. `*appdomain.Role` carries its id privately in
// BaseEntity and `aggregatevos.RolePermission` deliberately has none; adding one
// to either would corrupt the write path to serve a read.
//
// THE ARCHIVE STAMP OF THE JOINED TABLE IS A FIELD LIKE ANY OTHER, and that is the
// single most important thing about RolePermissionEdge. A declared join is NOT
// gated on the archived state of its target — the scope governs which rows come
// back, never which rows a traversal reaches into — so a revoked permission still
// arrives with a perfectly good resource and action. Mapping the column is what
// lets the criteria filter on it, which the framework's own guidance names as the
// honest expression. Drop the field, or the predicate that reads it, and the
// sign-in mints tokens carrying permissions the operator revoked.
//
// THE DUPLICATION IS REAL AND IS TESTED. grant_edge_direct_schemas_test.go asserts
// each pair agrees on the table and on every column they both name, so a rename on
// either side fails the suite instead of the production answer.

package schemas

import (
	"time"

	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// RoleRow is one row of `roles` as the grant walk reads it: the identity the
// framework requires, and the key the token carries.
//
// Name, description and the owning tenant are absent on purpose. The walk answers
// "which roles does this user hold", and a column read here would be a column
// scanned, allocated and discarded on the hottest security path in the platform.
type RoleRow struct {
	ID  domain.ID
	Key string
}

// RolePermissionEdge is one row of `role_permissions`, with the catalog entry it
// points at resolved through the declared traversal.
//
// Resource and Action arrive separately because that is how the catalog stores
// them and how vos.PermissionKey is built; joining them into one string here would
// put a storage decision in a read.
//
// ParentID — the owning role — carries no Go field: it is the child's parent key,
// injected by infra, and Resolve answers "ParentID" in a criteria exactly as it
// answers a declared field. The walk filters on it and never scans it.
type RolePermissionEdge struct {
	ID           domain.ID
	PermissionID domain.ID

	// Filled by the declared join into `permissions`, never persisted.
	Resource string
	Action   string
	// The joined permission's archive stamp — see the file header. Non-nil means
	// the catalog entry was retired, and a retired entry confers nothing.
	PermissionArchivedAt *time.Time
}

// RoleRowSchema maps RoleRow to `roles` for a Direct read.
//
// DeletedAt is declared because the scope gate reads the COLUMN off the schema,
// which is what makes `active` the query's default and keeps the archive predicate
// out of every call site. Dropping it would put retired roles in every token.
func RoleRowSchema() *core.TableSchema {
	return core.NewDirectSchema[RoleRow]("roles").
		ID("id").
		Field("Key", "role_key").
		DeletedAt("deleted_at")
}

// RolePermissionEdgeSchema maps RolePermissionEdge to `role_permissions`.
//
// ParentID is the owning role, which is what the walk filters on. DeletedAt gates
// a REVOKED grant — the edge's own archive stamp, distinct from the catalog entry's
// and dropped by the repository's scope rather than by a predicate.
func RolePermissionEdgeSchema() *core.TableSchema {
	return core.NewDirectSchema[RolePermissionEdge]("role_permissions").
		ID("id").
		ParentID("role_id").
		Field("PermissionID", "permission_id").
		DeletedAt("deleted_at")
}
