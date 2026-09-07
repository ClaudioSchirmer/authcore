// The drift guard for the second declaration of the two claim edge tables.
//
// user_claims and client_claims are each declared TWICE — once as the aggregate
// child the generator writes, once as the Direct anchor the held-value probes
// read. Two declarations of one table can disagree, and here the disagreement is
// SILENT in the worst direction: a probe reading a column the child renamed finds
// nothing, answers "nobody holds it", and clears a narrowing that strands every
// value already written for the definition.
//
// So the pair is asserted rather than trusted. A rename on either side fails here,
// naming the table and the field, instead of surfacing as a rule that quietly
// stopped refusing.

package schemas

import (
	"testing"

	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// agreeOn asserts the two declarations resolve the same physical column for one
// Go field. It goes through Resolve — the same surface every read path consults —
// so a field moved to a managed slot is compared the way a query would see it.
func agreeOn(t *testing.T, child, direct *core.TableSchema, goField string) {
	t.Helper()

	want, ok := child.Resolve(goField)
	if !ok {
		t.Fatalf("%s: the aggregate child no longer resolves %q — the Direct schema beside it "+
			"still names it, so one of the two has moved", child.Table(), goField)
	}
	got, ok := direct.Resolve(goField)
	if !ok {
		t.Fatalf("%s: the Direct schema no longer resolves %q", direct.Table(), goField)
	}
	if got.Column != want.Column {
		t.Errorf("%s: %q is column %q on the aggregate child and %q on the Direct schema",
			child.Table(), goField, want.Column, got.Column)
	}
}

// assertEdgePair runs every agreement the probe depends on: the table it reads,
// the column it filters by, the primary key, and the archive column whose presence
// is what makes `active` the query's default scope.
func assertEdgePair(t *testing.T, child, direct *core.TableSchema) {
	t.Helper()

	if child.Table() != direct.Table() {
		t.Fatalf("the pair describes two different tables: %q and %q", child.Table(), direct.Table())
	}
	if child.IDColumn() != direct.IDColumn() {
		t.Errorf("%s: the id is %q on the aggregate child and %q on the Direct schema",
			child.Table(), child.IDColumn(), direct.IDColumn())
	}

	// The archive column, asserted on BOTH sides for presence and not only for
	// agreement: a Direct schema that stopped declaring it would still compile and
	// still answer — about archived entries too, which is the one answer the two
	// collections chose softRemove to avoid.
	childDel, childHas := child.ArchivedAtColumn()
	directDel, directHas := direct.ArchivedAtColumn()
	switch {
	case !childHas:
		t.Errorf("%s: the aggregate child declares no archive column", child.Table())
	case !directHas:
		t.Errorf("%s: the Direct schema declares no archive column — every probe over it "+
			"would count removed entries", direct.Table())
	case childDel != directDel:
		t.Errorf("%s: the archive column is %q on the aggregate child and %q on the Direct schema",
			child.Table(), childDel, directDel)
	}

	agreeOn(t, child, direct, "ClaimID")
}

func TestUserClaimEdgeSchemaAgreesWithTheAggregateChild(t *testing.T) {
	assertEdgePair(t, UserClaimSchema(), UserClaimEdgeSchema())
}

func TestClientClaimEdgeSchemaAgreesWithTheAggregateChild(t *testing.T) {
	assertEdgePair(t, ClientClaimSchema(), ClientClaimEdgeSchema())
}
