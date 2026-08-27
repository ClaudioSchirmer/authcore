// Tests for the effective-permission statement.
//
// WHAT IS ACTUALLY BEING PROTECTED HERE is not SQL formatting. It is that every
// hop of the grant graph carries its archive gate, and that both grant paths are
// present. A statement that quietly lost the `deleted_at IS NULL` on
// role_permissions would keep working, keep passing an integration test written
// against clean data, and hand out permissions an operator had revoked — which is
// the worst outcome this file can produce and the reason these assertions read
// the statement rather than trusting it.
//
// The statement is composed from the TableSchema declarations, so the assertions
// below ask the SAME schemas for the names they expect. A rename that moves both
// therefore keeps the test green (correctly — nothing broke), while a rename that
// moves only one fails at construction, which is where mustColumn panics.

package infra

import (
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// ── a dialect that renders nothing surprising ───────────────────────────────

// testDialect renders Postgres-shaped SQL, and ONLY the methods the statement
// builder actually calls do anything. The rest panic rather than return a zero
// value: if the builder ever starts calling one of them, this test should say so
// loudly instead of silently exercising a different statement than production.
type testDialect struct{}

func (testDialect) Placeholder(n int) string { return "$" + itoa(n) }
func (testDialect) QuoteIdent(name string) string {
	return `"` + name + `"`
}
func (testDialect) EncodeArg(val any) any { return val }
func (testDialect) ApplyLimit(sql string, n int) string {
	return sql + " LIMIT " + itoa(n)
}

func (testDialect) DecodeID(string) (string, error)   { panic("unexpected DecodeID") }
func (testDialect) ILikeClause(string, string) string { panic("unexpected ILikeClause") }
func (testDialect) LikeClause(string, string) string  { panic("unexpected LikeClause") }
func (testDialect) NowExpr() string                   { panic("unexpected NowExpr") }
func (testDialect) ApplyLimitOffset(string, int, int) string {
	panic("unexpected ApplyLimitOffset")
}
func (testDialect) Savepoint(string) string                { panic("unexpected Savepoint") }
func (testDialect) RollbackToSavepoint(string) string      { panic("unexpected RollbackToSavepoint") }
func (testDialect) ReleaseSavepoint(string) string         { panic("unexpected ReleaseSavepoint") }
func (testDialect) IsUniqueViolation(error) (string, bool) { panic("unexpected IsUniqueViolation") }
func (testDialect) IsForeignKeyViolation(error) (string, bool) {
	panic("unexpected IsForeignKeyViolation")
}
func (testDialect) BuildUpsert(string, []string, []string, []core.UpsertSet) string {
	panic("unexpected BuildUpsert")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

var _ core.Dialect = testDialect{}

// ── the assertions ──────────────────────────────────────────────────────────

// THE ONE THAT MATTERS. Every node in the grant graph must be archive-gated, and
// each gate is checked against the alias it belongs to — a gate on the wrong
// alias would read as present while filtering the wrong table.
func TestEffectivePermissionsStatement_GatesEveryHop(t *testing.T) {
	stmt := buildEffectivePermissionsStatement(testDialect{})

	gates := map[string]string{
		"roles":            "r",
		"role_permissions": "rp",
		"permissions":      "p",
		"user_roles":       "ur",
		"group_roles":      "gr",
		"user_groups":      "ug",
		"groups":           "g",
	}
	for table, alias := range gates {
		want := alias + `."deleted_at" IS NULL`
		if !strings.Contains(stmt, want) {
			t.Errorf("no archive gate for %s: expected %q in\n%s\n\n"+
				"a missing gate hands out permissions the operator believes were revoked",
				table, want, stmt)
		}
	}
}

// Both grant paths have to be there. Losing the group branch would silently
// reduce every group member to their direct grants — a permission loss that looks
// like a data problem, not a code one.
func TestEffectivePermissionsStatement_CarriesBothGrantPaths(t *testing.T) {
	stmt := buildEffectivePermissionsStatement(testDialect{})

	if !strings.Contains(stmt, `"user_roles" ur`) {
		t.Error("the direct grant path is missing")
	}
	if !strings.Contains(stmt, `"group_roles" gr`) || !strings.Contains(stmt, `"user_groups" ug`) {
		t.Error("the group grant path is missing")
	}
	// Two IN branches over the anchor, joined by OR — not a single one.
	if strings.Count(stmt, " IN (") != 2 {
		t.Errorf("expected exactly two grant branches, got %d in\n%s", strings.Count(stmt, " IN ("), stmt)
	}
	if !strings.Contains(stmt, " OR ") {
		t.Error("the two branches are not unioned")
	}
}

// The permission join is a LEFT JOIN so a role that currently grants nothing is
// still reported as a role the user HOLDS. An inner join here would silently drop
// it, and a consumer branching on role membership would never see it.
func TestEffectivePermissionsStatement_AnchorsOnRolesAndLeftJoinsPermissions(t *testing.T) {
	stmt := buildEffectivePermissionsStatement(testDialect{})

	if !strings.HasPrefix(stmt, "SELECT DISTINCT") {
		t.Errorf("statement does not de-duplicate: %s", stmt)
	}
	if !strings.Contains(stmt, `FROM "roles" r`) {
		t.Errorf("statement is not anchored on the roles: %s", stmt)
	}
	if strings.Count(stmt, "LEFT JOIN") != 2 {
		t.Errorf("expected the two reaches out to the permissions to be LEFT joins, got:\n%s", stmt)
	}
	if strings.Contains(stmt, `INNER JOIN "role_permissions"`) {
		t.Error("an inner join to role_permissions drops roles that grant nothing")
	}
}

// The id is bound TWICE, once per branch. Postgres would accept $1 in both
// positions; MySQL's `?` would not, and ResolveGrants passes the argument twice
// to match.
func TestEffectivePermissionsStatement_BindsTheSubjectTwice(t *testing.T) {
	stmt := buildEffectivePermissionsStatement(testDialect{})

	if !strings.Contains(stmt, "$1") || !strings.Contains(stmt, "$2") {
		t.Errorf("expected two distinct placeholders so the statement survives an engine swap:\n%s", stmt)
	}
	if strings.Count(stmt, "$1") != 1 || strings.Count(stmt, "$2") != 1 {
		t.Errorf("each placeholder should appear once:\n%s", stmt)
	}
}

// The select list is the ROLE key and the permission halves — and specifically
// the role column of user_roles, not its parent key. Those two are adjacent in
// the builder and swapping them would produce a statement that runs and returns
// the wrong rows.
func TestEffectivePermissionsStatement_SelectsTheRightColumns(t *testing.T) {
	stmt := buildEffectivePermissionsStatement(testDialect{})

	role := schemas.RoleSchema()
	permission := schemas.PermissionSchema()
	roleKey, _ := role.Resolve("Key")
	resource, _ := permission.Resolve("Resource")
	action, _ := permission.Resolve("Action")

	head := "SELECT DISTINCT " +
		`r."` + roleKey.Column + `", ` +
		`p."` + resource.Column + `", ` +
		`p."` + action.Column + `" `
	if !strings.HasPrefix(stmt, head) {
		t.Errorf("select list = %.120q…\nwant it to start with %q", stmt, head)
	}

	// The user_roles branch selects the ROLE, and filters on the PARENT key.
	userRole := schemas.UserRoleSchema()
	roleID, _ := userRole.Resolve("RoleID")
	if !strings.Contains(stmt, `SELECT ur."`+roleID.Column+`" FROM "user_roles" ur`) {
		t.Errorf("the direct branch does not select the role column:\n%s", stmt)
	}
	if !strings.Contains(stmt, `ur."`+userRole.ParentIDColumn()+`" = $1`) {
		t.Errorf("the direct branch does not filter on the parent key:\n%s", stmt)
	}
}

// Bare table aliases, never `AS alias`: Oracle rejects the latter, and the
// framework's own join renderer writes them bare for the same reason.
func TestEffectivePermissionsStatement_UsesBareAliases(t *testing.T) {
	stmt := buildEffectivePermissionsStatement(testDialect{})
	if strings.Contains(stmt, `" AS `) {
		t.Errorf("an `AS` before a table alias is rejected by Oracle:\n%s", stmt)
	}
}

// mustColumn is the guard that turns a schema rename into a boot abort rather
// than a query that matches nothing. Its panic is the feature.
func TestMustColumn_PanicsOnAFieldThatNoLongerResolves(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic — a silently unresolved field is a SELECT that returns nothing")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "NoSuchField") || !strings.Contains(msg, "roles") {
			t.Errorf("panic must name the field and the table, got: %v", r)
		}
	}()
	mustColumn(schemas.RoleSchema(), "NoSuchField")
}

// The managed archive slot has no Go field, so it resolves only through Resolve —
// which is exactly why the builder uses that and not ColumnOf.
func TestMustColumn_ResolvesTheManagedArchiveSlot(t *testing.T) {
	if got := mustColumn(schemas.RoleSchema(), "DeletedAt"); got != "deleted_at" {
		t.Errorf("DeletedAt resolved to %q, want deleted_at", got)
	}
	if _, ok := schemas.RoleSchema().ColumnOf("DeletedAt"); ok {
		t.Error("ColumnOf answering for DeletedAt would make this test meaningless — re-check which surface the builder uses")
	}
}

// ── the timing decoy ────────────────────────────────────────────────────────

// The equalisation hash must be a real, verifiable Argon2id hash that NOTHING
// matches — otherwise the burn either costs nothing (defeating its purpose) or
// could be made to return early.
func TestEqualisationHash_IsARealHashNothingMatches(t *testing.T) {
	if !strings.HasPrefix(equalisationHash, "$argon2id$") {
		t.Fatalf("equalisation hash = %.32q…, want a PHC-encoded Argon2id hash", equalisationHash)
	}
	if userHasher.Matches("timing-equalisation", equalisationHash) {
		t.Error("the decoy matched the string the burn verifies — the burn would return early")
	}
}

// An empty stored hash answers false WITHOUT hashing: a row with no credential
// authenticates nobody, and there is nothing to compare against.
func TestPasswordMatches_EmptyStoredHash(t *testing.T) {
	r := &AuthenticationReader{}
	if r.PasswordMatches("anything", "") {
		t.Error("an empty stored hash must never match")
	}
}
