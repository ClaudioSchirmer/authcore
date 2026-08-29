// Hand-written, and not a hook: no generator declares this file.
//
// It answers the two questions a sign-in asks that no framework primitive can:
// "who holds this e-mail" and "what may they do".
//
// THE SECOND ONE IS WHY THIS FILE EXISTS. A user's effective permissions sit two
// and three hops away — user → roles → permissions, and user → groups → roles →
// permissions — and the framework's read primitives deliberately stop short of
// that: a read join is 1:1, ONE hop and no collections, and a relational view
// refuses a filter over a 1:N child with a typed 400. So the honest path is the
// neutral read seam (core.Querier), which is the same surface the framework's own
// composer runs on, and NOT a join bent past its documented boundary.
//
// WHY NOT REUSE THE WALK THAT ALREADY EXISTS. user_service_manual.go resolves the
// same graph through memoised roleRow/groupRow probes, and it is correct. It also
// costs 1 + G + R aggregate loads — a user in three groups of four roles pays
// around eighteen — because it exists to guard a WRITE, where one extra read is
// noise. A token endpoint is the hottest security path in the platform and has a
// different budget. This file is now the single owner of "the effective
// permission set of a user"; nothing else re-derives it, so the two cannot drift.
//
// NOT ONE IDENTIFIER IS TYPED BY HAND. Every table and column below is read off
// the TableSchema declarations, and the statement is assembled at CONSTRUCTION —
// so a renamed column aborts the boot with the field named, instead of shipping a
// SELECT that quietly matches nothing at three in the morning.

package infra

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// AuthenticationReader serves the sign-in path.
//
// It holds the User repository (for the aggregate load, which brings the tenant
// join and both child collections along) and the neutral engine (for the one
// statement no aggregate load can express).
type AuthenticationReader struct {
	users  *UserRepository
	claims *ClaimRepository
	engine core.RelationalEngine

	// Assembled once, at construction. See the file header: a schema that no
	// longer resolves is a boot failure, not a runtime surprise.
	permissionsStmt string
}

// NewAuthenticationReader builds the reader and, with it, the permission
// statement.
//
// It PANICS when a declared field no longer resolves. That is the intended
// severity: this runs inside feature construction, so the failure surfaces as a
// boot abort naming the field — the same class of answer the framework gives for
// a schema that disagrees with its entity.
func NewAuthenticationReader(engine core.RelationalEngine) *AuthenticationReader {
	r := &AuthenticationReader{
		users:  NewUserRepository(engine),
		claims: NewClaimRepository(engine),
		engine: engine,
	}
	r.permissionsStmt = buildEffectivePermissionsStatement(engine.Dialect())
	return r
}

// FindUserByEmail loads the account behind an address.
//
// The aggregate arrives complete: the repository's read joins fill
// TenantWorkspace and TenantStatus from the owning tenant, and both child
// collections carry their joined keys (GroupKey/GroupName, RoleKey/RoleName). So
// everything the token's claims need about the user — except the permissions —
// comes from this single load.
//
// ACTIVE ROWS ONLY, which is the loader's default scope and is exactly right
// here: an archived account must not authenticate, and it must not be
// distinguishable from an address that was never registered.
func (r *AuthenticationReader) FindUserByEmail(ctx *configuration.AppContext, email string) (*appdomain.User, error) {
	found, err := r.users.Loader.FindOne(ctx, criteria.Where(criteria.Eq("Email", email)))
	switch {
	case err == nil:
		return found, nil
	case isRecordNotFound(err):
		// ABSENCE IS (nil, nil), NOT AN ERROR, and the distinction is the whole
		// point of this branch. The caller refuses both cases identically — it
		// must, or the status code becomes an existence oracle — but it RECORDS
		// them differently: a miss is logged with identity_existed = false, a
		// genuine failure with NULL, because in the second case nobody knows.
		//
		// Collapsing the two would leave a security reviewer unable to tell
		// credential stuffing against addresses that do not exist here from an
		// attack during an outage, which is exactly the distinction the column was
		// added for.
		return nil, nil
	default:
		return nil, err
	}
}

// FindUserByID loads the account behind an id.
//
// The refresh path needs it: RedeemRefreshToken takes claims FRESH from the
// caller at redemption time, which is the framework's mechanism for a permission
// revoked between logins reaching the mesh within minutes instead of at the next
// full sign-in. Honouring that means re-reading the row and re-resolving the
// bundle on every refresh — never replaying what the previous token carried.
func (r *AuthenticationReader) FindUserByID(ctx *configuration.AppContext, id domain.ID) (*appdomain.User, error) {
	return r.users.Loader.FindOne(ctx, criteria.ByID(id))
}

// ClaimDefinitionsOfTenant returns the claim definitions a user of this tenant
// may carry a value for — LEVEL 2 of the two-level chain, and the vocabulary
// level 1 is read against.
//
// THE ACTIVE-ONLY SCOPE IS THE POINT OF READING THE CATALOG AT ALL, not a
// detail inherited from the loader's default. The emission could have been
// driven off the user's own entries instead: `UserClaim` already carries
// ClaimName and ClaimValueType from the read join this repository declares, so
// level 1 needs no query. But a read join is deliberately NOT archive-gated on
// its target — the scope governs which ROOTS come back, never which rows a
// traversal reaches into — so an entry whose definition was retired still
// arrives with a name and a type, and minting from it would put a claim in a
// token for a definition the tenant took out of service. Driving from the
// catalog drops it, and drops the default with it. Fail-closed in both halves.
//
// FindAll and not the neutral seam ResolveGrants uses one function over: this
// is rows to walk, on the same criteria surface, one hop and no aggregation —
// exactly what the list primitive is for. The tenant predicate is served by the
// leading column of claims_tenant_id_name_key, so the extra round trip a token
// operation pays here is one indexed read.
//
// The `both` member is included because it means "either identity kind may hold
// this", NOT "only a principal that is both" — there is no such principal. The
// client half of the same set is what POST /auth/client/token will read when it
// exists; nothing here anticipates it.
func (r *AuthenticationReader) ClaimDefinitionsOfTenant(ctx *configuration.AppContext, tenantID domain.ID) ([]*appdomain.Claim, error) {
	return r.claims.Loader.FindAll(ctx, criteria.Where(criteria.And(
		criteria.Eq("TenantID", tenantID),
		criteria.In("AppliesTo",
			vos.ClaimAppliesToUser.Value(),
			vos.ClaimAppliesToBoth.Value()),
	)))
}

// ResolveGrants returns the roles this user holds by ANY path and the permissions
// those roles confer, in one round trip.
//
// TWO SLICES AND NO STRUCT. A named result type was written and deleted: it had no
// identity, no rule and no validation, so it was neither an entity nor a value
// object — it existed only to give the application port something to name, which
// is inventing a type to work around a layering question rather than answering it.
// The slices say the same thing and belong to nobody.
//
// Every hop is archive-gated, and each gate is load-bearing rather than
// defensive: a revoked grant confers nothing, a retired role confers nothing, and
// a user removed from a group inherits nothing through it. Missing any one of
// them would hand out permissions the operator believes they took away — the
// worst failure this file could have.
//
// THE PERMISSION JOIN IS A LEFT JOIN, deliberately. An inner join would drop a
// role that currently grants nothing, and such a role is still a role the user
// HOLDS — a consumer branching on role membership has to see it. So the statement
// is anchored on the roles and reaches out to the permissions, never the reverse.
//
// There is deliberately NO tenant predicate. Every path that attaches a role or a
// group already refuses a foreign tenant in the aggregate's rules, and both a
// role's and a group's owning tenant are immutable after creation, so no row can
// exist for such a filter to catch. Adding one anyway would silently swallow the
// data problem it was pretending to guard against instead of surfacing it.
func (r *AuthenticationReader) ResolveGrants(ctx context.Context, userID domain.ID) ([]string, []vos.PermissionKey, error) {
	d := r.engine.Dialect()

	// The id is bound TWICE — once per branch — rather than reusing a single
	// placeholder. Postgres would accept $1 in both positions; MySQL's `?` would
	// not, and this statement is built to survive an engine swap that is a
	// configuration change everywhere else in this service.
	arg := d.EncodeArg(userID)
	rows, err := r.engine.Querier().Query(ctx, r.permissionsStmt, arg, arg)
	if err != nil {
		return nil, nil, fmt.Errorf("authentication reader: resolve grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var (
		roleKeys    []string
		permissions []vos.PermissionKey
		seenRole    = map[string]struct{}{}
		seenPerm    = map[string]struct{}{}
	)
	for rows.Next() {
		var (
			roleKey  string
			resource *string // NULL for a role that grants nothing — see the LEFT JOIN above
			action   *string
		)
		if err := rows.Scan(&roleKey, &resource, &action); err != nil {
			return nil, nil, fmt.Errorf("authentication reader: scan grant: %w", err)
		}
		if _, dup := seenRole[roleKey]; !dup && roleKey != "" {
			seenRole[roleKey] = struct{}{}
			roleKeys = append(roleKeys, roleKey)
		}
		if resource == nil || action == nil {
			continue
		}
		key := *resource + ":" + *action
		if _, dup := seenPerm[key]; dup {
			continue
		}
		seenPerm[key] = struct{}{}
		permissions = append(permissions, vos.PermissionKey{Resource: *resource, Action: *action})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("authentication reader: iterate grants: %w", err)
	}

	// Stable order, so two tokens minted from the same grants are byte-identical
	// in these claims — which is what makes a diff between two tokens readable
	// when somebody is working out why a permission disappeared.
	sort.Strings(roleKeys)
	sort.Slice(permissions, func(i, j int) bool {
		a, b := permissions[i], permissions[j]
		if a.Resource != b.Resource {
			return a.Resource < b.Resource
		}
		return a.Action < b.Action
	})
	return roleKeys, permissions, nil
}

// PasswordMatches reports whether the plaintext produced the stored hash.
//
// An EMPTY stored hash answers false without hashing anything: a row carrying no
// credential authenticates nobody, and there is nothing to compare against.
func (r *AuthenticationReader) PasswordMatches(plaintext, encoded string) bool {
	if encoded == "" {
		return false
	}
	return userHasher.Matches(plaintext, encoded)
}

// BurnPasswordVerification spends one Argon2id verification and throws the answer
// away.
//
// IT IS NOT DEAD CODE AND MUST NOT BE OPTIMISED OUT. A sign-in that finds no row
// answers in about a millisecond; one that finds a row and rejects the password
// answers in about a hundred. That difference IS the answer to "does this address
// have an account here" — the precise question the shared refusal message exists
// to refuse. Every path that declines without reaching a real hash calls this
// first, so the two cost the same from outside.
//
// The hash it verifies against is derived once, at package init, from a random
// secret: nobody — including this process — knows a plaintext that matches it, so
// there is no input an attacker could send to make this call return early.
func (r *AuthenticationReader) BurnPasswordVerification() {
	userHasher.Matches("timing-equalisation", equalisationHash)
}

// equalisationHash is the decoy BurnPasswordVerification verifies against.
//
// Derived from 32 random bytes at process start rather than from a constant, so
// it is not a value anyone can precompute a match for, and it differs between
// processes.
var equalisationHash = func() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// A broken random source at init is not something to degrade past: every
		// credential this process would go on to mint depends on the same source.
		panic("authentication reader: cannot seed the timing-equalisation hash: " + err.Error())
	}
	return userHasher.Hash(hex.EncodeToString(buf))
}()

// buildEffectivePermissionsStatement assembles the one query, from the schemas.
//
// ANCHORED ON THE ROLES, reaching out to the permissions — not the reverse. The
// two grant paths are two IN branches over the same anchor, so the database does
// the de-duplication it is built for and the caller gets one cursor instead of
// two result sets to merge in Go.
func buildEffectivePermissionsStatement(d core.Dialect) string {
	var (
		permission     = schemas.PermissionSchema()
		rolePermission = schemas.RolePermissionSchema()
		role           = schemas.RoleSchema()
		userRole       = schemas.UserRoleSchema()
		groupRole      = schemas.GroupRoleSchema()
		userGroup      = schemas.UserGroupSchema()
		group          = schemas.GroupSchema()
	)

	// Bare aliases, never `AS <alias>`: `AS` before a TABLE alias is optional in
	// standard SQL and outright rejected by Oracle — the same reason the
	// framework's own join renderer writes them bare.
	const (
		aP  = "p"
		aRP = "rp"
		aR  = "r"
		aUR = "ur"
		aGR = "gr"
		aUG = "ug"
		aG  = "g"
	)

	q := d.QuoteIdent
	// qualified renders `alias.column`. The alias is a literal from the closed set
	// above, so it needs no quoting; the column comes from a schema and gets the
	// dialect's own identifier treatment.
	qualified := func(alias, column string) string { return alias + "." + q(column) }
	// from renders `table alias`.
	from := func(s *core.TableSchema, alias string) string { return q(s.Table()) + " " + alias }
	// live renders the archive gate for one node. Every hop carries one.
	live := func(s *core.TableSchema, alias string) string {
		return qualified(alias, mustColumn(s, "DeletedAt")) + " IS NULL"
	}

	// The direct grants: the roles this user holds in their own right.
	//
	// Note which column is selected and which is filtered — they are different and
	// easy to swap: the SELECT list is the ROLE the entry points at, the WHERE is
	// the parent key that ties the entry to its user.
	directRoles := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s = %s AND %s",
		qualified(aUR, mustColumn(userRole, "RoleID")),
		from(userRole, aUR),
		qualified(aUR, userRole.ParentIDColumn()),
		d.Placeholder(1),
		live(userRole, aUR),
	)

	// The inherited grants: roles conferred by a group this user belongs to. Three
	// hops, three archive gates — the membership, the group itself, and the
	// attachment of the role to the group.
	groupRoles := fmt.Sprintf(
		"SELECT %s FROM %s INNER JOIN %s ON %s = %s AND %s INNER JOIN %s ON %s = %s AND %s WHERE %s = %s AND %s",
		qualified(aGR, mustColumn(groupRole, "RoleID")),
		from(groupRole, aGR),
		from(userGroup, aUG),
		qualified(aUG, mustColumn(userGroup, "GroupID")),
		qualified(aGR, groupRole.ParentIDColumn()),
		live(userGroup, aUG),
		from(group, aG),
		qualified(aG, group.IDColumn()),
		qualified(aUG, mustColumn(userGroup, "GroupID")),
		live(group, aG),
		qualified(aUG, userGroup.ParentIDColumn()),
		d.Placeholder(2),
		live(groupRole, aGR),
	)

	return fmt.Sprintf(
		"SELECT DISTINCT %s, %s, %s FROM %s "+
			"LEFT JOIN %s ON %s = %s AND %s "+
			"LEFT JOIN %s ON %s = %s AND %s "+
			"WHERE %s AND (%s IN (%s) OR %s IN (%s))",
		qualified(aR, mustColumn(role, "Key")),
		qualified(aP, mustColumn(permission, "Resource")),
		qualified(aP, mustColumn(permission, "Action")),
		from(role, aR),

		from(rolePermission, aRP),
		qualified(aRP, rolePermission.ParentIDColumn()),
		qualified(aR, role.IDColumn()),
		live(rolePermission, aRP),

		from(permission, aP),
		qualified(aP, permission.IDColumn()),
		qualified(aRP, mustColumn(rolePermission, "PermissionID")),
		live(permission, aP),

		live(role, aR),
		qualified(aR, role.IDColumn()), directRoles,
		qualified(aR, role.IDColumn()), groupRoles,
	)
}

// mustColumn resolves a logical field name to its physical column, or panics.
//
// Resolve rather than ColumnOf: it is the framework's single read-side resolution
// surface, and it is the only one that answers for the managed slots — DeletedAt
// among them — which have no Go field to look up.
//
// The panic is the point. This runs at construction, inside feature wiring, so a
// field that stopped resolving aborts the BOOT with its name in the message. The
// alternative is a statement that compiles, runs, and returns nothing.
func mustColumn(s *core.TableSchema, goField string) string {
	resolved, ok := s.Resolve(goField)
	if !ok {
		panic(fmt.Sprintf(
			"authentication reader: %q does not resolve on the schema for table %q — "+
				"the effective-permission query is composed from the TableSchema declarations, "+
				"so a renamed or removed field has to be renamed here too",
			goField, s.Table()))
	}
	return resolved.Column
}
