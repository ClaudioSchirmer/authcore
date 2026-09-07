// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for User (10 to implement).
//
// entity:     User
// spec:       specs/omnicore-gen/user.omnicore.yaml
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
	"strings"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The floor on a name word before it can ban a password containing it.
//
// Without it a family name like "Ng" would refuse every password containing
// "ng", which is most of them — a rule that reads sensible and locks a
// population out.
const passwordIdentityWordMinRunes = 4

// customRules is called at the end of the generated BuildRules, with the same
// arguments, and reports a violation the same way: r.AddNotification.
//
// ONE gate per verb, holding every rule that runs on that verb — the shape
// the generated BuildRules already has. A gate per rule reads as a wall of
// near-identical closures and makes the framework dispatch the same verb once
// per rule on every write; rules that share a verb belong in the same block. A
// rule that appears under two gates is still ONE rule: write it as a method
// and call it from both, rather than as two copies that can drift apart.
func (e *User) customRules(actionName string, service domain.Service, r *domain.Rules) {
	svc, _ := service.(UserService)

	r.IfInsert(func() {
		e.deriveCredential(svc)
		e.refusePasswordEchoingIdentity(r)
		e.refuseUnavailableTenant(svc, r)
	})

	r.IfInsertOrUpdate(func() {
		e.refuseUnjoinableGroups(svc, r)
		e.refuseUngrantableRoles(svc, r)
		e.refuseUnsettableClaims(svc, r)
	})

	r.IfUpdate(func() {
		// All three of these dispatch ModeUpdate — the ordinary PATCH included —
		// so the ACTION NAME is what tells them apart. Without this switch the
		// password checks would fire on a rename, against a field that write
		// never carried, and the two credential operations would be
		// indistinguishable from each other.
		switch actionName {
		case ActionChangePassword:
			e.changePasswordRules(svc, r)
		case ActionResetPassword:
			e.resetPasswordRules(svc, r)
		}
	})

	r.IfArchive(func() {
		// ── archive-forces-suspended ──
		// A MUTATION, not a validation: set the field and raise nothing. An
		// archived user is never active, so archived+active becomes an
		// unrepresentable state rather than a refused one — there is no state
		// to validate against and no report that has to filter it out.
		//
		// It reaches the ROW and not merely the audit event, because archive is
		// an ordinary full-field write at this pin: it emits the same UPDATE
		// every other verb does, with archived_at riding along as one more
		// column.
		//
		// It cannot collide with the status transition rule: that one is
		// IfUpdate, this is IfArchive, and the two modes never run together.
		//
		// Verbatim the shape Tenant already ships.
		e.Status = vos.UserStatusSuspended
	})
}

// deriveCredential fills every field the spec declared `assignedFrom: derived`.
//
// ONE method for all six, because they are one act: a user is created WITH a
// credential, and the five fields around the hash are what that credential's
// state looks like at minute zero. Splitting them into six rules would be six
// gates the framework dispatches separately to write one row.
//
// THE PLAINTEXT LEAVES NO TRACE HERE. It arrives on a field with no column,
// goes into the hasher, and the hash is what lands. Nothing in this method
// logs, wraps, or copies it — and there is nowhere it could leak to even by
// accident, because the field it came from is in no TableSchema, no payload,
// no audit event and no response.
//
// The hash comes through the SERVICE and not from a crypto package imported
// here: an aggregate that knew Argon2id would be an aggregate that changes when
// the parameters do.
func (e *User) deriveCredential(service UserService) {
	// A nil service is the framework's own failure mode, not this rule's — it
	// answers ServiceIsRequiredNotification before the rules run — but the type
	// assertion above can still yield nil in a test that builds the entity by
	// hand. Leaving the hash empty is the honest outcome there: no password
	// matches an empty hash, so the row authenticates nobody.
	if service != nil {
		e.PasswordHash = service.HashPassword(e.Password.Value())
	}

	now := time.Now().UTC()
	e.PasswordChangedAt = now

	// The creator knows the password they chose, so the next sign-in has to
	// rotate it. This is what makes an admin-set initial password tolerable at
	// all, and it is the flag the public change-password route clears.
	e.MustChangePassword = true

	// EmailVerifiedAt is deliberately LEFT NIL. Nothing in this service verifies
	// an address — there is no token store and no outbound mail path — so
	// writing anything here would be a claim the service cannot support, and
	// nil is how the wire says "not verified" to a caller. The column exists so the flow, when it is built, is code only.
}

// refusePasswordEchoingIdentity refuses a password built out of the user's own
// identity.
//
// NIST SP 800-63B-4 forbids composition requirements; it does NOT forbid this,
// and it recommends it. The two are different things: a class rule says what
// shape a password must have, this says the password must not be the one thing
// an attacker already knows about the account. A password that IS the e-mail's
// local part is the single most guessable credential this service can issue.
//
// Case-insensitive and over RUNES, so "Maria" in the name refuses "maria2024!"
// in every one of the seven languages this service serves — a byte-wise or
// ASCII-folded comparison would miss "José" and "Müller" exactly where it
// matters most.
//
// The four-rune floor on name words is what keeps it from refusing everything:
// without it, a family name like "Ng" would ban every password containing "ng",
// which is most of them.
func (e *User) refusePasswordEchoingIdentity(r *domain.Rules) {
	password := strings.ToLower(e.Password.Value())
	if password == "" {
		// The value object already answered this one with
		// RequiredFieldNotification. Saying it again would be the duplicate
		// that every rule in this project is careful not to be.
		return
	}

	// The local part only. The domain is shared by every colleague, so banning
	// it would refuse "acme" for the entire company — a rule that reads
	// sensible and blocks half of a tenant's users.
	local := strings.ToLower(e.Email.Value())
	if at := strings.Index(local, "@"); at > 0 {
		local = local[:at]
	}

	echoes := local != "" && strings.Contains(password, local)
	if !echoes {
		for _, half := range []string{e.Name.Given, e.Name.Family} {
			for _, word := range strings.Fields(strings.ToLower(half)) {
				if len([]rune(word)) >= passwordIdentityWordMinRunes && strings.Contains(password, word) {
					echoes = true
					break
				}
			}
			if echoes {
				break
			}
		}
	}

	if echoes {
		// The value is NOT echoed back. Every other rule in this project passes
		// the offending input so a caller can see what was refused; here that
		// would put the plaintext in the 422 payload and in any log rendering
		// one.
		r.AddNotification(&e.Password, PasswordEchoesIdentityNotification{}, false)
	}
}

// refuseUnavailableTenant refuses a user whose owning tenant is missing,
// archived or commercially SUSPENDED.
//
// THREE conditions, and TRIAL PASSES. "Unavailable" is not "not active": a
// trial is a live customer being onboarded, and users are the first thing they
// need. Reading this as `Status != active` would break every trial signup.
//
// IfInsert only. Re-asking on every update would make a user impossible to
// rename the day their tenant is suspended — a 422 on a request whose only
// change is a label, on rows that already exist and still have to be
// administrable. Suspension withholds NEW users; it does not freeze the ones
// already there.
func (e *User) refuseUnavailableTenant(service UserService, r *domain.Rules) {
	if service == nil {
		return
	}
	if service.TenantIsUnavailable(e.TenantID) {
		r.AddNotification(&e.TenantID, UserTenantDoesNotExistNotification{}, true)
	}
}

// refuseUnjoinableGroups judges every group membership this write ADDS.
//
// Three rules in one loop, in an order that is load-bearing — see the comments
// at each step. Mechanically this is GetAddedItemsOf and not GetCurrentItemsOf:
// the former crosses the original and current status, so a row loaded from the
// database is excluded and a re-joined one is not.
//
// WHY ONLY THE ADDED ONES. Re-judging stored memberships makes unrelated writes
// hostages of the past: a group archived after the user joined it would make
// the user impossible to RENAME, and an operator who has since lost a
// permission could no longer even REMOVE the other memberships — which is the
// tool for fixing exactly that situation.
func (e *User) refuseUnjoinableGroups(service UserService, r *domain.Rules) {
	if service == nil {
		return
	}
	added := domain.GetAddedItemsOf[aggregatevos.UserGroup](&e.AggregateRoot)

	// The identity gate stands down when the request carried no identity at
	// all — auth.mode disabled, which the framework's own boot guard permits
	// only under APP_PROFILE=dev. An identity that IS present but holds an
	// insufficient claim still refuses; collapsing the two states would make
	// the entity unusable on a bench that has no tokens.
	identityGates := e.RequestingIdentityPresent

	// USABLE IDS ONLY, and the filter runs BEFORE the questions rather than
	// inside the loop. Every fact below hands these ids to a criterion against
	// a UUID column, so a value uuid.Parse refuses makes the query error and
	// the probe panic: a 500 on a request whose problem is plain validation. It
	// raises nothing on purpose — the framework's own child validation reports
	// the bad id.
	judged := make([]aggregatevos.UserGroup, 0, len(added))
	groupIDs := make([]domain.ID, 0, len(added))
	for _, joined := range added {
		if _, err := joined.GroupID.UUID(); err != nil {
			continue
		}
		judged = append(judged, joined)
		groupIDs = append(groupIDs, joined.GroupID)
	}
	if len(judged) == 0 {
		return
	}

	// THREE QUESTIONS, ASKED ONCE EACH FOR THE WHOLE COLLECTION. The facts are
	// `perEntry`, so each answers a map keyed by the entry's id and the service
	// resolves the set in one read — a user joining ten groups costs what one
	// costs. The verdicts are still per entry: the loop below reads them.
	//
	// EVERY QUESTION IS ASKED OF EVERY ENTRY, including entries the first
	// answer already refuses. The old shape skipped the later probes for a bad
	// entry to save round trips; there are no round trips left to save, and the
	// answers are identical — an unresolvable group reads TRUE from the
	// wildcard fact either way. What did NOT change is what the caller is told:
	// the interlock below still reports one problem per entry.
	//
	// The escalation fact is not asked at all when the request carried no
	// identity: there is nobody to compare the bundles against.
	unavailable := service.GroupIsUnavailableInTenant(e.TenantID, groupIDs)
	grantsWildcard := service.GroupGrantsWildcard(groupIDs)

	var escalates map[domain.ID]bool
	if identityGates {
		escalates = service.CallerLacksAnyPermissionOfGroup(groupIDs)
	}

	for _, joined := range judged {
		// NEVER read joined.GroupKey / joined.GroupName here. They are
		// read-join fields: filled on entries LOADED from the row, and blank on
		// an entry this write just added — which is every entry in this loop.
		// Both would read "", indistinguishable from a group whose key is
		// genuinely empty, so a test against them would wave through exactly
		// the memberships these rules exist to judge.

		// ── group-available-in-tenant ──
		// In the table, still active, AND owned by THIS USER's tenant. One
		// notification for all three: a distinct "belongs to another tenant"
		// reply confirms to a caller in tenant A that a specific UUID is a live
		// group in some other tenant — an existence oracle over a competitor's
		// org chart.
		//
		// The tenant asked about is the ROW's, not the caller's. On the
		// ordinary path they are the same value, but a *:* super-admin crosses
		// that scope, and when they do "this tenant" must mean the user's.
		if unavailable[joined.GroupID] {
			r.AddNotificationNamed("Groups", GroupNotAvailableInTenantNotification{}, joined.GroupID.String())
			// Nothing below can say anything true about a group that is not
			// there, and the wildcard probe already answers "yes" for an
			// unknown id — reporting all three for one bad id would be noise.
			continue
		}

		// ── group-wildcard-refused ──
		// IT RUNS BEFORE THE ESCALATION RULE AND THAT ORDER IS LOAD-BEARING:
		// Identity.HasPermission PANICS on any argument containing '*', so this
		// is what removes the input that would crash the request into a 500 on
		// exactly the case the escalation rule exists to stop.
		//
		// Consequence, accepted and consistent with the rest of the service:
		// the platform's own superadmin user cannot be created through this
		// API. It is seeded by migration beside the reserved platform tenant.
		if grantsWildcard[joined.GroupID] {
			r.AddNotificationNamed("Groups", CannotJoinWildcardGroupNotification{}, joined.GroupID.String())
			continue
		}

		// ── group-no-escalation ──
		// THREE HOPS: the group confers roles, each role grants permissions,
		// and the caller must hold every one of them. A set, not one key.
		// A *:* superadmin passes by construction — HasPermission answers true
		// for any concrete permission when the claim set contains it.
		if escalates[joined.GroupID] {
			r.AddNotificationNamed("Groups", CannotJoinGroupWithUnheldPermissionsNotification{}, joined.GroupID.String())
		}
	}
}

// refuseUngrantableRoles judges every direct role grant this write ADDS.
//
// The same three questions one hop shorter, in the same order and for the same
// reasons. It is a separate method rather than a generic one over both
// collections because the two ask DIFFERENT service facts — a group's walk is
// three hops and a role's is two — and folding them together would hide which
// depth a refusal came from.
func (e *User) refuseUngrantableRoles(service UserService, r *domain.Rules) {
	if service == nil {
		return
	}
	added := domain.GetAddedItemsOf[aggregatevos.UserRole](&e.AggregateRoot)
	identityGates := e.RequestingIdentityPresent

	// The same batched shape the group half above documents: unusable ids
	// filtered first, three questions asked once each, verdicts read per entry.
	judged := make([]aggregatevos.UserRole, 0, len(added))
	roleIDs := make([]domain.ID, 0, len(added))
	for _, granted := range added {
		if _, err := granted.RoleID.UUID(); err != nil {
			continue
		}
		judged = append(judged, granted)
		roleIDs = append(roleIDs, granted.RoleID)
	}
	if len(judged) == 0 {
		return
	}

	unavailable := service.RoleIsUnavailableInTenant(e.TenantID, roleIDs)
	grantsWildcard := service.RoleGrantsWildcard(roleIDs)

	var escalates map[domain.ID]bool
	if identityGates {
		escalates = service.CallerLacksAnyPermissionOfRole(roleIDs)
	}

	for _, granted := range judged {
		// ── role-available-in-tenant ──
		if unavailable[granted.RoleID] {
			r.AddNotificationNamed("Roles", RoleNotAvailableInTenantNotification{}, granted.RoleID.String())
			continue
		}

		// ── role-wildcard-refused ──
		if grantsWildcard[granted.RoleID] {
			r.AddNotificationNamed("Roles", CannotGrantWildcardRoleNotification{}, granted.RoleID.String())
			continue
		}

		// ── role-no-escalation ──
		if escalates[granted.RoleID] {
			r.AddNotificationNamed("Roles", CannotGrantRoleWithUnheldPermissionsNotification{}, granted.RoleID.String())
		}
	}
}

// refuseUnsettableClaims judges every claim value this write ADDS or CHANGES.
//
// The three checks are the same shape the two collections above use, one hop
// each, and in the same deliberate order. What is different is the SET being
// judged: this is the only collection in the service that mounts a `change`
// verb, so an entry whose value was corrected has to be judged too. An added
// entry and a changed one are equally unvalidated — the value is new either
// way — and judging only the additions would let "correct this cost center"
// write anything at all.
//
// WHAT IS NOT HERE, and it is a decision rather than an omission: no wildcard
// probe and no escalation probe. The two collections above carry them because
// they confer PERMISSIONS, and the caller must already hold what they hand
// over. A claim confers nothing inside this service — it is in no BuildRules,
// it gates no route, and there is no permission set to compare a caller
// against, so a probe here would gate nothing. What answers that worry is the
// verb the routes declare: user:set-claim, its own, rather than user:grant.
func (e *User) refuseUnsettableClaims(service UserService, r *domain.Rules) {
	if service == nil {
		return
	}

	// ADDED **and** CHANGED. The change verb is a PATCH carrying only `value`
	// (`change.shape: patch`, `patchExcludes: [ClaimID]`), so a correction
	// arrives with the definition read off the stored entry and a value nothing
	// has judged yet — exactly as unjudged as a new one. Judging only the
	// additions would let a correction write anything at all.
	touched := append(
		domain.GetAddedItemsOf[aggregatevos.UserClaim](&e.AggregateRoot),
		domain.GetChangedItemsOf[aggregatevos.UserClaim](&e.AggregateRoot)...,
	)

	// USABLE IDS ONLY, filtered before anything is asked — every fact below
	// hands these ids to a criterion against a UUID column, so a value
	// uuid.Parse refuses would make the query error and the probe panic. It
	// raises nothing: the framework's own child validation reports the bad id.
	//
	// THE ENTRIES TRAVEL WHOLE, not as two parallel slices: the value-type
	// question needs the id AND the value of the SAME entry, and the generated
	// carrier is what keeps that pair from being misaligned by a caller.
	judged := make([]aggregatevos.UserClaim, 0, len(touched))
	claimIDs := make([]domain.ID, 0, len(touched))
	entries := make([]UserClaimValueDoesNotMatchValueTypeEntry, 0, len(touched))
	for _, held := range touched {
		if _, err := held.ClaimID.UUID(); err != nil {
			continue
		}
		judged = append(judged, held)
		claimIDs = append(claimIDs, held.ClaimID)
		entries = append(entries, UserClaimValueDoesNotMatchValueTypeEntry{
			ClaimID: held.ClaimID,
			Value:   held.Value.Value(),
		})
	}
	if len(judged) == 0 {
		return
	}

	// THREE QUESTIONS, ASKED ONCE EACH, exactly as the two collections above.
	// One read of the catalog answers all three however many claims the write
	// carries; the verdicts below are still per entry.
	unavailable := service.ClaimIsUnavailableInTenant(e.TenantID, claimIDs)
	notForUsers := service.ClaimDoesNotApplyToUser(claimIDs)
	valueMismatched := service.ClaimValueDoesNotMatchValueType(entries)

	for _, held := range judged {
		// NEVER read held.ClaimName / held.ClaimValueType here. They are
		// read-join fields — filled on entries LOADED from the row and blank on
		// one this write just added — so the type check below asks the SERVICE
		// for the definition's value type instead of reading the field sitting
		// right there. On an added entry that field is "", which no member
		// matches, and every value would be refused for the wrong reason.
		//
		// This is also why the check is not a value object: "parse as whatever
		// the definition says" reads a field of another aggregate, which no
		// value object can see.

		// ── claim-available-in-tenant ──
		// In the table, still active, AND owned by THIS USER's tenant. One
		// notification for all three: a distinct "belongs to another tenant"
		// reply confirms to a caller in tenant A that a specific UUID is a live
		// definition in some other tenant — an existence oracle over a
		// competitor's claim vocabulary.
		//
		// The tenant asked about is the ROW's, not the caller's: on the
		// ordinary path they are the same value, but a *:* super-admin crosses
		// that scope, and when they do "this tenant" must mean the user's.
		if unavailable[held.ClaimID] {
			r.AddNotificationNamed("Claims", ClaimNotAvailableInTenantNotification{}, held.ClaimID.String())
			// Nothing below can say anything true about a definition that is
			// not there, and both probes answer "the problem is present" for an
			// unknown id — reporting all three for one bad id would be noise.
			// Same interlock the group and role loops use.
			continue
		}

		// ── claim-applies-to-user ──
		// The definition declares which identity kinds may hold a value:
		// `user`, `client` or `both`. This is what finally makes that column
		// mean something rather than merely state it.
		if notForUsers[held.ClaimID] {
			r.AddNotificationNamed("Claims", ClaimDoesNotApplyToUserNotification{}, held.ClaimID.String())
			continue
		}

		// ── claim-value-matches-value-type ──
		// The value must parse as the type the definition declares, using the
		// SAME three readings the catalog's own default-value check uses — two
		// levels of one chain must not disagree about what a bool is.
		//
		// The rejected value is echoed rather than the id: the caller knows
		// which entry they sent, and what they need told back is the string
		// that did not parse.
		if valueMismatched[held.ClaimID] {
			r.AddNotificationNamed("Claims", ClaimValueDoesNotMatchValueTypeNotification{}, held.Value.Value())
		}
	}
}
