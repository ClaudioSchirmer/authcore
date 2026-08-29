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
	"fmt"
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
	claimUnavailable  bool
	claimAppliesElse  bool
	claimValueBadType bool

	askedTenant       int
	askedRoleAvail    []clientScopedQuestion
	askedRoleWildcard []domain.ID
	askedRoleEscalate []domain.ID
	hashedSecrets     []string
	askedClaimAvail   []clientScopedQuestion
	askedClaimApplies []domain.ID
	askedClaimType    []clientClaimTypeQuestion
}

// clientClaimTypeQuestion records BOTH arguments of the value-type probe: which
// definition was asked about AND the value that was judged. The second is what
// proves a CHANGED entry reaches the rule with its NEW value.
type clientClaimTypeQuestion struct {
	claimID domain.ID
	value   string
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

// ── client-rotates-only-its-own-secret ──────────────────────────────────────

// TestTheRowRuleIsInertWithoutTheClaim is the deliberate part, not an oversight:
// nothing mints identity_kind yet, so the field reads "" and the rule stands
// down. A change that made it fire on an absent claim would refuse every write
// in the service.
func TestTheRowRuleIsInertWithoutTheClaim(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "" // nothing mints it today
	e.RequestingClientID = someOtherClientID

	updateClient(t, e, &probingClientService{}, ActionRotateSecret)

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyRotateItsOwnSecretNotification") {
		t.Fatal("the row rule fired without the claim; it must read an absent claim as a user")
	}
}

// TestAUserSubjectCallerIsUnaffectedByTheRowRule is the helpdesk case, and the
// mirror of User's reset: an operator holding client:rotate-secret rotates any
// client in their tenant. The early return is on the KIND, so a person never
// meets this rule at all.
func TestAUserSubjectCallerIsUnaffectedByTheRowRule(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "user"
	e.RequestingClientID = someOtherClientID

	updateClient(t, e, &probingClientService{}, ActionRotateSecret)

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyRotateItsOwnSecretNotification") {
		t.Fatal("a user-subject caller was refused by the client row rule")
	}
}

func TestAClientSubjectCallerMayRotateItsOwnSecret(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "client"
	e.RequestingClientID = clientRowID

	updateClient(t, e, &probingClientService{}, ActionRotateSecret)

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyRotateItsOwnSecretNotification") {
		t.Fatal("a client was refused the rotation of its own secret")
	}
}

// TestAClientSubjectCallerMayNotRotateAnotherSecret is what the rule narrowed
// DOWN to on 2026-08-28. Rotating is not editing: the call mints a credential and
// starts retiring the one in use, so a machine able to do it to another machine
// could lock it out and take its place — one call that is both a denial of
// service and an impersonation.
func TestAClientSubjectCallerMayNotRotateAnotherSecret(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "client"
	e.RequestingClientID = someOtherClientID

	updateClient(t, e, &probingClientService{}, ActionRotateSecret)

	if !clientHasKey(clientNotificationKeys(e), "ClientMayOnlyRotateItsOwnSecretNotification") {
		t.Fatalf("a client rotated somebody else's secret; got %v", clientNotificationKeys(e))
	}
}

// TestAClientSubjectCallerMayEditAnotherRow is the other half of that narrowing,
// and it is a test rather than an absence because the old behaviour was
// deliberate too: editing a sibling client is an ordinary tenant-scoped write,
// gated by the permission the caller carries and by nothing else.
func TestAClientSubjectCallerMayEditAnotherRow(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "client"
	e.RequestingClientID = someOtherClientID

	updateClient(t, e, &probingClientService{}, "Patch")

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyRotateItsOwnSecretNotification") {
		t.Fatalf("a client was refused an ordinary edit of a sibling row; got %v", clientNotificationKeys(e))
	}
}

// TestAClientSubjectCallerMayCreateAClient replaces a rule that used to refuse
// this outright. The refusal was dropped on 2026-08-28 for the same reason the
// edit was: a client-subject caller holding client:insert creates clients in its
// tenant like any other caller, and the permission is what says whether it may.
//
// What bounded the damage before still bounds it: a client can be granted no role
// whose permissions its grantor does not already hold, and the tenant scope
// applies to a machine exactly as it does to a person.
func TestAClientSubjectCallerMayCreateAClient(t *testing.T) {
	e := validClient()
	e.RequestingIdentityKind = "client"
	e.RequestingClientID = someOtherClientID

	insertClient(t, e, &probingClientService{})

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyRotateItsOwnSecretNotification") {
		t.Fatalf("the rotation rule fired on an insert; it guards one action and no other; got %v", clientNotificationKeys(e))
	}
}

// TestAUserSubjectCallerMayStillCreateAClient outlived the rule it was written
// against: with the client refusal gone, both kinds create clients, and the test
// stays because "a person may" is worth pinning on its own.
func TestAUserSubjectCallerMayStillCreateAClient(t *testing.T) {
	e := validClient()
	e.RequestingIdentityKind = "user"

	insertClient(t, e, &probingClientService{})

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyRotateItsOwnSecretNotification") {
		t.Fatalf("a person was refused the creation of a client; got %v", clientNotificationKeys(e))
	}
}

// TestAClientSubjectCallerMayArchiveAnotherRow: the archive gate no longer
// carries the row rule at all. Archiving a sibling is an ordinary tenant-scoped
// write — it takes a client out of service, it does not take it over.
func TestAClientSubjectCallerMayArchiveAnotherRow(t *testing.T) {
	e := validClient()
	e.SetID(domain.NewID(clientRowID))
	e.RequestingIdentityKind = "client"
	e.RequestingClientID = someOtherClientID

	_, _ = domain.GetArchivable(e, &probingClientService{}, "Archive")

	if clientHasKey(clientNotificationKeys(e), "ClientMayOnlyRotateItsOwnSecretNotification") {
		t.Fatalf("a client was refused the archive of a sibling row; got %v", clientNotificationKeys(e))
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

func (s *probingClientService) ClaimIsUnavailableInTenant(tenantID domain.ID, claimID domain.ID) bool {
	s.askedClaimAvail = append(s.askedClaimAvail, clientScopedQuestion{tenantID: tenantID, targetID: claimID})
	return s.claimUnavailable
}

func (s *probingClientService) ClaimDoesNotApplyToClient(claimID domain.ID) bool {
	s.askedClaimApplies = append(s.askedClaimApplies, claimID)
	return s.claimAppliesElse
}

func (s *probingClientService) ClaimValueDoesNotMatchValueType(claimID domain.ID, value string) bool {
	s.askedClaimType = append(s.askedClaimType, clientClaimTypeQuestion{claimID: claimID, value: value})
	return s.claimValueBadType
}

// ── the claims collection: the three per-entry rules ────────────────────────
//
// The twin of User's block. It is written out rather than shared, for the same
// reason the two rules are separate methods: the two parents ask DIFFERENT
// facts, and a shared table-driven case would hide which side a refusal came
// from.

const (
	someClientClaimID      = "0198f3e0-7b31-7c02-8a55-1f9d2e6b4c17"
	someOtherClientClaimID = "0198f3e0-8c42-7d13-9b66-2a0e3f7c5d28"
)

// clientHolding returns a valid client that SETS one claim value.
func clientHolding(claimID, value string) *Client {
	e := validClient()
	e.AddClientClaim(aggregatevos.ClientClaim{
		ClaimID: domain.NewID(claimID),
		Value:   vos.ClaimValue(value),
	})
	return e
}

func TestClientClaimValueIsRefusedWhenTheDefinitionIsNotAvailableInTheTenant(t *testing.T) {
	e := clientHolding(someClientClaimID, "sa-east-1")
	svc := insertClient(t, e, &probingClientService{claimUnavailable: true})

	if !clientHasKey(clientNotificationKeys(e), "ClaimNotAvailableInTenantNotification") {
		t.Fatalf("a value was set against an absent, archived or foreign definition; answers were %v",
			clientNotificationKeys(e))
	}
	if len(svc.askedClaimAvail) != 1 {
		t.Fatalf("the availability probe was asked %d times, want once per added entry", len(svc.askedClaimAvail))
	}
	if svc.askedClaimAvail[0].tenantID != e.TenantID {
		t.Errorf("the probe was scoped by %v, want the row's tenant %v",
			svc.askedClaimAvail[0].tenantID, e.TenantID)
	}
}

func TestClientNothingBelowIsAskedAboutADefinitionThatIsNotThere(t *testing.T) {
	svc := insertClient(t, clientHolding(someClientClaimID, "sa-east-1"),
		&probingClientService{claimUnavailable: true})

	if len(svc.askedClaimApplies) != 0 || len(svc.askedClaimType) != 0 {
		t.Fatalf("the later probes ran on an unresolvable definition: applies=%d type=%d",
			len(svc.askedClaimApplies), len(svc.askedClaimType))
	}
}

// The other half of the pair that makes appliesTo mean something: a definition
// declaring `user` is refused here, and its twin on User refuses `client`.
func TestClientClaimValueIsRefusedWhenTheDefinitionDoesNotApplyToClients(t *testing.T) {
	e := clientHolding(someClientClaimID, "sa-east-1")
	insertClient(t, e, &probingClientService{claimAppliesElse: true})

	if !clientHasKey(clientNotificationKeys(e), "ClaimDoesNotApplyToClientNotification") {
		t.Fatalf("a client holds a value for a user-only definition; answers were %v",
			clientNotificationKeys(e))
	}
}

func TestClientClaimValueIsRefusedWhenItDoesNotParseAsTheDeclaredType(t *testing.T) {
	e := clientHolding(someClientClaimID, "maybe")
	svc := insertClient(t, e, &probingClientService{claimValueBadType: true})

	if !clientHasKey(clientNotificationKeys(e), "ClaimValueDoesNotMatchValueTypeNotification") {
		t.Fatalf("a value of the wrong type was stored; answers were %v", clientNotificationKeys(e))
	}
	if len(svc.askedClaimType) != 1 || svc.askedClaimType[0].value != "maybe" {
		t.Fatalf("the value handed to the type probe was %v", svc.askedClaimType)
	}
}

func TestAValidClientClaimValueIsAccepted(t *testing.T) {
	e := clientHolding(someClientClaimID, "sa-east-1")
	if _, err := domain.GetInsertable(e, &probingClientService{}, "Insert"); err != nil {
		t.Fatalf("a valid claim value was refused: %v", err)
	}
}

// A CHANGED entry is judged like an added one — the PATCH path. See the User
// twin for the full reasoning.
func TestACorrectedClientValueIsJudgedLikeANewOne(t *testing.T) {
	e := validClient()
	original := aggregatevos.ClientClaim{
		ClaimID: domain.NewID(someClientClaimID),
		Value:   vos.ClaimValue("sa-east-1"),
	}
	e.AddClientClaim(original)
	domain.ChangeAggregateChild(e, original, aggregatevos.ClientClaim{
		ClaimID: domain.NewID(someClientClaimID),
		Value:   vos.ClaimValue("not-a-region"),
	})

	svc := insertClient(t, e, &probingClientService{claimValueBadType: true})

	if !clientHasKey(clientNotificationKeys(e), "ClaimValueDoesNotMatchValueTypeNotification") {
		t.Fatalf("a corrected value skipped the type check; answers were %v", clientNotificationKeys(e))
	}
	if len(svc.askedClaimType) != 1 || svc.askedClaimType[0].value != "not-a-region" {
		t.Fatalf("the probe was handed %v, want the NEW value", svc.askedClaimType)
	}
}

func TestASecondValueForTheSameDefinitionIsRefusedOnAClient(t *testing.T) {
	e := clientHolding(someClientClaimID, "sa-east-1")
	e.AddClientClaim(aggregatevos.ClientClaim{
		ClaimID: domain.NewID(someClientClaimID),
		Value:   vos.ClaimValue("us-east-1"),
	})

	if !clientHasKey(clientNotificationKeys(e), "ClientAlreadyHoldsClaimNotification") {
		t.Fatalf("one client holds two values for one definition; answers were %v",
			clientNotificationKeys(e))
	}
}

func TestTwoDifferentDefinitionsCoexistOnAClient(t *testing.T) {
	e := clientHolding(someClientClaimID, "sa-east-1")
	e.AddClientClaim(aggregatevos.ClientClaim{
		ClaimID: domain.NewID(someOtherClientClaimID),
		Value:   vos.ClaimValue("1000"),
	})

	if clientHasKey(clientNotificationKeys(e), "ClientAlreadyHoldsClaimNotification") {
		t.Fatalf("two distinct definitions were treated as a duplicate: %v", clientNotificationKeys(e))
	}
}

func TestAClientMayNotHoldMoreThanTwentyClaimValues(t *testing.T) {
	e := validClient()
	for i := 0; i < 21; i++ {
		e.AddClientClaim(aggregatevos.ClientClaim{
			ClaimID: domain.NewID(fmt.Sprintf("0198f3e0-7b31-7c02-8a55-1f9d2e6b%04d", i)),
			Value:   vos.ClaimValue("v"),
		})
	}
	insertClient(t, e, &probingClientService{})

	if !clientHasKey(clientNotificationKeys(e), "TooManyClaimsForClientNotification") {
		t.Fatalf("21 claim values were accepted; answers were %v", clientNotificationKeys(e))
	}
}

// NO ESCALATION PROBE, and this is the assertion for it. The roles collection
// beside this one refuses a grant the caller does not already hold; a claim
// confers nothing inside this service, so setting one must not consult the
// caller's permissions at all. If somebody later "fixes" the asymmetry by
// wiring the role probes into the claims loop, this fails.
func TestSettingAClaimValueDoesNotConsultTheCallersPermissions(t *testing.T) {
	e := clientHolding(someClientClaimID, "sa-east-1")
	svc := insertClient(t, e, &probingClientService{lacksRolePerm: true, roleWildcard: true})

	if len(svc.askedRoleEscalate) != 0 || len(svc.askedRoleWildcard) != 0 {
		t.Fatalf("a claim value was judged against the caller's privileges: escalate=%d wildcard=%d",
			len(svc.askedRoleEscalate), len(svc.askedRoleWildcard))
	}
	if _, err := domain.GetInsertable(clientHolding(someClientClaimID, "sa-east-1"),
		&probingClientService{lacksRolePerm: true, roleWildcard: true}, "Insert"); err != nil {
		t.Fatalf("a claim value was refused for a privilege reason: %v", err)
	}
}
