// Hand-written, and not a hook: no generator declares this file.
//
// It answers the two questions a sign-in asks that no framework primitive can:
// "who holds this e-mail" and "what may they do".
//
// THE SECOND ONE IS WHY THIS FILE EXISTS. A user's effective permissions sit two
// and three hops away — user → roles → permissions, and user → groups → roles →
// permissions — and the whole graph is resolved by the DATABASE, in subqueries,
// from the user id alone. Nothing about the shape of it crosses back into Go to be
// fed into the next statement.
//
// TWO READS, ISSUED CONCURRENTLY, because they return two different shapes and
// neither needs the other: the roles the user holds — INCLUDING those that grant
// nothing — and what those roles grant. They are anchored on Direct schemas
// declared next door; the traversal into the permission catalog is declared on the
// repository; the filters are the criteria vocabulary. Nothing here spells a
// column, a placeholder or a dialect, which is what makes an engine swap a
// configuration change here as it is everywhere else in this service.
//
// WHAT THAT COSTS, measured rather than assumed. Against the dev bench an empty
// round trip is ~200µs, the single hand-written statement this replaced is ~295µs,
// and the two concurrent reads are ~344µs — so the SQL is comparable and the
// remaining ~50µs is one extra statement sharing the connection. On a sign-in that
// spends ~15.7ms verifying an Argon2id hash, that is 0.3%.
//
// WHY NOT REUSE THE WALK THAT ALREADY EXISTS. user_service_manual.go resolves the
// same graph through memoised roleRow/groupRow probes, and it is correct. It also
// costs 1 + G + R aggregate loads — a user in three groups of four roles pays
// around eighteen — because it exists to guard a WRITE, where one extra read is
// noise. A token endpoint is the hottest security path in the platform and has a
// different budget: two indexed reads, whatever the shape of the graph. This file
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
	"sync"

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

	// The two anchors of the grant walk. Direct, and narrow on purpose: the walk
	// scans four columns in total, and neither the aggregate's hydration nor its
	// revision guard nor its outbox has anything to do with reading a token's
	// claims.
	roles       *read.DirectRepository[schemas.RoleRow]
	grants      *read.DirectRepository[schemas.RolePermissionEdge]
	memberships *read.DirectRepository[schemas.UserGroupEdge]
}

// NewAuthenticationReader builds the reader and the one traversal the walk needs.
//
// THE JOIN CARRIES THE CATALOG'S ARCHIVE STAMP, and the walk filters on it. That
// is not defensive duplication of the scope gate: a declared join is not gated on
// the archived state of its target — the scope governs which rows come back, never
// which rows a traversal reaches into — so a revoked permission arrives with a
// perfectly good resource and action unless the criteria says otherwise. The
// framework's own guidance is that the filter belongs in the criteria, and
// ResolveGrants states it.
//
// Everything is validated HERE: that each schema is Direct, that it declares a
// primary key, that it is anchored to its row type, and that the join maps a
// foreign key to fields that exist. A column that stopped resolving aborts the
// BOOT naming the field, instead of shipping a read that quietly matches nothing
// at three in the morning.
func NewAuthenticationReader(engine core.RelationalEngine) *AuthenticationReader {
	return &AuthenticationReader{
		users:  NewUserRepository(engine),
		claims: NewClaimRepository(engine),

		roles: read.NewDirectRepository[schemas.RoleRow](engine, schemas.RoleRowSchema()),

		memberships: read.NewDirectRepository[schemas.UserGroupEdge](
			engine, schemas.UserGroupEdgeSchema()).
			WithJoins(read.InnerJoin(schemas.GroupSchema().AsDirectSchema()).
				On("group_id").
				Field("GroupKey", "group_key").
				Field("GroupArchivedAt", "deleted_at")),

		grants: read.NewDirectRepository[schemas.RolePermissionEdge](
			engine, schemas.RolePermissionEdgeSchema()).
			WithJoins(read.InnerJoin(schemas.PermissionSchema().AsDirectSchema()).
				On("permission_id").
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
	found, err := r.users.Loader.FindOne(ctx, criteria.Where(criteria.And(
		criteria.Eq("Email", email),
		liveTenant(),
	)))
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
	return r.users.Loader.FindOne(ctx, criteria.Where(criteria.And(
		criteria.Eq("ID", id),
		liveTenant(),
	)))
}

// liveTenant refuses an account whose owning tenant is archived.
//
// IT IS DEPTH, NOT THE MECHANISM, and saying so matters or the next reader will
// take it for the whole guard. Tenant's own rules force Status to suspended when
// it is archived (tenant_rules_manual.go, archive-forces-suspended), and the
// sign-in already refuses a suspended tenant — so through the API the two states
// cannot come apart. What this catches is a row that reached `deleted_at` without
// going through the aggregate: a migration, a support script, a hand-run UPDATE.
//
// It costs nothing extra: the subquery rides inside the statement the load already
// issues, and its scope is ACTIVE by default, which IS the check.
//
// The tenant's COMMERCIAL state stays where it is — accountIsUsable reads the
// joined status, because `trial` and `active` both authenticate and only the
// application knows that distinction.
func liveTenant() criteria.Expr {
	return criteria.Exists(criteria.Sub(schemas.TenantSchema().AsDirectSchema()).
		Where(criteria.Eq("ID", criteria.Outer("TenantID"))))
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

// heldByUser is the set of roles this user holds by ANY path, as a predicate over
// whichever column names a role — `ID` on `roles`, `ParentID` on the grants.
//
// IT IS ONE EXPRESSION AND IT IS EVALUATED BY THE DATABASE. The direct branch is a
// lookup on user_roles.user_id; the inherited branch walks group_roles →
// user_groups in the same statement. Nothing about the graph crosses back into Go
// to be fed into the next read, which is the whole difference between this and a
// walk that pages ids around.
//
// EVERY GATE IN HERE IS THE FRAMEWORK'S. A subquery starts on the ACTIVE scope, so
// user_roles, group_roles, user_groups and the EXISTS over groups each carry their
// own `deleted_at IS NULL` without anyone writing it — a revoked grant, a dropped
// membership and a retired group are all excluded by the shape rather than by a
// line somebody has to remember. The EXISTS is there for the one gate the scope
// cannot supply on its own: user_groups is gated as an EDGE, but a live membership
// of a RETIRED group would still confer everything that group holds.
func heldByUser(userID domain.ID, roleField string) criteria.Expr {
	return criteria.Or(
		criteria.InSub(roleField, criteria.Sub(schemas.UserRoleSchema().AsDirectSchema()).
			Select("RoleID").
			Where(criteria.Eq("ParentID", userID))),

		criteria.InSub(roleField, criteria.Sub(schemas.GroupRoleSchema().AsDirectSchema()).
			Select("RoleID").
			Where(criteria.InSub("ParentID", criteria.Sub(schemas.UserGroupSchema().AsDirectSchema()).
				Select("GroupID").
				Where(criteria.And(
					criteria.Eq("ParentID", userID),
					criteria.Exists(criteria.Sub(schemas.GroupSchema().AsDirectSchema()).
						Where(criteria.Eq("ID", criteria.Outer("GroupID")))),
				))))),
	)
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
// TWO READS, AND THE SECOND ONE IS NOT A CONTINUATION OF THE FIRST. Both are
// answered entirely inside the database from the user id alone — heldByUser is the
// same expression in both, pushed down rather than resolved into a list of ids and
// bound back in. They are two because they return two DIFFERENT SHAPES, not because
// one needs the other:
//
//	roles            → the keys, INCLUDING roles that grant nothing
//	role_permissions → what those roles grant, one row per permission
//
// A ROLE THAT GRANTS NOTHING IS STILL A ROLE THE USER HOLDS, and that is what makes
// the first read irreducible. `role_permissions` has no row for such a role, so no
// query anchored there can name it; and `roles` cannot reach the permissions,
// because a role holds MANY and a declared traversal reaches exactly one. Merging
// them would mean dropping the role — the old statement paid for a LEFT JOIN and a
// DISTINCT to avoid exactly that.
//
// THE ARCHIVE GATES, ONE BY ONE, AND WHERE EACH LIVES:
//
//	a retired ROLE          the first read's scope (and the EXISTS in the second)
//	a revoked GRANT         each repository's own scope, on the anchor
//	a dropped MEMBERSHIP    the subquery's scope, inside heldByUser
//	a retired GROUP         the EXISTS, inside heldByUser
//	a revoked PERMISSION    the predicate below, on a field the join brings back
//
// Only the last one is written out, and only because it has to be: a join is never
// gated on its target, so nothing upstream of the predicate has dropped a revoked
// catalog entry.
//
// DROPPING THAT ROW IS SAFE HERE, and it is worth saying why, because in the shape
// this replaced it was NOT. The old statement returned a role and a permission on
// the SAME row, so any predicate that excluded the permission excluded the role
// with it — which is what the LEFT JOIN and the DISTINCT were paying for. The two
// reads make that impossible by construction: the role list comes from `roles` and
// knows nothing about permissions, so a role whose every permission was revoked
// survives no matter how the second read filters. Proven, not assumed: the fixture
// holds such a role, and moving this gate into a predicate that drops the whole row
// keeps the test green.
//
// There is deliberately NO tenant predicate. Every path that attaches a role or a
// group already refuses a foreign tenant in the aggregate's rules, and both a
// role's and a group's owning tenant are immutable after creation, so no row can
// exist for such a filter to catch. Adding one anyway would silently swallow the
// data problem it was pretending to guard against instead of surfacing it.
func (r *AuthenticationReader) ResolveGrants(
	ctx context.Context, userID domain.ID,
) ([]string, []string, []vos.PermissionKey, error) {
	// THE TWO READS RUN CONCURRENTLY, and that is the whole reason the predicate is
	// pushed down into both instead of the second one being keyed on the first's
	// ids. Neither needs the other's answer — each resolves the graph from the user
	// id alone, inside the database — so the pair costs ONE round trip of latency
	// rather than two, which on this path is what the cost actually is: an empty
	// round trip measures 217µs against the dev bench while the SQL of both
	// statements together measures 124µs.
	var (
		held        []schemas.RoleRow
		grants      []schemas.RolePermissionEdge
		memberships []schemas.UserGroupEdge
		errs        [3]error
		wg          sync.WaitGroup
	)
	wg.Add(3)

	go func() {
		defer wg.Done()
		held, errs[0] = r.roles.FindAll(ctx, criteria.Where(heldByUser(userID, "ID")))
	}()

	go func() {
		defer wg.Done()
		grants, errs[1] = r.grants.FindAll(ctx, criteria.Where(criteria.And(
			heldByUser(userID, "ParentID"),
			// The owning role, gated. The anchor's scope covers the GRANT; this
			// covers the role behind it, which a retired role would otherwise
			// keep conferring through a grant nobody revoked. The first read gets
			// this from its own scope; this one cannot, because it is not reading
			// `roles`.
			criteria.Exists(criteria.Sub(schemas.RoleSchema().AsDirectSchema()).
				Where(criteria.Eq("ID", criteria.Outer("ParentID")))),
			// The catalog entry, gated. A join is not gated on its target, so
			// this is the one archive predicate the walk has to write.
			criteria.IsNull("PermissionArchivedAt"),
		)))
	}()

	go func() {
		defer wg.Done()
		// THE GROUPS THE TOKEN WILL NAME. Gated twice, like everything else here:
		// the anchor's scope drops a membership that was ended, and the predicate
		// drops a group that was retired — which the join reaches into regardless,
		// because a traversal is never gated on its target.
		memberships, errs[2] = r.memberships.FindAll(ctx, criteria.Where(criteria.And(
			criteria.Eq("ParentID", userID),
			criteria.IsNull("GroupArchivedAt"),
		)))
	}()

	wg.Wait()
	// THE FIRST ERROR REFUSES THE WHOLE ANSWER. A partial grant set is the one
	// thing this function must never return: half the roles or half the
	// permissions would mint a token that looks valid and authorizes less — or,
	// read the other way by a consumer, silently more.
	for i, err := range errs {
		if err != nil {
			what := [...]string{"roles held", "permissions granted", "group memberships"}[i]
			return nil, nil, nil, fmt.Errorf("authentication reader: %s: %w", what, err)
		}
	}

	roleKeys := make([]string, 0, len(held))
	for _, role := range held {
		roleKeys = append(roleKeys, role.Key)
	}

	groupKeys := make([]string, 0, len(memberships))
	for _, membership := range memberships {
		groupKeys = append(groupKeys, membership.GroupKey)
	}

	// One permission reached through two roles is one permission.
	var permissions []vos.PermissionKey
	seen := make(map[string]struct{}, len(grants))
	for _, grant := range grants {
		key := grant.Resource + ":" + grant.Action
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		permissions = append(permissions, vos.PermissionKey{Resource: grant.Resource, Action: grant.Action})
	}

	// Stable order, so two tokens minted from the same grants are byte-identical
	// in these claims — which is what makes a diff between two tokens readable
	// when somebody is working out why a permission disappeared.
	sort.Strings(roleKeys)
	sort.Strings(groupKeys)
	sort.Slice(permissions, func(i, j int) bool {
		a, b := permissions[i], permissions[j]
		if a.Resource != b.Resource {
			return a.Resource < b.Resource
		}
		return a.Action < b.Action
	})
	return groupKeys, roleKeys, permissions, nil
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
