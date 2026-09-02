// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written facts for User (7 to implement).
//
// entity:     User
// spec:       specs/omnicore-gen/user.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-26
//
// Unlike the rules hook, these are NOT quiet: each one panics until it is
// written. The service builds, boots and serves everything else, and the
// failure arrives the moment a rule asks — as a 500, with the write rolled
// back.
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

// How to answer one of these depends on where the truth lives: another service
// of your own is normally gRPC, a third-party API is the HTTP client, and data
// you need to filter and sort locally is usually better mirrored in through an
// upstream subscription than fetched per request.
//
// If the truth is THIS database — a question the spec could not phrase
// declaratively — ask it the way the generated facts beside this file do:
// the loader's existence probe for a yes/no, its aggregate DSL for a count,
// total, average or extreme, and the grouped form for per-key facts. Loading
// the rows and folding the answer in Go reads a whole table to compute what
// one SELECT computes, on the write path.
//
// Two constraints the port imposes. The method returns a plain value and NO
// error, so decide here what an unavailable source means — failing loudly is
// the safe default, because returning a plausible answer skips the very rule
// this exists to enforce. And it runs inside the write, so a slow call is a
// slow write: use the bound context.

// All seven are implemented below. Four design notes that are load-bearing:
//
//  1. THE COMPANION REPOSITORIES. Every fact here but the hasher reads a table
//     this aggregate does not own — tenants, groups and roles. The generated
//     impl carries only its own repository, so the companions are built lazily
//     and cached KEYED BY THE OWNING UserRepository. Keying matters: a
//     package-level sync.Once would build them from the first engine it ever
//     saw and hand those to every service built afterwards, which is wrong the
//     moment a test or a second bootstrap uses a different engine. Same shape
//     GroupServiceImpl and RoleServiceImpl use.
//
//  2. THE ROLE ROW IS THE ONE Group ALREADY DECLARED. `dtos.RoleRow` and its
//     grantsWildcard() live in group_service_manual.go, in this package, and
//     they answer exactly the question this entity asks one level up. A second
//     copy here would be the same rule written twice, free to disagree about
//     what an unresolvable role means — which is the one thing the fail-closed
//     direction cannot afford. What is local is the memo and the resolution,
//     because those hang off THIS service's context and repositories.
//
//  3. EVERY COLLECTION FACT IS ASKED ONCE, FOR THE WHOLE COLLECTION. The spec
//     declares them `perEntry`, so each arrives with the entry ids the write
//     touched and answers a map keyed by them. The resolvers below read that
//     set in ONE query — `In("ID", …)` — instead of one query per id, which is
//     what a write carrying twenty claims used to cost. The rule still judges
//     entry by entry: it reads the map.
//
//     THE GROUP WALK IS THREE HOPS AND RESOLVES IN TWO READS. A group confers
//     roles; a role grants permissions. The groups come back with their role
//     ids (GroupRepository declares the traversal, so the entries also carry
//     each role's key and name — unused here, free anyway), and every role
//     those groups name is then resolved in a second batched read, together
//     with the roles granted directly. Two reads for any number of groups.
//
//     THE MEMO IS WHAT MAKES THREE FACTS ONE QUERY, and it is per REQUEST,
//     keyed by the row's CANONICAL id. Canonical rather than as-written,
//     because `018F…` and `018f…` are the same row and one read must answer
//     both spellings — the returned map is still keyed by the id the caller
//     PASSED, so the rule can look up what it asked about.
//
//  4. THE HASHER IS NOT A QUERY. HashPassword is on this port for one reason:
//     the domain must not import a crypto package. It costs no round trip, it
//     never logs its input, and it is the only fact here that would answer the
//     same in an empty database.

package infra

import (
	"github.com/ClaudioSchirmer/authcore/internal/infra/dtos"
	"github.com/ClaudioSchirmer/authcore/internal/infra/utils"
	"strconv"
	"sync"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// userCompanionRepos are the repositories these facts read besides User's own.
type userCompanionRepos struct {
	tenants *TenantRepository
	groups  *GroupRepository
	roles   *RoleRepository
	claims  *ClaimRepository
}

var (
	userCompanionsMu sync.Mutex
	// Keyed by the OWNING repository — see note 1 in the file header. Never a
	// bare package-level singleton.
	userCompanionsByRepo = map[*UserRepository]*userCompanionRepos{}
)

// userHasher is the one hasher these facts use.
//
// It is a package-level value rather than a field because Argon2idHasher is
// stateless and carries no engine: there is nothing per-repository about it,
// and nothing to key it by.
var userHasher = NewArgon2idHasher()

// companions returns the four repositories this service reads across, building
// them once per owning UserRepository.
//
// They share that repository's engine, which is what keeps every probe on the
// same connection pool and the same dialect as the write it is guarding.
func (s *UserServiceImpl) companions() *userCompanionRepos {
	userCompanionsMu.Lock()
	defer userCompanionsMu.Unlock()

	if c, ok := userCompanionsByRepo[s.repo]; ok {
		return c
	}
	c := &userCompanionRepos{
		tenants: NewTenantRepository(s.repo.Engine),
		groups:  NewGroupRepository(s.repo.Engine),
		roles:   NewRoleRepository(s.repo.Engine),
		claims:  NewClaimRepository(s.repo.Engine),
	}
	userCompanionsByRepo[s.repo] = c
	return c
}

// groupRow is everything the three per-entry group facts need to know about one
// group, resolved in a single read.
//
// found is false for a group no ACTIVE row carries — which deliberately
// collapses "no such group" and "a group this tenant retired" into one state,
// for the same reason dtos.RoleRow does: the notification these facts feed answers
// all three of its questions with one message, because a distinct reply would
// be an existence oracle over a competitor's org chart.
//
// roleIDs are the roles the group confers, ACTIVE attachments only — the
// loader's default child scope — because a detached role is conferred on
// nobody and must not be judged.
type groupRow struct {
	found    bool
	tenantID domain.ID
	roleIDs  []domain.ID
}

// ── the rows these facts resolve ───────────────────────────────────────────

// The memo prefixes. One per row type, so the three resolvers never collide,
// and named for THIS service: the memo hangs off the request's AppContext,
// which every service of the request shares.
const (
	userRoleMemoPrefix  = "authcore.user.role:"
	userGroupMemoPrefix = "authcore.user.group:"
	userClaimMemoPrefix = "authcore.user.claim:"
)

// roleRows resolves every requested role and the permission keys it confers in
// ONE read, memoised for the request. Same row type GroupServiceImpl uses, on
// this service's context.
func (s *UserServiceImpl) roleRows(roleIDs []domain.ID) map[domain.ID]dtos.RoleRow {
	return utils.ResolveRows(s.ctx, userRoleMemoPrefix, roleIDs, func(missing []domain.ID) map[string]dtos.RoleRow {
		q := criteria.Where(criteria.In("ID", utils.IDArgs(missing)...))
		found, err := s.companions().roles.Loader.FindAll(s.queryContext(), q)
		if err != nil {
			// A failed probe PANICS rather than inventing an answer. Every
			// caller of this row is a security rule, and a plausible answer
			// would skip the invariant it exists to enforce.
			panic("User: role probe failed for the " + strconv.Itoa(len(missing)) + " role(s) this write reaches")
		}

		rows := make(map[string]dtos.RoleRow, len(found))
		for _, role := range found {
			// The grants arrive with resource and action already filled — Role
			// declares the traversal into the catalog. Nothing here queries it.
			grants := domain.GetCurrentItemsOf[aggregatevos.RolePermission](role.GetAggregateRoot())
			keys := make([]vos.PermissionKey, 0, len(grants))
			for _, grant := range grants {
				keys = append(keys, vos.PermissionKey{Resource: grant.Resource, Action: grant.Action})
			}
			rows[utils.CanonicalIDOf(role.GetID())] = dtos.RoleRow{Found: true, TenantID: role.TenantID, Keys: keys}
		}
		return rows
	})
}

// groupRows resolves every requested group and the roles it confers in ONE
// read, memoised for the request. The roles themselves are resolved by
// rolesOfGroups, and only for the facts that need their permission keys.
func (s *UserServiceImpl) groupRows(groupIDs []domain.ID) map[domain.ID]groupRow {
	return utils.ResolveRows(s.ctx, userGroupMemoPrefix, groupIDs, func(missing []domain.ID) map[string]groupRow {
		q := criteria.Where(criteria.In("ID", utils.IDArgs(missing)...))
		found, err := s.companions().groups.Loader.FindAll(s.queryContext(), q)
		if err != nil {
			panic("User: group probe failed for the " + strconv.Itoa(len(missing)) + " group(s) this write joins")
		}

		rows := make(map[string]groupRow, len(found))
		for _, group := range found {
			attached := domain.GetCurrentItemsOf[aggregatevos.GroupRole](group.GetAggregateRoot())
			ids := make([]domain.ID, 0, len(attached))
			for _, entry := range attached {
				ids = append(ids, entry.RoleID)
			}
			rows[utils.CanonicalIDOf(group.GetID())] = groupRow{found: true, tenantID: group.TenantID, roleIDs: ids}
		}
		return rows
	})
}

// rolesOfGroups is the SECOND hop of the group walk, and the reason the walk
// costs two reads instead of one per group: a group's roles are not known until
// the groups themselves are read, so this takes the resolved groups and
// resolves everything they confer together.
//
// It shares the role memo with the DIRECT grants, so a role reached both ways
// is read once — the case note 3 in the header exists for.
func (s *UserServiceImpl) rolesOfGroups(groups map[domain.ID]groupRow) map[domain.ID]dtos.RoleRow {
	conferred := make([]domain.ID, 0, len(groups))
	for _, row := range groups {
		conferred = append(conferred, row.roleIDs...)
	}
	return s.roleRows(conferred)
}

// HashPassword returns the irreversible Argon2id hash of the plaintext,
// PHC-encoded.
//
// It is on this port so the DOMAIN never imports a crypto package: an aggregate
// that knew Argon2id would be an aggregate that changes when the parameters do.
// The whole implementation is one delegation, and that is the point — every
// decision about cost, salt and encoding lives in password_hasher.go.
//
// It never logs its input, and neither does anything it calls.
func (s *UserServiceImpl) HashPassword(password string) string {
	return userHasher.Hash(password)
}

// PasswordIsUnchanged reports whether the plaintext given is the password
// already on the row.
//
// It is the refusal behind "the new password must differ from the current one",
// and it is NOT password reuse prevention: it can only see the ONE hash the row
// carries. Real reuse prevention needs a history table, which this model
// deliberately does not have — saying so here keeps the rule from being sold as
// something it is not.
//
// An empty stored hash answers FALSE: a row with no credential has no password
// for a new one to be identical to.
func (s *UserServiceImpl) PasswordIsUnchanged(password string, passwordHash string) bool {
	if passwordHash == "" {
		return false
	}
	return userHasher.Matches(password, passwordHash)
}

// TenantIsUnavailable reports whether the owning tenant is missing, archived, or
// commercially SUSPENDED. Queries the tenants table by its primary key.
//
// Named for the PROBLEM rather than the healthy state, which is what lets the
// generated suite's zero-value service stub read as "nothing is wrong".
//
// THREE conditions, and the third is the one that is easy to get wrong. Tenant
// carries its commercial lifecycle beside the archive stamp and the two are
// orthogonal: archiving forces `suspended`, but a tenant can be suspended while
// perfectly un-archived — a customer who stopped paying. Minting a user inside
// one hands out a credential the commercial state says to withhold.
//
// TRIAL IS AVAILABLE, deliberately. "Unavailable" is not "not active": a trial
// is a live customer being onboarded, and users are the first thing they need.
// Reading this as `Status != active` would break every trial signup.
func (s *UserServiceImpl) TenantIsUnavailable(tenantID domain.ID) bool {
	// Usable, not merely non-empty — the same reason the row probes guard their
	// arguments. An unparseable owner is not a tenant that exists, so
	// "unavailable" is the true and fail-closed reading.
	if _, err := tenantID.UUID(); err != nil {
		return true
	}

	// One query for all three conditions: the ACTIVE scope is the default, so an
	// archived row does not match, and the status predicate rules out a
	// suspended one.
	q := criteria.Where(criteria.And(
		criteria.Eq("ID", tenantID),
		criteria.Ne("Status", vos.TenantStatusSuspended.Value()),
	))
	found, err := s.companions().tenants.Loader.Exists(s.queryContext(), q)
	if err != nil {
		panic("User: TenantIsUnavailable probe failed")
	}
	return !found
}

// GroupIsUnavailableInTenant reports whether this group id is absent from the
// groups table, points at an archived group, or belongs to a tenant OTHER than
// the one passed.
//
// ONE ANSWER FOR ALL THREE, and collapsing them is the requirement rather than a
// convenience — a distinct "that group belongs to another tenant" reply confirms
// to a caller in tenant A that a specific UUID is a live group in some other
// tenant.
//
// THE TENANT COMPARED IS THE USER'S, NOT THE CALLER'S, and the argument carries
// it. On the ordinary path the two are the same value — the write guard refused
// anything else — but a *:* super-admin crosses that scope, and when they do,
// "this tenant" has to mean the user's or the rule would compare a group against
// the operator's own tenant and refuse every legitimate join.
func (s *UserServiceImpl) GroupIsUnavailableInTenant(tenantID domain.ID, groupIDSet []domain.ID) map[domain.ID]bool {
	rows := s.groupRows(groupIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = !row.found || row.tenantID != tenantID
	}
	return out
}

// GroupGrantsWildcard reports whether any role this group confers grants a
// permission carrying a wildcard in either part.
//
// It answers TRUE for a group that resolves to nothing, and TRUE for a group
// whose role resolves to nothing. Both are the fail-closed direction: an
// unresolvable membership must never reach the escalation probe, which would
// hand a key to Identity.HasPermission — and that panics on a wildcard and on
// an empty string alike.
func (s *UserServiceImpl) GroupGrantsWildcard(groupIDSet []domain.ID) map[domain.ID]bool {
	groups := s.groupRows(groupIDSet)
	roles := s.rolesOfGroups(groups)

	out := make(map[domain.ID]bool, len(groups))
	for id, group := range groups {
		out[id] = groupGrantsWildcard(group, roles)
	}
	return out
}

// groupGrantsWildcard is one group's verdict, read off rows already resolved.
//
// A role the second hop did not answer for reads as the zero dtos.RoleRow, whose
// grantsWildcard() is TRUE — the fail-closed direction, and the same answer the
// per-id resolution gave an unknown role before this was a batch.
func groupGrantsWildcard(group groupRow, roles map[domain.ID]dtos.RoleRow) bool {
	if !group.found {
		return true
	}
	for _, roleID := range group.roleIDs {
		if roles[roleID].GrantsWildcard() {
			return true
		}
	}
	return false
}

// CallerLacksAnyPermissionOfGroup reports whether the requesting caller fails to
// hold at least one permission conferred by any role in this group.
//
// THE THREE-HOP HALF, and the deepest reach in this service: a role bundles
// permissions and a group bundles roles, so joining somebody to a group hands
// them the union of every bundle it carries. Every key must be held; the first
// one that is not refuses the membership.
//
// It GUARDS THE WILDCARD ITSELF and answers "lacks" rather than calling through
// — defence in depth behind the wildcard rule, because Identity.HasPermission
// panics on any argument containing '*' and a panic on a security rule is a 500
// on exactly the path that rule exists to close.
//
// An ABSENT identity stands down (answers false), matching authz.noIdentity: an
// identity is nil only under auth.mode disabled, which the framework's own boot
// guard permits in the dev profile alone. An identity that IS present and lacks
// a permission refuses.
func (s *UserServiceImpl) CallerLacksAnyPermissionOfGroup(groupIDSet []domain.ID) map[domain.ID]bool {
	identity := s.requestingIdentity()
	if identity == nil {
		// STANDS DOWN WITHOUT READING ANYTHING. An empty answer raises nothing
		// — an absent key is this fact answering nothing for that entry — and
		// it is also the cheap direction: there is no caller to compare the
		// bundles against, so resolving them would be work for no verdict.
		return nil
	}

	groups := s.groupRows(groupIDSet)
	roles := s.rolesOfGroups(groups)

	out := make(map[domain.ID]bool, len(groups))
	for id, group := range groups {
		if !group.found {
			out[id] = true
			continue
		}
		lacks := false
		for _, roleID := range group.roleIDs {
			if utils.CallerLacksAnyPermissionOfRole(identity, roles[roleID]) {
				lacks = true
				break
			}
		}
		out[id] = lacks
	}
	return out
}

// RoleIsUnavailableInTenant reports whether this role id is absent from the
// roles table, points at an archived role, or belongs to a tenant OTHER than the
// one passed. Same single-answer contract as the group probe, one hop shorter.
func (s *UserServiceImpl) RoleIsUnavailableInTenant(tenantID domain.ID, roleIDSet []domain.ID) map[domain.ID]bool {
	rows := s.roleRows(roleIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = !row.Found || row.TenantID != tenantID
	}
	return out
}

// RoleGrantsWildcard reports whether the role behind this id confers any
// permission carrying a wildcard. Answers true for an unknown id.
//
// The keys it scans came out of the SAME read RoleIsUnavailableInTenant used —
// filled by the traversal RoleRepository declares into the catalog, so this
// costs no query of its own.
func (s *UserServiceImpl) RoleGrantsWildcard(roleIDSet []domain.ID) map[domain.ID]bool {
	rows := s.roleRows(roleIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = row.GrantsWildcard()
	}
	return out
}

// CallerLacksAnyPermissionOfRole reports whether the requesting caller fails to
// hold at least one of the permissions this role grants — the two-hop half.
func (s *UserServiceImpl) CallerLacksAnyPermissionOfRole(roleIDSet []domain.ID) map[domain.ID]bool {
	identity := s.requestingIdentity()
	if identity == nil {
		// Stands down without reading anything, exactly as the group half does.
		return nil
	}

	rows := s.roleRows(roleIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = utils.CallerLacksAnyPermissionOfRole(identity, row)
	}
	return out
}

// requestingIdentity is the caller the two escalation facts judge against, or
// nil when there is none.
//
// An ABSENT identity stands the escalation rules down, matching
// authz.noIdentity: an identity is nil only under auth.mode disabled, which the
// framework's own boot guard permits in the dev profile alone.
func (s *UserServiceImpl) requestingIdentity() *configuration.Identity {
	if s.ctx == nil {
		return nil
	}
	return s.ctx.Identity()
}

var _ appdomain.UserService = (*UserServiceImpl)(nil)

// ── the claims collection: one row, three questions ─────────────────────────
//
// claimRow is what a single load of a claim definition answers, memoised per
// request. The three facts below each need a different part of the SAME row —
// whether it exists at all, which tenant owns it, which identity kinds it
// admits, and what its values must parse as — so a probe per question would be
// three reads of one row per entry of the collection.

type claimRow struct {
	found     bool
	tenantID  domain.ID
	appliesTo vos.ClaimAppliesTo
	valueType vos.ClaimValueType
}

// claimRows resolves every requested definition in ONE read, memoised for the
// request — so the three facts below share it and the write pays for one query
// however many claims it carries.
func (s *UserServiceImpl) claimRows(claimIDs []domain.ID) map[domain.ID]claimRow {
	return utils.ResolveRows(s.ctx, userClaimMemoPrefix, claimIDs, func(missing []domain.ID) map[string]claimRow {
		q := criteria.Where(criteria.In("ID", utils.IDArgs(missing)...))
		found, err := s.companions().claims.Loader.FindAll(s.queryContext(), q)
		if err != nil {
			panic("User: claim probe failed for the " + strconv.Itoa(len(missing)) + " claim(s) this write holds")
		}

		// ARCHIVED IS SIMPLY ABSENT, and that is the point rather than a
		// coincidence: the loader filters the archived rows out, so a retired
		// definition never comes back and lands as the zero row — exactly the
		// answer the rules want for it.
		rows := make(map[string]claimRow, len(found))
		for _, claim := range found {
			rows[utils.CanonicalIDOf(claim.GetID())] = claimRow{
				found:     true,
				tenantID:  claim.TenantID,
				appliesTo: claim.AppliesTo,
				valueType: claim.ValueType,
			}
		}
		return rows
	})
}

// ClaimIsUnavailableInTenant answers absent, archived and foreign-tenant as ONE
// value, for every entry of the collection.
//
// The caller-facing message must not distinguish them: a distinct "belongs to
// another tenant" reply confirms to a caller in tenant A that a specific UUID is
// a live definition in some other tenant — an existence oracle over a
// competitor's claim vocabulary. Verbatim the shape RoleIsUnavailableInTenant
// already ships.
func (s *UserServiceImpl) ClaimIsUnavailableInTenant(tenantID domain.ID, claimIDSet []domain.ID) map[domain.ID]bool {
	rows := s.claimRows(claimIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = !row.found || row.tenantID != tenantID
	}
	return out
}

// ClaimDoesNotApplyToUser answers whether the definition excludes users.
//
// TRUE for an unknown id, the direction every probe in this file takes: a fact
// that cannot resolve its subject reports the problem as present, so an
// unresolvable entry can never pass a check by accident. In practice the
// availability rule refuses it first and the rule never reads this entry's
// answer — the answer is computed anyway, because it rode the same read.
func (s *UserServiceImpl) ClaimDoesNotApplyToUser(claimIDSet []domain.ID) map[domain.ID]bool {
	rows := s.claimRows(claimIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		// ClaimAppliesToUser and Both admit a user; Client does not, and
		// neither does the Unknown sentinel — a value outside the closed set is
		// not a permission to hold anything.
		out[id] = !row.found ||
			(row.appliesTo != vos.ClaimAppliesToUser && row.appliesTo != vos.ClaimAppliesToBoth)
	}
	return out
}

// ClaimValueDoesNotMatchValueType answers whether each entry's value fails to
// parse as the type its definition declares.
//
// It calls appdomain.ClaimValueMatchesValueType — the SAME function the catalog
// uses for its own default_value at level 2 — rather than repeating the switch
// here. Two levels of one chain must not disagree about what a bool is, and a
// second copy is a rule that can drift from the first.
//
// THE ENTRY CARRIES BOTH HALVES, in the generated carrier: the id says which
// definition, the value is what has to parse. Two parallel slices would be the
// same pair with one more way to go wrong.
//
// TRUE for an unknown id, like its neighbour above.
func (s *UserServiceImpl) ClaimValueDoesNotMatchValueType(entries []appdomain.UserClaimValueDoesNotMatchValueTypeEntry) map[domain.ID]bool {
	ids := make([]domain.ID, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ClaimID)
	}
	rows := s.claimRows(ids)

	out := make(map[domain.ID]bool, len(entries))
	for _, entry := range entries {
		row := rows[entry.ClaimID]
		out[entry.ClaimID] = !row.found ||
			!appdomain.ClaimValueMatchesValueType(row.valueType, entry.Value)
	}
	return out
}
