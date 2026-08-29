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
	"fmt"
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
	hasher *infra.Argon2idHasher
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
func (s *credentialService) ClaimIsUnavailableInTenant(domain.ID, domain.ID) bool { return false }
func (s *credentialService) ClaimDoesNotApplyToUser(domain.ID) bool               { return false }
func (s *credentialService) ClaimValueDoesNotMatchValueType(domain.ID, string) bool {
	return false
}

// ── fixtures ────────────────────────────────────────────────────────────────

const (
	someOtherTenant = "0198f4aa-1111-7c9e-9f2a-6d3b1e77a410"
	ownTenant       = "0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"
	targetUserID    = "7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"

	oldPassword = "Str0ng!Passphrase"
	newPassword = "An0ther!Passphrase"
)

func resetFixture(t *testing.T) (*fakeStore, *infra.Argon2idHasher, *ResetPasswordHandler) {
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

// ── the change ──────────────────────────────────────────────────────────────

// credentialRefusals names the notifications a refusal raised, by type.
//
// Asserting on the TYPE and not on the field is what tells a failing test which
// rule actually refused: "an error came back" is satisfied by every rule in the
// aggregate, and several of them blame the same field. The field itself travels
// in Path rather than in FieldName for a rule that names it positionally, which
// is what these rules do.
func credentialRefusals(err error) []string {
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		return nil
	}
	var out []string
	for _, ctx := range carrier.NotificationContexts() {
		for _, msg := range ctx.Messages() {
			out = append(out, fmt.Sprintf("%T", msg.Notification))
		}
	}
	return out
}

// raised reports whether the refusal carries the given notification type, named
// the way it is declared in the domain package.
func raised(err error, notification string) bool {
	want := "domain." + notification
	for _, got := range credentialRefusals(err) {
		if got == want {
			return true
		}
	}
	return false
}

const someOperatorID = "2c9e1f44-8a7b-4d61-b0c3-5f1e7a9d3b28"

func changeFixture(t *testing.T) (*fakeStore, *infra.Argon2idHasher, *ChangePasswordHandler) {
	t.Helper()

	hasher := infra.NewArgon2idHasher()
	user := &appdomain.User{
		TenantID:           domain.NewID(ownTenant),
		Email:              vos.Email("maria@acme.com"),
		Name:               vos.PersonName{Given: "Maria", Family: "Souza Lima"},
		Status:             vos.UserStatus("active"),
		PasswordHash:       hasher.Hash(oldPassword),
		MustChangePassword: true,
	}
	user.SetID(domain.NewID(targetUserID))

	store := &fakeStore{user: user}
	return store, hasher, &ChangePasswordHandler{
		Store:   store,
		Service: &credentialService{hasher: hasher},
	}
}

// selfCtx is the caller acting on their OWN row: the subject on the token is
// the id the path carries.
func selfCtx() *configuration.AppContext {
	ctx := testCtx()
	ctx.SetIdentity(&configuration.Identity{
		Subject: targetUserID,
		Claims:  map[string]any{"tenant_id": ownTenant},
	})
	return ctx
}

// operatorCtx is a caller in the SAME tenant acting on somebody else's row —
// so the tenant guard stands down and the only thing left to refuse is the row
// decision each operation makes.
func operatorCtx() *configuration.AppContext {
	ctx := testCtx()
	ctx.SetIdentity(&configuration.Identity{
		Subject: someOperatorID,
		Claims:  map[string]any{"tenant_id": ownTenant},
	})
	return ctx
}

func changeCmd(current, next string) *ChangePasswordCommand {
	cmd := &ChangePasswordCommand{
		CurrentPassword:      current,
		Password:             next,
		PasswordConfirmation: next,
	}
	cmd.SetPathID(targetUserID)
	return cmd
}

func TestTheChangeRotatesTheCredentialAndClearsTheFlag(t *testing.T) {
	store, hasher, h := changeFixture(t)

	if _, err := h.Handle(selfCtx(), changeCmd(oldPassword, newPassword)); err != nil {
		t.Fatalf("the owner was refused their own change: %v", err)
	}
	if !hasher.Matches(newPassword, store.user.PasswordHash) {
		t.Fatal("the change did not rotate the credential")
	}
	// The password is the caller's own choice, so there is nothing for the next
	// sign-in to rotate. This is the ONLY operation that clears the flag.
	if store.user.MustChangePassword {
		t.Fatal("the change left the must-change flag standing on a password the caller chose")
	}
}

// TestTheChangeRefusesARowThatIsNotTheCallers is the row decision the route's
// permission cannot make. The caller here is in the right tenant and holds the
// door, so the tenant guard says nothing — the refusal has to come from the
// aggregate comparing the id on the token with the id on the row.
func TestTheChangeRefusesARowThatIsNotTheCallers(t *testing.T) {
	store, hasher, h := changeFixture(t)

	_, err := h.Handle(operatorCtx(), changeCmd(oldPassword, newPassword))
	if err == nil {
		t.Fatal("a caller changed a password on a row that was not theirs")
	}
	if !raised(err, "PasswordChangeRequiresSelfNotification") {
		t.Fatalf("the refusal did not come from the row decision: %v", credentialRefusals(err))
	}
	if !hasher.Matches(oldPassword, store.user.PasswordHash) {
		t.Fatal("a refused change rotated the credential anyway")
	}
}

func TestTheChangeRefusesAWrongCurrentPassword(t *testing.T) {
	store, hasher, h := changeFixture(t)

	_, err := h.Handle(selfCtx(), changeCmd("N0tTheOne!Really", newPassword))
	if err == nil {
		t.Fatal("a change went through without the current password")
	}
	if !raised(err, "InvalidCurrentPasswordNotification") {
		t.Fatalf("the refusal did not come from the current-password proof: %v", credentialRefusals(err))
	}
	if !hasher.Matches(oldPassword, store.user.PasswordHash) {
		t.Fatal("a refused change rotated the credential anyway")
	}
}

// TestTheChangeRefusesAnEmptyCurrentPassword covers the short circuit: an empty
// value is answered without spending ~100 ms of Argon2id, and says no more than
// the wrong-password case does.
func TestTheChangeRefusesAnEmptyCurrentPassword(t *testing.T) {
	_, _, h := changeFixture(t)

	_, err := h.Handle(selfCtx(), changeCmd("", newPassword))
	if err == nil {
		t.Fatal("a change went through with an empty current password")
	}
	if !raised(err, "InvalidCurrentPasswordNotification") {
		t.Fatalf("the refusal did not come from the current-password proof: %v", credentialRefusals(err))
	}
}

func TestTheChangeRefusesTheCurrentPasswordAsTheNewOne(t *testing.T) {
	_, _, h := changeFixture(t)

	_, err := h.Handle(selfCtx(), changeCmd(oldPassword, oldPassword))
	if err == nil {
		t.Fatal("the current password was accepted as the new one")
	}
	if !raised(err, "PasswordUnchangedNotification") {
		t.Fatalf("the refusal did not come from the must-differ rule: %v", credentialRefusals(err))
	}
}

// TestTheChangeStillEnforcesThePolicy is the case the value object would MISS
// on its own: Password declares modes: [insert], so the generated update gate
// ignores it, and without credentialValueRules asking IsValid directly a
// five-character password would sail through this endpoint.
func TestTheChangeStillEnforcesThePolicy(t *testing.T) {
	store, hasher, h := changeFixture(t)

	_, err := h.Handle(selfCtx(), changeCmd(oldPassword, "short"))
	if err == nil {
		t.Fatal("a password below the policy was accepted by the change")
	}
	if !hasher.Matches(oldPassword, store.user.PasswordHash) {
		t.Fatal("a refused change rotated the credential anyway")
	}
}

func TestTheChangeComparesTheConfirmation(t *testing.T) {
	_, _, h := changeFixture(t)

	cmd := changeCmd(oldPassword, newPassword)
	cmd.PasswordConfirmation = "An0ther!Passphrase.typo"

	_, err := h.Handle(selfCtx(), cmd)
	if err == nil {
		t.Fatal("a mismatched confirmation was accepted by the change")
	}
	if !raised(err, "PasswordConfirmationMismatchNotification") {
		t.Fatalf("the refusal did not come from the confirmation rule: %v", credentialRefusals(err))
	}
}

// TestTheChangeStandsDownWithoutAnIdentity is the development bench: auth.mode
// disabled, which the framework only permits under APP_PROFILE=dev. The row
// decision asks whether an identity was PRESENT before it compares anything, so
// a tokenless bench can still exercise the endpoint.
func TestTheChangeStandsDownWithoutAnIdentity(t *testing.T) {
	store, hasher, h := changeFixture(t)

	if _, err := h.Handle(testCtx(), changeCmd(oldPassword, newPassword)); err != nil {
		t.Fatalf("a tokenless bench could not exercise the change: %v", err)
	}
	if !hasher.Matches(newPassword, store.user.PasswordHash) {
		t.Fatal("the change did not rotate the credential")
	}
}

// ── the reset's own row decision ────────────────────────────────────────────

// TestTheResetRefusesTheCallersOwnRow is what keeps the two operations
// disjoint. Without it a holder of user:reset-password could point the reset at
// their OWN row and replace the credential without proving the previous one —
// defeating the change endpoint's currentPassword by choosing the other URL.
func TestTheResetRefusesTheCallersOwnRow(t *testing.T) {
	store, hasher, h := resetFixture(t)

	_, err := h.Handle(selfCtx(), resetCmd())
	if err == nil {
		t.Fatal("a caller reset their own password without proving the current one")
	}
	if !raised(err, "PasswordResetRequiresAnotherUserNotification") {
		t.Fatalf("the refusal did not come from the row decision: %v", credentialRefusals(err))
	}
	if !hasher.Matches(oldPassword, store.user.PasswordHash) {
		t.Fatal("a refused reset rotated the credential anyway")
	}
}

// TestTheResetStillServesAnOperatorInTheSameTenant is the other half: the guard
// above must refuse the caller's OWN row and nothing else.
func TestTheResetStillServesAnOperatorInTheSameTenant(t *testing.T) {
	store, hasher, h := resetFixture(t)

	if _, err := h.Handle(operatorCtx(), resetCmd()); err != nil {
		t.Fatalf("an operator in the same tenant was refused: %v", err)
	}
	if !hasher.Matches(newPassword, store.user.PasswordHash) {
		t.Fatal("the operator's reset did not rotate the credential")
	}
	// Somebody else chose this password, so the next sign-in has to replace it.
	if !store.user.MustChangePassword {
		t.Fatal("the reset did not force a change on the next sign-in")
	}
}

// ── the row that is not there ───────────────────────────────────────────────

// A missing user answers the framework's own not-found and NOT a credential
// refusal: the caller is authenticated and already holds the permission, so
// there is no enumeration to protect against and a 404 is the honest answer.
// Both handlers load before they judge, so both have this path.

func TestTheChangeAnswersNotFoundForAMissingUser(t *testing.T) {
	store, _, h := changeFixture(t)
	store.user = nil

	if _, err := h.Handle(selfCtx(), changeCmd(oldPassword, newPassword)); err == nil {
		t.Fatal("the change accepted a user that does not exist")
	}
}

func TestTheResetAnswersNotFoundForAMissingUser(t *testing.T) {
	store, _, h := resetFixture(t)
	store.user = nil

	if _, err := h.Handle(operatorCtx(), resetCmd()); err == nil {
		t.Fatal("the reset accepted a user that does not exist")
	}
}
