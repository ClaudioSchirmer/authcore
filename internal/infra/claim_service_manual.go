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
	"context"
	"sync"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/command/read"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

var (
	claimCompanionsMu sync.Mutex
	// Keyed by the OWNING repository — see the note in the file header. Never a
	// bare package-level singleton.
	claimTenantsByRepo = map[*ClaimRepository]*TenantRepository{}

	// Held as edgeHolder, NOT as the concrete repository, and that narrowing is
	// the point rather than tidiness. read.DirectRepository carries a writer
	// beside its reader — Insert, Update, Delete, Archive — so storing it whole
	// would leave every caller in this package one field access away from writing
	// into another aggregate's collection, outside its root, with no rules, no
	// revision guard and no audit. The probe needs one question answered; the
	// interface below is exactly that question, and the write half stays
	// unreachable from here.
	claimUserEdgesByRepo   = map[*ClaimRepository]claimEdge{}
	claimClientEdgesByRepo = map[*ClaimRepository]claimEdge{}
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
// These two probes read a COLLECTION table that belongs to ANOTHER aggregate —
// user_claims is User's, client_claims is Client's — and that is the whole reason
// they exist as hand-written facts. A criteria over `claims` cannot phrase the
// question, and one over `users` cannot either: the loader's resolver offers the
// anchor, its siblings, its shared base and its ROOT joins, and refuses a child's
// fields in as many words — "child joins are load-only … filtering the root by a
// field of a 1:N child is a pushdown a single root SELECT cannot express".
//
// The answer is to stop entering through a root. core.NewDirectSchema anchors a
// schema ON THE TABLE, and read.DirectRepository reads it with the loader's own
// vocabulary — so these two are ordinary Exists calls, with no SQL written here
// and no column named here. The schemas live beside the generated ones, in
// internal/infra/schemas/claim_edge_direct_schemas.go, with the reasoning for the
// second declaration and the test that keeps the two in step.
//
// ONE PREDICATE EACH, and it is worth saying what that replaced. While the spec
// could not filter a fact by the row id (before omnicore-gen 0.52.0) these facts
// took (TenantID, Name) and paid twice for it: a join to `claims` that existed
// only to translate the name back into the id the edge row already stores, and a
// second archive predicate on that join, because (tenant, name) is ambiguous
// between a live definition and an archived one still sharing the pair. The id is
// never ambiguous, so both are gone.

// edgeHolder is what both probes ask of their repository, and the only thing they
// ask. Naming it keeps claimEdgeHolds free of the repository's row type, so one
// body serves both tables without either side widening its reach.
type edgeHolder interface {
	Exists(ctx context.Context, q *criteria.Query) (bool, error)
}

// claimEdge is one edge table as a probe sees it: the read, and the name to blame
// when the read fails.
//
// The NAME COMES OFF THE SCHEMA rather than being typed at the call site, which
// is the same discipline the rest of this file follows for columns. A Direct
// repository exposes its reads and its joins but not its table, so the string is
// taken where the schema is still in hand — at construction, once — instead of
// being re-spelled beside every panic. A literal there would be a fourth place
// naming the table, and the only one nothing checks.
type claimEdge struct {
	holder edgeHolder
	table  string
}

// claimEdgeFor returns the edge for one table, built once per owning
// ClaimRepository and cached in the map it is handed.
//
// Built lazily and keyed by the OWNING repository, for the reason in the file
// header: a package-level singleton would build from the first engine it ever saw
// and hand that to every service constructed afterwards.
//
// The row type is the type parameter and the schema is the argument, and the
// framework cross-checks the two at construction — one schema, one row type — so
// a pair that does not belong together fails at the first call rather than
// reading the wrong table quietly.
func claimEdgeFor[T any](cache map[*ClaimRepository]claimEdge, repo *ClaimRepository, schema *core.TableSchema) claimEdge {
	claimCompanionsMu.Lock()
	defer claimCompanionsMu.Unlock()

	if edge, ok := cache[repo]; ok {
		return edge
	}
	edge := claimEdge{
		holder: read.NewDirectRepository[T](repo.Engine, schema),
		table:  schema.Table(),
	}
	cache[repo] = edge
	return edge
}

func (s *ClaimServiceImpl) userClaimEdges() claimEdge {
	return claimEdgeFor[schemas.UserClaimEdge](claimUserEdgesByRepo, s.repo, schemas.UserClaimEdgeSchema())
}

func (s *ClaimServiceImpl) clientClaimEdges() claimEdge {
	return claimEdgeFor[schemas.ClientClaimEdge](claimClientEdgesByRepo, s.repo, schemas.ClientClaimEdgeSchema())
}

// claimEdgeHolds answers whether any ACTIVE row of one edge table points at this
// claim definition.
//
// ACTIVE is the query's DEFAULT SCOPE rather than a predicate spelled here, and
// that is exactly what it should be: the schema declares archived_at, so the gate
// is the framework's. Archived edges must not count — a value somebody removed is
// history, and history must not freeze a definition's shape forever, which is the
// direct consequence of both collections choosing softRemove.
//
// The edge arrives as a THUNK, not as a value, and that is load-bearing rather
// than a style choice: an argument is evaluated before the call, so passing it
// directly would build the repository — and reach into s.repo — before the guard
// below had a chance to refuse. A probe that is not going to query should
// construct nothing, which is also what lets a zero-value service be a legitimate
// fixture for the guard's own test.
func claimEdgeHolds(ctx context.Context, edge func() claimEdge, claimID domain.ID) bool {
	// Usable, not merely non-empty. An id the driver would reject is not a
	// definition anybody can hold a value for, but it is also not a definition
	// this probe can answer ABOUT — so it refuses rather than clearing the way.
	// That is the opposite of the old (tenant, name) shape's guard, and
	// deliberately: an impossible TENANT meant "nobody holds it", while an
	// impossible SUBJECT means "no answer", and the narrowing this guards is the
	// expensive direction to get wrong.
	if _, err := claimID.UUID(); err != nil {
		return true
	}

	e := edge()
	held, err := e.holder.Exists(ctx, criteria.Where(criteria.Eq("ClaimID", claimID)))
	if err != nil {
		// A failed probe PANICS rather than inventing an answer, like every other
		// probe in this package. Answering "nobody holds it" would let the
		// narrowing through and strand the values this rule exists to protect —
		// the expensive direction, and invisible afterwards.
		panic("Claim: held-value probe failed on " + e.table)
	}
	return held
}

// ClaimIsHeldByAUser reports whether any ACTIVE user_claims row references this
// claim definition.
//
// Named for the PROBLEM, and here that took a moment's care rather than being
// automatic. The generated suite stubs the service so every probe answers
// "nothing found": this reads FALSE under that stub, meaning "nobody holds it",
// so a narrowing is allowed and the generated happy path passes on the day it is
// written. The healthy-state spelling — ClaimIsFreeOfUserValues — would read
// false too, and would then mean "somebody holds it", turning a correct spec red.
func (s *ClaimServiceImpl) ClaimIsHeldByAUser(id domain.ID) bool {
	return claimEdgeHolds(s.queryContext(), s.userClaimEdges, id)
}

func (s *ClaimServiceImpl) ClaimIsHeldByAClient(id domain.ID) bool {
	return claimEdgeHolds(s.queryContext(), s.clientClaimEdges, id)
}
