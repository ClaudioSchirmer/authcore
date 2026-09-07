// Hand-written, and not a hook: no generator declares this file.
//
// THE READ SIDE OF A SIGN-IN, and nothing else. It answers two questions: "who
// holds this address" and "everything a token has to say about them".
//
// IT DOES NOT ENTER THROUGH AN AGGREGATE, and that is the decision the whole file
// turns on. A sign-in has no invariant to protect and no lifecycle to drive — it
// reads rows. Loading the User aggregate cost FOUR sequential statements, three of
// them hydrating collections this endpoint does not want, and one of those was
// then re-read because the aggregate's join could not carry the gate the token
// needs. The schemas next door describe exactly the shapes this endpoint asks for,
// and nothing else in the service uses them.
//
//	before   8 statements, 6 round trips of latency
//	after    5 statements, 2 round trips of latency
//
// specs/implement/authentication-token-reads/requirements.md holds the full
// derivation: every datum the endpoint consumes, who consumes it, and why each
// statement cannot merge into another.
//
// TWO STEPS, NOT ONE, AND THE REASON IS THE ATTACK PATH. LoadAccountByEmail is one
// cheap indexed read; everything else hides behind it. Collapsing them would make
// every credential-stuffing attempt against an unknown address cost five
// statements instead of one.
//
// NOT ONE IDENTIFIER IS TYPED BY HAND, and every one is validated at CONSTRUCTION —
// the schemas, the anchors, the foreign keys, the mapped fields and the type of
// every field a left join can leave NULL. A column that stopped resolving aborts
// the BOOT naming the field, instead of shipping a read that quietly matches
// nothing at three in the morning.

package infra

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/command/read"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// AuthenticationReader serves the sign-in path. Five repositories, all Direct, all
// anchored on schemas this endpoint owns.
type AuthenticationReader struct {
	accounts *read.DirectRepository[schemas.SignInAccount]
	// The two grant paths, each entered by the index that belongs to it.
	directGrants    *read.DirectRepository[schemas.UserRoleGrant]
	inheritedGrants *read.DirectRepository[schemas.UserGroupGrant]
	claimValues     *read.DirectRepository[schemas.HeldClaimValue]
	definitions     *read.DirectRepository[schemas.ClaimDefinition]
}

// NewAuthenticationReader builds the reader and its declared traversals.
//
// THE GRANT REPOSITORY IS THE INTERESTING ONE. Its chain reaches from a role out
// to that role's grants — a 1:N fan-out, declared by giving the target schema
// ID("role_id") — and one hop further to the catalog entry each grant names. Both
// hops are LEFT joins because both are optional: a role that confers nothing is
// still a role the user holds, and it has to survive the read that asks what it
// confers.
func NewAuthenticationReader(engine core.RelationalEngine) *AuthenticationReader {
	return &AuthenticationReader{
		accounts: read.NewDirectRepository[schemas.SignInAccount](
			engine, schemas.SignInAccountSchema()).
			WithJoins(read.InnerJoin(schemas.TenantSchema().AsDirectSchema()).
				On("tenant_id").
				Field("TenantWorkspace", "workspace").
				Field("TenantStatus", "status")),

		directGrants:    newDirectGrantRepository(engine),
		inheritedGrants: newInheritedGrantRepository(engine),

		claimValues: read.NewDirectRepository[schemas.HeldClaimValue](
			engine, schemas.HeldClaimValueSchema()),

		definitions: read.NewDirectRepository[schemas.ClaimDefinition](
			engine, schemas.ClaimDefinitionSchema()),
	}
}

// newDirectGrantRepository walks user_roles → roles → role_permissions →
// permissions, entering by the member's own index — which is the whole reason the
// anchor is the edge table and not the role catalog. See the schemas file.
func newDirectGrantRepository(engine core.RelationalEngine) *read.DirectRepository[schemas.UserRoleGrant] {
	roles, grants, catalog := schemas.DirectGrantJoins()
	return read.NewDirectRepository[schemas.UserRoleGrant](engine, schemas.UserRoleGrantSchema()).
		WithJoins(read.InnerJoin(roles).On("role_id").
			Field("RoleKey", "role_key").
			Field("RoleName", "name").
			Field("RoleArchivedAt", "archived_at").
			Then(read.LeftJoin(grants).On("id").
				Field("GrantArchivedAt", "archived_at").
				Then(read.LeftJoin(catalog).On("permission_id").
					Field("Resource", "resource_name").
					Field("Action", "action_name").
					Field("PermissionArchivedAt", "archived_at"))))
}

// newInheritedGrantRepository walks user_groups → groups → group_roles → roles →
// role_permissions → permissions, entering by the same index.
//
// IT ALSO ANSWERS THE MEMBERSHIPS: the first hop is the group itself, so the
// token's `groups` claim falls out of a read that had to happen anyway.
func newInheritedGrantRepository(engine core.RelationalEngine) *read.DirectRepository[schemas.UserGroupGrant] {
	groups, groupRoles, roles, grants, catalog := schemas.InheritedGrantJoins()
	return read.NewDirectRepository[schemas.UserGroupGrant](engine, schemas.UserGroupGrantSchema()).
		WithJoins(read.InnerJoin(groups).On("group_id").
			Field("GroupKey", "group_key").
			Field("GroupName", "name").
			Field("GroupArchivedAt", "archived_at").
			Then(read.LeftJoin(groupRoles).On("id").
				Field("GroupGrantArchivedAt", "archived_at").
				Then(read.LeftJoin(roles).On("role_id").
					Field("RoleKey", "role_key").
					Field("RoleName", "name").
					Field("RoleArchivedAt", "archived_at").
					Then(read.LeftJoin(grants).On("id").
						Field("GrantArchivedAt", "archived_at").
						Then(read.LeftJoin(catalog).On("permission_id").
							Field("Resource", "resource_name").
							Field("Action", "action_name").
							Field("PermissionArchivedAt", "archived_at"))))))
}

// ── step one: the account ───────────────────────────────────────────────────

// LoadAccountByEmail finds the account behind an address. ONE statement.
//
// ABSENCE IS (nil, nil), NOT AN ERROR, and the distinction is the whole point.
// The caller refuses both cases identically — it must, or the status code becomes
// an existence oracle — but it RECORDS them differently: a miss is logged with
// identity_existed = false, a genuine failure with NULL, because in the second
// case nobody knows. Collapsing the two would leave a security reviewer unable to
// tell credential stuffing against addresses that do not exist here from an attack
// during an outage.
func (r *AuthenticationReader) LoadAccountByEmail(
	ctx context.Context, email string,
) (*schemas.SignInAccount, error) {
	return r.loadAccount(ctx, criteria.Eq("Email", email))
}

// LoadAccountByID is the refresh path's entry.
//
// RedeemRefreshToken takes claims FRESH at redemption time — the framework's
// mechanism for a permission revoked between logins reaching the mesh within
// minutes rather than at the next full sign-in — so the row is re-read and the
// bundle re-resolved on every rotation, never replayed from the token being
// redeemed.
func (r *AuthenticationReader) LoadAccountByID(
	ctx context.Context, id domain.ID,
) (*schemas.SignInAccount, error) {
	return r.loadAccount(ctx, criteria.Eq("ID", id))
}

// loadAccount is the one read both entries share.
//
// ACTIVE ROWS ONLY — the repository's default scope — and a LIVE TENANT, which the
// predicate states. The tenant gate is DEPTH rather than the mechanism: Tenant's
// own rules force Status to suspended when it is archived, and the sign-in refuses
// a suspended tenant, so through the API the two states cannot come apart. What
// this catches is a row that reached `archived_at` without going through the
// aggregate — a migration, a support script, a hand-run UPDATE. It costs no round
// trip: the subquery rides inside the statement this already issues.
func (r *AuthenticationReader) loadAccount(
	ctx context.Context, who criteria.Expr,
) (*schemas.SignInAccount, error) {
	// Limit(2) rather than FindOne: one row is the answer, two is a data problem
	// worth naming, and this keeps the MISS cheap — which matters because a miss is
	// what every credential-stuffing attempt gets.
	found, err := r.accounts.FindAll(ctx, criteria.Where(criteria.And(
		who,
		criteria.Exists(criteria.Sub(schemas.TenantSchema().AsDirectSchema()).
			Where(criteria.Eq("ID", criteria.Outer("TenantID")))),
	)).Limit(2))
	switch {
	case err != nil:
		return nil, fmt.Errorf("authentication reader: load account: %w", err)
	case len(found) == 0:
		return nil, nil
	case len(found) > 1:
		return nil, fmt.Errorf(
			"authentication reader: load account: %d rows for one identity", len(found))
	}
	return &found[0], nil
}

// ── step two: everything a token says ───────────────────────────────────────

// NamedGrant is a key and a display name. The token carries the key; the response
// body carries both.
type NamedGrant struct {
	Key  string
	Name string
}

// SignInBundle is what the token and the response body are built from.
type SignInBundle struct {
	// The groups this user belongs to, live ones only.
	Groups []NamedGrant
	// Every role held by ANY path, live ones only — INCLUDING roles that confer
	// nothing. Names are present for every one of them, which the statement this
	// replaced could not manage for the inherited half.
	Roles []NamedGrant
	// What those roles confer, deduplicated, live catalog entries only.
	Permissions []vos.PermissionKey
	// Level 1 of the claim chain: this user's own values, by definition id.
	ClaimValues map[domain.ID]string
	// Level 2, and the vocabulary the resolution walks.
	Definitions []schemas.ClaimDefinition
}

// ResolveSignIn reads everything the token needs, in FOUR CONCURRENT statements.
//
// None of them depends on another's answer: each resolves what it needs from the
// account alone, inside the database. So the four cost ONE round trip of latency
// rather than four — which on this path is what the cost actually is, an empty
// round trip measuring ~200µs against the dev bench.
//
// THE GATES, ONE BY ONE, AND WHERE EACH LIVES:
//
//	a retired ROLE          the grant anchor's own scope
//	a revoked GRANT         GrantArchivedAt, filtered per PAIR below
//	a retired PERMISSION    PermissionArchivedAt, filtered per PAIR below
//	a dropped MEMBERSHIP    the membership anchor's own scope
//	a retired GROUP         GroupArchivedAt, in the predicate
//	a removed CLAIM VALUE   the value anchor's own scope
//	a retired DEFINITION    the definition anchor's own scope
//	a revoked GRANT/MEMBERSHIP inside the graph walk   the subqueries' own scope
//
// The two filtered PER PAIR are the ones that must not become predicates: a role
// arrives as one row per grant, so a WHERE dropping a revoked grant drops the ROLE
// with it — and a role whose every permission was revoked is still a role the user
// holds. The columns come back; the decision is made here, per row.
func (r *AuthenticationReader) ResolveSignIn(
	ctx context.Context, account *schemas.SignInAccount,
) (SignInBundle, error) {
	var (
		directRows    []schemas.UserRoleGrant
		inheritedRows []schemas.UserGroupGrant
		valueRows     []schemas.HeldClaimValue
		definitions   []schemas.ClaimDefinition
		errs          [4]error
		wg            sync.WaitGroup
	)
	wg.Add(4)

	// Both grant reads are keyed on the member and enter by that index. Neither
	// filters on the archive state of anything it REACHES: those columns come back
	// and are judged per row below, because a role arrives once per grant and a
	// predicate dropping a grant would drop the role with it.
	go func() {
		defer wg.Done()
		directRows, errs[0] = r.directGrants.FindAll(ctx,
			criteria.Where(criteria.Eq("ParentID", account.ID)))
	}()

	go func() {
		defer wg.Done()
		inheritedRows, errs[1] = r.inheritedGrants.FindAll(ctx,
			criteria.Where(criteria.Eq("ParentID", account.ID)))
	}()

	go func() {
		defer wg.Done()
		valueRows, errs[2] = r.claimValues.FindAll(ctx,
			criteria.Where(criteria.Eq("ParentID", account.ID)))
	}()

	go func() {
		defer wg.Done()
		// The `both` member is included because it means "either identity kind may
		// hold this", NOT "only a principal that is both" — there is no such
		// principal. The client half of the same set is what POST /auth/client/token
		// will read when it exists; nothing here anticipates it.
		definitions, errs[3] = r.definitions.FindAll(ctx, criteria.Where(criteria.And(
			criteria.Eq("TenantID", account.TenantID),
			criteria.In("AppliesTo",
				vos.ClaimAppliesToUser.Value(),
				vos.ClaimAppliesToBoth.Value()),
		)))
	}()

	wg.Wait()
	// THE FIRST ERROR REFUSES THE WHOLE ANSWER. A partial bundle is the one thing
	// this must never return: half the roles or half the permissions would mint a
	// token that looks valid and authorizes less — or, read the other way by a
	// consumer, silently more.
	for i, err := range errs {
		if err != nil {
			what := [...]string{"direct grants", "inherited grants", "claim values", "claim definitions"}[i]
			return SignInBundle{}, fmt.Errorf("authentication reader: %s: %w", what, err)
		}
	}
	return assemble(directRows, inheritedRows, valueRows, definitions), nil
}

// assemble collapses the two fan-outs into the four answers a token is built from.
//
// EVERY GATE THAT IS NOT THE FRAMEWORK'S IS APPLIED HERE, per row, and the reason
// is the shape: a role arrives once per grant it confers, so a predicate excluding
// a revoked grant would exclude the ROLE with it — and a role whose every
// permission was revoked is still a role the user holds. The scopes on the two
// anchors have already dropped a revoked grant and a dropped membership; what is
// left is what the traversals reached into.
func assemble(
	direct []schemas.UserRoleGrant,
	inherited []schemas.UserGroupGrant,
	values []schemas.HeldClaimValue,
	definitions []schemas.ClaimDefinition,
) SignInBundle {
	var (
		roles      = map[string]NamedGrant{}
		groups     = map[string]NamedGrant{}
		perms      []vos.PermissionKey
		seenPerm   = map[string]struct{}{}
		addPerm    func(res, act *string, grantArch, permArch *time.Time)
		claimValue = make(map[domain.ID]string, len(values))
	)
	addPerm = func(res, act *string, grantArch, permArch *time.Time) {
		switch {
		case res == nil || act == nil: // the role confers nothing
			return
		case grantArch != nil: // the grant was revoked
			return
		case permArch != nil: // the catalog entry was retired
			return
		}
		key := *res + ":" + *act
		if _, dup := seenPerm[key]; dup {
			return
		}
		seenPerm[key] = struct{}{}
		perms = append(perms, vos.PermissionKey{Resource: *res, Action: *act})
	}

	for _, row := range direct {
		if row.RoleArchivedAt != nil { // a retired role confers nothing and is not held
			continue
		}
		roles[row.RoleKey] = NamedGrant{Key: row.RoleKey, Name: row.RoleName}
		addPerm(row.Resource, row.Action, row.GrantArchivedAt, row.PermissionArchivedAt)
	}

	for _, row := range inherited {
		if row.GroupArchivedAt != nil { // a retired group confers nothing
			continue
		}
		groups[row.GroupKey] = NamedGrant{Key: row.GroupKey, Name: row.GroupName}
		switch {
		case row.RoleKey == nil, // the group confers no role
			row.GroupGrantArchivedAt != nil, // the group's grant of it was revoked
			row.RoleArchivedAt != nil:       // the role itself was retired
			continue
		}
		roles[*row.RoleKey] = NamedGrant{Key: *row.RoleKey, Name: derefName(row.RoleName)}
		addPerm(row.Resource, row.Action, row.GrantArchivedAt, row.PermissionArchivedAt)
	}

	for _, row := range values {
		claimValue[row.ClaimID] = row.Value
	}

	// Stable order, so two tokens minted from the same grants are byte-identical in
	// these claims — which is what makes a diff between two tokens readable when
	// somebody is working out why a permission disappeared.
	sort.Slice(perms, func(i, j int) bool {
		a, b := perms[i], perms[j]
		if a.Resource != b.Resource {
			return a.Resource < b.Resource
		}
		return a.Action < b.Action
	})
	return SignInBundle{
		Groups:      sortedGrants(groups),
		Roles:       sortedGrants(roles),
		Permissions: perms,
		ClaimValues: claimValue,
		Definitions: definitions,
	}
}

func derefName(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func sortedGrants(m map[string]NamedGrant) []NamedGrant {
	out := make([]NamedGrant, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// ── the credential ──────────────────────────────────────────────────────────

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
// answers in well under a millisecond; one that finds a row and rejects the
// password answers in about fifteen. That difference IS the answer to "does this
// address have an account here" — the precise question the shared refusal message
// exists to refuse. Every path that declines without reaching a real hash calls
// this first, so the two cost the same from outside.
//
// The hash it verifies against is derived once, at package init, from a random
// secret: nobody — including this process — knows a plaintext that matches it, so
// there is no input an attacker could send to make this call return early.
func (r *AuthenticationReader) BurnPasswordVerification() {
	userHasher.Matches("timing-equalisation", equalisationHash)
}

// equalisationHash is the decoy BurnPasswordVerification verifies against.
//
// Derived from 32 random bytes at process start rather than from a constant, so it
// is not a value anyone can precompute a match for, and it differs between
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
