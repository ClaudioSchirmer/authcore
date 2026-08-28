// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written facts for Claim (1 to implement).
//
// entity:     Claim
// spec:       specs/omnicore-gen/claim.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-28
//
// It is implemented below. One design note, carried over from Role's identical
// probe rather than rediscovered:
//
//	THE COMPANION REPOSITORY. This fact reads a table this aggregate does not
//	own. The generated impl carries only its own repository, so the companion
//	is built lazily and cached KEYED BY THE OWNING ClaimRepository. Keying
//	matters: a package-level sync.Once would build it from the first engine it
//	ever saw and hand that to every service built afterwards, which is wrong
//	the moment a test or a second bootstrap uses a different engine.
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

package infra

import (
	"sync"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

var (
	claimCompanionsMu sync.Mutex
	// Keyed by the OWNING repository — see the note in the file header. Never a
	// bare package-level singleton.
	claimTenantsByRepo = map[*ClaimRepository]*TenantRepository{}
)

// tenants returns the repository this fact reads across, building it once per
// owning ClaimRepository.
//
// It shares that repository's engine, which is what keeps the probe on the same
// connection pool and the same dialect as the write it is guarding.
func (s *ClaimServiceImpl) tenants() *TenantRepository {
	claimCompanionsMu.Lock()
	defer claimCompanionsMu.Unlock()

	if repo, ok := claimTenantsByRepo[s.repo]; ok {
		return repo
	}
	repo := NewTenantRepository(s.repo.Engine)
	claimTenantsByRepo[s.repo] = repo
	return repo
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
// suspended while perfectly un-archived — a customer who stopped paying.
// Minting a claim definition inside a suspended tenant would extend the
// vocabulary of tokens that tenant is no longer entitled to be issued.
//
// TRIAL IS AVAILABLE, and that is deliberate. "Unavailable" is not "not
// active": a trial is a live customer being onboarded. Reading this as
// `Status != active` would break every trial signup.
func (s *ClaimServiceImpl) TenantIsUnavailable(tenantID domain.ID) bool {
	// Usable, not merely non-empty. This guard is LOAD-BEARING rather than
	// defensive: the framework runs every IfInsert clause before every
	// IfInsertOrUpdate one, so the rule that calls this fires BEFORE the
	// generated `tenant-is-a-usable-id` barrier — an unparseable owner reaches
	// here on an ordinary bad request. It is not a tenant that exists, so
	// answering "unavailable" is both the true and the fail-closed reading.
	if _, err := tenantID.UUID(); err != nil {
		return true
	}

	// By the PRIMARY KEY: the value a caller sends, the value the FK enforces
	// and the value the tenant_id token claim carries are all this same id.
	//
	// One query for all three conditions: the ACTIVE scope is the query
	// default, so an archived row simply does not match, and the status
	// predicate rules out a suspended one. A row that comes back is therefore a
	// tenant that exists, is not archived and is not suspended.
	q := criteria.Where(criteria.And(
		criteria.Eq("ID", tenantID),
		criteria.Ne("Status", vos.TenantStatusSuspended.Value()),
	))
	found, err := s.tenants().Loader.Exists(s.queryContext(), q)
	if err != nil {
		// A plausible answer here would skip the very rule this exists to
		// enforce. The pipeline turns the panic into a 500 with the write
		// rolled back, which is the honest outcome for an unreachable source.
		panic("Claim: TenantIsUnavailable probe failed")
	}
	return !found
}
