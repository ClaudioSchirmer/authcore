package domain

import (
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The four rules in tenant_rules_manual.go are the only logic in this entity
// with no generated test behind it, which is exactly why they are the ones
// worth testing by hand.

// newTenant is a Tenant as it arrives on an INSERT: no primary key yet, because
// the framework mints that one and refuses an insert that carries it
// (UnableToInsertWithIDNotification). Its TenantID is empty for the same
// reason — deriving it is the rule under test.
//
// loadedTenant below is the update-path counterpart.
func newTenant() *Tenant {
	e := &Tenant{
		Name:        vos.DisplayName("Acme Comércio e Serviços Ltda"),
		Workspace:   vos.TenantWorkspace("acme-comercio"),
		Description: vos.Description("Retail operations of the Acme group in Brazil."),
		Status:      vos.TenantStatus("active"),
	}
	return e
}

// loadedTenant is a Tenant as it comes back from the database: it carries its
// primary key, and its TenantID already agrees with its Workspace.
//
// It is deliberately NOT the generated validTenant(): that fixture carries a
// hard-coded TenantID that does not derive from its handle. On the insert path
// that is invisible, because derive-tenant-id overwrites the field before
// anything reads it — but an update never re-derives, so a test of the update
// path needs a row that is internally consistent to begin with.
func loadedTenant() *Tenant {
	e := newTenant()
	e.SetID(domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"))
	e.TenantID = e.Workspace.DeriveTenantID()
	return e
}

// ── derive-tenant-id ──

// The derivation is the reason the field exists, and an insert is the only
// place it ever runs.
func TestInsertDerivesTenantIDFromWorkspace(t *testing.T) {
	e := &Tenant{
		Name:        vos.DisplayName("Acme Comércio e Serviços Ltda"),
		Workspace:   vos.TenantWorkspace("acme-comercio"),
		Description: vos.Description("Retail operations of the Acme group in Brazil."),
		Status:      vos.TenantStatus("trial"),
	}
	if !e.TenantID.IsEmpty() {
		t.Fatal("the fixture already carries a tenant id, so this would prove nothing")
	}

	if _, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable"); err != nil {
		t.Fatalf("a valid Tenant was rejected: %v (fields: %v)", err, tenantRejectedFields(err))
	}

	want := vos.TenantWorkspace("acme-comercio").DeriveTenantID()
	if e.TenantID != want {
		t.Errorf("TenantID = %q, want %q — the insert did not derive it", e.TenantID.Value(), want.Value())
	}
}

// A value the caller could never have sent must not be invented from an
// invalid handle either: the handle is refused on the same pass, so deriving
// from it would only produce a value the write never commits.
func TestInsertDoesNotDeriveFromAnEmptyWorkspace(t *testing.T) {
	e := &Tenant{
		Name:        vos.DisplayName("Acme Comércio e Serviços Ltda"),
		Description: vos.Description("Retail operations of the Acme group in Brazil."),
		Status:      vos.TenantStatus("trial"),
	}

	_, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a Tenant with no workspace was accepted")
	}
	if !tenantBlames(err, "Workspace") {
		t.Errorf("the rejection should name Workspace, it named %v", tenantRejectedFields(err))
	}
	if !e.TenantID.IsEmpty() {
		t.Errorf("a public key was derived from an empty handle: %q", e.TenantID.Value())
	}
}

// ── tenant-id-matches-workspace ──

// The rule guards a row whose two identifiers disagree — which the write path
// cannot produce, and a hand-written row or a future mapper bug can.
func TestUpdateRefusesATenantIDThatDoesNotDeriveFromItsWorkspace(t *testing.T) {
	e := loadedTenant()
	// Corrupt the row the way a hand-written INSERT would, then leave both
	// identifiers alone in the update so the immutability rules stay silent.
	e.TenantID = domain.NewID("00000000-0000-0000-0000-000000000001")

	_, err := domain.GetUpdatable(e, func(x *Tenant) error {
		x.Name = vos.DisplayName("Acme Comércio Ltda")
		return nil
	}, &stubTenantService{}, "GetUpdatable")
	if err == nil {
		t.Fatal("a row whose tenant id does not derive from its workspace was written back")
	}
	if !tenantBlames(err, "TenantID") {
		t.Errorf("the rejection should name TenantID, it named %v", tenantRejectedFields(err))
	}
}

func TestUpdateAcceptsAConsistentRow(t *testing.T) {
	e := loadedTenant()
	if _, err := domain.GetUpdatable(e, func(x *Tenant) error {
		x.Name = vos.DisplayName("Acme Comércio Ltda")
		return nil
	}, &stubTenantService{}, "GetUpdatable"); err != nil {
		t.Fatalf("a consistent row was rejected: %v (fields: %v)", err, tenantRejectedFields(err))
	}
}

// ── description-differs-from-name-and-workspace ──

func TestDescriptionMustDifferFromNameAndWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name        string
		description string
	}{
		{"the name pasted verbatim", "Acme Comércio e Serviços Ltda"},
		{"the name, case-folded and unspaced", "acmecomércioeserviçosltda"},
		{"the name with hyphens for spaces", "Acme-Comércio-e-Serviços-Ltda"},
		{"the workspace pasted verbatim", "acme-comercio"},
		{"the workspace without its hyphen", "acme comercio"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newTenant()
			e.Description = vos.Description(tc.description)

			_, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable")
			if err == nil {
				t.Fatalf("a description of %q was accepted", tc.description)
			}
			if !tenantBlames(err, "Description") {
				t.Errorf("the rejection should name Description, it named %v", tenantRejectedFields(err))
			}
		})
	}
}

// The rule is a "must differ", not a "must not contain": a description that
// legitimately opens with the company's name is ordinary and must pass.
func TestDescriptionMayMentionTheNameWithoutBeingIt(t *testing.T) {
	e := newTenant()
	e.Description = vos.Description("Acme Comércio e Serviços Ltda, retail arm of the Acme group.")

	if _, err := domain.GetInsertable(e, &stubTenantService{}, "GetInsertable"); err != nil {
		t.Fatalf("a description mentioning the name was rejected: %v (fields: %v)", err, tenantRejectedFields(err))
	}
}

// ── archive-forces-suspended ──

// Archiving is a mutation here, not a validation: it must be ACCEPTED and it
// must move the field.
func TestArchiveForcesSuspended(t *testing.T) {
	for _, from := range []vos.TenantStatus{
		vos.TenantStatusTrial,
		vos.TenantStatusActive,
		vos.TenantStatusSuspended, // the no-op case still has to be accepted
	} {
		t.Run(string(from), func(t *testing.T) {
			e := loadedTenant()
			e.Status = from

			if _, err := domain.GetArchivable(e, &stubTenantService{}, "GetArchivable"); err != nil {
				t.Fatalf("archiving a %s tenant was rejected: %v (fields: %v)", from, err, tenantRejectedFields(err))
			}
			if e.Status != vos.TenantStatusSuspended {
				t.Errorf("after archiving, Status = %q, want %q — archived+active is meant to be unrepresentable",
					e.Status, vos.TenantStatusSuspended)
			}
		})
	}
}

// It is one-way: unarchiving brings the tenant back SUSPENDED, because the
// stored value is suspended — archiving wrote it. Restoring a tenant must
// never silently resume a billable account.
func TestUnarchiveLeavesTheTenantSuspended(t *testing.T) {
	e := loadedTenant()
	e.Status = vos.TenantStatusActive

	if _, err := domain.GetArchivable(e, &stubTenantService{}, "GetArchivable"); err != nil {
		t.Fatalf("archiving was rejected: %v", err)
	}
	if _, err := domain.GetUnarchivable(e, &stubTenantService{}, "GetUnarchivable"); err != nil {
		t.Fatalf("unarchiving was rejected: %v (fields: %v)", err, tenantRejectedFields(err))
	}
	if e.Status != vos.TenantStatusSuspended {
		t.Errorf("after unarchiving, Status = %q, want %q — reactivation must be a separate, audited act",
			e.Status, vos.TenantStatusSuspended)
	}
}

// ── the status state machine (generated rule, exercised here) ──

func TestStatusTransitions(t *testing.T) {
	for _, tc := range []struct {
		from, to vos.TenantStatus
		allowed  bool
	}{
		{vos.TenantStatusTrial, vos.TenantStatusActive, true},
		{vos.TenantStatusTrial, vos.TenantStatusSuspended, true},
		{vos.TenantStatusActive, vos.TenantStatusSuspended, true},
		{vos.TenantStatusSuspended, vos.TenantStatusActive, true},
		{vos.TenantStatusActive, vos.TenantStatusActive, true},    // a no-op is always allowed
		{vos.TenantStatusActive, vos.TenantStatusTrial, false},    // a trial is a beginning
		{vos.TenantStatusSuspended, vos.TenantStatusTrial, false}, // and never a destination
	} {
		name := string(tc.from) + "->" + string(tc.to)
		t.Run(name, func(t *testing.T) {
			e := loadedTenant()
			e.Status = tc.from

			_, err := domain.GetUpdatable(e, func(x *Tenant) error {
				x.Status = tc.to
				return nil
			}, &stubTenantService{}, "GetUpdatable")

			if tc.allowed && err != nil {
				t.Fatalf("%s was rejected: %v (fields: %v)", name, err, tenantRejectedFields(err))
			}
			if !tc.allowed {
				if err == nil {
					t.Fatalf("%s was accepted", name)
				}
				if !tenantBlames(err, "Status") {
					t.Errorf("the rejection should name Status, it named %v", tenantRejectedFields(err))
				}
			}
		})
	}
}

// The workspace pre-check is the half of the uniqueness chain that lets a
// duplicate be reported WITH the other problems instead of alone afterwards.
type takenWorkspaceService struct {
	domain.ServiceBase
}

func (takenWorkspaceService) WorkspaceTaken(_ string, _ domain.ID) bool { return true }

func TestWorkspaceAlreadyTakenIsRefusedBeforeTheDatabase(t *testing.T) {
	e := newTenant()

	_, err := domain.GetInsertable(e, &takenWorkspaceService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a workspace the service reported as taken was accepted")
	}
	if !tenantBlames(err, "Workspace") {
		t.Errorf("the rejection should name Workspace, it named %v", tenantRejectedFields(err))
	}
}
