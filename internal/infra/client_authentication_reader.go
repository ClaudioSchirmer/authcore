// Hand-written, and not a hook: no generator declares this file.
//
// THE READ SIDE OF A MACHINE SIGN-IN, and nothing else. It answers two questions:
// "which integration is this id" and "everything a token has to say about it".
//
// IT DOES NOT ENTER THROUGH AN AGGREGATE, for the reason its user-side twin does
// not: a sign-in has no invariant to protect and no lifecycle to drive — it reads
// rows. Loading the Client aggregate would hydrate three collections, two of which
// this endpoint does not want, and its read joins cannot carry the archive gates
// the token needs. The schemas next door describe exactly the shapes this endpoint
// asks for, and nothing else in the service uses them.
//
//	one statement   the client and its tenant
//	three more      grants, claim values, definitions, allowed ranges — concurrent
//
// FOUR CONCURRENT, NOT THREE: the allow-list rides in the same burst rather than
// in a read of its own. It costs no extra latency there, and refusing an address
// before resolving grants would have cost a second round trip on the path where
// the secret was CORRECT — which is not a path worth optimising against.
//
// TWO STEPS, AND THE REASON IS THE ATTACK PATH — the same one the user reader
// states. LoadClientByID is one cheap indexed read; everything else hides behind
// it. Collapsing them would make every attempt against an unknown id cost five
// statements instead of one.

package infra

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/infra/db/command/read"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
)

// ClientAuthenticationReader serves the machine sign-in. Four repositories, all
// Direct, all anchored on schemas this endpoint owns.
//
// The claim DEFINITIONS repository is deliberately its own rather than borrowed
// from AuthenticationReader: the two readers are constructed independently by the
// same feature, and reaching into another struct's unexported field to save one
// cheap value would couple them for nothing.
type ClientAuthenticationReader struct {
	clients     *read.DirectRepository[schemas.SignInClient]
	grants      *read.DirectRepository[schemas.ClientRoleGrant]
	claimValues *read.DirectRepository[schemas.HeldClientClaimValue]
	definitions *read.DirectRepository[schemas.ClaimDefinition]
	ranges      *read.DirectRepository[schemas.ClientAllowedRange]
}

// NewClientAuthenticationReader builds the reader and its declared traversals.
//
// Every identifier below is validated at CONSTRUCTION — the schemas, the anchors,
// the foreign keys, the mapped fields and the type of every field a left join can
// leave NULL. A column that stopped resolving aborts the BOOT naming the field,
// instead of shipping a read that quietly matches nothing at three in the morning.
func NewClientAuthenticationReader(engine core.RelationalEngine) *ClientAuthenticationReader {
	return &ClientAuthenticationReader{
		clients: read.NewDirectRepository[schemas.SignInClient](
			engine, schemas.SignInClientSchema()).
			WithJoins(read.InnerJoin(schemas.TenantSchema().AsDirectSchema()).
				On("tenant_id").
				Field("TenantWorkspace", "workspace").
				Field("TenantStatus", "status")),

		grants: newClientGrantRepository(engine),

		claimValues: read.NewDirectRepository[schemas.HeldClientClaimValue](
			engine, schemas.HeldClientClaimValueSchema()),

		definitions: read.NewDirectRepository[schemas.ClaimDefinition](
			engine, schemas.ClaimDefinitionSchema()),

		ranges: read.NewDirectRepository[schemas.ClientAllowedRange](
			engine, schemas.ClientAllowedRangeSchema()),
	}
}

// newClientGrantRepository walks client_roles → roles → role_permissions →
// permissions, entering by the client's own index.
//
// Both hops out of the role are LEFT joins because both are optional: a role that
// confers nothing is still a role the client holds, and it has to survive the read
// that asks what it confers.
func newClientGrantRepository(engine core.RelationalEngine) *read.DirectRepository[schemas.ClientRoleGrant] {
	roles, grants, catalog := schemas.ClientGrantJoins()
	return read.NewDirectRepository[schemas.ClientRoleGrant](engine, schemas.ClientRoleGrantSchema()).
		WithJoins(read.InnerJoin(roles).On("role_id").
			Field("RoleKey", "role_key").
			Field("RoleName", "name").
			Field("RoleArchivedAt", "archived_at").
			Then(read.LeftJoin(grants).On("id").
				Field("GrantArchivedAt", "archived_at").
				Then(read.LeftJoin(catalog).On("permission_id").
					Field("Resource", "resource_name").
					Field("Action", "action_name").
					Field("PermissionArchivedAt", "archived_at"))))
}

// ── step one: the account ───────────────────────────────────────────────────

// LoadClientByID finds the integration behind an id. ONE statement.
//
// ABSENCE IS (nil, nil), NOT AN ERROR, and the distinction is the whole point. The
// caller refuses both cases identically — it must, or the status code becomes an
// existence oracle — but it RECORDS them differently: a miss is logged with
// identity_existed = false, a genuine failure with NULL, because in the second case
// nobody knows.
//
// ACTIVE ROWS ONLY — the repository's default scope — and a LIVE TENANT, which the
// predicate states. The tenant gate is DEPTH rather than the mechanism: Tenant's
// own rules force Status to suspended when it is archived, and the sign-in refuses
// a suspended tenant, so through the API the two states cannot come apart. What
// this catches is a row that reached `archived_at` without going through the
// aggregate — a migration, a support script, a hand-run UPDATE. It costs no round
// trip: the subquery rides inside the statement this already issues.
func (r *ClientAuthenticationReader) LoadClientByID(
	ctx context.Context, id domain.ID,
) (*schemas.SignInClient, error) {
	// Limit(2) rather than FindOne: one row is the answer, two is a data problem
	// worth naming, and this keeps the MISS cheap — which is what every attempt
	// against an unknown id gets.
	found, err := r.clients.FindAll(ctx, criteria.Where(criteria.And(
		criteria.Eq("ID", id),
		criteria.Exists(criteria.Sub(schemas.TenantSchema().AsDirectSchema()).
			Where(criteria.Eq("ID", criteria.Outer("TenantID")))),
	)).Limit(2))
	switch {
	case err != nil:
		return nil, fmt.Errorf("client authentication reader: load client: %w", err)
	case len(found) == 0:
		return nil, nil
	case len(found) > 1:
		return nil, fmt.Errorf(
			"client authentication reader: load client: %d rows for one id", len(found))
	}
	return &found[0], nil
}

// ── step two: everything a token says ───────────────────────────────────────

// ClientSignInBundle is what the token and the response body are built from.
type ClientSignInBundle struct {
	// Every role this client holds, live ones only — INCLUDING roles that confer
	// nothing, each with its display name.
	Roles []NamedGrant
	// What those roles confer, deduplicated, live catalog entries only.
	Permissions []vos.PermissionKey
	// Level 1 of the claim chain: this client's own values, by definition id.
	ClaimValues map[domain.ID]string
	// Level 2, and the vocabulary the resolution walks.
	Definitions []schemas.ClaimDefinition
	// The network ranges this client may authenticate from. EMPTY MEANS ANY
	// ADDRESS — fail-open by design, and the reason vos.CIDRBlock refuses
	// 0.0.0.0/0 and ::/0 is so there is exactly one spelling of "no restriction".
	AllowedCIDRs []string
}

// ResolveClientSignIn reads everything the token needs, in FOUR CONCURRENT
// statements.
//
// None of them depends on another's answer: each resolves what it needs from the
// client alone, inside the database. So the four cost ONE round trip of latency
// rather than four.
//
// THE GATES, ONE BY ONE, AND WHERE EACH LIVES:
//
//	a revoked GRANT of a role      the grant anchor's own scope
//	a retired ROLE                 RoleArchivedAt, filtered per PAIR below
//	a revoked ROLE→PERMISSION      GrantArchivedAt, filtered per PAIR below
//	a retired PERMISSION           PermissionArchivedAt, filtered per PAIR below
//	a removed CLAIM VALUE          the value anchor's own scope
//	a retired DEFINITION           the definition anchor's own scope
//	a revoked CIDR entry           the range anchor's own scope
//
// The three filtered PER PAIR are the ones that must not become predicates: a role
// arrives as one row per grant, so a WHERE dropping a revoked grant drops the ROLE
// with it — and a role whose every permission was revoked is still a role the client
// holds. The columns come back; the decision is made here, per row.
func (r *ClientAuthenticationReader) ResolveClientSignIn(
	ctx context.Context, client *schemas.SignInClient,
) (ClientSignInBundle, error) {
	var (
		grantRows   []schemas.ClientRoleGrant
		valueRows   []schemas.HeldClientClaimValue
		definitions []schemas.ClaimDefinition
		rangeRows   []schemas.ClientAllowedRange
		errs        [4]error
		wg          sync.WaitGroup
	)
	wg.Add(4)

	go func() {
		defer wg.Done()
		grantRows, errs[0] = r.grants.FindAll(ctx,
			criteria.Where(criteria.Eq("ParentID", client.ID)))
	}()

	go func() {
		defer wg.Done()
		valueRows, errs[1] = r.claimValues.FindAll(ctx,
			criteria.Where(criteria.Eq("ParentID", client.ID)))
	}()

	go func() {
		defer wg.Done()
		// The `both` member is included because it means "either identity kind may
		// hold this", NOT "only a principal that is both" — there is no such
		// principal. This is the client half of the same set the user path reads;
		// a definition scoped to `user` mints nothing here.
		definitions, errs[2] = r.definitions.FindAll(ctx, criteria.Where(criteria.And(
			criteria.Eq("TenantID", client.TenantID),
			criteria.In("AppliesTo",
				vos.ClaimAppliesToClient.Value(),
				vos.ClaimAppliesToBoth.Value()),
		)))
	}()

	go func() {
		defer wg.Done()
		rangeRows, errs[3] = r.ranges.FindAll(ctx,
			criteria.Where(criteria.Eq("ParentID", client.ID)))
	}()

	wg.Wait()
	// THE FIRST ERROR REFUSES THE WHOLE ANSWER. A partial bundle is the one thing
	// this must never return: half the roles would mint a token that authorizes
	// less — or, read the other way by a consumer, silently more. And a partial
	// ALLOW-LIST is worse than either: a read that lost the one entry covering the
	// caller would refuse a legitimate integration, while a read that lost every
	// entry would look like "no restriction" and admit the whole internet.
	for i, err := range errs {
		if err != nil {
			what := [...]string{"grants", "claim values", "claim definitions", "allowed ranges"}[i]
			return ClientSignInBundle{}, fmt.Errorf("client authentication reader: %s: %w", what, err)
		}
	}
	return assembleClient(grantRows, valueRows, definitions, rangeRows), nil
}

// assembleClient collapses the fan-out into the answers a token is built from.
//
// EVERY GATE THAT IS NOT THE FRAMEWORK'S IS APPLIED HERE, per row, for the reason
// the shape forces: a role arrives once per grant it confers, so a predicate
// excluding a revoked grant would exclude the ROLE with it.
func assembleClient(
	grants []schemas.ClientRoleGrant,
	values []schemas.HeldClientClaimValue,
	definitions []schemas.ClaimDefinition,
	ranges []schemas.ClientAllowedRange,
) ClientSignInBundle {
	var (
		roles      = map[string]NamedGrant{}
		perms      permissionSet
		claimValue = make(map[domain.ID]string, len(values))
		allowed    = make([]string, 0, len(ranges))
	)

	for _, row := range grants {
		if row.RoleArchivedAt != nil { // a retired role confers nothing and is not held
			continue
		}
		roles[row.RoleKey] = NamedGrant{Key: row.RoleKey, Name: row.RoleName}

		// The three gates this row still has to pass, and the collapse, are
		// permissionSet's — the same ones the user path applies, because the
		// answer must not depend on which kind of principal signed in.
		perms.add(row.Resource, row.Action, row.GrantArchivedAt, row.PermissionArchivedAt)
	}

	for _, row := range values {
		claimValue[row.ClaimID] = row.Value
	}

	for _, row := range ranges {
		allowed = append(allowed, row.CIDR)
	}

	sort.Strings(allowed)

	return ClientSignInBundle{
		Roles:        sortedGrants(roles),
		Permissions:  perms.sorted(),
		ClaimValues:  claimValue,
		Definitions:  definitions,
		AllowedCIDRs: allowed,
	}
}

// ── the credential ──────────────────────────────────────────────────────────

// SecretMatches reports whether the presented secret is one this client currently
// accepts — the live one, or the retiring one while its window is open.
//
// TWO VERIFIES ON THE MISS PATH, and that is exactly what the Client spec's Q4 cost
// argument was about: the digest is SHA-256 rather than Argon2id precisely so a
// rotation in flight can be checked for free. Two Argon2id verifies on an
// unauthenticated route would allocate 38 MiB and burn 200 ms per wrong guess.
//
// THE ORDER IS LIVE-FIRST because that is the overwhelmingly common case; the
// retiring hash is reached only when the live one did not match, so an ordinary
// sign-in costs one digest.
//
// THE WINDOW IS COMPARED AGAINST THE APP CLOCK, not the database's. The stamped
// columns of this service are written under `relational.clock: db`, but reading one
// back to decide "has this expired" is a comparison, not a stamp, and the lockout
// window already answers its own question the same way. On a 24-hour default grace
// the pod-to-pod drift a NTP-synced fleet carries is not a quantity this decision
// can notice.
func (r *ClientAuthenticationReader) SecretMatches(presented string, client *schemas.SignInClient) bool {
	if client == nil {
		return false
	}
	if clientHasher.Matches(presented, client.SecretHash) {
		return true
	}
	// NO ROTATION IN FLIGHT is the ordinary state, and both columns move together:
	// a hash with no expiry, or an expiry with no hash, is a row nothing in this
	// service can write. Reading them as "no previous secret" is the fail-closed
	// direction if one ever appears.
	if client.PreviousSecretHash == nil || client.PreviousSecretExpiresAt == nil {
		return false
	}
	if !time.Now().UTC().Before(client.PreviousSecretExpiresAt.UTC()) {
		// The window closed. The hash is still on the row until the next rotation
		// overwrites it, and it must stop authenticating the moment it expires —
		// otherwise a `gracePeriodSeconds: 0` rotation, which is what somebody does
		// with a LEAKED credential, would not actually kill anything.
		return false
	}
	return clientHasher.Matches(presented, *client.PreviousSecretHash)
}

// BurnSecretVerification spends one digest and throws the answer away.
//
// IT IS NOT DEAD CODE AND MUST NOT BE OPTIMISED OUT — though it is worth being
// honest about what it buys HERE versus on the user route. There, the gap between
// "no row" and "row, wrong password" is a full Argon2id verification, some fifteen
// milliseconds, and it is plainly measurable from outside; the burn is load-bearing.
// A SHA-256 digest is microseconds and sits well under the noise of the round trip
// that precedes it, so this is cheap insurance rather than the thing holding the
// property up.
//
// It stays for two reasons anyway. The hash algorithm behind ClientService is a
// decision that could be revisited, and a timing-equalisation call that already
// exists cannot be forgotten when it is. And the client id, unlike an e-mail
// address, is not a secret in the first place — it is the `sub` of every token this
// integration presents — so there is little for an enumeration oracle to reveal;
// that is a reason not to spend MORE, not a reason to spend nothing.
//
// The hash it verifies against is derived once, at package init, from a random
// secret: nobody — including this process — knows a plaintext that matches it.
func (r *ClientAuthenticationReader) BurnSecretVerification() {
	clientHasher.Matches("timing-equalisation", secretEqualisationHash)
}

// secretEqualisationHash is the decoy BurnSecretVerification verifies against.
var secretEqualisationHash = func() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// A broken random source at init is not something to degrade past: every
		// credential this process would go on to mint depends on the same source.
		panic("client authentication reader: cannot seed the timing-equalisation hash: " + err.Error())
	}
	return clientHasher.Hash(hex.EncodeToString(buf))
}()

// ── the allow-list ──────────────────────────────────────────────────────────

// AddressAllowed reports whether an origin address may obtain a token for a client
// holding these ranges.
//
// IT IS A PURE FUNCTION AND IT LIVES IN INFRA because the parsing is the whole of
// it: `netip` is stdlib, the stored form is already canonical (vos.CIDRBlock
// refuses a prefix with host bits set, so the unique index tells the truth), and
// there is no policy here that the domain would recognise.
//
// THREE ANSWERS, AND THE MIDDLE ONE IS THE EASY ONE TO GET WRONG:
//
//   - NO RANGES → allowed. Fail-OPEN, and it is a decision the README already
//     fixed: fail-closed would make every newly created client unable to sign in
//     until a second call, a step every provisioning script forgets, whose symptom
//     is a generic 401. The empty array served on the read is what tells an operator
//     which state a client is in.
//   - RANGES BUT NO USABLE ADDRESS → refused. Fail-CLOSED, and the asymmetry with
//     the line above is the point: the operator STATED a restriction, so admitting a
//     caller whose origin nobody could determine would silently void it.
//   - RANGES AND AN ADDRESS → containment, and an unparseable stored range is
//     skipped rather than treated as universal.
//
// WHAT IT CAN AND CANNOT PROMISE, and every document about this feature has to say
// it. The address handed in is AppContext.ClientIP() — what the FRAMEWORK resolved,
// not what this service read off a socket. Since omnicore v0.69.0 that is:
//
//   - with no `http.trustProxy` block, the socket peer. Unforgeable, and the
//     BALANCER's address on any deployment that has one — so an allow-list of real
//     egress ranges refuses everybody there.
//   - with the block declared, the RIGHTMOST UNTRUSTED entry of the forwarded chain.
//     The framework walks it right to left and takes the first hop that is not an
//     allowlisted proxy, so an edge that appends and one that overwrites are both
//     safe, and a caller reaching the service directly cannot forge its own origin.
//
// SO THIS CONTROL IS ONLY AS GOOD AS THAT BLOCK. The framework resolves an address;
// it cannot make an undeclared topology trustworthy. A deployment behind a proxy that
// has not declared `http.trustProxy` should leave allowedCIDRs empty rather than
// believe a restriction it does not have.
//
// AND IN EVERY DEPLOYMENT it constrains where a token is OBTAINED, never where it is
// USED: authcore does not see the requests a client later makes to other services.
func AddressAllowed(ip string, ranges []string) bool {
	if len(ranges) == 0 {
		return true
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	// An IPv4-mapped IPv6 address is the same host written the other way, and a
	// socket peer can arrive in either form. Unmapping first is what keeps
	// "203.0.113.7" matching a 203.0.113.0/24 entry when the listener is dual-stack.
	addr = addr.Unmap()
	for _, raw := range ranges {
		prefix, perr := netip.ParsePrefix(raw)
		if perr != nil {
			// Unreachable through the API — vos.CIDRBlock refuses anything netip
			// cannot parse — and reachable by a migration or a hand-run UPDATE.
			// SKIPPED rather than treated as a match: a range nobody can read must
			// not be the one that admits a caller.
			continue
		}
		if prefix.Addr().Is4In6() {
			// A range stored in the IPv4-mapped form — "::ffff:203.0.113.0/120" —
			// is the same range as 203.0.113.0/24, and the length has to be
			// converted with the address: the mapped prefix counts the 96 bits of
			// the ::ffff: header, which the 4-byte form does not have. Keeping the
			// IPv6 length would build an invalid prefix that matches nothing, which
			// fails CLOSED and would therefore be invisible until an integration
			// could not sign in.
			//
			// Unreachable through the API — vos.CIDRBlock requires the canonical
			// spelling — and reachable by a migration or a hand-run UPDATE.
			if prefix.Bits() < 96 {
				// Not a mapped IPv4 range at all: a prefix short enough to span
				// beyond the ::ffff: header covers addresses that are not IPv4.
				// Skipped rather than misread.
				continue
			}
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
