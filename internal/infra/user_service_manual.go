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
//  2. THE ROLE ROW IS THE ONE Group ALREADY DECLARED. `roleRow` and its
//     grantsWildcard() live in group_service_manual.go, in this package, and
//     they answer exactly the question this entity asks one level up. A second
//     copy here would be the same rule written twice, free to disagree about
//     what an unresolvable role means — which is the one thing the fail-closed
//     direction cannot afford. What is local is the memo and the resolution,
//     because those hang off THIS service's context and repositories.
//
//  3. THE GROUP WALK IS THREE HOPS, AND IT RESOLVES IN 1 + N READS. A group
//     confers roles; a role grants permissions. Loading the group hands back
//     its role ids (GroupRepository declares the traversal, so the entries also
//     carry each role's key and name — unused here, free anyway), and each role
//     goes through the same memoised roleRow the direct grants use. So a user
//     joining a group of five roles pays six reads, and joining that same group
//     twice in one request pays them once.
//
//  4. THE HASHER IS NOT A QUERY. HashPassword is on this port for one reason:
//     the domain must not import a crypto package. It costs no round trip, it
//     never logs its input, and it is the only fact here that would answer the
//     same in an empty database.

package infra

import (
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
var userHasher appdomain.PasswordHasher = NewArgon2idHasher()

// companions returns the three repositories this service reads across, building
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
	}
	userCompanionsByRepo[s.repo] = c
	return c
}

// groupRow is everything the three per-entry group facts need to know about one
// group, resolved in a single read.
//
// found is false for a group no ACTIVE row carries — which deliberately
// collapses "no such group" and "a group this tenant retired" into one state,
// for the same reason roleRow does: the notification these facts feed answers
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

// roleRow resolves one role and its conferred permission keys, memoised for the
// request. Same contract as GroupServiceImpl's, on this service's context.
//
// The memo lives on the AppContext, so it is per REQUEST and never shared
// between two writes. Outside a request there is no context to memoise on and
// every call is a fresh read, which is correct rather than merely acceptable: a
// long-lived cache would answer with a role's bundle from an arbitrary point in
// the past, and that bundle is what the escalation rule is judging.
func (s *UserServiceImpl) roleRow(roleID domain.ID) roleRow {
	const memoPrefix = "authcore.user.role:"

	if s.ctx != nil {
		if cached, ok := s.ctx.Get(memoPrefix + roleID.String()); ok {
			if row, ok := cached.(roleRow); ok {
				return row
			}
		}
	}

	// DEFENCE IN DEPTH. The domain already refuses an entry id that is not a
	// usable UUID before asking, but this is the seat that would pay for it: an
	// unparseable value binds into a criterion against a UUID column, the driver
	// rejects it, and the panic below turns a validation problem into a 500.
	// Answering "not found" is both safe and true — no role row carries it.
	if _, err := roleID.UUID(); err != nil {
		return roleRow{found: false}
	}

	q := criteria.Where(criteria.Eq("ID", roleID))
	found, err := s.companions().roles.Loader.FindOne(s.queryContext(), q)

	var row roleRow
	switch {
	case err == nil:
		// The grants arrive with resource and action already filled — Role
		// declares the traversal into the catalog. Nothing here queries it.
		grants := domain.GetCurrentItemsOf[aggregatevos.RolePermission](found.GetAggregateRoot())
		keys := make([]vos.PermissionKey, 0, len(grants))
		for _, grant := range grants {
			keys = append(keys, vos.PermissionKey{Resource: grant.Resource, Action: grant.Action})
		}
		row = roleRow{found: true, tenantID: found.TenantID, keys: keys}
	case isRecordNotFound(err):
		row = roleRow{found: false}
	default:
		// A failed probe PANICS rather than inventing an answer. Every caller
		// of this row is a security rule, and a plausible answer would skip the
		// invariant it exists to enforce.
		panic("User: role probe failed for role " + roleID.String())
	}

	if s.ctx != nil {
		s.ctx.Set(memoPrefix+roleID.String(), row)
	}
	return row
}

// groupRow resolves one group and the roles it confers, memoised for the
// request. The roles themselves are resolved lazily, through roleRow, only by
// the facts that need their permission keys.
func (s *UserServiceImpl) groupRow(groupID domain.ID) groupRow {
	const memoPrefix = "authcore.user.group:"

	if s.ctx != nil {
		if cached, ok := s.ctx.Get(memoPrefix + groupID.String()); ok {
			if row, ok := cached.(groupRow); ok {
				return row
			}
		}
	}

	if _, err := groupID.UUID(); err != nil {
		return groupRow{found: false}
	}

	q := criteria.Where(criteria.Eq("ID", groupID))
	found, err := s.companions().groups.Loader.FindOne(s.queryContext(), q)

	var row groupRow
	switch {
	case err == nil:
		attached := domain.GetCurrentItemsOf[aggregatevos.GroupRole](found.GetAggregateRoot())
		ids := make([]domain.ID, 0, len(attached))
		for _, entry := range attached {
			ids = append(ids, entry.RoleID)
		}
		row = groupRow{found: true, tenantID: found.TenantID, roleIDs: ids}
	case isRecordNotFound(err):
		row = groupRow{found: false}
	default:
		panic("User: group probe failed for group " + groupID.String())
	}

	if s.ctx != nil {
		s.ctx.Set(memoPrefix+groupID.String(), row)
	}
	return row
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
func (s *UserServiceImpl) GroupIsUnavailableInTenant(tenantID domain.ID, groupID domain.ID) bool {
	row := s.groupRow(groupID)
	if !row.found {
		return true
	}
	return row.tenantID != tenantID
}

// GroupGrantsWildcard reports whether any role this group confers grants a
// permission carrying a wildcard in either part.
//
// It answers TRUE for a group that resolves to nothing, and TRUE for a group
// whose role resolves to nothing. Both are the fail-closed direction: an
// unresolvable membership must never reach the escalation probe, which would
// hand a key to Identity.HasPermission — and that panics on a wildcard and on
// an empty string alike.
func (s *UserServiceImpl) GroupGrantsWildcard(groupID domain.ID) bool {
	row := s.groupRow(groupID)
	if !row.found {
		return true
	}
	for _, roleID := range row.roleIDs {
		if s.roleRow(roleID).grantsWildcard() {
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
func (s *UserServiceImpl) CallerLacksAnyPermissionOfGroup(groupID domain.ID) bool {
	if s.ctx == nil {
		return false
	}
	identity := s.ctx.Identity()
	if identity == nil {
		return false
	}

	row := s.groupRow(groupID)
	if !row.found {
		return true
	}
	for _, roleID := range row.roleIDs {
		if s.callerLacksAnyPermissionOfRole(identity, roleID) {
			return true
		}
	}
	return false
}

// RoleIsUnavailableInTenant reports whether this role id is absent from the
// roles table, points at an archived role, or belongs to a tenant OTHER than the
// one passed. Same single-answer contract as the group probe, one hop shorter.
func (s *UserServiceImpl) RoleIsUnavailableInTenant(tenantID domain.ID, roleID domain.ID) bool {
	row := s.roleRow(roleID)
	if !row.found {
		return true
	}
	return row.tenantID != tenantID
}

// RoleGrantsWildcard reports whether the role behind this id confers any
// permission carrying a wildcard. Answers true for an unknown id.
//
// The keys it scans came out of the SAME read RoleIsUnavailableInTenant used —
// filled by the traversal RoleRepository declares into the catalog, so this
// costs no query of its own.
func (s *UserServiceImpl) RoleGrantsWildcard(roleID domain.ID) bool {
	return s.roleRow(roleID).grantsWildcard()
}

// CallerLacksAnyPermissionOfRole reports whether the requesting caller fails to
// hold at least one of the permissions this role grants — the two-hop half.
func (s *UserServiceImpl) CallerLacksAnyPermissionOfRole(roleID domain.ID) bool {
	if s.ctx == nil {
		return false
	}
	identity := s.ctx.Identity()
	if identity == nil {
		return false
	}
	return s.callerLacksAnyPermissionOfRole(identity, roleID)
}

// callerLacksAnyPermissionOfRole is the shared body of the two escalation facts.
//
// It exists so the group walk and the direct grant ask the question exactly the
// same way: one definition of "holds every key", one wildcard guard, one
// treatment of an unresolvable role. Two copies would be one rule that can
// disagree with itself about the case that matters most.
//
// It judges EVERY key the role confers, including one whose catalog row has
// since been retired. That is the fail-closed direction — a role whose bundle
// contains a retired `tenant:export` is refused to a caller who does not hold
// `tenant:export` — and since the grants no longer carry the catalog row's
// archive stamp it is also the only reading expressible without a second query.
// Strictly more restrictive, never more permissive.
func (s *UserServiceImpl) callerLacksAnyPermissionOfRole(identity *configuration.Identity, roleID domain.ID) bool {
	row := s.roleRow(roleID)
	if row.grantsWildcard() {
		// Never handed to HasPermission — it panics on a wildcard, and this
		// also covers the unresolvable role. The wildcard rule refuses this
		// write on its own; this is the second lock.
		return true
	}
	for _, key := range row.keys {
		if !identity.HasPermission(key.String()) {
			return true
		}
	}
	return false
}

var _ appdomain.UserService = (*UserServiceImpl)(nil)
