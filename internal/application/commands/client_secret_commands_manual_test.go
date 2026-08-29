// Tests for the secret-rotation handler.
//
// The hasher is the REAL one. SHA-256 costs nothing, so unlike the password
// suite next door there is no reason to be frugal — and faking it would fake
// exactly what several of these assert: that the stored hash actually changes,
// that the returned secret verifies against what landed, and that the retiring
// slot holds the hash that was there before.

package commands

import (
	"errors"
	"strings"
	"testing"
	"time"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/persistence"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// ── fakes ───────────────────────────────────────────────────────────────────

type fakeClientStore struct {
	client  *appdomain.Client
	updates int
}

func (s *fakeClientStore) ScopedReader(_ *configuration.AppContext) domain.Reader[*appdomain.Client] {
	return &fakeClientReader{store: s}
}

func (s *fakeClientStore) Scope(_ *configuration.AppContext, _ ...persistence.WriteOption[*appdomain.Client]) domain.Writer {
	return &fakeClientWriter{store: s}
}

type fakeClientReader struct{ store *fakeClientStore }

func (r *fakeClientReader) FindByID(domain.ID) (*appdomain.Client, error) {
	if r.store.client == nil {
		return nil, errors.New("not found")
	}
	return r.store.client, nil
}
func (r *fakeClientReader) New() *appdomain.Client { return &appdomain.Client{} }

type fakeClientWriter struct{ store *fakeClientStore }

func (w *fakeClientWriter) Insert(domain.Insertable) (domain.ID, error) { return domain.ID{}, nil }
func (w *fakeClientWriter) Update(domain.Updatable) error               { w.store.updates++; return nil }
func (w *fakeClientWriter) Archive(domain.Archivable) error             { return nil }
func (w *fakeClientWriter) Unarchive(domain.Unarchivable) error         { return nil }
func (w *fakeClientWriter) Delete(domain.Deletable) error               { return nil }

// clientSecretService is the domain service the aggregate requires. It delegates
// the hash to the real adapter and answers "nothing wrong" to everything else —
// the posture the generated stub takes.
type clientSecretService struct {
	domain.ServiceBase
	hasher *infra.SHA256SecretHasher
}

func (s *clientSecretService) NameTaken(domain.ID, string, domain.ID) bool          { return false }
func (s *clientSecretService) HashSecret(secret string) string                      { return s.hasher.Hash(secret) }
func (s *clientSecretService) TenantIsUnavailable(domain.ID) bool                   { return false }
func (s *clientSecretService) RoleIsUnavailableInTenant(domain.ID, domain.ID) bool  { return false }
func (s *clientSecretService) RoleGrantsWildcard(domain.ID) bool                    { return false }
func (s *clientSecretService) CallerLacksAnyPermissionOfRole(domain.ID) bool        { return false }
func (s *clientSecretService) ClaimIsUnavailableInTenant(domain.ID, domain.ID) bool { return false }
func (s *clientSecretService) ClaimDoesNotApplyToClient(domain.ID) bool             { return false }
func (s *clientSecretService) ClaimValueDoesNotMatchValueType(domain.ID, string) bool {
	return false
}

// ── fixtures ────────────────────────────────────────────────────────────────

const (
	clientTenant   = "0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"
	rotatedClient  = "7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"
	standingSecret = "acs_theSecretThisClientIsUsingRightNowAAAAAAA"
)

func rotateFixture(t *testing.T) (*fakeClientStore, *infra.SHA256SecretHasher, *RotateClientSecretHandler) {
	t.Helper()

	hasher := infra.NewSHA256SecretHasher()
	client := &appdomain.Client{
		TenantID:        domain.NewID(clientTenant),
		Name:            vos.DisplayName("Billing integration"),
		Description:     vos.Description("Posts invoices from the billing system into the ledger."),
		Status:          vos.ClientStatusActive,
		SecretHash:      hasher.Hash(standingSecret),
		SecretChangedAt: time.Now().UTC().Add(-90 * 24 * time.Hour),
	}
	client.SetID(domain.NewID(rotatedClient))

	store := &fakeClientStore{client: client}
	return store, hasher, &RotateClientSecretHandler{
		Store:   store,
		Service: &clientSecretService{hasher: hasher},
	}
}

func rotateCmd(grace *int) *RotateClientSecretCommand {
	cmd := &RotateClientSecretCommand{GracePeriodSeconds: grace}
	cmd.SetPathID(rotatedClient)
	return cmd
}

func intOf(v int) *int { return &v }

// ── the rotation ────────────────────────────────────────────────────────────

func TestRotationMintsANewSecretAndReturnsIt(t *testing.T) {
	store, hasher, h := rotateFixture(t)
	before := store.client.SecretHash

	res, err := h.Handle(testCtx(), rotateCmd(nil))
	if err != nil {
		t.Fatalf("a valid rotation was refused: %v", err)
	}

	if res.Secret == "" {
		t.Fatal("the response carries no secret; nothing else can ever show it")
	}
	if !strings.HasPrefix(res.Secret, "acs_") {
		t.Fatalf("the secret %q carries no acs_ prefix", res.Secret)
	}
	if store.client.SecretHash == before {
		t.Fatal("the stored hash did not change")
	}
	if !hasher.Matches(res.Secret, store.client.SecretHash) {
		t.Fatal("the returned secret does not verify against the stored hash")
	}
	if strings.Contains(store.client.SecretHash, res.Secret) {
		t.Fatal("the plaintext is inside the stored hash")
	}
	if store.updates != 1 {
		t.Fatalf("the row was written %d times, want 1", store.updates)
	}
}

// TestAnOmittedWindowGetsTheDefault — absent is not zero, and this is the half
// of that contract the handler owns.
func TestAnOmittedWindowGetsTheDefault(t *testing.T) {
	store, hasher, h := rotateFixture(t)

	res, err := h.Handle(testCtx(), rotateCmd(nil))
	if err != nil {
		t.Fatalf("a valid rotation was refused: %v", err)
	}
	if res.PreviousSecretExpiresAt == nil {
		t.Fatal("an omitted window retired the old secret immediately; absent must mean the default")
	}
	want := res.SecretChangedAt.Add(appdomain.ClientSecretGraceDefaultSeconds * time.Second)
	if !res.PreviousSecretExpiresAt.Equal(want) {
		t.Fatalf("the window closes at %v, want %v", res.PreviousSecretExpiresAt, want)
	}
	// The retiring slot must hold the hash that was standing, or the overlap is
	// a swap wearing an overlap's name.
	if store.client.PreviousSecretHash == nil || !hasher.Matches(standingSecret, *store.client.PreviousSecretHash) {
		t.Fatal("the retiring slot does not hold the secret that was in use")
	}
}

// TestAZeroWindowClearsTheRetiringSlot is the leaked-credential case, and the
// assertion that matters is that both columns are NULL rather than stamped in
// the past: a stamped deadline says a rotation is in flight when none is.
func TestAZeroWindowClearsTheRetiringSlot(t *testing.T) {
	store, _, h := rotateFixture(t)

	res, err := h.Handle(testCtx(), rotateCmd(intOf(0)))
	if err != nil {
		t.Fatalf("a zero-window rotation was refused: %v", err)
	}
	if res.PreviousSecretExpiresAt != nil {
		t.Fatalf("a zero window left an expiry of %v", res.PreviousSecretExpiresAt)
	}
	if store.client.PreviousSecretHash != nil || store.client.PreviousSecretExpiresAt != nil {
		t.Fatal("a zero window stamped a retiring secret instead of clearing both columns")
	}
}

func TestACallerSuppliedWindowIsHonoured(t *testing.T) {
	_, _, h := rotateFixture(t)

	res, err := h.Handle(testCtx(), rotateCmd(intOf(3600)))
	if err != nil {
		t.Fatalf("a valid rotation was refused: %v", err)
	}
	if res.PreviousSecretExpiresAt == nil {
		t.Fatal("an hour-long window produced no expiry")
	}
	if got := res.PreviousSecretExpiresAt.Sub(res.SecretChangedAt); got != time.Hour {
		t.Fatalf("the window is %v, want 1h", got)
	}
}

// TestAWindowAboveTheCeilingIsRefusedAndChangesNothing — refused rather than
// clamped, and the credential the caller is using must survive the refusal.
func TestAWindowAboveTheCeilingIsRefusedAndChangesNothing(t *testing.T) {
	store, hasher, h := rotateFixture(t)
	before := store.client.SecretHash

	if _, err := h.Handle(testCtx(), rotateCmd(intOf(appdomain.ClientSecretGraceMaxSeconds+1))); err == nil {
		t.Fatal("a window above the ceiling was accepted")
	}
	if store.client.SecretHash != before {
		t.Fatal("a refused rotation replaced the credential anyway")
	}
	if !hasher.Matches(standingSecret, store.client.SecretHash) {
		t.Fatal("the standing secret stopped working after a refused rotation")
	}
	if store.updates != 0 {
		t.Fatalf("a refused rotation wrote the row %d times", store.updates)
	}
}

func TestANegativeWindowIsRefused(t *testing.T) {
	_, _, h := rotateFixture(t)
	if _, err := h.Handle(testCtx(), rotateCmd(intOf(-1))); err == nil {
		t.Fatal("a negative window was accepted")
	}
}

// TestASuspendedClientCannotRotate — a client somebody switched off deliberately
// must not be handed a fresh credential; reactivating it is an ordinary PATCH
// and an ordinary permission.
func TestASuspendedClientCannotRotate(t *testing.T) {
	store, hasher, h := rotateFixture(t)
	store.client.Status = vos.ClientStatusSuspended
	before := store.client.SecretHash

	if _, err := h.Handle(testCtx(), rotateCmd(nil)); err == nil {
		t.Fatal("a suspended client was handed a new secret")
	}
	if store.client.SecretHash != before {
		t.Fatal("a refused rotation replaced the credential anyway")
	}
	if !hasher.Matches(standingSecret, store.client.SecretHash) {
		t.Fatal("the standing secret stopped working after a refused rotation")
	}
}

func TestRotatingAMissingClientAnswersTheReadsError(t *testing.T) {
	store, _, h := rotateFixture(t)
	store.client = nil

	if _, err := h.Handle(testCtx(), rotateCmd(nil)); err == nil {
		t.Fatal("rotating a client that is not there succeeded")
	}
}

// TestTheRotationCarriesTheIdentityForward is the guard that is invisible in a
// green build: without it, the aggregate's row rules stand down because each
// asks whether an identity was PRESENT before comparing anything — and a holder
// of client:rotate-secret in one tenant could mint a credential in another.
func TestTheRotationCarriesTheIdentityForward(t *testing.T) {
	store, _, h := rotateFixture(t)

	if _, err := h.Handle(testCtx(), rotateCmd(nil)); err != nil {
		t.Fatalf("a valid rotation was refused: %v", err)
	}
	// testCtx carries no identity, so the flag must be FALSE — proving the
	// mapper reads the context rather than hard-coding it. The tenant guard's
	// own behaviour under a present identity is covered in the domain suite.
	if store.client.RequestingIdentityPresent {
		t.Fatal("the mapper claimed an identity where the context had none")
	}
}

// ── the identity feed ───────────────────────────────────────────────────────

// authenticatedCtx builds a context carrying a real identity, which is the only
// way to reach the branch feedClientRowScope exists for.
func authenticatedCtx(subject, tenant, kind string, superAdmin bool) *configuration.AppContext {
	ctx := testCtx()
	claims := map[string]any{"tenant_id": tenant}
	if kind != "" {
		claims["identity_kind"] = kind
	}
	if superAdmin {
		claims["permissions"] = []any{"*:*"}
	}
	ctx.SetIdentity(&configuration.Identity{Subject: subject, Claims: claims})
	return ctx
}

// TestTheRotationFeedsEveryRowScopeField is the guard whose absence is invisible
// in a green build: each of these is what a row rule reads BEFORE it compares
// anything, so a mapper that skipped them would leave every one standing down —
// and a holder of client:rotate-secret in one tenant could mint a credential in
// another.
func TestTheRotationFeedsEveryRowScopeField(t *testing.T) {
	store, _, h := rotateFixture(t)
	ctx := authenticatedCtx(rotatedClient, clientTenant, "client", false)

	if _, err := h.Handle(ctx, rotateCmd(nil)); err != nil {
		t.Fatalf("a valid rotation was refused: %v", err)
	}

	c := store.client
	if !c.RequestingIdentityPresent {
		t.Fatal("the presence flag was not fed; every row guard reads it first")
	}
	if c.RequestingTenant != clientTenant {
		t.Fatalf("the caller's tenant was fed as %q, want %q", c.RequestingTenant, clientTenant)
	}
	// Identity.Subject and not Claims["sub"] — the framework builds identities
	// that set Subject and carry no sub claim at all.
	if c.RequestingClientID != rotatedClient {
		t.Fatalf("the subject was fed as %q, want %q", c.RequestingClientID, rotatedClient)
	}
	if c.RequestingIdentityKind != "client" {
		t.Fatalf("the subject kind was fed as %q, want %q", c.RequestingIdentityKind, "client")
	}
	if c.RequestingMayCrossScope {
		t.Fatal("an ordinary caller was fed as crossing the row scope")
	}
}

// TestASuperAdminIsFedAsCrossingTheScope — asked through IsSuperAdmin and never
// through HasPermission, which panics on the *:* the claim carries.
func TestASuperAdminIsFedAsCrossingTheScope(t *testing.T) {
	store, _, h := rotateFixture(t)
	ctx := authenticatedCtx("some-operator", "another-tenant", "user", true)

	if _, err := h.Handle(ctx, rotateCmd(nil)); err != nil {
		t.Fatalf("a valid rotation was refused: %v", err)
	}
	if !store.client.RequestingMayCrossScope {
		t.Fatal("a *:* operator was not fed as crossing the row scope")
	}
}

// TestAnAbsentKindClaimFeedsEmpty is the fail-open direction, stated as a test
// so a later change cannot make it fail closed by accident: nothing mints
// identity_kind yet, and every caller must keep working until it does.
func TestAnAbsentKindClaimFeedsEmpty(t *testing.T) {
	store, _, h := rotateFixture(t)
	ctx := authenticatedCtx(rotatedClient, clientTenant, "", false)

	if _, err := h.Handle(ctx, rotateCmd(nil)); err != nil {
		t.Fatalf("a valid rotation was refused: %v", err)
	}
	if store.client.RequestingIdentityKind != "" {
		t.Fatalf("an absent claim was fed as %q", store.client.RequestingIdentityKind)
	}
}

// TestARotationWithNoStandingSecretRetiresNothing — the branch a first-ever
// rotation over an empty hash takes. There is nothing to retire, so both columns
// stay clear rather than holding an empty string with a deadline on it.
func TestARotationWithNoStandingSecretRetiresNothing(t *testing.T) {
	store, _, h := rotateFixture(t)
	store.client.SecretHash = ""

	res, err := h.Handle(testCtx(), rotateCmd(intOf(3600)))
	if err != nil {
		t.Fatalf("a valid rotation was refused: %v", err)
	}
	if res.PreviousSecretExpiresAt != nil {
		t.Fatal("a rotation with nothing standing stamped a retiring deadline")
	}
	if store.client.PreviousSecretHash != nil {
		t.Fatal("an empty hash was moved into the retiring slot")
	}
}

// TestTheInsertResultCarriesTheMintedSecret guards the hop the rotation does not
// have to make.
//
// The create mints a credential too — `mintCredential` under the insert gate —
// and the plaintext lives on a runtime field that no column, payload or audit
// event holds. `FromEntity` reading it off the entity is therefore the ONLY
// thing standing between a minted secret and a secret that dies with the
// request: drop that one line and the row still gets its hash, the write still
// answers 201, and the client it created can authenticate nobody.
//
// The value is a sentinel rather than a real mint. What the rules put there is
// proven next door in the domain suite; what is unproven here is that whatever
// they put there travels.
func TestTheInsertResultCarriesTheMintedSecret(t *testing.T) {
	ctx := &configuration.AppContext{}
	c := &InsertClientCommand{
		Name:        "Billing integration",
		Description: "Posts invoices from the billing system into the ledger.",
		Status:      "active",
		TenantID:    func() *domain.ID { v := domain.ID(domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410")); return &v }(),
	}
	e, err := c.ToEntity(ctx)
	if err != nil {
		t.Fatalf("ToEntity: %v", err)
	}
	e.SetID(domain.NewRandomID())

	// Stand where the rules stand: the plaintext on the entity, the hash on the
	// row, and the two deliberately different.
	e.Secret = "acs_sentinel_plaintext"
	e.SecretHash = "sha256:acs_sentinel_plaintext"

	res, err := c.FromEntity(ctx, e)
	if err != nil {
		t.Fatalf("FromEntity: %v", err)
	}
	if res.Secret != "acs_sentinel_plaintext" {
		t.Fatalf("the minted secret reached the result as %q; a create that answers without it leaves a client nobody can authenticate as", res.Secret)
	}
}
