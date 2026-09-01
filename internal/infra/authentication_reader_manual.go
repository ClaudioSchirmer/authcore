// Hand-written, and not a hook: no generator declares this file.
//
// It answers the two questions a sign-in asks that no framework primitive can:
// "who holds this e-mail" and "what may they do".
//
// THE SECOND ONE IS WHY THIS FILE EXISTS. A user's effective permissions sit two
// and three hops away — user → roles → permissions, and user → groups → roles →
// permissions — and no single read expresses that: the reach is many-to-one on
// four hops and one-to-MANY on two, and a traversal only goes the first way.
//
// SO THE WALK IS FOUR DIRECT READS, each anchored on the edge table it is about
// and reaching sideways for what it needs. Not a statement assembled here: the
// tables are described as Direct schemas next door, the traversals are declared on
// the repositories, and the criteria vocabulary states the filters. Nothing in
// this file spells a column, a placeholder or a dialect, which is what makes an
// engine swap a configuration change here as it is everywhere else in this
// service.
//
// WHY NOT REUSE THE WALK THAT ALREADY EXISTS. user_service_manual.go resolves the
// same graph through memoised roleRow/groupRow probes, and it is correct. It also
// costs 1 + G + R aggregate loads — a user in three groups of four roles pays
// around eighteen — because it exists to guard a WRITE, where one extra read is
// noise. A token endpoint is the hottest security path in the platform and has a
// different budget: four indexed reads, whatever the shape of the graph. This file
// is the single owner of "the effective permission set of a user"; nothing else
// re-derives it, so the two cannot drift.
//
// NOT ONE IDENTIFIER IS TYPED BY HAND, and every one of them is validated at
// CONSTRUCTION — the schemas, the anchors, the foreign keys and the mapped
// fields — so a renamed column aborts the boot naming the field, instead of
// shipping a read that quietly matches nothing at three in the morning.

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
	"github.com/ClaudioSchirmer/omnicore/infra/db/command/read"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// AuthenticationReader serves the sign-in path.
//
// It holds the User repository (for the aggregate load, which brings the tenant
// join and both child collections along), the Claim repository (for the tenant's
// definition catalog) and the four edge repositories the grant walk runs on.
//
// THE FOUR ARE BUILT ONCE, AT CONSTRUCTION, and each validates its schema there:
// that it is Direct, that it declares a primary key, that it is anchored to its
// row type, and that every declared join maps a foreign key to a field that
// exists. So a column that stopped resolving aborts the BOOT, naming the field,
// instead of shipping a read that quietly matches nothing at three in the morning.
type AuthenticationReader struct {
	users  *UserRepository
	claims *ClaimRepository

	directRoles     *read.DirectRepository[schemas.UserRoleEdge]
	memberships     *read.DirectRepository[schemas.UserGroupEdge]
	inheritedRoles  *read.DirectRepository[schemas.GroupRoleEdge]
	rolePermissions *read.DirectRepository[schemas.RolePermissionEdge]
}

// NewAuthenticationReader builds the reader and the four traversals the grant
// walk reads through.
//
// EVERY JOIN BRINGS BACK THE TARGET'S ARCHIVE STAMP, and the walk filters on it.
// That is not defensive duplication of the scope gate: a declared join is not
// gated on the archived state of its target — the scope governs which rows come
// back, never which rows a traversal reaches into — so the retired role behind a
// live grant arrives with a perfectly good key unless something says otherwise.
// The framework's own guidance is that the filter belongs in the criteria, and
// ResolveGrants states it on every hop.
func NewAuthenticationReader(engine core.RelationalEngine) *AuthenticationReader {
	return &AuthenticationReader{
		users:  NewUserRepository(engine),
		claims: NewClaimRepository(engine),

		directRoles: read.NewDirectRepository[schemas.UserRoleEdge](
			engine, schemas.UserRoleEdgeSchema()).
			WithJoins(read.InnerJoin(schemas.RoleSchema().AsDirectSchema()).On("role_id").
				Field("RoleKey", "role_key").
				Field("RoleArchivedAt", "deleted_at")),

		memberships: read.NewDirectRepository[schemas.UserGroupEdge](
			engine, schemas.UserGroupEdgeSchema()).
			WithJoins(read.InnerJoin(schemas.GroupSchema().AsDirectSchema()).On("group_id").
				Field("GroupArchivedAt", "deleted_at")),

		inheritedRoles: read.NewDirectRepository[schemas.GroupRoleEdge](
			engine, schemas.GroupRoleEdgeSchema()).
			WithJoins(read.InnerJoin(schemas.RoleSchema().AsDirectSchema()).On("role_id").
				Field("RoleKey", "role_key").
				Field("RoleArchivedAt", "deleted_at")),

		rolePermissions: read.NewDirectRepository[schemas.RolePermissionEdge](
			engine, schemas.RolePermissionEdgeSchema()).
			WithJoins(read.InnerJoin(schemas.PermissionSchema().AsDirectSchema()).On("permission_id").
				Field("Resource", "resource_name").
				Field("Action", "action_name").
				Field("PermissionArchivedAt", "deleted_at")),
	}
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
// those roles confer.
//
// TWO SLICES AND NO STRUCT. A named result type was written and deleted: it had no
// identity, no rule and no validation, so it was neither an entity nor a value
// object — it existed only to give the application port something to name, which
// is inventing a type to work around a layering question rather than answering it.
// The slices say the same thing and belong to nobody.
//
// THE WALK IS FOUR READS, AND THE SHAPE OF THE GRAPH IS WHY. A declared traversal
// reaches from many rows to ONE — a grant to the role it names, a membership to the
// group it names — which is every hop here except two, and those two are one-to-MANY:
// a group holds many roles, a role holds many permissions. That direction is not a
// join in any read the framework offers, so it is a second read keyed on what the
// first one returned. Fewer reads would mean a hand-written statement again; more
// would mean asking a question already answered.
//
//	user_roles       ─join→ roles        : the roles held directly
//	user_groups      ─join→ groups       : the live memberships
//	group_roles      ─join→ roles        : the roles those groups confer
//	role_permissions ─join→ permissions  : what all of them grant
//
// EVERY HOP IS ARCHIVE-GATED TWICE, and both halves are load-bearing. The EDGE is
// gated by the repository's own scope — a revoked grant, a dropped membership — and
// the JOINED ROW is gated by a predicate this function states, because a traversal
// is not gated on its target: a retired role arrives with a perfectly good key
// behind a live grant. Missing either half would hand out permissions the operator
// believes they took away — the worst failure this file could have.
//
// A ROLE THAT GRANTS NOTHING IS STILL A ROLE THE USER HOLDS, and it survives here
// by construction rather than by a LEFT JOIN: the keys come from the first and third
// reads, which know nothing about permissions. A consumer branching on role
// membership sees it.
//
// There is deliberately NO tenant predicate. Every path that attaches a role or a
// group already refuses a foreign tenant in the aggregate's rules, and both a
// role's and a group's owning tenant are immutable after creation, so no row can
// exist for such a filter to catch. Adding one anyway would silently swallow the
// data problem it was pretending to guard against instead of surfacing it.
func (r *AuthenticationReader) ResolveGrants(ctx context.Context, userID domain.ID) ([]string, []vos.PermissionKey, error) {
	// Keyed by role id rather than by key: two paths can confer the SAME role —
	// held directly and inherited from a group — and the id is what says they are
	// one role rather than two spellings of one.
	held := map[domain.ID]string{}

	direct, err := r.directRoles.FindAll(ctx, criteria.Where(criteria.And(
		criteria.Eq("ParentID", userID),
		criteria.IsNull("RoleArchivedAt"),
	)))
	if err != nil {
		return nil, nil, fmt.Errorf("authentication reader: direct roles: %w", err)
	}
	for _, edge := range direct {
		held[edge.RoleID] = edge.RoleKey
	}

	memberships, err := r.memberships.FindAll(ctx, criteria.Where(criteria.And(
		criteria.Eq("ParentID", userID),
		criteria.IsNull("GroupArchivedAt"),
	)))
	if err != nil {
		return nil, nil, fmt.Errorf("authentication reader: group memberships: %w", err)
	}

	// The next read is SKIPPED rather than issued with an empty set. `IN ()` is not
	// a predicate any dialect accepts, and a user in no group has nothing to inherit
	// — so the common case of a direct-only account costs one read fewer.
	if len(memberships) > 0 {
		groupIDs := make([]domain.ID, 0, len(memberships))
		for _, edge := range memberships {
			groupIDs = append(groupIDs, edge.GroupID)
		}
		inherited, err := r.inheritedRoles.FindAll(ctx, criteria.Where(criteria.And(
			criteria.In("ParentID", idArgs(groupIDs)...),
			criteria.IsNull("RoleArchivedAt"),
		)))
		if err != nil {
			return nil, nil, fmt.Errorf("authentication reader: inherited roles: %w", err)
		}
		for _, edge := range inherited {
			held[edge.RoleID] = edge.RoleKey
		}
	}

	var (
		roleKeys    []string
		permissions []vos.PermissionKey
	)
	for _, key := range held {
		roleKeys = append(roleKeys, key)
	}

	if len(held) > 0 {
		roleIDs := make([]domain.ID, 0, len(held))
		for id := range held {
			roleIDs = append(roleIDs, id)
		}
		grants, err := r.rolePermissions.FindAll(ctx, criteria.Where(criteria.And(
			criteria.In("ParentID", idArgs(roleIDs)...),
			criteria.IsNull("PermissionArchivedAt"),
		)))
		if err != nil {
			return nil, nil, fmt.Errorf("authentication reader: role permissions: %w", err)
		}
		// One permission reached through two roles is one permission. The database
		// de-duplicated the ROWS it returned; what collapses here is the same
		// resource/action arriving from different grants.
		seen := map[string]struct{}{}
		for _, edge := range grants {
			key := edge.Resource + ":" + edge.Action
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			permissions = append(permissions, vos.PermissionKey{Resource: edge.Resource, Action: edge.Action})
		}
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
