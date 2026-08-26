// Tests for the two credential handlers.
//
// They exercise the part the contract suite cannot reach cheaply and the
// generated suite never knew about: the ORDER of the credential barrier, what
// each step answers, and the one write that has to survive a rejection.
//
// The hasher is the REAL one. Argon2id costs ~100 ms a call, so the cases below
// are deliberately few — but faking it would fake exactly the thing several of
// these assert (that a miss and a hit cost the same, that a wrong password does
// not verify, that a new hash is not the old one).

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

// fakeStore records every write, which is how the failure-counter case proves a
// row was persisted on a request that was REFUSED.
type fakeStore struct {
	user      *appdomain.User
	findErr   error
	updates   []*appdomain.User
	updateErr error
}

func (s *fakeStore) FindOneByEmail(_ *configuration.AppContext, email string) (*appdomain.User, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	if s.user == nil || s.user.Email.Value() != email {
		// (nil, nil) for an address nobody holds — the contract the real
		// repository keeps, so the handler's miss path is the one under test.
		return nil, nil
	}
	return s.user, nil
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
	return w.store.updateErr
}
func (w *fakeWriter) Archive(domain.Archivable) error     { return nil }
func (w *fakeWriter) Unarchive(domain.Unarchivable) error { return nil }
func (w *fakeWriter) Delete(domain.Deletable) error       { return nil }

// countingHasher wraps the REAL hasher and records which paths were taken, so a
// case can assert that the miss burned a verification rather than returning
// early.
type countingHasher struct {
	inner   appdomain.PasswordHasher
	matches int
	dummies int
}

func (h *countingHasher) Hash(p string) string { return h.inner.Hash(p) }
func (h *countingHasher) Matches(p, e string) bool {
	h.matches++
	return h.inner.Matches(p, e)
}
func (h *countingHasher) DummyMatches(p string) {
	h.dummies++
	h.inner.DummyMatches(p)
}

// credentialService is the domain service the aggregate requires. It delegates
// the two credential facts to the real hasher and answers "nothing wrong" to
// everything else — the same posture the generated stub takes.
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
	knownEmail  = "maria@acme.com"
	oldPassword = "Str0ng!Passphrase"
	newPassword = "An0ther!Passphrase"
)

func credentialFixture(t *testing.T) (*fakeStore, *countingHasher, *ChangePasswordHandler) {
	t.Helper()

	hasher := &countingHasher{inner: infra.NewArgon2idHasher()}
	user := &appdomain.User{
		TenantID:     domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"),
		Email:        vos.Email(knownEmail),
		Name:         vos.PersonName{Given: "Maria", Family: "Souza Lima"},
		Status:       vos.UserStatus("active"),
		PasswordHash: hasher.Hash(oldPassword),
	}
	user.SetID(domain.NewID("7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"))

	store := &fakeStore{user: user}
	return store, hasher, &ChangePasswordHandler{
		Store:   store,
		Hasher:  hasher,
		Service: &credentialService{hasher: hasher},
	}
}

func testCtx() *configuration.AppContext {
	return configuration.NewAppContextWithRandomID(configuration.LangENG)
}

func isGenericRefusal(t *testing.T, err error) bool {
	t.Helper()
	if err == nil {
		return false
	}
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		return false
	}
	for _, c := range carrier.NotificationContexts() {
		for _, m := range c.Messages() {
			if _, ok := m.Notification.(appdomain.InvalidCredentialsNotification); ok {
				return true
			}
		}
	}
	return false
}

// ── the barrier, step by step ───────────────────────────────────────────────

// TestAnUnknownAddressBurnsAVerification is the timing guard at the handler
// level. The generic message is worthless if a miss returns in a millisecond
// and a hit takes a hundred: the clock enumerates users just as well as a
// distinct reply would.
func TestAnUnknownAddressBurnsAVerification(t *testing.T) {
	store, hasher, h := credentialFixture(t)
	store.user.Email = vos.Email("someone-else@acme.com")

	_, err := h.Handle(testCtx(), &ChangePasswordCommand{
		Email: knownEmail, CurrentPassword: oldPassword,
		Password: newPassword, PasswordConfirmation: newPassword,
	})

	if !isGenericRefusal(t, err) {
		t.Fatalf("an unknown address did not answer the generic refusal: %v", err)
	}
	if hasher.dummies != 1 {
		t.Fatalf("the miss path burned %d dummy verifications; it must burn exactly one", hasher.dummies)
	}
	if len(store.updates) != 0 {
		t.Fatal("a miss wrote to the store")
	}
}

// TestALockedAccountAnswersTheSameThingAsAWrongPassword is the assertion that a
// lockout does not announce itself. One that did would be the oracle the generic
// message exists to close — and it would tell an attacker exactly when to come
// back.
func TestALockedAccountAnswersTheSameThingAsAWrongPassword(t *testing.T) {
	store, hasher, h := credentialFixture(t)
	store.user.LockedUntil = time.Now().UTC().Add(10 * time.Minute)

	_, err := h.Handle(testCtx(), &ChangePasswordCommand{
		Email: knownEmail, CurrentPassword: oldPassword, // the CORRECT password
		Password: newPassword, PasswordConfirmation: newPassword,
	})

	if !isGenericRefusal(t, err) {
		t.Fatalf("a locked account answered something other than the generic refusal: %v", err)
	}
	// Even with the right password: the lock is checked BEFORE the verification,
	// and it burns the dummy so the locked path costs what every other path
	// costs.
	if hasher.dummies != 1 {
		t.Fatalf("the locked path burned %d dummy verifications", hasher.dummies)
	}
	if len(store.updates) != 0 {
		t.Fatal("a locked account was written to")
	}
}

// TestAnExpiredLockIsNotALock — the window passing is what releases it, with no
// sweeper and no second write.
func TestAnExpiredLockIsNotALock(t *testing.T) {
	store, _, h := credentialFixture(t)
	store.user.LockedUntil = time.Now().UTC().Add(-time.Minute)

	_, err := h.Handle(testCtx(), &ChangePasswordCommand{
		Email: knownEmail, CurrentPassword: oldPassword,
		Password: newPassword, PasswordConfirmation: newPassword,
	})
	if err != nil {
		t.Fatalf("an expired lock still refused the change: %v", err)
	}
}

// TestAWrongPasswordPersistsTheCounterOnARejectedRequest is the case that could
// not be a rule.
//
// A rejected aggregate write persists nothing, so if this counter lived in
// BuildRules it would be discarded with the refusal and the lockout would
// silently never engage — while every happy-path test stayed green.
func TestAWrongPasswordPersistsTheCounterOnARejectedRequest(t *testing.T) {
	store, _, h := credentialFixture(t)

	_, err := h.Handle(testCtx(), &ChangePasswordCommand{
		Email: knownEmail, CurrentPassword: "Wr0ng!Passphrase",
		Password: newPassword, PasswordConfirmation: newPassword,
	})

	if !isGenericRefusal(t, err) {
		t.Fatalf("a wrong password answered something other than the generic refusal: %v", err)
	}
	if len(store.updates) != 1 {
		t.Fatalf("the failure counter was not persisted: %d write(s)", len(store.updates))
	}
	if store.user.FailedLoginAttempts != 1 {
		t.Fatalf("the counter reads %d after one failure", store.user.FailedLoginAttempts)
	}
	// And the credential itself is untouched: a refused change must leave the
	// stored password exactly as it was.
	if !h.Hasher.Matches(oldPassword, store.user.PasswordHash) {
		t.Fatal("a refused change altered the stored credential")
	}
}

// TestTheThresholdLocks walks the counter to the bound.
func TestTheThresholdLocks(t *testing.T) {
	store, _, h := credentialFixture(t)

	for i := 0; i < 5; i++ {
		_, _ = h.Handle(testCtx(), &ChangePasswordCommand{
			Email: knownEmail, CurrentPassword: "Wr0ng!Passphrase",
			Password: newPassword, PasswordConfirmation: newPassword,
		})
	}

	if !store.user.IsLockedAt(time.Now().UTC()) {
		t.Fatal("five consecutive failures did not lock the account")
	}
	// The counter resets rather than sitting at the threshold, so an expired
	// lock gives a fresh budget instead of locking again on the next mistake.
	if store.user.FailedLoginAttempts != 0 {
		t.Fatalf("the counter reads %d after locking; it should reset", store.user.FailedLoginAttempts)
	}
}

// ── past the barrier, the answers become specific ───────────────────────────

// TestAWeakNewPasswordIsReportedSpecifically is the other half of the generic
// contract, and the half that is easy to lose.
//
// A caller who PROVED possession has to be told why their new password was
// refused, or they can never succeed. Collapsing this into the generic answer
// would be a change-password endpoint nobody can use.
func TestAWeakNewPasswordIsReportedSpecifically(t *testing.T) {
	_, _, h := credentialFixture(t)

	_, err := h.Handle(testCtx(), &ChangePasswordCommand{
		Email: knownEmail, CurrentPassword: oldPassword,
		Password: "short", PasswordConfirmation: "short",
	})

	if err == nil {
		t.Fatal("a password that breaks the policy was accepted")
	}
	if isGenericRefusal(t, err) {
		t.Fatal("the new password's own failure was answered generically — the caller can never succeed")
	}
}

// TestTheConfirmationIsComparedOnTheChangeToo — the rule is declarative on the
// insert path and hand-written here, and this is what proves the second one
// exists at all.
func TestTheConfirmationIsComparedOnTheChangeToo(t *testing.T) {
	_, _, h := credentialFixture(t)

	_, err := h.Handle(testCtx(), &ChangePasswordCommand{
		Email: knownEmail, CurrentPassword: oldPassword,
		Password: newPassword, PasswordConfirmation: "An0ther!Passphras",
	})
	if err == nil {
		t.Fatal("a mismatched confirmation was accepted")
	}
}

// TestReusingTheCurrentPasswordIsRefused.
func TestReusingTheCurrentPasswordIsRefused(t *testing.T) {
	_, _, h := credentialFixture(t)

	_, err := h.Handle(testCtx(), &ChangePasswordCommand{
		Email: knownEmail, CurrentPassword: oldPassword,
		Password: oldPassword, PasswordConfirmation: oldPassword,
	})
	if err == nil {
		t.Fatal("the current password was accepted as the new one")
	}
}

// TestASuccessfulChangeRotatesTheCredentialAndClearsTheState.
func TestASuccessfulChangeRotatesTheCredentialAndClearsTheState(t *testing.T) {
	store, _, h := credentialFixture(t)
	store.user.MustChangePassword = true
	store.user.FailedLoginAttempts = 3
	before := store.user.PasswordHash

	if _, err := h.Handle(testCtx(), &ChangePasswordCommand{
		Email: knownEmail, CurrentPassword: oldPassword,
		Password: newPassword, PasswordConfirmation: newPassword,
	}); err != nil {
		t.Fatalf("a valid change was refused: %v", err)
	}

	if store.user.PasswordHash == before {
		t.Fatal("the stored hash did not change")
	}
	if !h.Hasher.Matches(newPassword, store.user.PasswordHash) {
		t.Fatal("the new password does not verify against the stored hash")
	}
	if store.user.MustChangePassword {
		t.Fatal("the must-change flag survived a change the user made themselves")
	}
	if store.user.FailedLoginAttempts != 0 || !store.user.LockedUntil.IsZero() {
		t.Fatal("a successful change left the lockout state behind")
	}
	if strings.Contains(store.user.PasswordHash, newPassword) {
		t.Fatal("the plaintext is inside the stored hash")
	}
}

// ── the reset ───────────────────────────────────────────────────────────────

// TestTheResetSetsTheMustChangeFlag is the ONE field the two operations differ
// on, and it is the difference that matters: the password a reset leaves is
// somebody else's choice, so the next sign-in has to replace it.
func TestTheResetSetsTheMustChangeFlag(t *testing.T) {
	store, hasher, _ := credentialFixture(t)
	store.user.MustChangePassword = false

	h := &ResetPasswordHandler{Store: store, Service: &credentialService{hasher: hasher}}

	cmd := &ResetPasswordCommand{Password: newPassword, PasswordConfirmation: newPassword}
	cmd.SetPathID(store.user.GetID().String())

	if _, err := h.Handle(testCtx(), cmd); err != nil {
		t.Fatalf("a valid reset was refused: %v", err)
	}

	if !store.user.MustChangePassword {
		t.Fatal("the reset did not force a change on the next sign-in")
	}
	if !hasher.Matches(newPassword, store.user.PasswordHash) {
		t.Fatal("the reset did not rotate the credential")
	}
}

// TestTheResetNeedsNoCurrentPassword — not knowing it is the point. This is the
// negative of the change test: the same command shape, minus the proof of
// possession, and it succeeds.
func TestTheResetNeedsNoCurrentPassword(t *testing.T) {
	store, hasher, _ := credentialFixture(t)
	h := &ResetPasswordHandler{Store: store, Service: &credentialService{hasher: hasher}}

	cmd := &ResetPasswordCommand{Password: newPassword, PasswordConfirmation: newPassword}
	cmd.SetPathID(store.user.GetID().String())

	if _, err := h.Handle(testCtx(), cmd); err != nil {
		t.Fatalf("the reset required something it should not have: %v", err)
	}
}

// TestTheResetStillEnforcesThePolicy — the authorization is different, the
// password rules are not.
func TestTheResetStillEnforcesThePolicy(t *testing.T) {
	store, hasher, _ := credentialFixture(t)
	h := &ResetPasswordHandler{Store: store, Service: &credentialService{hasher: hasher}}

	cmd := &ResetPasswordCommand{Password: "short", PasswordConfirmation: "short"}
	cmd.SetPathID(store.user.GetID().String())

	if _, err := h.Handle(testCtx(), cmd); err == nil {
		t.Fatal("a reset accepted a password the policy forbids")
	}
}
