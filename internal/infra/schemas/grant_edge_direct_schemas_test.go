// The drift guard for the second declaration of the four grant edge tables.
//
// user_roles, user_groups, group_roles and role_permissions are each declared
// TWICE — once as the aggregate child the generator writes, once as the Direct
// anchor the sign-in's grant walk reads. Two declarations of one table can
// disagree, and here the disagreement is SILENT in both directions: a walk reading
// a renamed column finds nothing and mints a token missing permissions the user
// holds, and a walk that lost the archive column mints one carrying grants the
// operator revoked.
//
// So the pairs are asserted rather than trusted. A rename on either side fails
// here, naming the table and the field.

package schemas

import (
	"testing"

	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// assertGrantEdgePair runs every agreement the walk depends on: the table, the
// primary key, the PARENT key it filters by, the archive column that makes
// `active` the default scope, and the foreign key the traversal hangs off.
func assertGrantEdgePair(t *testing.T, child, direct *core.TableSchema, fkField string) {
	t.Helper()

	if child.Table() != direct.Table() {
		t.Fatalf("the pair describes two different tables: %q and %q", child.Table(), direct.Table())
	}
	if child.IDColumn() != direct.IDColumn() {
		t.Errorf("%s: the id is %q on the aggregate child and %q on the Direct schema",
			child.Table(), child.IDColumn(), direct.IDColumn())
	}

	// THE PARENT KEY IS WHAT EVERY HOP OF THE WALK FILTERS ON — "the grants of this
	// user", "the roles of these groups". A Direct schema that lost it would answer
	// with the whole table.
	if child.ParentIDColumn() != direct.ParentIDColumn() {
		t.Errorf("%s: the parent key is %q on the aggregate child and %q on the Direct schema",
			child.Table(), child.ParentIDColumn(), direct.ParentIDColumn())
	}
	if direct.ParentIDColumn() == "" {
		t.Errorf("%s: the Direct schema declares no parent key — the walk could not filter by "+
			"the holder and would read every row of the table", direct.Table())
	}

	childDel, childHas := child.DeletedAtColumn()
	directDel, directHas := direct.DeletedAtColumn()
	switch {
	case !childHas:
		t.Errorf("%s: the aggregate child declares no archive column", child.Table())
	case !directHas:
		t.Errorf("%s: the Direct schema declares no archive column — the walk would count "+
			"revoked grants and hand out access the operator took away", direct.Table())
	case childDel != directDel:
		t.Errorf("%s: the archive column is %q on the aggregate child and %q on the Direct schema",
			child.Table(), childDel, directDel)
	}

	// The foreign key the traversal is declared on. It is named as a raw column in
	// the reader's WithJoins(...).On(...), so a rename here has to reach that call
	// too — and this is what says so.
	want, ok := child.Resolve(fkField)
	if !ok {
		t.Fatalf("%s: the aggregate child no longer resolves %q", child.Table(), fkField)
	}
	got, ok := direct.Resolve(fkField)
	if !ok {
		t.Fatalf("%s: the Direct schema no longer resolves %q", direct.Table(), fkField)
	}
	if got.Column != want.Column {
		t.Errorf("%s: %q is column %q on the aggregate child and %q on the Direct schema",
			child.Table(), fkField, want.Column, got.Column)
	}
	if !direct.IsDirect() {
		t.Errorf("%s: the second declaration is not Direct — the repository would refuse it", direct.Table())
	}
}

func TestRolePermissionEdgeSchemaAgreesWithTheAggregateChild(t *testing.T) {
	assertGrantEdgePair(t, RolePermissionSchema(), RolePermissionEdgeSchema(), "PermissionID")
}

// THE COLUMNS THE READER'S JOINS NAME, asserted where they can be. WithJoins takes
// the target's column as a raw string — the joined side's spelling never surfaces
// above infra, which is exactly why nothing else would catch a rename on the
// TARGET table.
//
// The archive column is the one that matters most: the walk filters on it because
// a traversal is NOT gated on its target's archived state, so losing it turns a
// retired role into a live grant.
func TestTheJoinTargetsStillCarryTheColumnsTheWalkNames(t *testing.T) {
	for _, tc := range []struct {
		table   *core.TableSchema
		goField string
		// The column exactly as the reader's WithJoins(...).Field(...) spells it.
		column string
	}{
		{RoleSchema(), "Key", "role_key"},
		{PermissionSchema(), "Resource", "resource_name"},
		{PermissionSchema(), "Action", "action_name"},
	} {
		resolved, ok := tc.table.Resolve(tc.goField)
		if !ok {
			t.Errorf("%s no longer resolves %q", tc.table.Table(), tc.goField)
			continue
		}
		if resolved.Column != tc.column {
			t.Errorf("%s: %q is column %q, but the reader's join names %q — the read would come "+
				"back empty, or fail at the database, on the sign-in path",
				tc.table.Table(), tc.goField, resolved.Column, tc.column)
		}
	}

	// THE ARCHIVE COLUMN OF EVERY TARGET, which the walk filters on because a
	// traversal is not gated on its target. Losing it turns a retired role, a
	// retired group or a revoked permission back into a live grant.
	for _, target := range []*core.TableSchema{RoleSchema(), GroupSchema(), PermissionSchema()} {
		col, has := target.DeletedAtColumn()
		if !has {
			t.Errorf("%s declares no archive column; the walk could not tell a retired row "+
				"from a live one", target.Table())
			continue
		}
		if col != "deleted_at" {
			t.Errorf("%s archives on %q, but the reader's join names \"deleted_at\"", target.Table(), col)
		}
	}
}
