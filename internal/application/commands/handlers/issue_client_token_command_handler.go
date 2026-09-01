// Hand-written, and not a hook: no generator declares this file.
//
// THE MACHINE SIGN-IN — the operation that turns a client id and a server-minted
// secret into an access token. It is the sibling of the user sign-in one file over,
// and the split between what the framework owns and what we own is identical: the
// framework's authcore.Issuer owns everything security-critical about the TOKEN and
// authenticates NOBODY. Deciding who is authentic is this file's entire job.
//
// ── WHAT IS DELIBERATELY MISSING, AND WHY ────────────────────────────────────
//
// THERE IS NO REFRESH. RFC 6749 §4.4.3 says the client-credentials grant SHOULD NOT
// issue one, and the reason is not ceremony: a refresh token exists for a credential
// that cannot be re-presented — a password the human typed once. A client holds its
// secret in a file or a vault by construction, so it can reauthenticate whenever it
// likes. Issuing a refresh token would trade "the secret crosses the wire on every
// renewal" for "a refresh token crosses the wire on every renewal AND the secret is
// still stored" — two long-lived credentials where there was one, plus a row per
// restarted process in a table whose subject never logs out. So there is no
// RefreshClientTokenHandler, and there is no route for one.
//
// THERE IS NO must_change_password ANALOGUE. A machine has no credential it can be
// told to rotate itself; rotation is an operator's act through
// POST /clients/{id}/secret.
//
// THERE IS NO groups CLAIM. Client has no groups — the model gate refused them
// outright — so this token reaches roles by exactly one path. The claim is ABSENT
// rather than empty: an empty list would say "this machine belongs to no groups",
// which implies groups are a thing it could belong to.
//
// THE TTL IS NOT CHOSEN HERE. TokenRequest.TTL is left at its zero value, so the
// Issuer applies auth.issuer.tokenTtlSeconds — the same lifetime a user token gets.
// A longer machine lifetime was reachable (the field is honoured up to
// maxTokenTtlSeconds) and was refused: this service wires no Wiring.TokenChecker, so
// nothing revokes a token already in flight, and while that is true the TTL IS the
// revocation time.
//
// ── THE LOCKOUT DOES NOT APPLY HERE, AND THAT IS A DECISION ──────────────────
//
// Every outcome is COUNTED — the journal writes the same rows under
// identity_kind = 'client', so the forensic and alerting surface is whole — but
// LockedUntil is never consulted on this route and RecordLocked is unreachable from
// it. The lockout exists to make guessing a ~30-bit human password expensive. A
// client secret is 32 bytes from crypto/rand, so no rate at any budget guesses it,
// and the lock buys nothing against the attack it was designed for. What it WOULD
// buy an attacker is a lever: a client id is not a secret — it is the row id, the
// `sub` of every token that client presents, and a column of GET /clients — so five
// wrong guesses would take a production integration off the air for fifteen minutes,
// repeatable forever. specs/implement/client-credentials-token/plan.md §3 holds the
// argument; it overrides the line specs/scaffold-entity/client/spec.md §F wrote.

package handlers

import (
	"maps"
	"strings"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/dtos"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/utils"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// ── the result ──────────────────────────────────────────────────────────────

// ── the sign-in ─────────────────────────────────────────────────────────────

// IssueClientTokenHandler turns a client id and a secret into an access token.
type IssueClientTokenHandler struct {
	Store    dtos.ClientAuthenticationStore
	Attempts dtos.AttemptRecorder
	Issuer   dtos.AccessTokenIssuer

	// Events carries the per-attempt record that the rollup table does not keep. A
	// nil publisher disables the announcements and changes nothing else — the same
	// semantic the framework gives its own event port, and what lets a test drive
	// the utils.Refusal branches without one.
	Events dtos.AuthenticationEventPublisher
}

// journal is this route's view of the shared path to the auxiliary tables and the
// log stream, bound to the subject kind it authenticates.
//
// Bound to utils.IdentityKindClient at construction, which is what keeps a machine
// attempt off a user's counter row: the attempt table's natural key includes the
// kind, so a client id that happened to spell an e-mail address still counts on its
// own row.
func (h *IssueClientTokenHandler) journal() utils.Journal {
	return utils.NewJournal(h.Attempts, h.Events, utils.IdentityKindClient)
}

// Handle authenticates and mints.
//
// THE ORDER OF THE CHECKS IS THE SECURITY PROPERTY, not a style choice:
//
//  1. no row for this id → burn a verification, then refuse.
//  2. a row exists → verify the secret FIRST, before looking at status. Every
//     found-row path then costs the same digests, so a suspended client and a wrong
//     secret are indistinguishable from outside.
//  3. only then, the client and its tenant have to be usable.
//  4. and only then, the origin address has to be one the client allows — LAST,
//     because it is the check that needs the bundle, and because refusing it any
//     earlier would tell a caller holding a wrong secret something about the
//     client's network configuration.
//
// Every utils.Refusal is the same notification, the same field name and the same 401.
func (h *IssueClientTokenHandler) Handle(ctx *configuration.AppContext, cmd *commands.IssueClientTokenCommand) (commands.ClientTokenResult, error) {
	// Trimmed but NOT lowercased: this is a UUID and the attempt log counts by this
	// exact string, so normalising case would be inventing a second spelling. The
	// user route lowercases because vos.Email stores addresses that way.
	clientID := strings.TrimSpace(cmd.ClientID)
	ip := ctx.ClientIP()
	journal := h.journal()

	// NO LOCKOUT PROBE, and the file header holds the argument. The counters below
	// still move on every outcome; nothing reads them to refuse.

	client, err := h.Store.LoadClientByID(ctx, domain.NewID(clientID))
	if err != nil || client == nil {
		// THE ANSWER IS THE SAME, THE RECORD IS NOT. The caller is refused
		// identically whether the id is unknown or the store Failed: distinguishing
		// them would mean 500 for one and 401 for the other, and a caller could then
		// learn an id exists by finding an input that changes the status code.
		//
		// The LOG, which no attacker reads, keeps the difference. A store that
		// answered (nil, nil) established the id is not here — false. One that
		// answered an error established nothing — nil. Writing false in that second
		// case would put a claim in the table that nobody verified.
		var existed *bool
		if err == nil {
			absent := false
			existed = &absent
		}
		h.Store.BurnSecretVerification()
		reason := "client sign-in Failed: no client for this identity"
		if err != nil {
			reason = "client sign-in Failed: identity lookup could not be performed"
		}
		if rerr := journal.Failed(ctx, clientID, ip, reason, existed); rerr != nil {
			return commands.ClientTokenResult{}, rerr
		}
		return commands.ClientTokenResult{}, utils.Refusal(InvalidClientCredentialsNotification{})
	}

	// From here the identity provably exists, and every remaining branch says so.
	existed := true

	if !h.Store.SecretMatches(cmd.ClientSecret, client) {
		// One reason for both halves of the credential. A secret that WAS this
		// client's and whose grace window closed is a wrong secret now, and saying
		// so separately would tell whoever holds a rotated-out value that they once
		// had the right one.
		if rerr := journal.Failed(ctx, clientID, ip,
			"client sign-in Failed: credential rejected", &existed); rerr != nil {
			return commands.ClientTokenResult{}, rerr
		}
		return commands.ClientTokenResult{}, utils.Refusal(InvalidClientCredentialsNotification{})
	}

	if !clientIsUsable(client) {
		// A suspended client or a withdrawn tenant is a FAILURE in the log, not a
		// success: nobody got in. The one branch where the credential was RIGHT and
		// the answer is still a utils.Refusal — indistinguishable to the caller by design;
		// on the stream it is the line that explains a support ticket in one read.
		if rerr := journal.Failed(ctx, clientID, ip,
			"client sign-in Failed: credential valid but client or tenant not usable", &existed); rerr != nil {
			return commands.ClientTokenResult{}, rerr
		}
		return commands.ClientTokenResult{}, utils.Refusal(InvalidClientCredentialsNotification{})
	}

	// STEP TWO. Everything a token says, in one concurrent burst.
	//
	// NOT a credential utils.Refusal, and NOT an attempt worth counting: the caller proved
	// who they are and this service Failed them. Recording a failure here would let a
	// database problem count against the very integrations it is already failing. It
	// escapes as an exception → 500.
	bundle, err := h.Store.ResolveClientSignIn(ctx, client)
	if err != nil {
		return commands.ClientTokenResult{}, err
	}

	// WHERE FROM — the last gate, and the only one this service applies to a machine
	// that a person never meets.
	//
	// It needs the bundle, so it cannot be asked earlier; and it SHOULD not be, since
	// the caller has already proved the secret by the time it runs. An empty
	// collection admits any address (fail-open, and the empty array served on the
	// read is what tells an operator which state a client is in); a non-empty one
	// with no usable origin address refuses (fail-closed, because the operator stated
	// a restriction).
	if !infra.AddressAllowed(ip, bundle.AllowedCIDRs) {
		// THE REASON RIDES ON THE STREAM AND NEVER IN THE ANSWER. Telling the holder
		// of a stolen secret that only the NETWORK was wrong confirms the secret is
		// good; the operator whose egress address changed reads this line instead.
		if rerr := journal.Failed(ctx, clientID, ip,
			"client sign-in Failed: origin address outside the client's allowed ranges",
			&existed); rerr != nil {
			return commands.ClientTokenResult{}, rerr
		}
		return commands.ClientTokenResult{}, utils.Refusal(InvalidClientCredentialsNotification{})
	}

	// ONE resolution, two readers. The token below and the profile at the bottom of
	// this function both receive this exact map.
	//
	// `false` is the restriction flag: there is no must-change-password state for a
	// machine, so nothing here ever mints an empty map for that reason.
	customClaims := utils.ResolveClaimChain(false, bundle.ClaimValues, bundle.Definitions)

	access, err := h.Issuer.Issue(ctx, authcore.TokenRequest{
		Subject: client.ID.Value(),
		Claims:  buildClientClaims(client, bundle, customClaims),
		// TTL deliberately unset — see the file header.
	})
	if err != nil {
		return commands.ClientTokenResult{}, err
	}

	// Recorded LAST, once the sign-in has actually Succeeded — the same ordering the
	// user route keeps, so a counter is never moved for a sign-in that had not
	// happened yet.
	if rerr := journal.Succeeded(ctx, clientID, ip); rerr != nil {
		return commands.ClientTokenResult{}, rerr
	}

	return commands.ClientTokenResult{
		AccessToken: access.Token,
		TokenType:   utils.TokenTypeBearer,
		ExpiresAt:   access.ExpiresAt.Unix(),
		Client:      buildClientProfile(client, bundle, customClaims),
	}, nil
}

// ── the shared decisions ────────────────────────────────────────────────────

// clientIsUsable reports whether this client may hold a token at all.
//
// TWO conditions, and the second is the one that is easy to forget. The client
// itself must be active — a suspended integration is refused — and its owning TENANT
// must not be commercially suspended, because a customer who stopped paying should
// not be issuing credentials. Trial and active tenants both pass.
//
// The archive stamp needs no check here: the loader's default scope already refuses
// archived rows, so an archived client never reaches this function.
func clientIsUsable(client *schemas.SignInClient) bool {
	if client.Status != vos.ClientStatusActive.Value() {
		return false
	}
	// The joined column, filled on every load by the reader's InnerJoin into Tenant.
	// An empty value means the join found nothing, which for an INNER join over a NOT
	// NULL key cannot happen — but reading it as "unusable" is the fail-closed
	// direction if it ever does.
	if client.TenantStatus == "" || client.TenantStatus == vos.TenantStatusSuspended.Value() {
		return false
	}
	return true
}

// buildClientClaims assembles the access token's claim set.
//
// SEVEN NAMES, AND THE THREE THAT ARE ABSENT ARE AS DELIBERATE AS THE SEVEN:
//
//	identity_kind      "client" — what wakes Client's own row rules
//	tenant_id          read by the framework's Identity across the whole mesh
//	tenant_workspace   no other service can resolve this service's tenant UUID
//	name               the audit trail's analogue of a person's e-mail
//	permissions        read by the framework's Identity; the wire vocabulary
//	roles              stable keys, never display names
//	(sub)              the Issuer's own, from TokenRequest.Subject
//
//	email                 ABSENT — a machine has none
//	groups                ABSENT — Client has no groups, so an empty list would lie
//	must_change_password  ABSENT — there is no such state for a machine
//
// THE TENANT-DEFINED CLAIMS ARE MERGED UNDER THE FIXED SET, never over it — the same
// ordering the user path uses, and for the same reason: the two guards fail in
// opposite directions, so if the reserved-prefix rule is ever wrong the consequence
// is a tenant claim that quietly does not appear, rather than `permissions` or
// `tenant_id` being replaced by a value the tenant chose.
func buildClientClaims(
	client *schemas.SignInClient, bundle infra.ClientSignInBundle, custom map[string]any,
) map[string]any {
	claims := make(map[string]any, len(custom)+6)
	maps.Copy(claims, custom)
	claims[utils.ClaimIdentityKind] = utils.IdentityKindClient
	claims[utils.ClaimTenantID] = client.TenantID.Value()
	claims[utils.ClaimTenantWorkspace] = client.TenantWorkspace
	claims[utils.ClaimName] = client.Name
	claims[utils.ClaimPermissions] = utils.RenderPermissions(bundle.Permissions)
	claims[utils.ClaimRoles] = utils.KeysOf(bundle.Roles)
	return claims
}

// buildClientProfile assembles the richer shape the response body carries.
//
// Permissions is filled from utils.RenderPermissions — the SAME call buildClientClaims
// makes — rather than re-derived, which is the anti-drift rule the user path learned
// by shipping the bug: a body advertising more than the token carries would have the
// integration attempting actions every request then refuses.
//
// There is no utils.EffectivePermissions equivalent here because there is nothing to
// restrict: the one restriction in this service is the must-change-password session,
// and a machine has no such state.
func buildClientProfile(
	client *schemas.SignInClient, bundle infra.ClientSignInBundle, custom map[string]any,
) commands.AuthenticatedClientResult {
	return commands.AuthenticatedClientResult{
		ID:              client.ID.Value(),
		Name:            client.Name,
		Status:          client.Status,
		TenantID:        client.TenantID.Value(),
		TenantWorkspace: client.TenantWorkspace,
		Roles:           utils.NamedGrantsOf(bundle.Roles),
		Permissions:     utils.RenderPermissions(bundle.Permissions),
		Claims:          custom,
	}
}
