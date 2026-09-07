// Hand-written, and deliberately NOT generated: no spec declares these.
//
// The two claim edge tables, declared a SECOND time — as DIRECT schemas, anchored
// on themselves rather than on the aggregate that owns them.
//
// WHY A SECOND DECLARATION EXISTS AT ALL. user_claims and client_claims are
// collections of User and Client, and their generated schemas next door
// (user_claim_schema.go, client_claim_schema.go) describe them in that role: a
// child, reached through its root, load-only in a criteria. Claim asks the
// opposite question — "does any live entry point at this definition" — and that
// question has no root to enter through. core.NewDirectSchema is the framework's
// answer for a table read on its own terms, and read.NewDirectRepository refuses
// any anchor that is not one (`IsDirect`), so the child schema cannot be reused
// here however much the two agree.
//
// WHY THE ROW TYPES ARE NOT THE AGGREGATE'S VALUE OBJECTS. A Direct schema
// requires an exported `ID domain.ID` on its anchored type — a Direct row has no
// SetID, so the id is scanned like any other column. aggregatevos.UserClaim
// deliberately has no such field ("an exported ID here would compile, never be
// persisted, and never come back on a read"), and adding one to satisfy this file
// would corrupt the aggregate path to serve the probe. So the row types below are
// the minimum this read needs and nothing else.
//
// THE DUPLICATION IS REAL AND IS TESTED. Two declarations of one table can drift,
// and drift here is silent: a probe reading a renamed column answers "nobody holds
// it" and lets a narrowing through. claim_edge_schemas_test.go asserts each
// pair agrees on the table and on every column they both name, so a change to one
// side that forgets the other fails the suite instead of the production answer.

package schemas

import (
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// UserClaimEdge is one row of user_claims as the held-value probe reads it: the
// identity the framework requires of a Direct row, and the column the question is
// about.
//
// The owner is ABSENT on purpose. user_id is the child's ParentID, injected by
// infra on the aggregate path and carried by no Go field, so there is nothing to
// map it to — and nothing here needs it: "does anybody hold this" does not care
// who. Value, ClaimName and ClaimValueType are absent for the same reason.
type UserClaimEdge struct {
	ID      domain.ID
	ClaimID domain.ID
}

// UserClaimEdgeSchema maps UserClaimEdge to user_claims for a Direct read.
//
// ArchivedAt is declared even though no field carries it: the scope gate reads the
// COLUMN off the schema, which is what makes `active` the query's default and
// keeps the archive predicate out of every call site. Dropping it would silently
// widen every probe to include removed entries — the one mistake this table's
// softRemove exists to prevent.
func UserClaimEdgeSchema() *core.TableSchema {
	return core.NewDirectSchema[UserClaimEdge]("user_claims").
		ID("id").
		Field("ClaimID", "claim_id").
		ArchivedAt("archived_at")
}
