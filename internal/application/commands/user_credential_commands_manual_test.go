// Tests for the password-reset handler.
//
// The hasher is the REAL one. Argon2id costs ~100 ms a call, so the cases below
// are deliberately few — but faking it would fake exactly what several of these
// assert: that the stored hash actually changes, that the new password verifies
// against what landed, and that a refused reset leaves the old one alone.
//
// A larger suite used to live here for a PUBLIC change-password endpoint — the
// credential barrier, the timing guard, the lockout counter surviving a rejected
// request. That endpoint was removed on 2026-08-26: it did what this one does
// without a token, on a premise this service does not have. Its tests went with
// it rather than being kept green against nothing.

package commands

import (
	"errors"
	"strings"
	"testing"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/persistence"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// ── fakes ───────────────────────────────────────────────────────────────────

type fakeStore struct {
	user    *appdomain.User
	updates []*appdomain.User
}

func (s *fakeStore) ScopedReader(_ *configuration.AppContext) domain.Reader[*appdomain.User] {
	return &fakeReader{store: s}
}

func (s *fakeStore) Scope(_ *configuration.AppContext, _ ...persistence.WriteOption[*appdomain.User]) domain.Writer {
	return &fakeWriter{store: s}
}

type fakeReader struct{ store *fakeStore }

func (r *fakeReader) FindByID(domain.ID) (*appdomain.User, error) {
	if r.store.user == nil {
		return nil, errors.New("not found")
	}
	return r.store.user, nil
}
func (r *fakeReader) New() *appdomain.User { return &appdomain.User{} }

type fakeWriter struct{ store *fakeStore }

func (w *fakeWriter) Insert(domain.Insertable) (domain.ID, error) { return domain.ID{}, nil }
func (w *fakeWriter) Update(domain.Updatable) error {
	w.store.updates = append(w.store.updates, w.store.user)
	return nil
}
func (w *fakeWriter) Archive(domain.Archivable) error     { return nil }
func (w *fakeWriter) Unarchive(domain.Unarchivable) error { return nil }
func (w *fakeWriter) Delete(domain.Deletable) error       { return nil }

// credentialService is the domain service the aggregate requires. It delegates
// the two credential facts to the real hasher and answers "nothing wrong" to
// everything else — the posture the generated stub takes, which is what lets a
// valid fixture through and makes each negative case fail for the rule it is
// testing.
type credentialService struct {
	domain.ServiceBase
	hasher appdomain.PasswordHasher
}

func (s *credentialService) EmailTaken(string, domain.ID) bool { return false }
func (s *credentialService) HashPassword(p string) string      { return s.hasher.Hash(p) }
func (s *credentialService) PasswordIsUnchanged(p, hash string) bool {
	return hash != "" && s.hasher.Matches(p, hash)
}
func (s *credentialService) TenantIsUnavailable(domain.ID) bool                   { return false }
func (s *credentialService) GroupIsUnavailableInTenant(domain.ID, domain.ID) bool { return false }
func (s *credentialService) GroupGrantsWildcard(domain.ID) bool                   { return false }
func (s *credentialService) CallerLacksAnyPermissionOfGroup(domain.ID) bool       { return false }
func (s *credentialService) RoleIsUnavailableInTenant(domain.ID, domain.ID) bool  { return false }
func (s *credentialService) RoleGrantsWildcard(domain.ID) bool                    { return false }
func (s *credentialService) CallerLacksAnyPermissionOfRole(domain.ID) bool        { return false }

// ── fixtures ────────────────────────────────────────────────────────────────

const (
	someOtherTenant = "0198f4aa-1111-7c9e-9f2a-6d3b1e77a410"
	ownTenant       = "0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"
	targetUserID    = "7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"

	oldPassword = "Str0ng!Passphrase"
	newPassword = "An0ther!Passphrase"
)

func resetFixture(t *testing.T) (*fakeStore, appdomain.PasswordHasher, *ResetPasswordHandler) {
	t.Helper()

	hasher := infra.NewArgon2idHasher()
	user := &appdomain.User{
		TenantID:     domain.NewID(ownTenant),
		Email:        vos.Email("maria@acme.com"),
		Name:         vos.PersonName{Given: "Maria", Family: "Souza Lima"},
		Status:       vos.UserStatus("active"),
		PasswordHash: hasher.Hash(oldPassword),
	}
	user.SetID(domain.NewID(targetUserID))

	store := &fakeStore{user: user}
	return store, hasher, &ResetPasswordHandler{
		Store:   store,
		Service: &credentialService{hasher: hasher},
	}
}

func testCtx() *configuration.AppContext {
	return configuration.NewAppContextWithRandomID(configuration.LangENG)
}

func resetCmd() *ResetPasswordCommand {
	cmd := &ResetPasswordCommand{Password: newPassword, PasswordConfirmation: newPassword}
	cmd.SetPathID(targetUserID)
	return cmd
}

// ── the reset ───────────────────────────────────────────────────────────────

func TestTheResetRotatesTheCredential(t *testing.T) {
	store, hasher, h := resetFixture(t)
	before := store.user.PasswordHash

	if _, err := h.Handle(testCtx(), resetCmd()); err != nil {
		t.Fatalf("a valid reset was refused: %v", err)
	}

	if store.user.PasswordHash == before {
		t.Fatal("the stored hash did not change")
	}
	if !hasher.Matches(newPassword, store.user.PasswordHash) {
		t.Fatal("the new password does not verify against the stored hash")
	}
	if strings.Contains(store.user.PasswordHash, newPassword) {
		t.Fatal("the plaintext is inside the stored hash")
	}
}

// TestTheResetForcesAChangeOnTheNextSignIn is the field that makes a reset a
// reset: the password it leaves is somebody else's choice.
func TestTheResetForcesAChangeOnTheNextSignIn(t *testing.T) {
	store, _, h := resetFixture(t)
	store.user.MustChangePassword = false

	if _, err := h.Handle(testCtx(), resetCmd()); err != nil {
		t.Fatalf("a valid reset was refused: %v", err)
	}
	if !store.user.MustChangePassword {
		t.Fatal("the reset did not force a change on the next sign-in")
	}
}

// TestTheResetStillEnforcesThePolicy — the authorization is different from an
// insert's, the password rules are not.
//
// It also covers the trap the credential rules carry: the Password value object
// is EXCLUDED from the framework's automatic pass on every update, because the
// field declares `modes: [insert]`. The policy runs here only because
// credentialRules calls IsValid directly. Replace that with the framework's
// ValidateValueObject and this test goes red — the forced list honours the same
// ignore set, which is how a five-character password was accepted once.
func TestTheResetStillEnforcesThePolicy(t *testing.T) {
	store, hasher, h := resetFixture(t)
	before := store.user.PasswordHash

	cmd := &ResetPasswordCommand{Password: "short", PasswordConfirmation: "short"}
	cmd.SetPathID(targetUserID)

	if _, err := h.Handle(testCtx(), cmd); err == nil {
		t.Fatal("a reset accepted a password the policy forbids")
	}
	if store.user.PasswordHash != before || !hasher.Matches(oldPassword, store.user.PasswordHash) {
		t.Fatal("a refused reset altered the stored credential")
	}
}

func TestTheResetComparesTheConfirmation(t *testing.T) {
	_, _, h := resetFixture(t)

	cmd := &ResetPasswordCommand{Password: newPassword, PasswordConfirmation: "An0ther!Passphras"}
	cmd.SetPathID(targetUserID)

	if _, err := h.Handle(testCtx(), cmd); err == nil {
		t.Fatal("a mismatched confirmation was accepted")
	}
}

func TestTheResetRefusesTheCurrentPassword(t *testing.T) {
	_, _, h := resetFixture(t)

	cmd := &ResetPasswordCommand{Password: oldPassword, PasswordConfirmation: oldPassword}
	cmd.SetPathID(targetUserID)

	if _, err := h.Handle(testCtx(), cmd); err == nil {
		t.Fatal("the current password was accepted as the new one")
	}
}

// ── the row scope ───────────────────────────────────────────────────────────

// TestTheResetFeedsTheRowScopeGuard covers a hole the route's permission cannot.
//
// RequirePermission("user:reset-password") answers WHO may attempt the verb and
// says nothing about WHOSE row — so the aggregate's own row-scope guard has to
// run, and it only runs if the handler feeds it the identity the way every
// generated command mapper does. Without that, a holder of the permission in one
// tenant could reset a password in another, and the read filter would hide the
// damage from the side that caused it. The build stays green either way; this is
// what does not.
func TestTheResetFeedsTheRowScopeGuard(t *testing.T) {
	store, hasher, h := resetFixture(t)

	ctx := testCtx()
	ctx.SetIdentity(&configuration.Identity{
		Subject: "some-operator",
		Claims:  map[string]any{"tenant_id": someOtherTenant},
	})

	if _, err := h.Handle(ctx, resetCmd()); err == nil {
		t.Fatal("a caller from another tenant reset a password and was not refused")
	}
	if !hasher.Matches(oldPassword, store.user.PasswordHash) {
		t.Fatal("the credential was rotated by a caller from another tenant")
	}
}

// TestASuperAdminCrossesTheRowScope is the other half: the bypass is what lets a
// platform operator support a customer, and it is asked through IsSuperAdmin
// rather than HasPermission, which panics on the wildcard the claim carries.
func TestASuperAdminCrossesTheRowScope(t *testing.T) {
	store, hasher, h := resetFixture(t)

	ctx := testCtx()
	ctx.SetIdentity(&configuration.Identity{
		Subject: "platform-operator",
		Claims: map[string]any{
			"tenant_id":   someOtherTenant,
			"permissions": []any{"*:*"},
		},
	})

	if _, err := h.Handle(ctx, resetCmd()); err != nil {
		t.Fatalf("a *:* operator was refused: %v", err)
	}
	if !hasher.Matches(newPassword, store.user.PasswordHash) {
		t.Fatal("the operator's reset did not rotate the credential")
	}
}
