// Tests for the two token handlers.
//
// WHAT THESE ASSERT IS THE SECURITY POSTURE, not the happy path. The happy path
// is one test; the rest exist because every one of them protects a property that
// is invisible in a green build and expensive to lose:
//
//	every refusal is the SAME notification, so no branch becomes an oracle;
//	the not-found branch BURNS a verification, so timing is not an oracle either;
//	a must-change-password token carries one permission, on BOTH paths;
//	a rotation rebuilds claims from the database instead of replaying them.
//
// The ports are faked because they are small and because faking them is what
// makes the refusal branches reachable without a signing key or a database. The
// one thing NOT faked is the notification: each refusal is inspected for the real
// InvalidCredentialsNotification, since "answers an error" would pass even if the
// branches answered different errors.

package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// ── fakes ───────────────────────────────────────────────────────────────────

type fakeAuthStore struct {
	user        *appdomain.User
	byID        *appdomain.User
	findErr     error
	roleKeys    []string
	permissions []vos.PermissionKey
	grantsErr   error
	matches     bool
	burned      int
	askedEmail  string
}

func (s *fakeAuthStore) FindUserByEmail(_ *configuration.AppContext, email string) (*appdomain.User, error) {
	s.askedEmail = email
	if s.findErr != nil {
		return nil, s.findErr
	}
	if s.user == nil {
		// (nil, nil) is the port's "no such account" — distinct from an error,
		// which means the lookup itself could not run.
		return nil, nil
	}
	return s.user, nil
}

func (s *fakeAuthStore) FindUserByID(_ *configuration.AppContext, _ domain.ID) (*appdomain.User, error) {
	if s.byID == nil {
		return nil, errors.New("not found")
	}
	return s.byID, nil
}

func (s *fakeAuthStore) ResolveGrants(context.Context, domain.ID) ([]string, []vos.PermissionKey, error) {
	return s.roleKeys, s.permissions, s.grantsErr
}

func (s *fakeAuthStore) PasswordMatches(string, string) bool { return s.matches }
func (s *fakeAuthStore) BurnPasswordVerification()           { s.burned++ }

type fakeAttempts struct {
	recorded     []recordedAttempt
	lockedFor    time.Duration
	knownToExist *bool
	lockErr      error
	recordErr    error
}

func (a *fakeAttempts) LockedUntil(context.Context, string) (time.Time, bool, *bool, error) {
	if a.lockErr != nil {
		return time.Time{}, false, nil, a.lockErr
	}
	if a.lockedFor == 0 {
		return time.Time{}, false, nil, nil
	}
	return time.Now().Add(a.lockedFor), true, a.knownToExist, nil
}

func (a *fakeAttempts) RecordFailure(_ context.Context, identity, kind, ip string, existed *bool) error {
	a.recorded = append(a.recorded, recordedAttempt{"failure", identity, kind, ip, existed})
	return a.recordErr
}

func (a *fakeAttempts) RecordSuccess(_ context.Context, identity, kind, ip string) error {
	// The store writes identity_existed = true for a success without being told;
	// the fake mirrors that so a test reading this slice sees what the table would.
	existed := true
	a.recorded = append(a.recorded, recordedAttempt{"success", identity, kind, ip, &existed})
	return a.recordErr
}

func (a *fakeAttempts) RecordLocked(_ context.Context, identity, kind, ip string, existed *bool) error {
	a.recorded = append(a.recorded, recordedAttempt{"locked", identity, kind, ip, existed})
	return a.recordErr
}

// outcomes renders what was logged, so a test can assert the SEQUENCE rather than
// just the last row.
func (a *fakeAttempts) outcomes() []string {
	out := make([]string, 0, len(a.recorded))
	for _, r := range a.recorded {
		out = append(out, r.outcome)
	}
	return out
}

// recordedAttempt is what the fake keeps. It mirrors the row the store would
// write, including the fields the port passes as separate arguments — so a test
// can assert that a branch logged the identity it refused.
type recordedAttempt struct {
	outcome  string
	identity string
	kind     string
	ip       string
	existed  *bool
}

type fakeLookup struct {
	subject string
	err     error
	asked   string
}

func (l *fakeLookup) SubjectForRefreshToken(_ context.Context, value string) (string, error) {
	l.asked = value
	return l.subject, l.err
}

type fakeIssuer struct {
	claims    map[string]any
	issueErr  error
	redeemErr error
	calls     int
}

func (i *fakeIssuer) IssueWithRefresh(_ context.Context, req authcore.TokenRequest) (authcore.IssuedToken, authcore.RefreshToken, error) {
	i.calls++
	i.claims = req.Claims
	if i.issueErr != nil {
		return authcore.IssuedToken{}, authcore.RefreshToken{}, i.issueErr
	}
	return authcore.IssuedToken{Token: "access-token", ExpiresAt: time.Unix(1000, 0)},
		authcore.RefreshToken{Value: "refresh-value", ExpiresAt: time.Unix(2000, 0)}, nil
}

func (i *fakeIssuer) RedeemRefreshToken(_ context.Context, _ string, claims map[string]any) (authcore.IssuedToken, authcore.RefreshToken, error) {
	i.calls++
	i.claims = claims
	if i.redeemErr != nil {
		return authcore.IssuedToken{}, authcore.RefreshToken{}, i.redeemErr
	}
	return authcore.IssuedToken{Token: "access-2", ExpiresAt: time.Unix(3000, 0)},
		authcore.RefreshToken{Value: "refresh-2", ExpiresAt: time.Unix(4000, 0)}, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func authCtx() *configuration.AppContext {
	return configuration.NewAppContextWithRandomID(configuration.LangENG)
}

// usableUser builds a loaded, signable account: active, in an active tenant, with
// the joined tenant columns the read joins fill on every real load.
func usableUser() *appdomain.User {
	id := domain.NewID("11111111-1111-1111-1111-111111111111")
	user := &appdomain.User{
		TenantID:        domain.NewID("22222222-2222-2222-2222-222222222222"),
		Name:            vos.PersonName{Given: "Ada", Family: "Lovelace"},
		Email:           vos.Email("ada@acme.test"),
		PasswordHash:    "$argon2id$stored",
		Status:          vos.UserStatusActive,
		TenantWorkspace: "acme",
		TenantStatus:    vos.TenantStatusActive.Value(),
	}
	user.SetID(id)
	return user
}

// assertCredentialRefusal fails unless err is EXACTLY the shared refusal: the
// right notification, under the right context, on the neutral field name.
//
// The field name is asserted because it is half the protection — a refusal keyed
// on "email" would tell a caller which of the two inputs was wrong.
func assertCredentialRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a refusal, got nil")
	}

	// The carrier is asserted as the INTERFACE, because that is what the pipeline
	// matches on — and then explicitly as NOT a domain error, because that is the
	// property worth pinning: these endpoints dispatch no rule and touch no
	// aggregate, so a refusal arriving as a *domain.DomainError would mean
	// something had started going through the domain without anyone deciding it
	// should.
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		t.Fatalf("expected a NotificationCarrier, got %T: %v", err, err)
	}
	var domainErr *domain.DomainError
	if errors.As(err, &domainErr) {
		t.Errorf("refusal arrived as a domain error (%T); the token endpoints decide this in the application layer", err)
	}
	contexts := carrier.NotificationContexts()
	if len(contexts) != 1 {
		t.Fatalf("expected exactly one notification context, got %d", len(contexts))
	}
	if got := contexts[0].Context(); got != "Authentication" {
		t.Errorf("context = %q, want %q", got, "Authentication")
	}
	msgs := contexts[0].Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected exactly one message, got %d", len(msgs))
	}
	if _, ok := msgs[0].Notification.(InvalidCredentialsNotification); !ok {
		t.Errorf("notification = %T, want InvalidCredentialsNotification", msgs[0].Notification)
	}
	if msgs[0].FieldName != "credentials" {
		t.Errorf("field = %q, want %q — a field name that says which half was wrong is an oracle",
			msgs[0].FieldName, "credentials")
	}
}

// ── the sign-in ─────────────────────────────────────────────────────────────

func TestIssueToken_Succeeds(t *testing.T) {
	store := &fakeAuthStore{
		user:        usableUser(),
		matches:     true,
		roleKeys:    []string{"billing-admin"},
		permissions: []vos.PermissionKey{{Resource: "user", Action: "read"}},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	result, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "right"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken != "access-token" || result.RefreshToken != "refresh-value" {
		t.Errorf("token pair not carried through: %+v", result)
	}
	if result.TokenType != "Bearer" {
		t.Errorf("tokenType = %q, want Bearer", result.TokenType)
	}
	if result.ExpiresAt != 1000 || result.RefreshExpiresAt != 2000 {
		t.Errorf("expiries not carried through: %d / %d", result.ExpiresAt, result.RefreshExpiresAt)
	}
	if store.burned != 0 {
		t.Errorf("a successful sign-in burned %d verifications; the burn is only for paths that never reach a real hash", store.burned)
	}
	if result.User.TenantWorkspace != "acme" {
		t.Errorf("workspace missing from the profile: %+v", result.User)
	}
}

// The e-mail is normalised before the lookup. Without this a user who capitalises
// their own address is told their credentials are wrong, because the column
// stores it lowercase.
func TestIssueToken_NormalisesEmail(t *testing.T) {
	store := &fakeAuthStore{user: usableUser(), matches: true}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

	if _, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "  Ada@ACME.test ", Password: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.askedEmail != "ada@acme.test" {
		t.Errorf("looked up %q, want the trimmed lowercase form", store.askedEmail)
	}
}

// THE TIMING TEST. A branch that never reaches a stored hash has to spend one
// anyway, or the response time answers "does this address exist here".
func TestIssueToken_UnknownEmailBurnsAVerification(t *testing.T) {
	store := &fakeAuthStore{} // no user
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "nobody@acme.test", Password: "x"})
	assertCredentialRefusal(t, err)
	if store.burned != 1 {
		t.Errorf("burned %d verifications, want exactly 1 — without it the refusal is faster than a real rejection and that difference IS the answer", store.burned)
	}
}

// A load FAILURE is refused like a miss, deliberately: answering 500 for one and
// 401 for the other would let a caller learn an address exists by finding an
// input that changes the status code.
func TestIssueToken_LookupFailureIsRefusedNotSurfaced(t *testing.T) {
	store := &fakeAuthStore{findErr: errors.New("connection reset")}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	assertCredentialRefusal(t, err)
	if store.burned != 1 {
		t.Errorf("burned %d, want 1", store.burned)
	}
}

// Every found-row refusal must be the same answer. Table-driven so a new branch
// added later without the shared refusal fails here rather than in production.
func TestIssueToken_EveryFoundRowRefusalIsIdentical(t *testing.T) {
	suspendedUser := usableUser()
	suspendedUser.Status = vos.UserStatusSuspended

	suspendedTenant := usableUser()
	suspendedTenant.TenantStatus = vos.TenantStatusSuspended.Value()

	missingTenantJoin := usableUser()
	missingTenantJoin.TenantStatus = ""

	cases := []struct {
		name    string
		user    *appdomain.User
		matches bool
	}{
		{"wrong password", usableUser(), false},
		{"suspended account", suspendedUser, true},
		{"suspended tenant", suspendedTenant, true},
		{"tenant join produced nothing", missingTenantJoin, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeAuthStore{user: tc.user, matches: tc.matches}
			h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

			_, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
			assertCredentialRefusal(t, err)
			// No burn on these paths: a real hash was already available, so the
			// cost is the real cost and a second one would only make the refusal
			// SLOWER than a success.
			if store.burned != 0 {
				t.Errorf("burned %d on a path that reached a real hash, want 0", store.burned)
			}
		})
	}
}

// A grants failure is NOT a credential refusal: the caller proved who they are,
// and answering 401 would both lie to them and bury an outage.
func TestIssueToken_GrantsFailureIsNotACredentialRefusal(t *testing.T) {
	boom := errors.New("permission query exploded")
	store := &fakeAuthStore{user: usableUser(), matches: true, grantsErr: boom}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the underlying failure to escape as an exception", err)
	}
	var carrier domain.NotificationCarrier
	if errors.As(err, &carrier) {
		t.Error("a resolution failure must not be dressed up as a credential refusal")
	}
}

// ── the claim set ───────────────────────────────────────────────────────────

func TestIssueToken_ClaimSet(t *testing.T) {
	user := usableUser()
	domain.AddAggregateChild(user, aggregatevos.UserGroup{
		GroupID:  domain.NewID("33333333-3333-3333-3333-333333333333"),
		GroupKey: "engineering", GroupName: "Engineering",
	})
	store := &fakeAuthStore{
		user:     user,
		matches:  true,
		roleKeys: []string{"billing-admin", "viewer"},
		permissions: []vos.PermissionKey{
			{Resource: "user", Action: "read"},
			{Resource: "tenant", Action: "update"},
		},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	if _, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// THE TWO SPELLINGS THAT ARE NOT OURS. Renaming either silently disables
	// authorization across the whole mesh rather than failing loudly, so they are
	// pinned here by literal.
	perms, ok := issuer.claims["permissions"].([]string)
	if !ok {
		t.Fatalf("permissions claim = %#v, want []string under the exact key the framework reads", issuer.claims["permissions"])
	}
	if len(perms) != 2 {
		t.Errorf("permissions = %v, want both grants", perms)
	}
	if issuer.claims["tenant_id"] != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("tenant_id claim = %v", issuer.claims["tenant_id"])
	}

	// The two with no other vehicle into another service's audit trail.
	if issuer.claims["tenant_workspace"] != "acme" {
		t.Errorf("tenant_workspace claim = %v — without it every audit line in the mesh names its tenant by an unresolvable UUID", issuer.claims["tenant_workspace"])
	}
	if issuer.claims["email"] != "ada@acme.test" {
		t.Errorf("email claim = %v", issuer.claims["email"])
	}

	if issuer.claims["name"] != "Ada Lovelace" {
		t.Errorf("name claim = %v", issuer.claims["name"])
	}
	if got, _ := issuer.claims["groups"].([]string); len(got) != 1 || got[0] != "engineering" {
		t.Errorf("groups claim = %v, want the KEYS only", issuer.claims["groups"])
	}
	if got, _ := issuer.claims["roles"].([]string); len(got) != 2 {
		t.Errorf("roles claim = %v, want every role held by any path", issuer.claims["roles"])
	}
	if issuer.claims["must_change_password"] != false {
		t.Errorf("must_change_password claim = %v", issuer.claims["must_change_password"])
	}

	// The display names are deliberately ABSENT from the token and PRESENT in the
	// body. This is the split the whole claim-set design turns on.
	if _, leaked := issuer.claims["group_names"]; leaked {
		t.Error("display names must not ride in the token")
	}
}

// ── the restricted session ──────────────────────────────────────────────────

func TestIssueToken_MustChangePasswordRestrictsTheBundle(t *testing.T) {
	user := usableUser()
	user.MustChangePassword = true

	store := &fakeAuthStore{
		user:    user,
		matches: true,
		permissions: []vos.PermissionKey{
			{Resource: "user", Action: "read"},
			{Resource: "user", Action: "change-password"},
			{Resource: "*", Action: "*"},
		},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	result, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	perms, _ := issuer.claims["permissions"].([]string)
	if len(perms) != 1 || perms[0] != PermissionChangeOwnPassword {
		t.Fatalf("permissions = %v, want exactly [%s] — a session that exists to rotate an expired credential must not be able to do anything else",
			perms, PermissionChangeOwnPassword)
	}
	// `*:*` in particular must be dropped: a super-admin with an expired password
	// is the most dangerous unrestricted session there is.
	for _, p := range perms {
		if p == "*:*" {
			t.Error("the wildcard survived the restriction")
		}
	}
	if issuer.claims["must_change_password"] != true {
		t.Error("the client is not told to route to the change screen")
	}
	// The response body mirrors the token, so a client never has to decode it.
	if len(result.User.Permissions) != 1 || result.User.Permissions[0] != PermissionChangeOwnPassword {
		t.Errorf("body permissions = %v, want the same restricted set the token carries", result.User.Permissions)
	}
}

// The dead end, asserted rather than discovered: a user who must change their
// password and does not hold the permission gets an EMPTY set, not an invented
// grant and not a wildcard.
func TestIssueToken_MustChangePasswordWithoutThePermissionIsADeadEnd(t *testing.T) {
	user := usableUser()
	user.MustChangePassword = true

	store := &fakeAuthStore{
		user:        user,
		matches:     true,
		permissions: []vos.PermissionKey{{Resource: "user", Action: "read"}},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	if _, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	perms, _ := issuer.claims["permissions"].([]string)
	if len(perms) != 0 {
		t.Errorf("permissions = %v, want an empty set — inventing the permission would hand out a grant no role confers", perms)
	}
}

// ── the rotation ────────────────────────────────────────────────────────────

func TestRefreshToken_Succeeds(t *testing.T) {
	store := &fakeAuthStore{
		byID:        usableUser(),
		roleKeys:    []string{"viewer"},
		permissions: []vos.PermissionKey{{Resource: "user", Action: "read"}},
	}
	lookup := &fakeLookup{subject: "11111111-1111-1111-1111-111111111111"}
	issuer := &fakeIssuer{}
	h := &RefreshTokenHandler{Store: store, Lookup: lookup, Issuer: issuer}

	result, err := h.Handle(authCtx(), &RefreshTokenCommand{RefreshToken: " opaque-value "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lookup.asked != "opaque-value" {
		t.Errorf("looked up %q, want the trimmed value", lookup.asked)
	}
	if result.AccessToken != "access-2" || result.RefreshToken != "refresh-2" {
		t.Errorf("rotated pair not carried through: %+v", result)
	}
	// THE POINT OF THE WHOLE PATH: the claims were rebuilt, not replayed.
	if issuer.claims["tenant_workspace"] != "acme" {
		t.Errorf("claims were not rebuilt from the database: %#v", issuer.claims)
	}
}

// A restricted session must not widen itself by rotating.
func TestRefreshToken_KeepsTheMustChangePasswordRestriction(t *testing.T) {
	user := usableUser()
	user.MustChangePassword = true

	store := &fakeAuthStore{
		byID: user,
		permissions: []vos.PermissionKey{
			{Resource: "user", Action: "change-password"},
			{Resource: "*", Action: "*"},
		},
	}
	issuer := &fakeIssuer{}
	h := &RefreshTokenHandler{Store: store, Lookup: &fakeLookup{subject: "s"}, Issuer: issuer}

	if _, err := h.Handle(authCtx(), &RefreshTokenCommand{RefreshToken: "v"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	perms, _ := issuer.claims["permissions"].([]string)
	if len(perms) != 1 || perms[0] != PermissionChangeOwnPassword {
		t.Errorf("permissions = %v — a restricted token laundered itself into a full one by refreshing", perms)
	}
}

// All three redemption failures answer identically. Reuse in particular: telling
// the holder of a stolen token that it was already redeemed confirms both that
// the token was real and that its owner is active.
func TestRefreshToken_EveryRedemptionFailureIsIdentical(t *testing.T) {
	for _, sentinel := range []error{
		authcore.ErrRefreshTokenNotFound,
		authcore.ErrRefreshTokenExpired,
		authcore.ErrRefreshTokenReused,
	} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			store := &fakeAuthStore{byID: usableUser()}
			h := &RefreshTokenHandler{
				Store:  store,
				Lookup: &fakeLookup{subject: "s"},
				Issuer: &fakeIssuer{redeemErr: sentinel},
			}
			_, err := h.Handle(authCtx(), &RefreshTokenCommand{RefreshToken: "v"})
			assertCredentialRefusal(t, err)
		})
	}
}

func TestRefreshToken_RefusesWithoutTouchingTheStore(t *testing.T) {
	lookup := &fakeLookup{}
	h := &RefreshTokenHandler{Store: &fakeAuthStore{}, Lookup: lookup, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &RefreshTokenCommand{RefreshToken: "   "})
	assertCredentialRefusal(t, err)
	if lookup.asked != "" {
		t.Error("an empty value must be refused before any lookup")
	}
}

// An unknown value is refused BEFORE grants are resolved — otherwise posting
// random strings is a free database walk.
func TestRefreshToken_UnknownValueResolvesNoGrants(t *testing.T) {
	store := &fakeAuthStore{grantsErr: errors.New("must not be called")}
	h := &RefreshTokenHandler{Store: store, Lookup: &fakeLookup{subject: ""}, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &RefreshTokenCommand{RefreshToken: "bogus"})
	assertCredentialRefusal(t, err)
}

// A lookup FAILURE is an outage, not a refusal.
func TestRefreshToken_LookupFailureEscapes(t *testing.T) {
	boom := errors.New("store down")
	h := &RefreshTokenHandler{
		Store:  &fakeAuthStore{},
		Lookup: &fakeLookup{err: boom},
		Issuer: &fakeIssuer{},
	}
	_, err := h.Handle(authCtx(), &RefreshTokenCommand{RefreshToken: "v"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the store failure to escape", err)
	}
}

// The account went away between rotations — archived, suspended, tenant
// withdrawn. The session ends, with the same refusal as everything else.
func TestRefreshToken_UnusableAccountEndsTheSession(t *testing.T) {
	suspended := usableUser()
	suspended.Status = vos.UserStatusSuspended

	h := &RefreshTokenHandler{
		Store:  &fakeAuthStore{byID: suspended},
		Lookup: &fakeLookup{subject: "s"},
		Issuer: &fakeIssuer{},
	}
	_, err := h.Handle(authCtx(), &RefreshTokenCommand{RefreshToken: "v"})
	assertCredentialRefusal(t, err)
}

// ── the projections ─────────────────────────────────────────────────────────

func TestBuildProfile_RolesComeFromTheResolvedGrants(t *testing.T) {
	user := usableUser()
	domain.AddAggregateChild(user, aggregatevos.UserRole{
		RoleID:  domain.NewID("44444444-4444-4444-4444-444444444444"),
		RoleKey: "billing-admin", RoleName: "Billing Admin",
	})

	// One direct (loaded as a row, so it has a display name) and one inherited
	// through a group (never loaded here, so it has none).
	roleKeys := []string{"billing-admin", "inherited-viewer"}

	profile := buildProfile(user, roleKeys, nil)
	if len(profile.Roles) != 2 {
		t.Fatalf("roles = %+v, want both the direct and the inherited one", profile.Roles)
	}
	byKey := map[string]string{}
	for _, role := range profile.Roles {
		byKey[role.Key] = role.Name
	}
	if byKey["billing-admin"] != "Billing Admin" {
		t.Errorf("the direct grant lost its display name: %+v", profile.Roles)
	}
	if byKey["inherited-viewer"] != "" {
		t.Errorf("an inherited role was given a name it never had: %q", byKey["inherited-viewer"])
	}
}

func TestRenderPermissions_DropsHalfKeysAndNeverReturnsNil(t *testing.T) {
	got := renderPermissions([]vos.PermissionKey{
		{Resource: "user", Action: "read"},
		{Resource: "", Action: "read"},
		{Resource: "user", Action: ""},
	})
	if len(got) != 1 || got[0] != "user:read" {
		t.Errorf("got %v, want only the complete key", got)
	}
	if renderPermissions(nil) == nil {
		t.Error("nil would render as JSON null; the claim must be present and empty")
	}
}

func TestAccountIsUsable(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*appdomain.User)
		want bool
	}{
		{"active in an active tenant", func(*appdomain.User) {}, true},
		{"active in a trial tenant", func(u *appdomain.User) { u.TenantStatus = vos.TenantStatusTrial.Value() }, true},
		{"suspended account", func(u *appdomain.User) { u.Status = vos.UserStatusSuspended }, false},
		{"suspended tenant", func(u *appdomain.User) { u.TenantStatus = vos.TenantStatusSuspended.Value() }, false},
		{"tenant join produced nothing", func(u *appdomain.User) { u.TenantStatus = "" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := usableUser()
			tc.mut(user)
			if got := accountIsUsable(user); got != tc.want {
				t.Errorf("accountIsUsable = %v, want %v", got, tc.want)
			}
		})
	}
}

// A TRIAL tenant signs in. This is the one that is easy to get wrong by reading
// "unavailable" as "not active" — and doing so would break every trial signup.
func TestIssueToken_TrialTenantSignsIn(t *testing.T) {
	user := usableUser()
	user.TenantStatus = vos.TenantStatusTrial.Value()

	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{user: user, matches: true},
		Attempts: &fakeAttempts{},
		Issuer:   &fakeIssuer{},
	}
	if _, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err != nil {
		t.Fatalf("a trial tenant was refused: %v", err)
	}
}

// ── the lockout ─────────────────────────────────────────────────────────────

// THE ORACLE TEST, and the reason the counter is not a column on `users`. A
// locked identity is refused BEFORE anything looks it up — so no credential is
// verified, no row is read, and the answer cannot depend on whether the account
// exists.
func TestIssueToken_LockedRefusesBeforeTouchingTheCredential(t *testing.T) {
	store := &fakeAuthStore{user: usableUser(), matches: true}
	attempts := &fakeAttempts{lockedFor: 7 * time.Minute}
	h := &IssueTokenHandler{Store: store, Attempts: attempts, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "Str0ng!Passphrase"})
	if err == nil {
		t.Fatal("a locked identity was let in")
	}
	// The correct password was supplied and must NOT have been checked: past the
	// threshold the whole point is to stop paying for guesses.
	if store.askedEmail != "" {
		t.Error("the account was looked up despite the lock — the lock must short-circuit first")
	}
	if store.burned != 0 {
		t.Error("a verification was spent on a locked identity")
	}
	// Recorded, and as `locked` rather than `failure`: a failure would count, and
	// counting attempts made during a lock would let anyone hold an account shut
	// indefinitely just by continuing to try.
	if got := attempts.outcomes(); len(got) != 1 || got[0] != "locked" {
		t.Errorf("logged %v, want exactly one locked attempt", got)
	}
}

// The 429 carries the remaining window, because telling somebody to wait without
// saying how long is not an instruction.
func TestIssueToken_LockedAnswersWithTheRemainingMinutes(t *testing.T) {
	attempts := &fakeAttempts{lockedFor: 7*time.Minute + 10*time.Second}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		t.Fatalf("expected a carrier, got %T", err)
	}
	msg := carrier.NotificationContexts()[0].Messages()[0]
	locked, ok := msg.Notification.(AccountTemporarilyLockedNotification)
	if !ok {
		t.Fatalf("notification = %T, want AccountTemporarilyLockedNotification", msg.Notification)
	}
	// ROUNDED UP: 7m10s is "8 minutes". Rounding down would send somebody back at
	// a moment still inside the lock.
	if locked.Minutes != "8" {
		t.Errorf("minutes = %q, want 8 — the remaining window rounds up, never down", locked.Minutes)
	}
	if msg.Notification.Semantic() != domain.SemanticTooManyRequests {
		t.Errorf("semantic = %v, want SemanticTooManyRequests (429)", msg.Notification.Semantic())
	}
	// Same neutral field name as the generic 401, so the two refusals are
	// indistinguishable in shape as well as in origin.
	if msg.FieldName != "credentials" {
		t.Errorf("field = %q, want credentials", msg.FieldName)
	}
}

// A lockout probe that cannot run has established NOTHING. Answering 401 would
// tell a caller with a correct password that it was wrong, and would hide an
// outage inside a login problem.
func TestIssueToken_LockoutProbeFailureEscapes(t *testing.T) {
	boom := errors.New("probe exploded")
	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{user: usableUser(), matches: true},
		Attempts: &fakeAttempts{lockErr: boom},
		Issuer:   &fakeIssuer{},
	}
	_, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the probe failure to escape as an exception", err)
	}
}

// A recording failure must NOT be swallowed. A brute-force protection that
// silently stops counting is worse than one that was never built — nobody would
// know it had stopped.
func TestIssueToken_RecordingFailurePropagates(t *testing.T) {
	boom := errors.New("cannot write the attempt")
	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{}, // unknown address → the failure branch records
		Attempts: &fakeAttempts{recordErr: boom},
		Issuer:   &fakeIssuer{},
	}
	_, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "nobody@acme.test", Password: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the recording failure to escape rather than be swallowed", err)
	}
}

// WHAT EACH BRANCH LOGS, and specifically the existence flag — the column a
// reviewer reads to tell a targeted attack from credential stuffing.
func TestIssueToken_LogsTheExistenceFlagPerBranch(t *testing.T) {
	cases := []struct {
		name        string
		store       *fakeAuthStore
		wantOutcome string
		wantExisted *bool
	}{
		{
			// The store established the address is NOT here, so the flag is false —
			// which is what separates credential stuffing from a targeted attack in
			// the log, while the RESPONSE stays identical either way.
			name: "unknown address", store: &fakeAuthStore{},
			wantOutcome: "failure", wantExisted: boolPtr(false),
		},
		{
			name: "wrong password", store: &fakeAuthStore{user: usableUser(), matches: false},
			wantOutcome: "failure", wantExisted: boolPtr(true),
		},
		{
			// A valid credential for a disabled account is still a failure —
			// nobody got in — and it counts, which is right: repeatedly presenting
			// a working password for a suspended account is exactly the pattern
			// worth rate-limiting.
			name: "suspended account", store: &fakeAuthStore{user: suspendedUser(), matches: true},
			wantOutcome: "failure", wantExisted: boolPtr(true),
		},
		{
			name: "success", store: &fakeAuthStore{user: usableUser(), matches: true},
			wantOutcome: "success", wantExisted: boolPtr(true),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attempts := &fakeAttempts{}
			h := &IssueTokenHandler{Store: tc.store, Attempts: attempts, Issuer: &fakeIssuer{}}

			_, _ = h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

			if len(attempts.recorded) != 1 {
				t.Fatalf("logged %d attempts, want exactly 1", len(attempts.recorded))
			}
			got := attempts.recorded[0]
			if got.outcome != tc.wantOutcome {
				t.Errorf("outcome = %q, want %q", got.outcome, tc.wantOutcome)
			}
			// The identity logged must be the NORMALISED one — otherwise two
			// spellings of an address count toward two windows and neither locks.
			if got.identity != "ada@acme.test" {
				t.Errorf("identity = %q, want the normalised address", got.identity)
			}
			if got.kind != "user" {
				t.Errorf("kind = %q, want user", got.kind)
			}
			switch {
			case tc.wantExisted == nil && got.existed != nil:
				t.Errorf("identity_existed = %v, want NULL — writing false here would record something the service does not know", *got.existed)
			case tc.wantExisted != nil && got.existed == nil:
				t.Errorf("identity_existed = NULL, want %v", *tc.wantExisted)
			case tc.wantExisted != nil && *got.existed != *tc.wantExisted:
				t.Errorf("identity_existed = %v, want %v", *got.existed, *tc.wantExisted)
			}
		})
	}
}

// The success is logged LAST, after the token is minted. It anchors the window —
// every failure before it stops counting — so writing it any earlier would clear
// the counter for a sign-in that had not happened yet.
func TestIssueToken_SuccessIsLoggedOnlyAfterTheTokenExists(t *testing.T) {
	attempts := &fakeAttempts{}
	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{user: usableUser(), matches: true},
		Attempts: attempts,
		Issuer:   &fakeIssuer{issueErr: errors.New("signing key unusable")},
	}
	if _, err := h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err == nil {
		t.Fatal("expected the issuer failure to surface")
	}
	if len(attempts.recorded) != 0 {
		t.Errorf("logged %v — a sign-in that produced no token must not anchor the window", attempts.outcomes())
	}
}

// A grants failure records NOTHING. The caller proved who they are and this
// service failed them; logging a failure there would let a database problem lock
// out the very users it is already failing.
func TestIssueToken_GrantsFailureRecordsNoAttempt(t *testing.T) {
	attempts := &fakeAttempts{}
	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{user: usableUser(), matches: true, grantsErr: errors.New("boom")},
		Attempts: attempts,
		Issuer:   &fakeIssuer{},
	}
	_, _ = h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	if len(attempts.recorded) != 0 {
		t.Errorf("logged %v — an outage on our side must not count against the caller", attempts.outcomes())
	}
}

// The origin address reaches the log from the context the /auth middleware fills.
func TestIssueToken_CarriesTheOriginAddressIntoTheLog(t *testing.T) {
	ctx := authCtx()
	ctx.Set(ContextKeyClientIP, "203.0.113.7")

	attempts := &fakeAttempts{}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Issuer: &fakeIssuer{}}
	_, _ = h.Handle(ctx, &IssueTokenCommand{Email: "x@y.test", Password: "z"})

	if got := attempts.recorded[0].ip; got != "203.0.113.7" {
		t.Errorf("ip = %q, want the address the middleware left on the context", got)
	}
}

// A missing address is "" and does not fail the sign-in: refusing an operation
// because a forensic field was absent would trade the operation for its own log.
func TestIssueToken_MissingOriginAddressIsNotFatal(t *testing.T) {
	attempts := &fakeAttempts{}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Issuer: &fakeIssuer{}}
	_, _ = h.Handle(authCtx(), &IssueTokenCommand{Email: "x@y.test", Password: "z"})

	if got := attempts.recorded[0].ip; got != "" {
		t.Errorf("ip = %q, want empty", got)
	}
}

func boolPtr(b bool) *bool { return &b }

func suspendedUser() *appdomain.User {
	u := usableUser()
	u.Status = vos.UserStatusSuspended
	return u
}

// THE DISTINCTION THE FORENSIC COLUMN EXISTS FOR: the response is identical, the
// record is not. An unknown address logs `false`; a lookup that could not run logs
// NULL, because in that case nobody established anything.
//
// Without this, a reviewer cannot tell credential stuffing against addresses that
// do not exist here from an attack that happened to land during an outage — which
// is the difference that decides how urgently anyone reacts.
func TestIssueToken_AbsenceAndOutageRefuseAlikeButLogDifferently(t *testing.T) {
	absent := &fakeAttempts{}
	absentH := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: absent, Issuer: &fakeIssuer{}}
	_, absentErr := absentH.Handle(authCtx(), &IssueTokenCommand{Email: "ghost@acme.test", Password: "x"})

	outage := &fakeAttempts{}
	outageH := &IssueTokenHandler{
		Store:    &fakeAuthStore{findErr: errors.New("connection reset")},
		Attempts: outage,
		Issuer:   &fakeIssuer{},
	}
	_, outageErr := outageH.Handle(authCtx(), &IssueTokenCommand{Email: "ghost@acme.test", Password: "x"})

	// Identical to the caller — this half is what keeps the status code from
	// becoming an existence oracle.
	assertCredentialRefusal(t, absentErr)
	assertCredentialRefusal(t, outageErr)

	// Different in the log — this half is what makes the log worth keeping.
	if absent.recorded[0].existed == nil || *absent.recorded[0].existed {
		t.Errorf("an unknown address logged %v, want false", absent.recorded[0].existed)
	}
	if outage.recorded[0].existed != nil {
		t.Errorf("an outage logged %v, want NULL — nobody established whether the account exists",
			*outage.recorded[0].existed)
	}
}

// THE REASON THE EXISTENCE COLUMN EXISTS, asserted at the handler: a locked
// attempt is stamped with what the failures that caused the lock established.
//
// Without this, somebody filtering `WHERE outcome = 'locked'` — which is exactly
// how a manager looks for accounts under attack — sees a column of blanks and
// cannot tell a real account being hammered from noise against an address that
// does not exist. The verdict costs nothing: the lockout probe read those rows
// anyway.
func TestIssueToken_LockedAttemptCarriesTheExistenceVerdict(t *testing.T) {
	real := true
	attempts := &fakeAttempts{lockedFor: 5 * time.Minute, knownToExist: &real}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Issuer: &fakeIssuer{}}

	_, _ = h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

	got := attempts.recorded[0]
	if got.outcome != "locked" {
		t.Fatalf("outcome = %q, want locked", got.outcome)
	}
	if got.existed == nil || !*got.existed {
		t.Errorf("identity_existed = %v on a locked row, want true — a manager filtering locked attempts must see which are real accounts", got.existed)
	}
}

// And it still claims nothing when nothing was established. The column never
// guesses: an identity whose failures all landed during an outage carries no
// verdict, and a locked row for it stays blank rather than inventing one.
func TestIssueToken_LockedAttemptClaimsNothingWhenNothingIsKnown(t *testing.T) {
	attempts := &fakeAttempts{lockedFor: 5 * time.Minute} // knownToExist nil
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Issuer: &fakeIssuer{}}

	_, _ = h.Handle(authCtx(), &IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

	if got := attempts.recorded[0].existed; got != nil {
		t.Errorf("identity_existed = %v, want NULL", *got)
	}
}
