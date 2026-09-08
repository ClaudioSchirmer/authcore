// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written facts for Role (5 to implement).
//
// entity:     Role
// spec:       specs/omnicore-gen/role.omnicore.yaml
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

// All five are implemented below. Two design notes that are load-bearing:
//
//  1. THE COMPANION REPOSITORIES. Four of these facts read tables this
//     aggregate does not own — tenants and permissions. The generated impl
//     carries only its own repository, so the companions are built lazily and
//     cached KEYED BY THE OWNING RoleRepository. Keying matters: a package-level
//     sync.Once would build them from the first engine it ever saw and hand
//     those to every service built afterwards, which is wrong the moment a test
//     or a second bootstrap uses a different engine.
//
//  2. ONE CATALOG READ PER ENTRY, NOT THREE. PermissionIsNotInCatalog,
//     PermissionIsWildcard and CallerDoesNotHoldPermission all ask about the
//     same catalog row. They funnel through catalogRow, memoised on the
//     REQUEST-scoped AppContext, so three questions about one id cost one
//     query. A role at the 250-permission cap therefore pays 250 round trips
//     inside the write transaction rather than 750.

package infra

import (
	"errors"
	"github.com/ClaudioSchirmer/authcore/internal/infra/utils"
	"strconv"
	"sync"
	"time"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// roleCompanionRepos are the repositories these facts read besides Role's own.
type roleCompanionRepos struct {
	tenants     *TenantRepository
	permissions *PermissionRepository
}

var (
	roleCompanionsMu sync.Mutex
	// Keyed by the OWNING repository — see note 1 in the file header. Never a
	// bare package-level singleton.
	roleCompanionsByRepo = map[*RoleRepository]*roleCompanionRepos{}
)

// companions returns the two repositories this service reads across, building
// them once per owning RoleRepository.
//
// They share that repository's engine, which is what keeps every probe on the
// same connection pool and the same dialect as the write it is guarding.
func (s *RoleServiceImpl) companions() *roleCompanionRepos {
	roleCompanionsMu.Lock()
	defer roleCompanionsMu.Unlock()

	if c, ok := roleCompanionsByRepo[s.repo]; ok {
		return c
	}
	c := &roleCompanionRepos{
		tenants:     NewTenantRepository(s.repo.Engine),
		permissions: NewPermissionRepository(s.repo.Engine),
	}
	roleCompanionsByRepo[s.repo] = c
	return c
}

// catalogRow is everything the three per-entry facts need to know about one
// catalog permission, resolved in a single read.
//
// found is false for an id no catalog row carries. archivedAt is non-nil when
// the row exists but has been retired — Permission archives ONE WAY, so that is
// a normal long-lived state and not an edge case.
// The memo prefix, named for THIS service: the memo hangs off the request's
// AppContext, which every service of the request shares.
const roleCatalogMemoPrefix = "authcore.role.catalog:"

type catalogRow struct {
	found      bool
	archivedAt *time.Time
	resource   string
	action     string
}

func (r catalogRow) active() bool { return r.found && r.archivedAt == nil }

// isWildcard reports whether either half is the wildcard.
//
// An id that resolves to nothing answers TRUE. That is deliberate and it is the
// fail-closed direction: an unresolvable grant must never reach the escalation
// probe, which would hand its key to Identity.HasPermission — and that panics on
// a wildcard and on an empty string alike.
func (r catalogRow) isWildcard() bool {
	if !r.found {
		return true
	}
	return r.resource == vos.PermissionWildcard || r.action == vos.PermissionWildcard
}

// catalogRows resolves every requested catalog permission in ONE read,
// memoised for the request — so the three facts below share it and a role
// granting twenty permissions pays for one query.
//
// The batch arithmetic — dedup, the canonical key, the parse guard, what an id
// the read did not answer for means — lives in row_resolution.go.
//
// ARCHIVED ROWS ARE INCLUDED in the read, and that is this resolver's one
// difference from its neighbours. The three callers need to tell "no such
// permission" from "a permission that was retired", and an active-only scope
// collapses both into "not found".
func (s *RoleServiceImpl) catalogRows(permissionIDs []domain.ID) map[domain.ID]catalogRow {
	return utils.ResolveRows(s.ctx, roleCatalogMemoPrefix, permissionIDs, func(missing []domain.ID) map[string]catalogRow {
		q := criteria.Where(criteria.In("ID", utils.IDArgs(missing)...)).IncludeArchived()
		found, err := s.companions().permissions.Loader.FindAll(s.queryContext(), q)
		if err != nil {
			// A failed probe PANICS rather than inventing an answer. The
			// pipeline turns it into a 500 and the write never happens — the
			// only safe outcome, because every caller of this row is a security
			// rule and a plausible answer would skip the invariant it enforces.
			panic("Role: catalog probe failed for the " + strconv.Itoa(len(missing)) + " permission(s) this write grants")
		}

		rows := make(map[string]catalogRow, len(found))
		for _, permission := range found {
			rows[utils.CanonicalIDOf(permission.GetID())] = catalogRow{
				found:      true,
				archivedAt: permission.GetArchivedAt(),
				resource:   permission.Permission.Resource,
				action:     permission.Permission.Action,
			}
		}
		return rows
	})
}

// isRecordNotFound separates "the row is not there" from "the query failed".
//
// FindOne answers a miss with the framework's canonical *domain.DomainError;
// anything else is infrastructure trouble and must not be read as absence.
func isRecordNotFound(err error) bool {
	var domainErr *domain.DomainError
	return errors.As(err, &domainErr)
}

// TenantIsUnavailable reports whether the owning tenant is missing, archived, or
// commercially SUSPENDED. Queries the tenants table by its primary key.
//
// Named for the PROBLEM rather than the healthy state, which is what lets the
// generated suite's zero-value service stub read as "nothing is wrong".
//
// THREE conditions, not two, and the third is the one that is easy to get
// wrong. Tenant carries its own commercial lifecycle beside the archive stamp,
// and the two are orthogonal: archiving forces `suspended`, but a tenant can be
// suspended while perfectly un-archived — a customer who stopped paying. A role
// is what grants access, so minting one inside a suspended tenant hands out
// access the commercial state says to withhold.
//
// TRIAL IS AVAILABLE, and that is deliberate. "Unavailable" is not "not
// active": a trial is a live customer being onboarded, and roles are exactly
// what they need. Only `suspended` withholds. Reading this as `Status !=
// active` would break every trial signup.
func (s *RoleServiceImpl) TenantIsUnavailable(tenantID domain.ID) bool {
	// Usable, not merely non-empty — the same reason catalogRow guards its own
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
		panic("Role: TenantIsUnavailable probe failed")
	}
	return !found
}

// PermissionIsNotInCatalog reports whether this permission id is absent from the
// catalog or points at an archived permission.
//
// Both halves answer true, and collapsing them is correct here: a grant may
// point at neither. The README's rule is what makes the archived half matter —
// a retired permission comes back as a NEW row with a NEW id, so re-granting the
// old id must be refused rather than silently honoured.
func (s *RoleServiceImpl) PermissionIsNotInCatalog(permissionIDSet []domain.ID) map[domain.ID]bool {
	rows := s.catalogRows(permissionIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = !row.active()
	}
	return out
}

// PermissionIsWildcard reports whether the catalog row behind this id carries a
// wildcard in either part. Answers true when the id is unknown, so an
// unresolvable grant never reaches the escalation probe.
func (s *RoleServiceImpl) PermissionIsWildcard(permissionIDSet []domain.ID) map[domain.ID]bool {
	rows := s.catalogRows(permissionIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		out[id] = row.isWildcard()
	}
	return out
}

// CallerDoesNotHoldPermission reports whether the requesting caller lacks the
// permission behind this id.
//
// It GUARDS THE WILDCARD ITSELF and answers "does not hold" rather than calling
// through — defence in depth behind the wildcard rule, because
// Identity.HasPermission panics on any argument containing '*' and a panic on a
// security rule is a 500 on exactly the path that rule exists to close.
//
// An ABSENT identity stands down (answers false, "the caller does holds it"),
// matching authz.noIdentity: an identity is nil only under auth.mode disabled,
// which the framework's own boot guard permits in the dev profile alone. An
// identity that IS present and lacks the permission refuses.
func (s *RoleServiceImpl) CallerDoesNotHoldPermission(permissionIDSet []domain.ID) map[domain.ID]bool {
	if s.ctx == nil {
		return nil
	}
	identity := s.ctx.Identity()
	if identity == nil {
		// STANDS DOWN WITHOUT READING ANYTHING. An empty answer raises nothing
		// — an absent key is this fact answering nothing for that entry — and
		// there is no caller to compare the catalog against, so resolving it
		// would be work for no verdict.
		return nil
	}

	rows := s.catalogRows(permissionIDSet)

	out := make(map[domain.ID]bool, len(rows))
	for id, row := range rows {
		if row.isWildcard() {
			// Never handed to HasPermission — it panics on a wildcard. The
			// wildcard rule refuses this grant on its own; this is the second
			// lock, and it is why the batch may ask about every entry.
			out[id] = true
			continue
		}
		key := vos.PermissionKey{Resource: row.resource, Action: row.action}
		out[id] = !identity.HasPermission(key.String())
	}
	return out
}

// CallerIsSuperAdmin reports whether the caller holds *:*.
//
// ctx.Identity().IsSuperAdmin() and nothing else — never HasPermission("*:*"),
// which panics by design, and never a hand-read of the claim, whose NAME is
// configurable via authorization.permissionsClaim. Note a resource wildcard is
// NOT a superadmin grant: role:* reports false.
func (s *RoleServiceImpl) CallerIsSuperAdmin() bool {
	if s.ctx == nil {
		return false
	}
	return s.ctx.Identity().IsSuperAdmin()
}

var _ appdomain.RoleService = (*RoleServiceImpl)(nil)
