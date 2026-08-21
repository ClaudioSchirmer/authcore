// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written facts for Role (4 to implement).
//
// entity:     Role
// spec:       specs/omnicore-gen/role.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-21
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

package infra

import (
	"errors"
	"sync"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// The two cross-aggregate repositories these facts read through.
//
// They are built lazily from the SAME relational engine the role repository
// already holds, rather than injected: NewRoleServiceImpl is generated from the
// spec and takes the role repository alone, and RoleServiceImpl is a generated
// struct this file cannot add a field to. Building them here keeps the wiring
// untouched and costs one construction per owning repository — not one per
// call, because a repository construction runs the framework's schema boot
// checks.
//
// KEYED BY THE OWNING REPOSITORY, and that is the whole point of the map. A
// bare sync.Once would build the pair from whichever RoleServiceImpl asked
// first and then hand those same two repositories to every later one — so a
// second engine would silently read the FIRST engine's tenants and permissions,
// which is a wrong answer to a security rule rather than a failure anybody
// notices. The key is a pointer, deliberately: the engine is an interface whose
// dynamic type is not guaranteed comparable, and a map key that can panic has
// no place on the write path. Two repositories sharing one engine would build
// the pair twice, which costs a construction and is never wrong.
//
// They are read-only here. Nothing in this file writes through them.
type roleCrossRepos struct {
	tenants *TenantRepository
	perms   *PermissionRepository
}

var (
	roleCrossReposMu sync.Mutex
	roleCrossReposBy = map[*RoleRepository]*roleCrossRepos{}
)

// crossRepos returns the two catalogs these facts consult, building them once
// per owning repository.
//
// The lock is held across the construction so two concurrent first requests
// cannot both build: that is one boot-check pass on a cold process, never a
// per-call cost, since every later call takes the hit of an uncontended lock
// and a map read.
func (s *RoleServiceImpl) crossRepos() (*TenantRepository, *PermissionRepository) {
	roleCrossReposMu.Lock()
	defer roleCrossReposMu.Unlock()

	if built, ok := roleCrossReposBy[s.repo]; ok {
		return built.tenants, built.perms
	}
	built := &roleCrossRepos{
		tenants: NewTenantRepository(s.repo.Engine),
		perms:   NewPermissionRepository(s.repo.Engine),
	}
	roleCrossReposBy[s.repo] = built
	return built.tenants, built.perms
}

// permissionProbeMemoKey is where this request's resolved catalog rows live on
// the AppContext. Namespaced, because the metadata map is shared with the whole
// framework and with every other feature of this service.
const permissionProbeMemoKey = "authcore.role.permissionProbe"

// permissionMemo returns this REQUEST's resolved-permission cache, creating it
// on first use. It answers nil where there is nothing to scope a cache to.
//
// AppContext.Set/Get is the framework's sanctioned request-scoped store
// (docs, app-context: "extra request-scoped state ... via ctx.Set(key, val)"),
// and it is the right lifetime here: the map must not outlive the write, or a
// permission archived between two requests would keep reading as active. It is
// safe as a plain map because the rules walk the collection sequentially inside
// one write; the entry never escapes to another goroutine.
//
// Outside a request — a test or a background job — s.ctx is nil and there is no
// cache. The probe still answers, one query at a time.
func (s *RoleServiceImpl) permissionMemo() map[domain.ID]*appdomain.Permission {
	if s.ctx == nil {
		return nil
	}
	if v, ok := s.ctx.Get(permissionProbeMemoKey); ok {
		// Somebody else owning this key is not something to fight over: fall
		// back to querying rather than corrupt whatever they stored.
		memo, ok := v.(map[domain.ID]*appdomain.Permission)
		if !ok {
			return nil
		}
		return memo
	}
	memo := map[domain.ID]*appdomain.Permission{}
	s.ctx.Set(permissionProbeMemoKey, memo)
	return memo
}

// findActivePermission resolves a grant's catalog row.
//
// It is the ONE place these facts read the permissions table, so the three
// questions asked per entry cannot drift apart on what "the catalog row" means.
//
// Three outcomes, and the caller needs all three apart: the row is there
// (found), the row is absent or archived (not found — an ordinary answer, since
// the active scope is the query default), or the query itself failed. The last
// one PANICS rather than returning "absent": the pipeline turns the panic into a
// 500 and the write never happens, whereas a plausible answer would skip the
// very invariant the fact exists to enforce.
//
// The answer is MEMOISED for the request. Three facts ask about the same entry
// — is it in the catalog, is it a wildcard, does the caller hold it — so
// without this a role at the 200-permission cap would pay 600 round trips
// inside one write transaction to learn 200 things. A not-found is cached as
// such (a nil row), because "absent or archived" is an answer and re-asking it
// costs exactly as much as asking it.
func (s *RoleServiceImpl) findActivePermission(permissionID domain.ID) (*appdomain.Permission, bool) {
	memo := s.permissionMemo()
	if memo != nil {
		if cached, seen := memo[permissionID]; seen {
			return cached, cached != nil
		}
	}

	_, perms := s.crossRepos()
	q := criteria.Where(criteria.Eq("ID", permissionID))

	p, err := perms.Loader.FindOne(s.queryContext(), q)
	if err != nil {
		// A not-found is a *domain.DomainError carrying
		// RecordNotFoundNotification, and it is an ANSWER here, not a failure.
		var notFound *domain.DomainError
		if errors.As(err, &notFound) {
			if memo != nil {
				memo[permissionID] = nil
			}
			return nil, false
		}
		// A FAILED query is never cached: the next ask must reach the database
		// again rather than inherit a verdict nothing ever established.
		panic("Role: permission catalog probe failed")
	}
	if memo != nil {
		memo[permissionID] = p
	}
	return p, true
}

// Whether the owner tenant is unknown or archived. Not answerable from
// this entity's own table — it is a question about the tenants table, so
// the implementation holds the tenant repository beside its own.
//
// It asks the database the question directly rather than loading the tenant and
// inspecting it: this is a yes/no, and the existence probe exists so a yes/no
// does not pay for full hydration.
//
// "Unavailable" is exactly the spec's R4 — unknown OR archived — and
// deliberately not "suspended". Status is orthogonal to archiving in this
// service (a suspended tenant still authenticates for billing), so refusing a
// role under one would be a rule nobody declared.
//
// Note the field: the match is on TenantID, the PUBLIC derived key every
// tenant-scoped row references, never on the surrogate id.
func (s *RoleServiceImpl) TenantIsUnavailable(tenantID domain.ID) bool {
	tenants, _ := s.crossRepos()
	// Archived rows do not take part: the active scope is the query default, so
	// an archived tenant simply does not match and the fact answers true.
	q := criteria.Where(criteria.Eq("TenantID", tenantID))

	found, err := tenants.Loader.Exists(s.queryContext(), q)
	if err != nil {
		panic("Role: TenantIsUnavailable probe failed")
	}
	return !found
}

// Whether this granted permission is absent from the catalog or archived.
// Asked once per entry. Not answerable declaratively — the column lives
// on the permissions table, not on this entity's.
//
// The database foreign key already guarantees the EXISTENCE half. What this
// answers that the constraint cannot is the ACTIVE half — a permission the
// platform has retired — and it turns both into a readable 422 instead of a raw
// constraint error arriving alone, after everything else passed.
func (s *RoleServiceImpl) PermissionIsNotInCatalog(permissionID domain.ID) bool {
	_, found := s.findActivePermission(permissionID)
	return !found
}

// Whether the catalog row this grant points at carries a wildcard in
// either part. Asked once per entry, and answered by resolving the id to
// its vos.PermissionKey. It is what keeps every wildcard string away from
// Identity.HasPermission, which panics on one — so it must answer TRUE
// for an unknown id too, rather than let an unresolvable grant fall
// through to the caller-holds question.
//
// Answering TRUE for an unknown id is the important half: an id this cannot
// resolve must not fall through to the caller-holds question, where the
// resolution would be attempted again on a value nothing has vetted. The
// catalog rule is reported separately by PermissionIsNotInCatalog, so refusing
// here costs the caller no clarity — they read both answers.
func (s *RoleServiceImpl) PermissionIsWildcard(permissionID domain.ID) bool {
	p, found := s.findActivePermission(permissionID)
	if !found {
		return true
	}
	return p.Key.Resource == vos.Wildcard || p.Key.Action == vos.Wildcard
}

// Whether the authenticated caller lacks the permission this grant points
// at. Asked once per entry, and only for concrete keys —
// no-wildcard-grant has already refused the wildcards. Implementation:
// resolve the id to its key; return false when ctx.Identity() is nil (auth
// disabled, dev only) so the rule stands down; otherwise answer NOT
// Identity.HasPermission(key). Guard the wildcard here as well and answer
// true rather than calling through — a panic on a security rule is a
// 500.
//
// TWO absent states, and collapsing them breaks one profile or the other:
//
//   - No Identity AT ALL — the middleware never populated one, which is
//     provably a development bench: auth.mode disabled is the only way there,
//     and the framework refuses that mode outside APP_PROFILE=dev. The rule
//     stands down, so the service stays usable with no tokens at all.
//   - An Identity that is PRESENT but does not hold the permission — an
//     ordinary authenticated request. REFUSED, fail closed.
//
// A *:* super-admin needs no branch: HasPermission answers true for any
// concrete permission when the claim set carries the wildcard, so the exemption
// falls out of the framework rather than out of claim parsing of our own.
//
// The wildcard is guarded here as well as in PermissionIsWildcard. That is
// defence in depth and not redundancy: HasPermission PANICS on an argument
// containing *, and a panic on a security rule is a 500 on the exact input the
// rule exists to refuse.
func (s *RoleServiceImpl) CallerDoesNotHoldPermission(permissionID domain.ID) bool {
	// s.ctx is nil outside a request — a test or a background job — and
	// AppContext.Identity takes a read lock, so it is not nil-receiver safe.
	// Both roads lead to the same place as an unauthenticated request: there is
	// nobody to scope to, so the rule stands down.
	if s.ctx == nil {
		return false
	}
	identity := s.ctx.Identity()
	if identity == nil {
		return false
	}

	p, found := s.findActivePermission(permissionID)
	if !found {
		return true
	}
	if p.Key.Resource == vos.Wildcard || p.Key.Action == vos.Wildcard {
		return true
	}
	return !identity.HasPermission(p.Key.String())
}
