// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for Role (4 to implement).
//
// entity:     Role
// spec:       specs/omnicore-gen/role.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-21
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
	r.IfInsert(func() {
		// ── tenant-must-be-active ──
		// A role under a tenant that is gone would be grantable and
		// unreachable at once. Insert only: the tenant is immutable
		// afterwards (role-tenant-immutable), so a tenant archived LATER must
		// not freeze edits to the roles that already exist under it — those
		// rows are exactly what an access review still needs to read.
		//
		// An empty id is refused without asking the database: the probe would
		// answer "not found" anyway, and this is the one write where the
		// answer is knowable without a query.
		if e.TenantID.IsEmpty() || service.(RoleService).TenantIsUnavailable(e.TenantID) {
			r.AddNotification("TenantID", RoleTenantDoesNotExistNotification{})
		}
	})

	r.IfInsertOrUpdate(func() {
		// The three grant rules below are ONE walk of the collection, not
		// three. They ask the same question of the same entry — "what is this
		// grant, and may this caller make it?" — so walking three times would
		// triple the per-entry probes for no extra answer.
		e.refuseUngrantablePermissions(service, r)
	})
}

// refuseUngrantablePermissions enforces the three rules that judge a grant:
// no-wildcard-grant, granted-permission-must-be-in-catalog and
// caller-must-hold-granted-permission.
//
// ORDER IS LOAD-BEARING, and it is the reason these are three rules rather than
// one. Identity.HasPermission PANICS on any argument containing '*', so the
// naive form of the no-escalation rule — ask HasPermission for every granted
// key — crashes the request into a 500 the moment somebody tries to grant the
// '*:*' catalog row, which is precisely the case the rule exists to stop.
//
// So the walk is catalog → wildcard → caller-holds, and each step abandons the
// entry when it refuses:
//
//   - CATALOG first, because the wildcard fact deliberately answers true for an
//     id it cannot resolve. Asking it first would report an unknown permission
//     as a wildcard grant — a 403 blaming the caller for privilege escalation
//     when the honest answer is a 422 saying the permission is not there.
//   - WILDCARD before caller-holds, which is the guarantee that matters: no
//     wildcard string ever reaches HasPermission.
//   - CALLER-HOLDS last, by which point every key is concrete and resolvable.
//
// Each notification is raised AT MOST ONCE. A role may carry up to 200 grants
// (role-permission-cap), and a caller who pasted the wrong list learns nothing
// from reading the same refusal two hundred times.
func (e *Role) refuseUngrantablePermissions(service domain.Service, r *domain.Rules) {
	svc := service.(RoleService)

	var wildcardRaised, catalogRaised, unheldRaised bool

	for _, grant := range domain.GetCurrentItemsOf[aggregatevos.RolePermission](e.GetAggregateRoot()) {
		// ── granted-permission-must-be-in-catalog ──
		// Asked FIRST so an unresolvable id is reported as what it is. The
		// foreign key already guarantees EXISTENCE; this earns its keep for
		// the ACTIVE half — a permission the platform has retired — and for
		// turning both into a readable 422 rather than a raw constraint error
		// arriving alone, after everything else passed.
		if svc.PermissionIsNotInCatalog(grant.PermissionID) {
			if !catalogRaised {
				r.AddNotification("Permissions", PermissionNotInCatalogNotification{})
				catalogRaised = true
			}
			// Neither remaining question has anything to work with: the id
			// resolves to no key at all.
			continue
		}

		// ── no-wildcard-grant ──
		// A wildcard permission is grantable to nobody through this API.
		// Consequence, accepted: the platform's own '*:*' role is seeded by
		// migration beside the reserved platform tenant, not created here.
		//
		// The fact also answers true for an unresolvable id, which the catalog
		// question above has already caught and reported more precisely. That
		// overlap is deliberate defence in depth, not redundancy: it is what
		// guarantees an unvetted value can never reach HasPermission, even if
		// this walk were ever reordered.
		if svc.PermissionIsWildcard(grant.PermissionID) {
			if !wildcardRaised {
				r.AddNotification("Permissions", CannotGrantWildcardPermissionNotification{})
				wildcardRaised = true
			}
			// The caller-holds question must never see a wildcard: it panics.
			continue
		}

		// ── caller-must-hold-granted-permission ──
		// No privilege escalation: you may only grant what you hold. Every key
		// reaching here is concrete, so the fact can ask the framework safely.
		// A '*:*' super-admin needs no special case — HasPermission answers
		// true for any concrete permission when the claim set carries the
		// wildcard, so the exemption falls out of the framework rather than
		// out of claim parsing of our own.
		if !unheldRaised && svc.CallerDoesNotHoldPermission(grant.PermissionID) {
			r.AddNotification("Permissions", CannotGrantUnheldPermissionNotification{})
			unheldRaised = true
		}
	}
}
