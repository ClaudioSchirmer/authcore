// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written facts for Claim (3 to implement).
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

// ── the narrowing guard: does anybody still hold a value for this definition ──
//
// These two are the only probes in this service that read a COLLECTION table
// rather than an aggregate root, and that is why they drop to SQL instead of a
// criteria. An AggregateLoader answers questions about roots — "is there a claim
// like this" — and the question here is "is there an ENTRY of another
// aggregate's collection pointing at this row", which no criteria over `claims`
// can phrase and no criteria over `users` can either: a collection's columns are
// load-only, served inside the entry and never filterable. Querier is the
// framework's own seam for exactly this ("the loader, composer and the
// consumer's own custom reads"), and both statements are strictly READ-ONLY.
//
// Dialect-neutral by construction rather than by luck: every identifier goes
// through QuoteIdent, every bound value through EncodeArg, and the placeholders
// come from Placeholder. Postgres is this project's only target today, and none
// of that is a reason to hand-render "$1".

// claimIsHeldBy answers whether an ACTIVE row of the given edge table points at
// the ACTIVE claim definition identified by this tenant and name.
//
// FILTERED BY THE NATURAL KEY, not the row id, because a fact's filters must
// name declared fields of the spec and the primary key is not one of them.
// (TenantID, Name) identifies the definition exactly: both are immutable, and
// Name is unique per tenant.
//
// TWO archive predicates, and the second is the one that is easy to miss. That
// uniqueness is scoped to the ACTIVE rows, so an archived definition may share
// the pair with the live one — and its leftover edges would otherwise block a
// narrowing on the row that replaced it, forever, with no way to see why.
func (s *ClaimServiceImpl) claimIsHeldBy(edgeTable string, tenantID domain.ID, name string) bool {
	// Usable, not merely non-empty: an unparseable owner would bind into a
	// comparison against a UUID column and make the driver reject the
	// statement. Nobody holds a value under a tenant that cannot exist.
	if _, err := tenantID.UUID(); err != nil {
		return false
	}

	d := s.repo.Engine.Dialect()
	q := d.QuoteIdent

	// COUNT(*), NOT `SELECT 1 … LIMIT 1`, and the difference is a bug rather than
	// a preference. This project's Querier hands back the driver's own Row, so a
	// SELECT that matches nothing fails the Scan with the DRIVER's no-rows
	// sentinel — pgx.ErrNoRows here — which isRecordNotFound does not recognise
	// (it looks for a framework DomainError). The panic below would then fire on
	// the ORDINARY case: a definition nobody holds a value for, which is exactly
	// the narrowing that must be allowed. Every legitimate narrowing would answer
	// 500.
	//
	// An aggregate always returns exactly one row, so there is no no-rows case to
	// spell, on any engine. Any error left is a genuine failure and the panic is
	// the honest answer. It also keeps the statement plain ANSI: no LIMIT, no
	// TOP, no FETCH FIRST, nothing per-dialect.
	//
	// The cost is counting instead of stopping at the first match, on a probe
	// that runs at most twice per claim-definition update — an operator action,
	// not a hot path — and against the claim_id index the two edge migrations add
	// by hand.
	sql := "SELECT COUNT(*) FROM " + q(edgeTable) + " AS e" +
		" INNER JOIN " + q("claims") + " AS c ON c." + q("id") + " = e." + q("claim_id") +
		" WHERE e." + q("deleted_at") + " IS NULL" +
		" AND c." + q("deleted_at") + " IS NULL" +
		" AND c." + q("tenant_id") + " = " + d.Placeholder(1) +
		" AND c." + q("name") + " = " + d.Placeholder(2)

	var held int64
	if err := s.repo.Engine.Querier().
		QueryRow(s.queryContext(), sql, d.EncodeArg(tenantID), d.EncodeArg(name)).
		Scan(&held); err != nil {
		// A failed probe PANICS rather than inventing an answer, like every
		// other probe in this package. Answering "nobody holds it" would let
		// the narrowing through and strand the values this rule exists to
		// protect — the expensive direction, and invisible afterwards.
		panic("Claim: held-value probe failed on " + edgeTable)
	}
	return held > 0
}

// ClaimIsHeldByAUser reports whether any ACTIVE user_claims row references the
// ACTIVE definition identified by this tenant and name.
//
// Named for the PROBLEM, and here that took a moment's care rather than being
// automatic. The generated suite stubs the service so every probe answers
// "nothing found": this reads FALSE under that stub, meaning "nobody holds it",
// so a narrowing is allowed and the generated happy path passes on the day it is
// written. The healthy-state spelling — ClaimIsFreeOfUserValues — would read
// false too, and would then mean "somebody holds it", turning a correct spec red.
//
// ARCHIVED EDGES DO NOT COUNT, which is the direct consequence of the two edge
// collections choosing softRemove: a value somebody removed is history, and
// history must not freeze a definition's shape forever.
func (s *ClaimServiceImpl) ClaimIsHeldByAUser(tenantID domain.ID, name string) bool {
	return s.claimIsHeldBy("user_claims", tenantID, name)
}

// ClaimIsHeldByAClient reports whether any ACTIVE client_claims row references
// the ACTIVE definition identified by this tenant and name. Same contract, same
// two archive predicates, the other side of the chain.
func (s *ClaimServiceImpl) ClaimIsHeldByAClient(tenantID domain.ID, name string) bool {
	return s.claimIsHeldBy("client_claims", tenantID, name)
}
