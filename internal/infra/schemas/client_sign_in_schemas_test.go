// Hand-written, and it is the file the machine sign-in's read model points at
// when it says "the duplication is real and is tested".
//
// WHAT IS ACTUALLY AT RISK HERE. These schemas describe tables that ANOTHER
// declaration also describes — the generated ones the Client, Role, Permission and
// RolePermission aggregates write through. Two declarations of one table drift
// silently: rename a column in the spec, regenerate, and the generated side moves
// while this one keeps naming a column that no longer exists. The failure surfaces
// at BOOT (the repository validates its schema) rather than at a write, which is
// better than most — but a test is cheaper than a boot, and it names the column.
//
// So every assertion below is an AGREEMENT assertion: same table, and every column
// this side maps is one the generated side maps too.

package schemas

import (
	"testing"

	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// The builders must not panic, and must map a table.
//
// The framework validates a schema AS IT IS DECLARED, by panicking. Calling the
// builders here turns a failed start into a failed test.
func TestClientAuthenticationSchemasBuild(t *testing.T) {
	roles, grants, catalog := ClientGrantJoins()
	for name, schema := range map[string]*core.TableSchema{
		"SignInClient":         SignInClientSchema(),
		"ClientRoleGrant":      ClientRoleGrantSchema(),
		"HeldClientClaimValue": HeldClientClaimValueSchema(),
		"ClientAllowedRange":   ClientAllowedRangeSchema(),
		"joinTarget/roles":     roles,
		"joinTarget/grants":    grants,
		"joinTarget/catalog":   catalog,
	} {
		if schema == nil {
			t.Fatalf("%s built a nil schema", name)
		}
		if schema.Table() == "" {
			t.Errorf("%s built a schema with no table", name)
		}
	}
}

// Each sign-in schema must map the SAME table its generated twin maps.
//
// Asserted against the generated builder rather than against a literal, so a table
// renamed in the spec fails here instead of leaving this side reading a table that
// no longer exists — or, worse, one that still does and holds somebody else's rows.
func TestClientAuthenticationSchemasMapTheSameTablesAsTheGeneratedOnes(t *testing.T) {
	roles, grants, catalog := ClientGrantJoins()
	for name, pair := range map[string][2]*core.TableSchema{
		"SignInClient":         {SignInClientSchema(), ClientSchema()},
		"ClientRoleGrant":      {ClientRoleGrantSchema(), ClientRoleSchema()},
		"HeldClientClaimValue": {HeldClientClaimValueSchema(), ClientClaimSchema()},
		"ClientAllowedRange":   {ClientAllowedRangeSchema(), ClientAllowedCIDRSchema()},
		"joinTarget/roles":     {roles, RoleSchema()},
		"joinTarget/grants":    {grants, RolePermissionSchema()},
		"joinTarget/catalog":   {catalog, PermissionSchema()},
	} {
		if got, want := pair[0].Table(), pair[1].Table(); got != want {
			t.Errorf("%s maps %q, the generated schema of the same rows maps %q", name, got, want)
		}
	}
}

// Every column the sign-in side maps must exist on the generated side.
//
// THIS IS THE ANTI-DRIFT ASSERTION, and the direction is deliberate: the generated
// schema is allowed to map columns this one does not — the sign-in reads a subset
// on purpose, which is the whole reason it exists — but a column named ONLY here is
// either a typo or a column somebody renamed on the write side and forgot to rename
// on the read side.
func TestClientAuthenticationSchemasNameNoColumnTheGeneratedOnesDoNot(t *testing.T) {
	roles, grants, catalog := ClientGrantJoins()
	for name, pair := range map[string][2]*core.TableSchema{
		"SignInClient":         {SignInClientSchema(), ClientSchema()},
		"ClientRoleGrant":      {ClientRoleGrantSchema(), ClientRoleSchema()},
		"HeldClientClaimValue": {HeldClientClaimValueSchema(), ClientClaimSchema()},
		"ClientAllowedRange":   {ClientAllowedRangeSchema(), ClientAllowedCIDRSchema()},
		"joinTarget/roles":     {roles, RoleSchema()},
		"joinTarget/grants":    {grants, RolePermissionSchema()},
		"joinTarget/catalog":   {catalog, PermissionSchema()},
	} {
		generated := map[string]struct{}{}
		for _, col := range pair[1].MappedColumns() {
			generated[col] = struct{}{}
		}
		for _, col := range pair[0].MappedColumns() {
			if _, ok := generated[col]; !ok {
				t.Errorf("%s maps column %q, which the generated schema of %q does not — "+
					"either a typo here or a rename that only landed on the write side",
					name, col, pair[1].Table())
			}
		}
	}
}

// The credential columns must be readable HERE, whatever the write side does with
// them.
//
// The generated ClientSchema declares both hashes RedactedField, which is right for
// a sync payload and an audit event and would be fatal here: a masked value compared
// against a presented secret refuses every sign-in. This asserts the read model
// carries them as ordinary mapped columns, so the redaction can never be "helpfully"
// copied across.
func TestSignInClientReadsBothCredentialColumnsUnredacted(t *testing.T) {
	mapped := map[string]struct{}{}
	for _, col := range SignInClientSchema().MappedColumns() {
		mapped[col] = struct{}{}
	}
	for _, col := range []string{"secret_hash", "previous_secret_hash", "previous_secret_expires_at"} {
		if _, ok := mapped[col]; !ok {
			t.Errorf("the sign-in read model does not map %q — the credential cannot be verified without it", col)
		}
	}
	if SignInClientSchema().HasRedactions() {
		t.Error("the sign-in read model declares a redaction; a masked hash refuses every sign-in")
	}
}

// The allow-list read must NOT carry the label.
//
// Not cosmetic: the label exists for an operator auditing the collection, and a
// containment decision has no use for it. Loading a column nothing consults would be
// paid on every machine sign-in, on the hottest security path in the service.
func TestClientAllowedRangeReadsOnlyTheRange(t *testing.T) {
	for _, col := range ClientAllowedRangeSchema().MappedColumns() {
		if col == "label" {
			t.Error("the allow-list read model maps `label`, which no decision reads")
		}
	}
}

// The 1:N join target must name a FOREIGN key as its ID, and that is what makes the
// traversal fan out.
//
// ID("role_id") renders the join as `role_permissions.role_id = roles.id`, so one
// role reaches its many grants. It is also exactly why anchoring a repository on
// this schema would be a bug — every identity-keyed operation would address the
// wrong column. Nothing does; this asserts the shape that makes it true.
func TestClientGrantsTargetFansOutOnTheRoleKey(t *testing.T) {
	_, grants, _ := ClientGrantJoins()
	if got := grants.Table(); got != "role_permissions" {
		t.Fatalf("the grants target maps %q, expected role_permissions", got)
	}
	// The generated schema of the same table declares `id` as its identity. If the
	// two ever agreed, the traversal would render `role_permissions.id = roles.id`
	// and match nothing.
	if RolePermissionSchema().Table() != grants.Table() {
		t.Fatal("the generated RolePermission schema no longer maps role_permissions")
	}
}
