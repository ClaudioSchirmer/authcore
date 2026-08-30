// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written facts for Group (4 to implement).
//
// entity:     Group
// spec:       specs/omnicore-gen/group.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-24
//
// Unlike the rules hook, these are NOT quiet: each one panics until it is
// written. The service builds, boots and serves everything else, and the
// failure arrives the moment a rule asks — as a 500, with the write rolled
// back.
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

// All four are implemented below. Three design notes that are load-bearing:
//
//  1. THE COMPANION REPOSITORIES. Every fact here reads a table this aggregate
//     does not own — tenants and roles. The generated impl carries only its own
//     repository, so the companions are built lazily and cached KEYED BY THE
//     OWNING GroupRepository. Keying matters: a package-level sync.Once would
//     build them from the first engine it ever saw and hand those to every
//     service built afterwards, which is wrong the moment a test or a second
//     bootstrap uses a different engine. Same shape RoleServiceImpl uses.
//
//  2. ONE ROLE READ PER ENTRY, NOT THREE. RoleIsUnavailableInTenant,
//     RoleGrantsWildcard and CallerLacksAnyPermissionOf all ask about the same
//     role. They funnel through roleRow, memoised on the REQUEST-scoped
//     AppContext, so three questions about one id cost one query. A group at the
//     50-role cap therefore pays 50 round trips inside the write transaction
//     rather than 150.
//
//  3. THAT ONE READ ALSO CARRIES THE PERMISSION KEYS, AND THAT IS ROLE'S DOING
//     RATHER THAN THIS FILE'S. RoleRepository declares the traversal
//     RolePermission → Permission, so loading a role through its loader hands
//     back every grant with resource and action ALREADY FILLED. There is no
//     second query into the catalog and no id→key resolution step, which is why
//     this service holds no PermissionRepository at all. A read join declared on
//     another aggregate, paying off here.

package infra

import (
	"strconv"
	"sync"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// groupCompanionRepos are the repositories these facts read besides Group's own.
type groupCompanionRepos struct {
	tenants *TenantRepository
	roles   *RoleRepository
}

var (
	groupCompanionsMu sync.Mutex
	// Keyed by the OWNING repository — see note 1 in the file header. Never a
	// bare package-level singleton.
	groupCompanionsByRepo = map[*GroupRepository]*groupCompanionRepos{}
)

// companions returns the two repositories this service reads across, building
// them once per owning GroupRepository.
//
// They share that repository's engine, which is what keeps every probe on the
// same connection pool and the same dialect as the write it is guarding.
// The memo prefix, named for THIS service: the memo hangs off the request's
// AppContext, which every service of the request shares.
const groupRoleMemoPrefix = "authcore.group.role:"

func (s *GroupServiceImpl) companions() *groupCompanionRepos {
	groupCompanionsMu.Lock()
	defer groupCompanionsMu.Unlock()

	if c, ok := groupCompanionsByRepo[s.repo]; ok {
		return c
	}
	c := &groupCompanionRepos{
		tenants: NewTenantRepository(s.repo.Engine),
		roles:   NewRoleRepository(s.repo.Engine),
	}
	groupCompanionsByRepo[s.repo] = c
	return c
}

// The `roleRow` type and its grantsWildcard() USED TO LIVE HERE. They moved to
// role_probe.go on 2026-08-26, when User needed the same answer: the type is
// about a ROLE, not about a group, and a second entity depending on a type
// declared in this file would have made removing Group break User for a reason
// that has nothing to do with groups.
//
// Nothing about the behaviour changed. The resolution below — the read, the memo
// and the repositories — stays here, because those are this service's.

// roleRow resolves one role and its conferred permission keys, memoised for the
// request.
//
// The memo lives on the AppContext, so it is per REQUEST and never shared
// between two writes. Outside a request (tests, background jobs) there is no
// context to memoise on and every call is a fresh read, which is correct rather
// than merely acceptable: a long-lived cache of roles would answer with a role's
// bundle from an arbitrary point in the past, and that bundle is what the
// escalation rule is judging.
//
// THE ACTIVE SCOPE IS THE QUERY DEFAULT AND IS LEFT ALONE, on both levels. An
// archived ROLE does not match, so it reads as absent — which is what the
// single-notification decision requires. An archived (revoked) GRANT is not
// hydrated, so a permission the role no longer confers is not judged: it would
// be fail-closed, but it would refuse an attachment over a grant that does not
// exist.
func (s *GroupServiceImpl) roleRows(roleIDs []domain.ID) map[domain.ID]roleRow {
	return resolveRows(s.ctx, groupRoleMemoPrefix, roleIDs, func(missing []domain.ID) map[string]roleRow {
		q := criteria.Where(criteria.In("ID", idArgs(missing)...))
		found, err := s.companions().roles.Loader.FindAll(s.queryContext(), q)
		if err != nil {
			// A failed probe PANICS rather than inventing an answer: every
			// caller of this row is a security rule, and a plausible answer
			// would skip the invariant it exists to enforce.
			panic("Group: role probe failed for the " + strconv.Itoa(len(missing)) + " role(s) this write attaches")
		}

		rows := make(map[string]roleRow, len(found))
		for _, role := range found {
			// The grants arrive with resource and action already filled — Role
			// declares the traversal into the catalog. Nothing here queries it.
			grants := domain.GetCurrentItemsOf[aggregatevos.RolePermission](role.GetAggregateRoot())
			keys := make([]vos.PermissionKey, 0, len(grants))
			for _, grant := range grants {
				keys = append(keys, vos.PermissionKey{Resource: grant.Resource, Action: grant.Action})
			}
			rows[canonicalIDOf(role.GetID())] = roleRow{found: true, tenantID: role.TenantID, keys: keys}
		}
		return rows
	})
}

func (s *GroupServiceImpl) TenantIsUnavailable(tenantID domain.ID) bool {
	// Usable, not merely non-empty — the same reason roleRow guards its own
	// argument. An unparseable owner is not a tenant that exists, so answering
	// "unavailable" is the true and fail-closed reading.
	if _, err := tenantID.UUID(); err != nil {
		return true
	}

	// By the PRIMARY KEY. Tenant has exactly one identifier now — the derived
	// UUIDv5 that used to be its public key was removed on 2026-08-24 — so the
	// value a caller sends, the value the FK enforces and the value the token
	// claim carries are all this same id.
	//
	// One query for all three conditions: the ACTIVE scope is the default, so an
	// archived row simply does not match, and the status predicate rules out a
	// suspended one. A row that comes back is therefore a tenant that exists, is
	// not archived and is not suspended.
	q := criteria.Where(criteria.And(
		criteria.Eq("ID", tenantID),
		criteria.Ne("Status", vos.TenantStatusSuspended.Value()),
	))
	found, err := s.companions().tenants.Loader.Exists(s.queryContext(), q)
	if err != nil {
		panic("Group: TenantIsUnavailable probe failed")
	}
	return !found
}

// RoleIsUnavailableInTenant reports whether this role id is absent from the
// roles table, points at an archived role, or belongs to a tenant OTHER than the
// one passed.
//
// ONE ANSWER FOR ALL THREE, and collapsing them is the requirement rather than a
// convenience. A distinct "that role belongs to another tenant" reply confirms
// to a caller in tenant A that a specific UUID is a live role in some OTHER
// tenant — an existence oracle over a competitor's org structure. The same
// reasoning that makes a cross-tenant by-id read answer 404 rather than 403.
//
// THE TENANT COMPARED IS THE GROUP'S, NOT THE CALLER'S, and the argument is what
// carries that. On the ordinary path the two are the same value — the write
// guard already refused anything else — but a *:* super-admin crosses that
// scope, and when they do, "this tenant" has to mean the group's or the rule
// would compare a role against the operator's own tenant and refuse every
// legitimate attach.
//
// The database foreign key already covers the EXISTENCE half. What earns this
// probe its keep is the other two: the FK cannot see the archive stamp, and it
// cannot see whose tenant the role is in.
func (s *GroupServiceImpl) RoleIsUnavailableInTenant(tenantID domain.ID, roleIDSet []domain.ID) map[domain.ID]bool {
	rows := s.roleRows(roleIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = !row.found || row.tenantID != tenantID
	}
	return out
}

// RoleGrantsWildcard reports whether the role behind this id confers any
// permission carrying a wildcard in either part. Answers true when the id is
// unknown, so an unresolvable attachment never reaches the escalation probe.
//
// The keys it scans came out of the SAME read RoleIsUnavailableInTenant used —
// filled by the traversal RoleRepository declares into the catalog, so this
// costs no query of its own.
func (s *GroupServiceImpl) RoleGrantsWildcard(roleIDSet []domain.ID) map[domain.ID]bool {
	rows := s.roleRows(roleIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = row.grantsWildcard()
	}
	return out
}

// CallerLacksAnyPermissionOf reports whether the requesting caller fails to hold
// at least one of the permissions this role grants.
//
// THE TRANSITIVE HALF, and what makes this rule different from Role's one level
// down: a role bundles permissions, a group bundles bundles, so the question is
// about a SET and not about one key. Every key must be held; the first one that
// is not refuses the attachment.
//
// It GUARDS THE WILDCARD ITSELF and answers "lacks" rather than calling through
// — defence in depth behind the wildcard rule, because Identity.HasPermission
// panics on any argument containing '*' and a panic on a security rule is a 500
// on exactly the path that rule exists to close.
//
// It judges EVERY key the role confers, including one whose catalog row has
// since been retired. That is the fail-closed direction — a role whose bundle
// contains a retired `tenant:export` is refused to a caller who does not hold
// `tenant:export` — and since the grants no longer carry the catalog row's
// archive stamp it is also the only reading expressible without a second query.
// Strictly more restrictive, never more permissive.
//
// An ABSENT identity stands down (answers false, "the caller holds it"),
// matching authz.noIdentity: an identity is nil only under auth.mode disabled,
// which the framework's own boot guard permits in the dev profile alone. An
// identity that IS present and lacks a permission refuses.
func (s *GroupServiceImpl) CallerLacksAnyPermissionOf(roleIDSet []domain.ID) map[domain.ID]bool {
	if s.ctx == nil {
		return nil
	}
	identity := s.ctx.Identity()
	if identity == nil {
		// STANDS DOWN WITHOUT READING ANYTHING. An empty answer raises nothing
		// — an absent key is this fact answering nothing for that entry — and
		// there is no caller to compare the bundles against, so resolving them
		// would be work for no verdict.
		return nil
	}

	rows := s.roleRows(roleIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = callerLacksAnyPermissionOfRole(identity, row)
	}
	return out
}

var _ appdomain.GroupService = (*GroupServiceImpl)(nil)
