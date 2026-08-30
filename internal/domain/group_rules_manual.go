// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for Group (4 to implement).
//
// entity:     Group
// spec:       specs/omnicore-gen/group.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-24
//
// Until these are written the service runs and accepts writes — it simply
// does not enforce the invariants the spec described. That is quiet, which
// is exactly why it is worth doing now rather than later.
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

package domain

import (
	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// customRules is called at the end of the generated BuildRules, with the same
// arguments, and reports a violation the same way: r.AddNotification.
//
// ONE gate per verb, holding every rule that runs on that verb — the shape
// the generated BuildRules already has. A gate per rule reads as a wall of
// near-identical closures and makes the framework dispatch the same verb once
// per rule on every write; rules that share a verb belong in the same block. A
// rule that appears under two gates is still ONE rule: write it as a method
// and call it from both, rather than as two copies that can drift apart.
func (e *Group) customRules(actionName string, service domain.Service, r *domain.Rules) {
	groupService := service.(GroupService)

	r.IfInsert(func() {
		// ── tenant-must-be-available ──
		// The owning tenant must exist, must not be archived, and must not be
		// SUSPENDED. A TRIAL tenant is a live customer and passes: "unavailable"
		// is not the same question as "not active", and collapsing them would
		// lock every trial customer out of creating their own org chart.
		//
		// A read join cannot answer this. The traversal into Tenant IS declared
		// on this repository — TenantWorkspace and TenantStatus are right there
		// on the entity — but on an INSERT the root was built by the mapper a
		// millisecond ago, so nothing traversed a foreign key and both fields
		// are blank. Reading TenantStatus here would test "" against "suspended"
		// and wave every insert through, which is fail-open on the one verb this
		// rule exists to gate.
		//
		// NO GUARD ON THE OWNER HERE, and that is a property of the spec rather
		// than an omission. The `tenant-is-a-usable-id` rule pulls domain.ID's
		// own IsValid forward and carries `guard: true`, so an owner that is
		// empty or malformed ends the validation pass BEFORE this hook is
		// reached. Anything arriving here has already parsed as a UUID.
		//
		// Drop that rule from the spec and this stops being true: the probe
		// below would take the bad value, bind it to a UUID column and panic
		// into a 500. That is the bug Role actually shipped once, and the whole
		// reason G0 exists.
		if groupService.TenantIsUnavailable(e.TenantID) {
			r.AddNotification("TenantID", GroupTenantDoesNotExistNotification{}, e.TenantID.String())
		}
	})

	r.IfInsertOrUpdate(func() {
		e.refuseUnattachableRoles(groupService, r)
	})
}

// refuseUnattachableRoles walks the entries this write ATTACHES exactly once and
// applies the three rules that judge them, in the order they must run.
//
// ONE pass, not three. The three rules ask about the same entries and the same
// roles, so walking the collection once is what keeps a group at the 50-role cap
// from paying three traversals for one write; the service funnels their
// questions through a single memoised read per entry.
//
// WHY ONLY THE ADDED ENTRIES. All three ask about the ACT of attaching, and an
// entry already in the row was asked all three when it entered. Re-asking them
// makes unrelated writes hostages of the past: a role the tenant retires AFTER
// an attachment would make the group impossible to rename — a 422 on a request
// whose only change is a label — and a caller who has since lost a permission
// could no longer even DETACH the other roles, since a detach is an update and
// every remaining entry would be re-judged against a claim set that no longer
// holds them. Neither refusal describes anything the caller is doing.
//
// An INSERT is unchanged by this: every entry of a new group is an added one, so
// the whole collection is still judged there. A DETACH asks nothing, because it
// adds nothing — which is precisely what keeps detaching available as the tool
// for an attachment that stopped being acceptable.
//
// Mechanically this is GetAddedItemsOf and not GetCurrentItemsOf: the former
// crosses the original and current status (OpInsert), so a row loaded from the
// database is excluded and a re-attached one is not.
func (e *Group) refuseUnattachableRoles(service GroupService, r *domain.Rules) {
	// No early return on an empty set: ranging over it already does nothing,
	// and a return at the top of a rule is the shape that hides work from the
	// spec. Stopping a validation pass is `guard: true`'s job, and it says so
	// in the yaml.
	added := domain.GetAddedItemsOf[aggregatevos.GroupRole](&e.AggregateRoot)

	// The identity gate stands down when the request carried no identity at
	// all — auth.mode disabled, which the framework's own boot guard permits
	// only under APP_PROFILE=dev. An identity that IS present but holds an
	// insufficient claim still refuses, which is the whole point of keeping
	// these two states apart: collapsing them into one fail-closed test would
	// make the entity unusable on a bench that has no tokens.
	identityGates := e.RequestingIdentityPresent

	// USABLE IDS ONLY, and the filter runs BEFORE the questions rather than
	// inside the loop. Every fact below hands these ids to a criterion against a
	// UUID column, so a value uuid.Parse refuses — "" and "tatu" alike — makes
	// the query error and the probe panic: a 500 on a request whose problem is
	// plain validation.
	//
	// Testing IsEmpty alone was the bug on Role, and it is the same bug the
	// ROOT's owner had: the empty case is the one you think of, the
	// malformed one is the one that reaches production. The root is covered
	// by its `valueObject` barrier; a child id is not, because the framework
	// validates children AFTER the rules and this loop runs inside them.
	//
	// It raises nothing on purpose: the framework's own child validation
	// reports the bad id, so complaining here would say it twice.
	judged := make([]aggregatevos.GroupRole, 0, len(added))
	roleIDs := make([]domain.ID, 0, len(added))
	for _, attached := range added {
		if _, err := attached.RoleID.UUID(); err != nil {
			continue
		}
		judged = append(judged, attached)
		roleIDs = append(roleIDs, attached.RoleID)
	}
	if len(judged) == 0 {
		return
	}

	// THREE QUESTIONS, ASKED ONCE EACH FOR THE WHOLE COLLECTION. The facts are
	// `perEntry`, so each answers a map keyed by the entry's id and the service
	// resolves the set in one read — a group attaching ten roles costs what one
	// costs. The verdicts are still per entry: the loop below reads them.
	//
	// EVERY QUESTION IS ASKED OF EVERY ENTRY, including entries the first answer
	// already refuses. There are no round trips left to save by skipping them,
	// and the answers are identical — an unresolvable role reads TRUE from the
	// wildcard fact either way. What did NOT change is what the caller is told:
	// the interlock below still reports one problem per entry.
	unavailable := service.RoleIsUnavailableInTenant(e.TenantID, roleIDs)
	grantsWildcard := service.RoleGrantsWildcard(roleIDs)

	var escalates map[domain.ID]bool
	if identityGates {
		escalates = service.CallerLacksAnyPermissionOf(roleIDs)
	}

	for _, attached := range judged {

		// NEVER read attached.RoleKey / attached.RoleName here. They are
		// read-join fields: filled on entries LOADED from the row, and blank on
		// an entry this write just added — which is every entry in this loop.
		// Both would read "", which is indistinguishable from a role whose key
		// is genuinely empty, so a test against them would wave through exactly
		// the attachments these rules exist to judge. A security rule that
		// passes on a blank field is the worst possible failure direction. The
		// service asks the roles table instead.

		// ── attached-roles-are-available-in-this-tenant ──
		// The role must be in the table, still active, AND owned by THIS
		// group's tenant. The third question is the first thing in this entity
		// that Role did not need: Permission is a global catalog, Role is
		// tenant-scoped, so a group in tenant A attaching tenant B's role would
		// confer another customer's permissions on A's members — a cross-tenant
		// leak through a route that looks like ordinary group editing.
		//
		// One notification for all three, deliberately. A distinct "that role
		// belongs to another tenant" message confirms to a caller in tenant A
		// that a specific UUID is a live role in some OTHER tenant, which is an
		// existence oracle over a competitor's org structure. Same reasoning
		// that makes a cross-tenant by-id read answer 404 rather than 403.
		//
		// The tenant it is asked about is the ROW's, not the caller's. On the
		// ordinary path they are the same value — refuseForeignTenant already
		// refused anything else — but a *:* super-admin crosses that scope, and
		// when they do, "this tenant" must mean the group's.
		if unavailable[attached.RoleID] {
			r.AddNotification("Roles", RoleNotAvailableInTenantNotification{}, attached.RoleID.String())
			// Nothing below can say anything true about a role that is not
			// there, and the wildcard probe already answers "yes" for an
			// unknown id — reporting all three for one bad id would be noise.
			continue
		}

		// ── no-wildcard-role-attach ──
		// A role granting a permission with a wildcard in either part cannot be
		// conferred by any group through this API.
		//
		// IT RUNS BEFORE THE ESCALATION RULE AND THAT ORDER IS LOAD-BEARING:
		// Identity.HasPermission PANICS on any argument containing '*', so this
		// rule is what removes the input that would crash the request into a
		// 500 — on exactly the case the escalation rule exists to stop. The
		// service guards the wildcard a second time for the same reason.
		//
		// It also runs AFTER the availability rule, and that half of the order
		// is load-bearing too: RoleGrantsWildcard answers TRUE for an id it
		// cannot resolve, so an unknown role reaching it first would be reported
		// as an escalation attempt — a 403 blaming the caller where the honest
		// answer is a 422 saying the role is not there.
		//
		// What it costs, stated plainly: the platform's own superadmin group —
		// the one carrying the *:* role — cannot be created through this API,
		// and is seeded by migration beside the reserved platform tenant.
		if grantsWildcard[attached.RoleID] {
			r.AddNotification("Roles", CannotGrantWildcardRoleNotification{}, attached.RoleID.String())
			continue
		}

		// ── no-privilege-escalation ──
		// TRANSITIVE: a caller may confer a role only if they hold EVERY
		// permission that role grants. A set, not one key — which is what makes
		// this rule different from Role's, one level down: a role bundles
		// permissions, a group bundles bundles.
		//
		// Every key reaching here is concrete, because the wildcard refusal
		// above already removed the rest.
		//
		// The *:* superadmin exemption is free rather than special-cased:
		// HasPermission returns true for ANY concrete permission when the claim
		// set contains *:*, so "you may only confer what you hold, unless you
		// are a superadmin" is one question and not two.
		if escalates[attached.RoleID] {
			r.AddNotification("Roles", CannotGrantRoleWithUnheldPermissionsNotification{}, attached.RoleID.String())
		}
	}
}
