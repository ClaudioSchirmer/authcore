// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for Role (4 to implement).
//
// entity:     Role
// spec:       specs/omnicore-gen/role.omnicore.yaml
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
func (e *Role) customRules(actionName string, service domain.Service, r *domain.Rules) {
	roleService := service.(RoleService)

	r.IfInsert(func() {
		// ── tenant-must-exist ──
		// The owning tenant must exist and must not be archived. A read join
		// cannot answer this, for two independent reasons: a join's predicate
		// is always fk = target.id and roles.tenant_id points at
		// tenants.tenant_id, the derived PUBLIC key; and on an insert the root
		// was built by the mapper a millisecond ago, so nothing traversed a
		// foreign key and every joined field is blank anyway.
		// NO GUARD ON THE OWNER HERE, and that is a property of the spec rather
		// than an omission. The `tenant-is-a-usable-id` rule pulls domain.ID's
		// own IsValid forward and carries `guard: true`, so an owner that is
		// empty or malformed ends the validation pass BEFORE this hook is
		// reached — measured: with "" or "tatu" this function does not run at
		// all. Anything arriving here has already parsed as a UUID.
		//
		// Drop that rule from the spec and this stops being true: the probe
		// below would take the bad value, bind it to a UUID column and panic
		// into a 500. That is the bug this entity actually shipped once.
		if roleService.TenantIsUnavailable(e.TenantID) {
			r.AddNotification("TenantID", RoleTenantDoesNotExistNotification{}, e.TenantID.String())
		}
	})

	r.IfInsertOrUpdate(func() {
		e.refuseUngrantablePermissions(roleService, r)
	})
}

// refuseUngrantablePermissions walks the grants this write ADDS exactly once and
// applies the three rules that judge them, in the order they must run.
//
// ONE pass, not three. The three rules ask about the same entries and the same
// catalog rows, so walking the collection once is what keeps a role at the
// 200-permission cap from paying three traversals for one write; the service
// funnels their questions through a single memoised read per entry.
//
// WHY ONLY THE ADDED ENTRIES. All three ask about the ACT of granting, and a
// grant already in the row was asked all three when it entered. Re-asking them
// makes unrelated writes hostages of the past: a permission the platform retires
// AFTER a grant would make the role impossible to rename — a 422 on a request
// whose only change is a label — and a caller who has since lost a permission
// could no longer even REVOKE the others, since a revocation is an update and
// every remaining grant would be re-judged against a claim set that no longer
// holds them. Neither refusal describes anything the caller is doing.
//
// An INSERT is unchanged by this: every entry of a new role is an added one, so
// the whole collection is still judged there. A REVOKE asks nothing, because it
// adds nothing.
//
// Mechanically this is GetAddedItemsOf and not GetCurrentItemsOf: the former
// crosses the original and current status (OpInsert), so a row loaded from the
// database is excluded and a re-granted one is not.
func (e *Role) refuseUngrantablePermissions(service RoleService, r *domain.Rules) {
	// No early return on an empty set: ranging over it already does nothing,
	// and a return at the top of a rule is the shape that hides work from the
	// spec. Stopping a validation pass is `guard: true`'s job, and it says so
	// in the yaml.
	added := domain.GetAddedItemsOf[aggregatevos.RolePermission](&e.AggregateRoot)

	// The identity gate stands down when the request carried no identity at
	// all — auth.mode disabled, which the framework's own boot guard permits
	// only under APP_PROFILE=dev. An identity that IS present but holds an
	// insufficient claim still refuses, which is the whole point of keeping
	// these two states apart: collapsing them into one fail-closed test would
	// make the entity unusable on a bench that has no tokens.
	identityGates := e.RequestingIdentityPresent

	for _, grant := range added {
		// USABLE, not merely non-empty. Every probe below hands this id to a
		// criterion against a UUID column, so a value uuid.Parse refuses — ""
		// and "tatu" alike — makes the query error and the probe panic: a 500
		// on a request whose problem is plain validation.
		//
		// Testing IsEmpty alone was the bug, and it is the same bug the ROOT's
		// owner had: the empty case is the one you think of, the malformed one
		// is the one that reaches production. The root is covered by its
		// `valueObject` barrier; a child id is not, because the framework
		// validates children AFTER the rules and this loop runs inside them.
		//
		// It raises nothing on purpose: the framework's own child validation
		// reports the bad id, so complaining here would say it twice.
		if _, err := grant.PermissionID.UUID(); err != nil {
			continue
		}

		// NEVER read grant.Resource / grant.Action / grant.ArchivedAt here.
		// They are read-join fields: filled on entries LOADED from the row, and
		// blank on an entry this write just added — which is every entry in this
		// loop. Resource would read "" and ArchivedAt would read nil, the same
		// nil a live permission carries, so a test against them would wave
		// through exactly the grants these rules exist to judge. The service
		// asks the catalog instead.

		// ── granted-permissions-are-in-the-catalog ──
		// Every permission this write adds must exist in the catalog and still
		// be active. A retired permission comes back as a NEW row with a NEW id,
		// so re-granting the old id is refused rather than silently honoured.
		if service.PermissionIsNotInCatalog(grant.PermissionID) {
			r.AddNotification("Permissions", PermissionNotInCatalogNotification{}, grant.PermissionID.String())
			// Nothing below can say anything true about a permission that is
			// not there, and the wildcard probe already answers "yes" for an
			// unknown id — reporting all three for one bad id would be noise.
			continue
		}

		// ── no-wildcard-grant ──
		// A permission with a wildcard in either part cannot be granted on any
		// role through this API.
		//
		// IT RUNS BEFORE THE ESCALATION RULE AND THAT ORDER IS LOAD-BEARING:
		// Identity.HasPermission PANICS on any argument containing '*', so this
		// rule is what removes the input that would crash the request into a
		// 500 — on exactly the case the escalation rule exists to stop. The
		// service guards the wildcard a second time for the same reason.
		if service.PermissionIsWildcard(grant.PermissionID) {
			r.AddNotification("Permissions", CannotGrantWildcardPermissionNotification{}, grant.PermissionID.String())
			continue
		}

		// ── no-privilege-escalation ──
		// A caller may only grant a permission they themselves hold. Every key
		// reaching here is concrete, because the wildcard refusal above already
		// removed the rest.
		//
		// The *:* superadmin exemption is free rather than special-cased:
		// HasPermission returns true for ANY concrete permission when the claim
		// set contains *:*, so "you may only grant what you hold, unless you are
		// a superadmin" is one question and not two.
		if identityGates && service.CallerDoesNotHoldPermission(grant.PermissionID) {
			r.AddNotification("Permissions", CannotGrantUnheldPermissionNotification{}, grant.PermissionID.String())
		}
	}
}
