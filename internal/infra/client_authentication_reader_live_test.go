//go:build integration && postgres

// Drives the MACHINE sign-in's reads against a REAL Postgres, because the things
// worth proving about them cannot be seen in the code.
//
// THREE PROPERTIES HERE ARE PURE SQL and have no Go branch to unit-test:
//
//  1. THE `appliesTo` PREDICATE. A client token must resolve a tenant's `client`
//     and `both` definitions and NEVER its `user`-scoped ones. That is one
//     criteria.In in the burst; a test over the assembled bundle cannot see it,
//     because by then the row either arrived or did not.
//  2. THE ARCHIVE SCOPES ON THREE ANCHORS. A revoked role grant, a removed claim
//     value and a REVOKED CIDR ENTRY are dropped by the repository's default scope,
//     not by any line in this service. The third is the one that matters most: an
//     archived allow-list entry that kept admitting would be a revoked restriction
//     that still reads as revoked in the API.
//  3. THE TENANT GATE AND THE JOIN. The client row comes back only when its tenant
//     row exists, and it comes back carrying that tenant's workspace and status —
//     the two values the handler's usability check and the token's claims are built
//     from.
//
// EVERY ROW IS SPELLED OUT rather than written through the aggregates: what is
// under test is a READ over a shape, and driving the write paths to arrange it
// would put five other components between the fixture and the assertion.

package infra

import (
	"context"
	"os"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/engine/postgres"
)

// The fixture's fixed identities. Fixed rather than minted so a failed run leaves
// rows a human can look at, and so the cleanup below is exact. They share no prefix
// with the user reader's fixture, so the two suites cannot clear each other's rows.
const (
	machineTenant          = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
	machineTenantSuspended = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2"

	machineClient          = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1"
	machineClientArchived  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2"
	machineClientSuspended = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3"

	machineRoleHeld       = "cccccccc-cccc-4ccc-8ccc-ccccccccccc1"
	machineRoleRetired    = "cccccccc-cccc-4ccc-8ccc-ccccccccccc2"
	machineRoleNoGrants   = "cccccccc-cccc-4ccc-8ccc-ccccccccccc3"
	machineRoleRevokedFor = "cccccccc-cccc-4ccc-8ccc-ccccccccccc4"

	machinePermLive = "dddddddd-dddd-4ddd-8ddd-ddddddddddd1"
	// Granted ONLY by the retired role, so nothing else can put it in the answer:
	// a gate that stopped excluding that role would leak exactly this permission.
	machinePermOnlyViaRetired = "dddddddd-dddd-4ddd-8ddd-ddddddddddd2"
	machinePermRevoked        = "dddddddd-dddd-4ddd-8ddd-ddddddddddd3"

	claimForClients = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeee1"
	claimForBoth    = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeee2"
	claimForUsers   = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeee3"
	claimRetired    = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeee4"
)

func machineReader(t *testing.T) *ClientAuthenticationReader {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://omnicore:omnicore@localhost:5432/authcore_db?sslmode=disable"
	}
	eng, err := postgres.NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Skipf("no dev bench reachable: %v", err)
	}
	eng.SetClock(core.ClockDB)
	t.Cleanup(func() {
		clearMachineFixture(t, eng)
		eng.Close()
	})
	clearMachineFixture(t, eng)
	seedMachineFixture(t, eng)
	return NewClientAuthenticationReader(eng)
}

func clearMachineFixture(t *testing.T, eng *postgres.Postgres) {
	t.Helper()
	for _, stmt := range []string{
		`DELETE FROM client_claims WHERE client_id::text LIKE 'bbbbbbbb%'`,
		`DELETE FROM client_allowed_cidrs WHERE client_id::text LIKE 'bbbbbbbb%'`,
		`DELETE FROM client_roles WHERE client_id::text LIKE 'bbbbbbbb%'`,
		`DELETE FROM role_permissions WHERE role_id::text LIKE 'cccccccc%'`,
		`DELETE FROM clients WHERE id::text LIKE 'bbbbbbbb%'`,
		`DELETE FROM claims WHERE id::text LIKE 'eeeeeeee%'`,
		`DELETE FROM roles WHERE id::text LIKE 'cccccccc%'`,
		`DELETE FROM permissions WHERE id::text LIKE 'dddddddd%'`,
		`DELETE FROM tenants WHERE id::text LIKE 'aaaaaaaa%'`,
	} {
		if _, err := eng.Pool().Exec(context.Background(), stmt); err != nil {
			t.Fatalf("clearing (%s): %v", stmt, err)
		}
	}
}

func seedMachineFixture(t *testing.T, eng *postgres.Postgres) {
	t.Helper()
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := eng.Pool().Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding (%.60s…): %v", sql, err)
		}
	}

	exec(`INSERT INTO tenants (id, name, workspace, description, status)
	      VALUES ($1, 'Machine Walk', 'machine-walk', '', 'active')`, machineTenant)
	exec(`INSERT INTO tenants (id, name, workspace, description, status)
	      VALUES ($1, 'Lapsed', 'lapsed-walk', '', 'suspended')`, machineTenantSuspended)

	// The integrations: one signable, one archived, one suspended inside a
	// suspended tenant. The last two exist so the READER's answer can be told apart
	// from the HANDLER's — the reader drops an archived row and returns a suspended
	// one, because "suspended" is a decision the handler makes and a row it needs
	// in hand to make it.
	exec(`INSERT INTO clients (id, tenant_id, name, description, secret_hash,
	                           secret_changed_at, status, deleted_at)
	      VALUES ($1, $2, 'Billing integration', '', 'stored-hash', NOW(), 'active', NULL)`,
		machineClient, machineTenant)
	exec(`INSERT INTO clients (id, tenant_id, name, description, secret_hash,
	                           secret_changed_at, status, deleted_at)
	      VALUES ($1, $2, 'Retired integration', '', 'stored-hash', NOW(), 'active', NOW())`,
		machineClientArchived, machineTenant)
	exec(`INSERT INTO clients (id, tenant_id, name, description, secret_hash,
	                           secret_changed_at, status, deleted_at)
	      VALUES ($1, $2, 'Lapsed integration', '', 'stored-hash', NOW(), 'suspended', NULL)`,
		machineClientSuspended, machineTenantSuspended)

	for _, r := range []struct{ id, key, archived string }{
		{machineRoleHeld, "held", "NULL"},
		{machineRoleRetired, "retired", "NOW()"},
		{machineRoleNoGrants, "grants-nothing", "NULL"},
		{machineRoleRevokedFor, "revoked-grant", "NULL"},
	} {
		exec(`INSERT INTO roles (id, tenant_id, role_key, name, description, deleted_at)
		      VALUES ($1, $2, $3, $3, '', `+r.archived+`)`, r.id, machineTenant, r.key)
	}

	exec(`INSERT INTO permissions (id, resource_name, action_name, description, deleted_at)
	      VALUES ($1, 'invoice', 'read', '', NULL)`, machinePermLive)
	exec(`INSERT INTO permissions (id, resource_name, action_name, description, deleted_at)
	      VALUES ($1, 'ledger', 'purge', '', NULL)`, machinePermOnlyViaRetired)
	exec(`INSERT INTO permissions (id, resource_name, action_name, description, deleted_at)
	      VALUES ($1, 'invoice', 'delete', '', NOW())`, machinePermRevoked)

	// The client's grants: three live edges and one REVOKED edge onto a live role.
	exec(`INSERT INTO client_roles (id, client_id, role_id, deleted_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL),
	      (gen_random_uuid(), $1, $3, NULL),
	      (gen_random_uuid(), $1, $4, NULL),
	      (gen_random_uuid(), $1, $5, NOW())`,
		machineClient, machineRoleHeld, machineRoleRetired, machineRoleNoGrants, machineRoleRevokedFor)

	// What the roles confer. The retired role grants a LIVE permission that nothing
	// else grants, so a gate that stopped excluding it would put `ledger:purge` in
	// the answer and nothing else could.
	exec(`INSERT INTO role_permissions (id, role_id, permission_id, deleted_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL),
	      (gen_random_uuid(), $1, $3, NULL)`, machineRoleHeld, machinePermLive, machinePermRevoked)
	exec(`INSERT INTO role_permissions (id, role_id, permission_id, deleted_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL)`, machineRoleRetired, machinePermOnlyViaRetired)
	exec(`INSERT INTO role_permissions (id, role_id, permission_id, deleted_at) VALUES
	      (gen_random_uuid(), $1, $2, NULL)`, machineRoleRevokedFor, machinePermLive)

	// THE CLAIM CATALOG, one definition per `appliesTo` member plus a retired one.
	// The `user` row is the assertion: it belongs to the same tenant and must never
	// reach a client token.
	for _, c := range []struct{ id, name, applies, archived string }{
		{claimForClients, "x_fleet", "client", "NULL"},
		{claimForBoth, "x_region", "both", "NULL"},
		{claimForUsers, "x_desk", "user", "NULL"},
		{claimRetired, "x_legacy", "client", "NOW()"},
	} {
		exec(`INSERT INTO claims (id, tenant_id, name, value_type, applies_to,
		                          default_value, description, deleted_at)
		      VALUES ($1, $2, $3, 'string', $4, NULL, '', `+c.archived+`)`,
			c.id, machineTenant, c.name, c.applies)
	}

	// The client's own values. The one on the RETIRED definition is the second half
	// of that gate: holding a value must not resurrect a definition nobody offers.
	exec(`INSERT INTO client_claims (id, client_id, claim_id, value, deleted_at) VALUES
	      (gen_random_uuid(), $1, $2, 'fleet-a', NULL),
	      (gen_random_uuid(), $1, $3, 'gone', NULL),
	      (gen_random_uuid(), $1, $4, 'removed', NOW())`,
		machineClient, claimForClients, claimRetired, claimForBoth)

	// The allow-list: one live range and one REVOKED entry. The revoked one is the
	// property this fixture exists for — an archived restriction that kept admitting
	// would read as revoked in the API and still be enforced.
	exec(`INSERT INTO client_allowed_cidrs (id, client_id, cidr, label, deleted_at) VALUES
	      (gen_random_uuid(), $1, '203.0.113.0/24', 'office', NULL),
	      (gen_random_uuid(), $1, '198.51.100.0/24', 'old NAT', NOW())`, machineClient)
}

// ── step one ────────────────────────────────────────────────────────────────

// The load answers the row and fills the tenant columns the join declares.
func TestLive_LoadClientByIDCarriesTheTenantJoin(t *testing.T) {
	reader := machineReader(t)

	client, err := reader.LoadClientByID(context.Background(), domain.NewID(machineClient))
	if err != nil {
		t.Fatalf("loading the client: %v", err)
	}
	if client == nil {
		t.Fatal("the signable client did not come back")
	}
	if client.Name != "Billing integration" {
		t.Errorf("name = %q", client.Name)
	}
	if client.TenantWorkspace != "machine-walk" {
		t.Errorf("tenantWorkspace = %q — the token's claim is built from this", client.TenantWorkspace)
	}
	if client.TenantStatus != vos.TenantStatusActive.Value() {
		t.Errorf("tenantStatus = %q — the usability check is built from this", client.TenantStatus)
	}
	if client.SecretHash != "stored-hash" {
		t.Errorf("secretHash = %q — a redacted read here would refuse every sign-in", client.SecretHash)
	}
}

// An ARCHIVED client does not come back at all, and the absence is (nil, nil).
//
// The repository's default scope is what does it. Asserted because the handler
// treats (nil, nil) as "no such client" and refuses — so a scope that stopped
// applying would let a retired integration keep signing in.
func TestLive_LoadClientByIDDropsAnArchivedClient(t *testing.T) {
	reader := machineReader(t)

	client, err := reader.LoadClientByID(context.Background(), domain.NewID(machineClientArchived))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client != nil {
		t.Error("an archived client came back; the loader's scope is what retires it")
	}
}

// A SUSPENDED client in a SUSPENDED tenant DOES come back — the reader reads, the
// handler decides.
//
// The split matters: if the reader dropped it, the handler's "credential valid but
// client or tenant not usable" branch would be unreachable and its log line would
// never be written, so a support ticket about a lapsed customer would show a bare
// "no such client".
func TestLive_LoadClientByIDReturnsASuspendedRowForTheHandlerToJudge(t *testing.T) {
	reader := machineReader(t)

	client, err := reader.LoadClientByID(context.Background(), domain.NewID(machineClientSuspended))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("a suspended client did not come back; the handler needs the row to refuse it correctly")
	}
	if client.Status != vos.ClientStatusSuspended.Value() {
		t.Errorf("status = %q, want suspended", client.Status)
	}
	if client.TenantStatus != vos.TenantStatusSuspended.Value() {
		t.Errorf("tenantStatus = %q, want suspended", client.TenantStatus)
	}
}

// ── step two ────────────────────────────────────────────────────────────────

func resolveMachine(t *testing.T, reader *ClientAuthenticationReader) ClientSignInBundle {
	t.Helper()
	ctx := context.Background()
	client, err := reader.LoadClientByID(ctx, domain.NewID(machineClient))
	if err != nil || client == nil {
		t.Fatalf("loading the client: %v (nil=%v)", err, client == nil)
	}
	bundle, err := reader.ResolveClientSignIn(ctx, client)
	if err != nil {
		t.Fatalf("resolving the sign-in: %v", err)
	}
	return bundle
}

// The grant walk, with every gate applied.
func TestLive_ResolveClientSignInAppliesEveryGrantGate(t *testing.T) {
	bundle := resolveMachine(t, machineReader(t))

	roles := map[string]bool{}
	for _, r := range bundle.Roles {
		roles[r.Key] = true
	}
	if !roles["held"] {
		t.Error("the live role is missing")
	}
	if !roles["grants-nothing"] {
		t.Error("a role that confers nothing was dropped; it is still a role the client holds")
	}
	if roles["retired"] {
		t.Error("a RETIRED role reached the token through a perfectly live edge")
	}
	if roles["revoked-grant"] {
		t.Error("a REVOKED grant still conferred its role; the anchor's own scope is what drops it")
	}

	perms := map[string]bool{}
	for _, p := range bundle.Permissions {
		perms[p.Resource+":"+p.Action] = true
	}
	if !perms["invoice:read"] {
		t.Error("the live permission is missing")
	}
	if perms["ledger:purge"] {
		t.Error("the retired role's permission leaked; nothing else grants it, so this is that gate failing")
	}
	if perms["invoice:delete"] {
		t.Error("a RETIRED catalog entry reached the token")
	}
}

// THE `appliesTo` PREDICATE — the assertion this whole live file exists for.
//
// A client token resolves the tenant's `client` and `both` definitions and never
// its `user` ones. The `user` row here belongs to the SAME tenant, so nothing but
// the predicate keeps it out.
func TestLive_ResolveClientSignInReadsOnlyTheClientHalfOfTheCatalog(t *testing.T) {
	bundle := resolveMachine(t, machineReader(t))

	names := map[string]bool{}
	for _, d := range bundle.Definitions {
		names[d.Name] = true
	}
	if !names["x_fleet"] {
		t.Error("a `client` definition did not reach the machine token")
	}
	if !names["x_region"] {
		t.Error("a `both` definition did not reach the machine token; `both` means either kind may hold one")
	}
	if names["x_desk"] {
		t.Error("a `user`-scoped definition reached a MACHINE token — the appliesTo predicate is not doing its job")
	}
	if names["x_legacy"] {
		t.Error("a RETIRED definition reached the token; the anchor's own scope is what drops it")
	}
}

// The claim VALUES, with the archive gate on the entry itself.
func TestLive_ResolveClientSignInReadsTheClientsOwnClaimValues(t *testing.T) {
	bundle := resolveMachine(t, machineReader(t))

	if got := bundle.ClaimValues[domain.NewID(claimForClients)]; got != "fleet-a" {
		t.Errorf("the client's own value for x_fleet = %q, want fleet-a", got)
	}
	if _, present := bundle.ClaimValues[domain.NewID(claimForBoth)]; present {
		t.Error("an ARCHIVED claim entry still carried its value; removing a value must take effect at the next token")
	}
	// The value on the retired DEFINITION is still read — the entry is live — and it
	// mints nothing, because the resolution walks the catalog and the definition is
	// not in it. Asserted so the two gates stay visibly independent.
	if got := bundle.ClaimValues[domain.NewID(claimRetired)]; got != "gone" {
		t.Errorf("the entry on a retired definition = %q; the entry gate and the definition gate are separate", got)
	}
}

// THE ALLOW-LIST, and the gate that makes revoking a range mean something.
func TestLive_ResolveClientSignInDropsARevokedRange(t *testing.T) {
	bundle := resolveMachine(t, machineReader(t))

	if len(bundle.AllowedCIDRs) != 1 || bundle.AllowedCIDRs[0] != "203.0.113.0/24" {
		t.Fatalf("allowedCIDRs = %v, want only the live range", bundle.AllowedCIDRs)
	}
	// And the decision built on it behaves accordingly: the revoked range must stop
	// admitting the moment it is archived, or a restriction that reads as revoked in
	// the API is still enforced.
	if !AddressAllowed("203.0.113.7", bundle.AllowedCIDRs) {
		t.Error("the live range refused an address inside it")
	}
	if AddressAllowed("198.51.100.7", bundle.AllowedCIDRs) {
		t.Error("a REVOKED range still admitted a caller")
	}
}

// A client with no rows at all resolves to an empty bundle rather than an error —
// and an empty allow-list means any address, which is the fail-open decision.
func TestLive_ResolveClientSignInOnAClientWithNothingGranted(t *testing.T) {
	reader := machineReader(t)
	ctx := context.Background()

	client, err := reader.LoadClientByID(ctx, domain.NewID(machineClientSuspended))
	if err != nil || client == nil {
		t.Fatalf("loading: %v", err)
	}
	bundle, err := reader.ResolveClientSignIn(ctx, client)
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if len(bundle.Roles) != 0 || len(bundle.Permissions) != 0 {
		t.Errorf("a client with no grants resolved to %+v", bundle)
	}
	if len(bundle.AllowedCIDRs) != 0 {
		t.Errorf("allowedCIDRs = %v, want empty", bundle.AllowedCIDRs)
	}
	if !AddressAllowed("203.0.113.7", bundle.AllowedCIDRs) {
		t.Error("an empty allow-list refused an address; empty means ANY, by design")
	}
	// Its tenant is a different one, and it declares no claims at all.
	if len(bundle.Definitions) != 0 {
		t.Errorf("definitions = %+v; the read is scoped to the client's OWN tenant", bundle.Definitions)
	}
}
