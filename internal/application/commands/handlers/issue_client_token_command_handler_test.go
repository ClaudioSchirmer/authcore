// Tests for the machine sign-in.
//
// WHAT THIS FILE PROTECTS, in order of how badly it would hurt to lose it:
//
//  1. THE CLAIM SET IS EXACTLY SEVEN NAMES, and three specific ones are ABSENT. A
//     token that grew an `email` or a `groups` claim would be a lie about a machine;
//     one that lost `identity_kind` would silently disarm Client's row rules, which
//     read it to tell a machine caller from a person.
//  2. THE BODY AND THE TOKEN CANNOT DISAGREE. One resolution per request feeds
//     both — the user route shipped this bug once and its test is what caught it.
//  3. EVERY REFUSAL IS THE SAME 401, on the same neutral field name, including the
//     one somebody will want to split out: the allow-list.
//  4. THE LOCKOUT IS NOT CONSULTED HERE. That is a decision (see the handler's
//     header), so a future change that "restores consistency" with the user route
//     has to break a test that says why.
//
// It reuses fakeAttempts, fakePublisher, authCtx, vals and namedGrants from
// authentication_commands_manual_test.go: the two routes share the journal, so
// sharing its doubles is what keeps them asserting the same thing about it.

package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// ── the doubles ─────────────────────────────────────────────────────────────

type fakeClientAuthStore struct {
	client  *schemas.SignInClient
	findErr error

	matches   bool
	burned    int
	bundleErr error

	roles       []infra.NamedGrant
	permissions []vos.PermissionKey
	claimValues map[domain.ID]string
	definitions []schemas.ClaimDefinition
	allowed     []string

	resolves int
}

func (s *fakeClientAuthStore) LoadClientByID(context.Context, domain.ID) (*schemas.SignInClient, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	return s.client, nil
}

func (s *fakeClientAuthStore) ResolveClientSignIn(context.Context, *schemas.SignInClient) (infra.ClientSignInBundle, error) {
	if s.bundleErr != nil {
		return infra.ClientSignInBundle{}, s.bundleErr
	}
	s.resolves++
	return infra.ClientSignInBundle{
		Roles:        s.roles,
		Permissions:  s.permissions,
		ClaimValues:  s.claimValues,
		Definitions:  s.definitions,
		AllowedCIDRs: s.allowed,
	}, nil
}

func (s *fakeClientAuthStore) SecretMatches(string, *schemas.SignInClient) bool { return s.matches }
func (s *fakeClientAuthStore) BurnSecretVerification()                          { s.burned++ }

// fakeAccessIssuer stands in for the framework's Issuer, narrowed to the ONE method
// this route calls. That it needs no refresh method is the port doing its job: a
// double that cannot mint a refresh token cannot accidentally be asked for one.
type fakeAccessIssuer struct {
	request authcore.TokenRequest
	err     error
	calls   int
}

func (i *fakeAccessIssuer) Issue(_ context.Context, req authcore.TokenRequest) (authcore.IssuedToken, error) {
	i.calls++
	i.request = req
	if i.err != nil {
		return authcore.IssuedToken{}, i.err
	}
	return authcore.IssuedToken{Token: "client-access-token", ExpiresAt: time.Unix(5000, 0)}, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

const testClientID = "33333333-3333-3333-3333-333333333333"

// usableClient builds a loaded, signable integration: active, in an active tenant,
// with the joined tenant columns the read joins fill on every real load.
func usableSignInClient() *schemas.SignInClient {
	return &schemas.SignInClient{
		ID:              domain.NewID(testClientID),
		TenantID:        domain.NewID("22222222-2222-2222-2222-222222222222"),
		Name:            "Billing integration",
		Status:          vos.ClientStatusActive.Value(),
		SecretHash:      "sha256-stored",
		TenantWorkspace: "acme",
		TenantStatus:    vos.TenantStatusActive.Value(),
	}
}

func clientCtxFrom(ip string) *configuration.AppContext {
	ctx := authCtx()
	ctx.SetClientIP(ip)
	return ctx
}

func signableClientAuthStore() *fakeClientAuthStore {
	return &fakeClientAuthStore{
		client:      usableSignInClient(),
		matches:     true,
		roles:       namedGrants("billing-admin"),
		permissions: []vos.PermissionKey{{Resource: "user", Action: "read"}},
	}
}

// assertClientCredentialRefusal fails unless err is EXACTLY this route's utils.Refusal.
//
// The field name is asserted for the reason its user twin asserts it: a utils.Refusal
// keyed on "clientId" or "clientSecret" would say which half was wrong.
func assertClientCredentialRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a utils.Refusal, got nil")
	}
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		t.Fatalf("expected a NotificationCarrier, got %T: %v", err, err)
	}
	var domainErr *domain.DomainError
	if errors.As(err, &domainErr) {
		t.Errorf("utils.Refusal arrived as a domain error (%T); this endpoint decides in the application layer", err)
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
	if _, ok := msgs[0].Notification.(InvalidClientCredentialsNotification); !ok {
		t.Errorf("notification = %T, want InvalidClientCredentialsNotification", msgs[0].Notification)
	}
	if msgs[0].Override != "credentials" {
		t.Errorf("field = %q, want %q — a field name that says which half was wrong is an oracle",
			msgs[0].Override, "credentials")
	}
}

// ── the happy path ──────────────────────────────────────────────────────────

func TestIssueClientToken_Succeeds(t *testing.T) {
	store := signableClientAuthStore()
	issuer := &fakeAccessIssuer{}
	attempts := &fakeAttempts{}
	h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Issuer: issuer}

	result, err := h.Handle(clientCtxFrom("203.0.113.7"),
		&commands.IssueClientTokenCommand{ClientID: testClientID, ClientSecret: "acs_right"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken != "client-access-token" {
		t.Errorf("token not carried through: %+v", result)
	}
	if result.TokenType != "Bearer" {
		t.Errorf("tokenType = %q, want Bearer", result.TokenType)
	}
	if result.ExpiresAt != 5000 {
		t.Errorf("expiry not carried through: %d", result.ExpiresAt)
	}
	if store.burned != 0 {
		t.Errorf("a successful sign-in burned %d verifications; the burn is only for paths that never reach a real hash", store.burned)
	}
	if got := attempts.outcomes(); len(got) != 1 || got[0] != "success" {
		t.Errorf("outcomes = %v, want exactly one success", got)
	}
	if attempts.recorded[0].kind != "client" {
		t.Errorf("recorded kind = %q, want %q — a machine attempt on a user's counter row is a shared lockout",
			attempts.recorded[0].kind, "client")
	}
	if attempts.recorded[0].identity != testClientID {
		t.Errorf("recorded identity = %q, want the client id", attempts.recorded[0].identity)
	}
	if attempts.recorded[0].ip != "203.0.113.7" {
		t.Errorf("recorded ip = %q, want the origin address", attempts.recorded[0].ip)
	}
}

// THE SUBJECT IS THE CLIENT ID, and the TTL is left for the Issuer to apply.
//
// A TTL set here would silently override auth.issuer.tokenTtlSeconds for machines
// only — which is a policy decision, and the plan records that it was taken the
// other way. Asserting the zero value is what makes a later change deliberate.
func TestIssueClientToken_SubjectIsTheClientAndTheTTLIsTheIssuersToChoose(t *testing.T) {
	issuer := &fakeAccessIssuer{}
	h := &IssueClientTokenHandler{Store: signableClientAuthStore(), Attempts: &fakeAttempts{}, Issuer: issuer}

	if _, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issuer.request.Subject != testClientID {
		t.Errorf("subject = %q, want the client id", issuer.request.Subject)
	}
	if issuer.request.TTL != 0 {
		t.Errorf("TTL = %v, want 0 — the lifetime is auth.issuer.tokenTtlSeconds, not this route's to pick",
			issuer.request.TTL)
	}
}

// THE CLAIM SET, name by name — the assertion this whole file exists for.
func TestIssueClientToken_MintsExactlyTheClientClaimSet(t *testing.T) {
	store := signableClientAuthStore()
	store.roles = namedGrants("billing-admin", "reader")
	store.permissions = []vos.PermissionKey{
		{Resource: "user", Action: "read"},
		{Resource: "client", Action: "insert"},
	}
	issuer := &fakeAccessIssuer{}
	h := &IssueClientTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	if _, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	claims := issuer.request.Claims

	// PRESENT, and each carrying the value a consumer reads it for.
	if claims["identity_kind"] != "client" {
		t.Errorf("identity_kind = %v, want \"client\" — Client's row rules read this to tell a machine from a person",
			claims["identity_kind"])
	}
	if claims["tenant_id"] != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("tenant_id = %v; the framework's isolation filter compares this exact value", claims["tenant_id"])
	}
	if claims["tenant_workspace"] != "acme" {
		t.Errorf("tenant_workspace = %v; no other service can resolve this service's tenant UUID", claims["tenant_workspace"])
	}
	if claims["name"] != "Billing integration" {
		t.Errorf("name = %v, want the client's label — it is the audit trail's analogue of a person's e-mail", claims["name"])
	}
	perms, _ := claims["permissions"].([]string)
	if len(perms) != 2 || perms[0] != "user:read" || perms[1] != "client:insert" {
		t.Errorf("permissions = %v, want the rendered resource:action pairs", claims["permissions"])
	}
	roles, _ := claims["roles"].([]string)
	if len(roles) != 2 || roles[0] != "billing-admin" || roles[1] != "reader" {
		t.Errorf("roles = %v, want stable keys and never display names", claims["roles"])
	}

	// ABSENT, and each for its own reason.
	for claim, why := range map[string]string{
		"email":                "a machine has no address",
		"groups":               "Client has no groups, so an empty list would say groups are a thing it could belong to",
		"must_change_password": "there is no credential a machine can be told to rotate itself",
	} {
		if _, present := claims[claim]; present {
			t.Errorf("the token carries %q (= %v); it must be absent: %s", claim, claims[claim], why)
		}
	}

	if len(claims) != 6 {
		t.Errorf("the token carries %d claims (%v); the fixed set is six beside the Issuer's own `sub`",
			len(claims), claims)
	}
}

// The tenant's own claims reach a client token, which is the whole reason
// ClientClaim exists.
func TestIssueClientToken_ResolvesTheTenantClaimChain(t *testing.T) {
	defID := domain.NewID("44444444-4444-4444-4444-444444444444")
	defaultedID := domain.NewID("55555555-5555-5555-5555-555555555555")
	fallback := "eu-west-1"

	store := signableClientAuthStore()
	store.claimValues = map[domain.ID]string{defID: "sa-east-1"}
	store.definitions = []schemas.ClaimDefinition{
		{ID: defID, Name: "x_region", ValueType: vos.ClaimValueTypeString.Value()},
		{ID: defaultedID, Name: "x_zone", ValueType: vos.ClaimValueTypeString.Value(), DefaultValue: &fallback},
	}
	issuer := &fakeAccessIssuer{}
	h := &IssueClientTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	result, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Level 1 wins where the client holds a value; level 2 fills the hole.
	if issuer.request.Claims["x_region"] != "sa-east-1" {
		t.Errorf("x_region = %v, want the value set on this client", issuer.request.Claims["x_region"])
	}
	if issuer.request.Claims["x_zone"] != "eu-west-1" {
		t.Errorf("x_zone = %v, want the definition's tenant-wide default", issuer.request.Claims["x_zone"])
	}

	// AND THE BODY MIRRORS THE TOKEN EXACTLY. One resolution, two readers.
	if result.Client.Claims["x_region"] != "sa-east-1" || result.Client.Claims["x_zone"] != "eu-west-1" {
		t.Errorf("the body advertises claims the token does not carry: %v", result.Client.Claims)
	}
}

// The profile a caller renders, and the anti-drift property under it.
func TestIssueClientToken_BodyMirrorsTheToken(t *testing.T) {
	store := signableClientAuthStore()
	issuer := &fakeAccessIssuer{}
	h := &IssueClientTokenHandler{Store: store, Attempts: &fakeAttempts{}, Issuer: issuer}

	result, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Client.ID != testClientID || result.Client.Name != "Billing integration" {
		t.Errorf("profile identity is wrong: %+v", result.Client)
	}
	if result.Client.Status != vos.ClientStatusActive.Value() {
		t.Errorf("status = %q, want active — a suspended client cannot reach this response", result.Client.Status)
	}
	if result.Client.TenantWorkspace != "acme" {
		t.Errorf("tenantWorkspace = %q", result.Client.TenantWorkspace)
	}
	if len(result.Client.Roles) != 1 || result.Client.Roles[0].Name != "billing-admin" {
		t.Errorf("the body lost the role display names the token deliberately omits: %+v", result.Client.Roles)
	}

	claimed, _ := issuer.request.Claims["permissions"].([]string)
	if len(claimed) != len(result.Client.Permissions) {
		t.Fatalf("body advertises %v while the token carries %v", result.Client.Permissions, claimed)
	}
	for i := range claimed {
		if claimed[i] != result.Client.Permissions[i] {
			t.Errorf("body advertises %v while the token carries %v", result.Client.Permissions, claimed)
			break
		}
	}
}

// ── the refusals ────────────────────────────────────────────────────────────

// No such client: burn a verification, refuse, and RECORD that the id is provably
// absent.
func TestIssueClientToken_RefusesAnUnknownClient(t *testing.T) {
	store := &fakeClientAuthStore{client: nil}
	attempts := &fakeAttempts{}
	publisher := &fakePublisher{}
	issuer := &fakeAccessIssuer{}
	h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Events: publisher, Issuer: issuer}

	_, err := h.Handle(clientCtxFrom("203.0.113.7"), &commands.IssueClientTokenCommand{ClientID: testClientID})
	assertClientCredentialRefusal(t, err)

	if store.burned != 1 {
		t.Errorf("burned %d verifications, want 1 — without it the response time says whether the id exists", store.burned)
	}
	if issuer.calls != 0 {
		t.Error("a token was minted for a client that does not exist")
	}
	if len(attempts.recorded) != 1 || attempts.recorded[0].outcome != "failure" {
		t.Fatalf("outcomes = %v, want one failure", attempts.outcomes())
	}
	existed := attempts.recorded[0].existed
	if existed == nil || *existed {
		t.Errorf("identityExisted = %v, want false — the store established the id is not here", existed)
	}
	if msg := publisher.only(t).Msg; msg != "client sign-in Failed: no client for this identity" {
		t.Errorf("announced %q", msg)
	}
}

// A lookup that FAILED established nothing, and the record must say so rather than
// claim the id is absent.
func TestIssueClientToken_RecordsNothingKnownWhenTheLookupFails(t *testing.T) {
	store := &fakeClientAuthStore{findErr: errors.New("connection reset")}
	attempts := &fakeAttempts{}
	publisher := &fakePublisher{}
	h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Events: publisher, Issuer: &fakeAccessIssuer{}}

	_, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID})
	// STILL A 401, not a 500: a status code that changed with the failure would let
	// a caller learn an id exists by finding an input that changes it.
	assertClientCredentialRefusal(t, err)

	if got := attempts.recorded[0].existed; got != nil {
		t.Errorf("identityExisted = %v, want nil — a Failed lookup established nothing, and false would be a recorded falsehood", *got)
	}
	if _, present := vals(t, publisher.only(t), "identityExisted"); present {
		t.Error("the announcement carried identityExisted; nobody found out, so the key must be absent rather than false")
	}
}

func TestIssueClientToken_RefusesAWrongSecret(t *testing.T) {
	store := signableClientAuthStore()
	store.matches = false
	attempts := &fakeAttempts{}
	publisher := &fakePublisher{}
	issuer := &fakeAccessIssuer{}
	h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Events: publisher, Issuer: issuer}

	_, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID, ClientSecret: "acs_wrong"})
	assertClientCredentialRefusal(t, err)

	if issuer.calls != 0 {
		t.Error("a token was minted for a wrong secret")
	}
	if existed := attempts.recorded[0].existed; existed == nil || !*existed {
		t.Errorf("identityExisted = %v, want true — the row was found", existed)
	}
	if msg := publisher.only(t).Msg; msg != "client sign-in Failed: credential rejected" {
		t.Errorf("announced %q", msg)
	}
}

// A suspended client, and a client whose tenant is suspended, are the branches where
// the credential was RIGHT and the answer is still a utils.Refusal.
func TestIssueClientToken_RefusesAnUnusableClientOrTenant(t *testing.T) {
	for name, mutate := range map[string]func(*schemas.SignInClient){
		"suspended client": func(c *schemas.SignInClient) { c.Status = vos.ClientStatusSuspended.Value() },
		"suspended tenant": func(c *schemas.SignInClient) { c.TenantStatus = vos.TenantStatusSuspended.Value() },
		"tenant join found nothing": func(c *schemas.SignInClient) {
			// Impossible over an INNER join on a NOT NULL key — asserted because
			// "unusable" is the fail-closed reading if it ever happens.
			c.TenantStatus = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := signableClientAuthStore()
			mutate(store.client)
			attempts := &fakeAttempts{}
			publisher := &fakePublisher{}
			issuer := &fakeAccessIssuer{}
			h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Events: publisher, Issuer: issuer}

			_, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID})
			assertClientCredentialRefusal(t, err)

			if issuer.calls != 0 {
				t.Error("a token was minted for an unusable client")
			}
			if got := attempts.outcomes(); len(got) != 1 || got[0] != "failure" {
				t.Errorf("outcomes = %v; nobody got in, so it is a failure", got)
			}
			if msg := publisher.only(t).Msg; msg != "client sign-in Failed: credential valid but client or tenant not usable" {
				t.Errorf("announced %q", msg)
			}
		})
	}
}

// A store failure resolving the bundle is NOT a credential utils.Refusal and NOT an
// attempt worth counting.
//
// The caller proved who they are and this service Failed them. Recording a failure
// here would let a database problem count against the very integrations it is
// already failing.
func TestIssueClientToken_BundleFailureEscapesAsAnException(t *testing.T) {
	store := signableClientAuthStore()
	store.bundleErr = errors.New("pool exhausted")
	attempts := &fakeAttempts{}
	h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Issuer: &fakeAccessIssuer{}}

	_, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID})
	if err == nil {
		t.Fatal("expected the store failure to escape")
	}
	var carrier domain.NotificationCarrier
	if errors.As(err, &carrier) {
		t.Error("a store failure was rendered as a credential utils.Refusal; it must be a 500")
	}
	if len(attempts.recorded) != 0 {
		t.Errorf("recorded %v; a database problem must not count against the client", attempts.outcomes())
	}
}

// ── the allow-list ──────────────────────────────────────────────────────────

func TestIssueClientToken_HonoursTheAllowList(t *testing.T) {
	for name, tc := range map[string]struct {
		ip      string
		allowed []string
		wantOK  bool
	}{
		"empty collection admits any address":  {"203.0.113.7", nil, true},
		"inside a declared range":              {"203.0.113.7", []string{"203.0.113.0/24"}, true},
		"outside every declared range":         {"198.51.100.7", []string{"203.0.113.0/24"}, false},
		"no usable origin under a restriction": {"", []string{"203.0.113.0/24"}, false},
		"no usable origin, no restriction":     {"", nil, true},
	} {
		t.Run(name, func(t *testing.T) {
			store := signableClientAuthStore()
			store.allowed = tc.allowed
			attempts := &fakeAttempts{}
			publisher := &fakePublisher{}
			issuer := &fakeAccessIssuer{}
			h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Events: publisher, Issuer: issuer}

			_, err := h.Handle(clientCtxFrom(tc.ip), &commands.IssueClientTokenCommand{ClientID: testClientID})

			if tc.wantOK {
				if err != nil {
					t.Fatalf("unexpected utils.Refusal: %v", err)
				}
				if issuer.calls != 1 {
					t.Errorf("minted %d tokens, want 1", issuer.calls)
				}
				return
			}

			assertClientCredentialRefusal(t, err)
			if issuer.calls != 0 {
				t.Error("a token was minted for a disallowed address")
			}
			// THE REASON IS ON THE STREAM AND NOWHERE ELSE. Telling the caller that
			// only the NETWORK was wrong confirms the secret is good.
			if msg := publisher.only(t).Msg; msg != "client sign-in Failed: origin address outside the client's allowed ranges" {
				t.Errorf("announced %q; the operator whose egress address changed reads this line", msg)
			}
		})
	}
}

// The allow-list is checked AFTER the secret, and that ordering is the property.
//
// A wrong secret from a disallowed address must report the CREDENTIAL, not the
// network — otherwise the utils.Refusal reason itself tells an attacker their secret was
// right whenever it says "network".
func TestIssueClientToken_ChecksTheSecretBeforeTheAllowList(t *testing.T) {
	store := signableClientAuthStore()
	store.matches = false
	store.allowed = []string{"203.0.113.0/24"}
	publisher := &fakePublisher{}
	h := &IssueClientTokenHandler{Store: store, Attempts: &fakeAttempts{}, Events: publisher, Issuer: &fakeAccessIssuer{}}

	_, err := h.Handle(clientCtxFrom("198.51.100.7"), &commands.IssueClientTokenCommand{ClientID: testClientID})
	assertClientCredentialRefusal(t, err)

	if msg := publisher.only(t).Msg; msg != "client sign-in Failed: credential rejected" {
		t.Errorf("announced %q; a wrong secret must be reported as a wrong secret whatever the address", msg)
	}
	if store.resolves != 0 {
		t.Error("the grant burst ran for a wrong secret; nothing should be resolved before the credential verifies")
	}
}

// ── the lockout is NOT consulted here ───────────────────────────────────────

// A locked counter row must NOT refuse a machine sign-in.
//
// THIS TEST IS THE DECISION, not an implementation detail. The lockout makes
// guessing a ~30-bit password expensive; a client secret is 32 bytes from
// crypto/rand, so the lock buys nothing against guessing and hands anyone who knows
// a client id — which is public, it is the `sub` of every token that client
// presents — a five-request outage against a production integration. See the
// handler's header; it overrides the line specs/scaffold-entity/client/spec.md §F
// wrote.
func TestIssueClientToken_IsNotRefusedByTheLockout(t *testing.T) {
	store := signableClientAuthStore()
	attempts := &fakeAttempts{lockedFor: time.Hour}
	issuer := &fakeAccessIssuer{}
	h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Issuer: issuer}

	result, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID})
	if err != nil {
		t.Fatalf("a locked counter refused a valid machine credential: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("no token was minted")
	}
	for _, o := range attempts.outcomes() {
		if o == "locked" {
			t.Error("the route recorded a `locked` outcome; it never consults the lock, so it can never refuse for it")
		}
	}
}

// Outcomes are still COUNTED, which is what keeps the forensic and alerting surface
// whole even though nothing refuses for it.
func TestIssueClientToken_StillCountsEveryOutcome(t *testing.T) {
	store := signableClientAuthStore()
	store.matches = false
	attempts := &fakeAttempts{}
	h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Issuer: &fakeAccessIssuer{}}

	for range 3 {
		if _, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID}); err == nil {
			t.Fatal("expected a utils.Refusal")
		}
	}
	if got := attempts.outcomes(); len(got) != 3 {
		t.Errorf("outcomes = %v, want three failures — not locking is not the same as not counting", got)
	}
}

// A counter write that fails REFUSES the request rather than proceeding.
//
// The lockout's evidence is the reason: a sign-in that could not be recorded has no
// business being recorded as having happened, and the user route holds the same line.
func TestIssueClientToken_CounterFailureStopsTheRequest(t *testing.T) {
	store := signableClientAuthStore()
	attempts := &fakeAttempts{recordErr: errors.New("counter unavailable")}
	h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Issuer: &fakeAccessIssuer{}}

	_, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID})
	if err == nil {
		t.Fatal("a Failed counter write was swallowed")
	}
	var carrier domain.NotificationCarrier
	if errors.As(err, &carrier) {
		t.Error("a counter failure was rendered as a credential utils.Refusal; it must be a 500")
	}
}

// A nil publisher disables the announcements and changes nothing else.
func TestIssueClientToken_WorksWithoutAPublisher(t *testing.T) {
	h := &IssueClientTokenHandler{Store: signableClientAuthStore(), Attempts: &fakeAttempts{}, Issuer: &fakeAccessIssuer{}}

	if _, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID}); err != nil {
		t.Fatalf("a nil publisher broke the sign-in: %v", err)
	}
}

// The client id is trimmed but NEVER lowercased.
//
// It is a UUID, and the attempt log counts by this exact string. The user route
// lowercases because vos.Email stores addresses that way; doing it here would invent
// a second spelling of an identifier that has only one.
func TestIssueClientToken_TrimsButDoesNotLowercaseTheClientID(t *testing.T) {
	mixed := "33333333-AAAA-3333-3333-333333333333"
	store := signableClientAuthStore()
	store.client = nil
	attempts := &fakeAttempts{}
	h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Issuer: &fakeAccessIssuer{}}

	_, _ = h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: "  " + mixed + "  "})

	if got := attempts.recorded[0].identity; got != mixed {
		t.Errorf("recorded identity = %q, want %q — trimmed, case untouched", got, mixed)
	}
}

// EVERY utils.Refusal branch stops on a Failed counter write, not just the success one.
//
// The asymmetry with the announcement is the design: an announcement that fails
// costs a log line and is swallowed, a counter that fails costs the record its
// evidence and must refuse. Asserted per branch because each writes through its own
// call site, and a branch that dropped the error would refuse with a 401 while
// having recorded nothing — the one combination that is invisible in a green build.
func TestIssueClientToken_CounterFailureStopsEveryRefusalBranch(t *testing.T) {
	for name, arrange := range map[string]func(*fakeClientAuthStore){
		"unknown client":    func(s *fakeClientAuthStore) { s.client = nil },
		"lookup Failed":     func(s *fakeClientAuthStore) { s.findErr = errors.New("connection reset") },
		"wrong secret":      func(s *fakeClientAuthStore) { s.matches = false },
		"unusable client":   func(s *fakeClientAuthStore) { s.client.Status = vos.ClientStatusSuspended.Value() },
		"disallowed origin": func(s *fakeClientAuthStore) { s.allowed = []string{"203.0.113.0/24"} },
	} {
		t.Run(name, func(t *testing.T) {
			store := signableClientAuthStore()
			arrange(store)
			attempts := &fakeAttempts{recordErr: errors.New("counter unavailable")}
			h := &IssueClientTokenHandler{Store: store, Attempts: attempts, Issuer: &fakeAccessIssuer{}}

			_, err := h.Handle(clientCtxFrom("198.51.100.7"), &commands.IssueClientTokenCommand{ClientID: testClientID})
			if err == nil {
				t.Fatal("a Failed counter write was swallowed")
			}
			var carrier domain.NotificationCarrier
			if errors.As(err, &carrier) {
				t.Error("the counter failure was rendered as a credential utils.Refusal; it must be a 500")
			}
		})
	}
}

// A minting failure is NOT a credential utils.Refusal.
//
// The caller proved who they are and the signing path Failed them. A 401 here would
// tell an integration holding a perfectly good secret that it was rejected, and bury
// a key problem inside a credential one.
func TestIssueClientToken_IssuerFailureEscapesAsAnException(t *testing.T) {
	attempts := &fakeAttempts{}
	h := &IssueClientTokenHandler{
		Store:    signableClientAuthStore(),
		Attempts: attempts,
		Issuer:   &fakeAccessIssuer{err: errors.New("no current signing key")},
	}

	_, err := h.Handle(clientCtxFrom(""), &commands.IssueClientTokenCommand{ClientID: testClientID})
	if err == nil {
		t.Fatal("expected the minting failure to escape")
	}
	var carrier domain.NotificationCarrier
	if errors.As(err, &carrier) {
		t.Error("a minting failure was rendered as a credential utils.Refusal; it must be a 500")
	}
	if len(attempts.recorded) != 0 {
		t.Errorf("recorded %v; the success is written only once a token actually exists", attempts.outcomes())
	}
}
