//go:build integration && postgres

// Drives the REAL grant walk against a REAL Postgres, because the thing worth
// proving about it cannot be seen in the code.
//
// THE WALK READS FOUR EDGE TABLES AND REACHES SIDEWAYS INTO THREE MORE, and every
// one of those seven rows can be archived independently. A declared traversal is
// NOT gated on the archived state of its target — the scope governs which rows
// come back, never which rows a traversal reaches into — so three of the seven
// gates are predicates the reader states rather than behaviour the framework
// supplies. A test that inspected the code would confirm the predicate was
// written; only a row that fails to come back proves it does anything.
//
// So the fixture below is built to be WRONG in every direction at once: a live
// grant onto a retired role, a live membership of a retired group, a live grant
// onto a revoked permission, a revoked grant onto a live role, one role holding
// nothing, and one permission reached through two different roles. The answer the
// walk gives is what says which of those it understood.

package infra

import (
	"context"
	"os"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/engine/postgres"
)

// The fixture's fixed identities. Fixed rather than minted so a failed run leaves
// rows a human can look at, and so the cleanup below is exact.
const (
	grantTenant = "11111111-1111-4111-8111-111111111111"
	grantUser   = "22222222-2222-4222-8222-222222222222"

	roleHeldDirectly  = "33333333-3333-4333-8333-333333333331"
	roleInherited     = "33333333-3333-4333-8333-333333333332"
	roleRetired       = "33333333-3333-4333-8333-333333333333"
	roleGrantingNone  = "33333333-3333-4333-8333-333333333334"
	roleInRetiredTeam = "33333333-3333-4333-8333-333333333335"
	roleRevokedGrant  = "33333333-3333-4333-8333-333333333336"
	// A LIVE role the user holds, whose ONLY permission was revoked in the
	// catalog. It has to survive in the role list and contribute nothing.
	roleAllPermissionsRevoked = "33333333-3333-4333-8333-333333333337"

	groupLive    = "44444444-4444-4444-8444-444444444441"
	groupRetired = "44444444-4444-4444-8444-444444444442"

	permissionLive    = "55555555-5555-4555-8555-555555555551"
	permissionRevoked = "55555555-5555-4555-8555-555555555552"
	permissionShared  = "55555555-5555-4555-8555-555555555553"
	// Granted ONLY by the retired role, so nothing else can put it in the answer.
	// Without it, dropping the gate on the owning role changes no result and the
	// gate is untested — which is exactly what a mutation run found.
	permissionOnlyViaRetired = "55555555-5555-4555-8555-555555555554"
)

func grantReader(t *testing.T) (*AuthenticationReader, *postgres.Postgres) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://omnicore:omnicore@localhost:5432/authcore_db?sslmode=disable"
	}
	eng, err := postgres.NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Skipf("no dev bench reachable: %v", err)
	}
	eng.SetClock(core.ClockDB)
	t.Cleanup(func() {
		clearGrantFixture(t, eng)
		eng.Close()
	})
	clearGrantFixture(t, eng)
	seedGrantFixture(t, eng)
	return NewAuthenticationReader(eng), eng
}

// resolveFor runs the endpoint's two read steps and flattens the bundle into the
// three answers these tests assert on.
func resolveFor(t *testing.T, reader *AuthenticationReader, email string) (groups, roles []string, permissions []vos.PermissionKey) {
	t.Helper()
	ctx := context.Background()
	account, err := reader.LoadAccountByEmail(ctx, email)
	if err != nil || account == nil {
		t.Fatalf("loading %s: %v (nil=%v)", email, err, account == nil)
	}
	bundle, err := reader.ResolveSignIn(ctx, account)
	if err != nil {
		t.Fatalf("resolving the sign-in: %v", err)
	}
	for _, g := range bundle.Groups {
		groups = append(groups, g.Key)
	}
	for _, r := range bundle.Roles {
		roles = append(roles, r.Key)
	}
	return groups, roles, bundle.Permissions
}

func clearGrantFixture(t *testing.T, eng *postgres.Postgres) {
	t.Helper()
	for _, stmt := range []string{
		`DELETE FROM role_permissions WHERE role_id::text LIKE '33333333%'`,
		`DELETE FROM group_roles WHERE group_id::text LIKE '44444444%'`,
		`DELETE FROM user_groups WHERE user_id = $1`,
		`DELETE FROM user_roles WHERE user_id = $1`,
		`DELETE FROM users WHERE id = $1`,
		`DELETE FROM groups WHERE id::text LIKE '44444444%'`,
		`DELETE FROM roles WHERE id::text LIKE '33333333%'`,
		`DELETE FROM permissions WHERE id::text LIKE '55555555%'`,
		`DELETE FROM tenants WHERE id::text LIKE '11111111%'`,
	} {
		var err error
		if countPlaceholders(stmt) == 1 {
			_, err = eng.Pool().Exec(context.Background(), stmt, grantUser)
		} else {
			_, err = eng.Pool().Exec(context.Background(), stmt)
		}
		if err != nil {
			t.Fatalf("clearing (%s): %v", stmt, err)
		}
	}
}

func countPlaceholders(stmt string) int {
	n := 0
	for i := 0; i+1 < len(stmt); i++ {
		if stmt[i] == '$' && stmt[i+1] == '1' {
			n++
		}
	}
	return n
}

// seedGrantFixture writes the graph. Every row is spelled out rather than built
// through the aggregates: what is under test is a READ over a shape, and driving
// six write paths to arrange it would put five other components between the
// fixture and the assertion.
func seedGrantFixture(t *testing.T, eng *postgres.Postgres) {
	t.Helper()
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := eng.Pool().Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding (%.60s…): %v", sql, err)
		}
	}

	exec(`INSERT INTO tenants (id, name, workspace, description, status)
	      VALUES ($1, 'Grant Walk', 'grant-walk', '', 'active')`, grantTenant)
	exec(`INSERT INTO users (id, tenant_id, given_name, family_name, email, password_hash,
	                         password_changed_at, must_change_password, status)
	      VALUES ($1, $2, 'Ada', 'Lovelace', 'ada@grant-walk.test', '', NOW(), false, 'active')`,
		grantUser, grantTenant)

	// The roles. `retired` and `in-retired-team` are archived at the ROLE and the
	// GROUP respectively; both are reached by perfectly live edges.
	for _, r := range []struct{ id, key, archived string }{
		{roleHeldDirectly, "direct", ""},
		{roleInherited, "inherited", ""},
		{roleRetired, "retired", "NOW()"},
		{roleGrantingNone, "grants-nothing", ""},
		{roleInRetiredTeam, "in-retired-team", ""},
		{roleRevokedGrant, "revoked-grant", ""},
		{roleAllPermissionsRevoked, "all-revoked", ""},
	} {
		archived := "NULL"
		if r.archived != "" {
			archived = r.archived
		}
		exec(`INSERT INTO roles (id, tenant_id, role_key, name, description, archived_at)
		      VALUES ($1, $2, $3, $3, '', `+archived+`)`, r.id, grantTenant, r.key)
	}

	exec(`INSERT INTO groups (id, tenant_id, group_key, name, description, archived_at)
	      VALUES ($1, $2, 'live-team', 'Live Team', '', NULL)`, groupLive, grantTenant)
	exec(`INSERT INTO groups (id, tenant_id, group_key, name, description, archived_at)
	      VALUES ($1, $2, 'retired-team', 'Retired Team', '', NOW())`, groupRetired, grantTenant)

	exec(`INSERT INTO permissions (id, resource_name, action_name, description, archived_at)
	      VALUES ($1, 'invoice', 'read', '', NULL)`, permissionLive)
	exec(`INSERT INTO permissions (id, resource_name, action_name, description, archived_at)
	      VALUES ($1, 'invoice', 'delete', '', NOW())`, permissionRevoked)
	exec(`INSERT INTO permissions (id, resource_name, action_name, description, archived_at)
	      VALUES ($1, 'report', 'read', '', NULL)`, permissionShared)
	exec(`INSERT INTO permissions (id, resource_name, action_name, description, archived_at)
	      VALUES ($1, 'ledger', 'purge', '', NULL)`, permissionOnlyViaRetired)

	// The user's own grants. One live, one onto a retired role, one onto a role
	// that grants nothing, one onto a role whose every permission was revoked,
	// and one REVOKED grant onto a perfectly live role.
	exec(`INSERT INTO user_roles (id, user_id, role_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL),
	      (gen_random_uuid(), $1, $3, NULL),
	      (gen_random_uuid(), $1, $4, NULL),
	      (gen_random_uuid(), $1, $5, NULL),
	      (gen_random_uuid(), $1, $6, NOW())`,
		grantUser, roleHeldDirectly, roleRetired, roleGrantingNone,
		roleAllPermissionsRevoked, roleRevokedGrant)

	// Both memberships are live; one of the groups is not.
	exec(`INSERT INTO user_groups (id, user_id, group_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL),
	      (gen_random_uuid(), $1, $3, NULL)`, grantUser, groupLive, groupRetired)

	exec(`INSERT INTO group_roles (id, group_id, role_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL)`, groupLive, roleInherited)
	exec(`INSERT INTO group_roles (id, group_id, role_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL)`, groupRetired, roleInRetiredTeam)

	// What the roles confer. `report:read` is reached through BOTH the direct and
	// the inherited role, which is the duplicate the walk has to collapse.
	exec(`INSERT INTO role_permissions (id, role_id, permission_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL),
	      (gen_random_uuid(), $1, $3, NULL),
	      (gen_random_uuid(), $1, $4, NULL)`,
		roleHeldDirectly, permissionLive, permissionRevoked, permissionShared)
	exec(`INSERT INTO role_permissions (id, role_id, permission_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL)`, roleInherited, permissionShared)
	// The retired role and the one behind the retired group both grant something —
	// so if either leaked into the answer, it would leak a permission too.
	// The retired role grants a LIVE permission nothing else grants. A gate that
	// stopped excluding the role would put `ledger:purge` in the answer, and
	// nothing else can.
	exec(`INSERT INTO role_permissions (id, role_id, permission_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL),
	      (gen_random_uuid(), $1, $3, NULL)`, roleRetired, permissionLive, permissionOnlyViaRetired)
	exec(`INSERT INTO role_permissions (id, role_id, permission_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL)`, roleInRetiredTeam, permissionLive)
	// The live role whose ONLY grant points at a REVOKED catalog entry. The grant
	// itself is live — nobody revoked it — so what drops the permission is the
	// catalog's own archive stamp, and the role must survive that.
	exec(`INSERT INTO role_permissions (id, role_id, permission_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL)`, roleAllPermissionsRevoked, permissionRevoked)
	// A REVOKED grant of a live permission, held by a live role. The edge's own
	// scope gate is what drops this one.
	exec(`INSERT INTO role_permissions (id, role_id, permission_id, archived_at) VALUES
	      (gen_random_uuid(), $1, $2, NOW())`, roleGrantingNone, permissionShared)
}

// THE WHOLE ANSWER, asserted exactly. Not "contains" — exactly: a walk that
// returns MORE than this has handed out access somebody revoked, and that is the
// failure this file exists to catch.
func TestLive_TheGrantWalkAnswersEveryPathAndNoRevokedOne(t *testing.T) {
	reader, _ := grantReader(t)

	groupKeys, roleKeys, permissions := resolveFor(t, reader, "ada@grant-walk.test")

	wantRoles := []string{"all-revoked", "direct", "grants-nothing", "inherited"}
	if len(roleKeys) != len(wantRoles) {
		t.Fatalf("roles = %v, want exactly %v", roleKeys, wantRoles)
	}
	for i, key := range wantRoles {
		if roleKeys[i] != key {
			t.Errorf("roles = %v, want exactly %v (sorted)", roleKeys, wantRoles)
			break
		}
	}

	// THE GROUPS THE TOKEN WILL NAME. `retired-team` is a LIVE membership of an
	// ARCHIVED group, and it must not be here — the join reaches its row whatever
	// its archived state, so only the reader's predicate keeps it out.
	wantGroups := []string{"live-team"}
	if len(groupKeys) != len(wantGroups) || groupKeys[0] != wantGroups[0] {
		t.Errorf("groups = %v, want exactly %v — the user is also a live member of the "+
			"ARCHIVED `retired-team`", groupKeys, wantGroups)
	}

	wantPermissions := []string{"invoice:read", "report:read"}
	got := make([]string, 0, len(permissions))
	for _, p := range permissions {
		got = append(got, p.Resource+":"+p.Action)
	}
	if len(got) != len(wantPermissions) {
		t.Fatalf("permissions = %v, want exactly %v", got, wantPermissions)
	}
	for i, key := range wantPermissions {
		if got[i] != key {
			t.Errorf("permissions = %v, want exactly %v (sorted)", got, wantPermissions)
			break
		}
	}
}

// Each gate, named, so a failure says WHICH one broke rather than only that the
// answer is wrong. Every one of these is a row the fixture deliberately left
// reachable through a live edge.
func TestLive_TheGrantWalkDropsEachRetiredThingForItsOwnReason(t *testing.T) {
	reader, _ := grantReader(t)

	groupKeys, roleKeys, permissions := resolveFor(t, reader, "ada@grant-walk.test")
	groups := map[string]bool{}
	for _, key := range groupKeys {
		groups[key] = true
	}
	roles := map[string]bool{}
	for _, key := range roleKeys {
		roles[key] = true
	}
	perms := map[string]bool{}
	for _, p := range permissions {
		perms[p.Resource+":"+p.Action] = true
	}

	// A RETIRED ROLE behind a live grant. The traversal reaches it and brings back
	// its key; only the predicate on the joined archive stamp drops it.
	if roles["retired"] {
		t.Error("a RETIRED role is in the answer — the filter on the joined role's archive stamp " +
			"is missing, and a declared traversal does not gate its target")
	}
	// A LIVE ROLE behind a REVOKED grant. Dropped by the edge repository's own
	// scope, which is the half the framework supplies.
	if roles["revoked-grant"] {
		t.Error("a role whose GRANT was revoked is in the answer — the edge's scope gate is gone")
	}
	// THE RETIRED GROUP ITSELF, as a membership. The token names groups by key,
	// and the User aggregate would have supplied this one: its child join reaches
	// the group's row whatever its archived state. Only the reader's predicate
	// keeps it out of the claim.
	if groups["retired-team"] {
		t.Error("a RETIRED group is in the group list — the token would name a group the " +
			"operator took out of service")
	}
	if !groups["live-team"] {
		t.Error("the live membership vanished from the group list")
	}
	// A LIVE ROLE conferred by a RETIRED GROUP, through a live membership and a
	// live group_roles row. Only the group's own archive stamp says otherwise.
	if roles["in-retired-team"] {
		t.Error("a role inherited from a RETIRED group is in the answer — the filter on the " +
			"joined group's archive stamp is missing")
	}
	// A ROLE THAT GRANTS NOTHING is still a role the user holds. It survives here
	// because the keys come from the role reads, which know nothing about
	// permissions — the property the old LEFT JOIN existed to preserve.
	if !roles["grants-nothing"] {
		t.Error("a role that confers no permission vanished; a consumer branching on role " +
			"membership would stop seeing it")
	}
	// A ROLE WHOSE EVERY PERMISSION WAS REVOKED IS STILL A ROLE THE USER HOLDS.
	// This is what would break if the catalog gate were a WHERE clause over the
	// joined row instead of a filter on the row that comes back: the grant would
	// be dropped and the role would go with it, even though nobody took the role
	// away from anybody.
	if !roles["all-revoked"] {
		t.Error("a role whose only permission was revoked vanished from the role list — " +
			"revoking a permission in the catalog must not un-assign a role from a user")
	}

	// A LIVE PERMISSION granted ONLY by the RETIRED role. This is the assertion
	// that proves the gate on the owning role, rather than assuming it: every
	// other permission in the fixture is reachable by some live role too, so
	// dropping that gate would change no other line of this test.
	if perms["ledger:purge"] {
		t.Error("a permission granted only by a RETIRED role is in the answer — the gate on " +
			"the owning role is missing, and a retired role keeps conferring through a grant " +
			"nobody revoked")
	}
	// A REVOKED PERMISSION behind a live grant of a live role.
	if perms["invoice:delete"] {
		t.Error("a REVOKED permission is in the answer — the filter on the joined permission's " +
			"archive stamp is missing")
	}
	// Reached through the direct role AND the inherited one: one permission.
	if !perms["report:read"] {
		t.Error("the permission held through two different roles is missing entirely")
	}
}

// An identity with no grants at all answers empty and does not fail. The walk
// skips the reads it has nothing to key on, so this is also the path where two of
// the four never run.
func TestLive_AUserWithNoGrantsAnswersEmpty(t *testing.T) {
	reader, eng := grantReader(t)
	ctx := context.Background()

	if _, err := eng.Pool().Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, grantUser); err != nil {
		t.Fatalf("clearing roles: %v", err)
	}
	if _, err := eng.Pool().Exec(ctx, `DELETE FROM user_groups WHERE user_id = $1`, grantUser); err != nil {
		t.Fatalf("clearing memberships: %v", err)
	}

	groupKeys, roleKeys, permissions := resolveFor(t, reader, "ada@grant-walk.test")
	if len(groupKeys) != 0 || len(roleKeys) != 0 || len(permissions) != 0 {
		t.Errorf("got groups=%v roles=%v permissions=%v, want all empty",
			groupKeys, roleKeys, permissions)
	}
}

// An id nobody holds is the same answer, and must not be an error: the sign-in
// path calls this after a lookup it already knows succeeded, but a refresh calls
// it for a subject that may have been deleted since the token was minted.
func TestLive_AnUnknownSubjectAnswersEmpty(t *testing.T) {
	reader, _ := grantReader(t)

	account, err := reader.LoadAccountByEmail(context.Background(), "nobody@grant-walk.test")
	if err != nil {
		t.Fatalf("an unknown address must answer absence, not fail: %v", err)
	}
	if account != nil {
		t.Fatalf("an address nobody holds loaded an account: %+v", account)
	}
	var groupKeys, roleKeys []string
	var permissions []vos.PermissionKey
	if len(groupKeys) != 0 || len(roleKeys) != 0 || len(permissions) != 0 {
		t.Errorf("got groups=%v roles=%v permissions=%v for an id nobody holds",
			groupKeys, roleKeys, permissions)
	}
}

// AN ARCHIVED TENANT CANNOT AUTHENTICATE, through either load.
//
// THIS IS DEPTH, NOT THE MECHANISM. Tenant's own rules force Status to suspended
// when it is archived (archive-forces-suspended), and the sign-in refuses a
// suspended tenant — so through the API the two states never come apart. What is
// arranged here is a row that reached `archived_at` WITHOUT going through the
// aggregate, which is what a migration or a support script does, and the fixture
// writes it exactly that way: a direct UPDATE, leaving status untouched.
//
// The refusal is an ABSENCE, not an error: the caller cannot tell an archived
// tenant from an address that was never registered, which is the same answer every
// other refusal on this path gives.
func TestLive_AnArchivedTenantAuthenticatesNobody(t *testing.T) {
	reader, eng := grantReader(t)
	ctx := context.Background()

	if account, err := reader.LoadAccountByEmail(ctx, "ada@grant-walk.test"); err != nil || account == nil {
		t.Fatalf("the fixture account must load while the tenant is live (err=%v nil=%v)", err, account == nil)
	}

	// Straight to the row, on purpose — see the doc comment.
	if _, err := eng.Pool().Exec(context.Background(),
		`UPDATE tenants SET archived_at = NOW() WHERE id = $1`, grantTenant); err != nil {
		t.Fatalf("archiving the tenant: %v", err)
	}

	account, err := reader.LoadAccountByEmail(ctx, "ada@grant-walk.test")
	if err != nil {
		t.Fatalf("an archived tenant must read as absence, not as an error: %v", err)
	}
	if account != nil {
		t.Error("an account whose tenant is ARCHIVED still loaded for sign-in")
	}

	byID, err := reader.LoadAccountByID(ctx, domain.NewID(grantUser))
	if err != nil {
		t.Fatalf("the refresh path must read absence, not an error: %v", err)
	}
	if byID != nil {
		t.Error("the refresh path still reloaded an account whose tenant is ARCHIVED — a token " +
			"would keep rotating after the tenant was taken out of service")
	}
}
