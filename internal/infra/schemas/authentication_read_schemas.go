// Hand-written, and deliberately NOT generated: no spec declares these, and none
// should. They belong to ONE endpoint.
//
// THE SIGN-IN'S OWN READ MODEL. POST /auth/user/token asks a question no other
// caller in this service asks — "who is this address, and everything a token has
// to say about them" — once per attempt, on the hottest security path in the
// platform. The entity repositories answer a different question: they load an
// aggregate, with its revision, its write path and every collection it declares,
// because that is what a write needs. Entering through them cost this endpoint
// FOUR sequential statements for the account alone, three of which hydrated
// collections it does not want.
//
// specs/implement/authentication-token-reads/requirements.md holds the
// measurement and the reasoning.
//
// ── THE 1:N TRAVERSAL, AND WHY IT IS DECLARED THE WAY IT IS ──────────────────
//
// A declared join renders `target.<ID> = anchor.<fk>`, and the framework does not
// police what that means: it emits the SQL and returns whatever rows come back.
// So the direction of a traversal is decided HERE, by which column the target
// schema calls its ID — not by any rule the framework enforces.
//
//	RoleGrantsSchema declares role_permissions with ID("role_id")
//	→ the join renders  role_permissions.role_id = roles.id
//	→ ONE role fans out to MANY grant rows, which is exactly what a role's
//	  permissions are.
//
// That is why the whole grant graph is one statement instead of two. A row type
// scanned from it is one ROLE-AND-GRANT pair, and a role holding three permissions
// arrives as three rows; collapsing them is the caller's job, in Go, the way the
// statement this replaced used DISTINCT.
//
// THESE TARGET SCHEMAS ARE JOIN TARGETS AND NOTHING ELSE. Anchoring a repository
// on one would be a bug: their ID() names a foreign key, not a primary key, so
// every identity-keyed operation over them would address the wrong thing. Nothing
// in this file builds a repository on them, and the test next door asserts it.
//
// ── WHY EVERY ARCHIVE STAMP IS A MAPPED COLUMN ───────────────────────────────
//
// A declared traversal is NOT gated on the archived state of its target — the
// scope governs which rows come back, never which rows a traversal reaches into —
// so a retired role, a retired group and a revoked permission all arrive with
// perfectly good keys. Mapping the column is what lets the answer be filtered.
//
// AND THE FILTERING HAPPENS IN GO, NOT IN THE PREDICATE, for the grant graph.
// A `WHERE grant.deleted_at IS NULL` would drop the ROW, and the row is a
// role-and-grant pair — so a role whose every grant was revoked would vanish along
// with its grants, and a role the user holds must survive whatever happened to
// what it confers. The columns come back; the caller decides per pair.
//
// THE DUPLICATION IS REAL AND IS TESTED. authentication_read_schemas_test.go
// asserts every one of these agrees with the generated schema of the same table.

package schemas

import (
	"time"

	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

// ── the account ─────────────────────────────────────────────────────────────

// SignInAccount is everything the sign-in needs about the person and their
// tenant, in one row.
//
// IT IS NOT A USER AGGREGATE AND MUST NOT BECOME ONE. There is no invariant to
// protect here and no lifecycle to drive: the credential is checked against
// PasswordHash, the two statuses decide whether the account may proceed, and the
// rest becomes claims. A write to any of these columns goes through the User
// aggregate, which is where the rules live.
type SignInAccount struct {
	ID                 domain.ID
	TenantID           domain.ID
	Email              string
	GivenName          string
	FamilyName         string
	PasswordHash       string
	Status             string
	MustChangePassword bool

	// Filled by the declared join into `tenants`, never persisted.
	TenantWorkspace string
	TenantStatus    string
}

// SignInAccountSchema maps SignInAccount to `users`.
//
// PasswordHash is an ORDINARY field here, where the entity's schema declares it
// redacted. The redaction governs what leaves the service in a payload or an audit
// event; this row never becomes either — it is scanned, compared against a
// presented credential inside PasswordMatches, and dropped. Declaring it redacted
// here would hand the comparison a masked value and refuse every sign-in.
func SignInAccountSchema() *core.TableSchema {
	return core.NewDirectSchema[SignInAccount]("users").
		ID("id").
		Field("TenantID", "tenant_id").
		Field("Email", "email").
		Field("GivenName", "given_name").
		Field("FamilyName", "family_name").
		Field("PasswordHash", "password_hash").
		Field("Status", "status").
		Field("MustChangePassword", "must_change_password").
		DeletedAt("deleted_at")
}

// ── the grant graph: two reads, both anchored on the USER ───────────────────
//
// ANCHORED ON THE USER'S OWN EDGE TABLE, NOT ON `roles`, and that is the whole
// performance story. Anchoring on `roles` and filtering with `id IN (subquery)`
// reads well and plans badly: Postgres turns the IN into a hashed SubPlan and
// SEQ SCANS the role catalog, discarding 5.003 rows of 5.006 to find three. The
// same read anchored on `user_roles` — where the index is — walks
//
//	user_roles → roles → role_permissions → permissions
//
// as four index scans. Measured on a tenant of 5k roles, 2k permissions and 10k
// users: 0.165ms against a plan that seq-scanned three tables.
//
// TWO READS BECAUSE THERE ARE TWO PATHS, and each has its own index to enter by:
// the direct grant enters at `user_roles`, the inherited one at `user_groups`.
// A single read reaching both would have to start above them, which is exactly
// the seq scan this shape exists to avoid. They run concurrently.
//
// THE SECOND ONE ALSO ANSWERS THE MEMBERSHIPS. It anchors on `user_groups` and
// its first hop is the group itself, so the token's `groups` claim falls out of a
// read that had to happen anyway — one statement fewer than asking separately.

// UserRoleGrant is one row of the DIRECT path: a role held outright, and one
// permission it confers.
//
// A ROLE HOLDING THREE PERMISSIONS ARRIVES AS THREE ROWS, and a role holding none
// arrives as one row with nil grants — which is what keeps a role the user holds
// in the answer whatever happened to what it confers.
type UserRoleGrant struct {
	ID     domain.ID
	RoleID domain.ID

	// Declared so the hop out of a role can join on it. Never mapped as a join
	// FIELD and never read — a join field carries no domain type, and nothing
	// here wants the value.
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

// UserRoleGrantSchema anchors the direct path on `user_roles`, keyed by the
// member — which is the index every sign-in enters by.
func UserRoleGrantSchema() *core.TableSchema {
	return core.NewDirectSchema[UserRoleGrant]("user_roles").
		ID("id").
		ParentID("user_id").
		Field("RoleID", "role_id").
		DeletedAt("deleted_at")
}

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

// ── the join targets ────────────────────────────────────────────────────────
//
// EVERY ONE OF THESE IS A JOIN TARGET AND NOTHING ELSE. Two of them name a
// FOREIGN key as their ID — that is what makes the traversal fan out 1:N — so
// anchoring a repository on one would key every operation on the wrong column.
// Nothing does; the test next door asserts it.
//
// They are written per PATH rather than shared, because a target has to declare
// the columns its join maps and the two paths map them onto different row types.
// One shared declaration would have to name fields of both, and a schema is
// checked against the type it is anchored to.

// ── targets of the DIRECT path ──

func directRolesTarget() *core.TableSchema {
	return core.NewDirectSchema[UserRoleGrant]("roles").
		ID("id").
		Field("RoleKey", "role_key").
		Field("RoleName", "name").
		DeletedAt("deleted_at")
}

// directGrantsTarget is `role_permissions` reached FROM a role, and the
// declaration that makes it 1:N: ID("role_id") renders the join as
// `role_permissions.role_id = roles.id`, so one role fans out to its grants.
func directGrantsTarget() *core.TableSchema {
	return core.NewDirectSchema[UserRoleGrant]("role_permissions").
		ID("role_id").
		Field("HopPermissionID", "permission_id").
		DeletedAt("deleted_at")
}

func directCatalogTarget() *core.TableSchema {
	return core.NewDirectSchema[UserRoleGrant]("permissions").
		ID("id").
		Field("Resource", "resource_name").
		Field("Action", "action_name").
		DeletedAt("deleted_at")
}

// ── targets of the INHERITED path ──

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

// The traversals the reader declares, exported as ONE call per path so the join
// chain and the schemas that shape it stay in the same file.
func DirectGrantJoins() (roles, grants, catalog *core.TableSchema) {
	return directRolesTarget(), directGrantsTarget(), directCatalogTarget()
}

func InheritedGrantJoins() (groups, groupRoles, roles, grants, catalog *core.TableSchema) {
	return inheritedGroupsTarget(), inheritedGroupRolesTarget(), inheritedRolesTarget(),
		inheritedGrantsTarget(), inheritedCatalogTarget()
}

// ── the claim chain ─────────────────────────────────────────────────────────

// HeldClaimValue is level 1: the value this user holds for one definition.
//
// NO JOIN INTO THE CATALOG, and that is a correction rather than an omission. The
// User aggregate's own child join fills a name and a value type on every entry,
// and the resolution reads NEITHER — it walks the catalog, which is the
// vocabulary, and looks each definition's value up here by id. That join was paid
// on every sign-in and discarded.
type HeldClaimValue struct {
	ID      domain.ID
	ClaimID domain.ID
	Value   string
}

// HeldClaimValueSchema maps HeldClaimValue to `user_claims`.
func HeldClaimValueSchema() *core.TableSchema {
	return core.NewDirectSchema[HeldClaimValue]("user_claims").
		ID("id").
		ParentID("user_id").
		Field("ClaimID", "claim_id").
		Field("Value", "value").
		DeletedAt("deleted_at")
}

// ClaimDefinition is level 2, and the VOCABULARY the resolution iterates: one
// active definition of the tenant.
//
// The walk is over these rather than over the user's entries because of the
// DEFAULTS: a definition carrying one mints a claim for a user who holds no value
// for it, so entering through the entries would never reach it. The archive gate
// rides along for free — the anchor's own scope — where the aggregate's join could
// not apply one.
//
// DefaultValue is a pointer because NULL is meaningful: null at both levels means
// the claim is ABSENT from the token, not empty and not zero.
type ClaimDefinition struct {
	ID           domain.ID
	TenantID     domain.ID
	Name         string
	ValueType    string
	AppliesTo    string
	DefaultValue *string
}

// ClaimDefinitionSchema maps ClaimDefinition to `claims`.
func ClaimDefinitionSchema() *core.TableSchema {
	return core.NewDirectSchema[ClaimDefinition]("claims").
		ID("id").
		Field("TenantID", "tenant_id").
		Field("Name", "name").
		Field("ValueType", "value_type").
		Field("AppliesTo", "applies_to").
		Field("DefaultValue", "default_value").
		DeletedAt("deleted_at")
}
