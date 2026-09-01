// Tests for the machine sign-in reader.
//
// WHAT THIS FILE PROTECTS are the three decisions that are pure functions of a
// loaded row and therefore testable without a database: which secrets a client
// still accepts, which addresses may obtain a token for it, and how the grant
// fan-out collapses into the answers a token is built from.
//
// The statements themselves — the anchors, the joins, the predicates — are proven
// against a real engine in the live suite beside the user reader's, and validated
// at construction by the framework, which aborts the boot naming a column that
// stopped resolving.

package infra

import (
	"testing"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// ── the credential ──────────────────────────────────────────────────────────

func hashOf(secret string) string { return clientHasher.Hash(secret) }

func ptrTime(t time.Time) *time.Time { return &t }
func ptrStr(s string) *string        { return &s }

// The live secret is accepted, and anything else is not.
func TestSecretMatchesTheLiveSecret(t *testing.T) {
	r := &ClientAuthenticationReader{}
	client := &schemas.SignInClient{SecretHash: hashOf("acs_live")}

	if !r.SecretMatches("acs_live", client) {
		t.Error("the live secret was refused")
	}
	if r.SecretMatches("acs_wrong", client) {
		t.Error("a wrong secret was accepted")
	}
	if r.SecretMatches("", client) {
		t.Error("an empty secret was accepted")
	}
}

// A rotation in flight keeps the retiring secret working until its window closes.
//
// THIS IS THE WHOLE POINT OF THE OVERLAP: an integration can be redeployed without
// a window in which neither value works. It is also the reason the digest is
// SHA-256 rather than Argon2id — two verifies on the miss path, on an
// unauthenticated route.
func TestSecretMatchesTheRetiringSecretInsideItsWindow(t *testing.T) {
	r := &ClientAuthenticationReader{}
	client := &schemas.SignInClient{
		SecretHash:              hashOf("acs_new"),
		PreviousSecretHash:      ptrStr(hashOf("acs_old")),
		PreviousSecretExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
	}

	if !r.SecretMatches("acs_new", client) {
		t.Error("the live secret was refused while a rotation was in flight")
	}
	if !r.SecretMatches("acs_old", client) {
		t.Error("the retiring secret was refused inside its grace window — a redeploy would have broken")
	}
	if r.SecretMatches("acs_never", client) {
		t.Error("a secret that was never this client's was accepted")
	}
}

// Past the window the retiring secret is simply a wrong secret.
//
// A `gracePeriodSeconds: 0` rotation is what somebody does with a LEAKED credential,
// and it has to kill the old value the moment it is asked to — the hash stays on the
// row until the next rotation overwrites it, so the EXPIRY is the only thing
// stopping it.
func TestSecretRefusesTheRetiringSecretOnceItsWindowClosed(t *testing.T) {
	r := &ClientAuthenticationReader{}
	client := &schemas.SignInClient{
		SecretHash:              hashOf("acs_new"),
		PreviousSecretHash:      ptrStr(hashOf("acs_old")),
		PreviousSecretExpiresAt: ptrTime(time.Now().UTC().Add(-time.Second)),
	}

	if r.SecretMatches("acs_old", client) {
		t.Error("a retired secret still authenticated after its window closed — a leaked credential would survive its own revocation")
	}
	if !r.SecretMatches("acs_new", client) {
		t.Error("the live secret was refused")
	}
}

// A half-written rotation reads as no rotation, which is the fail-closed direction.
func TestSecretIgnoresAHalfWrittenRotation(t *testing.T) {
	r := &ClientAuthenticationReader{}

	hashNoExpiry := &schemas.SignInClient{
		SecretHash:         hashOf("acs_new"),
		PreviousSecretHash: ptrStr(hashOf("acs_old")),
	}
	if hashNoExpiry.PreviousSecretExpiresAt != nil {
		t.Fatal("the fixture is wrong")
	}
	if r.SecretMatches("acs_old", hashNoExpiry) {
		t.Error("a previous hash with no expiry authenticated; absent an expiry nothing bounds it")
	}

	expiryNoHash := &schemas.SignInClient{
		SecretHash:              hashOf("acs_new"),
		PreviousSecretExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
	}
	if r.SecretMatches("acs_old", expiryNoHash) {
		t.Error("an expiry with no previous hash authenticated something")
	}
}

// An empty stored hash authenticates nobody, including an empty secret.
//
// A row with no credential is a row nobody can authenticate as, which is exactly
// what false means here — and a nil client is the same answer for the same reason,
// so no caller can reach a match by handing in nothing.
func TestSecretRefusesARowWithNoCredential(t *testing.T) {
	r := &ClientAuthenticationReader{}
	if r.SecretMatches("", &schemas.SignInClient{}) {
		t.Error("a row carrying no hash authenticated an empty secret")
	}
	if r.SecretMatches("acs_anything", &schemas.SignInClient{}) {
		t.Error("a row carrying no hash authenticated a secret")
	}
	if r.SecretMatches("acs_anything", nil) {
		t.Error("a nil client authenticated something")
	}
}

// The timing decoy must not be optimisable away, and must never match.
//
// It is cheap insurance on this route rather than the load-bearing thing it is on
// the user one — a SHA-256 digest sits under the noise of the round trip — but it
// exists so that a change of hash algorithm cannot silently remove the
// equalisation. What must hold either way: nobody, including this process, knows a
// plaintext that matches the decoy.
func TestBurnSecretVerificationNeverMatches(t *testing.T) {
	r := &ClientAuthenticationReader{}
	r.BurnSecretVerification() // must not panic

	if secretEqualisationHash == "" {
		t.Fatal("the decoy hash is empty; the equalisation verifies against nothing")
	}
	if clientHasher.Matches("timing-equalisation", secretEqualisationHash) {
		t.Error("the decoy matched its own probe input — it must be seeded from randomness, not from a constant")
	}
}

// ── the allow-list ──────────────────────────────────────────────────────────

// An empty collection means ANY address. Fail-open, and it is a decision rather
// than an oversight: fail-closed would make every newly created client unable to
// sign in until a second call, a step every provisioning script forgets.
func TestAddressAllowedWithNoRangesAdmitsAnything(t *testing.T) {
	for _, ip := range []string{"203.0.113.7", "2001:db8::1", "", "not-an-ip"} {
		if !AddressAllowed(ip, nil) {
			t.Errorf("an empty allow-list refused %q", ip)
		}
	}
}

// A declared restriction with no usable origin address REFUSES.
//
// The asymmetry with the case above is the point: the operator stated a
// restriction, so admitting a caller whose origin nobody could determine would
// silently void it.
func TestAddressAllowedWithRangesRefusesAnUnusableAddress(t *testing.T) {
	ranges := []string{"203.0.113.0/24"}
	for _, ip := range []string{"", "not-an-ip", "203.0.113"} {
		if AddressAllowed(ip, ranges) {
			t.Errorf("a stated restriction admitted the unusable address %q", ip)
		}
	}
}

func TestAddressAllowedContainment(t *testing.T) {
	ranges := []string{"203.0.113.0/24", "2001:db8::/32"}

	for _, ip := range []string{"203.0.113.1", "203.0.113.255", "2001:db8::1", "2001:db8:dead:beef::9"} {
		if !AddressAllowed(ip, ranges) {
			t.Errorf("%q is inside a declared range and was refused", ip)
		}
	}
	for _, ip := range []string{"203.0.114.1", "198.51.100.7", "2001:db9::1"} {
		if AddressAllowed(ip, ranges) {
			t.Errorf("%q is outside every declared range and was admitted", ip)
		}
	}
}

// An IPv4-mapped IPv6 peer is the same host written the other way.
//
// A dual-stack listener hands back "::ffff:203.0.113.7" for an IPv4 caller. Without
// the unmap, a perfectly correct 203.0.113.0/24 entry would refuse it, and the
// symptom would be an integration that works on one cluster and not another.
func TestAddressAllowedUnmapsIPv4MappedAddresses(t *testing.T) {
	if !AddressAllowed("::ffff:203.0.113.7", []string{"203.0.113.0/24"}) {
		t.Error("an IPv4-mapped peer was refused by the IPv4 range that contains it")
	}
	if AddressAllowed("::ffff:198.51.100.7", []string{"203.0.113.0/24"}) {
		t.Error("an IPv4-mapped peer outside the range was admitted")
	}
}

// A stored range nobody can parse is SKIPPED, never treated as universal.
//
// Unreachable through the API — vos.CIDRBlock refuses what netip cannot parse — and
// reachable by a migration or a hand-run UPDATE. A range nobody can read must not be
// the one that admits a caller.
func TestAddressAllowedSkipsAnUnparseableRange(t *testing.T) {
	if AddressAllowed("203.0.113.7", []string{"garbage"}) {
		t.Error("an unparseable range admitted a caller; a range nobody can read must admit nobody")
	}
	if !AddressAllowed("203.0.113.7", []string{"garbage", "203.0.113.0/24"}) {
		t.Error("one unparseable entry poisoned a collection that also held a matching range")
	}
}

// ── the fan-out ─────────────────────────────────────────────────────────────

func grantRow(roleKey string, mut func(*schemas.ClientRoleGrant)) schemas.ClientRoleGrant {
	row := schemas.ClientRoleGrant{RoleKey: roleKey, RoleName: roleKey + " name"}
	if mut != nil {
		mut(&row)
	}
	return row
}

func perm(resource, action string) (*string, *string) { return &resource, &action }

// A role holding several permissions arrives as several rows and collapses to one
// role and several permissions, deduplicated and ordered.
func TestAssembleClientCollapsesTheFanOut(t *testing.T) {
	res1, act1 := perm("user", "read")
	res2, act2 := perm("client", "insert")
	res3, act3 := perm("user", "read") // the same permission through another role

	bundle := assembleClient([]schemas.ClientRoleGrant{
		grantRow("ops", func(r *schemas.ClientRoleGrant) { r.Resource, r.Action = res1, act1 }),
		grantRow("ops", func(r *schemas.ClientRoleGrant) { r.Resource, r.Action = res2, act2 }),
		grantRow("billing", func(r *schemas.ClientRoleGrant) { r.Resource, r.Action = res3, act3 }),
	}, nil, nil, nil)

	if len(bundle.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d: %+v", len(bundle.Roles), bundle.Roles)
	}
	if bundle.Roles[0].Key != "billing" || bundle.Roles[1].Key != "ops" {
		t.Errorf("roles are not in stable key order: %+v", bundle.Roles)
	}
	if bundle.Roles[1].Name != "ops name" {
		t.Errorf("the role lost its display name: %+v", bundle.Roles[1])
	}
	if len(bundle.Permissions) != 2 {
		t.Fatalf("expected 2 deduplicated permissions, got %d: %+v", len(bundle.Permissions), bundle.Permissions)
	}
	if bundle.Permissions[0] != (vos.PermissionKey{Resource: "client", Action: "insert"}) ||
		bundle.Permissions[1] != (vos.PermissionKey{Resource: "user", Action: "read"}) {
		t.Errorf("permissions are not in stable order: %+v", bundle.Permissions)
	}
}

// A role that confers nothing is still a role the client HOLDS.
//
// It arrives as one row with nil grants, and it has to survive: a token that dropped
// it would say the client holds fewer roles than it does, and an operator reading
// the response would go looking for a grant that was never revoked.
func TestAssembleClientKeepsARoleThatConfersNothing(t *testing.T) {
	bundle := assembleClient([]schemas.ClientRoleGrant{grantRow("empty", nil)}, nil, nil, nil)

	if len(bundle.Roles) != 1 || bundle.Roles[0].Key != "empty" {
		t.Fatalf("a role conferring nothing was dropped: %+v", bundle.Roles)
	}
	if len(bundle.Permissions) != 0 {
		t.Errorf("a role conferring nothing produced permissions: %+v", bundle.Permissions)
	}
}

// The three archive gates are applied PER PAIR, and the role survives all of them.
//
// This is the assertion the shape forces: a `WHERE grant.deleted_at IS NULL` in the
// predicate would drop the ROW, and the row is a role-and-grant pair — so the role
// would vanish along with what it confers. A role whose every permission was revoked
// is still a role the client holds.
func TestAssembleClientAppliesTheArchiveGatesPerPair(t *testing.T) {
	archived := time.Now().UTC()

	res1, act1 := perm("user", "read")
	res2, act2 := perm("user", "delete")
	res3, act3 := perm("user", "update")

	bundle := assembleClient([]schemas.ClientRoleGrant{
		// a live pair
		grantRow("ops", func(r *schemas.ClientRoleGrant) { r.Resource, r.Action = res1, act1 }),
		// the grant was revoked — the permission goes, the role stays
		grantRow("ops", func(r *schemas.ClientRoleGrant) {
			r.Resource, r.Action = res2, act2
			r.GrantArchivedAt = &archived
		}),
		// the catalog entry was retired — same
		grantRow("ops", func(r *schemas.ClientRoleGrant) {
			r.Resource, r.Action = res3, act3
			r.PermissionArchivedAt = &archived
		}),
		// the ROLE itself was retired — it is not held at all
		grantRow("retired", func(r *schemas.ClientRoleGrant) {
			r.Resource, r.Action = res1, act1
			r.RoleArchivedAt = &archived
		}),
	}, nil, nil, nil)

	if len(bundle.Roles) != 1 || bundle.Roles[0].Key != "ops" {
		t.Fatalf("expected only the live role, got %+v", bundle.Roles)
	}
	if len(bundle.Permissions) != 1 || bundle.Permissions[0].Action != "read" {
		t.Errorf("a revoked grant or a retired catalog entry reached the token: %+v", bundle.Permissions)
	}
}

// The claim values and the allow-list come back as the token path expects them.
func TestAssembleClientCarriesClaimValuesAndRanges(t *testing.T) {
	id := domain.NewID("018f2c7e-9a41-7b3d-8c52-2f7a1d9e4b60")

	bundle := assembleClient(nil,
		[]schemas.HeldClientClaimValue{{ClaimID: id, Value: "sa-east-1"}},
		[]schemas.ClaimDefinition{{Name: "x_region"}},
		[]schemas.ClientAllowedRange{{CIDR: "203.0.113.0/24"}, {CIDR: "198.51.100.0/24"}},
	)

	if got := bundle.ClaimValues[id]; got != "sa-east-1" {
		t.Errorf("the held claim value did not survive: %q", got)
	}
	if len(bundle.Definitions) != 1 || bundle.Definitions[0].Name != "x_region" {
		t.Errorf("the definitions did not survive: %+v", bundle.Definitions)
	}
	// Sorted, so a token minted from the same rows is byte-identical every time and
	// an operator diffing two allow-lists reads a real difference.
	if len(bundle.AllowedCIDRs) != 2 ||
		bundle.AllowedCIDRs[0] != "198.51.100.0/24" || bundle.AllowedCIDRs[1] != "203.0.113.0/24" {
		t.Errorf("the allow-list is missing or unsorted: %+v", bundle.AllowedCIDRs)
	}
}

// A range STORED in the IPv4-mapped form still matches an IPv4 peer.
//
// The mirror of the case above, on the other side of the comparison. Unreachable
// through the API — vos.CIDRBlock requires the canonical spelling — and reachable by
// a migration that wrote "::ffff:203.0.113.0/120". Normalising both sides is what
// keeps one host from being two different answers.
func TestAddressAllowedUnmapsAMappedRange(t *testing.T) {
	if !AddressAllowed("203.0.113.7", []string{"::ffff:203.0.113.0/120"}) {
		t.Error("an IPv4 peer was refused by the mapped range that contains it")
	}
}

// Permissions sharing a resource are ordered by ACTION.
//
// The second half of the comparator, and it is what makes the ordering total: two
// tokens minted from the same grants have to be byte-identical in this claim, or a
// diff between them stops being readable when somebody is working out why a
// permission disappeared.
func TestAssembleClientOrdersPermissionsByActionWithinAResource(t *testing.T) {
	resU1, actU1 := perm("user", "update")
	resU2, actU2 := perm("user", "archive")
	resU3, actU3 := perm("user", "read")

	bundle := assembleClient([]schemas.ClientRoleGrant{
		grantRow("ops", func(r *schemas.ClientRoleGrant) { r.Resource, r.Action = resU1, actU1 }),
		grantRow("ops", func(r *schemas.ClientRoleGrant) { r.Resource, r.Action = resU2, actU2 }),
		grantRow("ops", func(r *schemas.ClientRoleGrant) { r.Resource, r.Action = resU3, actU3 }),
	}, nil, nil, nil)

	want := []string{"archive", "read", "update"}
	if len(bundle.Permissions) != len(want) {
		t.Fatalf("got %+v", bundle.Permissions)
	}
	for i, action := range want {
		if bundle.Permissions[i].Action != action {
			t.Errorf("permission %d = %q, want %q — the order is not total: %+v",
				i, bundle.Permissions[i].Action, action, bundle.Permissions)
		}
	}
}

// A mapped-LOOKING prefix that is too short to be a mapped IPv4 range is skipped.
//
// "::ffff:0.0.0.0/64" parses, and its address answers Is4In6 — but a /64 spans far
// beyond the 96-bit ::ffff: header, so it covers addresses that are not IPv4 at all
// and cannot be rewritten as an IPv4 prefix. Converting it anyway would build a
// nonsense range; skipping it fails CLOSED, which is the right direction for a row
// only a migration could have written.
func TestAddressAllowedSkipsAMappedPrefixTooShortToBeIPv4(t *testing.T) {
	if AddressAllowed("203.0.113.7", []string{"::ffff:0.0.0.0/64"}) {
		t.Error("a prefix too short to be a mapped IPv4 range admitted a caller")
	}
	if !AddressAllowed("203.0.113.7", []string{"::ffff:0.0.0.0/64", "203.0.113.0/24"}) {
		t.Error("one unconvertible entry poisoned a collection that also held a matching range")
	}
}
