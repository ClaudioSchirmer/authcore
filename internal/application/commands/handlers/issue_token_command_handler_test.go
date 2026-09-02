// Tests for the two token handlers.
//
// WHAT THESE ASSERT IS THE SECURITY POSTURE, not the happy path. The happy path
// is one test; the rest exist because every one of them protects a property that
// is invisible in a green build and expensive to lose:
//
//	every utils.Refusal is the SAME notification, so no branch becomes an oracle;
//	the not-found branch BURNS a verification, so timing is not an oracle either;
//	a must-change-password token carries one permission, on BOTH paths;
//	a rotation rebuilds claims from the database instead of replaying them.
//
// The ports are faked because they are small and because faking them is what
// makes the utils.Refusal branches reachable without a signing key or a database. The
// one thing NOT faked is the notification: each utils.Refusal is inspected for the real
// InvalidCredentialsNotification, since "answers an error" would pass even if the
// branches answered different errors.

package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/utils"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/persistence"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// ── fakes ───────────────────────────────────────────────────────────────────

type fakeAuthStore struct {
	account     *schemas.SignInAccount
	byID        *schemas.SignInAccount
	findErr     error
	groups      []infra.NamedGrant
	roles       []infra.NamedGrant
	permissions []vos.PermissionKey
	claimValues map[domain.ID]string
	definitions []schemas.ClaimDefinition
	grantsErr   error
	matches     bool
	burned      int
	askedEmail  string
	catalogErr  error
	// Counted, so the rotation test can assert the catalog is RE-READ rather
	// than replayed from the token being redeemed.
	catalogReads int
	askedTenant  domain.ID
}

func (s *fakeAuthStore) LoadAccountByEmail(_ context.Context, email string) (*schemas.SignInAccount, error) {
	s.askedEmail = email
	if s.findErr != nil {
		return nil, s.findErr
	}
	return s.account, nil
}

func (s *fakeAuthStore) LoadAccountByID(context.Context, domain.ID) (*schemas.SignInAccount, error) {
	if s.byID == nil {
		return nil, errors.New("not found")
	}
	return s.byID, nil
}

func (s *fakeAuthStore) ResolveSignIn(context.Context, *schemas.SignInAccount) (infra.SignInBundle, error) {
	if s.grantsErr != nil {
		return infra.SignInBundle{}, s.grantsErr
	}
	s.catalogReads++
	return infra.SignInBundle{
		Groups:      s.groups,
		Roles:       s.roles,
		Permissions: s.permissions,
		ClaimValues: s.claimValues,
		Definitions: s.definitions,
	}, s.catalogErr
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

func (a *fakeAttempts) LockedUntil(context.Context, string, string) (time.Time, bool, *bool, error) {
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

// RecordLocked takes NO existence flag: it bumps a lifetime counter on the
// failure row and this path never performs a lookup. What the failures
// established travels with the ANNOUNCEMENT instead — see fakePublisher.
func (a *fakeAttempts) RecordLocked(_ context.Context, identity, kind, ip string) error {
	a.recorded = append(a.recorded, recordedAttempt{"locked", identity, kind, ip, nil})
	return a.recordErr
}

// fakePublisher stands in for the service's log stream: it keeps every
// announcement so a test can assert what a reviewer would read later.
//
// It records through the framework's own port signature, which is what the
// application declares — so this double proves the handler talks to something
// *events.SlogPublisher can be.
type fakePublisher struct {
	published []domain.DomainEvent
	err       error
}

func (p *fakePublisher) Publish(_ persistence.RequestContext, event domain.Event) error {
	if de, ok := event.(domain.DomainEvent); ok {
		p.published = append(p.published, de)
	}
	return p.err
}

// messages renders what was announced, so a test can assert the SEQUENCE.
func (p *fakePublisher) messages() []string {
	out := make([]string, 0, len(p.published))
	for _, e := range p.published {
		out = append(out, e.Msg)
	}
	return out
}

// only returns the single announcement a branch is expected to have made.
func (p *fakePublisher) only(t *testing.T) domain.DomainEvent {
	t.Helper()
	if len(p.published) != 1 {
		t.Fatalf("announced %v, want exactly one record", p.messages())
	}
	return p.published[0]
}

// vals reads one key out of an announcement's payload.
func vals(t *testing.T, e domain.DomainEvent, key string) (any, bool) {
	t.Helper()
	m, ok := e.Vals.(map[string]any)
	if !ok {
		t.Fatalf("payload is %T, want map[string]any", e.Vals)
	}
	v, present := m[key]
	return v, present
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
// namedGrants spells a list of keys as the grants the reader returns. The display
// name mirrors the key: these tests assert on what the TOKEN carries, and the
// token carries keys.
func namedGrants(keys ...string) []infra.NamedGrant {
	out := make([]infra.NamedGrant, 0, len(keys))
	for _, k := range keys {
		out = append(out, infra.NamedGrant{Key: k, Name: k})
	}
	return out
}

func usableAccount() *schemas.SignInAccount {
	return &schemas.SignInAccount{
		ID:              domain.NewID("11111111-1111-1111-1111-111111111111"),
		TenantID:        domain.NewID("22222222-2222-2222-2222-222222222222"),
		GivenName:       "Ada",
		FamilyName:      "Lovelace",
		Email:           "ada@acme.test",
		PasswordHash:    "$argon2id$stored",
		Status:          vos.UserStatusActive.Value(),
		TenantWorkspace: "acme",
		TenantStatus:    vos.TenantStatusActive.Value(),
	}
}

// assertCredentialRefusal fails unless err is EXACTLY the shared utils.Refusal: the
// right notification, under the right context, on the neutral field name.
//
// The field name is asserted because it is half the protection — a utils.Refusal keyed
// on "email" would tell a caller which of the two inputs was wrong.
func assertCredentialRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a utils.Refusal, got nil")
	}

	// The carrier is asserted as the INTERFACE, because that is what the pipeline
	// matches on — and then explicitly as NOT a domain error, because that is the
	// property worth pinning: these endpoints dispatch no rule and touch no
	// aggregate, so a utils.Refusal arriving as a *domain.DomainError would mean
	// something had started going through the domain without anyone deciding it
	// should.
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		t.Fatalf("expected a NotificationCarrier, got %T: %v", err, err)
	}
	var domainErr *domain.DomainError
	if errors.As(err, &domainErr) {
		t.Errorf("utils.Refusal arrived as a domain error (%T); the token endpoints decide this in the application layer", err)
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
		account:     usableAccount(),
		matches:     true,
		roles:       namedGrants("billing-admin"),
		permissions: []vos.PermissionKey{{Resource: "user", Action: "read"}},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	result, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "right"})
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
	store := &fakeAuthStore{account: usableAccount(), matches: true}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

	if _, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "  Ada@ACME.test ", Password: "x"}); err != nil {
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

	_, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "nobody@acme.test", Password: "x"})
	assertCredentialRefusal(t, err)
	if store.burned != 1 {
		t.Errorf("burned %d verifications, want exactly 1 — without it the utils.Refusal is faster than a real rejection and that difference IS the answer", store.burned)
	}
}

// A load FAILURE is refused like a miss, deliberately: answering 500 for one and
// 401 for the other would let a caller learn an address exists by finding an
// input that changes the status code.
func TestIssueToken_LookupFailureIsRefusedNotSurfaced(t *testing.T) {
	store := &fakeAuthStore{findErr: errors.New("connection reset")}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	assertCredentialRefusal(t, err)
	if store.burned != 1 {
		t.Errorf("burned %d, want 1", store.burned)
	}
}

// Every found-row utils.Refusal must be the same answer. Table-driven so a new branch
// added later without the shared utils.Refusal fails here rather than in production.
func TestIssueToken_EveryFoundRowRefusalIsIdentical(t *testing.T) {
	suspendedUser := usableAccount()
	suspendedUser.Status = vos.UserStatusSuspended.Value()

	suspendedTenant := usableAccount()
	suspendedTenant.TenantStatus = vos.TenantStatusSuspended.Value()

	missingTenantJoin := usableAccount()
	missingTenantJoin.TenantStatus = ""

	cases := []struct {
		name    string
		account *schemas.SignInAccount
		matches bool
	}{
		{"wrong password", usableAccount(), false},
		{"suspended account", suspendedUser, true},
		{"suspended tenant", suspendedTenant, true},
		{"tenant join produced nothing", missingTenantJoin, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeAuthStore{account: tc.account, matches: tc.matches}
			h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

			_, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
			assertCredentialRefusal(t, err)
			// No burn on these paths: a real hash was already available, so the
			// cost is the real cost and a second one would only make the utils.Refusal
			// SLOWER than a success.
			if store.burned != 0 {
				t.Errorf("burned %d on a path that reached a real hash, want 0", store.burned)
			}
		})
	}
}

// A grants failure is NOT a credential utils.Refusal: the caller proved who they are,
// and answering 401 would both lie to them and bury an outage.
func TestIssueToken_GrantsFailureIsNotACredentialRefusal(t *testing.T) {
	boom := errors.New("permission query exploded")
	store := &fakeAuthStore{account: usableAccount(), matches: true, grantsErr: boom}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the underlying failure to escape as an exception", err)
	}
	var carrier domain.NotificationCarrier
	if errors.As(err, &carrier) {
		t.Error("a resolution failure must not be dressed up as a credential utils.Refusal")
	}
}

// ── the claim set ───────────────────────────────────────────────────────────

func TestIssueToken_ClaimSet(t *testing.T) {
	// THE MEMBERSHIPS COME FROM THE RESOLUTION AND FROM NOWHERE ELSE, which is
	// now true by construction rather than by discipline: the account is a ROW,
	// with no collections hanging off it, so there is no second source for a
	// membership to arrive from. The version of this test that guarded against
	// one — an aggregate carrying a retired group the resolution excluded — is
	// gone with the aggregate.
	account := usableAccount()
	store := &fakeAuthStore{
		account: account,
		matches: true,
		groups:  namedGrants("engineering"),
		roles:   namedGrants("billing-admin", "viewer"),
		permissions: []vos.PermissionKey{
			{Resource: "user", Action: "read"},
			{Resource: "tenant", Action: "update"},
		},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	if _, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err != nil {
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
	// The KEYS only — the display names are the body's half.
	if got, _ := issuer.claims["groups"].([]string); len(got) != 1 || got[0] != "engineering" {
		t.Errorf("groups claim = %v, want exactly [engineering]", issuer.claims["groups"])
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
	account := usableAccount()
	account.MustChangePassword = true

	store := &fakeAuthStore{
		account: account,
		matches: true,
		permissions: []vos.PermissionKey{
			{Resource: "user", Action: "read"},
			{Resource: "user", Action: "change-password"},
			{Resource: "*", Action: "*"},
		},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	result, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	perms, _ := issuer.claims["permissions"].([]string)
	if len(perms) != 1 || perms[0] != utils.PermissionChangeOwnPassword {
		t.Fatalf("permissions = %v, want exactly [%s] — a session that exists to rotate an expired credential must not be able to do anything else",
			perms, utils.PermissionChangeOwnPassword)
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
	if len(result.User.Permissions) != 1 || result.User.Permissions[0] != utils.PermissionChangeOwnPassword {
		t.Errorf("body permissions = %v, want the same restricted set the token carries", result.User.Permissions)
	}
}

// The grant is EMBEDDED, not filtered: a must-change session carries
// user:change-password whatever the bundle holds.
//
// Both bundles below would have yielded an empty claim under a filter — one
// holds an unrelated permission, the other holds only the wildcard, and neither
// spells the literal the route is gated with. An empty claim is a session that
// cannot do the one thing it exists to do, which is the deadlock this embedding
// removes. The reach stays bounded elsewhere: the route this opens is held to
// the caller's own subject, so the grant can only rotate the password of the
// account whose credential expired.
func TestIssueToken_MustChangePasswordEmbedsTheGrantWhateverTheBundleHolds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bundle []vos.PermissionKey
	}{
		{"a bundle that never confers it", []vos.PermissionKey{{Resource: "user", Action: "read"}}},
		{"the wildcard, which spells no literal", []vos.PermissionKey{{Resource: "*", Action: "*"}}},
		{"an empty bundle", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := usableAccount()
			account.MustChangePassword = true

			store := &fakeAuthStore{
				account:     account,
				matches:     true,
				permissions: tc.bundle,
			}
			issuer := &fakeIssuer{}
			h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

			result, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			perms, _ := issuer.claims["permissions"].([]string)
			if len(perms) != 1 || perms[0] != utils.PermissionChangeOwnPassword {
				t.Errorf("permissions = %v, want exactly [%s]", perms, utils.PermissionChangeOwnPassword)
			}
			// The body mirrors the token; the two must not disagree.
			if len(result.User.Permissions) != 1 || result.User.Permissions[0] != utils.PermissionChangeOwnPassword {
				t.Errorf("body permissions = %v, want the same single grant the token carries", result.User.Permissions)
			}
		})
	}
}

// ── the rotation ────────────────────────────────────────────────────────────

func TestRefreshToken_Succeeds(t *testing.T) {
	store := &fakeAuthStore{
		byID:        usableAccount(),
		roles:       namedGrants("viewer"),
		permissions: []vos.PermissionKey{{Resource: "user", Action: "read"}},
	}
	lookup := &fakeLookup{subject: "11111111-1111-1111-1111-111111111111"}
	issuer := &fakeIssuer{}
	h := &RefreshTokenHandler{Store: store, Lookup: lookup, Issuer: issuer}

	result, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: " opaque-value "})
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
	account := usableAccount()
	account.MustChangePassword = true

	store := &fakeAuthStore{
		byID: account,
		permissions: []vos.PermissionKey{
			{Resource: "user", Action: "change-password"},
			{Resource: "*", Action: "*"},
		},
	}
	issuer := &fakeIssuer{}
	h := &RefreshTokenHandler{Store: store, Lookup: &fakeLookup{subject: "s"}, Issuer: issuer}

	if _, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: "v"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	perms, _ := issuer.claims["permissions"].([]string)
	if len(perms) != 1 || perms[0] != utils.PermissionChangeOwnPassword {
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
			store := &fakeAuthStore{byID: usableAccount()}
			h := &RefreshTokenHandler{
				Store:  store,
				Lookup: &fakeLookup{subject: "s"},
				Issuer: &fakeIssuer{redeemErr: sentinel},
			}
			_, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: "v"})
			assertCredentialRefusal(t, err)
		})
	}
}

func TestRefreshToken_RefusesWithoutTouchingTheStore(t *testing.T) {
	lookup := &fakeLookup{}
	h := &RefreshTokenHandler{Store: &fakeAuthStore{}, Lookup: lookup, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: "   "})
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

	_, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: "bogus"})
	assertCredentialRefusal(t, err)
}

// A lookup FAILURE is an outage, not a utils.Refusal.
func TestRefreshToken_LookupFailureEscapes(t *testing.T) {
	boom := errors.New("store down")
	h := &RefreshTokenHandler{
		Store:  &fakeAuthStore{},
		Lookup: &fakeLookup{err: boom},
		Issuer: &fakeIssuer{},
	}
	_, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: "v"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the store failure to escape", err)
	}
}

// The account went away between rotations — archived, suspended, tenant
// withdrawn. The session ends, with the same utils.Refusal as everything else.
func TestRefreshToken_UnusableAccountEndsTheSession(t *testing.T) {
	suspended := usableAccount()
	suspended.Status = vos.UserStatusSuspended.Value()

	h := &RefreshTokenHandler{
		Store:  &fakeAuthStore{byID: suspended},
		Lookup: &fakeLookup{subject: "s"},
		Issuer: &fakeIssuer{},
	}
	_, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: "v"})
	assertCredentialRefusal(t, err)
}

// ── the projections ─────────────────────────────────────────────────────────

// EVERY ROLE CARRIES ITS DISPLAY NAME NOW, inherited ones included.
//
// This assertion is the INVERSE of the one it replaces. While the roles were
// resolved by a statement anchored on the user's own grants, an inherited role was
// never loaded as a row and the body showed a blank name for it; the test asserted
// that blank, because naming the ones we could was better than naming none. The
// grant read is anchored on `roles` itself now — every role the user holds by any
// path arrives as a row, with its key AND its name — so the blank is gone and
// asserting it would be asserting a limitation that no longer exists.
func TestBuildProfile_EveryRoleCarriesItsName(t *testing.T) {
	profile := utils.BuildProfile(usableAccount(), infra.SignInBundle{
		Roles: []infra.NamedGrant{
			{Key: "billing-admin", Name: "Billing Admin"},
			{Key: "inherited-viewer", Name: "Inherited Viewer"},
		},
	}, nil)

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
	if byKey["inherited-viewer"] != "Inherited Viewer" {
		t.Errorf("the INHERITED role lost its display name (%q) — the whole point of anchoring "+
			"the grant read on `roles` is that it has one", byKey["inherited-viewer"])
	}
}

func TestRenderPermissions_DropsHalfKeysAndNeverReturnsNil(t *testing.T) {
	got := utils.RenderPermissions([]vos.PermissionKey{
		{Resource: "user", Action: "read"},
		{Resource: "", Action: "read"},
		{Resource: "user", Action: ""},
	})
	if len(got) != 1 || got[0] != "user:read" {
		t.Errorf("got %v, want only the complete key", got)
	}
	if utils.RenderPermissions(nil) == nil {
		t.Error("nil would render as JSON null; the claim must be present and empty")
	}
}

func TestAccountIsUsable(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*schemas.SignInAccount)
		want bool
	}{
		{"active in an active tenant", func(*schemas.SignInAccount) {}, true},
		{"active in a trial tenant", func(u *schemas.SignInAccount) { u.TenantStatus = vos.TenantStatusTrial.Value() }, true},
		{"suspended account", func(u *schemas.SignInAccount) { u.Status = vos.UserStatusSuspended.Value() }, false},
		{"suspended tenant", func(u *schemas.SignInAccount) { u.TenantStatus = vos.TenantStatusSuspended.Value() }, false},
		{"tenant join produced nothing", func(u *schemas.SignInAccount) { u.TenantStatus = "" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := usableAccount()
			tc.mut(account)
			if got := utils.AccountIsUsable(account); got != tc.want {
				t.Errorf("utils.AccountIsUsable = %v, want %v", got, tc.want)
			}
		})
	}
}

// A TRIAL tenant signs in. This is the one that is easy to get wrong by reading
// "unavailable" as "not active" — and doing so would break every trial signup.
func TestIssueToken_TrialTenantSignsIn(t *testing.T) {
	account := usableAccount()
	account.TenantStatus = vos.TenantStatusTrial.Value()

	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{account: account, matches: true},
		Attempts: &fakeAttempts{},
		Issuer:   &fakeIssuer{},
	}
	if _, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err != nil {
		t.Fatalf("a trial tenant was refused: %v", err)
	}
}

// ── the lockout ─────────────────────────────────────────────────────────────

// THE ORACLE TEST, and the reason the counter is not a column on `users`. A
// locked identity is refused BEFORE anything looks it up — so no credential is
// verified, no row is read, and the answer cannot depend on whether the account
// exists.
func TestIssueToken_LockedRefusesBeforeTouchingTheCredential(t *testing.T) {
	store := &fakeAuthStore{account: usableAccount(), matches: true}
	attempts := &fakeAttempts{lockedFor: 7 * time.Minute}
	h := &IssueTokenHandler{Store: store, Attempts: attempts, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "Str0ng!Passphrase"})
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
	// Counted through RecordLocked rather than RecordFailure: a failure would move
	// the live counter and the window anchor, and letting attempts made DURING a
	// lock do that would let anyone hold an account shut indefinitely just by
	// continuing to try.
	if got := attempts.outcomes(); len(got) != 1 || got[0] != "locked" {
		t.Errorf("logged %v, want exactly one locked attempt", got)
	}
}

// The 429 carries the remaining window, because telling somebody to wait without
// saying how long is not an instruction.
func TestIssueToken_LockedAnswersWithTheRemainingMinutes(t *testing.T) {
	attempts := &fakeAttempts{lockedFor: 7*time.Minute + 10*time.Second}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

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
		Store:    &fakeAuthStore{account: usableAccount(), matches: true},
		Attempts: &fakeAttempts{lockErr: boom},
		Issuer:   &fakeIssuer{},
	}
	_, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
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
	_, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "nobody@acme.test", Password: "x"})
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
			name: "wrong password", store: &fakeAuthStore{account: usableAccount(), matches: false},
			wantOutcome: "failure", wantExisted: boolPtr(true),
		},
		{
			// A valid credential for a disabled account is still a failure —
			// nobody got in — and it counts, which is right: repeatedly presenting
			// a working password for a suspended account is exactly the pattern
			// worth rate-limiting.
			name: "suspended account", store: &fakeAuthStore{account: suspendedUser(), matches: true},
			wantOutcome: "failure", wantExisted: boolPtr(true),
		},
		{
			name: "success", store: &fakeAuthStore{account: usableAccount(), matches: true},
			wantOutcome: "success", wantExisted: boolPtr(true),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attempts := &fakeAttempts{}
			h := &IssueTokenHandler{Store: tc.store, Attempts: attempts, Issuer: &fakeIssuer{}}

			_, _ = h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

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

// The success is recorded LAST, after the token is minted. It CLEARS the failure
// counter, so recording it any earlier would release a lock for a sign-in that
// had not happened yet.
func TestIssueToken_SuccessIsLoggedOnlyAfterTheTokenExists(t *testing.T) {
	attempts := &fakeAttempts{}
	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{account: usableAccount(), matches: true},
		Attempts: attempts,
		Issuer:   &fakeIssuer{issueErr: errors.New("signing key unusable")},
	}
	if _, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err == nil {
		t.Fatal("expected the issuer failure to surface")
	}
	if len(attempts.recorded) != 0 {
		t.Errorf("counted %v — a sign-in that produced no token must not clear the counter", attempts.outcomes())
	}
}

// A grants failure records NOTHING. The caller proved who they are and this
// service Failed them; logging a failure there would let a database problem lock
// out the very users it is already failing.
func TestIssueToken_GrantsFailureRecordsNoAttempt(t *testing.T) {
	attempts := &fakeAttempts{}
	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{account: usableAccount(), matches: true, grantsErr: errors.New("boom")},
		Attempts: attempts,
		Issuer:   &fakeIssuer{},
	}
	_, _ = h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	if len(attempts.recorded) != 0 {
		t.Errorf("logged %v — an outage on our side must not count against the caller", attempts.outcomes())
	}
}

// The origin address reaches the log from the context the /auth middleware fills.
func TestIssueToken_CarriesTheOriginAddressIntoTheLog(t *testing.T) {
	ctx := authCtx()
	ctx.SetClientIP("203.0.113.7")

	attempts := &fakeAttempts{}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Issuer: &fakeIssuer{}}
	_, _ = h.Handle(ctx, &commands.IssueTokenCommand{Email: "x@y.test", Password: "z"})

	if got := attempts.recorded[0].ip; got != "203.0.113.7" {
		t.Errorf("ip = %q, want the address the middleware left on the context", got)
	}
}

// A missing address is "" and does not fail the sign-in: refusing an operation
// because a forensic field was absent would trade the operation for its own log.
func TestIssueToken_MissingOriginAddressIsNotFatal(t *testing.T) {
	attempts := &fakeAttempts{}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Issuer: &fakeIssuer{}}
	_, _ = h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "x@y.test", Password: "z"})

	if got := attempts.recorded[0].ip; got != "" {
		t.Errorf("ip = %q, want empty", got)
	}
}

func boolPtr(b bool) *bool { return &b }

func suspendedUser() *schemas.SignInAccount {
	u := usableAccount()
	u.Status = vos.UserStatusSuspended.Value()
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
	_, absentErr := absentH.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ghost@acme.test", Password: "x"})

	outage := &fakeAttempts{}
	outageH := &IssueTokenHandler{
		Store:    &fakeAuthStore{findErr: errors.New("connection reset")},
		Attempts: outage,
		Issuer:   &fakeIssuer{},
	}
	_, outageErr := outageH.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ghost@acme.test", Password: "x"})

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

// THE REASON THE EXISTENCE VERDICT IS CARRIED BACK AT ALL, asserted at the
// handler. The lockout probe reads the failure row anyway, so the verdict those
// failures established comes back for free — and it rides the ANNOUNCEMENT about
// the blocked attempt.
//
// Without it, somebody scanning the stream for locked identities — which is
// exactly how you look for accounts under attack — cannot tell a real account
// being hammered from noise against an address that does not exist here. That is
// the difference that decides how urgently anyone reacts.
//
// It does NOT go back to the table: this path performs no lookup, and the flag
// already sits on the row the causing failures wrote.
func TestIssueToken_LockedAnnouncementCarriesTheExistenceVerdict(t *testing.T) {
	real := true
	attempts := &fakeAttempts{lockedFor: 5 * time.Minute, knownToExist: &real}
	events := &fakePublisher{}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Events: events, Issuer: &fakeIssuer{}}

	_, _ = h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

	if got := attempts.outcomes(); len(got) != 1 || got[0] != "locked" {
		t.Fatalf("counted %v, want exactly one blocked attempt", got)
	}
	got, present := vals(t, events.only(t), "identityExisted")
	if !present || got != true {
		t.Errorf("identityExisted = %v (present %v), want true — a reviewer must see which locks are real accounts",
			got, present)
	}
}

// And it still claims nothing when nothing was established. An identity whose
// failures all landed during an outage carries no verdict, and the announcement
// OMITS the key rather than emitting null — so a reader can tell "we know there
// is no account" from "nobody ever found out".
func TestIssueToken_LockedAnnouncementClaimsNothingWhenNothingIsKnown(t *testing.T) {
	attempts := &fakeAttempts{lockedFor: 5 * time.Minute} // knownToExist nil
	events := &fakePublisher{}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Events: events, Issuer: &fakeIssuer{}}

	_, _ = h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

	if _, present := vals(t, events.only(t), "identityExisted"); present {
		t.Error("identityExisted was announced when nothing established it")
	}
}

// The 429 tells the caller how long to wait; the announcement tells the operator
// exactly when the lock lifts, which is what makes a support ticket answerable
// without a database session.
func TestIssueToken_LockedAnnouncementCarriesTheExpiry(t *testing.T) {
	attempts := &fakeAttempts{lockedFor: 5 * time.Minute}
	events := &fakePublisher{}
	h := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: attempts, Events: events, Issuer: &fakeIssuer{}}

	_, _ = h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

	e := events.only(t)
	if e.Type != domain.EventWarning {
		t.Errorf("severity = %v, want EventWarning — a utils.Refusal is what an operator alerts on", e.Type)
	}
	raw, present := vals(t, e, "LockedUntilFor")
	if !present {
		t.Fatal("the announcement does not say when the lock lifts")
	}
	until, ok := raw.(time.Time)
	if !ok {
		t.Fatalf("LockedUntilFor = %#v, want a time", raw)
	}
	if remaining := time.Until(until); remaining < 4*time.Minute || remaining > 6*time.Minute {
		t.Errorf("LockedUntilFor is %v away, want about 5 minutes", remaining)
	}
}

// ── the announcements ───────────────────────────────────────────────────────

// EVERY OUTCOME REACHES THE STREAM, and the severities are not uniform: a
// utils.Refusal is what an operator alerts on, a success is a routine record. This is
// the test that fails if a future branch forgets to announce itself — which
// would be invisible otherwise, since the caller's answer would not change.
func TestIssueToken_EveryOutcomeIsAnnouncedAtItsOwnSeverity(t *testing.T) {
	cases := []struct {
		name         string
		store        *fakeAuthStore
		attempts     *fakeAttempts
		wantSeverity domain.EventType
		wantMessage  string
	}{
		{"locked", &fakeAuthStore{}, &fakeAttempts{lockedFor: time.Minute},
			domain.EventWarning, "sign-in refused: identity is locked"},
		{"no such identity", &fakeAuthStore{}, &fakeAttempts{},
			domain.EventWarning, "sign-in Failed: no account for this identity"},
		{"lookup Failed", &fakeAuthStore{findErr: errors.New("connection reset")}, &fakeAttempts{},
			domain.EventWarning, "sign-in Failed: identity lookup could not be performed"},
		{"wrong password", &fakeAuthStore{account: usableAccount()}, &fakeAttempts{},
			domain.EventWarning, "sign-in Failed: credential rejected"},
		{"suspended account", &fakeAuthStore{account: suspendedUser(), matches: true}, &fakeAttempts{},
			domain.EventWarning, "sign-in Failed: credential valid but account or tenant not usable"},
		{"success", &fakeAuthStore{account: usableAccount(), matches: true}, &fakeAttempts{},
			domain.EventLog, "sign-in Succeeded"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := &fakePublisher{}
			h := &IssueTokenHandler{Store: tc.store, Attempts: tc.attempts, Events: events, Issuer: &fakeIssuer{}}
			_, _ = h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})

			e := events.only(t)
			if e.Msg != tc.wantMessage {
				t.Errorf("message = %q, want %q", e.Msg, tc.wantMessage)
			}
			if e.Type != tc.wantSeverity {
				t.Errorf("severity = %v, want %v", e.Type, tc.wantSeverity)
			}
			if e.Class != utils.EventClass {
				t.Errorf("class = %q, want %q — one filter must reach every sign-in outcome",
					e.Class, utils.EventClass)
			}
			if v, _ := vals(t, e, "identity"); v != "ada@acme.test" {
				t.Errorf("identity = %v, want the address that was tried", v)
			}
			if v, _ := vals(t, e, "identityKind"); v != utils.IdentityKindUser {
				t.Errorf("identityKind = %v, want %q", v, utils.IdentityKindUser)
			}
		})
	}
}

// THE ANSWER IS THE SAME, THE RECORD IS NOT — now asserted across the stream
// too. An unknown address and a lookup that never ran are one utils.Refusal to the
// caller and two different lines to whoever reads this later. Collapsing them
// would cost the very distinction the branch exists to preserve.
func TestIssueToken_AbsenceAndOutageAnnounceDifferently(t *testing.T) {
	absent := &fakePublisher{}
	absentH := &IssueTokenHandler{Store: &fakeAuthStore{}, Attempts: &fakeAttempts{}, Events: absent, Issuer: &fakeIssuer{}}
	_, absentErr := absentH.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ghost@acme.test", Password: "x"})

	outage := &fakePublisher{}
	outageH := &IssueTokenHandler{
		Store:    &fakeAuthStore{findErr: errors.New("connection reset")},
		Attempts: &fakeAttempts{},
		Events:   outage,
		Issuer:   &fakeIssuer{},
	}
	_, outageErr := outageH.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ghost@acme.test", Password: "x"})

	assertCredentialRefusal(t, absentErr)
	assertCredentialRefusal(t, outageErr)

	if absentMsg, outageMsg := absent.only(t).Msg, outage.only(t).Msg; absentMsg == outageMsg {
		t.Fatalf("both announced %q; a reviewer cannot tell stuffing from an outage", absentMsg)
	}
	// The unknown address KNOWS it is absent and says so; the outage established
	// nothing and omits the key rather than guessing.
	if v, present := vals(t, absent.only(t), "identityExisted"); !present || v != false {
		t.Errorf("an unknown address announced identityExisted = %v (present %v), want false", v, present)
	}
	if _, present := vals(t, outage.only(t), "identityExisted"); present {
		t.Error("an outage announced an existence verdict nobody established")
	}
}

// AN ANNOUNCEMENT THAT FAILS MUST NOT REFUSE THE SIGN-IN. The counters are the
// load-bearing half and they propagate; the record is best-effort. Turning a log
// problem into a 500 would let a full buffer refuse valid credentials.
func TestIssueToken_AFailedAnnouncementDoesNotRefuseTheSignIn(t *testing.T) {
	events := &fakePublisher{err: errors.New("stdout is gone")}
	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{account: usableAccount(), matches: true},
		Attempts: &fakeAttempts{},
		Events:   events,
		Issuer:   &fakeIssuer{},
	}
	if _, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err != nil {
		t.Fatalf("a publisher failure refused a valid sign-in: %v", err)
	}
}

// A nil publisher disables the announcements and changes nothing else — the same
// semantic the framework gives its own event port, and what lets every other test
// in this file drive the branches without one.
func TestIssueToken_NoPublisherIsNotAFailure(t *testing.T) {
	h := &IssueTokenHandler{
		Store:    &fakeAuthStore{account: usableAccount(), matches: true},
		Attempts: &fakeAttempts{},
		Issuer:   &fakeIssuer{},
	}
	if _, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err != nil {
		t.Fatalf("a nil publisher broke the sign-in: %v", err)
	}
}

// THE RULE THAT MATTERS MOST NOW THAT THIS STREAM LEAVES THE BOX: no
// announcement, on any branch, may carry the presented credential. The table was
// always protected by having no column for it; the stream has no such structural
// guard, so it gets a test instead.
func TestIssueToken_NoAnnouncementCarriesTheCredential(t *testing.T) {
	const secret = "Sup3rSecret!Passphrase"
	stores := []*fakeAuthStore{
		{},
		{findErr: errors.New("connection reset")},
		{account: usableAccount()},
		{account: suspendedUser(), matches: true},
		{account: usableAccount(), matches: true},
	}
	events := &fakePublisher{}
	for _, store := range stores {
		h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Events: events, Issuer: &fakeIssuer{}}
		_, _ = h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: secret})
	}
	if len(events.published) != len(stores) {
		t.Fatalf("announced %v, want one record per branch", events.messages())
	}
	for _, e := range events.published {
		m, ok := e.Vals.(map[string]any)
		if !ok {
			t.Fatalf("payload is %T, want map[string]any", e.Vals)
		}
		for k, v := range m {
			if str, isStr := v.(string); isStr && strings.Contains(str, secret) {
				t.Errorf("announcement %q leaked the credential under key %q", e.Msg, k)
			}
		}
		if strings.Contains(e.Msg, secret) {
			t.Errorf("the message itself leaked the credential: %q", e.Msg)
		}
	}
}

// ── the identity_kind claim ─────────────────────────────────────────────────

// THE USER TOKEN SAYS WHAT KIND OF SUBJECT IT SPEAKS FOR, on both paths.
//
// It changes no decision today — Client's row rules stand down on anything that
// is not "client" — and that is exactly why it can be minted now. What it removes
// is the inference: until this claim existed, a user token was recognised by the
// ABSENCE of it, which cannot tell a user from an issuer that forgot or from a
// token minted before the claim was introduced.
//
// The refresh is asserted too because it rebuilds claims from the database rather
// than replaying the old token's — so a claim added to the sign-in and not to
// utils.BuildClaims would silently vanish at the first rotation.
func TestBuildClaims_TokenDeclaresTheSubjectKindOnBothPaths(t *testing.T) {
	t.Run("sign-in", func(t *testing.T) {
		issuer := &fakeIssuer{}
		h := &IssueTokenHandler{
			Store:    &fakeAuthStore{account: usableAccount(), matches: true},
			Attempts: &fakeAttempts{},
			Issuer:   issuer,
		}
		if _, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := issuer.claims[utils.ClaimIdentityKind]; got != utils.IdentityKindUser {
			t.Errorf("%s = %v, want %q", utils.ClaimIdentityKind, got, utils.IdentityKindUser)
		}
	})

	t.Run("refresh", func(t *testing.T) {
		issuer := &fakeIssuer{}
		h := &RefreshTokenHandler{
			Store:  &fakeAuthStore{byID: usableAccount()},
			Lookup: &fakeLookup{subject: usableAccount().ID.Value()},
			Issuer: issuer,
		}
		if _, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: "v"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := issuer.claims[utils.ClaimIdentityKind]; got != utils.IdentityKindUser {
			t.Errorf("%s = %v, want %q — a rotation must not drop the subject kind", utils.ClaimIdentityKind, got, utils.IdentityKindUser)
		}
	})
}

// THE PRODUCER AND THE CONSUMER MUST SPELL IT THE SAME, and nothing else checks.
//
// utils.BuildClaims is the only thing that MINTS this claim; Client's runtime feeders
// READ it, by a name the omnicore-gen spec declares. Rename either side alone and
// Client's row rules go quietly inert — a client token would stop being
// recognised as one, and the rule that keeps a client inside its own row would
// stand down without a single test failing anywhere else.
func TestIdentityKindClaim_ProducerAndConsumerAgree(t *testing.T) {
	if utils.ClaimIdentityKind != clientIdentityKindClaim {
		t.Errorf("the token mints %q and Client reads %q — Client's row rules would go inert",
			utils.ClaimIdentityKind, clientIdentityKindClaim)
	}
	// Pinned against the literal the generated feeders carry inline and the
	// omnicore-gen spec declares, which no compiler checks for us.
	if utils.ClaimIdentityKind != "identity_kind" {
		t.Errorf("claim = %q, want %q — specs/omnicore-gen/client.omnicore.yaml is the source of truth",
			utils.ClaimIdentityKind, "identity_kind")
	}
}

// The vocabulary is closed and matches what the attempt table stores, so one
// value never means two things across the token, the rules and the log.
func TestIdentityKinds_AreTheTwoTheRestOfTheServiceUses(t *testing.T) {
	if utils.IdentityKindUser != "user" || utils.IdentityKindClient != "client" {
		t.Errorf("kinds = %q/%q, want user/client — authentication_attempts.identity_kind stores these",
			utils.IdentityKindUser, utils.IdentityKindClient)
	}
}

// ── the tenant claims on the two token paths ────────────────────────────────
//
// The chain itself is proven in authentication_claims_manual_test.go. What these
// assert is the WIRING around it: that the catalog is read on both paths, that
// its failure is not dressed up as a utils.Refusal, that a definition can never take
// over a platform name, and that the body and the token cannot disagree.

// A catalog failure is NOT a credential utils.Refusal, for the same reason a grants
// failure is not: the caller proved who they are and this service Failed them.
func TestIssueToken_CatalogFailureIsNotACredentialRefusal(t *testing.T) {
	boom := errors.New("claims query exploded")
	store := &fakeAuthStore{account: usableAccount(), matches: true, catalogErr: boom}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the underlying failure to escape as an exception", err)
	}
	var carrier domain.NotificationCarrier
	if errors.As(err, &carrier) {
		t.Error("a catalog failure must not be dressed up as a credential utils.Refusal")
	}
}

func TestIssueToken_MintsTheTenantClaimsBesideTheFixedSet(t *testing.T) {
	account := usableAccount()

	store := &fakeAuthStore{
		account:     account,
		matches:     true,
		claimValues: map[domain.ID]string{domain.NewID(claimID(1)): "9000"},
		definitions: []schemas.ClaimDefinition{
			definition(claimID(1), "x_cost_center", vos.ClaimValueTypeNumber, stringValue("1000")),
			definition(claimID(2), "x_region", vos.ClaimValueTypeString, stringValue("emea")),
		},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	result, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "right"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The bundle is resolved FOR THIS ACCOUNT — the reader keys the catalog on
	// account.TenantID inside the same call, so there is no separate tenant
	// argument left to get wrong.
	// Level 1 beat level 2, and level 2 filled the claim level 1 said nothing
	// about — both, on one token.
	if got := issuer.claims["x_cost_center"]; got != float64(9000) {
		t.Errorf("x_cost_center = %#v, want the user's own value", got)
	}
	if got := issuer.claims["x_region"]; got != "emea" {
		t.Errorf("x_region = %#v, want the tenant default", got)
	}
	// BESIDE, not instead of. The fixed set is what every service in the mesh
	// reads, and a merge that displaced any of it would be the worst outcome
	// this feature could have.
	if issuer.claims[utils.ClaimTenantWorkspace] != "acme" || issuer.claims[utils.ClaimEmail] != "ada@acme.test" {
		t.Errorf("the fixed claim set did not survive the merge: %#v", issuer.claims)
	}
	if result.User.Claims["x_region"] != "emea" {
		t.Errorf("the body did not carry the resolved claims: %#v", result.User.Claims)
	}
}

// The merge order, tested where it can actually be reached: utils.ResolveCustomClaims
// already refuses a platform name, so this drives utils.BuildClaims directly.
//
// The two guards fail in OPPOSITE directions, which is the whole reason both
// exist. If the first is ever wrong — a name added to the platform's vocabulary
// that an operator had already seeded — the consequence must be a tenant claim
// that quietly does not appear, never `permissions` replaced by a value the
// tenant chose.
func TestBuildClaims_TheFixedSetIsNeverDisplacedByACustomClaim(t *testing.T) {
	account := usableAccount()
	hostile := map[string]any{
		utils.ClaimPermissions: []string{"*:*"},
		utils.ClaimTenantID:    "99999999-9999-9999-9999-999999999999",
		"x_region":             "emea",
	}

	claims := utils.BuildClaims(account, infra.SignInBundle{Roles: namedGrants("viewer"), Permissions: []vos.PermissionKey{{Resource: "user", Action: "read"}}}, hostile)

	if got := claims[utils.ClaimTenantID]; got != account.TenantID.Value() {
		t.Errorf("tenant_id = %#v, want the loaded row's own tenant", got)
	}
	permissions, ok := claims[utils.ClaimPermissions].([]string)
	if !ok || len(permissions) != 1 || permissions[0] != "user:read" {
		t.Errorf("permissions = %#v, want the resolved bundle and not the injected one", claims[utils.ClaimPermissions])
	}
	// And the harmless one still rode along.
	if claims["x_region"] != "emea" {
		t.Errorf("a legitimate custom claim was lost: %#v", claims)
	}
}

// The anti-drift test, mirroring the one that already compares the body's
// permissions against the token's. A body advertising claims the token does not
// carry would have a client offering actions every request then refuses.
func TestIssueToken_BodyClaimsMirrorTheToken(t *testing.T) {
	account := usableAccount()

	store := &fakeAuthStore{
		account:     account,
		claimValues: map[domain.ID]string{domain.NewID(claimID(1)): "true"},
		matches:     true,
		definitions: []schemas.ClaimDefinition{
			definition(claimID(1), "x_beta_enabled", vos.ClaimValueTypeBool, nil),
			definition(claimID(2), "x_region", vos.ClaimValueTypeString, stringValue("emea")),
		},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	result, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "right"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.User.Claims) != 2 {
		t.Fatalf("body claims = %#v, want both resolved claims", result.User.Claims)
	}
	for name, value := range result.User.Claims {
		if issuer.claims[name] != value {
			t.Errorf("body claim %s = %#v, token has %#v — the two must not disagree",
				name, value, issuer.claims[name])
		}
	}
}

// The case where body and token are MOST likely to disagree, because the
// restriction applies to one of them for a reason the other does not share.
func TestIssueToken_BodyAndTokenAgreeOnARestrictedSession(t *testing.T) {
	account := usableAccount()
	account.MustChangePassword = true

	store := &fakeAuthStore{
		account:     account,
		claimValues: map[domain.ID]string{domain.NewID(claimID(1)): "emea"},
		matches:     true,
		definitions: []schemas.ClaimDefinition{
			definition(claimID(1), "x_region", vos.ClaimValueTypeString, nil),
		},
	}
	issuer := &fakeIssuer{}
	h := &IssueTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	result, err := h.Handle(authCtx(), &commands.IssueTokenCommand{Email: "ada@acme.test", Password: "right"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := issuer.claims["x_region"]; present {
		t.Errorf("a restricted token carried a tenant claim: %#v", issuer.claims)
	}
	if len(result.User.Claims) != 0 {
		t.Errorf("the body advertised claims the token does not carry: %#v", result.User.Claims)
	}
}

// A corrected value has to propagate at the next rotation, which it only can if
// the catalog is RE-READ rather than replayed from the token being redeemed.
func TestRefreshToken_RebuildsTheTenantClaims(t *testing.T) {
	account := usableAccount()

	store := &fakeAuthStore{
		byID:        account,
		claimValues: map[domain.ID]string{domain.NewID(claimID(1)): "9000"},
		definitions: []schemas.ClaimDefinition{definition(claimID(1), "x_cost_center", vos.ClaimValueTypeNumber, nil)},
	}
	lookup := &fakeLookup{subject: "11111111-1111-1111-1111-111111111111"}
	issuer := &fakeIssuer{}
	h := &RefreshTokenHandler{Store: store, Lookup: lookup, Issuer: issuer}

	result, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: "opaque-value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.catalogReads != 1 {
		t.Errorf("catalog reads = %d, want exactly one per rotation", store.catalogReads)
	}
	if got := issuer.claims["x_cost_center"]; got != float64(9000) {
		t.Errorf("x_cost_center = %#v, want it rebuilt from the database", got)
	}
	if result.User.Claims["x_cost_center"] != float64(9000) {
		t.Errorf("the rotation's body lost the claims: %#v", result.User.Claims)
	}
}

// A rotation that refuses must not have paid for the catalog either.
func TestRefreshToken_CatalogFailureIsNotACredentialRefusal(t *testing.T) {
	boom := errors.New("claims query exploded")
	store := &fakeAuthStore{byID: usableAccount(), catalogErr: boom}
	lookup := &fakeLookup{subject: "11111111-1111-1111-1111-111111111111"}
	h := &RefreshTokenHandler{Store: store, Lookup: lookup, Issuer: &fakeIssuer{}}

	_, err := h.Handle(authCtx(), &commands.RefreshTokenCommand{RefreshToken: "opaque-value"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the underlying failure to escape as an exception", err)
	}
	var carrier domain.NotificationCarrier
	if errors.As(err, &carrier) {
		t.Error("a catalog failure must not be dressed up as a credential utils.Refusal")
	}
}

// claimID builds the nth distinct definition id.
//
// A twin of the helper in the utils suite, and deliberately a copy rather than an
// export: it is a two-line test fixture, and widening a package's public surface so
// another package's tests can borrow one is the wrong trade. The utils suite proves
// the chain; this one proves the HANDLER hands it the right inputs and mirrors its
// answer into the body.
func claimID(n int) string {
	return fmt.Sprintf("33333333-3333-3333-3333-%012d", n)
}

// definition builds one catalog row. The id is what an entry points at, so it
// is always set: a definition with none can match nothing.
func definition(id, name string, valueType vos.ClaimValueType, def *string) schemas.ClaimDefinition {
	return schemas.ClaimDefinition{
		ID:           domain.NewID(id),
		Name:         name,
		ValueType:    valueType.Value(),
		AppliesTo:    vos.ClaimAppliesToUser.Value(),
		DefaultValue: def,
	}
}

func stringValue(v string) *string { return &v }
