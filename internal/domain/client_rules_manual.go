// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for Client (7 to implement).
//
// entity:     Client
// spec:       specs/omnicore-gen/client.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-26
//
// Until these are written the service runs and accepts writes — it simply
// does not enforce the invariants the spec described. That is quiet, which
// is exactly why it is worth doing now rather than later.
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

package domain

import (
	"crypto/rand"
	"encoding/base64"
	"strconv"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The prefix every secret this service issues carries, and the number of random
// bytes behind it.
//
// THE PREFIX IS THE POINT, and it is not decoration. 122 bits and 256 bits are
// both uncrackable, so entropy is not what this buys; what it buys is that a
// leaked secret is DETECTABLE. A UUID sitting in a git repository, a CI log or
// a chat message is indistinguishable from the other UUIDs beside it, while
// `acs_` followed by 43 base64url characters is a pattern GitHub secret
// scanning, gitleaks and trufflehog all match — which is how GitHub (`ghp_`),
// Stripe (`sk_live_`) and Slack (`xoxb-`) ship credentials. It also lets this
// service's own log scrubber recognise its own secrets, which no UUID could.
//
// 32 bytes render as exactly 43 base64url characters unpadded, so every secret
// this service has ever issued is the same length — which is one less thing a
// consumer's validation can be wrong about.
const (
	clientSecretPrefix    = "acs_"
	clientSecretRandBytes = 32
)

// The ceiling and the default on the rotation grace window.
//
// A caller may ask for anything from ZERO to a week. Zero is not an omission
// and must stay reachable: rotating a LEAKED secret is exactly the case where
// the old one has to stop working now, and a floor of "at least an hour" would
// make this API useless for the incident it exists to serve. The ceiling exists
// because a window nobody closes is a second live credential nobody is tracking.
const (
	ClientSecretGraceDefaultSeconds = 86400  // 24 h — what a rotation gets when the caller says nothing.
	ClientSecretGraceMaxSeconds     = 604800 // 7 days — a migration may legitimately need one.
)

// The claim value that says the caller is a machine.
//
// It is a bare constant beside the rule that compares against it, not a shared
// vocabulary type: `authentication_attempts.identity_kind` already stores this
// word and its column comment already documents it, so there is nothing here to
// declare — only something to compare. The USER value is not spelled at all,
// because nothing compares against it: an absent claim reads as a user by
// standing the rule down, which is the fail-open direction §B-Q8e chose.
const identityKindClient = "client"

// ActionRotateSecret is the action name the secret rotation dispatches under.
//
// It exists as a constant because the ordinary PATCH and this operation both
// dispatch ModeUpdate, so the action name is the only thing that tells the
// aggregate which of the two it is judging — the same discriminator User's two
// credential operations use, and for the same reason.
const ActionRotateSecret = "RotateSecret"

// customRules is called at the end of the generated BuildRules, with the same
// arguments, and reports a violation the same way: r.AddNotification.
//
// ONE gate per verb, holding every rule that runs on that verb — the shape
// the generated BuildRules already has. A gate per rule reads as a wall of
// near-identical closures and makes the framework dispatch the same verb once
// per rule on every write; rules that share a verb belong in the same block. A
// rule that appears under two gates is still ONE rule: write it as a method
// and call it from both, rather than as two copies that can drift apart.
func (e *Client) customRules(actionName string, service domain.Service, r *domain.Rules) {
	svc, _ := service.(ClientService)

	r.IfInsert(func() {
		// NO CLIENT-CALLER REFUSAL HERE, and its absence is a decision rather than an
		// omission. A client-subject caller holding client:insert creates clients in its
		// tenant like any other caller — see the rotation branch below for the one act
		// that stays on the caller's own row.
		e.refuseUnavailableTenant(svc, r)
		// LAST in the gate, deliberately. Minting derives a hash, and deriving
		// one for a secret the rules above refused is a write nobody asked for
		// — today it would be discarded with the entity, and one refactor from
		// now it would not.
		e.mintCredential(svc)
	})

	r.IfInsertOrUpdate(func() {
		e.refuseUngrantableRoles(svc, r)
		e.refuseUnsettableClaims(svc, r)
	})

	r.IfUpdate(func() {
		// The ordinary PATCH and the rotation both dispatch ModeUpdate, so the
		// ACTION NAME is what tells them apart. Without this switch the rotation
		// would fire on a rename, replacing a live credential because somebody
		// fixed a typo in a description.
		//
		// THE ROW RULE LIVES INSIDE THIS BRANCH and no longer wraps the whole verb:
		// an ordinary patch by a client-subject caller is an ordinary tenant-scoped
		// write. The row decision comes FIRST here, as it does on User's two
		// credential verbs — there is no point validating a window for a rotation
		// the caller may not perform.
		if actionName == ActionRotateSecret {
			e.refuseRotatingAnotherClientsSecret(r)
			e.rotateSecretRules(svc, r)
		}
	})

	r.IfArchive(func() {
		// ── archive-forces-suspended ──
		// A MUTATION, not a validation: set the field and raise nothing. An
		// archived client is never active, so archived+active becomes an
		// unrepresentable state rather than a refused one — there is no state
		// to validate against and no report that has to filter it out.
		//
		// It reaches the ROW and not merely the audit event, because archive is
		// an ordinary full-field write at this pin: it emits the same UPDATE
		// every other verb does, with deleted_at riding along as one more
		// column.
		//
		// It cannot collide with the status transition rule: that one is
		// IfUpdate, this is IfArchive, and the two modes never run together.
		//
		// Verbatim the shape Tenant and User already ship.
		e.Status = vos.ClientStatusSuspended
	})
}

// mintCredential issues the client's first secret.
//
// THE PLAINTEXT LEAVES NO TRACE ON THE ROW. It lands on a field with no column,
// goes into the hasher, and the hash is what is stored. Nothing here logs,
// wraps or copies it, and there is nowhere it could leak to even by accident:
// the field it lives on is in no TableSchema, no outbox payload, no audit event
// and no response.
//
// The hash comes through the SERVICE and not from a crypto package imported
// here, for the reason the Argon2id adapter's own header gives: an aggregate
// that knew which digest answers would be an aggregate that changes when that
// decision does. The RANDOMNESS is different and stays here — "32 bytes from
// the operating system" is not an algorithm choice, it is the only correct
// answer, and routing it through infrastructure would buy a seam nobody would
// ever move.
//
// The two retiring columns are left absent: there is nothing being replaced on
// a first issue, and writing a deadline for a secret that never existed would
// make secretRotationPending true for every brand-new client.
func (e *Client) mintCredential(service ClientService) {
	e.Secret = newClientSecret()

	// A nil service is the framework's own failure mode, not this rule's — it
	// answers ServiceIsRequiredNotification before the rules run — but the type
	// assertion above can still yield nil in a test that builds the entity by
	// hand. Leaving the hash empty is the honest outcome there: no secret
	// matches an empty hash, so the row authenticates nobody.
	if service != nil {
		e.SecretHash = service.HashSecret(e.Secret)
	}
	e.SecretChangedAt = time.Now().UTC()

	// CLEARED, not merely left alone. Both columns are `assignedFrom: derived`,
	// so no caller can send one and in production a fresh entity carries
	// neither — but "there is nothing retiring on a first issue" is an invariant
	// of the ROW, and the rule that owns the row's credential state is where it
	// belongs. A client born mid-rotation would read as one to every consumer of
	// previousSecretExpiresAt.
	e.PreviousSecretHash = nil
	e.PreviousSecretExpiresAt = nil
}

// rotateSecretRules is the branch the rotation runs through.
//
// It is reached only under ActionRotateSecret, so an ordinary PATCH never
// touches a credential — which matters in both directions: the rotation does
// not fire on a rename, and a rename's rules do not have to know a credential
// exists.
func (e *Client) rotateSecretRules(service ClientService, r *domain.Rules) {
	// ── the window ──
	// Asked FIRST, because everything below it writes. A window the caller got
	// wrong must not cost them their working credential.
	if e.GracePeriodSeconds < 0 || e.GracePeriodSeconds > ClientSecretGraceMaxSeconds {
		r.AddNotification("GracePeriodSeconds", InvalidGracePeriodNotification{
			Max: strconv.Itoa(ClientSecretGraceMaxSeconds),
		}, e.GracePeriodSeconds)
		return
	}

	// ── the row must be able to hold a credential ──
	// A suspended client is one somebody deliberately switched off; handing it
	// a fresh secret is the opposite of what that switch meant. Reactivate it
	// first, which is an ordinary PATCH and an ordinary permission.
	if e.Status != vos.ClientStatusActive {
		r.AddNotification("Status", ClientMustBeActiveToRotateNotification{}, e.Status.Value())
		return
	}

	if r.Context() != nil && r.Context().HasErrors() {
		return
	}

	retiring := e.SecretHash

	e.Secret = newClientSecret()
	if service != nil {
		e.SecretHash = service.HashSecret(e.Secret)
	}
	e.SecretChangedAt = time.Now().UTC()

	// ── the overlap ──
	// A window of ZERO is an immediate kill and CLEARS both columns rather than
	// stamping a deadline in the past. The two are indistinguishable to the
	// token path — an expired deadline is not honoured either way — but they are
	// not indistinguishable to a person reading the row: a stamped past deadline
	// says "a rotation is in flight and just ended", and nothing was in flight.
	if e.GracePeriodSeconds == 0 || retiring == "" {
		e.PreviousSecretHash = nil
		e.PreviousSecretExpiresAt = nil
		return
	}

	expires := e.SecretChangedAt.Add(time.Duration(e.GracePeriodSeconds) * time.Second)
	e.PreviousSecretHash = &retiring
	e.PreviousSecretExpiresAt = &expires
}

// refuseUnavailableTenant refuses a client whose owning tenant is missing,
// archived or commercially SUSPENDED.
//
// THREE conditions, and TRIAL PASSES. "Unavailable" is not "not active": a
// trial is a live customer being onboarded, and an integration is often the
// first thing they wire up. Reading this as `Status != active` would break every
// trial.
//
// IfInsert only, exactly as User does it. Re-asking on every update would make
// a client impossible to rename the day their tenant is suspended — a 422 on a
// request whose only change is a label, on rows that still have to be
// administrable. Suspension withholds NEW clients; it does not freeze the ones
// already there.
func (e *Client) refuseUnavailableTenant(service ClientService, r *domain.Rules) {
	if service == nil {
		return
	}
	if service.TenantIsUnavailable(e.TenantID) {
		r.AddNotification("TenantID", ClientTenantDoesNotExistNotification{}, e.TenantID.String())
	}
}

// refuseRotatingAnotherClientsSecret keeps a CLIENT-subject caller from minting
// a credential for a client that is not itself.
//
// THE ROW RULES OF THIS SERVICE, stated once: a tenant token writes inside its
// own tenant (the generated refuseForeignTenant), a `*:*` token crosses that
// scope, and a client token rotates only its own secret. This is the third.
//
// IT USED TO COVER EVERY UPDATE AND THE ARCHIVE, and a companion rule refused a
// client-subject INSERT outright. Both were narrowed away on 2026-08-28: they
// left a client unable to administer its tenant's other clients at all, which is
// closed past the point of usefulness and is not where the boundary belongs.
// Creating, editing, archiving, granting a role and editing the allow-list are
// ordinary tenant-scoped writes — the permission the caller carries is what says
// whether they may, exactly as it does for a user token.
//
// WHAT STAYS, AND WHY ONLY THIS. Rotating a secret is not editing a row: it mints
// a credential AND starts retiring the one in use. A machine that could rotate
// another machine's secret could lock it out and take its place — one call that
// is both a denial of service and an impersonation, executed by something
// unattended. That is a different act from every other verb here, and it is the
// one that stays on the caller's own row.
//
// A USER TOKEN NEVER MEETS THIS. The early return is on the KIND, so an operator
// holding client:rotate-secret rotates any client in their tenant, which is the
// helpdesk case and the mirror of User's reset.
//
// IT IS INERT TODAY, AND THAT IS DELIBERATE. RequestingIdentityKind is fed from
// the `identity_kind` claim, which only POST /auth/client/token mints and which
// does not exist yet. Until it does the field reads "", this stands down, and
// nothing changes. Do NOT add a fallback that infers the subject kind from
// another claim's absence: a security decision resting on a field that exists
// for a different reason changes behaviour the day that field does.
//
// It also stands down with NO IDENTITY AT ALL — the posture every other guard in
// this service takes, and provably a development bench: the middleware is only
// bypassable with auth.mode disabled, which the framework refuses outside
// APP_PROFILE=dev.
func (e *Client) refuseRotatingAnotherClientsSecret(r *domain.Rules) {
	if !e.RequestingIdentityPresent || e.RequestingIdentityKind != identityKindClient {
		return
	}

	rowID := ""
	if id := e.GetID(); id != nil {
		rowID = id.Value()
	}
	if e.RequestingClientID == "" || e.RequestingClientID != rowID {
		r.AddNotification("ID", ClientMayOnlyRotateItsOwnSecretNotification{})
	}
}

// refuseUngrantableRoles judges every role grant this write ADDS.
//
// The same three questions User asks of a direct grant, in the same order and
// for the same reasons — and they matter more here than they do there. A role
// granted to a person is exercised by a person who can be told no; a role
// granted to a machine is a non-expiring, non-interactive credential that does
// whatever it was configured to do at three in the morning.
//
// Mechanically this is GetAddedItemsOf and not GetCurrentItemsOf: the former
// crosses the original and current status, so a row loaded from the database is
// excluded and a re-granted one is not.
//
// WHY ONLY THE ADDED ONES. Re-judging stored grants makes unrelated writes
// hostages of the past: a role archived after the grant would make the client
// impossible to RENAME, and an operator who has since lost a permission could no
// longer even REVOKE the other grants — which is the tool for fixing exactly
// that situation.
func (e *Client) refuseUngrantableRoles(service ClientService, r *domain.Rules) {
	if service == nil {
		return
	}
	added := domain.GetAddedItemsOf[aggregatevos.ClientRole](&e.AggregateRoot)

	// The identity gate stands down when the request carried no identity at
	// all — auth.mode disabled, which the framework's own boot guard permits
	// only under APP_PROFILE=dev. An identity that IS present but holds an
	// insufficient claim still refuses; collapsing the two states would make the
	// entity unusable on a bench that has no tokens.
	identityGates := e.RequestingIdentityPresent

	for _, granted := range added {
		// USABLE, not merely non-empty. Every probe below hands this id to a
		// criterion against a UUID column, so a value uuid.Parse refuses makes
		// the query error and the probe panic: a 500 on a request whose problem
		// is plain validation. It raises nothing on purpose — the framework's
		// own child validation reports the bad id.
		if _, err := granted.RoleID.UUID(); err != nil {
			continue
		}

		// NEVER read granted.RoleKey / granted.RoleName here. They are read-join
		// fields: filled on entries LOADED from the row, and blank on an entry
		// this write just added — which is every entry in this loop.

		// ── role-available-in-tenant ──
		// In the table, still active, AND owned by THIS CLIENT's tenant. One
		// notification for all three: a distinct "belongs to another tenant"
		// reply confirms to a caller in tenant A that a specific UUID is a live
		// role in some other tenant.
		//
		// The tenant asked about is the ROW's, not the caller's — a `*:*`
		// super-admin crosses that scope, and when they do "this tenant" must
		// mean the client's.
		if service.RoleIsUnavailableInTenant(e.TenantID, granted.RoleID) {
			r.AddNotification("Roles", RoleNotAvailableInTenantNotification{}, granted.RoleID.String())
			continue
		}

		// ── no-wildcard-role ──
		// IT RUNS BEFORE THE ESCALATION RULE AND THAT ORDER IS LOAD-BEARING:
		// Identity.HasPermission PANICS on any argument containing '*', so this
		// is what removes the input that would turn the rule below into a 500 on
		// exactly the case it exists to stop.
		//
		// A machine credential carrying *:* is the single worst thing this
		// entity could mint: it never expires, nobody is watching it, and it
		// outlives the person who created it.
		if service.RoleGrantsWildcard(granted.RoleID) {
			r.AddNotification("Roles", CannotGrantWildcardRoleNotification{}, granted.RoleID.String())
			continue
		}

		// ── no-escalation ──
		// TWO HOPS: the role grants permissions, and the caller must hold every
		// one of them. A *:* superadmin passes by construction — HasPermission
		// answers true for any concrete permission when the claim set contains
		// the wildcard.
		if identityGates && service.CallerLacksAnyPermissionOfRole(granted.RoleID) {
			r.AddNotification("Roles", CannotGrantRoleWithUnheldPermissionsNotification{}, granted.RoleID.String())
		}
	}
}

// newClientSecret draws one secret from the operating system's random source.
//
// base64url WITHOUT padding, so the value is URL- and header-safe and every
// secret this service has ever issued is the same length.
//
// IT PANICS ON A FAILED RANDOM SOURCE, and that is the project's established
// answer rather than a shortcut — the Argon2id adapter's own header states it:
// the write pipeline turns a panic into a 500 with the transaction rolled back,
// which is the correct outcome for "this process cannot produce credentials
// right now". The alternative is worse in both directions: a notification would
// tell a caller to fix something they did not do, and continuing with a weaker
// source would issue a credential that looks exactly like a real one.
func newClientSecret() string {
	buf := make([]byte, clientSecretRandBytes)
	if _, err := rand.Read(buf); err != nil {
		panic("Client: the random source failed while minting a secret")
	}
	return clientSecretPrefix + base64.RawURLEncoding.EncodeToString(buf)
}

// refuseUnsettableClaims judges every claim value this write ADDS or CHANGES.
//
// The twin of User.refuseUnsettableClaims, and deliberately a separate method
// rather than something generic over both parents: the two ask DIFFERENT
// service facts (ClaimDoesNotApplyToClient here, ...ToUser there), and folding
// them together would hide which side a refusal came from.
//
// ADDED **and** CHANGED. The change verb is a PATCH carrying only `value`
// (`change.shape: patch`, `patchExcludes: [ClaimID]`), so a correction
// arrives with the definition read off the stored entry and a value nothing
// has judged yet — exactly as unjudged as a new one. Judging only the
// additions would let a correction write anything at all.
//
// NO wildcard probe and NO escalation probe, unlike the roles loop above.
// Those exist because a role confers permissions and a machine credential
// carrying *:* is the worst thing this entity could mint. A claim confers
// nothing inside this service — no BuildRules reads it, no route is gated on
// it, and there is no permission set to compare a caller against.
//
// It also has nothing to do with refuseRotatingAnotherClientsSecret: that rule
// is about handing out a CREDENTIAL, and setting a claim value is ordinary
// tenant-scoped administration.
func (e *Client) refuseUnsettableClaims(service ClientService, r *domain.Rules) {
	if service == nil {
		return
	}

	touched := append(
		domain.GetAddedItemsOf[aggregatevos.ClientClaim](&e.AggregateRoot),
		domain.GetChangedItemsOf[aggregatevos.ClientClaim](&e.AggregateRoot)...,
	)

	for _, held := range touched {
		// USABLE, not merely non-empty — every probe below hands this id to a
		// criterion against a UUID column. It raises nothing: the framework's
		// own child validation reports the bad id.
		if _, err := held.ClaimID.UUID(); err != nil {
			continue
		}

		// NEVER read held.ClaimName / held.ClaimValueType here — read-join
		// fields, blank on an entry this write just added. The type check asks
		// the SERVICE for the definition's value type instead of reading the
		// field sitting right there, which on an added entry is "" and would
		// refuse every value for the wrong reason.

		// ── claim-available-in-tenant ──
		// Present, active, and owned by THIS CLIENT's tenant. One notification
		// for all three: a distinct "belongs to another tenant" reply is an
		// existence oracle over a competitor's claim vocabulary.
		if service.ClaimIsUnavailableInTenant(e.TenantID, held.ClaimID) {
			r.AddNotification("Claims", ClaimNotAvailableInTenantNotification{}, held.ClaimID.String())
			continue
		}

		// ── claim-applies-to-client ──
		// A definition declaring appliesTo: user is refused here, and its twin
		// on User refuses appliesTo: client. The pair is what makes that column
		// mean something instead of merely stating it.
		if service.ClaimDoesNotApplyToClient(held.ClaimID) {
			r.AddNotification("Claims", ClaimDoesNotApplyToClientNotification{}, held.ClaimID.String())
			continue
		}

		// ── claim-value-matches-value-type ──
		// The same three readings the catalog's own default-value check uses.
		// The rejected VALUE is echoed rather than the id: the caller knows
		// which entry they sent, and what they need told back is the string
		// that did not parse.
		if service.ClaimValueDoesNotMatchValueType(held.ClaimID, held.Value.Value()) {
			r.AddNotification("Claims", ClaimValueDoesNotMatchValueTypeNotification{}, held.Value.Value())
		}
	}
}
