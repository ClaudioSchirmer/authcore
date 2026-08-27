// Tests for client_rules_manual.go — the invariants the spec declared as ones
// the DSL could not express, which is exactly why no generated test stands
// behind them.
//
// They follow user_rules_manual_test.go: a probing service that answers
// "nothing found" like the generated stub except where a case overrides one
// question, and that RECORDS what it was asked — which is how the ordering case
// below proves a wildcard never reaches the escalation probe, and how the
// cross-tenant case proves WHICH tenant was compared.

package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

const (
	clientRoleID      = "0198f3e0-1a44-7bb2-9c31-77c0d5e1b904"
	clientOwnTenant   = "0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"
	clientOtherTenant = "0198f4aa-1111-7c9e-9f2a-6d3b1e77a410"
	clientRowID       = "7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"
	someOtherClientID = "1f6e6ac6-2a1e-4c22-9c0a-2b7a9c5f21d4"
)

// probingClientService records every question and answers "nothing wrong" by
// default — the posture the generated stub takes, which is what lets a valid
// fixture through and makes each negative case fail for the rule it is testing.
type probingClientService struct {
	domain.ServiceBase

	nameTaken         bool
	tenantUnavailable bool
	roleUnavailable   bool
	roleWildcard      bool
	lacksRolePerm     bool

	askedTenant       int
	askedRoleAvail    []clientScopedQuestion
	askedRoleWildcard []domain.ID
	askedRoleEscalate []domain.ID
	hashedSecrets     []string
}

// clientScopedQuestion records BOTH arguments, because WHICH tenant the rule
// passes is itself a security property — see the super-admin case below.
type clientScopedQuestion struct {
	tenantID domain.ID
	targetID domain.ID
}

func (s *probingClientService) NameTaken(domain.ID, string, domain.ID) bool { return s.nameTaken }

func (s *probingClientService) HashSecret(secret string) string {
	s.hashedSecrets = append(s.hashedSecrets, secret)
	return "sha256:" + secret
}

func (s *probingClientService) TenantIsUnavailable(domain.ID) bool {
	s.askedTenant++
	return s.tenantUnavailable
}

func (s *probingClientService) RoleIsUnavailableInTenant(tenantID, roleID domain.ID) bool {
	s.askedRoleAvail = append(s.askedRoleAvail, clientScopedQuestion{tenantID, roleID})
	return s.roleUnavailable
}

func (s *probingClientService) RoleGrantsWildcard(id domain.ID) bool {
	s.askedRoleWildcard = append(s.askedRoleWildcard, id)
	return s.roleWildcard
}

func (s *probingClientService) CallerLacksAnyPermissionOfRole(id domain.ID) bool {
	s.askedRoleEscalate = append(s.askedRoleEscalate, id)
	return s.lacksRolePerm
}

// clientNotificationKeys reads what the aggregate itself recorded — the seat the
// rules report through. The type NAME is what is asserted, because that name IS
// the translation key, so the assertion is about the answer the caller receives.
func clientNotificationKeys(e *Client) []string {
	var keys []string
	for _, m := range e.GetAggregateRoot().NotificationContext().Messages() {
		keys = append(keys, strings.TrimPrefix(clientTypeName(m.Notification), "*"))
	}
	return keys
}

func clientTypeName(n domain.Notification) string {
	if n == nil {
		return "<nil>"
	}
	t := reflect.TypeOf(n)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

func clientHasKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}

// insertClient runs the insert-side rules and returns the service so a case can
// assert what was asked.
func insertClient(t *testing.T, e *Client, svc *probingClientService) *probingClientService {
	t.Helper()
	_, _ = domain.GetInsertable(e, svc, "Insert")
	return svc
}

func updateClient(t *testing.T, e *Client, svc *probingClientService, action string) *probingClientService {
	t.Helper()
	_, _ = domain.GetUpdatable(e, func(*Client) error { return nil }, svc, action)
	return svc
}

// clientGranting returns a valid client that is granted one role.
func clientGranting(roleID string) *Client {
	e := validClient()
	e.AddClientRole(aggregatevos.ClientRole{RoleID: domain.NewID(roleID)})
	return e
}

// ── credential-minting ──────────────────────────────────────────────────────

// TestInsertMintsTheWholeCredentialState asserts every field the rule writes:
// leaving one out is how a column silently keeps its zero value with nothing
// reporting it.
func TestInsertMintsTheWholeCredentialState(t *testing.T) {
	e := validClient()
	// Deliberately WRONG starting values, so the assertions prove the rule wrote
	// them rather than that the fixture happened to carry them.
	e.Secret = ""
	e.SecretHash = ""
	e.SecretChangedAt = time.Time{}

	svc := insertClient(t, e, &probingClientService{})

	if e.Secret == "" {
		t.Fatal("no secret was minted")
	}
	if !strings.HasPrefix(e.Secret, "acs_") {
		t.Fatalf("the secret %q carries no acs_ prefix; a leaked value would be invisible to every scanner", e.Secret)
	}
	// 32 bytes render as exactly 43 base64url characters unpadded.
	if got := len(e.Secret) - len("acs_"); got != 43 {
		t.Fatalf("the secret body is %d characters, want 43 (32 random bytes, base64url unpadded)", got)
	}
	if strings.ContainsAny(e.Secret, "+/=") {
		t.Fatalf("the secret %q is not base64URL: it must survive a header and a query string", e.Secret)
	}
	if e.SecretHash != "sha256:"+e.Secret {
		t.Fatalf("the hash was not derived through the service: %q", e.SecretHash)
	}
	if e.SecretChangedAt.IsZero() {
		t.Fatal("SecretChangedAt was not stamped")
	}
	if len(svc.hashedSecrets) != 1 {
		t.Fatalf("the service hashed %d times, want exactly 1", len(svc.hashedSecrets))
	}
	// Nothing is retiring on a first issue, and stamping a deadline here would
	// make every brand-new client look like one mid-rotation.
	if e.PreviousSecretHash != nil || e.PreviousSecretExpiresAt != nil {
		t.Fatal("a first issue left a retiring secret behind")
	}
}

// TestTwoInsertsMintDifferentSecrets is the property a fixed test double would
// hide: the randomness is real.
func TestTwoInsertsMintDifferentSecrets(t *testing.T) {
	a, b := validClient(), validClient()
	insertClient(t, a, &probingClientService{})
	insertClient(t, b, &probingClientService{})
	if a.Secret == b.Secret {
		t.Fatal("two clients were minted the same secret")
	}
}

// TestAnUnavailableTenantIsRefusedBeforeAnythingIsMinted — the ordering the rule
// gate encodes. Minting derives a hash, and deriving one for a write the rules
// refused is a value nobody asked for.
func TestAnUnavailableTenantIsRefusedBeforeAnythingIsMinted(t *testing.T) {
	e := validClient()
	e.SecretHash = ""
	svc := insertClient(t, e, &probingClientService{tenantUnavailable: true})

	if !clientHasKey(clientNotificationKeys(e), "ClientTenantDoesNotExistNotification") {
		t.Fatalf("an unavailable tenant was accepted; got %v", clientNotificationKeys(e))
	}
	if svc.askedTenant != 1 {
		t.Fatalf("the tenant was probed %d times, want 1", svc.askedTenant)
	}
}

// ── the role grants ─────────────────────────────────────────────────────────

func TestAnUnavailableRoleIsRefused(t *testing.T) {
	e := clientGranting(clientRoleID)
	insertClient(t, e, &probingClientService{roleUnavailable: true})

	if !clientHasKey(clientNotificationKeys(e), "RoleNotAvailableInTenantNotification") {
		t.Fatalf("an unavailable role was granted; got %v", clientNotificationKeys(e))
	}
}

// TestTheRoleIsJudgedAgainstTheCLIENTsTenant is the super-admin case. On the
// ordinary path the two tenants are the same value; a `*:*` operator crosses the
// scope, and when they do, "this tenant" has to mean the client's or every
// legitimate grant is refused.
func TestTheRoleIsJudgedAgainstTheCLIENTsTenant(t *testing.T) {
	e := clientGranting(clientRoleID)
	e.RequestingTenant = clientOtherTenant // the operator's own tenant
	e.RequestingMayCrossScope = true

	svc := insertClient(t, e, &probingClientService{})

	if len(svc.askedRoleAvail) != 1 {
		t.Fatalf("the role availability was asked %d times, want 1", len(svc.askedRoleAvail))
	}
	if got := svc.askedRoleAvail[0].tenantID.Value(); got != clientOwnTenant {
		t.Fatalf("the role was judged against %q; it must be the CLIENT's tenant %q", got, clientOwnTenant)
	}
}

// TestAWildcardRoleNeverReachesTheEscalationProbe is the ordering rule, and it
// is load-bearing rather than tidy: Identity.HasPermission PANICS on a wildcard,
// so reaching the probe would turn a refusal into a 500 on exactly the case the
// escalation rule exists to stop.
func TestAWildcardRoleNeverReachesTheEscalationProbe(t *testing.T) {
	e := clientGranting(clientRoleID)
	svc := insertClient(t, e, &probingClientService{roleWildcard: true})

	if !clientHasKey(clientNotificationKeys(e), "CannotGrantWildcardRoleNotification") {
		t.Fatalf("a wildcard role was granted to a machine; got %v", clientNotificationKeys(e))
	}
	if len(svc.askedRoleEscalate) != 0 {
		t.Fatal("the escalation probe was asked about a wildcard role; that call panics in production")
	}
}

// TestARoleCarryingUnheldPermissionsIsRefused is the rule that keeps a machine
// credential from carrying more than the person who created it.
func TestARoleCarryingUnheldPermissionsIsRefused(t *testing.T) {
	e := clientGranting(clientRoleID)
	insertClient(t, e, &probingClientService{lacksRolePerm: true})

	if !clientHasKey(clientNotificationKeys(e), "CannotGrantRoleWithUnheldPermissionsNotification") {
		t.Fatalf("a caller granted a role they do not hold; got %v", clientNotificationKeys(e))
	}
}

// TestTheEscalationProbeStandsDownWithNoIdentity — a development bench with
// auth.mode disabled has nobody to compare against, and refusing there would
// make the entity unusable where the framework already allows it.
func TestTheClientEscalationProbeStandsDownWithNoIdentity(t *testing.T) {
	e := clientGranting(clientRoleID)
	e.RequestingIdentityPresent = false
	svc := insertClient(t, e, &probingClientService{lacksRolePerm: true})

	if len(svc.askedRoleEscalate) != 0 {
		t.Fatal("the escalation probe ran with no identity to compare against")
	}
	if clientHasKey(clientNotificationKeys(e), "CannotGrantRoleWithUnheldPermissionsNotification") {
		t.Fatal("the escalation rule fired with no identity present")
	}
}

// TestAnUnusableRoleIDIsNotProbed — the id is handed to a criterion against a
// UUID column, so an unparseable one would make the query error and the probe
// panic: a 500 for a problem that is plain validation, which the framework's own
// child validation already reports.
func TestAnUnusableRoleIDIsNotProbed(t *testing.T) {
	e := clientGranting("not-a-uuid")
	svc := insertClient(t, e, &probingClientService{})

	if len(svc.askedRoleAvail) != 0 {
		t.Fatal("an unparseable role id reached the database probe")
	}
}

// ── archive-forces-suspended ────────────────────────────────────────────────

func TestArchivingAClientForcesSuspended(t *testing.T) {
	e := validClient()
	e.Status = vos.ClientStatusActive
	_, _ = domain.GetArchivable(e, &probingClientService{}, "Archive")

	if e.Status != vos.ClientStatusSuspended {
		t.Fatalf("an archived client is still %q; archived+active is a state nothing should be able to represent", e.Status)
	}
}

// ── client-modifies-only-itself ─────────────────────────────────────────────

// TestTheRowRuleIsInertWithoutTheClaim is the deliberate part, not an oversight:
// nothing mints identity_kind yet, so the field reads "" and the rule stands
// down. A change that made it fire on an absent claim would refuse every write
// in the service.
func TestTheRowRuleIsInertWithoutTheClaim(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "" // nothing mints it today
	e.RequestingClientID = someOtherClientID

	updateClient(t, e, &probingClientService{}, "Patch")

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyModifyItselfNotification") {
		t.Fatal("the row rule fired without the claim; it must read an absent claim as a user")
	}
}

func TestAUserSubjectCallerIsUnaffectedByTheRowRule(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "user"
	e.RequestingClientID = someOtherClientID

	updateClient(t, e, &probingClientService{}, "Patch")

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyModifyItselfNotification") {
		t.Fatal("a user-subject caller was refused by the client row rule")
	}
}

func TestAClientSubjectCallerMayEditItsOwnRow(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "client"
	e.RequestingClientID = clientRowID

	updateClient(t, e, &probingClientService{}, "Patch")

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyModifyItselfNotification") {
		t.Fatal("a client was refused its own row")
	}
}

func TestAClientSubjectCallerMayNotEditAnotherRow(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "client"
	e.RequestingClientID = someOtherClientID

	updateClient(t, e, &probingClientService{}, "Patch")

	if !clientHasKey(clientNotificationKeys(e), "ClientMayOnlyModifyItselfNotification") {
		t.Fatalf("a client edited somebody else's row; got %v", clientNotificationKeys(e))
	}
}

// TestAClientSubjectCallerCannotCreateAClient is the self-replication answer,
// and it costs no rule of its own: on an insert there is no id yet, so the
// comparison is false and the write is refused.
func TestAClientSubjectCallerCannotCreateAClient(t *testing.T) {
	e := validClient()
	e.RequestingIdentityKind = "client"
	e.RequestingClientID = someOtherClientID

	insertClient(t, e, &probingClientService{})

	if !clientHasKey(clientNotificationKeys(e), "ClientMayOnlyModifyItselfNotification") {
		t.Fatalf("a machine credential minted another machine credential; got %v", clientNotificationKeys(e))
	}
}

func TestAClientSubjectCallerMayNotArchiveAnotherRow(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "client"
	e.RequestingClientID = someOtherClientID

	_, _ = domain.GetArchivable(e, &probingClientService{}, "Archive")

	if !clientHasKey(clientNotificationKeys(e), "ClientMayOnlyModifyItselfNotification") {
		t.Fatalf("a client archived somebody else's row; got %v", clientNotificationKeys(e))
	}
}

// ── what no unit test can reach ─────────────────────────────────────────────
//
// Two branches in client_rules_manual.go are unreachable from here, and both are
// recorded rather than chased:
//
//   * the `service == nil` guards. The GENERATED BuildRules asserts
//     `service.(ClientService)` unguarded and panics first, so no service that
//     reaches customRules can be nil. They mirror user_rules_manual.go verbatim
//     and stay as the second lock, in case that assertion ever softens.
//   * the panic in newClientSecret, which fires only when the operating system's
//     random source fails. Faking crypto/rand to reach it would test the fake.
//
// They are why this file lands near 91% rather than at the repository's 95%, and
// the same two shapes put user_rules_manual.go at 91.5%.
