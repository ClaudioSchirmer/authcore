// Tests for the four hand-written rules in tenant_rules_manual.go.
//
// They are hand-written because the rules are: the generator declared them as
// invariants the spec language could not express, wrote a stub, and wrote no
// test. This is the only place they are proven.
//
// The harness — validTenant, stubTenantService, tenantBlames — comes from the
// generated tenant_test.go in this same package, so a fixture change lands in
// one place.

package domain

import (
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// persistedTenant is validTenant as it comes back OUT of the database: with an
// id. Update, archive and unarchive all act on a row that already exists, and
// the framework validates that id like any other field — so a fixture without
// one fails on the id before reaching the rule under test.
func persistedTenant() *Tenant {
	e := validTenant()
	e.SetID(domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"))
	e.TenantID = e.Workspace.DeriveTenantID()
	return e
}

// ── derive-tenant-id ─────────────────────────────────────────────────────────

// The public key is minted from the handle on insert, whatever the caller's
// entity happened to carry — and it CAN carry something, because the fixture
// does: the field is absent from every write DTO, so the only value that ever
// reaches the row is the one this rule writes.
func TestTenantIDIsDerivedFromWorkspaceOnInsert(t *testing.T) {
	e := validTenant()
	e.Workspace = vos.TenantWorkspace("acme-comercio")
	e.TenantID = domain.NewID("00000000-0000-0000-0000-000000000001") // a value nobody should keep

	if _, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable"); err != nil {
		t.Fatalf("a valid Tenant was rejected: %v (fields: %v)", err, tenantRejectedFields(err))
	}

	want := vos.TenantWorkspace("acme-comercio").DeriveTenantID()
	if e.TenantID != want {
		t.Errorf("TenantID = %s, want the derived %s", e.TenantID.Value(), want.Value())
	}
}

// The derivation is a pure function of the handle, so running the rules twice
// cannot produce a second answer. That is what makes it safe to do in a
// validation pass the framework may run more than once.
func TestTenantIDDerivationIsIdempotent(t *testing.T) {
	e := validTenant()

	if _, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable"); err != nil {
		t.Fatalf("first pass rejected a valid Tenant: %v", err)
	}
	first := e.TenantID

	if _, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable"); err != nil {
		t.Fatalf("second pass rejected the same Tenant: %v (fields: %v)", err, tenantRejectedFields(err))
	}
	if e.TenantID != first {
		t.Errorf("a second pass moved TenantID from %s to %s", first.Value(), e.TenantID.Value())
	}
}

// Two different handles must never derive one public key — the whole isolation
// model rests on this being injective in practice.
func TestTenantIDDerivationDistinguishesHandles(t *testing.T) {
	first, second := validTenant(), validTenant()
	first.Workspace = vos.TenantWorkspace("acme-comercio")
	second.Workspace = vos.TenantWorkspace("acme-servicos")

	for _, e := range []*Tenant{first, second} {
		if _, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable"); err != nil {
			t.Fatalf("a valid Tenant was rejected: %v (fields: %v)", err, tenantRejectedFields(err))
		}
	}
	if first.TenantID == second.TenantID {
		t.Errorf("two handles derived the same public key: %s", first.TenantID.Value())
	}
}

// ── tenant-id-matches-workspace ──────────────────────────────────────────────

// On UPDATE the derivation does not re-run — the handle is immutable, so there
// is nothing to re-derive — which is what leaves this rule something to catch:
// a row whose stored pair disagrees, from a hand-written INSERT or a bug in the
// derivation itself.
//
// It is driven by moving BOTH fields together, so the two immutability rules
// stand down and this rule is the only one left to fire.
func TestTenantIDMismatchIsRefusedOnUpdate(t *testing.T) {
	e := persistedTenant()
	// Put the entity in the state a hand-written row would leave it in: the
	// handle and the public key disagree, and both are "unchanged" by the write.
	e.Workspace = vos.TenantWorkspace("acme-comercio")
	e.TenantID = domain.NewID("00000000-0000-0000-0000-000000000009")

	_, err := domain.GetUpdatable(e, func(x *Tenant) error {
		x.Name = vos.DisplayName("Acme Comércio Renomeada")
		return nil
	}, &stubTenantService{}, "GetUpdatable")

	if err == nil {
		t.Fatal("a tenant whose public key does not match its handle was accepted")
	}
	if !tenantBlames(err, "TenantID") {
		t.Errorf("the rejection should name TenantID, it named %v", tenantRejectedFields(err))
	}
}

// The matching pair must pass, or the rule above would be passing for the
// wrong reason on every update.
func TestTenantIDMatchingItsWorkspaceIsAcceptedOnUpdate(t *testing.T) {
	e := persistedTenant()
	e.Workspace = vos.TenantWorkspace("acme-comercio")
	e.TenantID = vos.TenantWorkspace("acme-comercio").DeriveTenantID()

	if _, err := domain.GetUpdatable(e, func(x *Tenant) error {
		x.Name = vos.DisplayName("Acme Comércio Renomeada")
		return nil
	}, &stubTenantService{}, "GetUpdatable"); err != nil {
		t.Fatalf("a consistent tenant was rejected on update: %v (fields: %v)", err, tenantRejectedFields(err))
	}
}

// ── description-differs-from-name-and-workspace ──────────────────────────────

// The pasted-name description, in the shapes it actually arrives in: the name
// verbatim, the name in another case, and the handle with its hyphens.
func TestDescriptionRepeatingTheNameOrHandleIsRefused(t *testing.T) {
	const name = "Acme Comercio e Servicos"
	cases := map[string]string{
		name:                        "the name verbatim",
		"ACME  COMERCIO E SERVICOS": "the name in another case, double-spaced",
		"acme-comercio-e-servicos":  "the name as a hyphenated handle",
	}
	for description, why := range cases {
		e := validTenant()
		e.Name = vos.DisplayName(name)
		e.Description = vos.Description(description)

		_, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable")
		if err == nil {
			t.Errorf("a description repeating %s was accepted: %q", why, description)
			continue
		}
		if !tenantBlames(err, "Description") {
			t.Errorf("for %q the rejection should name Description, it named %v", description, tenantRejectedFields(err))
		}
	}
}

// The handle itself, pasted as the description, is the other half of the rule.
func TestDescriptionRepeatingTheWorkspaceIsRefused(t *testing.T) {
	e := validTenant()
	e.Workspace = vos.TenantWorkspace("acme-comercio-servicos")
	e.Description = vos.Description("Acme Comercio Servicos")

	_, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a description repeating the workspace handle was accepted")
	}
	if !tenantBlames(err, "Description") {
		t.Errorf("the rejection should name Description, it named %v", tenantRejectedFields(err))
	}
}

// A description that merely RESEMBLES the name is legitimate — the rule is
// about a paste, not about similarity, and it must not start refusing real
// prose that happens to open with the company's name.
func TestDescriptionMerelyContainingTheNameIsAccepted(t *testing.T) {
	e := validTenant()
	e.Name = vos.DisplayName("Acme Comercio")
	e.Description = vos.Description("Acme Comercio runs the group's retail operations in Brazil.")

	if _, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable"); err != nil {
		t.Fatalf("a legitimate description was rejected: %v (fields: %v)", err, tenantRejectedFields(err))
	}
}

// The comparison is deliberately NOT accent-folding: folding accents would
// start refusing descriptions that differ from the name only by an accent,
// which is real prose rather than a paste.
func TestDescriptionComparisonDoesNotFoldAccents(t *testing.T) {
	e := validTenant()
	// Both sides are past the description's 15-rune floor and differ ONLY by
	// their accents, so the paste rule is the only thing under test here.
	e.Name = vos.DisplayName("Acme Comercio e Servicos")
	e.Description = vos.Description("Acme Comércio e Serviços")

	if _, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable"); err != nil {
		t.Fatalf("an accented near-match was rejected as a paste: %v (fields: %v)", err, tenantRejectedFields(err))
	}
}

func TestNormalizeForComparisonCollapsesCaseWhitespaceAndHyphens(t *testing.T) {
	cases := map[string]string{
		"Acme Comercio":     "acme comercio",
		"acme-comercio":     "acme comercio",
		"  ACME   COMERCIO": "acme comercio",
		"acme--comercio":    "acme comercio",
		"":                  "",
		"-":                 "",
	}
	for in, want := range cases {
		if got := normalizeForComparison(in); got != want {
			t.Errorf("normalizeForComparison(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── archive-forces-suspended ─────────────────────────────────────────────────

// Archiving is a MUTATION here, not a validation: the tenant comes out
// suspended whatever it was before, so "archived and commercially active"
// becomes an unrepresentable state.
func TestArchivingForcesTheTenantSuspended(t *testing.T) {
	for _, from := range []vos.TenantStatus{
		vos.TenantStatusTrial,
		vos.TenantStatusActive,
		vos.TenantStatusSuspended,
	} {
		e := persistedTenant()
		e.Status = from

		if _, err := domain.GetArchivable(e, &stubTenantService{}, "GetArchivable"); err != nil {
			t.Fatalf("archiving a %s tenant was rejected: %v (fields: %v)", from.Value(), err, tenantRejectedFields(err))
		}
		if e.Status != vos.TenantStatusSuspended {
			t.Errorf("archiving a %s tenant left it %s, want suspended", from.Value(), e.Status.Value())
		}
	}
}

// It is ONE-WAY, and the consequence is deliberate: unarchiving does NOT
// restore the previous commercial state, because the stored value IS suspended
// — archiving wrote it. Restoring a tenant must never silently resume a
// billable, functioning account.
func TestUnarchivingLeavesTheTenantSuspended(t *testing.T) {
	e := persistedTenant()
	e.Status = vos.TenantStatusActive

	if _, err := domain.GetArchivable(e, &stubTenantService{}, "GetArchivable"); err != nil {
		t.Fatalf("archiving was rejected: %v", err)
	}
	if _, err := domain.GetUnarchivable(e, &stubTenantService{}, "GetUnarchivable"); err != nil {
		t.Fatalf("unarchiving was rejected: %v (fields: %v)", err, tenantRejectedFields(err))
	}
	if e.Status != vos.TenantStatusSuspended {
		t.Errorf("unarchiving resumed the tenant as %s; it must come back suspended", e.Status.Value())
	}
}

// The status transition rule is IfUpdate and the archive mutation is IfArchive,
// so the two modes never run together and the forced move to suspended can
// never be refused as an illegal transition — including from trial, which the
// update path allows but which is worth proving cannot regress.
func TestArchivingATrialTenantIsNotBlockedByTheTransitionRule(t *testing.T) {
	e := persistedTenant()
	e.Status = vos.TenantStatusTrial

	if _, err := domain.GetArchivable(e, &stubTenantService{}, "GetArchivable"); err != nil {
		t.Fatalf("archiving a trial tenant was refused: %v (fields: %v)", err, tenantRejectedFields(err))
	}
	if e.Status != vos.TenantStatusSuspended {
		t.Errorf("a trial tenant archived as %s, want suspended", e.Status.Value())
	}
}

// ── the declarative rules the generated suite leaves unexercised ─────────────
//
// These two are the generator's own output, not hand-written rules. They are
// tested here because the generated suite does not reach them: the status
// machine needs a previous value, and the uniqueness answer needs a service
// that says yes — and the generated stub is deliberately the one that says no
// to everything, so the happy path stays green.

// takenWorkspaceService is the stub's opposite: it reports the handle as
// already held, which is the only way to reach the conflict branch.
type takenWorkspaceService struct {
	domain.ServiceBase
}

func (takenWorkspaceService) WorkspaceTaken(_ string, _ domain.ID) bool { return true }

// A handle another tenant already holds is refused as a conflict, and it is
// reported TOGETHER with the other validation problems rather than alone after
// them — which is the whole reason the pre-check exists beside the database
// index.
func TestWorkspaceAlreadyHeldIsRefused(t *testing.T) {
	e := validTenant()

	_, err := domain.GetInsertable(e, &takenWorkspaceService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a workspace another tenant already holds was accepted")
	}
	if !tenantBlames(err, "Workspace") {
		t.Errorf("the rejection should name Workspace, it named %v", tenantRejectedFields(err))
	}
}

// The commercial lifecycle: every move the spec allows, and the ones it does
// not. The refusals matter most — a trial is a beginning, and no tenant returns
// to it, which is what stops a paying customer being quietly reset to a free
// plan.
func TestTenantStatusTransitions(t *testing.T) {
	for _, tc := range []struct {
		from, to vos.TenantStatus
		allowed  bool
	}{
		{vos.TenantStatusTrial, vos.TenantStatusActive, true},
		{vos.TenantStatusTrial, vos.TenantStatusSuspended, true},
		{vos.TenantStatusActive, vos.TenantStatusSuspended, true},
		{vos.TenantStatusSuspended, vos.TenantStatusActive, true},
		// A no-op is always allowed: an unrelated edit carries the status along.
		{vos.TenantStatusActive, vos.TenantStatusActive, true},
		{vos.TenantStatusTrial, vos.TenantStatusTrial, true},
		// No return to trial, from either side.
		{vos.TenantStatusActive, vos.TenantStatusTrial, false},
		{vos.TenantStatusSuspended, vos.TenantStatusTrial, false},
	} {
		e := persistedTenant()
		e.Status = tc.from

		target := tc.to
		_, err := domain.GetUpdatable(e, func(x *Tenant) error {
			x.Status = target
			return nil
		}, &stubTenantService{}, "GetUpdatable")

		switch {
		case tc.allowed && err != nil:
			t.Errorf("%s → %s was refused: %v (fields: %v)",
				tc.from.Value(), tc.to.Value(), err, tenantRejectedFields(err))
		case !tc.allowed && err == nil:
			t.Errorf("%s → %s was accepted; a tenant must never return to trial",
				tc.from.Value(), tc.to.Value())
		case !tc.allowed && !tenantBlames(err, "Status"):
			t.Errorf("%s → %s was refused without naming Status, it named %v",
				tc.from.Value(), tc.to.Value(), tenantRejectedFields(err))
		}
	}
}
