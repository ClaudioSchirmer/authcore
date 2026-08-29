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
	"strconv"
	"sync"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// clientCompanionRepos are the repositories these facts read besides Client's own.
type clientCompanionRepos struct {
	tenants *TenantRepository
	roles   *RoleRepository
	claims  *ClaimRepository
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

// companions returns the three repositories this service reads across, building
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
		claims:  NewClaimRepository(s.repo.Engine),
	}
	clientCompanionsByRepo[s.repo] = c
	return c
}

// The memo prefixes. One per row type, so the two resolvers never collide, and
// named for THIS service: the memo hangs off the request's AppContext, which
// every service of the request shares.
const (
	clientRoleMemoPrefix  = "authcore.client.role:"
	clientClaimMemoPrefix = "authcore.client.claim:"
)

// roleRows resolves every requested role and the permission keys it confers in
// ONE read, memoised for the request. Same row type GroupServiceImpl and
// UserServiceImpl use, on this service's context — and the same batch
// arithmetic, which lives in row_resolution.go.
func (s *ClientServiceImpl) roleRows(roleIDs []domain.ID) map[domain.ID]roleRow {
	return resolveRows(s.ctx, clientRoleMemoPrefix, roleIDs, func(missing []domain.ID) map[string]roleRow {
		q := criteria.Where(criteria.In("ID", idArgs(missing)...))
		found, err := s.companions().roles.Loader.FindAll(s.queryContext(), q)
		if err != nil {
			// A failed probe PANICS rather than inventing an answer: every
			// caller of this row is a security rule, and a plausible answer
			// would skip the invariant it exists to enforce.
			panic("Client: role probe failed for the " + strconv.Itoa(len(missing)) + " role(s) this write grants")
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
func (s *ClientServiceImpl) RoleIsUnavailableInTenant(tenantID domain.ID, roleIDSet []domain.ID) map[domain.ID]bool {
	rows := s.roleRows(roleIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = !row.found || row.tenantID != tenantID
	}
	return out
}

// RoleGrantsWildcard reports whether the role behind this id confers any
// permission carrying a wildcard. Answers true for an unknown id.
//
// The keys it scans came out of the SAME read RoleIsUnavailableInTenant used —
// filled by the traversal RoleRepository declares into the catalog, so this
// costs no query of its own.
func (s *ClientServiceImpl) RoleGrantsWildcard(roleIDSet []domain.ID) map[domain.ID]bool {
	rows := s.roleRows(roleIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = row.grantsWildcard()
	}
	return out
}

// CallerLacksAnyPermissionOfRole reports whether the requesting caller fails to
// hold at least one of the permissions this role grants.
//
// It judges EVERY key the role confers, including one whose catalog row has
// since been retired. That is the fail-closed direction, and since the grants no
// longer carry the catalog row's archive stamp it is also the only reading
// expressible without a second query. Strictly more restrictive, never more
// permissive.
func (s *ClientServiceImpl) CallerLacksAnyPermissionOfRole(roleIDSet []domain.ID) map[domain.ID]bool {
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

var _ appdomain.ClientService = (*ClientServiceImpl)(nil)

// ── the claims collection: one row, three questions ─────────────────────────
//
// claimRow is what a single load of a claim definition answers, memoised per
// request. The three facts below each need a different part of the SAME row —
// whether it exists at all, which tenant owns it, which identity kinds it
// admits, and what its values must parse as — so a probe per question would be
// three reads of one row per entry of the collection.

// claimRows resolves every requested definition in ONE read, memoised for the
// request — so the three facts below share it and the write pays for one query
// however many claims it carries.
func (s *ClientServiceImpl) claimRows(claimIDs []domain.ID) map[domain.ID]claimRow {
	return resolveRows(s.ctx, clientClaimMemoPrefix, claimIDs, func(missing []domain.ID) map[string]claimRow {
		q := criteria.Where(criteria.In("ID", idArgs(missing)...))
		found, err := s.companions().claims.Loader.FindAll(s.queryContext(), q)
		if err != nil {
			panic("Client: claim probe failed for the " + strconv.Itoa(len(missing)) + " claim(s) this write holds")
		}

		// ARCHIVED IS SIMPLY ABSENT, and that is the point rather than a
		// coincidence: the loader filters the archived rows out, so a retired
		// definition never comes back and lands as the zero row — exactly the
		// answer the rules want for it.
		rows := make(map[string]claimRow, len(found))
		for _, claim := range found {
			rows[canonicalIDOf(claim.GetID())] = claimRow{
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
// competitor's claim vocabulary.
func (s *ClientServiceImpl) ClaimIsUnavailableInTenant(tenantID domain.ID, claimIDSet []domain.ID) map[domain.ID]bool {
	rows := s.claimRows(claimIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = !row.found || row.tenantID != tenantID
	}
	return out
}

// ClaimDoesNotApplyToClient answers whether the definition excludes machine
// clients.
//
// TRUE for an unknown id, the direction every probe in this file takes: a fact
// that cannot resolve its subject reports the problem as present, so an
// unresolvable entry can never pass a check by accident. In practice the
// availability rule refuses it first and the rule never reads this entry's
// answer — the answer is computed anyway, because it rode the same read.
func (s *ClientServiceImpl) ClaimDoesNotApplyToClient(claimIDSet []domain.ID) map[domain.ID]bool {
	rows := s.claimRows(claimIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		// ClaimAppliesToClient and Both admit a client; User does not, and
		// neither does the Unknown sentinel — a value outside the closed set is
		// not a permission to hold anything.
		out[id] = !row.found ||
			(row.appliesTo != vos.ClaimAppliesToClient && row.appliesTo != vos.ClaimAppliesToBoth)
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
func (s *ClientServiceImpl) ClaimValueDoesNotMatchValueType(entries []appdomain.ClientClaimValueDoesNotMatchValueTypeEntry) map[domain.ID]bool {
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
