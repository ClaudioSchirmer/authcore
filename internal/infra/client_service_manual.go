// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written facts for Client (5 to implement).
//
// entity:     Client
// spec:       specs/omnicore-gen/client.omnicore.yaml
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
//
// FOUR THINGS THIS FILE DECIDES, and each one is a place the obvious answer is
// wrong:
//
//  1. THE COMPANION REPOSITORIES ARE KEYED BY THE OWNING REPOSITORY, never a
//     package-level singleton. They share that repository's engine, which is
//     what keeps every probe on the same connection pool and the same dialect
//     as the write it is guarding — and what keeps two services built over two
//     engines (a test and a boot) from stealing each other's.
//
//  2. THE ROLE WALK IS SHARED WITH User, through roleRow in role_probe.go. It
//     is the same question — "one role, resolved, and what does it confer" —
//     and two copies would be one security rule able to disagree with itself
//     about the case that matters most.
//
//  3. A FAILED PROBE PANICS RATHER THAN INVENTING AN ANSWER. Every caller of
//     these facts is a security rule; a plausible answer skips the invariant the
//     fact exists to enforce. The panic becomes a 500 with the write rolled
//     back, which is the honest outcome.
//
//  4. THE HASHER IS NOT A QUERY. HashSecret is on this port for one reason: the
//     domain must not import a digest package. It costs no round trip, never
//     logs its input, and is the only fact here that would answer the same in an
//     empty database. It is SHA-256 and NOT Argon2id — see secret_hasher.go and
//     specs/scaffold-entity/client/spec.md §B-Q4.

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

// clientCompanionRepos are the repositories these facts read besides Client's own.
type clientCompanionRepos struct {
	tenants *TenantRepository
	roles   *RoleRepository
}

var (
	clientCompanionsMu sync.Mutex
	// Keyed by the OWNING repository — see note 1 in the file header.
	clientCompanionsByRepo = map[*ClientRepository]*clientCompanionRepos{}
)

// clientHasher is the one hasher these facts use.
//
// A package-level value rather than a field because the adapter is stateless
// and carries no engine: there is nothing per-repository about it, and nothing
// to key it by. Exactly how userHasher is held, one file over.
var clientHasher = NewSHA256SecretHasher()

// companions returns the two repositories this service reads across, building
// them once per owning ClientRepository.
func (s *ClientServiceImpl) companions() *clientCompanionRepos {
	clientCompanionsMu.Lock()
	defer clientCompanionsMu.Unlock()

	if c, ok := clientCompanionsByRepo[s.repo]; ok {
		return c
	}
	c := &clientCompanionRepos{
		tenants: NewTenantRepository(s.repo.Engine),
		roles:   NewRoleRepository(s.repo.Engine),
	}
	clientCompanionsByRepo[s.repo] = c
	return c
}

// roleRow resolves one role and its conferred permission keys, memoised for the
// request. Same contract and same shape as UserServiceImpl's, on this service's
// context — see role_probe.go for what the type MEANS, which is the half both
// callers must agree on.
//
// The memo lives on the AppContext, so it is per REQUEST and never shared
// between two writes. Outside a request there is no context to memoise on and
// every call is a fresh read, which is correct rather than merely acceptable: a
// long-lived cache would answer with a role's bundle from an arbitrary point in
// the past, and that bundle is what the escalation rule is judging.
func (s *ClientServiceImpl) roleRow(roleID domain.ID) roleRow {
	const memoPrefix = "authcore.client.role:"

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
		panic("Client: role probe failed for role " + roleID.String())
	}

	if s.ctx != nil {
		s.ctx.Set(memoPrefix+roleID.String(), row)
	}
	return row
}

// HashSecret returns the SHA-256 of the secret, lowercase hex.
//
// NOT Argon2id, and the reason is not frugality: memory-hardness makes OFFLINE
// guessing expensive, which buys nothing against 256 random bits and costs
// 19 MiB per verify on an endpoint that is unauthenticated by construction. The
// whole argument, including what it would cost on the miss path once a rotation
// is in flight, is in specs/scaffold-entity/client/spec.md §B-Q4.
//
// It never logs its input, and whatever verifies against it compares with
// crypto/subtle — which is why verification is a method on the port rather than
// something a caller does with the string this returns.
func (s *ClientServiceImpl) HashSecret(secret string) string {
	return clientHasher.Hash(secret)
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
// perfectly un-archived — a customer who stopped paying. Minting a machine
// credential inside one hands out access the commercial state says to withhold.
//
// TRIAL IS AVAILABLE, deliberately. "Unavailable" is not "not active": a trial
// is a live customer being onboarded, and an integration is often the first
// thing they wire up.
func (s *ClientServiceImpl) TenantIsUnavailable(tenantID domain.ID) bool {
	// Usable, not merely non-empty — the same reason the row probe guards its
	// argument. An unparseable owner is not a tenant that exists, so
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
		panic("Client: TenantIsUnavailable probe failed")
	}
	return !found
}

// RoleIsUnavailableInTenant reports whether this role id is absent from the
// roles table, points at an archived role, or belongs to a tenant OTHER than the
// one passed.
//
// ONE ANSWER FOR ALL THREE, and collapsing them is the requirement rather than a
// convenience — a distinct "that role belongs to another tenant" reply confirms
// to a caller in tenant A that a specific UUID is a live role in some other
// tenant.
//
// THE TENANT COMPARED IS THE CLIENT'S, NOT THE CALLER'S, and the argument
// carries it. On the ordinary path the two are the same value — the write guard
// refused anything else — but a `*:*` super-admin crosses that scope, and when
// they do, "this tenant" has to mean the client's or the rule would compare a
// role against the operator's own tenant and refuse every legitimate grant.
func (s *ClientServiceImpl) RoleIsUnavailableInTenant(tenantID domain.ID, roleID domain.ID) bool {
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
func (s *ClientServiceImpl) RoleGrantsWildcard(roleID domain.ID) bool {
	return s.roleRow(roleID).grantsWildcard()
}

// CallerLacksAnyPermissionOfRole reports whether the requesting caller fails to
// hold at least one of the permissions this role grants.
//
// It judges EVERY key the role confers, including one whose catalog row has
// since been retired. That is the fail-closed direction, and since the grants no
// longer carry the catalog row's archive stamp it is also the only reading
// expressible without a second query. Strictly more restrictive, never more
// permissive.
func (s *ClientServiceImpl) CallerLacksAnyPermissionOfRole(roleID domain.ID) bool {
	if s.ctx == nil {
		return false
	}
	identity := s.ctx.Identity()
	if identity == nil {
		return false
	}
	return s.callerLacksAnyPermissionOfRole(identity, roleID)
}

func (s *ClientServiceImpl) callerLacksAnyPermissionOfRole(identity *configuration.Identity, roleID domain.ID) bool {
	row := s.roleRow(roleID)
	if row.grantsWildcard() {
		// Never handed to HasPermission — it panics on a wildcard, and this also
		// covers the unresolvable role. The wildcard rule refuses this write on
		// its own; this is the second lock.
		return true
	}
	for _, key := range row.keys {
		if !identity.HasPermission(key.String()) {
			return true
		}
	}
	return false
}

var _ appdomain.ClientService = (*ClientServiceImpl)(nil)
